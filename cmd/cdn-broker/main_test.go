package main

import (
	"net/http"
	"net/http/httptest"

	"code.cloudfoundry.org/lager"
	"github.com/18F/cf-cdn-service-broker/broker"
	"github.com/18F/cf-cdn-service-broker/config"
	"github.com/pivotal-cf/brokerapi"

	. "github.com/onsi/ginkgo"
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
