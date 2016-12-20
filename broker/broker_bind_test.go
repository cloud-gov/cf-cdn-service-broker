package broker_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pivotal-cf/brokerapi"

	"github.com/18F/cf-cdn-service-broker/broker"
)

func TestBind(t *testing.T) {
	b := broker.CdnServiceBroker{}
	_, err := b.Bind("", "", brokerapi.BindDetails{})
	assert.NotNil(t, err)
}

func TestUnbind(t *testing.T) {
	b := broker.CdnServiceBroker{}
	err := b.Unbind("", "", brokerapi.UnbindDetails{})
	assert.NotNil(t, err)
}
