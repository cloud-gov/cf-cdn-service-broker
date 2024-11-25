package broker_test

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/pivotal-cf/brokerapi/v10/domain"

	"code.cloudfoundry.org/lager/v3"
	"github.com/alphagov/paas-cdn-broker/broker"
	cfmock "github.com/alphagov/paas-cdn-broker/cf/mocks"
	"github.com/alphagov/paas-cdn-broker/config"
	"github.com/alphagov/paas-cdn-broker/models/mocks"
	"github.com/alphagov/paas-cdn-broker/utils"

	. "github.com/onsi/ginkgo/v2"
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

			if *updateDomain == "domain1.cloud.gov" &&
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
			RawParameters: json.RawMessage(`{"domain": "domain1.cloud.gov"}`),
			PreviousValues: domain.PreviousValues{
				OrgID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			},
		}
		_, err := s.Broker.Update(s.ctx, "123", details, true)
		Expect(err).NotTo(HaveOccurred())
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should succeed when given only a domain whose parent domain is owned in CloudFoundry", func() {
		s.Manager.UpdateStub = func(
			_ string,
			updateDomain *string,
			ttl *int64,
			headers *utils.Headers,
			forwardCookies *bool) (bool, error) {

			if *updateDomain == "domain7.rain.gov" &&
				ttl == defaultTTLNotPassed &&
				headers == forwardedHeadersNotPassed &&
				forwardCookies == forwardCookiesNotPassed {

				return false, nil
			} else {
				return false, errors.New("unexpected arguments")
			}
		}

		s.setupCFClientListV3DomainsDomain7()

		details := domain.UpdateDetails{
			RawParameters: json.RawMessage(`{"domain": "domain7.rain.gov"}`),
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
			RawParameters: json.RawMessage(`{"domain": "domain3.cloud.gov"}`),
		}
		_, err := s.Broker.Update(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
			"Domain domain3.cloud.gov does not exist in CloudFoundry; create it with: cf create-domain domain3.cloud.gov my-org",
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
			RawParameters: json.RawMessage(`{"domain": "domain3.cloud.gov,domain2.cloud.gov"}`),
		}
		_, err := s.Broker.Update(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
			"Domain domain3.cloud.gov does not exist in CloudFoundry; create it with: cf create-domain domain3.cloud.gov my-org",
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
			RawParameters: json.RawMessage(`{"domain": "domain3.cloud.gov,domain2.cloud.gov,domain4.four.cloud.gov"}`),
		}
		_, err := s.Broker.Update(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
			`Multiple domains do not exist in CloudFoundry; create them with:
cf create-domain domain3.cloud.gov my-org
cf create-domain domain4.four.cloud.gov my-org`,
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
			RawParameters: json.RawMessage(`{"domain": "domain3.cloud.gov,domain2.cloud.gov,domain4.four.cloud.gov"}`),
		}
		_, err := s.Broker.Update(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
			`Multiple domains do not exist in CloudFoundry; create them with:
cf create-domain domain3.cloud.gov <organization>
cf create-domain domain4.four.cloud.gov <organization>`,
		))
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should error when given a domain using invalid characters", func() {
		details := domain.UpdateDetails{
			RawParameters: json.RawMessage(`{"domain": "domain!.cloud.gov"}`),
			PreviousValues: domain.PreviousValues{
				OrgID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			},
		}
		_, err := s.Broker.Update(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
			"Domain domain!.cloud.gov doesn't look like a valid domain",
		))
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should error when given a domain with a trailing dot", func() {
		details := domain.UpdateDetails{
			RawParameters: json.RawMessage(`{"domain": "domain.cloud.gov.,foo.cloud.gov"}`),
			PreviousValues: domain.PreviousValues{
				OrgID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			},
		}
		_, err := s.Broker.Update(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
			"Domain domain.cloud.gov. doesn't look like a valid domain",
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
			RawParameters: json.RawMessage(`{"domain": "domain6.cloud.gov"}`),
		}
		_, err := s.Broker.Update(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
			"Domain domain6.cloud.gov is owned by a different organization in CloudFoundry",
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
			RawParameters: json.RawMessage(`{"domain": "domain6.cloud.gov,domain5.cloud.gov"}`),
		}
		_, err := s.Broker.Update(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
			"Multiple domains are owned by a different organization in CloudFoundry: domain6.cloud.gov, domain5.cloud.gov",
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
			RawParameters: json.RawMessage(`{"domain": "domain5.cloud.gov,domain1.cloud.gov,domain2.cloud.gov"}`),
		}
		_, err := s.Broker.Update(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
			"Domain domain5.cloud.gov is owned by a different organization in CloudFoundry",
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
			RawParameters: json.RawMessage(`{"domain": "domain4.four.cloud.gov,domain6.cloud.gov"}`),
		}
		_, err := s.Broker.Update(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
			"Domain domain4.four.cloud.gov does not exist in CloudFoundry; create it with: cf create-domain domain4.four.cloud.gov my-org",
		))
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should error when a parent Cloud Foundry domain doesn't belong to the organization", func() {
		s.Manager.UpdateReturns(true, nil)

		s.setupCFClientListV3DomainsDomain8()

		details := domain.UpdateDetails{
			PreviousValues: domain.PreviousValues{
				OrgID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			},
			RawParameters: json.RawMessage(`{"domain": "domain8.wet.snow.gov"}`),
		}
		_, err := s.Broker.Update(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
			"Domain domain8.wet.snow.gov is owned by a different organization in CloudFoundry",
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
			"domain": "domain1.cloud.gov",
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
			"domain": "domain1.cloud.gov",
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
			"domain": "domain1.cloud.gov",
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
			"domain": "domain1.cloud.gov",
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
			"domain": "domain1.cloud.gov",
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
			"domain": "domain1.cloud.gov",
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
