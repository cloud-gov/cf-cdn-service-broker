package models

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"strings"
	"time"

	goacme "golang.org/x/crypto/acme"
	legoacme "github.com/18F/cf-cdn-service-broker/lego/acme"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/18F/cf-cdn-service-broker/config"
	"github.com/18F/cf-cdn-service-broker/utils"
)

func (*AcmeClientProvider) GetHTTP01Client(user *utils.User, settings config.Settings) (legoacme.ClientInterface, error) {
	key := user.GetPrivateKey().(rsa.PrivateKey)
	client := goacme.Client{Key: &key}

	ctx := context.Background()
	a := goacme.Account{
		Contact: []string {fmt.Sprintf("mailto:%s", user.Email)},
	}

	account, err := client.Register(ctx, &a, goacme.AcceptTOS)
	if err != nil {
		return nil, err
	}

	if user.GetRegistration() == nil {
		user.Registration = &legoacme.RegistrationResource{
			Body:        legoacme.Registration{},
			URI:         account.URI,
			NewAuthzURL: "https://acme-v01.api.letsencrypt.org/acme/new-authz",
			TosURL:      "",
		}
	}
	legoclient, err := legoacme.NewClient(settings.AcmeUrl, user, legoacme.RSA2048)
	if err != nil {
		return &legoacme.Client{}, err
	}
	session := session.New(aws.NewConfig().WithRegion(settings.AwsDefaultRegion))

	legoclient.SetChallengeProvider(legoacme.HTTP01, &HTTPProvider{
		Settings: settings,
		Service:  s3.New(session),
	})
	legoclient.ExcludeChallenges([]legoacme.Challenge{legoacme.TLSSNI01, legoacme.DNS01})

	return legoclient, nil
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

	return legoacme.WaitFor(10*time.Second, 2*time.Second, func() (bool, error) {
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
