package main

import (
	"fmt"
	"net/http"
	"os"

	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/cloudfoundry-community/go-cfclient"

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

	client, err := cfclient.NewClient(&cfclient.Config{
		ApiAddress:   settings.APIAddress,
		ClientID:     settings.ClientID,
		ClientSecret: settings.ClientSecret,
	})
	if err != nil {
		logger.Fatal("client", err)
	}

	db.AutoMigrate(&models.Route{}, &models.Certificate{})

	session := session.New(aws.NewConfig().WithRegion(settings.AwsDefaultRegion))
	manager := models.RouteManager{
		Logger:     logger,
		Iam:        &utils.Iam{settings, iam.New(session)},
		CloudFront: &utils.Distribution{settings, cloudfront.New(session)},
		Acme:       &utils.Acme{settings, s3.New(session)},
		DB:         db,
	}
	broker := broker.New(
		&manager,
		client,
		settings,
		logger,
	)
	credentials := brokerapi.BrokerCredentials{
		Username: settings.BrokerUsername,
		Password: settings.BrokerPassword,
	}

	brokerAPI := brokerapi.New(broker, logger, credentials)
	http.Handle("/", brokerAPI)
	http.ListenAndServe(fmt.Sprintf(":%s", settings.Port), nil)
}
