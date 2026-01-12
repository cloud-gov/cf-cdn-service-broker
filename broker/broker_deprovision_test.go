package broker_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"

	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi"

	"github.com/cloud-gov/cf-cdn-service-broker/broker"
	cfmock "github.com/cloud-gov/cf-cdn-service-broker/cf/mocks"
	"github.com/cloud-gov/cf-cdn-service-broker/config"
	"github.com/cloud-gov/cf-cdn-service-broker/models"
	"github.com/cloud-gov/cf-cdn-service-broker/models/mocks"
)

func TestDeprovisioning(t *testing.T) {
	suite.Run(t, new(DeprovisionSuite))
}

type DeprovisionSuite struct {
	suite.Suite
	Manager  mocks.RouteManagerIface
	Broker   *broker.CdnServiceBroker
	cfclient cfmock.Client
	settings config.Settings
	logger   lager.Logger
	ctx      context.Context
}

func (s *DeprovisionSuite) SetupTest() {
	s.Manager = mocks.RouteManagerIface{}
	s.cfclient = cfmock.Client{}
	s.logger = lager.NewLogger("broker.provision.test")
	s.settings = config.Settings{
		DefaultOrigin: "origin.cloud.gov",
	}
	s.Broker = broker.New(
		&s.Manager,
		&s.cfclient,
		s.settings,
		s.logger,
	)
	s.ctx = context.Background()
}

func (s *DeprovisionSuite) TestDeprovisionSuccess() {
	details := brokerapi.DeprovisionDetails{}

	route := &models.Route{
		State: models.Provisioned,
	}
	s.Manager.On("Get", "123").Return(route, nil)

	s.Manager.On("Disable", route).Return(nil)

	_, err := s.Broker.Deprovision(s.ctx, "123", details, true)
	s.Nil(err)
}

func (s *DeprovisionSuite) TestDeprovisioDisableError() {
	details := brokerapi.DeprovisionDetails{}

	route := &models.Route{
		State: models.Provisioned,
	}
	s.Manager.On("Get", "123").Return(route, nil)

	s.Manager.On("Disable", route).Return(errors.New("failed disabling"))

	_, err := s.Broker.Deprovision(s.ctx, "123", details, true)
	s.NotNil(err)
	s.Contains(err.Error(), "failed disabling")
}
