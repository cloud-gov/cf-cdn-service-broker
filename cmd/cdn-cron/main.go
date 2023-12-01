package main

import (
	"os"
	"os/signal"

	"code.cloudfoundry.org/lager/v3"
	"github.com/robfig/cron"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudfront"

	"github.com/alphagov/paas-cdn-broker/config"
	"github.com/alphagov/paas-cdn-broker/models"
	"github.com/alphagov/paas-cdn-broker/utils"
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
		logger.Fatal("config-connect", err)
	}

	if err := db.AutoMigrate(&models.Route{}, &models.Certificate{}).Error; err != nil {
		logger.Fatal("migrate", err)
	}

	manager := models.NewManager(
		logger,
		&utils.Distribution{Settings: settings, Service: cloudfront.New(session)},
		settings,
		models.RouteStore{Database: db, Logger: logger.Session("route-store", lager.Data{"entry-point": "cron"})},
		utils.NewCertificateManager(logger, settings, session),
	)

	c := cron.New()

	c.AddFunc(settings.Schedule, func() {
		logger.Info("run-cert-cleanup")
		manager.DeleteOrphanedCerts()
	})

	c.AddFunc("@every 2m", func() {
		logger.Info("run-updating-routes-check")
		manager.CheckRoutesToUpdate()
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
