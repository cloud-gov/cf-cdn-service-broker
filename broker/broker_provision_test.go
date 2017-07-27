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
	"github.com/18F/cf-cdn-service-broker/models"
	"github.com/18F/cf-cdn-service-broker/models/mocks"
	"github.com/18F/cf-cdn-service-broker/utils"
)

func TestProvisioning(t *testing.T) {
	suite.Run(t, new(ProvisionSuite))
}

type ProvisionSuite struct {
	suite.Suite
	Manager  mocks.RouteManagerIface
	Broker   *broker.CdnServiceBroker
	cfclient cfmock.Client
	settings config.Settings
	logger   lager.Logger
	ctx      context.Context
}

func (s *ProvisionSuite) SetupTest() {
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

	s.cfclient.On("GetOrgByGuid", "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5").Return(cfclient.Org{Name: "my-org"}, nil)

}

func (s *ProvisionSuite) TestSync() {
	_, err := s.Broker.Provision(s.ctx, "", brokerapi.ProvisionDetails{}, false)
	s.Equal(err, brokerapi.ErrAsyncRequired)
}

func (s *ProvisionSuite) TestWithoutDetails() {
	_, err := s.Broker.Provision(s.ctx, "", brokerapi.ProvisionDetails{}, true)
	s.NotNil(err)
	s.Equal(err.Error(), "must be invoked with configuration parameters")
}

func (s *ProvisionSuite) TestWithoutOptions() {
	details := brokerapi.ProvisionDetails{
		RawParameters: []byte(`{}`),
	}
	_, err := s.Broker.Provision(s.ctx, "", details, true)
	s.NotNil(err)
	s.Equal(err.Error(), "must pass non-empty `domain`")
}

func (s *ProvisionSuite) TestInstanceExists() {
	route := &models.Route{
		State: models.Provisioned,
	}
	s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, nil)
	s.Manager.On("Get", "123").Return(route, nil)

	details := brokerapi.ProvisionDetails{
		RawParameters: []byte(`{"domain": "domain.gov"}`),
	}
	_, err := s.Broker.Provision(s.ctx, "123", details, true)
	s.Equal(err, brokerapi.ErrInstanceAlreadyExists)
}

func (s *ProvisionSuite) TestSuccess() {
	s.Manager.On("Get", "123").Return(&models.Route{}, errors.New("not found"))
	route := &models.Route{State: models.Provisioning}
	s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, nil)
	s.Manager.On("Create", "123", "domain.gov", "origin.cloud.gov", "", false, utils.Headers{"Host": true}, true,
		map[string]string{"Organization": "", "Space": "", "Service": "", "Plan": ""}).Return(route, nil)

	details := brokerapi.ProvisionDetails{
		RawParameters: []byte(`{"domain": "domain.gov"}`),
	}
	_, err := s.Broker.Provision(s.ctx, "123", details, true)
	s.Nil(err)
}

func (s *ProvisionSuite) TestSuccessCustomOrigin() {
	s.Manager.On("Get", "123").Return(&models.Route{}, errors.New("not found"))
	route := &models.Route{State: models.Provisioning}
	s.Manager.On("Create", "123", "domain.gov", "custom.cloud.gov", "", false, utils.Headers{}, true,
		map[string]string{"Organization": "", "Space": "", "Service": "", "Plan": ""}).Return(route, nil)

	details := brokerapi.ProvisionDetails{
		RawParameters: []byte(`{"domain": "domain.gov", "origin": "custom.cloud.gov"}`),
	}
	_, err := s.Broker.Provision(s.ctx, "123", details, true)
	s.Nil(err)
}

func (s *ProvisionSuite) TestDomainNotExists() {
	s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, errors.New("fail"))
	details := brokerapi.ProvisionDetails{
		OrganizationGUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
		RawParameters:    []byte(`{"domain": "domain.gov"}`),
	}
	_, err := s.Broker.Provision(s.ctx, "123", details, true)
	s.NotNil(err)
	s.Contains(err.Error(), "cf create-domain")
}

func (s *ProvisionSuite) TestMultipleDomainsOneNotExists() {
	s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, nil)
	s.cfclient.On("GetDomainByName", "domain2.gov").Return(cfclient.Domain{}, errors.New("fail"))
	details := brokerapi.ProvisionDetails{
		OrganizationGUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
		RawParameters:    []byte(`{"domain": "domain.gov,domain2.gov"}`),
	}
	_, err := s.Broker.Provision(s.ctx, "123", details, true)
	s.NotNil(err)
	s.Contains(err.Error(), "Domain does not exist")
	s.NotContains(err.Error(), "domain.gov")
	s.Contains(err.Error(), "domain2.gov")
}

func (s *ProvisionSuite) TestMultipleDomainsMoreThanOneNotExists() {
	s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, nil)
	s.cfclient.On("GetDomainByName", "domain2.gov").Return(cfclient.Domain{}, errors.New("fail"))
	s.cfclient.On("GetDomainByName", "domain3.gov").Return(cfclient.Domain{}, errors.New("fail"))
	details := brokerapi.ProvisionDetails{
		OrganizationGUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
		RawParameters:    []byte(`{"domain": "domain.gov,domain2.gov,domain3.gov"}`),
	}
	_, err := s.Broker.Provision(s.ctx, "123", details, true)
	s.NotNil(err)
	s.Contains(err.Error(), "Multiple domains do not exist")
	s.NotContains(err.Error(), "domain.gov")
	s.Contains(err.Error(), "domain2.gov")
	s.Contains(err.Error(), "domain3.gov")
}

func (s *ProvisionSuite) setupTestOfHeaderForwarding() {
	s.Manager.On("Get", "123").Return(&models.Route{}, errors.New("not found"))
	s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, nil)
}

func (s *ProvisionSuite) allowCreateWithExpectedHeaders(expectedHeaders utils.Headers) {
	route := &models.Route{State: models.Provisioning}
	s.Manager.On("Create", "123", "domain.gov", "origin.cloud.gov", "", false, expectedHeaders, true,
		map[string]string{"Organization": "", "Space": "", "Service": "", "Plan": ""}).Return(route, nil)
}

func (s *ProvisionSuite) failCreateWithExpectedHeaders(expectedHeaders utils.Headers) {
	s.Manager.On("Create", "123", "domain.gov", "origin.cloud.gov", "", false, expectedHeaders, true,
		map[string]string{"Organization": "", "Space": "", "Service": "", "Plan": ""}).Return(nil, errors.New("fail"))
}

func (s *ProvisionSuite) TestSuccessForwardingDuplicatedHostHeader() {
	s.setupTestOfHeaderForwarding()
	s.allowCreateWithExpectedHeaders(utils.Headers{"Host": true})

	details := brokerapi.ProvisionDetails{
		RawParameters: []byte(`{"domain": "domain.gov", "headers": ["Host"]}`),
	}
	_, err := s.Broker.Provision(s.ctx, "123", details, true)
	s.Nil(err)
}

func (s *ProvisionSuite) TestSuccessForwardedSingleHeader() {
	s.setupTestOfHeaderForwarding()
	s.allowCreateWithExpectedHeaders(utils.Headers{"User-Agent": true, "Host": true})

	details := brokerapi.ProvisionDetails{
		RawParameters: []byte(`{"domain": "domain.gov", "headers": ["User-Agent"]}`),
	}
	_, err := s.Broker.Provision(s.ctx, "123", details, true)
	s.Nil(err)
}

func (s *ProvisionSuite) TestSuccessForwardingWildcardHeader() {
	s.setupTestOfHeaderForwarding()
	s.allowCreateWithExpectedHeaders(utils.Headers{"*": true})

	details := brokerapi.ProvisionDetails{
		RawParameters: []byte(`{"domain": "domain.gov", "headers": ["*"]}`),
	}
	_, err := s.Broker.Provision(s.ctx, "123", details, true)
	s.Nil(err)
}

func (s *ProvisionSuite) TestSuccessNineForwardedHeaders() {
	s.setupTestOfHeaderForwarding()
	s.allowCreateWithExpectedHeaders(utils.Headers{"One": true, "Two": true, "Three": true, "Four": true, "Five": true, "Six": true, "Seven": true, "Eight": true, "Nine": true, "Host": true})

	details := brokerapi.ProvisionDetails{
		RawParameters: []byte(`{"domain": "domain.gov", "headers": ["One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine"]}`),
	}
	_, err := s.Broker.Provision(s.ctx, "123", details, true)
	s.Nil(err)
}

func (s *ProvisionSuite) TestForwardedHeadersDuplicates() {
	s.setupTestOfHeaderForwarding()
	s.failCreateWithExpectedHeaders(utils.Headers{"User-Agent": true, "Host": true})

	details := brokerapi.ProvisionDetails{
		RawParameters: []byte(`{"domain": "domain.gov", "headers": ["User-Agent", "Host", "User-Agent"]}`),
	}
	_, err := s.Broker.Provision(s.ctx, "123", details, true)
	s.NotNil(err)
	s.Contains(err.Error(), "must not pass duplicated header 'User-Agent'")
}

func (s *ProvisionSuite) TestForwardedHeadersWhitelistAndWildcard() {
	s.setupTestOfHeaderForwarding()
	s.failCreateWithExpectedHeaders(utils.Headers{"*": true})

	details := brokerapi.ProvisionDetails{
		RawParameters: []byte(`{"domain": "domain.gov", "headers": ["*", "User-Agent"]}`),
	}
	_, err := s.Broker.Provision(s.ctx, "123", details, true)
	s.NotNil(err)
	s.Contains(err.Error(), "must not pass whitelisted headers alongside wildcard")
}

func (s *ProvisionSuite) TestForwardedHeadersMoreThanNine() {
	s.setupTestOfHeaderForwarding()
	s.failCreateWithExpectedHeaders(utils.Headers{"One": true, "Two": true, "Three": true, "Four": true, "Five": true, "Six": true, "Seven": true, "Eight": true, "Nine": true, "Ten": true, "Host": true})

	details := brokerapi.ProvisionDetails{
		RawParameters: []byte(`{"domain": "domain.gov", "headers": ["One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine", "Ten"]}`),
	}
	_, err := s.Broker.Provision(s.ctx, "123", details, true)
	s.NotNil(err)
	s.Contains(err.Error(), "must pass no more than 9 custom headers to forward")
}
