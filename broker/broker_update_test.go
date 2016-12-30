package broker_test

import (
	"context"
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

func (s *UpdateSuite) TestUpdateWithoutOptions() {
	details := brokerapi.UpdateDetails{}
	_, err := s.Broker.Update(context.TODO(), "", details, true)
	s.NotNil(err)
	s.Equal(err.Error(), "must be invoked with `domain` or `origin` keys")
}

func (s *UpdateSuite) TestUpdateSuccessOnlyDomain() {
	details := brokerapi.UpdateDetails{
		Parameters: map[string]interface{}{
			"domain": "domain.gov",
		},
	}
	s.Manager.On("Update", "", "domain.gov", "", "", false).Return(nil)
	_, err := s.Broker.Update(context.TODO(), "", details, true)
	s.Nil(err)
}

func (s *UpdateSuite) TestUpdateSuccessOnlyOrigin() {
	details := brokerapi.UpdateDetails{
		Parameters: map[string]interface{}{
			"origin": "origin.gov",
		},
	}
	s.Manager.On("Update", "", "", "origin.gov", "", false).Return(nil)
	_, err := s.Broker.Update(context.TODO(), "", details, true)
	s.Nil(err)
}

func (s *UpdateSuite) TestUpdateSuccess() {
	details := brokerapi.UpdateDetails{
		Parameters: map[string]interface{}{
			"domain":          "domain.gov",
			"origin":          "origin.gov",
			"path":            ".",
			"insecure_origin": true,
		},
	}
	s.Manager.On("Update", "", "domain.gov", "origin.gov", ".", true).Return(nil)
	_, err := s.Broker.Update(context.TODO(), "", details, true)
	s.Nil(err)
}
