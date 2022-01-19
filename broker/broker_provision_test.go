package broker_test

import (
	"context"
	"errors"
	"github.com/pivotal-cf/brokerapi/v8/domain"
	"github.com/pivotal-cf/brokerapi/v8/domain/apiresponses"
	"github.com/stretchr/testify/suite"
	"reflect"

	"code.cloudfoundry.org/lager"
	"github.com/alphagov/paas-cdn-broker/broker"
	cfmock "github.com/alphagov/paas-cdn-broker/cf/mocks"
	"github.com/alphagov/paas-cdn-broker/config"
	"github.com/alphagov/paas-cdn-broker/models"
	"github.com/alphagov/paas-cdn-broker/models/mocks"
	"github.com/alphagov/paas-cdn-broker/utils"
	"github.com/cloudfoundry-community/go-cfclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type ProvisionSuite struct {
	suite.Suite
	Manager  mocks.RouteManagerIface
	Broker   *broker.CdnServiceBroker
	cfclient cfmock.Client
	settings config.Settings
	logger   lager.Logger
	ctx      context.Context
}

func (s *ProvisionSuite) allowCreateWithExpectedHeaders(expectedHeaders utils.Headers) {
	route := &models.Route{State: models.Provisioning}
	s.Manager.CreateStub = func(
		_ string,
		_ string,
		_ string,
		_ int64,
		headers utils.Headers,
		_ bool,
		_ map[string]string) (*models.Route, error) {

		if reflect.DeepEqual(headers, expectedHeaders) {
			return route, nil
		}

		return nil, errors.New("unexpected header values")
	}
}

var _ = Describe("Last operation", func() {
	var s *ProvisionSuite = &ProvisionSuite{}

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

		s.cfclient.On("GetOrgByGuid", "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5").Return(cfclient.Org{Name: "my-org"}, nil)
	})

	It("Should error when the broker is called synchronously", func() {
		_, err := s.Broker.Provision(s.ctx, "", domain.ProvisionDetails{}, false)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(apiresponses.ErrAsyncRequired))
	})

	It("Should error when the broker is called without config", func() {
		_, err := s.Broker.Provision(s.ctx, "", domain.ProvisionDetails{}, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(ContainSubstring("must be invoked with configuration parameters")))
	})

	It("Should error when the broker is called without a domain", func() {
		details := domain.ProvisionDetails{
			RawParameters: []byte(`{}`),
		}
		_, err := s.Broker.Provision(s.ctx, "", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(ContainSubstring("must pass non-empty `domain`")))
	})

	It("Should error when the broker is called with an already existing domain", func() {
		route := &models.Route{
			State: models.Provisioned,
		}
		s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, nil)
		s.Manager.GetReturns(route, nil)

		details := domain.ProvisionDetails{
			RawParameters: []byte(`{"domain": "domain.gov"}`),
		}
		_, err := s.Broker.Provision(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(apiresponses.ErrInstanceAlreadyExists))
	})

	It("Should succeed", func() {
		s.Manager.GetReturns(&models.Route{}, errors.New("not found"))
		route := &models.Route{State: models.Provisioning}
		s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, nil)
		s.Manager.CreateReturns(route, nil)

		details := domain.ProvisionDetails{
			RawParameters: []byte(`{"domain": "domain.gov"}`),
		}
		_, err := s.Broker.Provision(s.ctx, "123", details, true)

		Expect(err).NotTo(HaveOccurred())
	})

	It("Should create a cloudfront instance with a custom DefaultTTL", func() {
		s.Manager.GetReturns(&models.Route{}, errors.New("not found"))
		route := &models.Route{State: models.Provisioning}
		s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, nil)
		s.Manager.CreateReturns(route, nil)

		details := domain.ProvisionDetails{
			RawParameters: []byte(`{
				"domain": "domain.gov",
				"default_ttl": 52
			}`),
		}
		_, err := s.Broker.Provision(s.ctx, "123", details, true)

		Expect(err).NotTo(HaveOccurred())
		Expect(s.Manager.CreateCallCount()).To(Equal(1))
		_, _, _, ttl, _, _, _ := s.Manager.CreateArgsForCall(0)
		Expect(ttl).To(Equal(int64(52)))
	})

	It("Should set the correct tags", func() {
		instanceId := "123"
		s.Manager.GetReturns(&models.Route{}, errors.New("not found"))
		route := &models.Route{State: models.Provisioning}
		s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, nil)
		s.Manager.CreateReturns(route, nil)

		details := domain.ProvisionDetails{
			RawParameters:    []byte(`{"domain": "domain.gov"}`),
			OrganizationGUID: "org-1",
			SpaceGUID:        "space-1",
			ServiceID:        "service-1",
			PlanID:           "plan-1",
		}

		_, err := s.Broker.Provision(s.ctx, instanceId, details, true)

		Expect(err).NotTo(HaveOccurred())

		Expect(s.Manager.CreateCallCount()).To(Equal(1))
		_, _, _, _, _, _, inputTags := s.Manager.CreateArgsForCall(0)
		Expect(inputTags).To(HaveKeyWithValue("Organization", "org-1"))
		Expect(inputTags).To(HaveKeyWithValue("Space", "space-1"))
		Expect(inputTags).To(HaveKeyWithValue("Service", "service-1"))
		Expect(inputTags).To(HaveKeyWithValue("ServiceInstance", instanceId))
		Expect(inputTags).To(HaveKeyWithValue("Plan", "plan-1"))
		Expect(inputTags).To(HaveKeyWithValue("chargeable_entity", instanceId))
	})

	It("Should error when Cloud Foundry does not have the domain registered", func() {
		s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, errors.New("fail"))
		details := domain.ProvisionDetails{
			OrganizationGUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			RawParameters:    []byte(`{"domain": "domain.gov"}`),
		}
		_, err := s.Broker.Provision(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(ContainSubstring("cf create-domain")))
	})

	It("Should error when given multiple domains one of which Cloud Foundry does not have registered", func() {
		s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, nil)
		s.cfclient.On("GetDomainByName", "domain2.gov").Return(cfclient.Domain{}, errors.New("fail"))
		details := domain.ProvisionDetails{
			OrganizationGUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			RawParameters:    []byte(`{"domain": "domain.gov,domain2.gov"}`),
		}
		_, err := s.Broker.Provision(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(ContainSubstring("Domain does not exist")))
		Expect(err).NotTo(MatchError(ContainSubstring("domain.gov")))
		Expect(err).To(MatchError(ContainSubstring("domain2.gov")))
	})

	It("Should error when given multiple domains many of which Cloud Foundry does not have registered", func() {
		s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, nil)
		s.cfclient.On("GetDomainByName", "domain2.gov").Return(cfclient.Domain{}, errors.New("fail"))
		s.cfclient.On("GetDomainByName", "domain3.gov").Return(cfclient.Domain{}, errors.New("fail"))
		details := domain.ProvisionDetails{
			OrganizationGUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
			RawParameters:    []byte(`{"domain": "domain.gov,domain2.gov,domain3.gov"}`),
		}
		_, err := s.Broker.Provision(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(ContainSubstring("Multiple domains do not exist")))
		Expect(err).NotTo(MatchError(ContainSubstring("domain.gov")))
		Expect(err).To(MatchError(ContainSubstring("domain2.gov")))
		Expect(err).To(MatchError(ContainSubstring("domain3.gov")))
	})

	Context("Headers", func() {
		BeforeEach(func() {
			s.Manager.GetReturns(&models.Route{}, errors.New("not found"))
			s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, nil)
		})

		It("Should succeed forwarding duplicate host header", func() {
			s.allowCreateWithExpectedHeaders(utils.Headers{"Host": true})

			details := domain.ProvisionDetails{
				RawParameters: []byte(`{"domain": "domain.gov", "headers": ["Host"]}`),
			}
			_, err := s.Broker.Provision(s.ctx, "123", details, true)

			Expect(err).NotTo(HaveOccurred())
		})

		It("Should succeed forwarding a single header", func() {
			s.allowCreateWithExpectedHeaders(utils.Headers{"User-Agent": true, "Host": true})

			details := domain.ProvisionDetails{
				RawParameters: []byte(`{"domain": "domain.gov", "headers": ["User-Agent"]}`),
			}
			_, err := s.Broker.Provision(s.ctx, "123", details, true)

			Expect(err).NotTo(HaveOccurred())
		})

		It("Should succeed forwarding wildcard headers", func() {
			s.allowCreateWithExpectedHeaders(utils.Headers{"*": true})

			details := domain.ProvisionDetails{
				RawParameters: []byte(`{"domain": "domain.gov", "headers": ["*"]}`),
			}
			_, err := s.Broker.Provision(s.ctx, "123", details, true)

			Expect(err).NotTo(HaveOccurred())
		})

		It("Should succeed forwarding nine headers", func() {
			s.allowCreateWithExpectedHeaders(utils.Headers{"One": true, "Two": true, "Three": true, "Four": true, "Five": true, "Six": true, "Seven": true, "Eight": true, "Nine": true, "Host": true})

			details := domain.ProvisionDetails{
				RawParameters: []byte(`{"domain": "domain.gov", "headers": ["One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine"]}`),
			}
			_, err := s.Broker.Provision(s.ctx, "123", details, true)

			Expect(err).NotTo(HaveOccurred())
		})

		It("Should error when forwarding duplicate headers", func() {
			details := domain.ProvisionDetails{
				RawParameters: []byte(`{"domain": "domain.gov", "headers": ["User-Agent", "Host", "User-Agent"]}`),
			}
			_, err := s.Broker.Provision(s.ctx, "123", details, true)

			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("must not pass duplicated header 'User-Agent'")))
		})

		It("Should error when specifying a specific header and also wildcard headers", func() {
			details := domain.ProvisionDetails{
				RawParameters: []byte(`{"domain": "domain.gov", "headers": ["*", "User-Agent"]}`),
			}
			_, err := s.Broker.Provision(s.ctx, "123", details, true)

			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("must not pass whitelisted headers alongside wildcard")))
		})

		It("Should error when forwarding ten or more", func() {
			details := domain.ProvisionDetails{
				RawParameters: []byte(`{"domain": "domain.gov", "headers": ["One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine", "Ten"]}`),
			}
			_, err := s.Broker.Provision(s.ctx, "123", details, true)

			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("must not set more than 10 headers; got 11")))
		})
	})
})
