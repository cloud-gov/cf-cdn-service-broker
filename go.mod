module github.com/18F/cf-cdn-service-broker

go 1.12

require (
	code.cloudfoundry.org/lager v2.0.0+incompatible
	github.com/aws/aws-sdk-go v1.13.20
	github.com/cenkalti/backoff v2.1.1+incompatible // indirect
	github.com/cloudfoundry-community/go-cfclient v0.0.0-20190201205600-f136f9222381
	github.com/go-acme/lego v2.4.0+incompatible
	github.com/go-ini/ini v1.42.0 // indirect
	github.com/jinzhu/gorm v1.9.4
	github.com/jmespath/go-jmespath v0.0.0-20180206201540-c2b33e8439af // indirect
	github.com/kelseyhightower/envconfig v1.3.0
	github.com/lib/pq v1.0.0
	github.com/miekg/dns v1.1.8 // indirect
	github.com/pivotal-cf/brokerapi v4.2.3+incompatible
	github.com/robfig/cron v0.0.0-20180505203441-b41be1df6967
	github.com/stretchr/testify v1.3.0
	github.com/xenolf/lego v2.4.0+incompatible
	golang.org/x/crypto v0.0.0-20190325154230-a5d413f7728c
	gopkg.in/square/go-jose.v2 v2.3.1 // indirect
)
