module github.com/18F/cf-cdn-service-broker

go 1.20

require (
	code.cloudfoundry.org/lager v1.0.1-0.20180322215153-25ee72f227fe
	github.com/aws/aws-sdk-go v1.13.20
	github.com/cloudfoundry-community/go-cfclient v0.0.0-20180323021324-b5f0f59f96d6
	github.com/jinzhu/gorm v1.9.1
	github.com/kelseyhightower/envconfig v1.3.0
	github.com/lib/pq v0.0.0-20180325232643-a96442e255fc
	github.com/pivotal-cf/brokerapi v1.0.0
	github.com/robfig/cron v1.0.0
	github.com/stretchr/testify v1.7.0
	github.com/xenolf/lego v0.0.0-00010101000000-000000000000
)

require (
	code.cloudfoundry.org/gofileutils v0.0.0-20170111115228-4d0c80011a0f // indirect
	github.com/cloudfoundry/gofileutils v0.0.0-20170111115228-4d0c80011a0f // indirect
	github.com/codegangsta/inject v0.0.0-20150114235600-33e0aa1cb7c0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/denisenkom/go-mssqldb v0.12.3 // indirect
	github.com/drewolson/testflight v1.0.0 // indirect
	github.com/erikstmartin/go-testdb v0.0.0-20160219214506-8d10e4a1bae5 // indirect
	github.com/go-ini/ini v1.33.0 // indirect
	github.com/go-martini/martini v0.0.0-20170121215854-22fa46961aab // indirect
	github.com/go-sql-driver/mysql v1.7.1 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/gorilla/context v0.0.0-20160226214623-1ea25387ff6f // indirect
	github.com/gorilla/mux v1.6.1 // indirect
	github.com/jinzhu/inflection v0.0.0-20180308033659-04140366298a // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/jmespath/go-jmespath v0.0.0-20160202185014-0b12d6b521d8 // indirect
	github.com/martini-contrib/render v0.0.0-20150707142108-ec18f8345a11 // indirect
	github.com/mattn/go-sqlite3 v1.14.16 // indirect
	github.com/miekg/dns v1.0.4 // indirect
	github.com/onsi/ginkgo v1.16.5 // indirect
	github.com/onsi/gomega v1.27.7 // indirect
	github.com/oxtoacart/bpool v0.0.0-20190530202638-03653db5a59c // indirect
	github.com/pborman/uuid v1.2.1 // indirect
	github.com/pkg/errors v0.8.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/smartystreets/goconvey v1.8.0 // indirect
	github.com/stretchr/objx v0.1.0 // indirect
	golang.org/x/crypto v0.17.0 // indirect
	golang.org/x/net v0.10.0 // indirect
	golang.org/x/oauth2 v0.0.0-20180314180239-fdc9e635145a // indirect
	golang.org/x/sys v0.15.0 // indirect
	google.golang.org/appengine v1.0.0 // indirect
	google.golang.org/protobuf v1.28.0 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/square/go-jose.v1 v1.1.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/xenolf/lego => github.com/jmcarp/lego v0.3.2-0.20170424160445-b4deb96f1082
