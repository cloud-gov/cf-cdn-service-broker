package broker_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"

	"code.cloudfoundry.org/lager"
	"github.com/cloudfoundry-community/go-cfclient"
	"github.com/pivotal-cf/brokerapi"

	"github.com/18F/cf-cdn-service-broker/broker"
	cfmock "github.com/18F/cf-cdn-service-broker/cf/mocks"
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
	cfclient cfmock.Client
	settings config.Settings
	logger   lager.Logger
	ctx      context.Context
}

func (s *UpdateSuite) SetupTest() {
	s.Manager = mocks.RouteManagerIface{}
	s.cfclient = cfmock.Client{}
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
	s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, nil)
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
	s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, nil)
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
	s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, nil)
	_, err := s.Broker.Update(s.ctx, "", details, true)
	s.Nil(err)
}

func (s *UpdateSuite) TestDomainNotExists() {
	details := brokerapi.UpdateDetails{
		PreviousValues: brokerapi.PreviousValues{
			OrgID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
		},
		Parameters: map[string]interface{}{
			"domain": "domain.gov",
		},
	}
	s.Manager.On("Update", "", "domain.gov", "origin.cloud.gov", ".", true, []string{"Host"}).Return(nil)
	s.cfclient.On("GetOrgByGuid", "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5").Return(cfclient.Org{Name: "my-org"}, nil)
	s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, errors.New("bad"))
	_, err := s.Broker.Update(s.ctx, "", details, true)
	s.NotNil(err)
	s.Contains(err.Error(), "cf create-domain")
}
