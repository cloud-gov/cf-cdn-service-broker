package healthchecks

import (
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"

	"github.com/18F/cf-cdn-service-broker/config"
)

func Postgresql(settings config.Settings) error {
	db, err := gorm.Open("postgres", settings.DatabaseUrl)
	defer db.Close()

	if err != nil {
		return err
	}

	return nil
}
