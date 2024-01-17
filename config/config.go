package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/pkg/errors"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	_ "github.com/lib/pq"
)

type Settings struct {
	Port                    string            `json:"port"`
	BrokerUsername          string            `json:"broker_username"`
	BrokerPassword          string            `json:"broker_password"`
	Host                    string            `json:"host"`
	DatabaseUrl             string            `json:"database_url"`
	DatabaseConnMaxLifetime string            `json:"database_conn_max_lifetime"`
	DatabaseMaxIdleConns    int               `json:"database_max_idle_conns"`
	CloudFrontPrefix        string            `json:"cloudfront_prefix"`
	AwsAccessKeyId          string            `json:"aws_access_key_id"`
	AwsSecretAccessKey      string            `json:"aws_secret_access_key"`
	AwsDefaultRegion        string            `json:"aws_region"`
	ServerSideEncryption    string            `json:"server_side_encryption"`
	APIAddress              string            `json:"api_address"`
	ClientID                string            `json:"client_id"`
	ClientSecret            string            `json:"client_secret"`
	DefaultOrigin           string            `json:"default_origin"`
	DefaultDefaultTTL       int64             `json:"default_default_ttl"`
	Schedule                string            `json:"schedule"`
	ExtraRequestHeaders     map[string]string `json:"extra_request_headers"`
	Tls                     *TLSConfig        `json:"tls"`
}

func LoadConfig(configFile string) (config *Settings, err error) {
	if configFile == "" {
		return config, errors.New("Must provide a config file")
	}

	file, err := os.Open(configFile)
	if err != nil {
		return config, err
	}
	defer file.Close()

	if err = json.NewDecoder(file).Decode(&config); err != nil {
		return config, err
	}

	if err = config.Validate(); err != nil {
		return config, fmt.Errorf("Validating config contents: %s", err)
	}

	return config, nil
}

func Connect(settings Settings) (*gorm.DB, error) {
	gormDb, err := gorm.Open("postgres", settings.DatabaseUrl)
	if err != nil {
		return nil, errors.Wrap(err, "database")
	}

	connMaxLifetime, err := time.ParseDuration(settings.DatabaseConnMaxLifetime)
	if err != nil {
		return nil, errors.Wrap(err, "database_conn_max_lifetime")
	}
	gormDb.DB().SetConnMaxLifetime(connMaxLifetime)

	gormDb.DB().SetMaxIdleConns(settings.DatabaseMaxIdleConns)

	return gormDb, nil
}

func (s Settings) TLSEnabled() bool {
	return s.Tls != nil
}

func (c Settings) Validate() error {
	if c.BrokerUsername == "" {
		return errors.New("Must provide a non-empty BrokerUsername")
	}

	if c.BrokerPassword == "" {
		return errors.New("Must provide a non-empty BrokerPassword")
	}

	if c.Schedule == "" {
		return errors.New("must provide a non-empty Schedule")
	}

	if c.Tls != nil {
		tlsValidation := c.Tls.validate()
		if tlsValidation != nil {
			return tlsValidation
		}
	}

	return nil
}
