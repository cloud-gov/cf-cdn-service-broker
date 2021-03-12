package broker_test

import (
	"context"

	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi"

	"github.com/alphagov/paas-cdn-broker/broker"
	cfmock "github.com/alphagov/paas-cdn-broker/cf/mocks"
	"github.com/alphagov/paas-cdn-broker/config"
	"github.com/alphagov/paas-cdn-broker/models/mocks"

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
		_, err := b.Bind(context.Background(), "", "", brokerapi.BindDetails{}, false)
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
		_, err := b.Unbind(context.Background(), "", "", brokerapi.UnbindDetails{}, false)
		Expect(err).To(HaveOccurred())
	})

})
