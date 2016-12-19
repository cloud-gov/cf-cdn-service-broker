package broker_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/pivotal-cf/brokerapi"

	"github.com/18F/cf-cdn-service-broker/broker"
	"github.com/18F/cf-cdn-service-broker/models/mocks"
)

func TestUpdating(t *testing.T) {
	suite.Run(t, new(UpdateSuite))
}

type UpdateSuite struct {
	suite.Suite
	Manager mocks.RouteManagerIface
	Broker  broker.CdnServiceBroker
}

func (s *UpdateSuite) SetupTest() {
	s.Manager = mocks.RouteManagerIface{}
	s.Broker = broker.CdnServiceBroker{
		Manager: &s.Manager,
	}
}

func (s *UpdateSuite) TestWithoutOptions() {
	details := brokerapi.UpdateDetails{
		Parameters: make(map[string]interface{}),
	}
	_, err := s.Broker.Update("", details, true)
	s.NotNil(err)
	s.Equal(err.Error(), "must be invoked with `domain` or `origin` keys")
}
