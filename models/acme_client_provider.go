package models

import (
	"code.cloudfoundry.org/lager"
	"github.com/alphagov/paas-cdn-broker/config"
	"github.com/alphagov/paas-cdn-broker/lego/acme"
	"github.com/alphagov/paas-cdn-broker/utils"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate -o mocks/FakeAcmeClientProvider.go --fake-name FakeAcmeClientProvider . AcmeClientProviderInterface
//counterfeiter:generate -o mocks/FakeAcmeClient.go --fake-name FakeAcmeClient ../lego/acme ClientInterface

// For code comprehension the DNS and HTTP providers are implemented in separate files

type AcmeClientProviderInterface interface {
	GetDNS01Client(user *utils.User, settings config.Settings) (acme.ClientInterface, error)
	GetHTTP01Client(user *utils.User, settings config.Settings) (acme.ClientInterface, error)
}

type AcmeClientProvider struct {
	logger lager.Logger
}

func NewAcmeClientProvider(logger lager.Logger) AcmeClientProviderInterface {
	return &AcmeClientProvider{
		logger: logger,
	}
}
