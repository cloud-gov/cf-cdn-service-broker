package utils

import (
	"os"
	"path"
	"strings"

	"crypto"
	"crypto/rand"
	"crypto/rsa"

	"github.com/xenolf/lego/acme"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"

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

type HTTPProvider struct{}

func (*HTTPProvider) Present(domain, token, keyAuth string) error {
	svc := s3.New(session.New(&aws.Config{Region: aws.String(config.Region)}))

	_, err := svc.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(config.Bucket),
		Body:   strings.NewReader(keyAuth),
		Key:    aws.String(path.Join(".well-known", "acme-challenge", token)),
	})

	return err
}

func (*HTTPProvider) CleanUp(domain, token, keyAuth string) error {
	svc := s3.New(session.New(&aws.Config{Region: aws.String(config.Region)}))

	_, err := svc.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(config.Bucket),
		Key:    aws.String(path.Join(".well-known", "acme-challenge", token)),
	})

	return err
}

func ObtainCertificate(domain string) (acme.CertificateResource, error) {
	keySize := 2048
	key, err := rsa.GenerateKey(rand.Reader, keySize)
	if err != nil {
		return acme.CertificateResource{}, err
	}

	user := User{
		Email: os.Getenv("CDN_EMAIL"),
		key:   key,
	}
	client, err := acme.NewClient(os.Getenv("CDN_ACME_URL"), &user, acme.RSA2048)

	client.SetChallengeProvider(acme.HTTP01, &HTTPProvider{})
	client.ExcludeChallenges([]acme.Challenge{acme.DNS01, acme.TLSSNI01})

	reg, err := client.Register()
	user.Registration = reg

	err = client.AgreeToTOS()

	domains := []string{domain}
	certificate, failures := client.ObtainCertificate(domains, false, nil)

	if len(failures) > 0 {
		return acme.CertificateResource{}, failures[domain]
	}

	return certificate, nil
}
