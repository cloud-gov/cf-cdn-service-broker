package broker_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi"

	"github.com/18F/cf-cdn-service-broker/broker"
	cfmock "github.com/18F/cf-cdn-service-broker/cf/mocks"
	"github.com/18F/cf-cdn-service-broker/config"
	"github.com/18F/cf-cdn-service-broker/models"
	"github.com/18F/cf-cdn-service-broker/models/mocks"
)

func TestLastOperation(t *testing.T) {
	suite.Run(t, new(LastOperationSuite))
}

type LastOperationSuite struct {
	suite.Suite
	Manager  mocks.RouteManagerIface
	Broker   *broker.CdnServiceBroker
	cfclient cfmock.Client
	settings config.Settings
	logger   lager.Logger
	ctx      context.Context
}

func (s *LastOperationSuite) SetupTest() {
	s.Manager = mocks.RouteManagerIface{}
	s.cfclient = cfmock.Client{}
	s.Broker = broker.New(
		&s.Manager,
		&s.cfclient,
		s.settings,
		s.logger,
	)
	s.ctx = context.Background()
}

func (s *LastOperationSuite) TestLastOperationMissing() {
	manager := mocks.RouteManagerIface{}
	manager.On("Get", "").Return(&models.Route{}, errors.New("not found"))
	b := broker.New(
		&manager,
		&s.cfclient,
		s.settings,
		s.logger,
	)

	operation, err := b.LastOperation(s.ctx, "", "")
	s.Equal(operation.State, brokerapi.Failed)
	s.Equal(operation.Description, "Service instance not found")
	s.Nil(err)
}

func (s *LastOperationSuite) TestLastOperationSucceeded() {
	manager := mocks.RouteManagerIface{}
	route := &models.Route{
		State:          models.Provisioned,
		DomainExternal: "cdn.cloud.gov",
		Origin:         "cdn.apps.cloud.gov",
	}
	manager.On("Get", "123").Return(route, nil)
	manager.On("Poll", route).Return(nil)
	b := broker.New(
		&manager,
		&s.cfclient,
		s.settings,
		s.logger,
	)

	operation, err := b.LastOperation(s.ctx, "123", "")
	s.Equal(operation.State, brokerapi.Succeeded)
	s.Equal(operation.Description, "Service instance provisioned [cdn.cloud.gov => cdn.apps.cloud.gov]")
	s.Nil(err)
}

func (s *LastOperationSuite) TestLastOperationProvisioning() {
	manager := mocks.RouteManagerIface{}
	route := &models.Route{
		State:          models.Provisioning,
		DomainExternal: "cdn.cloud.gov",
		Origin:         "cdn.apps.cloud.gov",
	}
	manager.On("Get", "123").Return(route, nil)
	manager.On("Poll", route).Return(nil)
	b := broker.New(
		&manager,
		&s.cfclient,
		s.settings,
		s.logger,
	)

	operation, err := b.LastOperation(s.ctx, "123", "")
	s.Equal(operation.State, brokerapi.InProgress)
	s.True(strings.Contains(operation.Description, "Provisioning in progress [cdn.cloud.gov => cdn.apps.cloud.gov]"))
	s.Nil(err)
}

func (s *LastOperationSuite) TestLastOperationDeprovisioning() {
	manager := mocks.RouteManagerIface{}
	route := &models.Route{
		State:          models.Deprovisioning,
		DomainExternal: "cdn.cloud.gov",
		Origin:         "cdn.apps.cloud.gov",
	}
	manager.On("Get", "123").Return(route, nil)
	manager.On("Poll", route).Return(nil)
	b := broker.New(
		&manager,
		&s.cfclient,
		s.settings,
		s.logger,
	)

	operation, err := b.LastOperation(s.ctx, "123", "")
	s.Equal(operation.State, brokerapi.InProgress)
	s.Equal(operation.Description, "Deprovisioning in progress [cdn.cloud.gov => cdn.apps.cloud.gov]")
	s.Nil(err)
}
