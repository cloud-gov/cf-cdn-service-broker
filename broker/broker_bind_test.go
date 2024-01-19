package broker_test

import (
	"context"

	"github.com/pivotal-cf/brokerapi/v10/domain"

	"code.cloudfoundry.org/lager/v3"
	"github.com/alphagov/paas-cdn-broker/broker"
	cfmock "github.com/alphagov/paas-cdn-broker/cf/mocks"
	"github.com/alphagov/paas-cdn-broker/config"
	"github.com/alphagov/paas-cdn-broker/models/mocks"

	. "github.com/onsi/ginkgo/v2"
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
		_, err := b.Bind(context.Background(), "", "", domain.BindDetails{}, false)
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
		_, err := b.Unbind(context.Background(), "", "", domain.UnbindDetails{}, false)
		Expect(err).To(HaveOccurred())
	})

})
