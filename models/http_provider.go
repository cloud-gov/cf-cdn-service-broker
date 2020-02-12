package models

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"fmt"
	"gopkg.in/square/go-jose.v1"
	"io/ioutil"
	"net/http"
	"path"
	"strings"
	"time"

	legoacme "github.com/18F/cf-cdn-service-broker/lego/acme"
	goacme "golang.org/x/crypto/acme"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/18F/cf-cdn-service-broker/config"
	"github.com/18F/cf-cdn-service-broker/utils"
)

func (acp *AcmeClientProvider) GetHTTP01Client(user *utils.User, settings config.Settings) (legoacme.ClientInterface, error) {
	logSess := acp.logger.Session("get-http-01-client")

	logSess.Info("create-goacme-client")
	key := user.GetPrivateKey().(*rsa.PrivateKey)
	client := goacme.Client{Key: key}

	ctx := context.Background()

	logSess.Info("create-goacme-account-struct")
	a := goacme.Account{
		Contact: []string {fmt.Sprintf("mailto:%s", user.Email)},
	}

	logSess.Info("fetch-goacme-account")
	account, err := client.GetReg(ctx, "this argument is ignored because the CA is RFC8555 compliant. The key on the client is used instead.")
	 if err == goacme.ErrNoAccount {
		logSess.Info("goacme-account-not-found")
		logSess.Info("register-goacme-account")
		account, err = client.Register(ctx, &a, goacme.AcceptTOS)
		if err != nil {
			logSess.Error("register-goacme-account-error", err)
			return nil, err
		}
	} else if err != nil {
		 logSess.Error("fetch-goacme-account", err)
		 return nil, err
 	}

	if user.GetRegistration() == nil {
		logSess.Info("create-user-registration-resource")
		user.Registration = &legoacme.RegistrationResource{
			Body:        legoacme.Registration{
				Resource:       account.URI,
				ID:             0,
				Key:            jose.JsonWebKey{Key:key},
				Contact:        nil,
				Agreement:      account.AgreedTerms,
				Authorizations: account.Authorizations,
				Certificates:   account.Certificates,
			},
			URI:         account.URI,
			NewAuthzURL: "https://acme-v01.api.letsencrypt.org/acme/new-authz",
			TosURL:      "",
		}
	}

	logSess.Info("create-legoacme-client")
	legoclient, err := legoacme.NewClient(settings.AcmeUrl, user, legoacme.RSA2048)
	if err != nil {
		logSess.Error("create-legoacme-client-error", err)
		return &legoacme.Client{}, err
	}

	logSess.Info("create-aws-session")
	awsSession := session.New(aws.NewConfig().WithRegion(settings.AwsDefaultRegion))

	logSess.Info("set-challenge-provider")
	err = legoclient.SetChallengeProvider(legoacme.HTTP01, &HTTPProvider{
		Settings: settings,
		Service:  s3.New(awsSession),
	})
	if err != nil {
		logSess.Error("set-challenge-provider-error", err)
		return nil, err
	}
	legoclient.ExcludeChallenges([]legoacme.Challenge{legoacme.TLSSNI01, legoacme.DNS01})

	logSess.Info("created")
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
