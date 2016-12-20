package broker_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pivotal-cf/brokerapi"

	"github.com/18F/cf-cdn-service-broker/broker"
	"github.com/18F/cf-cdn-service-broker/models"
	"github.com/18F/cf-cdn-service-broker/models/mocks"
)

func TestLastOperationMissing(t *testing.T) {
	manager := mocks.RouteManagerIface{}
	manager.On("Get", "").Return(models.Route{}, errors.New("not found"))
	b := broker.CdnServiceBroker{
		Manager: &manager,
	}

	operation, err := b.LastOperation("")
	assert.Equal(t, operation.State, brokerapi.Failed)
	assert.Equal(t, operation.Description, "Service instance not found")
	assert.Nil(t, err)
}

func TestLastOperationSucceeded(t *testing.T) {
	manager := mocks.RouteManagerIface{}
	route := models.Route{
		State:          models.Provisioned,
		DomainExternal: "cdn.cloud.gov",
		Origin:         "cdn.apps.cloud.gov",
	}
	manager.On("Get", "123").Return(route, nil)
	manager.On("Update", route).Return(nil)
	b := broker.CdnServiceBroker{
		Manager: &manager,
	}

	operation, err := b.LastOperation("123")
	assert.Equal(t, operation.State, brokerapi.Succeeded)
	assert.Equal(t, operation.Description, "Service instance provisioned [cdn.cloud.gov => cdn.apps.cloud.gov]")
	assert.Nil(t, err)
}

func TestLastOperationProvisioning(t *testing.T) {
	manager := mocks.RouteManagerIface{}
	route := models.Route{
		State:          models.Provisioning,
		DomainExternal: "cdn.cloud.gov",
		Origin:         "cdn.apps.cloud.gov",
	}
	manager.On("Get", "123").Return(route, nil)
	manager.On("Update", route).Return(nil)
	b := broker.CdnServiceBroker{
		Manager: &manager,
	}

	operation, err := b.LastOperation("123")
	assert.Equal(t, operation.State, brokerapi.InProgress)
	assert.True(t, strings.Contains(operation.Description, "Provisioning in progress [cdn.cloud.gov => cdn.apps.cloud.gov]"))
	assert.Nil(t, err)
}

func TestLastOperationDeprovisioning(t *testing.T) {
	manager := mocks.RouteManagerIface{}
	route := models.Route{
		State:          models.Deprovisioning,
		DomainExternal: "cdn.cloud.gov",
		Origin:         "cdn.apps.cloud.gov",
	}
	manager.On("Get", "123").Return(route, nil)
	manager.On("Update", route).Return(nil)
	b := broker.CdnServiceBroker{
		Manager: &manager,
	}

	operation, err := b.LastOperation("123")
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
