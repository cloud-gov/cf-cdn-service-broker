package broker_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/pivotal-cf/brokerapi"

	"github.com/18F/cf-cdn-service-broker/broker"
	"github.com/18F/cf-cdn-service-broker/models"
)

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
			RouteCreate: models.Route{State: models.Provisioned},
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
			RouteCreate: models.Route{State: models.Provisioned},
			ErrorCreate: nil,
			RouteGet:    models.Route{State: models.Provisioned},
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
