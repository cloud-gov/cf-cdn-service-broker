package models_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"time"

	"github.com/18F/cf-cdn-service-broker/models"
	"github.com/18F/cf-cdn-service-broker/utils"
	"github.com/jinzhu/gorm"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("DataStore", func() {
	var (
		db          gorm.DB
		transaction gorm.DB
	)

	BeforeEach(func() {
		database, err := gorm.Open("postgres", os.Getenv("POSTGRES_URL"))

		Expect(err).ToNot(HaveOccurred())

		db = *database

		transaction = *db.Begin()
		err = models.Migrate(&db)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		transaction.Rollback()
	})

	Describe("RouteStore", func() {
		Describe("FindOneMatching", func() {
			BeforeEach(func() {
				leCert := models.Certificate{
					Domain:            "foo.bar",
					CertURL:           "cert.url",
					CertificateStatus: models.CertificateStatusLE,
					CertificateArn:    "managed-by-letsencrypt",
				}

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
					InstanceId:                "complex-route",
					State:                     "provisioned",
					ChallengeJSON:             []byte(`{}`),
					DomainExternal:            "foo.bar",
					DomainInternal:            "foo.london.cloudapps.digital",
					DistId:                    "cloudfront-dist",
					Origin:                    "foo.london.cloudapps.diigtal",
					Path:                      "",
					InsecureOrigin:            false,
					UserDataID:                1,
					DefaultTTL:                0,
					IsCertificateManagedByACM: false,
				}

				userData := &models.UserData{
					Email: "foo@bar.org",
					Reg: []byte(`
					{
						"Email": "the-mocky-cloud-paas-team@digital.cabinet-office.gov.uk",
						"Registration": null
					}
				`),
					Key: generateKey(),
				}

				err := transaction.Save(userData).Error
				Expect(err).ToNot(HaveOccurred())

				complexRoute.UserDataID = int(userData.ID)

				err = transaction.Save(complexRoute).Error
				Expect(err).ToNot(HaveOccurred())

				leCert.RouteId = complexRoute.ID
				err = transaction.Save(&leCert).Error
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
				}

				_, err := routeStore.FindOneMatching(models.Route{
					InstanceId: "no-match",
				})

				Expect(err).To(HaveOccurred())
			})

			It("hydrates the user field", func() {
				routeStore := models.RouteStore{
					Database: &transaction,
				}

				route, err := routeStore.FindOneMatching(models.Route{
					InstanceId: "complex-route",
				})

				Expect(err).ToNot(HaveOccurred())
				user := route.User
				Expect(user.Email).To(Equal("the-mocky-cloud-paas-team@digital.cabinet-office.gov.uk"))
			})

			It("hydrates the certificate field", func() {
				routeStore := models.RouteStore{
					Database: &transaction,
				}

				route, err := routeStore.FindOneMatching(models.Route{
					InstanceId: "complex-route",
				})

				Expect(err).ToNot(HaveOccurred())
				Expect(route.Certificates).To(HaveLen(2))
				Expect(route.Certificates[0].CertificateArn).To(Equal("managed-by-letsencrypt"))
				Expect(route.Certificates[1].CertificateArn).To(Equal("ValidatingCertArn"))
			})
		})

		Describe("FindAllMatching", func() {
			BeforeEach(func() {
				cert1 := models.Certificate{
					Domain:            "foo.bar",
					CertURL:           "cert.url.1",
					CertificateStatus: models.CertificateStatusLE,
					CertificateArn:    "managed-by-letsencrypt",
				}

				cert2 := models.Certificate{
					Domain:            "foo.bar,blog.foo.bar",
					CertURL:           "cert.url.2",
					CertificateStatus: models.CertificateStatusValidating,
					CertificateArn:    "ValidatingCertArn.2",
				}

				cert3 := models.Certificate{
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
					UserDataID:     1,
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
					UserDataID:     1,
					DefaultTTL:     0,
				}

				userData := &models.UserData{
					Email: "foo@bar.org",
					Reg: []byte(`
					{
						"Email": "the-mocky-cloud-paas-team@digital.cabinet-office.gov.uk",
						"Registration": null
					}
				`),
					Key: generateKey(),
				}

				err := transaction.Save(userData).Error
				Expect(err).ToNot(HaveOccurred())

				routeOne.UserDataID = int(userData.ID)
				routeTwo.UserDataID = int(userData.ID)

				err = transaction.Save(routeOne).Error
				Expect(err).ToNot(HaveOccurred())

				err = transaction.Save(routeTwo).Error
				Expect(err).ToNot(HaveOccurred())

				cert1.RouteId = routeOne.ID
				err = transaction.Save(&cert1).Error
				Expect(err).ToNot(HaveOccurred())

				cert2.RouteId = routeOne.ID
				err = transaction.Save(&cert2).Error
				Expect(err).ToNot(HaveOccurred())

				cert3.RouteId = routeTwo.ID
				err = transaction.Save(&cert3).Error
				Expect(err).ToNot(HaveOccurred())
			})

			It("finds all when the example struct is empty", func() {
				example := models.Route{}

				routeStore := models.RouteStore{
					Database: &transaction,
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
				}

				routes, err := routeStore.FindAllMatching(example)

				Expect(err).ToNot(HaveOccurred())
				Expect(routes).To(HaveLen(1))
				Expect(routes[0].InstanceId).To(Equal("route-two"))
			})

			It("hydrates the user field", func() {
				example := models.Route{
					State: models.Provisioning,
				}

				routeStore := models.RouteStore{
					Database: &transaction,
				}

				routes, err := routeStore.FindAllMatching(example)

				Expect(err).ToNot(HaveOccurred())
				Expect(routes).To(HaveLen(1))
				Expect(routes[0].InstanceId).To(Equal("route-two"))

				user := routes[0].User
				Expect(user.Email).To(Equal("the-mocky-cloud-paas-team@digital.cabinet-office.gov.uk"))
			})

			It("hydrates the certificate field", func() {
				example := models.Route{
					State: models.Provisioned,
				}

				routeStore := models.RouteStore{
					Database: &transaction,
				}

				routes, err := routeStore.FindAllMatching(example)

				Expect(err).ToNot(HaveOccurred())
				Expect(routes).To(HaveLen(1))
				Expect(routes[0].InstanceId).To(Equal("route-one"))
				Expect(routes[0].Certificates).To(HaveLen(2))
				Expect(routes[0].Certificates[0].CertURL).To(Equal("cert.url.1"))
				Expect(routes[0].Certificates[1].CertURL).To(Equal("cert.url.2"))
			})
		})

		Describe("FindWithExpiringCerts", func() {
			BeforeEach(func() {
				routeOne := &models.Route{
					InstanceId:     "expires-later",
					State:          "provisioned",
					ChallengeJSON:  []byte(`{}`),
					DomainExternal: "foo.bar",
					DomainInternal: "foo.london.cloudapps.digital",
					DistId:         "cloudfront-dist-one",
					Origin:         "foo.london.cloudapps.diigtal",
					Path:           "",
					InsecureOrigin: false,
					UserDataID:     1,
					DefaultTTL:     0,
					Certificate: models.Certificate{
						Domain:            "foo.bar",
						CertURL:           "cert.url.expires-later",
						Certificate:       []byte{},
						Expires:           time.Now().AddDate(0, 0, 90),
						CertificateStatus: models.CertificateStatusLE,
					},
				}

				routeTwo := &models.Route{
					InstanceId:     "expires-sooner",
					State:          "provisioned",
					ChallengeJSON:  []byte(`{}`),
					DomainExternal: "bar.bar",
					DomainInternal: "bar.london.cloudapps.digital",
					DistId:         "cloudfront-dist-two",
					Origin:         "bar.london.cloudapps.diigtal",
					Path:           "",
					InsecureOrigin: false,
					UserDataID:     1,
					DefaultTTL:     0,
					Certificate: models.Certificate{
						Domain:            "bar.bar",
						CertURL:           "cert.url.expires-sooner",
						Certificate:       []byte{},
						Expires:           time.Now().AddDate(0, 0, 10),
						CertificateStatus: models.CertificateStatusLE,
					},
				}

				routeThree := &models.Route{
					InstanceId:     "expires-sooner-but-not-provisoned",
					State:          "provisioning",
					ChallengeJSON:  []byte(`{}`),
					DomainExternal: "bar.bar",
					DomainInternal: "bar.london.cloudapps.digital",
					DistId:         "cloudfront-dist-two",
					Origin:         "bar.london.cloudapps.diigtal",
					Path:           "",
					InsecureOrigin: false,
					UserDataID:     1,
					DefaultTTL:     0,
					Certificate: models.Certificate{
						Domain:            "bar.bar",
						CertURL:           "cert.url.expires-sooner-but-not-provisoned",
						Certificate:       []byte{},
						Expires:           time.Now().AddDate(0, 0, 10),
						CertificateStatus: models.CertificateStatusLE,
					},
				}

				routeFour := &models.Route{
					InstanceId:     "expires-sooner-ACM-managed",
					State:          "provisioned",
					ChallengeJSON:  []byte(`{}`),
					DomainExternal: "bar.bar",
					DomainInternal: "bar.london.cloudapps.digital",
					DistId:         "cloudfront-dist-two",
					Origin:         "bar.london.cloudapps.diigtal",
					Path:           "",
					InsecureOrigin: false,
					UserDataID:     1,
					DefaultTTL:     0,
					Certificates: []models.Certificate{
						{
							Domain:            "bar.bar",
							CertURL:           "cert.url.expires-sooner-ACM-managed",
							Expires:           time.Now().AddDate(0, 0, 10),
							CertificateArn:    "CertificateArn",
							CertificateStatus: models.CertificateStatusAttached,
						},
					},
					IsCertificateManagedByACM: true,
				}

				userData := &models.UserData{
					Email: "foo@bar.org",
					Reg: []byte(`
					{
						"Email": "the-mocky-cloud-paas-team@digital.cabinet-office.gov.uk",
						"Registration": null
					}
				`),
					Key: generateKey(),
				}

				err := transaction.Save(userData).Error
				Expect(err).ToNot(HaveOccurred())

				routeOne.UserDataID = int(userData.ID)
				routeTwo.UserDataID = int(userData.ID)
				routeThree.UserDataID = int(userData.ID)
				routeFour.UserDataID = int(userData.ID)

				err = transaction.Save(&routeOne.Certificate).Error
				Expect(err).ToNot(HaveOccurred())

				err = transaction.Save(&routeTwo.Certificate).Error
				Expect(err).ToNot(HaveOccurred())

				err = transaction.Save(&routeThree.Certificate).Error
				Expect(err).ToNot(HaveOccurred())

				Expect(len(routeFour.Certificates)).To(Equal(1))
				err = transaction.Save(&routeFour.Certificates[0]).Error
				Expect(err).ToNot(HaveOccurred())

				err = transaction.Save(routeOne).Error
				Expect(err).ToNot(HaveOccurred())

				err = transaction.Save(routeTwo).Error
				Expect(err).ToNot(HaveOccurred())

				err = transaction.Save(routeThree).Error
				Expect(err).ToNot(HaveOccurred())

				err = transaction.Save(routeFour).Error
				Expect(err).ToNot(HaveOccurred())
			})

			It("finds certificates with less than 30 days until their expiry date and they are in the 'provisioned' state and managed by LetsEncrypt", func() {
				routeStore := models.RouteStore{
					Database: &transaction,
				}

				routes, err := routeStore.FindWithExpiringCerts()

				Expect(err).ToNot(HaveOccurred())
				Expect(routes).To(HaveLen(1))
				Expect(routes[0].InstanceId).To(Equal("expires-sooner"))

				Expect(string(routes[0].State)).To(Equal("provisioned"))
				Expect(routes[0].IsCertificateManagedByACM).To(Equal(false), "The certificate is managed by ACM")
			})

			It("hydrates the user field", func() {
				routeStore := models.RouteStore{
					Database: &transaction,
				}

				routes, err := routeStore.FindWithExpiringCerts()

				Expect(err).ToNot(HaveOccurred())
				Expect(routes).To(HaveLen(1))
				Expect(routes[0].InstanceId).To(Equal("expires-sooner"))

				user := routes[0].User
				Expect(user.Email).To(Equal("the-mocky-cloud-paas-team@digital.cabinet-office.gov.uk"))
			})

			It("hydrates the certificate field", func() {

				routeStore := models.RouteStore{
					Database: &transaction,
				}

				routes, err := routeStore.FindWithExpiringCerts()

				Expect(err).ToNot(HaveOccurred())
				Expect(routes).To(HaveLen(1))
				Expect(routes[0].InstanceId).To(Equal("expires-sooner"))
				certificates := routes[0].Certificates
				Expect(certificates).To(HaveLen(1))
				Expect(certificates[0].CertURL).To(Equal("cert.url.expires-sooner"))
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
					UserData: models.UserData{
						Email: "foo@bar.org",
						Reg: []byte(`
							{
								"Email": "the-mocky-cloud-paas-team@digital.cabinet-office.gov.uk",
								"Registration": null
							}
						`),
						Key: generateKey(),
					},
				}

				routeStore := models.RouteStore{
					Database: &transaction,
				}

				err = routeStore.Create(&newRoute)
				Expect(err).ToNot(HaveOccurred())

				var endCount int
				err = transaction.Model(models.Route{}).Count(&endCount).Error
				Expect(err).ToNot(HaveOccurred())
				Expect(endCount).To(Equal(1))
			})

			It("saves any user data alongside the input route", func() {
				var startCount int
				err := transaction.Model(models.UserData{}).Count(&startCount).Error
				Expect(err).ToNot(HaveOccurred())
				Expect(startCount).To(Equal(0))

				newRoute := models.Route{
					InstanceId: "new-route",
					State:      models.Provisioning,
					UserData: models.UserData{
						Email: "foo@bar.org",
						Reg: []byte(`
							{
								"Email": "the-mocky-cloud-paas-team@digital.cabinet-office.gov.uk",
								"Registration": null
							}
						`),
						Key: generateKey(),
					},
				}

				routeStore := models.RouteStore{
					Database: &transaction,
				}

				err = routeStore.Create(&newRoute)
				Expect(err).ToNot(HaveOccurred())

				var endCount int
				err = transaction.Model(models.UserData{}).Count(&endCount).Error
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
						models.Certificate {
							Domain:      "foo.bar",
							CertURL:     "cert.url",
							Certificate: nil,
							Expires:     time.Now().AddDate(0, 0, 90),
						},
					},
				}

				routeStore := models.RouteStore{
					Database: &transaction,
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
					UserData: models.UserData{
						Email: "foo@bar.org",
						Reg: []byte(`
							{
								"Email": "the-mocky-cloud-paas-team@digital.cabinet-office.gov.uk",
								"Registration": null
							}
						`),
						Key: generateKey(),
					},
				}

				Expect(newRoute.Model.ID).To(Equal(uint(0)))

				routeStore := models.RouteStore{
					Database: &transaction,
				}

				err := routeStore.Create(&newRoute)
				Expect(err).ToNot(HaveOccurred())

				Expect(newRoute.Model.ID).To(BeNumerically(">", uint(0)))
			})

			It("populates the UserDataID on the input route", func() {
				newRoute := models.Route{
					InstanceId: "new-route",
					State:      models.Provisioning,
					UserData: models.UserData{
						Email: "foo@bar.org",
						Reg: []byte(`
							{
								"Email": "the-mocky-cloud-paas-team@digital.cabinet-office.gov.uk",
								"Registration": null
							}
						`),
						Key: generateKey(),
					},
				}

				Expect(newRoute.UserDataID).To(Equal(0))

				routeStore := models.RouteStore{
					Database: &transaction,
				}

				err := routeStore.Create(&newRoute)
				Expect(err).ToNot(HaveOccurred())

				Expect(newRoute.UserDataID).To(BeNumerically(">", 0))
			})

			It("hydrates the user property", func() {
				newRoute := models.Route{
					InstanceId: "new-route",
					State:      models.Provisioning,
					UserData: models.UserData{
						Email: "foo@bar.org",
						Reg: []byte(`
							{
								"Email": "the-mocky-cloud-paas-team@digital.cabinet-office.gov.uk",
								"Registration": null
							}
						`),
						Key: generateKey(),
					},
				}

				//The newRoute.User struct will be empty on init
				Expect(newRoute.User).To(Equal(utils.User{}))

				routeStore := models.RouteStore{
					Database: &transaction,
				}

				//The newRoute.User will be initialised with values from 'UserData' after calling 'routeStore.Create'
				err := routeStore.Create(&newRoute)
				Expect(err).ToNot(HaveOccurred())
				Expect(newRoute.User.Email).To(Equal("the-mocky-cloud-paas-team@digital.cabinet-office.gov.uk"))
			})

			It("leaves the 'provisioning_since' value 'nil' if creating in 'Provisioned' state", func() {
				newRoute := models.Route{
					InstanceId: "new-route",
					State:      models.Provisioned,
					UserData: models.UserData{
						Email: "foo@bar.org",
						Reg: []byte(`
							{
								"Email": "the-mocky-cloud-paas-team@digital.cabinet-office.gov.uk",
								"Registration": null
							}
						`),
						Key: generateKey(),
					},
				}

				Expect(newRoute.ProvisioningSince).To(BeNil())

				routeStore := models.RouteStore{
					Database: &transaction,
				}

				err := routeStore.Create(&newRoute)

				Expect(err).ToNot(HaveOccurred())
				Expect(newRoute.ProvisioningSince).To(BeNil())

			})

			It("Sets the 'provisioning_since' value to 'now()' if creating in 'Provisioning' state", func() {
				newRoute := models.Route{
					InstanceId: "new-route",
					State:      models.Provisioning,
					UserData: models.UserData{
						Email: "foo@bar.org",
						Reg: []byte(`
							{
								"Email": "the-mocky-cloud-paas-team@digital.cabinet-office.gov.uk",
								"Registration": null
							}
						`),
						Key: generateKey(),
					},
				}

				Expect(newRoute.ProvisioningSince).To(BeNil())

				routeStore := models.RouteStore{
					Database: &transaction,
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

			It("updates user data", func() {
				userData := models.UserData{
					Email: "foo@bar.org",
					Reg:   nil,
					Key:   nil,
				}
				route := models.Route{
					InstanceId: "route-one",
					State:      models.Provisioning,
					UserData:   userData,
				}

				err := transaction.Create(&route).Error
				Expect(err).ToNot(HaveOccurred())

				routeStore := models.RouteStore{
					Database: &transaction,
				}

				route.UserData.Email = "bar@baz.org"
				err = routeStore.Save(&route)
				Expect(err).ToNot(HaveOccurred())

				var fetchedUserData models.UserData
				err = transaction.First(
					&fetchedUserData,
					models.UserData{
						Model: gorm.Model{ID: uint(route.UserDataID)},
					},
				).Error
				Expect(err).ToNot(HaveOccurred())

				Expect(fetchedUserData.Model.ID).To(Equal(uint(route.UserDataID)))
				Expect(fetchedUserData.Email).To(Equal("bar@baz.org"))
			})

			It("updates certificate details", func() {
				certificate := models.Certificate{
					Domain:      "foo.bar",
					CertURL:     "cert.url",
					Certificate: nil,
					Expires:     time.Time{},
				}
				route := models.Route{
					InstanceId:  "route-one",
					State:       "provisioning",
					Certificate: certificate,
				}

				err := transaction.Create(&route).Error
				Expect(err).ToNot(HaveOccurred())

				routeStore := models.RouteStore{
					Database: &transaction,
				}

				route.Certificate.CertURL = "new.cert.url"
				err = routeStore.Save(&route)
				Expect(err).ToNot(HaveOccurred())

				var fetchedCertDetails models.Certificate
				err = transaction.First(
					&fetchedCertDetails,
					models.Certificate{
						RouteId: route.ID,
					},
				).Error
				Expect(err).ToNot(HaveOccurred())

				Expect(fetchedCertDetails.CertURL).To(Equal("new.cert.url"))
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

func generateKey() []byte {
	rsaTestKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred(), "Generating test key")
	pemBytes := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(rsaTestKey),
		},
	)

	return pemBytes
}
