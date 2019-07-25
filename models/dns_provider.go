package models

import (
	"time"

	"github.com/18F/cf-cdn-service-broker/config"
	"github.com/18F/cf-cdn-service-broker/lego/acme"
	"github.com/18F/cf-cdn-service-broker/utils"
)

func (*AcmeClientProvider) GetDNS01Client(user *utils.User, settings config.Settings) (acme.ClientInterface, error) {
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

	client.SetChallengeProvider(acme.DNS01, &DNSProvider{})
	client.ExcludeChallenges([]acme.Challenge{acme.TLSSNI01, acme.HTTP01})

	return client, nil
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
