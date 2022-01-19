package broker_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/pivotal-cf/brokerapi/domain"

	"github.com/stretchr/testify/suite"

	"code.cloudfoundry.org/lager"
	"github.com/alphagov/paas-cdn-broker/broker"
	cfmock "github.com/alphagov/paas-cdn-broker/cf/mocks"
	"github.com/alphagov/paas-cdn-broker/config"
	"github.com/alphagov/paas-cdn-broker/models/mocks"
	"github.com/alphagov/paas-cdn-broker/utils"
	"github.com/cloudfoundry-community/go-cfclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var (
	defaultTTLNotPassed       *int64
	domainNotPassed           *string
	forwardedHeadersNotPassed *utils.Headers
	forwardCookiesNotPassed   *bool
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

	It("Should succeed when given only a domain", func() {
		details := domain.UpdateDetails{
			RawParameters: json.RawMessage(`{"domain": "domain.gov"}`),
		}

		domain := "domain.gov"
		s.Manager.UpdateStub = func(
			_ string,
			updateDomain *string,
			ttl *int64,
			headers *utils.Headers,
			forwardCookies *bool) (bool, error) {

			if *updateDomain == domain &&
				ttl == defaultTTLNotPassed &&
				headers == forwardedHeadersNotPassed &&
				forwardCookies == forwardCookiesNotPassed {

				return false, nil
			} else {
				return false, errors.New("unexpected arguments")
			}
		}
		s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, nil)

		_, err := s.Broker.Update(s.ctx, "", details, true)
		Expect(err).NotTo(HaveOccurred())
	})

	It("Should error when Cloud Foundry domain does not exist", func() {
		details := domain.UpdateDetails{
			PreviousValues: domain.PreviousValues{
				OrgID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			},
			RawParameters: json.RawMessage(`{"domain": "domain.gov"}`),
		}
		s.Manager.UpdateReturns(true, nil)
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
			details := domain.UpdateDetails{
				RawParameters: json.RawMessage(`{
			"domain": "domain.gov",
			"headers": ["Host"]
		}`),
			}

			s.Manager.UpdateReturns(false, nil)

			_, err := s.Broker.Update(s.ctx, "", details, true)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should succeed when forwarding a single header", func() {
			details := domain.UpdateDetails{
				RawParameters: json.RawMessage(`{
			"domain": "domain.gov",
			"headers": ["User-Agent"]
		}`),
			}

			s.Manager.UpdateReturns(false, nil)

			_, err := s.Broker.Update(s.ctx, "", details, true)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should succeed when forwarding wildcard headers", func() {
			details := domain.UpdateDetails{
				RawParameters: json.RawMessage(`{
			"domain": "domain.gov",
			"headers": ["*"]
		}`),
			}

			s.Manager.UpdateReturns(false, nil)

			_, err := s.Broker.Update(s.ctx, "", details, true)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should succeed when forwarding nine headers", func() {
			details := domain.UpdateDetails{
				RawParameters: json.RawMessage(`{
			"domain": "domain.gov",
			"headers": ["One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine"]
		}`),
			}

			s.Manager.UpdateReturns(false, nil)

			_, err := s.Broker.Update(s.ctx, "", details, true)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should error when specifying a specific header and also wildcard headers", func() {
			details := domain.UpdateDetails{
				RawParameters: json.RawMessage(`{
			"domain": "domain.gov",
			"headers": ["*", "User-Agent"]
		}`),
			}
			_, err := s.Broker.Update(s.ctx, "", details, true)

			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("must not pass whitelisted headers alongside wildcard")))
		})

		It("Should error when forwarding ten or more headers", func() {
			details := domain.UpdateDetails{
				RawParameters: json.RawMessage(`{
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
