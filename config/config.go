package config

import (
	"os"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	_ "github.com/lib/pq"
)

var Bucket string = os.Getenv("CDN_BUCKET")
var Region string = os.Getenv("CDN_REGION")

func Connect() (*gorm.DB, error) {
	return gorm.Open("postgres", os.Getenv("DATABASE_URL"))
}
