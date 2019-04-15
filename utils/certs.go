package utils

import (
	"crypto"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/18F/cf-cdn-service-broker/config"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/go-acme/lego/certcrypto"
	"github.com/go-acme/lego/lego"
	"github.com/go-acme/lego/registration"
)

type User struct {
	Email string
	// todo (mxplusb): fix this soon, deprecated!
	Registration *registration.Resource
	key          crypto.PrivateKey
}

func (u *User) GetEmail() string {
	return u.Email
}

func (u *User) GetRegistration() *registration.Resource {
	return u.Registration
}

func (u *User) GetPrivateKey() crypto.PrivateKey {
	return u.key
}

func (u *User) SetPrivateKey(key crypto.PrivateKey) {
	u.key = key
}

type HTTPProvider struct {
	Settings config.Settings
	Service  *s3.S3
}

func (p *HTTPProvider) Present(domain, token, keyAuth string) error {
	input := s3.PutObjectInput{
		Bucket: aws.String(p.Settings.Bucket),
		Key:    aws.String(path.Join(".well-known", "acme-challenge", token)),
		Body:   strings.NewReader(keyAuth),
	}
	if p.Settings.ServerSideEncryption != "" {
		input.ServerSideEncryption = aws.String(p.Settings.ServerSideEncryption)
	}
	if _, err := p.Service.PutObject(&input); err != nil {
		return err
	}

	insecureClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	return waitFor(10*time.Second, 2*time.Second, func() (bool, error) {
		resp, err := insecureClient.Get("https://" + path.Join(domain, ".well-known", "acme-challenge", token))
		if err != nil {
			return false, err
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return false, err
		}
		if string(body) == keyAuth {
			return true, nil
		}
		return false, fmt.Errorf("HTTP-01 token mismatch for %s: expected %s, got %s", token, keyAuth, string(body))
	})
}

// inlining deprecated function from github.com/go-acme/lego@3252b0b
func waitFor(timeout time.Duration, interval time.Duration, callback func() (bool, error)) error {
	var lastErr string
	timeup := time.After(timeout)
	for {
		select {
		case <-timeup:
			return fmt.Errorf("Time limit exceeded. Last error: %s", lastErr)
		default:
		}

		stop, err := callback()
		if stop {
			return nil
		}
		if err != nil {
			lastErr = err.Error()
		}

		time.Sleep(interval)
	}
}

func (p *HTTPProvider) CleanUp(domain, token, keyAuth string) error {
	_, err := p.Service.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(p.Settings.Bucket),
		Key:    aws.String(path.Join(".well-known", "acme-challenge", token)),
	})
	return err
}

type DNSProvider struct {
	// We need to export the needed information for the customer to update their DNS records, and we cannot safely
	// pass around a map, so this is the safest way to pass the information along while implementing the interface.
	Presentation chan string
}

//
func (p *DNSProvider) Present(domain, token, keyAuth string) error {
	p.Presentation <- fmt.Sprintf("domain %s", domain)
	p.Presentation <- fmt.Sprintf("token %s", token)
	p.Presentation <- fmt.Sprintf("keyauth %s", keyAuth)
	return nil
}

func (p *DNSProvider) CleanUp(domain, token, keyAuth string) error {
	return nil
}

func (p *DNSProvider) Timeout() (time.Duration, time.Duration) {
	return 10 * time.Second, 2 * time.Second
}

func NewClient(settings config.Settings, user *User, s3Service *s3.S3) (*lego.Client, chan string, error) {
	// we set to 3 since we are sending 3 items, we don't want this to block.
	presenter := make(chan string, 3)

	client, err := lego.NewClient(&lego.Config{
		CADirURL: "",
		Certificate: lego.CertificateConfig{
			KeyType: certcrypto.RSA2048,
		},
	})
	if err != nil {
		return &lego.Client{}, presenter, err
	}

	if user.GetRegistration() == nil {
		reg, err := client.Registration.Register(registration.RegisterOptions{
			// todo (mxplusb): we need to document with our users they are agreeing to the Let's Encrypt TOS.
			TermsOfServiceAgreed: true,
		})
		if err != nil {
			return client, presenter, err
		}
		user.Registration = reg
	}

	if err = client.Challenge.SetHTTP01Provider(&HTTPProvider{
		Settings: settings,
		Service:  s3Service,
	}); err != nil {
		return client, presenter, err
	}

	if err = client.Challenge.SetDNS01Provider(&DNSProvider{
		Presentation: presenter,
	}, nil); err != nil {
		return client, presenter, err
	}

	return client, presenter, nil
}
