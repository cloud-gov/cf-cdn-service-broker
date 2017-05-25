package healthchecks

import (
	"net/http"
	"time"

	"github.com/cloudfoundry-community/go-cfclient"

	"github.com/18F/cf-cdn-service-broker/config"
)

func Cloudfoundry(settings config.Settings) error {
	// We're only validating that the CF endpoint is contactable here, as
	// testing the authentication is tricky
	_, err := cfclient.NewClient(&cfclient.Config{
		ApiAddress:   settings.APIAddress,
		ClientID:     settings.ClientID,
		ClientSecret: settings.ClientSecret,
		HttpClient: &http.Client{
			Timeout: time.Second * 10,
		},
	})
	if err != nil {
		return err
	}

	return nil
}
