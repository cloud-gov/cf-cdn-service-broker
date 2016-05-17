package config

import (
	"os"

	"github.com/cloudfoundry-community/go-cfenv"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	_ "github.com/lib/pq"
)

func Connect() (*gorm.DB, error) {
	return gorm.Open("postgres", os.Getenv("DATABASE_URL"))
}

type Settings struct {
	Port         string
	BrokerUser   string
	BrokerPass   string
	DatabaseUrl  string
	Email        string
	AcmeUrl      string
	Bucket       string
	AwsAccessKey string
	AwsSecretKey string
	AwsRegion    string
}

func (s *Settings) exportAws() {
	os.Setenv("AWS_ACCESS_KEY_ID", s.AwsAccessKey)
	os.Setenv("AWS_SECRET_KEY_ID", s.AwsSecretKey)
	os.Setenv("AWS_DEFAULT_REGION", s.AwsRegion)
}

func NewSettings() Settings {
	env, err := cfenv.Current()
	if err != nil {
		env = &cfenv.App{}
	}

	creds, err := env.Services.WithName("cdn-creds")
	if err != nil {
		creds = &cfenv.Service{}
	}

	s := Settings{
		Port:         serviceOrEnv(creds, "PORT"),
		BrokerUser:   serviceOrEnv(creds, "BROKER_USER"),
		BrokerPass:   serviceOrEnv(creds, "BROKER_PASS"),
		DatabaseUrl:  serviceOrEnv(creds, "DATABASE_URL"),
		Email:        serviceOrEnv(creds, "EMAIL"),
		AcmeUrl:      serviceOrEnv(creds, "ACME_URL"),
		Bucket:       serviceOrEnv(creds, "BUCKET"),
		AwsAccessKey: serviceOrEnv(creds, "AWS_ACCESS_KEY_ID"),
		AwsSecretKey: serviceOrEnv(creds, "AWS_SECRET_ACCESS_KEY"),
		AwsRegion:    serviceOrEnv(creds, "AWS_DEFAULT_REGION"),
	}
	s.exportAws()

	return s
}

func serviceOrEnv(service *cfenv.Service, key string) string {
	raw, ok := service.Credentials[key]
	if !ok {
		return os.Getenv(key)
	}
	value, ok := raw.(string)
	if !ok {
		return os.Getenv(key)
	}
	return value
}
