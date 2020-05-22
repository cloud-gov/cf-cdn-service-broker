package broker_test

import (
	"context"
	"errors"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/suite"

	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi"

	"github.com/18F/cf-cdn-service-broker/broker"
	cfmock "github.com/18F/cf-cdn-service-broker/cf/mocks"
	"github.com/18F/cf-cdn-service-broker/config"
	"github.com/18F/cf-cdn-service-broker/models"
	"github.com/18F/cf-cdn-service-broker/models/mocks"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type LastOperationSuite struct {
	suite.Suite
	Manager  mocks.RouteManagerIface
	Broker   *broker.CdnServiceBroker
	cfclient cfmock.Client
	settings config.Settings
	logger   lager.Logger
	ctx      context.Context
}

var _ = Describe("Last operation", func() {
	var s *LastOperationSuite = &LastOperationSuite{}

	BeforeEach(func() {
		s.Manager = mocks.RouteManagerIface{}
		s.cfclient = cfmock.Client{}
		s.logger = lager.NewLogger("test")
		s.Broker = broker.New(
			&s.Manager,
			&s.cfclient,
			s.settings,
			s.logger,
		)
		s.ctx = context.Background()
	})

	It("Should fail when the routes are missing", func() {
		manager := mocks.RouteManagerIface{}
		manager.On("Get", "").Return(&models.Route{}, errors.New("not found"))
		b := broker.New(
			&manager,
			&s.cfclient,
			s.settings,
			s.logger,
		)

		operation, err := b.LastOperation(s.ctx, "", brokerapi.PollDetails{OperationData: ""})
		Expect(operation.State).To(Equal(brokerapi.Failed))
		Expect(operation.Description).To(Equal("Service instance not found"))
		Expect(err).NotTo(HaveOccurred())
	})

	It("Should succeed when provisioned", func() {
		manager := mocks.RouteManagerIface{}
		route := &models.Route{
			State:          models.Provisioned,
			DomainExternal: "cdn.cloud.gov",
			DomainInternal: "abc.cloudfront.net",
			Origin:         "cdn.apps.cloud.gov",
		}
		manager.On("Get", "123").Return(route, nil)
		b := broker.New(
			&manager,
			&s.cfclient,
			s.settings,
			s.logger,
		)

		operation, err := b.LastOperation(s.ctx, "123", brokerapi.PollDetails{OperationData: ""})
		Expect(operation.State).To(Equal(brokerapi.Succeeded))
		Expect(operation.Description).To(Equal("Service instance provisioned [cdn.cloud.gov => cdn.apps.cloud.gov]; CDN domain abc.cloudfront.net"))
		Expect(err).NotTo(HaveOccurred())
	})

	It("Should be in progress when provisioning", func() {
		manager := mocks.RouteManagerIface{}
		route := &models.Route{
			State:          models.Provisioning,
			DomainExternal: "cdn.cloud.gov",
			Origin:         "cdn.apps.cloud.gov",
			ChallengeJSON:  []byte("[]"),
			Model: gorm.Model{
				CreatedAt: time.Now().Add(-5 * time.Minute),
			},
		}
		manager.On("Get", "123").Return(route, nil)
		manager.On("GetDNSInstructions", route).Return([]string{"token"}, nil)
		b := broker.New(
			&manager,
			&s.cfclient,
			s.settings,
			s.logger,
		)

		operation, err := b.LastOperation(s.ctx, "123", brokerapi.PollDetails{OperationData: ""})
		Expect(operation.State).To(Equal(brokerapi.InProgress))
		Expect(operation.Description).To(ContainSubstring("Provisioning in progress"))
		Expect(err).NotTo(HaveOccurred())
	})

	It("Should be failed when the route's state is failed", func() {
		manager := mocks.RouteManagerIface{}
		route := &models.Route{
			State:          models.Failed,
			DomainExternal: "cdn.cloud.gov",
			Origin:         "cdn.apps.cloud.gov",
			ChallengeJSON:  []byte("[]"),
			Model: gorm.Model{
				CreatedAt: time.Now().Add(-5 * time.Minute),
			},
		}
		manager.On("Get", "123").Return(route, nil)
		manager.On("GetDNSInstructions", route).Return([]string{"token"}, nil)
		b := broker.New(
			&manager,
			&s.cfclient,
			s.settings,
			s.logger,
		)

		operation, err := b.LastOperation(s.ctx, "123", brokerapi.PollDetails{OperationData: ""})
		Expect(operation.State).To(Equal(brokerapi.Failed))
		Expect(err).NotTo(HaveOccurred())
	})

	It("Should be in progress when deprovisioning", func() {
		manager := mocks.RouteManagerIface{}
		route := &models.Route{
			State:          models.Deprovisioning,
			DomainExternal: "cdn.cloud.gov",
			DomainInternal: "abc.cloudfront.net",
			Origin:         "cdn.apps.cloud.gov",
		}
		manager.On("Get", "123").Return(route, nil)
		b := broker.New(
			&manager,
			&s.cfclient,
			s.settings,
			s.logger,
		)

		operation, err := b.LastOperation(s.ctx, "123", brokerapi.PollDetails{OperationData: ""})
		Expect(operation.State).To(Equal(brokerapi.InProgress))
		Expect(operation.Description).To(Equal("Deprovisioning in progress [cdn.cloud.gov => cdn.apps.cloud.gov]; CDN domain abc.cloudfront.net"))
		Expect(err).NotTo(HaveOccurred())
	})

})
