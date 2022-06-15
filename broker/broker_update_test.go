package broker_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/pivotal-cf/brokerapi/v8/domain"

	"code.cloudfoundry.org/lager"
	"github.com/alphagov/paas-cdn-broker/broker"
	cfmock "github.com/alphagov/paas-cdn-broker/cf/mocks"
	"github.com/alphagov/paas-cdn-broker/config"
	"github.com/alphagov/paas-cdn-broker/models/mocks"
	"github.com/alphagov/paas-cdn-broker/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var (
	defaultTTLNotPassed       *int64
	domainNotPassed           *string
	forwardedHeadersNotPassed *utils.Headers
	forwardCookiesNotPassed   *bool
)

var _ = Describe("Update", func() {
	var s *ProvisionUpdateSuite = &ProvisionUpdateSuite{}

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
		s.Manager.UpdateStub = func(
			_ string,
			updateDomain *string,
			ttl *int64,
			headers *utils.Headers,
			forwardCookies *bool) (bool, error) {

			if *updateDomain == "domain1.gov" &&
				ttl == defaultTTLNotPassed &&
				headers == forwardedHeadersNotPassed &&
				forwardCookies == forwardCookiesNotPassed {

				return false, nil
			} else {
				return false, errors.New("unexpected arguments")
			}
		}

		s.setupCFClientListV3DomainsDomain1()

		details := domain.UpdateDetails{
			RawParameters: json.RawMessage(`{"domain": "domain1.gov"}`),
			PreviousValues: domain.PreviousValues{
				OrgID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			},
		}
		_, err := s.Broker.Update(s.ctx, "123", details, true)
		Expect(err).NotTo(HaveOccurred())
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should error when Cloud Foundry domain does not exist", func() {
		s.Manager.UpdateReturns(true, nil)

		s.setupCFClientListV3DomainsDomain3()
		s.setupCFClientOrg()

		details := domain.UpdateDetails{
			PreviousValues: domain.PreviousValues{
				OrgID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			},
			RawParameters: json.RawMessage(`{"domain": "domain3.gov"}`),
		}
		_, err := s.Broker.Update(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
			"Domain domain3.gov does not exist in CloudFoundry; create it with: cf create-domain domain3.gov my-org",
		))
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should error when given multiple Cloud Foundry domains one of which does not exist", func() {
		s.Manager.UpdateReturns(true, nil)

		s.setupCFClientListV3DomainsDomain2()
		s.setupCFClientListV3DomainsDomain3()
		s.setupCFClientOrg()

		details := domain.UpdateDetails{
			PreviousValues: domain.PreviousValues{
				OrgID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			},
			RawParameters: json.RawMessage(`{"domain": "domain3.gov,domain2.gov"}`),
		}
		_, err := s.Broker.Update(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
			"Domain domain3.gov does not exist in CloudFoundry; create it with: cf create-domain domain3.gov my-org",
		))
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should error when given multiple Cloud Foundry domains many of which do not exist", func() {
		s.Manager.UpdateReturns(true, nil)

		s.setupCFClientListV3DomainsDomain2()
		s.setupCFClientListV3DomainsDomain3()
		s.setupCFClientListV3DomainsDomain4()
		s.setupCFClientOrg()

		details := domain.UpdateDetails{
			PreviousValues: domain.PreviousValues{
				OrgID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			},
			RawParameters: json.RawMessage(`{"domain": "domain3.gov,domain2.gov,domain4.gov"}`),
		}
		_, err := s.Broker.Update(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
`Multiple domains do not exist in CloudFoundry; create them with:
cf create-domain domain3.gov my-org
cf create-domain domain4.gov my-org`,
		))
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should sensibly handle errors fetching the organization name for error messages", func() {
		s.Manager.UpdateReturns(true, nil)

		s.setupCFClientListV3DomainsDomain2()
		s.setupCFClientListV3DomainsDomain3()
		s.setupCFClientListV3DomainsDomain4()
		s.setupCFClientOrgErr()

		details := domain.UpdateDetails{
			PreviousValues: domain.PreviousValues{
				OrgID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			},
			RawParameters: json.RawMessage(`{"domain": "domain3.gov,domain2.gov,domain4.gov"}`),
		}
		_, err := s.Broker.Update(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
`Multiple domains do not exist in CloudFoundry; create them with:
cf create-domain domain3.gov <organization>
cf create-domain domain4.gov <organization>`,
		))
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should error when given an domain using invalid characters", func() {
		details := domain.UpdateDetails{
			RawParameters: json.RawMessage(`{"domain": "domain!.gov"}`),
			PreviousValues: domain.PreviousValues{
				OrgID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			},
		}
		_, err := s.Broker.Update(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
			"Domain domain!.gov doesn't look like a valid domain",
		))
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should error when the Cloud Foundry domain doesn't belong to the organization", func() {
		s.Manager.UpdateReturns(true, nil)

		s.setupCFClientListV3DomainsDomain6()

		details := domain.UpdateDetails{
			PreviousValues: domain.PreviousValues{
				OrgID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			},
			RawParameters: json.RawMessage(`{"domain": "domain6.gov"}`),
		}
		_, err := s.Broker.Update(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
			"Domain domain6.gov is owned by a different organization in CloudFoundry",
		))
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should error when multiple Cloud Foundry domains don't belong to the organization", func() {
		s.Manager.UpdateReturns(true, nil)

		s.setupCFClientListV3DomainsDomain5()
		s.setupCFClientListV3DomainsDomain6()

		details := domain.UpdateDetails{
			PreviousValues: domain.PreviousValues{
				OrgID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			},
			RawParameters: json.RawMessage(`{"domain": "domain6.gov,domain5.gov"}`),
		}
		_, err := s.Broker.Update(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
			"Multiple domains are owned by a different organization in CloudFoundry: domain6.gov, domain5.gov",
		))
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should error when one of several Cloud Foundry domains don't belong to the organization", func() {
		s.Manager.UpdateReturns(true, nil)

		s.setupCFClientListV3DomainsDomain1()
		s.setupCFClientListV3DomainsDomain2()
		s.setupCFClientListV3DomainsDomain5()

		details := domain.UpdateDetails{
			PreviousValues: domain.PreviousValues{
				OrgID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			},
			RawParameters: json.RawMessage(`{"domain": "domain5.gov,domain1.gov,domain2.gov"}`),
		}
		_, err := s.Broker.Update(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
			"Domain domain5.gov is owned by a different organization in CloudFoundry",
		))
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should prioritize non-existent Cloud Foundry domain errors over ownership errors", func() {
		s.Manager.UpdateReturns(true, nil)

		s.setupCFClientListV3DomainsDomain4()
		s.setupCFClientListV3DomainsDomain6()
		s.setupCFClientOrg()

		details := domain.UpdateDetails{
			PreviousValues: domain.PreviousValues{
				OrgID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			},
			RawParameters: json.RawMessage(`{"domain": "domain4.gov,domain6.gov"}`),
		}
		_, err := s.Broker.Update(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
			"Domain domain4.gov does not exist in CloudFoundry; create it with: cf create-domain domain4.gov my-org",
		))
		s.cfclient.AssertExpectations(GinkgoT())
	})

	Context("Headers", func() {
		BeforeEach(func() {
			s.setupCFClientListV3DomainsDomain1()
		})

		It("Should succeed when forwarding duplicated host headers", func() {
			details := domain.UpdateDetails{
				PreviousValues: domain.PreviousValues{
					OrgID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
				},
				RawParameters: json.RawMessage(`{
			"domain": "domain1.gov",
			"headers": ["Host"]
		}`),
			}

			s.Manager.UpdateReturns(false, nil)

			_, err := s.Broker.Update(s.ctx, "123", details, true)
			Expect(err).NotTo(HaveOccurred())
			s.cfclient.AssertExpectations(GinkgoT())
		})

		It("Should succeed when forwarding a single header", func() {
			details := domain.UpdateDetails{
				PreviousValues: domain.PreviousValues{
					OrgID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
				},
				RawParameters: json.RawMessage(`{
			"domain": "domain1.gov",
			"headers": ["User-Agent"]
		}`),
			}

			s.Manager.UpdateReturns(false, nil)

			_, err := s.Broker.Update(s.ctx, "123", details, true)
			Expect(err).NotTo(HaveOccurred())
			s.cfclient.AssertExpectations(GinkgoT())
		})

		It("Should succeed when forwarding wildcard headers", func() {
			details := domain.UpdateDetails{
				PreviousValues: domain.PreviousValues{
					OrgID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
				},
				RawParameters: json.RawMessage(`{
			"domain": "domain1.gov",
			"headers": ["*"]
		}`),
			}

			s.Manager.UpdateReturns(false, nil)

			_, err := s.Broker.Update(s.ctx, "123", details, true)
			Expect(err).NotTo(HaveOccurred())
			s.cfclient.AssertExpectations(GinkgoT())
		})

		It("Should succeed when forwarding nine headers", func() {
			details := domain.UpdateDetails{
				PreviousValues: domain.PreviousValues{
					OrgID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
				},
				RawParameters: json.RawMessage(`{
			"domain": "domain1.gov",
			"headers": ["One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine"]
		}`),
			}

			s.Manager.UpdateReturns(false, nil)

			_, err := s.Broker.Update(s.ctx, "123", details, true)
			Expect(err).NotTo(HaveOccurred())
			s.cfclient.AssertExpectations(GinkgoT())
		})

		It("Should error when specifying a specific header and also wildcard headers", func() {
			details := domain.UpdateDetails{
				PreviousValues: domain.PreviousValues{
					OrgID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
				},
				RawParameters: json.RawMessage(`{
			"domain": "domain1.gov",
			"headers": ["*", "User-Agent"]
		}`),
			}
			_, err := s.Broker.Update(s.ctx, "123", details, true)

			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("must not pass whitelisted headers alongside wildcard")))
			s.cfclient.AssertExpectations(GinkgoT())
		})

		It("Should error when forwarding ten or more headers", func() {
			details := domain.UpdateDetails{
				PreviousValues: domain.PreviousValues{
					OrgID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
				},
				RawParameters: json.RawMessage(`{
			"domain": "domain1.gov",
			"headers": ["One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine", "Ten"]
		}`),
			}
			_, err := s.Broker.Update(s.ctx, "123", details, true)

			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("must not set more than 10 headers; got 11")))
			s.cfclient.AssertExpectations(GinkgoT())
		})
	})
})
