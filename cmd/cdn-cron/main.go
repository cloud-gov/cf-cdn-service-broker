package main

import (
	"fmt"
	"os"
	"os/signal"

	"code.cloudfoundry.org/lager"
	"github.com/robfig/cron"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/xenolf/lego/acme"

	"github.com/18F/cf-cdn-service-broker/config"
	"github.com/18F/cf-cdn-service-broker/models"
	"github.com/18F/cf-cdn-service-broker/utils"
)

func main() {
	logger := lager.NewLogger("cdn-cron")
	logger.RegisterSink(lager.NewWriterSink(os.Stderr, lager.INFO))

	settings, err := config.NewSettings()
	if err != nil {
		logger.Fatal("new-settings", err)
	}

	session := session.New(aws.NewConfig().WithRegion(settings.AwsDefaultRegion))

	db, err := config.Connect(settings)
	if err != nil {
		logger.Fatal("connect", err)
	}

	user, userData, err := models.GetOrCreateUser(db, settings.Email)
	fmt.Println("DEBUG:USER:GETORCREATE", user, userData, err)
	if err != nil {
		logger.Fatal("acme-create-user", err)
	}

	acmeClients := map[acme.Challenge]*acme.Client{}
	acmeClients[acme.HTTP01], err = utils.NewClient(settings, &user, s3.New(session), []acme.Challenge{acme.TLSSNI01, acme.DNS01})
	if err != nil {
		logger.Fatal("acme-client", err)
	}
	acmeClients[acme.DNS01], err = utils.NewClient(settings, &user, s3.New(session), []acme.Challenge{acme.TLSSNI01, acme.HTTP01})
	if err != nil {
		logger.Fatal("acme-client", err)
	}

	if err := models.SaveUser(db, user, userData); err != nil {
		logger.Fatal("acme-save-user", err)
	}

	manager := models.NewManager(
		logger,
		&utils.Iam{settings, iam.New(session)},
		&utils.Distribution{settings, cloudfront.New(session)},
		user,
		acmeClients,
		db,
	)

	c := cron.New()

	c.AddFunc(settings.Schedule, func() {
		logger.Info("Running renew")
		manager.RenewAll()
	})

	c.AddFunc(settings.Schedule, func() {
		logger.Info("Running cert cleanup")
		manager.DeleteOrphanedCerts()
	})

	logger.Info("Starting cron")
	c.Start()

	waitForExit()
}

func waitForExit() os.Signal {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)
	return <-c
}
