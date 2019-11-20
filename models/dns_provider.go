package models

import (
	"context"
	"crypto/rsa"
	"fmt"
	"time"

	"github.com/18F/cf-cdn-service-broker/config"
	"github.com/18F/cf-cdn-service-broker/utils"

	legoacme "github.com/18F/cf-cdn-service-broker/lego/acme"
	goacme "golang.org/x/crypto/acme"
)

func (*AcmeClientProvider) GetDNS01Client(user *utils.User, settings config.Settings) (legoacme.ClientInterface, error) {
	key := user.GetPrivateKey().(*rsa.PrivateKey)
	client := goacme.Client{Key: key}

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

	legoClient, err := legoacme.NewClient(settings.AcmeUrl, user, legoacme.RSA2048)
	if err != nil {
		return nil, err
	}
	err = legoClient.SetChallengeProvider(legoacme.DNS01, &DNSProvider{})
	if err != nil {
		return nil, err
	}
	legoClient.ExcludeChallenges([]legoacme.Challenge{legoacme.TLSSNI01, legoacme.HTTP01})

	return legoClient, nil
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
