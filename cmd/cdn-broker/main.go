package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/jinzhu/gorm"

	"code.cloudfoundry.org/lager/v3"
	"github.com/cloudfoundry-community/go-cfclient"
	"github.com/pivotal-cf/brokerapi/v10"

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

	err = startHTTPServer(&settings, broker, db, logger)
	if err != nil {
		logger.Fatal("Failed to start broker process: %s", err)
	}
}

func buildHTTPHandler(serviceBroker *broker.CdnServiceBroker, logger lager.Logger, config *config.Settings, db *gorm.DB) http.Handler {
	credentials := brokerapi.BrokerCredentials{
		Username: config.BrokerUsername,
		Password: config.BrokerPassword,
	}

	brokerAPI := brokerapi.New(serviceBroker, logger, credentials)
	mux := http.NewServeMux()
	mux.Handle("/", brokerAPI)
	healthchecks.Bind(mux, *config, db)
	return mux
}

func startHTTPServer(
	cfg *config.Settings,
	serviceBroker *broker.CdnServiceBroker,
	db *gorm.DB,
	logger lager.Logger,
) error {
	server := buildHTTPHandler(serviceBroker, logger, cfg, db)

	listenAddress := fmt.Sprintf("%s:%s", cfg.Host, cfg.Port)
	// We don't use http.ListenAndServe here so that the "start" log message is
	// logged after the socket is listening. This log message is used by the
	// tests to wait until the broker is ready.
	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.Port))
	if err != nil {
		return fmt.Errorf("failed to listen on address %s: %s", listenAddress, err)
	}
	if cfg.TLSEnabled() {
		tlsConfig, err := cfg.Tls.GenerateTLSConfig()
		if err != nil {
			logger.Fatal("Error configuring TLS: %s", err)
		}
		listener = tls.NewListener(listener, tlsConfig)
		logger.Info("start", lager.Data{"port": cfg.Port, "tls": true, "host": cfg.Host, "address": listenAddress})
	} else {
		logger.Info("start", lager.Data{"port": cfg.Port, "tls": false, "host": cfg.Host, "address": listenAddress})
	}
	return http.Serve(listener, server)
}
