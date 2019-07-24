package broker_test

import (
	"context"

	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi"

	"github.com/18F/cf-cdn-service-broker/broker"
	cfmock "github.com/18F/cf-cdn-service-broker/cf/mocks"
	"github.com/18F/cf-cdn-service-broker/config"
	"github.com/18F/cf-cdn-service-broker/models/mocks"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Bind", func() {

	It("Should error when binding", func() {
		b := broker.New(
			&mocks.RouteManagerIface{},
			&cfmock.Client{},
			config.Settings{
				DefaultOrigin: "origin.cloud.gov",
			},
			lager.NewLogger("test"),
		)
		_, err := b.Bind(context.Background(), "", "", brokerapi.BindDetails{})
		Expect(err).To(HaveOccurred())
	})

	It("Should error when unbinding", func() {
		b := broker.New(
			&mocks.RouteManagerIface{},
			&cfmock.Client{},
			config.Settings{
				DefaultOrigin: "origin.cloud.gov",
			},
			lager.NewLogger("test"),
		)
		err := b.Unbind(context.Background(), "", "", brokerapi.UnbindDetails{})
		Expect(err).To(HaveOccurred())
	})

})
