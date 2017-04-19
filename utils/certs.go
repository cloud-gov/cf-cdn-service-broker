package utils

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"errors"
	"io/ioutil"
	"net/http"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/xenolf/lego/acme"

	"github.com/18F/cf-cdn-service-broker/config"
)

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

var insecureClient = &http.Client{
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	},
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

	return acme.WaitFor(60, 15, func() (bool, error) {
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
		return false, errors.New("HTTP-01 token mismatch")
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

func NewClient(settings config.Settings, s3Service *s3.S3) (*acme.Client, error) {
	keySize := 2048
	key, err := rsa.GenerateKey(rand.Reader, keySize)
	if err != nil {
		return &acme.Client{}, err
	}

	user := User{
		Email: settings.Email,
		key:   key,
	}

	client, err := acme.NewClient(settings.AcmeUrl, &user, acme.RSA2048)
	if err != nil {
		return &acme.Client{}, err
	}

	reg, err := client.Register()

	if err != nil {
		return &acme.Client{}, err
	}

	user.Registration = reg

	err = client.AgreeToTOS()

	if err != nil {
		return &acme.Client{}, err
	}

	client.SetChallengeProvider(acme.HTTP01, &HTTPProvider{
		Settings: settings,
		Service:  s3Service,
	})
	client.SetChallengeProvider(acme.DNS01, &DNSProvider{})
	client.ExcludeChallenges([]acme.Challenge{acme.TLSSNI01})

	return client, nil
}
