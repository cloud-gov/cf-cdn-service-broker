package main

import (
	"net/http"
	"net/http/httptest"

	"code.cloudfoundry.org/lager"
	"github.com/alphagov/paas-cdn-broker/broker"
	"github.com/alphagov/paas-cdn-broker/config"
	"github.com/pivotal-cf/brokerapi/v8"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("HTTPHandler", func() {
	It("should handle HTTP correctly", func() {
		brokerAPI := brokerapi.New(
			&broker.CdnServiceBroker{},
			lager.NewLogger("main.test"),
			brokerapi.BrokerCredentials{},
		)
		handler := bindHTTPHandlers(brokerAPI, config.Settings{})
		req, err := http.NewRequest("GET", "http://example.com/healthcheck/http", nil)

		Expect(err).NotTo(HaveOccurred())

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		Expect(w.Code).To(Equal(200))
	})
})
