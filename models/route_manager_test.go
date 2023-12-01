package models_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"

	"code.cloudfoundry.org/lager/v3"
	"github.com/jinzhu/gorm"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/aws/aws-sdk-go/service/cloudfront"

	"github.com/alphagov/paas-cdn-broker/config"
	"github.com/alphagov/paas-cdn-broker/models"
	"github.com/alphagov/paas-cdn-broker/models/mocks"
	modelsmocks "github.com/alphagov/paas-cdn-broker/models/mocks"
	"github.com/alphagov/paas-cdn-broker/utils"
	utilsmocks "github.com/alphagov/paas-cdn-broker/utils/mocks"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RouteManager", func() {

	Context("DeleteOrphanedCerts", func() {
		var (
			manager models.RouteManager

			fakeDistribution *utilsmocks.FakeDistribution
			fakeCertsManager *utilsmocks.FakeCertificateManager
			fakeDatastore    *mocks.FakeRouteStore

			errorLogOutput *bytes.Buffer

			logger   lager.Logger
			settings config.Settings
		)

		BeforeEach(func() {
			settings = config.Settings{}

			errorLogOutput = bytes.NewBuffer([]byte{})

			logger = lager.NewLogger("delete-orphaned-acm-certs")
			logger.RegisterSink(lager.NewWriterSink(GinkgoWriter, lager.INFO))
			logger.RegisterSink(lager.NewWriterSink(errorLogOutput, lager.ERROR))

			fakeCertsManager = &utilsmocks.FakeCertificateManager{}
			fakeDatastore = &mocks.FakeRouteStore{}
			fakeDistribution = &utilsmocks.FakeDistribution{}
		})

		JustBeforeEach(func() {
			manager = models.NewManager(
				logger,
				fakeDistribution,
				settings,
				fakeDatastore,
				fakeCertsManager,
			)
		})

		Context("deleting ACM certs", func() {
			BeforeEach(func() {
				fakeDistribution.ListDistributionsReturns(nil)
			})

			It("only deletes certificates in the ISSUED state, which are NOT in use, are managed by a CDN broker, and are more than 24h old", func() {
				time48hAgo := time.Now().Add(-48 * time.Hour)
				time2hAgo := time.Now().Add(-2 * time.Hour)

				managedByCdnBrokerTag := acm.Tag{
					Key:   aws.String(utils.ManagedByTagName),
					Value: aws.String(utils.ManagedByTagValue),
				}

				certificateDetails := []utils.CertificateDetails{
					utils.CertificateDetails{
						CertificateArn: aws.String("cert-arn-1"),
						Status:         aws.String(acm.CertificateStatusIssued),
						InUseBy:        []*string{},
						IssuedAt:       &time48hAgo,
						Tags:           []*acm.Tag{&managedByCdnBrokerTag},
					},

					utils.CertificateDetails{
						CertificateArn: aws.String("cert-arn-2"),
						Status:         aws.String(acm.CertificateStatusPendingValidation),
						InUseBy:        []*string{},
						IssuedAt:       &time48hAgo,
						Tags:           []*acm.Tag{&managedByCdnBrokerTag},
					},

					utils.CertificateDetails{
						CertificateArn: aws.String("cert-arn-3"),
						Status:         aws.String(acm.CertificateStatusIssued),
						InUseBy:        []*string{aws.String("arn:aws:::something")},
						IssuedAt:       &time48hAgo,
						Tags:           []*acm.Tag{&managedByCdnBrokerTag},
					},

					utils.CertificateDetails{
						CertificateArn: aws.String("cert-arn-4"),
						Status:         aws.String(acm.CertificateStatusIssued),
						InUseBy:        []*string{},
						IssuedAt:       &time2hAgo,
						Tags:           []*acm.Tag{&managedByCdnBrokerTag},
					},

					utils.CertificateDetails{
						CertificateArn: aws.String("cert-arn-5"),
						Status:         aws.String(acm.CertificateStatusIssued),
						InUseBy:        []*string{},
						IssuedAt:       &time48hAgo,
						Tags:           []*acm.Tag{},
					},
				}

				fakeCertsManager.ListIssuedCertificatesReturns(certificateDetails, nil)

				manager.DeleteOrphanedCerts()

				Expect(fakeCertsManager.ListIssuedCertificatesCallCount()).To(Equal(1))
				Expect(fakeCertsManager.DeleteCertificateCallCount()).To(Equal(1))

				deletedCertArn := fakeCertsManager.DeleteCertificateArgsForCall(0)
				Expect(deletedCertArn).To(Equal(*certificateDetails[0].CertificateArn))
			})
		})
	})

	Context("Create", func() {
		var fakeDistribution *utilsmocks.FakeDistribution
		var fakeCertsManager *utilsmocks.FakeCertificateManager
		var manager models.RouteManager
		var cfDist *cloudfront.Distribution
		var fakeDatastore *modelsmocks.FakeRouteStore
		var logger lager.Logger
		var settings config.Settings

		BeforeEach(func() {
			settings = config.Settings{}

			logger = lager.NewLogger("test")

			logger.RegisterSink(lager.NewWriterSink(GinkgoWriter, lager.INFO))

			fakeDistribution = &utilsmocks.FakeDistribution{}

			fakeCertsManager = &utilsmocks.FakeCertificateManager{}

			fakeDatastore = &modelsmocks.FakeRouteStore{}

			manager = models.NewManager(
				logger,
				fakeDistribution,
				config.Settings{},
				fakeDatastore,
				fakeCertsManager,
			)

			cfDist = &cloudfront.Distribution{
				Id:                 aws.String("cloudfront-distribution-id"),
				DomainName:         aws.String("domainname.cloudfront.net"),
				Status:             aws.String("InProgress"),
				DistributionConfig: &cloudfront.DistributionConfig{},
			}

			cfDist.DistributionConfig.Enabled = aws.Bool(true)
			cfDist.DistributionConfig.Aliases = &cloudfront.Aliases{Items: []*string{aws.String("example.com"), aws.String("info.example.com")}}

			fakeDistribution.CreateReturns(cfDist, nil)

			fakeCertsManager.RequestCertificateReturns(aws.String("FirstCertArn"), nil)

		})

		JustBeforeEach(func() {
			manager = models.NewManager(
				logger,
				fakeDistribution,
				settings,
				fakeDatastore,
				fakeCertsManager,
			)
		})

		It("when creating a new route, the function creates a cloudfront distribution and persisting the newly created route to the database", func() {

			_, err := manager.Create("RouteInstanceID", "example.com,info.example.com", "origin.domain.com", 3600, utils.Headers{}, false, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			routeArg := fakeDatastore.CreateArgsForCall(0)

			Expect(fakeDistribution.CreateCallCount()).To(Equal(1), "distribution.Creat() function should have been called once")
			Expect(fakeDatastore.CreateCallCount()).To(Equal(1), "datastore.Create() function should have been called once")
			Expect(routeArg.DomainInternal).To(Equal(*cfDist.DomainName))
			Expect(routeArg.DistId).To(Equal(*cfDist.Id))

		})

		It("when creating a new route, the function requests a certificate from ACM and persisting the newly created route to the database", func() {

			_, err := manager.Create("RouteInstanceID", "example.com,info.example.com", "origin.domain.com", 3600, utils.Headers{}, false, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			routeArg := fakeDatastore.CreateArgsForCall(0)
			requestedDomains, cloudfoundryInstanceID := fakeCertsManager.RequestCertificateArgsForCall(0)

			Expect(fakeCertsManager.RequestCertificateCallCount()).To(Equal(1), "CertificateManager.RequestCertificate() function should have been called once")
			Expect(fakeDatastore.CreateCallCount()).To(Equal(1), "datastore.Create() function should have been called once")
			Expect(routeArg.Certificates[0].CertificateArn).To(Equal("FirstCertArn"))
			Expect(routeArg.Certificates[0].CertificateStatus).To(Equal(models.CertificateStatusValidating))
			Expect(requestedDomains).To(Equal(strings.Split(routeArg.DomainExternal, ",")))
			Expect(cloudfoundryInstanceID).To(Equal(routeArg.InstanceId))
		})

		It("when creating a new route, the function sets the relevant flags on the route struct to the correct values and persisting the newly created route to the database", func() {

			_, err := manager.Create("RouteInstanceID", "example.com,info.example.com", "origin.domain.com", 3600, utils.Headers{}, false, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			routeArg := fakeDatastore.CreateArgsForCall(0)

			Expect(fakeDistribution.CreateCallCount()).To(Equal(1), "distribution.Creat() function should have been called once")
			Expect(fakeDatastore.CreateCallCount()).To(Equal(1), "datastore.Create() function should have been called once")
			Expect(routeArg.State).To(Equal(models.Provisioning))
		})

	})

	Context("Poll", func() {
		var fakeDistribution *utilsmocks.FakeDistribution
		var fakeCertsManager *utilsmocks.FakeCertificateManager
		var manager models.RouteManager
		var route *models.Route
		var cfDist *cloudfront.Distribution
		var fakeDatastore *modelsmocks.FakeRouteStore
		var settings config.Settings
		var logger lager.Logger

		BeforeEach(func() {
			settings = config.Settings{}
			logger = lager.NewLogger("test")

			logger.RegisterSink(lager.NewWriterSink(GinkgoWriter, lager.INFO))

			fakeDistribution = &utilsmocks.FakeDistribution{}

			fakeCertsManager = &utilsmocks.FakeCertificateManager{}

			fakeDatastore = &modelsmocks.FakeRouteStore{}

			cfDist = &cloudfront.Distribution{
				Status:             aws.String("Deployed"),
				DistributionConfig: &cloudfront.DistributionConfig{},
			}

			cfDist.DistributionConfig.Enabled = aws.Bool(true)
			cfDist.DistributionConfig.Aliases = &cloudfront.Aliases{Items: []*string{aws.String("example.com"), aws.String("info.example.com")}}

			fakeDistribution.GetReturns(cfDist, nil)

		})

		JustBeforeEach(func() {
			manager = models.NewManager(
				logger,
				fakeDistribution,
				settings,
				fakeDatastore,
				fakeCertsManager,
			)
		})

		Context("When the route is provisioning", func() {
			BeforeEach(func() {
				route = &models.Route{
					InstanceId:     "RouteInstanceID",
					DistId:         "DistID",
					State:          models.Provisioning,
					DomainExternal: "example.com,info.example.com",
					Certificates: []models.Certificate{
						{
							Model: gorm.Model{
								CreatedAt: time.Now().Add(-60 * 24 * time.Hour),
							},
							CertificateArn:    "FirstCertArn",
							CertificateStatus: models.CertificateStatusAttached,
						},
						{
							Model: gorm.Model{
								CreatedAt: time.Now().Add(-60 * time.Second),
							},
							CertificateArn:    "SecondCertArn",
							CertificateStatus: models.CertificateStatusValidating,
						},
						{
							Model: gorm.Model{
								CreatedAt: time.Now().Add(-30 * time.Second),
							},
							CertificateArn:    "ThirdCertArn",
							CertificateStatus: models.CertificateStatusValidating,
						},
					},
				}
			})

			It("updateProvisioning returns 'nil' when distribution is NOT deployed", func() {

				cfDist.Status = aws.String("InProgress")

				err := manager.Poll(route)

				Expect(err).ToNot(HaveOccurred())
				Expect(fakeDistribution.GetCallCount()).To(BeNumerically(">", 0), "distribution Get function was called 0 times")
				Expect(fakeDistribution.SetCertificateAndCnameCallCount()).To(Equal(0), "SetCertificateAndCname function was called")
				Expect(fakeCertsManager.IsCertificateIssuedCallCount()).To(Equal(0), "IsCertificateIssued function was called")
			})

			It("updateProvisioning returns 'nil' when certificate is NOT issued", func() {
				cfDist.Status = aws.String("Deployed")

				fakeCertsManager.IsCertificateIssuedReturns(false, nil)

				err := manager.Poll(route)

				requestedCertificateArn := fakeCertsManager.IsCertificateIssuedArgsForCall(0)

				Expect(err).ToNot(HaveOccurred())
				Expect(fakeDistribution.GetCallCount()).To(BeNumerically(">", 0), "distribution Get function was called 0 times")
				Expect(fakeCertsManager.IsCertificateIssuedCallCount()).To(Equal(1), "IsCertificateIssued function was NOT called")
				Expect(fakeDistribution.SetCertificateAndCnameCallCount()).To(Equal(0), "SetCertificateAndCname function was called")
				Expect(requestedCertificateArn).To(Equal(route.Certificates[2].CertificateArn))
			})

			It("updateProvisioning returns error when cloud front distribution has returned 'nil'", func() {
				cfDist.Status = aws.String("InProgress")

				fakeDistribution.GetReturns(nil, errors.New("Just an error"))

				err := manager.Poll(route)

				Expect(err).ToNot(HaveOccurred())
				Expect(fakeDistribution.GetCallCount()).To(BeNumerically(">", 0), "distribution Get function was called 0 times")
				Expect(fakeDistribution.SetCertificateAndCnameCallCount()).To(Equal(0), "SetCertificateAndCname function was called")
				Expect(fakeCertsManager.IsCertificateIssuedCallCount()).To(Equal(0), "IsCertificateIssued function was called")
			})

			It("updateProvisioning sets the certificate status to 'Fail' when certificate validation has timed out", func() {
				Expect(route.Certificates[2].CertificateStatus).To(Equal(models.CertificateStatusValidating))

				fakeCertsManager.IsCertificateIssuedReturns(false, utils.ErrValidationTimedOut)

				err := manager.Poll(route)

				Expect(err).To(Equal(utils.ErrValidationTimedOut))
				Expect(route.Certificates[2].CertificateStatus).To(Equal(models.CertificateStatusFailed))

			})

			Context("The certificate was issued by ACM and CloudFront Distribution was deployed", func() {
				BeforeEach(func() {

					cfDist.Status = aws.String("Deployed")

					fakeCertsManager.IsCertificateIssuedReturns(true, nil)

				})

				It("making sure that the correct certificate is set to the CloudFront distribution", func() {
					err := manager.Poll(route)

					_, certArn, domains := fakeDistribution.SetCertificateAndCnameArgsForCall(0)

					Expect(err).ToNot(HaveOccurred())
					Expect(certArn).To(Equal(route.Certificates[2].CertificateArn))
					Expect(domains).To(Equal(route.GetDomains()))

				})

				It("making sure that we marked the certificates status as 'Attached' and 'Detached' accordingly, in the database", func() {
					Expect(route.Certificates[2].CertificateStatus).To(Equal(models.CertificateStatusValidating))
					Expect(route.Certificates[0].CertificateStatus).To(Equal(models.CertificateStatusAttached))

					err := manager.Poll(route)

					routeArg := fakeDatastore.SaveArgsForCall(0)
					Expect(err).ToNot(HaveOccurred())

					Expect(routeArg.Certificates[2].CertificateStatus).To(Equal(models.CertificateStatusAttached))
					Expect(routeArg.Certificates[0].CertificateStatus).To(Equal(models.CertificateStatusDeleted))
				})
			})

		})

		Context("When the route is deprovisioning", func() {
			BeforeEach(func() {
				route = &models.Route{
					InstanceId:     "RouteInstanceID",
					DistId:         "DistID",
					State:          models.Deprovisioning,
					DomainExternal: "example.com,info.example.com",
					Certificates: []models.Certificate{
						{
							Model: gorm.Model{
								CreatedAt: time.Now().Add(-60 * 24 * time.Hour),
							},
							CertificateArn:    "FirstCertArn",
							CertificateStatus: models.CertificateStatusAttached,
						},
					},
				}
			})

			It("when cloud front distribution is successfully deleted, the route status is set to Deprovisioned", func() {

				fakeDistribution.DeleteReturns(true, nil)

				err := manager.Poll(route)

				Expect(err).ToNot(HaveOccurred())
				Expect(fakeDistribution.DeleteCallCount()).To(Equal(1), "Delete function was NOT called")
				Expect(fakeDatastore.SaveCallCount()).To(Equal(1))
				Expect(fakeDatastore.SaveArgsForCall(0).State).To(Equal(models.Deprovisioned))

			})

			It("when cloud front distribution is not deleted yet, the route status is not being changed", func() {
				fakeDistribution.DeleteReturns(false, nil)

				err := manager.Poll(route)

				Expect(err).ToNot(HaveOccurred())
				Expect(fakeDistribution.DeleteCallCount()).To(Equal(1), "Delete function was NOT called")
				Expect(fakeDatastore.SaveCallCount()).To(Equal(0), "the route object WAS persisted")
				Expect(route.State).To(Equal(models.Deprovisioning), "the route.State is NOT deprovisioning")
			})

			It("when cloud front distribution delete operation has failed, the route status is not changed", func() {
				fakeDistribution.DeleteReturns(false, errors.New("couldn't delete distribution"))

				err := manager.Poll(route)

				Expect(err).To(HaveOccurred())
				Expect(fakeDistribution.DeleteCallCount()).To(Equal(1), "Delete function was NOT called")
				Expect(fakeDatastore.SaveCallCount()).To(Equal(0), "the route object WAS persisted")
				Expect(route.State).To(Equal(models.Deprovisioning), "the route.State is NOT deprovisioning")
			})
		})
	})

	Context("GetCurrentlyDeployedDomains", func() {
		var fakeDistribution *utilsmocks.FakeDistribution
		var fakeCertsManager *utilsmocks.FakeCertificateManager
		var manager models.RouteManager
		var route *models.Route
		var cfDist *cloudfront.Distribution
		var fakeDatastore *modelsmocks.FakeRouteStore
		var logger lager.Logger
		var settings config.Settings

		BeforeEach(func() {
			settings = config.Settings{}

			logger = lager.NewLogger("test")

			logger.RegisterSink(lager.NewWriterSink(GinkgoWriter, lager.INFO))

			fakeDistribution = &utilsmocks.FakeDistribution{}

			fakeCertsManager = &utilsmocks.FakeCertificateManager{}

			fakeDatastore = &modelsmocks.FakeRouteStore{}

			cfDist = &cloudfront.Distribution{
				Status:             aws.String("Deployed"),
				DistributionConfig: &cloudfront.DistributionConfig{},
			}

			cfDist.DistributionConfig.Enabled = aws.Bool(true)
			cfDist.DistributionConfig.Aliases = &cloudfront.Aliases{Items: []*string{aws.String("example.com"), aws.String("info.example.com")}}

			fakeDistribution.GetReturns(cfDist, nil)

			route = &models.Route{
				InstanceId:     "RouteInstanceID",
				DistId:         "DistID",
				State:          models.Deprovisioning,
				DomainExternal: "example.com,info.example.com",
				Certificates: []models.Certificate{
					{
						Model: gorm.Model{
							CreatedAt: time.Now().Add(-60 * 24 * time.Hour),
						},
						CertificateArn:    "FirstCertArn",
						CertificateStatus: models.CertificateStatusAttached,
					},
				},
			}
		})

		JustBeforeEach(func() {
			manager = models.NewManager(
				logger,
				fakeDistribution,
				settings,
				fakeDatastore,
				fakeCertsManager,
			)
		})

		It("returns deployed domains list, when cloudfront Get function returns a valid distribution", func() {
			fakeDistribution.GetReturns(cfDist, nil)

			deployedDomains, err := manager.GetCurrentlyDeployedDomains(route)

			Expect(err).ToNot(HaveOccurred())
			Expect(fakeDistribution.GetCallCount()).To(Equal(1), "Get function was NOT called")
			Expect(deployedDomains).To(Equal([]string{"example.com", "info.example.com"}))

		})

		It("returns an empty slice of strings, when cloudfront Get function returns an error", func() {
			fakeDistribution.GetReturns(nil, errors.New("can't get the distribution"))

			_, err := manager.GetCurrentlyDeployedDomains(route)

			Expect(err).To(HaveOccurred())
			Expect(fakeDistribution.GetCallCount()).To(Equal(1), "Get function was NOT called")
		})
	})

	Context("Update", func() {
		var (
			fakeDistribution *utilsmocks.FakeDistribution
			fakeCertsManager *utilsmocks.FakeCertificateManager
			manager          models.RouteManager
			route            *models.Route
			cfDist           *cloudfront.Distribution
			fakeDatastore    *modelsmocks.FakeRouteStore
			logger           lager.Logger
			settings         config.Settings
			defaultTTL       = int64(0)
			forwardedHeaders = utils.Headers{"X-Forwarded-Five": true}
			forwardCookies   = false
			cloudfrontDistID = "cloudfoundry-instance-id"
		)

		BeforeEach(func() {
			settings = config.Settings{}

			logger = lager.NewLogger("test")

			logger.RegisterSink(lager.NewWriterSink(GinkgoWriter, lager.INFO))

			fakeDistribution = &utilsmocks.FakeDistribution{}

			fakeCertsManager = &utilsmocks.FakeCertificateManager{}

			fakeDatastore = &modelsmocks.FakeRouteStore{}

			route = &models.Route{
				InstanceId:     "RouteInstanceID",
				DistId:         "DistID",
				DomainExternal: "example.com,info.example.com",
				Certificates: []models.Certificate{
					{
						Model: gorm.Model{
							CreatedAt: time.Now().Add(-60 * 24 * time.Hour),
						},
						CertificateArn:    "FirstCertArn",
						CertificateStatus: models.CertificateStatusAttached,
					},
					{
						Model: gorm.Model{
							CreatedAt: time.Now().Add(-60 * time.Second),
						},
						CertificateArn:    "SecondCertArn",
						CertificateStatus: models.CertificateStatusDeleted,
					},
				},
			}

			route.State = models.Provisioned

			fakeDatastore.FindOneMatchingReturns(*route, nil)

			cfDist = &cloudfront.Distribution{
				Id:                 aws.String("cfDistributionID"),
				DomainName:         aws.String("blahblah.cloudfront.net"),
				Status:             aws.String("Deployed"),
				DistributionConfig: &cloudfront.DistributionConfig{},
			}

			cfDist.DistributionConfig.Enabled = aws.Bool(true)
			cfDist.DistributionConfig.Aliases = &cloudfront.Aliases{Items: []*string{aws.String("example.com"), aws.String("info.example.com")}}

			fakeDistribution.GetReturns(cfDist, nil)

		})

		JustBeforeEach(func() {
			manager = models.NewManager(
				logger,
				fakeDistribution,
				settings,
				fakeDatastore,
				fakeCertsManager,
			)
		})

		It("should not request a new certificate when domains have not been passed in 'cf service-update' call ", func() {

			fakeDistribution.UpdateReturns(cfDist, nil)

			_, err := manager.Update(
				cloudfrontDistID,
				nil, //new domains were not set
				&defaultTTL,
				&forwardedHeaders,
				&forwardCookies)

			Expect(err).ToNot(HaveOccurred())
			Expect(fakeDistribution.UpdateCallCount()).To(Equal(1))
			Expect(fakeCertsManager.RequestCertificateCallCount()).To(Equal(0))
		})

		It("should request a new certificate from ACM when domains are updated", func() {

			Expect(route.Certificates).To(HaveLen(2))

			fakeDistribution.UpdateReturns(cfDist, nil)

			returnedCertArn := "A-newly-requested-cert-arn"

			fakeCertsManager.RequestCertificateReturns(aws.String(returnedCertArn), nil)

			newDomains := "example.com,info.example.com,blog.example.com"

			_, err := manager.Update(
				cloudfrontDistID,
				&newDomains,
				&defaultTTL,
				&forwardedHeaders,
				&forwardCookies)

			routeArg := fakeDatastore.SaveArgsForCall(0)

			Expect(err).ToNot(HaveOccurred())
			Expect(fakeDistribution.UpdateCallCount()).To(Equal(1))
			Expect(fakeCertsManager.RequestCertificateCallCount()).To(Equal(1))
			Expect(routeArg.Certificates).To(HaveLen(3))
			Expect(routeArg.Certificates[2].CertificateArn).To(Equal(returnedCertArn))
			Expect(routeArg.Certificates[2].CertificateStatus).To(Equal(models.CertificateStatusValidating))
		})
	})

	Context("CheckRoutesToUpdate", func() {
		var (
			fakeDistribution *utilsmocks.FakeDistribution
			fakeCertsManager *utilsmocks.FakeCertificateManager
			manager          models.RouteManager
			// route            *models.Route
			fakeDatastore        *modelsmocks.FakeRouteStore
			provisioningRoutes   []models.Route
			deprovisioningRoutes []models.Route
			logger               lager.Logger
			settings             config.Settings
		)

		BeforeEach(func() {
			settings = config.Settings{}

			logger = lager.NewLogger("test")

			logger.RegisterSink(lager.NewWriterSink(GinkgoWriter, lager.INFO))

			fakeDistribution = &utilsmocks.FakeDistribution{}

			fakeCertsManager = &utilsmocks.FakeCertificateManager{}

			fakeDatastore = &modelsmocks.FakeRouteStore{}

			provisioningRoutes = []models.Route{}
			deprovisioningRoutes = []models.Route{}

			fakeDatastore.FindAllMatchingCalls(func(r models.Route) ([]models.Route, error) {
				if r.State == models.Provisioning {
					return provisioningRoutes, nil
				} else if r.State == models.Deprovisioning {
					return deprovisioningRoutes, nil
				} else {
					return nil, errors.New("No routes found")
				}
			})

		})

		JustBeforeEach(func() {
			manager = models.NewManager(
				logger,
				fakeDistribution,
				settings,
				fakeDatastore,
				fakeCertsManager,
			)
		})

		It("finds routes in both `Provisioning` and `Deprovisioning` states", func() {

			manager.CheckRoutesToUpdate()

			Expect(fakeDatastore.FindAllMatchingCallCount()).To(Equal(2))

			fetchedProvisioning := false
			fetchedDeprovisioning := false

			firstCallExample := fakeDatastore.FindAllMatchingArgsForCall(0)
			secondCallExample := fakeDatastore.FindAllMatchingArgsForCall(1)

			if firstCallExample.State == models.Provisioning || secondCallExample.State == models.Provisioning {
				fetchedProvisioning = true
			}

			if firstCallExample.State == models.Deprovisioning || secondCallExample.State == models.Deprovisioning {
				fetchedDeprovisioning = true
			}

			Expect(fetchedProvisioning).To(BeTrue())
			Expect(fetchedDeprovisioning).To(BeTrue())
		})

		It("when a CNAMEAlreadyExists error occurs, set the route.State to Conflict", func() {
			//1. RouteStore needs to return at least a single route in a Provisioning state
			route := models.Route{
				InstanceId:     "instanceID",
				DistId:         "DistID",
				State:          models.Provisioning,
				DomainExternal: "domain.com",
				Origin:         "origin.com",
				Path:           "",
				DefaultTTL:     600,
				InsecureOrigin: false,
				Certificates: []models.Certificate{
					{
						CertificateArn:    "CertificateArn",
						CertificateStatus: models.CertificateStatusValidating,
					},
				},
			}

			provisioningRoutes = []models.Route{route}

			//2. CloudFront needs to return a distribution
			dist := cloudfront.Distribution{
				Id:     aws.String("DistID"),
				Status: aws.String("Deployed"),
				DistributionConfig: &cloudfront.DistributionConfig{
					Aliases: &cloudfront.Aliases{Items: []*string{aws.String("domain.com")}},
					Enabled: aws.Bool(true),
				},
			}

			fakeDistribution.GetReturns(&dist, nil)

			//3. ACM has to say that Certificate is Issued
			fakeCertsManager.IsCertificateIssuedReturns(true, nil)

			//4. Cloudfront SetCertificateAndCname should return an error CNAME Already Exists
			fakeDistribution.SetCertificateAndCnameReturns(errors.New(cloudfront.ErrCodeCNAMEAlreadyExists))

			//Call the method
			manager.CheckRoutesToUpdate()

			//5. Assert that the route was saved with a route.State = Conflict
			Expect(fakeDatastore.SaveCallCount()).To(Equal(1))
			Expect(fakeDatastore.SaveArgsForCall(0).State).To(Equal(models.Conflict))

		})

		It("When utils.ErrValidationTimedOut error occurs, set the route.State to Failed", func() {
			//1. RouteStore needs to return at least a single route in a Provisioning state
			route := models.Route{
				InstanceId:     "instanceID",
				DistId:         "DistID",
				State:          models.Provisioning,
				DomainExternal: "domain.com",
				Origin:         "origin.com",
				Path:           "",
				DefaultTTL:     600,
				InsecureOrigin: false,
				Certificates: []models.Certificate{
					{
						CertificateArn:    "CertificateArn",
						CertificateStatus: models.CertificateStatusValidating,
					},
				},
			}
			provisioningRoutes = []models.Route{route}

			//2. CloudFront needs to return a distribution
			dist := cloudfront.Distribution{
				Id:     aws.String("DistID"),
				Status: aws.String("Deployed"),
				DistributionConfig: &cloudfront.DistributionConfig{
					Aliases: &cloudfront.Aliases{Items: []*string{aws.String("domain.com")}},
					Enabled: aws.Bool(true),
				},
			}

			fakeDistribution.GetReturns(&dist, nil)

			//3. ACM triggers the 'utils.ErrValidationTimedOut' error
			fakeCertsManager.IsCertificateIssuedReturns(false, utils.ErrValidationTimedOut)

			//4 Call the method
			manager.CheckRoutesToUpdate()

			//5. Assert that the route was saved with a route.State = Conflict
			Expect(fakeDatastore.SaveCallCount()).To(Equal(1))
			Expect(fakeDatastore.SaveArgsForCall(0).State).To(Equal(models.Failed))

		})

		It("When provisioning has expired, set route.State to TimedOut", func() {
			//1. RouteStore needs to return at least a single route in a Provisioning state
			// and set the provisioningSince to 85 hours ago
			provisioningSincePeriod := time.Now().Add(-1 * models.ProvisioningExpirationPeriodHours).Add(-1 * time.Hour)
			route := models.Route{
				InstanceId:        "instanceID",
				DistId:            "DistID",
				State:             models.Provisioning,
				DomainExternal:    "domain.com",
				Origin:            "origin.com",
				Path:              "",
				DefaultTTL:        600,
				InsecureOrigin:    false,
				ProvisioningSince: &provisioningSincePeriod,
				Certificates: []models.Certificate{
					{
						CertificateArn:    "CertificateArn",
						CertificateStatus: models.CertificateStatusValidating,
					},
				},
			}

			provisioningRoutes = []models.Route{route}

			//2. CloudFront needs to return a distribution
			dist := cloudfront.Distribution{
				Id:     aws.String("DistID"),
				Status: aws.String("Deployed"),
				DistributionConfig: &cloudfront.DistributionConfig{
					Aliases: &cloudfront.Aliases{Items: []*string{aws.String("domain.com")}},
					Enabled: aws.Bool(true),
				},
			}

			fakeDistribution.GetReturns(&dist, nil)

			//3. ACM has to say that Certificate is Not Issued, so we can progress to the
			// Provisioing expired
			fakeCertsManager.IsCertificateIssuedReturns(false, nil)

			fakeDistribution.DisableReturns(nil)

			//4 Call the method
			manager.CheckRoutesToUpdate()

			//5. Assert that the route was saved with a route.State = Deprovisioning
			Expect(fakeDistribution.DisableCallCount()).To(Equal(0), "Disable was called, and it should not have been.")
			Expect(fakeDatastore.SaveCallCount()).To(Equal(1))
			Expect(fakeDatastore.SaveArgsForCall(0).State).To(Equal(models.TimedOut))
		})
	})

	Context("GetDNSChallenges", func() {
		var (
			validatingCert models.Certificate
			attachedCert   models.Certificate
			route          models.Route
			manager        models.RouteManager
			certsManager   *utilsmocks.FakeCertificateManager
			logger         lager.Logger
			settings       config.Settings
		)
		BeforeEach(func() {
			settings = config.Settings{}

			logger = lager.NewLogger("test")

			firstOfMay := time.Date(2020, 05, 01, 12, 00, 00, 00, time.UTC)
			firstOfJune := time.Date(2020, 06, 01, 12, 00, 00, 00, time.UTC)
			now := time.Now()

			validatingCert = models.Certificate{
				Model: gorm.Model{
					ID:        4,
					CreatedAt: firstOfJune,
					UpdatedAt: now,
				},
				CertificateStatus: models.CertificateStatusValidating,
				CertificateArn:    "arn:aws:acm::validating-cert",
			}

			attachedCert = models.Certificate{
				Model: gorm.Model{
					ID:        3,
					CreatedAt: firstOfMay,
					UpdatedAt: firstOfJune,
				},
				CertificateStatus: models.CertificateStatusAttached,
				CertificateArn:    "arn:aws:acm::attached-cert",
			}

			route = models.Route{
				Certificates: []models.Certificate{
					validatingCert,
					attachedCert,
				},
			}

			certsManager = &utilsmocks.FakeCertificateManager{}
		})

		JustBeforeEach(func() {
			manager = models.NewManager(
				logger,
				&utils.Distribution{},
				settings,
				&models.RouteStore{},
				certsManager,
			)
		})

		Context("when onlyValidatingCertificates = true", func() {
			It("only requests DNS challenges for certificates which are in the 'VALIDATING' state", func() {
				domainValidationChallenge := utils.DomainValidationChallenge{
					DomainName:       "domain.com",
					RecordName:       "domain.com",
					RecordType:       "CNAME",
					RecordValue:      "BlahBlahBlah",
					ValidationStatus: "PENDING_VALIDATION",
				}

				certsManager.GetDomainValidationChallengesReturns([]utils.DomainValidationChallenge{domainValidationChallenge}, nil)

				_, err := manager.GetDNSChallenges(&route, true)

				Expect(err).ToNot(HaveOccurred())
				Expect(certsManager.GetDomainValidationChallengesCallCount()).To(Equal(1))
				requestedArn := certsManager.GetDomainValidationChallengesArgsForCall(0)
				Expect(requestedArn).To(Equal(validatingCert.CertificateArn))
			})
		})

		Context("when onlyValidatingCertificates = false", func() {
			It("requests DNS challenges for any validating certificates as well as the currently attached cert", func() {
				domainValidationChallenge := utils.DomainValidationChallenge{
					DomainName:       "domain.com",
					RecordName:       "domain.com",
					RecordType:       "CNAME",
					RecordValue:      "BlahBlahBlah",
					ValidationStatus: "PENDING_VALIDATION",
				}

				certsManager.GetDomainValidationChallengesReturns([]utils.DomainValidationChallenge{domainValidationChallenge}, nil)

				_, err := manager.GetDNSChallenges(&route, false)

				Expect(err).ToNot(HaveOccurred())
				Expect(certsManager.GetDomainValidationChallengesCallCount()).To(Equal(2))

				requestedArns := []string{}
				for i := 0; i < certsManager.GetDomainValidationChallengesCallCount(); i++ {
					requestedArns = append(requestedArns, certsManager.GetDomainValidationChallengesArgsForCall(i))
				}

				Expect(requestedArns).To(ContainElements(attachedCert.CertificateArn, validatingCert.CertificateArn))
			})
		})

		Context("when a certificate ARN stored in the database does not exist in ACM", func() {
			BeforeEach(func() {
				certsManager.GetDomainValidationChallengesCalls(func(arn string) ([]utils.DomainValidationChallenge, error) {
					switch arn {
					case "arn:aws:acm::validating-cert":
						return nil, &acm.ResourceNotFoundException{}
					case "arn:aws:acm::attached-cert":
						return []utils.DomainValidationChallenge{
							utils.DomainValidationChallenge{
								DomainName:       "domain.com",
								RecordName:       "domain.com",
								RecordType:       "CNAME",
								RecordValue:      "BlahBlahBlah",
								ValidationStatus: "PENDING_VALIDATION",
							},
						}, nil
					}

					return nil, fmt.Errorf("unexpected arn: '%s'", arn)
				})
			})

			It("does not return an error, and returns the rest of the results", func() {
				challenges, err := manager.GetDNSChallenges(&route, false)

				Expect(err).ToNot(HaveOccurred())
				Expect(certsManager.GetDomainValidationChallengesCallCount()).To(Equal(2))

				Expect(challenges).To(HaveLen(1))
			})
		})
	})
})
