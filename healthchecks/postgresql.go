package healthchecks

import (
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"

	"github.com/alphagov/paas-cdn-broker/config"
)

func CreatePostgresqlChecker(db *gorm.DB) (func(config.Settings) error) {
	return func(settings config.Settings) error {
		return db.DB().Ping()
	}
}
