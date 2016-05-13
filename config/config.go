package config

import (
	"fmt"
	"os"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	_ "github.com/lib/pq"
)

var Bucket string = os.Getenv("CDN_BUCKET")
var Region string = os.Getenv("CDN_REGION")

func Connect() (*gorm.DB, error) {
	conn := "host=%s, dbname=%s, user=%s, password=%s, sslmode=%s"
	conn = fmt.Sprintf(
		conn,
		os.Getenv("DB_URL"),
		os.Getenv("DB_NAME"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASS"),
	)
	return gorm.Open("postgres", conn)
}
