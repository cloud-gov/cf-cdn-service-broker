package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"code.cloudfoundry.org/lager"
	"github.com/18F/cf-cdn-service-broker/broker"
	"github.com/18F/cf-cdn-service-broker/config"
	"github.com/pivotal-cf/brokerapi"
)

func TestHTTPHandler(t *testing.T) {
	brokerAPI := brokerapi.New(
		&broker.CdnServiceBroker{},
		lager.NewLogger("main.test"),
		brokerapi.BrokerCredentials{},
	)
	handler := bindHTTPHandlers(brokerAPI, config.Settings{})
	req, err := http.NewRequest("GET", "http://example.com/healthcheck/http", nil)
	if err != nil {
		t.Error("Building new HTTP request: error should not have occurred")
	}

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("HTTP response: response code was %d, expecting 200", w.Code)
	}
}
