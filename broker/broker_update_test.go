package broker_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi"

	"github.com/18F/cf-cdn-service-broker/broker"
	"github.com/18F/cf-cdn-service-broker/config"
	"github.com/18F/cf-cdn-service-broker/models/mocks"
)

func TestUpdating(t *testing.T) {
	suite.Run(t, new(UpdateSuite))
}

type UpdateSuite struct {
	suite.Suite
	Manager  mocks.RouteManagerIface
	Broker   *broker.CdnServiceBroker
	settings config.Settings
	logger   lager.Logger
	ctx      context.Context
}

func (s *UpdateSuite) SetupTest() {
	s.Manager = mocks.RouteManagerIface{}
	s.settings = config.Settings{
		DefaultOrigin: "origin.cloud.gov",
	}
	s.Broker = broker.New(
		&s.Manager,
		s.settings,
		s.logger,
	)
	s.ctx = context.Background()
}

func (s *UpdateSuite) TestUpdateWithoutOptions() {
	details := brokerapi.UpdateDetails{
		Parameters: map[string]interface{}{
			"origin": "",
		},
	}
	_, err := s.Broker.Update(s.ctx, "", details, true)
	s.NotNil(err)
	s.Equal(err.Error(), "must pass non-empty `domain` or `origin`")
}

func (s *UpdateSuite) TestUpdateSuccessOnlyDomain() {
	details := brokerapi.UpdateDetails{
		Parameters: map[string]interface{}{
			"domain": "domain.gov",
		},
	}
	s.Manager.On("Update", "", "domain.gov", "origin.cloud.gov", "", false, []string{"Host"}).Return(nil)
	_, err := s.Broker.Update(s.ctx, "", details, true)
	s.Nil(err)
}

func (s *UpdateSuite) TestUpdateSuccessOnlyOrigin() {
	details := brokerapi.UpdateDetails{
		Parameters: map[string]interface{}{
			"origin": "origin.gov",
		},
	}
	s.Manager.On("Update", "", "", "origin.gov", "", false, []string{}).Return(nil)
	_, err := s.Broker.Update(s.ctx, "", details, true)
	s.Nil(err)
}

func (s *UpdateSuite) TestUpdateSuccess() {
	details := brokerapi.UpdateDetails{
		Parameters: map[string]interface{}{
			"domain":          "domain.gov",
			"path":            ".",
			"insecure_origin": true,
		},
	}
	s.Manager.On("Update", "", "domain.gov", "origin.cloud.gov", ".", true, []string{"Host"}).Return(nil)
	_, err := s.Broker.Update(s.ctx, "", details, true)
	s.Nil(err)
}
