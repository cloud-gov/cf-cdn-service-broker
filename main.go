package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/pivotal-cf/brokerapi"
	"github.com/pivotal-golang/lager"

	"github.com/18F/cf-cdn-service-broker/broker"
	"github.com/18F/cf-cdn-service-broker/config"
	"github.com/18F/cf-cdn-service-broker/models"
)

func main() {
	db, err := config.Connect()
	if err != nil {
		fmt.Println(err)
	}

	db.AutoMigrate(&models.Route{}, &models.Certificate{})

	broker := broker.CdnServiceBroker{DB: db}
	logger := lager.NewLogger("cdn-service-broker")
	credentials := brokerapi.BrokerCredentials{
		Username: os.Getenv("CDN_USER"),
		Password: os.Getenv("CDN_PASS"),
	}

	brokerAPI := brokerapi.New(&broker, logger, credentials)
	http.Handle("/", brokerAPI)
	http.ListenAndServe(fmt.Sprintf(":%s", os.Getenv("PORT")), nil)
}
