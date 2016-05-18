package main

import (
	"os"
	"os/signal"

	"github.com/pivotal-golang/lager"
	"github.com/robfig/cron"

	"github.com/18F/cf-cdn-service-broker/config"
	"github.com/18F/cf-cdn-service-broker/models"
)

func main() {
	settings := config.NewSettings()

	logger := lager.NewLogger("cdn-cron")
	logger.RegisterSink(lager.NewWriterSink(os.Stderr, lager.INFO))

	db, err := config.Connect()
	if err != nil {
		logger.Fatal("Error", err)
	}

	c := cron.New()

	c.AddFunc("0 0 * * * *", func() {
		logger.Info("Running renew")
		models.Renew(settings, db)
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
