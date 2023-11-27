package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/jinzhu/gorm"

	"code.cloudfoundry.org/lager"
	"github.com/cloudfoundry-community/go-cfclient"
	"github.com/pivotal-cf/brokerapi/v8"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudfront"

	"github.com/alphagov/paas-cdn-broker/broker"
	"github.com/alphagov/paas-cdn-broker/config"
	"github.com/alphagov/paas-cdn-broker/healthchecks"
	"github.com/alphagov/paas-cdn-broker/models"
	"github.com/alphagov/paas-cdn-broker/utils"
)

func main() {
	logger := lager.NewLogger("cdn-service-broker")
	logger.RegisterSink(lager.NewWriterSink(os.Stderr, lager.INFO))

	settings, err := config.NewSettings()
	if err != nil {
		logger.Fatal("new-settings", err)
	}

	db, err := config.Connect(settings)
	if err != nil {
		logger.Fatal("config-connect", err)
	}

	cfClient, err := cfclient.NewClient(&cfclient.Config{
		ApiAddress:   settings.APIAddress,
		ClientID:     settings.ClientID,
		ClientSecret: settings.ClientSecret,
	})
	if err != nil {
		logger.Fatal("cf-client", err)
	}

	session, err := session.NewSession(aws.NewConfig().WithRegion(settings.AwsDefaultRegion))

	if err != nil {
		logger.Fatal("session", err)
	}

	if err := models.Migrate(db); err != nil {
		logger.Fatal("migrate", err)
	}

	manager := models.NewManager(
		logger,
		&utils.Distribution{Settings: settings, Service: cloudfront.New(session)},
		settings,
		models.RouteStore{Database: db, Logger: logger.Session("route-store", lager.Data{"entry-point": "broker"})},
		utils.NewCertificateManager(logger, settings, session),
	)
	broker := broker.New(
		&manager,
		cfClient,
		settings,
		logger,
	)
	credentials := brokerapi.BrokerCredentials{
		Username: settings.BrokerUsername,
		Password: settings.BrokerPassword,
	}

	brokerAPI := brokerapi.New(broker, logger, credentials)
	server := bindHTTPHandlers(brokerAPI, settings, db)
	http.ListenAndServe(fmt.Sprintf(":%s", settings.Port), server)
}

func bindHTTPHandlers(handler http.Handler, settings config.Settings, db *gorm.DB) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", handler)
	healthchecks.Bind(mux, settings, db)

	return mux
}
