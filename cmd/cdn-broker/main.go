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

	"github.com/18F/cf-cdn-service-broker/broker"
	"github.com/18F/cf-cdn-service-broker/config"
	"github.com/18F/cf-cdn-service-broker/iamcerts"
	"github.com/18F/cf-cdn-service-broker/iamuser"
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

	catalog, err := broker.LoadCatalog("./catalog.json")
	if err != nil {
		logger.Fatal("load-catalog", err)
	}

	db, err := config.Connect(settings)
	if err != nil {
		logger.Fatal("connect", err)
	}

	db.AutoMigrate(&models.Route{}, &models.Certificate{})

	sess := session.New(aws.NewConfig().WithRegion(settings.AwsDefaultRegion))
	distribution := &utils.Distribution{settings, cloudfront.New(sess)}
	user := iamuser.NewIAMUser(iam.New(sess), logger)

	manager := &models.RouteManager{
		Logger:     logger,
		Certs:      iamcerts.NewIAMCerts(settings, iam.New(sess), logger),
		CloudFront: &utils.Distribution{settings, cloudfront.New(sess)},
		Acme:       &utils.Acme{settings, s3.New(sess)},
		DB:         db,
	}
	broker := broker.NewCdnServiceBroker(
		manager,
		distribution,
		user,
		catalog,
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
