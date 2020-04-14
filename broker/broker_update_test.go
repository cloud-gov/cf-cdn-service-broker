package broker_test

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/stretchr/testify/suite"

	"code.cloudfoundry.org/lager"
	"github.com/cloudfoundry-community/go-cfclient"
	"github.com/pivotal-cf/brokerapi"

	"github.com/18F/cf-cdn-service-broker/broker"
	cfmock "github.com/18F/cf-cdn-service-broker/cf/mocks"
	"github.com/18F/cf-cdn-service-broker/config"
	"github.com/18F/cf-cdn-service-broker/models/mocks"
	"github.com/18F/cf-cdn-service-broker/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type UpdateSuite struct {
	suite.Suite
	Manager  mocks.RouteManagerIface
	Broker   *broker.CdnServiceBroker
	cfclient cfmock.Client
	settings config.Settings
	logger   lager.Logger
	ctx      context.Context
}

func (s *UpdateSuite) allowUpdateWithExpectedHeaders(expectedHeaders utils.Headers) {
	s.Manager.On("Update", "", "domain.gov", s.settings.DefaultDefaultTTL, expectedHeaders, true).Return(nil)
}

func (s *UpdateSuite) failOnUpdateWithExpectedHeaders(expectedHeaders utils.Headers) {
	s.Manager.On("Update", "", "domain.gov", s.settings.DefaultDefaultTTL, expectedHeaders, true).Return(errors.New("fail"))
}

var _ = Describe("Update", func() {
	var s *UpdateSuite = &UpdateSuite{}

	BeforeEach(func() {
		s.Manager = mocks.RouteManagerIface{}
		s.cfclient = cfmock.Client{}
		s.logger = lager.NewLogger("test")
		s.settings = config.Settings{
			DefaultOrigin:     "origin.cloud.gov",
			DefaultDefaultTTL: int64(0),
		}
		s.Broker = broker.New(
			&s.Manager,
			&s.cfclient,
			s.settings,
			s.logger,
		)
		s.ctx = context.Background()
	})

	It("Should error when not given options", func() {
		details := brokerapi.UpdateDetails{
			RawParameters: json.RawMessage(`{}`),
		}
		_, err := s.Broker.Update(s.ctx, "", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(ContainSubstring("must pass non-empty `domain`")))
	})

	It("Should succeed when given only a domain", func() {
		details := brokerapi.UpdateDetails{
			RawParameters: json.RawMessage(`{"domain": "domain.gov"}`),
		}
		s.Manager.On("Update", "", "domain.gov", s.settings.DefaultDefaultTTL, utils.Headers{"Host": true}, true).Return(nil)
		s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, nil)
		_, err := s.Broker.Update(s.ctx, "", details, true)

		Expect(err).NotTo(HaveOccurred())
	})

	It("Should error when Cloud Foundry domain does not exist", func() {
		details := brokerapi.UpdateDetails{
			PreviousValues: brokerapi.PreviousValues{
				OrgID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			},
			RawParameters: json.RawMessage(`{"domain": "domain.gov"}`),
		}
		s.Manager.On("Update", "", "domain.gov", s.settings.DefaultDefaultTTL, utils.Headers{"Host": true}, true).Return(nil)
		s.cfclient.On("GetOrgByGuid", "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5").Return(cfclient.Org{Name: "my-org"}, nil)
		s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, errors.New("bad"))
		_, err := s.Broker.Update(s.ctx, "", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(ContainSubstring("cf create-domain")))
	})

	Context("Headers", func() {
		BeforeEach(func() {
			s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, nil)
		})

		It("Should succeed when forwarding duplicated host headers", func() {
			s.allowUpdateWithExpectedHeaders(utils.Headers{"Host": true})

			details := brokerapi.UpdateDetails{
				RawParameters: json.RawMessage(`{
			"insecure_origin": true,
			"domain": "domain.gov",
			"headers": ["Host"]
		}`),
			}
			_, err := s.Broker.Update(s.ctx, "", details, true)

			Expect(err).NotTo(HaveOccurred())
		})

		It("Should succeed when forwarding a single header", func() {
			s.allowUpdateWithExpectedHeaders(utils.Headers{"User-Agent": true, "Host": true})

			details := brokerapi.UpdateDetails{
				RawParameters: json.RawMessage(`{
"insecure_origin": true,
			"domain": "domain.gov",
			"headers": ["User-Agent"]
		}`),
			}
			_, err := s.Broker.Update(s.ctx, "", details, true)

			Expect(err).NotTo(HaveOccurred())
		})

		It("Should succeed when forwarding wildcard headers", func() {
			s.allowUpdateWithExpectedHeaders(utils.Headers{"*": true})

			details := brokerapi.UpdateDetails{
				RawParameters: json.RawMessage(`{
"insecure_origin": true,
			"domain": "domain.gov",
			"headers": ["*"]
		}`),
			}
			_, err := s.Broker.Update(s.ctx, "", details, true)

			Expect(err).NotTo(HaveOccurred())
		})

		It("Should succeed when forwarding nine headers", func() {
			s.allowUpdateWithExpectedHeaders(utils.Headers{"One": true, "Two": true, "Three": true, "Four": true, "Five": true, "Six": true, "Seven": true, "Eight": true, "Nine": true, "Host": true})

			details := brokerapi.UpdateDetails{
				RawParameters: json.RawMessage(`{
"insecure_origin": true,
			"domain": "domain.gov",
			"headers": ["One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine"]
		}`),
			}
			_, err := s.Broker.Update(s.ctx, "", details, true)

			Expect(err).NotTo(HaveOccurred())
		})

		It("Should error when forwarding wildcard headers and normal headers", func() {
			s.failOnUpdateWithExpectedHeaders(utils.Headers{"*": true})

			details := brokerapi.UpdateDetails{
				RawParameters: json.RawMessage(`{
"insecure_origin": true,
			"domain": "domain.gov",
			"headers": ["*", "User-Agent"]
		}`),
			}
			_, err := s.Broker.Update(s.ctx, "", details, true)

			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("must not pass whitelisted headers alongside wildcard")))
		})

		It("Should error when forwarding ten or more headers", func() {
			s.failOnUpdateWithExpectedHeaders(utils.Headers{"One": true, "Two": true, "Three": true, "Four": true, "Five": true, "Six": true, "Seven": true, "Eight": true, "Nine": true, "Ten": true, "Host": true})

			details := brokerapi.UpdateDetails{
				RawParameters: json.RawMessage(`{
			"insecure_origin": true,
			"domain": "domain.gov",
			"headers": ["One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine", "Ten"]
		}`),
			}
			_, err := s.Broker.Update(s.ctx, "", details, true)

			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("must not set more than 10 headers; got 11")))
		})
	})
})
