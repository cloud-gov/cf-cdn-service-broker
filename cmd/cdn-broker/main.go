package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/pivotal-cf/brokerapi"
	"github.com/pivotal-golang/lager"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/18F/cf-cdn-service-broker/broker"
	"github.com/18F/cf-cdn-service-broker/config"
	"github.com/18F/cf-cdn-service-broker/models"
	"github.com/18F/cf-cdn-service-broker/utils"
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
		logger.Fatal("connect", err)
	}

	db.AutoMigrate(&models.Route{}, &models.Certificate{})

	session := session.New()
	manager := models.RouteManager{
		Iam:        &utils.Iam{iam.New(session)},
		CloudFront: &utils.Distribution{settings, cloudfront.New(session)},
		Acme:       &utils.Acme{settings, s3.New(session)},
		DB:         db,
	}
	broker := broker.CdnServiceBroker{
		Manager: &manager,
	}
	credentials := brokerapi.BrokerCredentials{
		Username: settings.Username,
		Password: settings.Password,
	}

	brokerAPI := brokerapi.New(&broker, logger, credentials)
	http.Handle("/", brokerAPI)
	http.ListenAndServe(fmt.Sprintf(":%s", settings.Port), nil)
}
