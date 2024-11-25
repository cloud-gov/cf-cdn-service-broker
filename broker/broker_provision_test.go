package broker_test

import (
	"context"
	"errors"

	"github.com/pivotal-cf/brokerapi/v10/domain"
	"github.com/pivotal-cf/brokerapi/v10/domain/apiresponses"

	"code.cloudfoundry.org/lager/v3"
	"github.com/alphagov/paas-cdn-broker/broker"
	cfmock "github.com/alphagov/paas-cdn-broker/cf/mocks"
	"github.com/alphagov/paas-cdn-broker/config"
	"github.com/alphagov/paas-cdn-broker/models"
	"github.com/alphagov/paas-cdn-broker/models/mocks"
	"github.com/alphagov/paas-cdn-broker/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Provision", func() {
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

	It("Should error when the broker is called synchronously", func() {
		_, err := s.Broker.Provision(s.ctx, "", domain.ProvisionDetails{}, false)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(apiresponses.ErrAsyncRequired))
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should error when the broker is called without config", func() {
		_, err := s.Broker.Provision(s.ctx, "", domain.ProvisionDetails{}, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(ContainSubstring("must be invoked with configuration parameters")))
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should error when the broker is called without a domain", func() {
		details := domain.ProvisionDetails{
			RawParameters: []byte(`{}`),
		}
		_, err := s.Broker.Provision(s.ctx, "", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(ContainSubstring("must pass non-empty `domain`")))
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should error when the broker is called with an already existing service instance id", func() {
		route := &models.Route{
			State: models.Provisioned,
		}
		s.Manager.GetReturns(route, nil)

		s.setupCFClientListV3DomainsDomain1()

		details := domain.ProvisionDetails{
			OrganizationGUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			RawParameters:    []byte(`{"domain": "domain1.cloud.gov"}`),
		}
		_, err := s.Broker.Provision(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(apiresponses.ErrInstanceAlreadyExists))
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should succeed with default settings", func() {
		s.Manager.GetReturns(&models.Route{}, errors.New("not found"))
		route := &models.Route{State: models.Provisioning}
		s.Manager.CreateReturns(route, nil)

		s.setupCFClientListV3DomainsDomain1()

		details := domain.ProvisionDetails{
			OrganizationGUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			RawParameters:    []byte(`{"domain": "domain1.cloud.gov"}`),
		}
		_, err := s.Broker.Provision(s.ctx, "123", details, true)

		Expect(err).NotTo(HaveOccurred())
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should succeed when a parent domain is owned in cloudfoundry", func() {
		s.Manager.GetReturns(&models.Route{}, errors.New("not found"))
		route := &models.Route{State: models.Provisioning}
		s.Manager.CreateReturns(route, nil)

		s.setupCFClientListV3DomainsDomain7()

		details := domain.ProvisionDetails{
			OrganizationGUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			RawParameters:    []byte(`{"domain": "domain7.rain.gov"}`),
		}
		_, err := s.Broker.Provision(s.ctx, "123", details, true)

		Expect(err).NotTo(HaveOccurred())
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should create a cloudfront instance with a custom DefaultTTL", func() {
		s.Manager.GetReturns(&models.Route{}, errors.New("not found"))
		route := &models.Route{State: models.Provisioning}
		s.Manager.CreateReturns(route, nil)

		s.setupCFClientListV3DomainsDomain1()

		details := domain.ProvisionDetails{
			OrganizationGUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			RawParameters: []byte(`{
				"domain": "domain1.cloud.gov",
				"default_ttl": 52
			}`),
		}
		_, err := s.Broker.Provision(s.ctx, "123", details, true)

		Expect(err).NotTo(HaveOccurred())
		Expect(s.Manager.CreateCallCount()).To(Equal(1))
		_, _, _, ttl, _, _, _ := s.Manager.CreateArgsForCall(0)
		Expect(ttl).To(Equal(int64(52)))
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should set the correct tags", func() {
		instanceId := "123"
		s.Manager.GetReturns(&models.Route{}, errors.New("not found"))
		route := &models.Route{State: models.Provisioning}
		s.Manager.CreateReturns(route, nil)

		s.setupCFClientListV3DomainsDomain1()

		details := domain.ProvisionDetails{
			OrganizationGUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			RawParameters:    []byte(`{"domain": "domain1.cloud.gov"}`),
			SpaceGUID:        "space-1",
			ServiceID:        "service-1",
			PlanID:           "plan-1",
		}

		_, err := s.Broker.Provision(s.ctx, instanceId, details, true)

		Expect(err).NotTo(HaveOccurred())

		Expect(s.Manager.CreateCallCount()).To(Equal(1))
		_, _, _, _, _, _, inputTags := s.Manager.CreateArgsForCall(0)
		Expect(inputTags).To(HaveKeyWithValue("Organization", "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5"))
		Expect(inputTags).To(HaveKeyWithValue("Space", "space-1"))
		Expect(inputTags).To(HaveKeyWithValue("Service", "service-1"))
		Expect(inputTags).To(HaveKeyWithValue("ServiceInstance", instanceId))
		Expect(inputTags).To(HaveKeyWithValue("Plan", "plan-1"))
		Expect(inputTags).To(HaveKeyWithValue("chargeable_entity", instanceId))
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should error when Cloud Foundry does not have the domain registered", func() {
		s.setupCFClientOrg()
		s.setupCFClientListV3DomainsDomain3()

		details := domain.ProvisionDetails{
			OrganizationGUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			RawParameters:    []byte(`{"domain": "domain3.cloud.gov"}`),
		}
		_, err := s.Broker.Provision(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
			"Domain domain3.cloud.gov does not exist in CloudFoundry; create it with: cf create-domain domain3.cloud.gov my-org",
		))
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should error when given multiple domains one of which Cloud Foundry does not have registered", func() {
		s.setupCFClientOrg()
		s.setupCFClientListV3DomainsDomain1()
		s.setupCFClientListV3DomainsDomain3()

		details := domain.ProvisionDetails{
			OrganizationGUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			RawParameters:    []byte(`{"domain": "domain1.cloud.gov,domain3.cloud.gov"}`),
		}
		_, err := s.Broker.Provision(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
			"Domain domain3.cloud.gov does not exist in CloudFoundry; create it with: cf create-domain domain3.cloud.gov my-org",
		))
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should error when given multiple domains many of which Cloud Foundry does not have registered", func() {
		s.setupCFClientOrg()
		s.setupCFClientListV3DomainsDomain1()
		s.setupCFClientListV3DomainsDomain3()
		s.setupCFClientListV3DomainsDomain4()

		details := domain.ProvisionDetails{
			OrganizationGUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			RawParameters:    []byte(`{"domain": "domain1.cloud.gov,domain4.four.cloud.gov,domain3.cloud.gov"}`),
		}
		_, err := s.Broker.Provision(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
			`Multiple domains do not exist in CloudFoundry; create them with:
cf create-domain domain4.four.cloud.gov my-org
cf create-domain domain3.cloud.gov my-org`,
		))
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should sensibly handle errors fetching the organization name for error messages", func() {
		s.setupCFClientOrgErr()
		s.setupCFClientListV3DomainsDomain4()

		details := domain.ProvisionDetails{
			OrganizationGUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			RawParameters:    []byte(`{"domain": "domain4.four.cloud.gov"}`),
		}
		_, err := s.Broker.Provision(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
			"Domain domain4.four.cloud.gov does not exist in CloudFoundry; create it with: cf create-domain domain4.four.cloud.gov <organization>",
		))
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should error when given a domain using invalid characters", func() {
		details := domain.ProvisionDetails{
			OrganizationGUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			RawParameters:    []byte(`{"domain": "domain&.cloud.gov"}`),
		}
		_, err := s.Broker.Provision(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
			"Domain domain&.cloud.gov doesn't look like a valid domain",
		))
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should error when given a domain with a trailing dot", func() {
		details := domain.ProvisionDetails{
			OrganizationGUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			RawParameters:    []byte(`{"domain": "domain.cloud.gov."}`),
		}
		_, err := s.Broker.Provision(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
			"Domain domain.cloud.gov. doesn't look like a valid domain",
		))
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should error when the Cloud Foundry domain doesn't belong to the organization", func() {
		s.setupCFClientListV3DomainsDomain5()

		details := domain.ProvisionDetails{
			OrganizationGUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			RawParameters:    []byte(`{"domain": "domain5.cloud.gov"}`),
		}
		_, err := s.Broker.Provision(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
			"Domain domain5.cloud.gov is owned by a different organization in CloudFoundry",
		))
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should error when multiple Cloud Foundry domains don't belong to the organization", func() {
		s.setupCFClientListV3DomainsDomain5()
		s.setupCFClientListV3DomainsDomain6()

		details := domain.ProvisionDetails{
			OrganizationGUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			RawParameters:    []byte(`{"domain": "domain6.cloud.gov,domain5.cloud.gov"}`),
		}
		_, err := s.Broker.Provision(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
			"Multiple domains are owned by a different organization in CloudFoundry: domain6.cloud.gov, domain5.cloud.gov",
		))
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should error when one of several Cloud Foundry domains don't belong to the organization", func() {
		s.setupCFClientListV3DomainsDomain1()
		s.setupCFClientListV3DomainsDomain2()
		s.setupCFClientListV3DomainsDomain6()

		details := domain.ProvisionDetails{
			OrganizationGUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			RawParameters:    []byte(`{"domain": "domain1.cloud.gov,domain6.cloud.gov,domain2.cloud.gov"}`),
		}
		_, err := s.Broker.Provision(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
			"Domain domain6.cloud.gov is owned by a different organization in CloudFoundry",
		))
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should prioritize non-existent Cloud Foundry domain errors over ownership errors", func() {
		s.setupCFClientOrg()
		s.setupCFClientListV3DomainsDomain1()
		s.setupCFClientListV3DomainsDomain4()
		s.setupCFClientListV3DomainsDomain6()

		details := domain.ProvisionDetails{
			OrganizationGUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			RawParameters:    []byte(`{"domain": "domain1.cloud.gov,domain6.cloud.gov,domain4.four.cloud.gov"}`),
		}
		_, err := s.Broker.Provision(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
			"Domain domain4.four.cloud.gov does not exist in CloudFoundry; create it with: cf create-domain domain4.four.cloud.gov my-org",
		))
		s.cfclient.AssertExpectations(GinkgoT())
	})

	It("Should error when a parent Cloud Foundry domain doesn't belong to the organization", func() {
		s.setupCFClientListV3DomainsDomain8()

		details := domain.ProvisionDetails{
			OrganizationGUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			RawParameters:    []byte(`{"domain": "domain8.wet.snow.gov"}`),
		}
		_, err := s.Broker.Provision(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(
			"Domain domain8.wet.snow.gov is owned by a different organization in CloudFoundry",
		))
		s.cfclient.AssertExpectations(GinkgoT())
	})

	Context("Headers", func() {
		BeforeEach(func() {
			s.Manager.GetReturns(&models.Route{}, errors.New("not found"))
		})

		It("Should succeed forwarding duplicate host header", func() {
			s.allowCreateWithExpectedHeaders(utils.Headers{"Host": true})
			s.setupCFClientListV3DomainsDomain1()

			details := domain.ProvisionDetails{
				OrganizationGUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
				RawParameters:    []byte(`{"domain": "domain1.cloud.gov", "headers": ["Host"]}`),
			}
			_, err := s.Broker.Provision(s.ctx, "123", details, true)

			Expect(err).NotTo(HaveOccurred())
			s.cfclient.AssertExpectations(GinkgoT())
		})

		It("Should succeed forwarding a single header", func() {
			s.allowCreateWithExpectedHeaders(utils.Headers{"User-Agent": true, "Host": true})
			s.setupCFClientListV3DomainsDomain1()

			details := domain.ProvisionDetails{
				OrganizationGUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
				RawParameters:    []byte(`{"domain": "domain1.cloud.gov", "headers": ["User-Agent"]}`),
			}
			_, err := s.Broker.Provision(s.ctx, "123", details, true)

			Expect(err).NotTo(HaveOccurred())
			s.cfclient.AssertExpectations(GinkgoT())
		})

		It("Should succeed forwarding wildcard headers", func() {
			s.allowCreateWithExpectedHeaders(utils.Headers{"*": true})
			s.setupCFClientListV3DomainsDomain1()

			details := domain.ProvisionDetails{
				OrganizationGUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
				RawParameters:    []byte(`{"domain": "domain1.cloud.gov", "headers": ["*"]}`),
			}
			_, err := s.Broker.Provision(s.ctx, "123", details, true)

			Expect(err).NotTo(HaveOccurred())
			s.cfclient.AssertExpectations(GinkgoT())
		})

		It("Should succeed forwarding nine headers", func() {
			s.allowCreateWithExpectedHeaders(utils.Headers{"One": true, "Two": true, "Three": true, "Four": true, "Five": true, "Six": true, "Seven": true, "Eight": true, "Nine": true, "Host": true})
			s.setupCFClientListV3DomainsDomain1()

			details := domain.ProvisionDetails{
				OrganizationGUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
				RawParameters:    []byte(`{"domain": "domain1.cloud.gov", "headers": ["One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine"]}`),
			}
			_, err := s.Broker.Provision(s.ctx, "123", details, true)

			Expect(err).NotTo(HaveOccurred())
			s.cfclient.AssertExpectations(GinkgoT())
		})

		It("Should error when forwarding duplicate headers", func() {
			s.setupCFClientListV3DomainsDomain1()

			details := domain.ProvisionDetails{
				OrganizationGUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
				RawParameters:    []byte(`{"domain": "domain1.cloud.gov", "headers": ["User-Agent", "Host", "User-Agent"]}`),
			}
			_, err := s.Broker.Provision(s.ctx, "123", details, true)

			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("must not pass duplicated header 'User-Agent'")))
			s.cfclient.AssertExpectations(GinkgoT())
		})

		It("Should error when specifying a specific header and also wildcard headers", func() {
			s.setupCFClientListV3DomainsDomain1()

			details := domain.ProvisionDetails{
				OrganizationGUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
				RawParameters:    []byte(`{"domain": "domain1.cloud.gov", "headers": ["*", "User-Agent"]}`),
			}
			_, err := s.Broker.Provision(s.ctx, "123", details, true)

			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("must not pass whitelisted headers alongside wildcard")))
			s.cfclient.AssertExpectations(GinkgoT())
		})

		It("Should error when forwarding ten or more", func() {
			s.setupCFClientListV3DomainsDomain1()

			details := domain.ProvisionDetails{
				OrganizationGUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
				RawParameters:    []byte(`{"domain": "domain1.cloud.gov", "headers": ["One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine", "Ten"]}`),
			}
			_, err := s.Broker.Provision(s.ctx, "123", details, true)

			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("must not set more than 10 headers; got 11")))
			s.cfclient.AssertExpectations(GinkgoT())
		})
	})
})
