package broker_test

import (
	"context"
	"testing"

	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi"
	"github.com/stretchr/testify/assert"

	"github.com/18F/cf-cdn-service-broker/broker"
	cfmock "github.com/18F/cf-cdn-service-broker/cf/mocks"
	"github.com/18F/cf-cdn-service-broker/config"
	"github.com/18F/cf-cdn-service-broker/models/mocks"
)

func TestBind(t *testing.T) {
	b := broker.New(
		&mocks.RouteManagerIface{},
		&cfmock.Client{},
		config.Settings{
			DefaultOrigin: "origin.cloud.gov",
		},
		lager.NewLogger("test"),
	)
	_, err := b.Bind(context.Background(), "", "", brokerapi.BindDetails{})
	assert.NotNil(t, err)
}

func TestUnbind(t *testing.T) {
	b := broker.New(
		&mocks.RouteManagerIface{},
		&cfmock.Client{},
		config.Settings{
			DefaultOrigin: "origin.cloud.gov",
		},
		lager.NewLogger("test"),
	)
	err := b.Unbind(context.Background(), "", "", brokerapi.UnbindDetails{})
	assert.NotNil(t, err)
}
