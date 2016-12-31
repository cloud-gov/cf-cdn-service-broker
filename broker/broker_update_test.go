package broker_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi"

	"github.com/18F/cf-cdn-service-broker/broker"
	"github.com/18F/cf-cdn-service-broker/config"
	"github.com/18F/cf-cdn-service-broker/iamuser"
	"github.com/18F/cf-cdn-service-broker/models/mocks"
	"github.com/18F/cf-cdn-service-broker/utils"
)

func TestUpdating(t *testing.T) {
	suite.Run(t, new(UpdateSuite))
}

type UpdateSuite struct {
	suite.Suite
	Logger  lager.Logger
	Manager *mocks.RouteManagerIface
	Broker  *broker.CdnServiceBroker
	ctx     context.Context
}

func (s *UpdateSuite) SetupTest() {
	s.Logger = lager.NewLogger("test")
	s.Manager = &mocks.RouteManagerIface{}
	s.Broker = broker.NewCdnServiceBroker(
		s.Manager,
		&utils.Distribution{},
		&iamuser.IAMUser{},
		broker.Catalog{},
		config.Settings{},
		s.Logger,
	)
	s.ctx = context.Background()
}

func (s *UpdateSuite) TestUpdateWithoutOptions() {
	details := brokerapi.UpdateDetails{}
	_, err := s.Broker.Update(s.ctx, "", details, true)
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
	_, err := s.Broker.Update(s.ctx, "", details, true)
	s.Nil(err)
}

func (s *UpdateSuite) TestUpdateSuccessOnlyOrigin() {
	details := brokerapi.UpdateDetails{
		Parameters: map[string]interface{}{
			"origin": "origin.gov",
		},
	}
	s.Manager.On("Update", "", "", "origin.gov", "", false).Return(nil)
	_, err := s.Broker.Update(s.ctx, "", details, true)
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
	_, err := s.Broker.Update(s.ctx, "", details, true)
	s.Nil(err)
}
