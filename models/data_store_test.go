package models_test

import (
	"os"
	"time"

	"code.cloudfoundry.org/lager/v3"

	"github.com/alphagov/paas-cdn-broker/models"
	"github.com/jinzhu/gorm"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DataStore", func() {
	var (
		db          gorm.DB
		transaction gorm.DB
		logger      lager.Logger
	)

	BeforeEach(func() {
		database, err := gorm.Open("postgres", os.Getenv("POSTGRES_URL"))

		Expect(err).ToNot(HaveOccurred())

		db = *database

		transaction = *db.Begin()
		err = models.Migrate(&db)
		Expect(err).ToNot(HaveOccurred())

		logger = lager.NewLogger("data-store-test")
	})

	AfterEach(func() {
		transaction.Rollback()
	})

	Describe("RouteStore", func() {
		Describe("FindOneMatching", func() {
			BeforeEach(func() {
				validatingCert := models.Certificate{
					Domain:            "foo.bar,blog.foo.bar",
					CertURL:           "cert.url",
					CertificateStatus: models.CertificateStatusValidating,
					CertificateArn:    "ValidatingCertArn",
				}

				orphanedCert := models.Certificate{
					Domain:            "example.com,blog.example.com",
					CertURL:           "cert.url",
					CertificateStatus: models.CertificateStatusValidating,
					CertificateArn:    "ValidatingCertArn",
				}

				complexRoute := &models.Route{
					InstanceId:     "complex-route",
					State:          "provisioned",
					ChallengeJSON:  []byte(`{}`),
					DomainExternal: "foo.bar",
					DomainInternal: "foo.london.cloudapps.digital",
					DistId:         "cloudfront-dist",
					Origin:         "foo.london.cloudapps.diigtal",
					Path:           "",
					InsecureOrigin: false,
					DefaultTTL:     0,
				}

				err := transaction.Save(complexRoute).Error
				Expect(err).ToNot(HaveOccurred())

				validatingCert.RouteId = complexRoute.ID
				err = transaction.Save(&validatingCert).Error
				Expect(err).ToNot(HaveOccurred())

				orphanedCert.RouteId = complexRoute.ID + 1
				err = transaction.Save(&orphanedCert).Error
				Expect(err).ToNot(HaveOccurred())

			})

			It("finds the first row matching the input", func() {
				routeStore := models.RouteStore{
					Database: &transaction,
					Logger:   logger,
				}

				route, err := routeStore.FindOneMatching(models.Route{
					InstanceId: "complex-route",
				})

				Expect(err).ToNot(HaveOccurred())
				Expect(route).ToNot(BeNil())
				Expect(route.ID).To(Equal(uint(1)))
			})

			It("returns an error if it can't find a matching row", func() {
				routeStore := models.RouteStore{
					Database: &transaction,
					Logger:   logger,
				}

				_, err := routeStore.FindOneMatching(models.Route{
					InstanceId: "no-match",
				})

				Expect(err).To(HaveOccurred())
			})

			It("hydrates the certificate field", func() {
				routeStore := models.RouteStore{
					Database: &transaction,
					Logger:   logger,
				}

				route, err := routeStore.FindOneMatching(models.Route{
					InstanceId: "complex-route",
				})

				Expect(err).ToNot(HaveOccurred())
				Expect(route.Certificates).To(HaveLen(1))
				Expect(route.Certificates[0].CertificateArn).To(Equal("ValidatingCertArn"))
			})
		})

		Describe("FindAllMatching", func() {
			BeforeEach(func() {
				cert1 := models.Certificate{
					Domain:            "foo.bar,blog.foo.bar",
					CertURL:           "cert.url.2",
					CertificateStatus: models.CertificateStatusValidating,
					CertificateArn:    "ValidatingCertArn.2",
				}

				cert2 := models.Certificate{
					Domain:            "example.com,blog.example.com",
					CertURL:           "cert.url.3",
					CertificateStatus: models.CertificateStatusValidating,
					CertificateArn:    "ValidatingCertArn.3",
				}

				routeOne := &models.Route{
					InstanceId:     "route-one",
					State:          "provisioned",
					ChallengeJSON:  []byte(`{}`),
					DomainExternal: "foo.bar",
					DomainInternal: "foo.london.cloudapps.digital",
					DistId:         "cloudfront-dist-one",
					Origin:         "foo.london.cloudapps.diigtal",
					Path:           "",
					InsecureOrigin: false,
					DefaultTTL:     0,
				}

				routeTwo := &models.Route{
					InstanceId:     "route-two",
					State:          "provisioning",
					ChallengeJSON:  []byte(`{}`),
					DomainExternal: "bar.bar",
					DomainInternal: "bar.london.cloudapps.digital",
					DistId:         "cloudfront-dist-two",
					Origin:         "bar.london.cloudapps.diigtal",
					Path:           "",
					InsecureOrigin: false,
					DefaultTTL:     0,
				}

				err := transaction.Save(routeOne).Error
				Expect(err).ToNot(HaveOccurred())

				err = transaction.Save(routeTwo).Error
				Expect(err).ToNot(HaveOccurred())

				cert1.RouteId = routeOne.ID
				err = transaction.Save(&cert1).Error
				Expect(err).ToNot(HaveOccurred())

				cert2.RouteId = routeTwo.ID
				err = transaction.Save(&cert2).Error
				Expect(err).ToNot(HaveOccurred())
			})

			It("finds all when the example struct is empty", func() {
				example := models.Route{}

				routeStore := models.RouteStore{
					Database: &transaction,
					Logger:   logger,
				}

				routes, err := routeStore.FindAllMatching(example)

				Expect(err).ToNot(HaveOccurred())
				Expect(routes).To(HaveLen(2))
			})

			It("filters based on the example struct", func() {
				example := models.Route{
					State: models.Provisioning,
				}

				routeStore := models.RouteStore{
					Database: &transaction,
					Logger:   logger,
				}

				routes, err := routeStore.FindAllMatching(example)

				Expect(err).ToNot(HaveOccurred())
				Expect(routes).To(HaveLen(1))
				Expect(routes[0].InstanceId).To(Equal("route-two"))
			})

			It("hydrates the certificate field", func() {
				example := models.Route{
					State: models.Provisioned,
				}

				routeStore := models.RouteStore{
					Database: &transaction,
					Logger:   logger,
				}

				routes, err := routeStore.FindAllMatching(example)

				Expect(err).ToNot(HaveOccurred())
				Expect(routes).To(HaveLen(1))
				Expect(routes[0].InstanceId).To(Equal("route-one"))
				Expect(routes[0].Certificates).To(HaveLen(1))
				Expect(routes[0].Certificates[0].CertURL).To(Equal("cert.url.2"))
			})
		})

		Describe("Create", func() {
			It("creates a new row from the input route", func() {
				var startCount int
				err := transaction.Model(models.Route{}).Count(&startCount).Error
				Expect(err).ToNot(HaveOccurred())
				Expect(startCount).To(Equal(0))

				newRoute := models.Route{
					InstanceId: "new-route",
					State:      models.Provisioning,
				}

				routeStore := models.RouteStore{
					Database: &transaction,
					Logger:   logger,
				}

				err = routeStore.Create(&newRoute)
				Expect(err).ToNot(HaveOccurred())

				var endCount int
				err = transaction.Model(models.Route{}).Count(&endCount).Error
				Expect(err).ToNot(HaveOccurred())
				Expect(endCount).To(Equal(1))
			})

			It("saves certificate details alongside the input route", func() {
				var startCount int
				err := transaction.Model(models.Certificate{}).Count(&startCount).Error
				Expect(err).ToNot(HaveOccurred())
				Expect(startCount).To(Equal(0))

				newRoute := models.Route{
					InstanceId: "new-route",
					State:      models.Provisioning,
					Certificates: []models.Certificate{
						models.Certificate{
							Domain:  "foo.bar",
							CertURL: "cert.url",
							Expires: time.Now().AddDate(0, 0, 90),
						},
					},
				}

				routeStore := models.RouteStore{
					Database: &transaction,
					Logger:   logger,
				}

				err = routeStore.Create(&newRoute)
				Expect(err).ToNot(HaveOccurred())

				var endCount int
				err = transaction.Model(models.Certificate{}).Count(&endCount).Error
				Expect(err).ToNot(HaveOccurred())
				Expect(endCount).To(Equal(1))
			})

			It("populates the primary key on the input route", func() {
				newRoute := models.Route{
					InstanceId: "new-route",
					State:      models.Provisioning,
				}

				Expect(newRoute.Model.ID).To(Equal(uint(0)))

				routeStore := models.RouteStore{
					Database: &transaction,
					Logger:   logger,
				}

				err := routeStore.Create(&newRoute)
				Expect(err).ToNot(HaveOccurred())

				Expect(newRoute.Model.ID).To(BeNumerically(">", uint(0)))
			})

			It("leaves the 'provisioning_since' value 'nil' if creating in 'Provisioned' state", func() {
				newRoute := models.Route{
					InstanceId: "new-route",
					State:      models.Provisioned,
				}

				Expect(newRoute.ProvisioningSince).To(BeNil())

				routeStore := models.RouteStore{
					Database: &transaction,
					Logger:   logger,
				}

				err := routeStore.Create(&newRoute)

				Expect(err).ToNot(HaveOccurred())
				Expect(newRoute.ProvisioningSince).To(BeNil())

			})

			It("Sets the 'provisioning_since' value to 'now()' if creating in 'Provisioning' state", func() {
				newRoute := models.Route{
					InstanceId: "new-route",
					State:      models.Provisioning,
				}

				Expect(newRoute.ProvisioningSince).To(BeNil())

				routeStore := models.RouteStore{
					Database: &transaction,
					Logger:   logger,
				}

				err := routeStore.Create(&newRoute)

				Expect(err).ToNot(HaveOccurred())
				Expect(newRoute.ProvisioningSince).NotTo(BeNil())
				Expect(*newRoute.ProvisioningSince).Should(BeTemporally("~", time.Now(), time.Second*2))
			})

		})

		//'Save' is like an 'Update' for a database.
		Describe("Save", func() {
			It("updates an existing record", func() {
				route := models.Route{
					InstanceId: "route-one",
					State:      models.Provisioning,
				}

				err := transaction.Create(&route).Error
				Expect(err).ToNot(HaveOccurred())

				routeStore := models.RouteStore{
					Database: &transaction,
					Logger:   logger,
				}

				route.State = "provisioned"
				err = routeStore.Save(&route)
				Expect(err).ToNot(HaveOccurred())

				var fetchedRoute models.Route
				err = transaction.First(&fetchedRoute, models.Route{InstanceId: "route-one"}).Error
				Expect(err).ToNot(HaveOccurred())

				Expect(fetchedRoute.InstanceId).To(Equal("route-one"))
				Expect(fetchedRoute.State).To(Equal(models.Provisioned))
			})

			It("Sets the 'provisioning_since' value to 'now()' if updating Route into 'Provisioning' state", func() {
				route := models.Route{
					InstanceId: "route-one",
					State:      models.Provisioned,
				}

				err := transaction.Create(&route).Error
				Expect(err).ToNot(HaveOccurred())

				routeStore := models.RouteStore{
					Database: &transaction,
					Logger:   logger,
				}

				route.State = models.Provisioning
				err = routeStore.Save(&route)
				Expect(err).ToNot(HaveOccurred())

				var fetchedRoute models.Route
				err = transaction.First(&fetchedRoute, models.Route{InstanceId: "route-one"}).Error
				Expect(err).ToNot(HaveOccurred())

				Expect(fetchedRoute.InstanceId).To(Equal("route-one"))
				Expect(fetchedRoute.State).To(Equal(models.Provisioning))
				Expect(fetchedRoute.ProvisioningSince).NotTo(BeNil())
				Expect(*fetchedRoute.ProvisioningSince).Should(BeTemporally("~", time.Now(), time.Second*2))
			})

			It("Sets the 'provisioning_since' value to 'NIL' if updating Route OUT OF 'Provisioning' state", func() {
				oneHourBefore := time.Now().Add(-1 * time.Hour)
				route := models.Route{
					InstanceId:        "route-one",
					State:             models.Provisioning,
					ProvisioningSince: &oneHourBefore,
				}

				err := transaction.Create(&route).Error
				Expect(err).ToNot(HaveOccurred())

				routeStore := models.RouteStore{
					Database: &transaction,
					Logger:   logger,
				}

				route.State = models.Provisioned
				err = routeStore.Save(&route)
				Expect(err).ToNot(HaveOccurred())

				var fetchedRoute models.Route
				err = transaction.First(&fetchedRoute, models.Route{InstanceId: "route-one"}).Error
				Expect(err).ToNot(HaveOccurred())

				Expect(fetchedRoute.InstanceId).To(Equal("route-one"))
				Expect(fetchedRoute.State).To(Equal(models.Provisioned))
				Expect(fetchedRoute.ProvisioningSince).To(BeNil())
			})
		})
	})

})
