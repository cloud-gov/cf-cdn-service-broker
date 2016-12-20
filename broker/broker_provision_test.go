package broker_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/pivotal-cf/brokerapi"

	"github.com/18F/cf-cdn-service-broker/broker"
	"github.com/18F/cf-cdn-service-broker/models"
	"github.com/18F/cf-cdn-service-broker/models/mocks"
)

func TestProvisioning(t *testing.T) {
	suite.Run(t, new(ProvisionSuite))
}

type ProvisionSuite struct {
	suite.Suite
	Manager mocks.RouteManagerIface
	Broker  broker.CdnServiceBroker
}

func (s *ProvisionSuite) SetupTest() {
	s.Manager = mocks.RouteManagerIface{}
	s.Broker = broker.CdnServiceBroker{
		Manager: &s.Manager,
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
	route := models.Route{
		State: models.Provisioned,
	}
	s.Manager.On("Get", "123").Return(route, nil)
	// s.Manager.On("Create", "123").Return(route, nil)

	details := brokerapi.ProvisionDetails{
		RawParameters: []byte(`{"domain": "domain.gov", "origin": "origin.gov"}`),
	}
	_, err := s.Broker.Provision("123", details, true)
	s.Equal(err, brokerapi.ErrInstanceAlreadyExists)
}

func (s *ProvisionSuite) TestSuccess() {
	s.Manager.On("Get", "123").Return(models.Route{}, errors.New("not found"))
	route := models.Route{State: models.Provisioning}
	s.Manager.On("Create", "123", "domain.gov", "origin.gov", "", false).Return(route, nil)

	details := brokerapi.ProvisionDetails{
		RawParameters: []byte(`{"domain": "domain.gov", "origin": "origin.gov"}`),
	}
	_, err := s.Broker.Provision("123", details, true)
	s.Nil(err)
}
