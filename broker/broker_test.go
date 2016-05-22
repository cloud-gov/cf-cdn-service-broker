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

type FakeRouteManager struct {
	Route models.Route
	Error error
}

func (m *FakeRouteManager) Create(instanceId, domain, origin string) (models.Route, error) {
	return m.Route, m.Error
}

func (m *FakeRouteManager) Get(instanceId string) (models.Route, error) {
	return m.Route, m.Error
}

func (m *FakeRouteManager) Update(models.Route) error {
	return m.Error
}

func (m *FakeRouteManager) Disable(models.Route) error {
	return m.Error
}

func (m *FakeRouteManager) Renew(models.Route) error {
	return m.Error
}

func (m *FakeRouteManager) RenewAll() {
	return
}

func TestProvisionSync(t *testing.T) {
	b := broker.CdnServiceBroker{
		Manager: &FakeRouteManager{
			Route: models.Route{State: "provisioned"},
			Error: nil,
		},
	}
	_, err := b.Provision("", brokerapi.ProvisionDetails{}, false)
	assert.Equal(t, err, brokerapi.ErrAsyncRequired)
}

func TestLastOperationMissing(t *testing.T) {
	b := broker.CdnServiceBroker{
		Manager: &FakeRouteManager{
			Route: models.Route{},
			Error: errors.New(""),
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
			Route: models.Route{State: "provisioned"},
			Error: nil,
		},
	}
	operation, err := b.LastOperation("")
	assert.Equal(t, operation.State, brokerapi.Succeeded)
	assert.Equal(t, operation.Description, "Service instance provisioned")
	assert.Nil(t, err)
}

func TestLastOperationProvisioning(t *testing.T) {
	b := broker.CdnServiceBroker{
		Manager: &FakeRouteManager{
			Route: models.Route{State: "provisioning"},
			Error: nil,
		},
	}
	operation, err := b.LastOperation("")
	assert.Equal(t, operation.State, brokerapi.InProgress)
	assert.True(t, strings.Contains(operation.Description, "Provisioning in progress"))
	assert.Nil(t, err)
}

func TestLastOperationDeprovisioning(t *testing.T) {
	b := broker.CdnServiceBroker{
		Manager: &FakeRouteManager{
			Route: models.Route{State: "deprovisioning"},
			Error: nil,
		},
	}
	operation, err := b.LastOperation("")
	assert.Equal(t, operation.State, brokerapi.InProgress)
	assert.Equal(t, operation.Description, "Deprovisioning in progress")
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
