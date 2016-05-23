package broker_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/pivotal-cf/brokerapi"

	"github.com/18F/cf-cdn-service-broker/broker"
	"github.com/18F/cf-cdn-service-broker/models"
)

type FakeRouteManager struct {
	RouteCreate models.Route
	ErrorCreate error
	RouteGet    models.Route
	ErrorGet    error
}

func (m *FakeRouteManager) Create(instanceId, domain, origin string) (models.Route, error) {
	return m.RouteCreate, m.ErrorCreate
}

func (m *FakeRouteManager) Get(instanceId string) (models.Route, error) {
	return m.RouteGet, m.ErrorGet
}

func (m *FakeRouteManager) Update(models.Route) error {
	return nil
}

func (m *FakeRouteManager) Disable(models.Route) error {
	return nil
}

func (m *FakeRouteManager) Renew(models.Route) error {
	return nil
}

func (m *FakeRouteManager) RenewAll() {
	return
}

func TestProvisioning(t *testing.T) {
	suite.Run(t, new(ProvisionSuite))
}

type ProvisionSuite struct {
	suite.Suite
	Broker broker.CdnServiceBroker
}

func (s *ProvisionSuite) SetupTest() {
	s.Broker = broker.CdnServiceBroker{
		Manager: &FakeRouteManager{
			RouteCreate: models.Route{State: "provisioned"},
			ErrorCreate: nil,
			RouteGet:    models.Route{},
			ErrorGet:    errors.New("not found"),
		},
	}
}

func (s *ProvisionSuite) TestSync() {
	_, err := s.Broker.Provision("", brokerapi.ProvisionDetails{}, false)
	s.Equal(err, brokerapi.ErrAsyncRequired)
}

func (s *ProvisionSuite) TestWithoutDetails() {
	_, err := s.Broker.Provision("", brokerapi.ProvisionDetails{}, true)
	s.NotNil(err)
	s.Equal(err.Error(), "must be invoked with configuration parameters")
}

func (s *ProvisionSuite) TestWithoutOptions() {
	details := brokerapi.ProvisionDetails{
		RawParameters: []byte(`{}`),
	}
	_, err := s.Broker.Provision("", details, true)
	s.NotNil(err)
	s.Equal(err.Error(), "must be invoked with `domain` and `origin` keys")
}

func (s *ProvisionSuite) TestInstanceExists() {
	b := broker.CdnServiceBroker{
		Manager: &FakeRouteManager{
			RouteCreate: models.Route{State: "provisioned"},
			ErrorCreate: nil,
			RouteGet:    models.Route{State: "provisioned"},
			ErrorGet:    nil,
		},
	}
	details := brokerapi.ProvisionDetails{
		RawParameters: []byte(`{"domain": "domain.gov", "origin": "origin.gov"}`),
	}
	_, err := b.Provision("", details, true)
	s.Equal(err, brokerapi.ErrInstanceAlreadyExists)
}

func (s *ProvisionSuite) TestSuccess() {
	details := brokerapi.ProvisionDetails{
		RawParameters: []byte(`{"domain": "domain.gov", "origin": "origin.gov"}`),
	}
	_, err := s.Broker.Provision("", details, true)
	s.Nil(err)
}

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
			RouteGet: models.Route{State: "provisioned"},
			ErrorGet: nil,
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
			RouteGet: models.Route{State: "provisioning"},
			ErrorGet: nil,
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
			RouteGet: models.Route{State: "deprovisioning"},
			ErrorGet: nil,
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
