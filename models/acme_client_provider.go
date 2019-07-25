package models

import (
	"code.cloudfoundry.org/lager"
	"github.com/18F/cf-cdn-service-broker/config"
	"github.com/18F/cf-cdn-service-broker/lego/acme"
	"github.com/18F/cf-cdn-service-broker/utils"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate -o mocks/FakeAcmeClientProvider.go --fake-name FakeAcmeClientProvider . AcmeClientProviderInterface
//counterfeiter:generate -o mocks/FakeAcmeClient.go --fake-name FakeAcmeClient ../lego/acme ClientInterface

type AcmeClientProviderInterface interface {
	GetDNS01Client(user *utils.User, settings config.Settings) (acme.ClientInterface, error)
	GetHTTP01Client(user *utils.User, settings config.Settings) (acme.ClientInterface, error)
}

type AcmeClientProvider struct {
	logger lager.Logger
}

func (*AcmeClientProvider) GetDNS01Client(user *utils.User, settings config.Settings) (acme.ClientInterface, error) {
	panic("implement me")
}

func (*AcmeClientProvider) GetHTTP01Client(user *utils.User, settings config.Settings) (acme.ClientInterface, error) {
	panic("implement me")
}
