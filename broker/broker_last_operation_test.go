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
	"github.com/18F/cf-cdn-service-broker/config"
	"github.com/18F/cf-cdn-service-broker/iamuser"
	"github.com/18F/cf-cdn-service-broker/models"
	"github.com/18F/cf-cdn-service-broker/models/mocks"
	"github.com/18F/cf-cdn-service-broker/utils"
)

func TestLastOperation(t *testing.T) {
	suite.Run(t, new(LastOperationSuite))
}

type LastOperationSuite struct {
	suite.Suite
	Logger  lager.Logger
	Manager *mocks.RouteManagerIface
	Broker  *broker.CdnServiceBroker
	ctx     context.Context
}

func (s *LastOperationSuite) SetupTest() {
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

func (s *LastOperationSuite) TestLastOperationMissing() {
	s.Manager.On("Get", "").Return(&models.Route{}, errors.New("not found"))

	operation, err := s.Broker.LastOperation(s.ctx, "", "")
	s.Equal(operation.State, brokerapi.Failed)
	s.Equal(operation.Description, "Service instance not found")
	s.Nil(err)
}

func (s *LastOperationSuite) TestLastOperationSucceeded() {
	route := &models.Route{
		State:          models.Provisioned,
		DomainExternal: "cdn.cloud.gov",
		Origin:         "cdn.apps.cloud.gov",
	}
	s.Manager.On("Get", "123").Return(route, nil)
	s.Manager.On("Poll", route).Return(nil)

	operation, err := s.Broker.LastOperation(s.ctx, "123", "")
	s.Equal(operation.State, brokerapi.Succeeded)
	s.Equal(operation.Description, "Service instance provisioned [cdn.cloud.gov => cdn.apps.cloud.gov]")
	s.Nil(err)
}

func (s *LastOperationSuite) TestLastOperationProvisioning() {
	route := &models.Route{
		State:          models.Provisioning,
		DomainExternal: "cdn.cloud.gov",
		Origin:         "cdn.apps.cloud.gov",
	}
	s.Manager.On("Get", "123").Return(route, nil)
	s.Manager.On("Poll", route).Return(nil)

	operation, err := s.Broker.LastOperation(s.ctx, "123", "")
	s.Equal(operation.State, brokerapi.InProgress)
	s.True(strings.Contains(operation.Description, "Provisioning in progress [cdn.cloud.gov => cdn.apps.cloud.gov]"))
	s.Nil(err)
}

func (s *LastOperationSuite) TestLastOperationDeprovisioning() {
	route := &models.Route{
		State:          models.Deprovisioning,
		DomainExternal: "cdn.cloud.gov",
		Origin:         "cdn.apps.cloud.gov",
	}
	s.Manager.On("Get", "123").Return(route, nil)
	s.Manager.On("Poll", route).Return(nil)

	operation, err := s.Broker.LastOperation(s.ctx, "123", "")
	s.Equal(operation.State, brokerapi.InProgress)
	s.Equal(operation.Description, "Deprovisioning in progress [cdn.cloud.gov => cdn.apps.cloud.gov]")
	s.Nil(err)
}
