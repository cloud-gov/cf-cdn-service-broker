package broker_test

import (
	"context"
	"encoding/json"
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
	"github.com/18F/cf-cdn-service-broker/utils"
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
		RawParameters: json.RawMessage(`{"origin": ""}`),
	}
	_, err := s.Broker.Update(s.ctx, "", details, true)
	s.NotNil(err)
	s.Equal(err.Error(), "must pass non-empty `domain` or `origin`")
}

func (s *UpdateSuite) TestUpdateSuccessOnlyDomain() {
	details := brokerapi.UpdateDetails{
		RawParameters: json.RawMessage(`{"domain": "domain.gov"}`),
	}
	s.Manager.On("Update", "", "domain.gov", "origin.cloud.gov", "", false, utils.Headers{"Host": true}, true).Return(nil)
	s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, nil)
	_, err := s.Broker.Update(s.ctx, "", details, true)
	s.Nil(err)
}

func (s *UpdateSuite) TestUpdateSuccessOnlyOrigin() {
	details := brokerapi.UpdateDetails{
		RawParameters: json.RawMessage(`{"origin": "origin.gov"}`),
	}
	s.Manager.On("Update", "", "", "origin.gov", "", false, utils.Headers{}, true).Return(nil)
	s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, nil)
	_, err := s.Broker.Update(s.ctx, "", details, true)
	s.Nil(err)
}

func (s *UpdateSuite) TestUpdateSuccess() {
	details := brokerapi.UpdateDetails{
		RawParameters: json.RawMessage(`{
			"insecure_origin": true,
			"domain": "domain.gov",
			"path": "."
		}`),
	}
	s.Manager.On("Update", "", "domain.gov", "origin.cloud.gov", ".", true, utils.Headers{"Host": true}, true).Return(nil)
	s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, nil)
	_, err := s.Broker.Update(s.ctx, "", details, true)
	s.Nil(err)
}

func (s *UpdateSuite) TestDomainNotExists() {
	details := brokerapi.UpdateDetails{
		PreviousValues: brokerapi.PreviousValues{
			OrgID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
		},
		RawParameters: json.RawMessage(`{"domain": "domain.gov"}`),
	}
	s.Manager.On("Update", "", "domain.gov", "origin.cloud.gov", ".", true, utils.Headers{"Host": true}, true).Return(nil)
	s.cfclient.On("GetOrgByGuid", "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5").Return(cfclient.Org{Name: "my-org"}, nil)
	s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, errors.New("bad"))
	_, err := s.Broker.Update(s.ctx, "", details, true)
	s.NotNil(err)
	s.Contains(err.Error(), "cf create-domain")
}

func (s *UpdateSuite) setupTestOfHeaderForwarding() {
	s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, nil)
}

func (s *UpdateSuite) allowUpdateWithExpectedHeaders(expectedHeaders utils.Headers) {
	s.Manager.On("Update", "", "domain.gov", "origin.cloud.gov", ".", true, expectedHeaders, true).Return(nil)
}

func (s *UpdateSuite) failOnUpdateWithExpectedHeaders(expectedHeaders utils.Headers) {
	s.Manager.On("Update", "", "domain.gov", "origin.cloud.gov", ".", true, expectedHeaders, true).Return(errors.New("fail"))
}

func (s *UpdateSuite) TestSuccessForwardingDuplicatedHostHeader() {
	s.setupTestOfHeaderForwarding()
	s.allowUpdateWithExpectedHeaders(utils.Headers{"Host": true})

	details := brokerapi.UpdateDetails{
		RawParameters: json.RawMessage(`{
			"insecure_origin": true,
			"domain": "domain.gov",
			"path": ".",
			"headers": ["Host"]
		}`),
	}
	_, err := s.Broker.Update(s.ctx, "", details, true)
	s.Nil(err)
}

func (s *UpdateSuite) TestSuccessForwardedSingleHeader() {
	s.setupTestOfHeaderForwarding()
	s.allowUpdateWithExpectedHeaders(utils.Headers{"User-Agent": true, "Host": true})

	details := brokerapi.UpdateDetails{
		RawParameters: json.RawMessage(`{
			"insecure_origin": true,
			"domain": "domain.gov",
			"path": ".",
			"headers": ["User-Agent"]
		}`),
	}
	_, err := s.Broker.Update(s.ctx, "", details, true)
	s.Nil(err)
}

func (s *UpdateSuite) TestSuccessForwardingWildcardHeader() {
	s.setupTestOfHeaderForwarding()
	s.allowUpdateWithExpectedHeaders(utils.Headers{"*": true})

	details := brokerapi.UpdateDetails{
		RawParameters: json.RawMessage(`{
			"insecure_origin": true,
			"domain": "domain.gov",
			"path": ".",
			"headers": ["*"]
		}`),
	}
	_, err := s.Broker.Update(s.ctx, "", details, true)
	s.Nil(err)
}

func (s *UpdateSuite) TestSuccessNineForwardedHeaders() {
	s.setupTestOfHeaderForwarding()
	s.allowUpdateWithExpectedHeaders(utils.Headers{"One": true, "Two": true, "Three": true, "Four": true, "Five": true, "Six": true, "Seven": true, "Eight": true, "Nine": true, "Host": true})

	details := brokerapi.UpdateDetails{
		RawParameters: json.RawMessage(`{
			"insecure_origin": true,
			"domain": "domain.gov",
			"path": ".",
			"headers": ["One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine"]
		}`),
	}
	_, err := s.Broker.Update(s.ctx, "", details, true)
	s.Nil(err)
}

func (s *UpdateSuite) TestForwardedHeadersWhitelistAndWildcard() {
	s.setupTestOfHeaderForwarding()
	s.failOnUpdateWithExpectedHeaders(utils.Headers{"*": true})

	details := brokerapi.UpdateDetails{
		RawParameters: json.RawMessage(`{
			"insecure_origin": true,
			"domain": "domain.gov",
			"path": ".",
			"headers": ["*", "User-Agent"]
		}`),
	}
	_, err := s.Broker.Update(s.ctx, "", details, true)
	s.NotNil(err)
	s.Contains(err.Error(), "must not pass whitelisted headers alongside wildcard")
}

func (s *UpdateSuite) TestForwardedHeadersMoreThanNine() {
	s.setupTestOfHeaderForwarding()
	s.failOnUpdateWithExpectedHeaders(utils.Headers{"One": true, "Two": true, "Three": true, "Four": true, "Five": true, "Six": true, "Seven": true, "Eight": true, "Nine": true, "Ten": true, "Host": true})

	details := brokerapi.UpdateDetails{
		RawParameters: json.RawMessage(`{
			"insecure_origin": true,
			"domain": "domain.gov",
			"path": ".",
			"headers": ["One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine", "Ten"]
		}`),
	}
	_, err := s.Broker.Update(s.ctx, "", details, true)
	s.NotNil(err)
	s.Contains(err.Error(), "must pass no more than 9 custom headers to forward")
}
