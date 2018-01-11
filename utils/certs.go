package utils

import (
	"crypto"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/xenolf/lego/acme"

	"github.com/18F/cf-cdn-service-broker/config"
)

func preCheckDNS(fqdn, value string) (bool, error) {
	record, err := net.LookupTXT(fqdn)
	if err != nil {
		return false, err
	}
	if len(record) == 1 && record[0] == value {
		return true, nil
	}
	return false, fmt.Errorf("DNS precheck failed on name %s, value %s", fqdn, value)
}

func init() {
	acme.PreCheckDNS = preCheckDNS
}

type User struct {
	Email        string
	Registration *acme.RegistrationResource
	key          crypto.PrivateKey
}

func (u *User) GetEmail() string {
	return u.Email
}

func (u *User) GetRegistration() *acme.RegistrationResource {
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

	return acme.WaitFor(10*time.Second, 2*time.Second, func() (bool, error) {
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

func (p *HTTPProvider) CleanUp(domain, token, keyAuth string) error {
	_, err := p.Service.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(p.Settings.Bucket),
		Key:    aws.String(path.Join(".well-known", "acme-challenge", token)),
	})
	return err
}

type DNSProvider struct{}

func (p *DNSProvider) Present(domain, token, keyAuth string) error {
	return nil
}

func (p *DNSProvider) CleanUp(domain, token, keyAuth string) error {
	return nil
}

func (p *DNSProvider) Timeout() (time.Duration, time.Duration) {
	return 10 * time.Second, 2 * time.Second
}

func NewClient(settings config.Settings, user *User, s3Service *s3.S3, excludes []acme.Challenge) (*acme.Client, error) {
	client, err := acme.NewClient(settings.AcmeUrl, user, acme.RSA2048)
	if err != nil {
		return &acme.Client{}, err
	}

	if user.GetRegistration() == nil {
		reg, err := client.Register()
		if err != nil {
			return client, err
		}
		user.Registration = reg
	}

	if err := client.AgreeToTOS(); err != nil {
		return client, err
	}

	client.SetChallengeProvider(acme.HTTP01, &HTTPProvider{
		Settings: settings,
		Service:  s3Service,
	})
	client.SetChallengeProvider(acme.DNS01, &DNSProvider{})
	client.ExcludeChallenges(excludes)

	return client, nil
}
