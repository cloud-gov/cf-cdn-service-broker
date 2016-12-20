package broker_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pivotal-cf/brokerapi"

	"github.com/18F/cf-cdn-service-broker/broker"
	"github.com/18F/cf-cdn-service-broker/models"
)

func TestLastOperationMissing(t *testing.T) {
	b := broker.CdnServiceBroker{
		Manager: &FakeRouteManager{
			RouteGet: models.Route{},
			ErrorGet: errors.New(""),
		},
	}
	operation, err := b.LastOperation("")
	assert.Equal(t, operation.State, brokerapi.Failed)
	assert.Equal(t, operation.Description, "Service instance not found")
	assert.Nil(t, err)
}

func TestLastOperationSucceeded(t *testing.T) {
	b := broker.CdnServiceBroker{
		Manager: &FakeRouteManager{
			RouteGet: models.Route{
				State:          models.Provisioned,
				DomainExternal: "cdn.cloud.gov",
				Origin:         "cdn.apps.cloud.gov",
			},
			ErrorGet: nil,
		},
	}
	operation, err := b.LastOperation("")
	assert.Equal(t, operation.State, brokerapi.Succeeded)
	assert.Equal(t, operation.Description, "Service instance provisioned [cdn.cloud.gov => cdn.apps.cloud.gov]")
	assert.Nil(t, err)
}

func TestLastOperationProvisioning(t *testing.T) {
	b := broker.CdnServiceBroker{
		Manager: &FakeRouteManager{
			RouteGet: models.Route{
				State:          models.Provisioning,
				DomainExternal: "cdn.cloud.gov",
				Origin:         "cdn.apps.cloud.gov",
			},
			ErrorGet: nil,
		},
	}
	operation, err := b.LastOperation("")
	assert.Equal(t, operation.State, brokerapi.InProgress)
	assert.True(t, strings.Contains(operation.Description, "Provisioning in progress [cdn.cloud.gov => cdn.apps.cloud.gov]"))
	assert.Nil(t, err)
}

func TestLastOperationDeprovisioning(t *testing.T) {
	b := broker.CdnServiceBroker{
		Manager: &FakeRouteManager{
			RouteGet: models.Route{
				State:          models.Deprovisioning,
				DomainExternal: "cdn.cloud.gov",
				Origin:         "cdn.apps.cloud.gov",
			},
			ErrorGet: nil,
		},
	}
	operation, err := b.LastOperation("")
	assert.Equal(t, operation.State, brokerapi.InProgress)
	assert.Equal(t, operation.Description, "Deprovisioning in progress [cdn.cloud.gov => cdn.apps.cloud.gov]")
	assert.Nil(t, err)
}

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
