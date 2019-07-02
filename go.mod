module github.com/18F/cf-cdn-service-broker

go 1.12

require (
	code.cloudfoundry.org/lager v2.0.0+incompatible
	github.com/JamesClonk/vultr v2.0.1+incompatible
	github.com/aws/aws-sdk-go v1.20.18
	github.com/cloudfoundry-community/go-cfclient v0.0.0-20190611131856-16c98753d315
	github.com/decker502/dnspod-go v0.2.0
	github.com/dnsimple/dnsimple-go v0.30.0
	github.com/edeckers/auroradnsclient v1.0.3
	github.com/gorilla/mux v1.7.3 // indirect
	github.com/jinzhu/gorm v1.9.10
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/lib/pq v1.1.1
	github.com/miekg/dns v1.1.15
	github.com/ovh/go-ovh v0.0.0-20181109152953-ba5adb4cf014
	github.com/pivotal-cf/brokerapi v6.0.0+incompatible
	github.com/rainycape/memcache v0.0.0-20150622160815-1031fa0ce2f2
	github.com/robfig/cron v1.2.0
	github.com/sirupsen/logrus v1.4.2 // indirect
	github.com/stretchr/testify v1.3.0
	github.com/timewasted/linode v0.0.0-20160829202747-37e84520dcf7
	github.com/urfave/cli v1.20.0
	golang.org/x/crypto v0.0.0-20190701094942-4def268fd1a4
	golang.org/x/net v0.0.0-20190628185345-da137c7871d7
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45
	golang.org/x/sys v0.0.0-20190710143415-6ec70d6a5542 // indirect
	golang.org/x/tools v0.0.0-20190710184609-286818132824 // indirect
	google.golang.org/api v0.7.0
	gopkg.in/ini.v1 v1.44.0 // indirect
	gopkg.in/ns1/ns1-go.v2 v2.0.0-20190703192230-737a440630af
	gopkg.in/square/go-jose.v1 v1.1.2
)
