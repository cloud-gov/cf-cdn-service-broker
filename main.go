package main

import (
	"fmt"
	"net/http"

	"github.com/pivotal-cf/brokerapi"
	"github.com/pivotal-golang/lager"
	"github.com/xenolf/lego/acme"

	"github.com/18F/cf-cdn-service-broker/broker"
	"github.com/18F/cf-cdn-service-broker/config"
	"github.com/18F/cf-cdn-service-broker/models"
)

func main() {
	settings := config.NewSettings()

	db, err := config.Connect()
	if err != nil {
		fmt.Println(err)
	}

	db.AutoMigrate(&models.Route{}, &models.Certificate{})

	broker := broker.CdnServiceBroker{
		Settings: settings,
		DB:       db,
	}
	logger := lager.NewLogger("cdn-service-broker")
	credentials := brokerapi.BrokerCredentials{
		Username: settings.BrokerUser,
		Password: settings.BrokerPass,
	}

	brokerAPI := brokerapi.New(&broker, logger, credentials)
	http.Handle("/", brokerAPI)
	http.ListenAndServe(fmt.Sprintf(":%s", settings.Port), nil)
}
