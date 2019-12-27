package broker_test

import (
	"context"
	"errors"

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
	s.Manager.On("Create", "123", "domain.gov", "origin.cloud.gov", "", s.settings.DefaultDefaultTTL, false, expectedHeaders, true,
		map[string]string{
			"Organization":    "",
			"Space":           "",
			"Service":         "",
			"ServiceInstance": "123",
			"Plan":            "",
		}).Return(route, nil)
}

func (s *ProvisionSuite) failCreateWithExpectedHeaders(expectedHeaders utils.Headers) {
	s.Manager.On("Create", "123", "domain.gov", "origin.cloud.gov", "", s.settings.DefaultDefaultTTL, false, expectedHeaders, true,
		map[string]string{
			"Organization":    "",
			"Space":           "",
			"Service":         "",
			"ServiceInstance": "123",
			"Plan":            "",
		}).Return(nil, errors.New("fail"))
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
		_, err := s.Broker.Provision(s.ctx, "", brokerapi.ProvisionDetails{}, false)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(brokerapi.ErrAsyncRequired))
	})

	It("Should error when the broker is called without config", func() {
		_, err := s.Broker.Provision(s.ctx, "", brokerapi.ProvisionDetails{}, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(ContainSubstring("must be invoked with configuration parameters")))
	})

	It("Should error when the broker is called without a domain", func() {
		details := brokerapi.ProvisionDetails{
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
		s.Manager.On("Get", "123").Return(route, nil)

		details := brokerapi.ProvisionDetails{
			RawParameters: []byte(`{"domain": "domain.gov"}`),
		}
		_, err := s.Broker.Provision(s.ctx, "123", details, true)

		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(brokerapi.ErrInstanceAlreadyExists))
	})

	It("Should succeed", func() {
		s.Manager.On("Get", "123").Return(&models.Route{}, errors.New("not found"))
		route := &models.Route{State: models.Provisioning}
		s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, nil)
		s.Manager.On("Create", "123", "domain.gov", "origin.cloud.gov", "", s.settings.DefaultDefaultTTL, false, utils.Headers{"Host": true}, true,
			map[string]string{
				"Organization":    "",
				"Space":           "",
				"Service":         "",
				"ServiceInstance": "123",
				"Plan":            "",
			}).Return(route, nil)

		details := brokerapi.ProvisionDetails{
			RawParameters: []byte(`{"domain": "domain.gov"}`),
		}
		_, err := s.Broker.Provision(s.ctx, "123", details, true)

		Expect(err).NotTo(HaveOccurred())
	})

	It("Should create a cloudfront instance with a custom DefaultTTL", func() {
		s.Manager.On("Get", "123").Return(&models.Route{}, errors.New("not found"))
		route := &models.Route{State: models.Provisioning}
		s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, nil)
		s.Manager.On("Create", "123", "domain.gov", "origin.cloud.gov", "", int64(52), false, utils.Headers{"Host": true}, true,
			map[string]string{
				"Organization":    "",
				"Space":           "",
				"ServiceInstance": "123",
				"Service":         "",
				"Plan":            "",
			}).Return(route, nil)

		details := brokerapi.ProvisionDetails{
			RawParameters: []byte(`{
				"domain": "domain.gov",
				"default_ttl": 52
			}`),
		}
		_, err := s.Broker.Provision(s.ctx, "123", details, true)

		Expect(err).NotTo(HaveOccurred())
	})

	It("Should succeed with a custom origin", func() {
		s.Manager.On("Get", "123").Return(&models.Route{}, errors.New("not found"))
		route := &models.Route{State: models.Provisioning}
		s.Manager.On("Create", "123", "domain.gov", "custom.cloud.gov", "", s.settings.DefaultDefaultTTL, false, utils.Headers{}, true,
			map[string]string{
				"Organization":    "",
				"Space":           "",
				"Service":         "",
				"ServiceInstance": "123",
				"Plan":            "",
			}).Return(route, nil)

		details := brokerapi.ProvisionDetails{
			RawParameters: []byte(`{"domain": "domain.gov", "origin": "custom.cloud.gov"}`),
		}
		_, err := s.Broker.Provision(s.ctx, "123", details, true)

		Expect(err).NotTo(HaveOccurred())
	})

	It("Should error when Cloud Foundry does not have the domain registered", func() {
		s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, errors.New("fail"))
		details := brokerapi.ProvisionDetails{
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
		details := brokerapi.ProvisionDetails{
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
		details := brokerapi.ProvisionDetails{
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
			s.Manager.On("Get", "123").Return(&models.Route{}, errors.New("not found"))
			s.cfclient.On("GetDomainByName", "domain.gov").Return(cfclient.Domain{}, nil)
		})

		It("Should succeed forwarding duplicate host header", func() {
			s.allowCreateWithExpectedHeaders(utils.Headers{"Host": true})

			details := brokerapi.ProvisionDetails{
				RawParameters: []byte(`{"domain": "domain.gov", "headers": ["Host"]}`),
			}
			_, err := s.Broker.Provision(s.ctx, "123", details, true)

			Expect(err).NotTo(HaveOccurred())
		})

		It("Should succeed forwarding a single header", func() {
			s.allowCreateWithExpectedHeaders(utils.Headers{"User-Agent": true, "Host": true})

			details := brokerapi.ProvisionDetails{
				RawParameters: []byte(`{"domain": "domain.gov", "headers": ["User-Agent"]}`),
			}
			_, err := s.Broker.Provision(s.ctx, "123", details, true)

			Expect(err).NotTo(HaveOccurred())
		})

		It("Should succeed forwarding wildcard headers", func() {
			s.allowCreateWithExpectedHeaders(utils.Headers{"*": true})

			details := brokerapi.ProvisionDetails{
				RawParameters: []byte(`{"domain": "domain.gov", "headers": ["*"]}`),
			}
			_, err := s.Broker.Provision(s.ctx, "123", details, true)

			Expect(err).NotTo(HaveOccurred())
		})

		It("Should succeed forwarding nine headers", func() {
			s.allowCreateWithExpectedHeaders(utils.Headers{"One": true, "Two": true, "Three": true, "Four": true, "Five": true, "Six": true, "Seven": true, "Eight": true, "Nine": true, "Host": true})

			details := brokerapi.ProvisionDetails{
				RawParameters: []byte(`{"domain": "domain.gov", "headers": ["One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine"]}`),
			}
			_, err := s.Broker.Provision(s.ctx, "123", details, true)

			Expect(err).NotTo(HaveOccurred())
		})

		It("Should error when forwarding duplicate headers", func() {
			s.failCreateWithExpectedHeaders(utils.Headers{"User-Agent": true, "Host": true})

			details := brokerapi.ProvisionDetails{
				RawParameters: []byte(`{"domain": "domain.gov", "headers": ["User-Agent", "Host", "User-Agent"]}`),
			}
			_, err := s.Broker.Provision(s.ctx, "123", details, true)

			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("must not pass duplicated header 'User-Agent'")))
		})

		It("Should error when forwarding wildcard headers and normal headers", func() {
			s.failCreateWithExpectedHeaders(utils.Headers{"*": true})

			details := brokerapi.ProvisionDetails{
				RawParameters: []byte(`{"domain": "domain.gov", "headers": ["*", "User-Agent"]}`),
			}
			_, err := s.Broker.Provision(s.ctx, "123", details, true)

			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("must not pass whitelisted headers alongside wildcard")))
		})

		It("Should error when forwarding ten or more", func() {
			s.failCreateWithExpectedHeaders(utils.Headers{"One": true, "Two": true, "Three": true, "Four": true, "Five": true, "Six": true, "Seven": true, "Eight": true, "Nine": true, "Ten": true, "Host": true})

			details := brokerapi.ProvisionDetails{
				RawParameters: []byte(`{"domain": "domain.gov", "headers": ["One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine", "Ten"]}`),
			}
			_, err := s.Broker.Provision(s.ctx, "123", details, true)

			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("must not set more than 10 headers; got 11")))
		})
	})
})
