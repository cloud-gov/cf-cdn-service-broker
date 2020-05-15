package models_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"time"

	"code.cloudfoundry.org/lager"
	"github.com/18F/cf-cdn-service-broker/lego/acme"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/aws/aws-sdk-go/service/iam"

	"github.com/18F/cf-cdn-service-broker/config"
	"github.com/18F/cf-cdn-service-broker/models"
	"github.com/18F/cf-cdn-service-broker/models/mocks"
	"github.com/18F/cf-cdn-service-broker/utils"

	"github.com/stretchr/testify/mock"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type MockUtilsIam struct {
	mock.Mock

	Settings config.Settings
	Service  *iam.IAM
}

// test doesn't execute this method
func (_f MockUtilsIam) UploadCertificate(name string, cert acme.CertificateResource) (string, error) {
	return "", nil
}

// don't mock this method
func (_f MockUtilsIam) ListCertificates(callback func(iam.ServerCertificateMetadata) bool) error {
	orig := &utils.Iam{Settings: _f.Settings, Service: _f.Service}
	return orig.ListCertificates(callback)
}

func (_f MockUtilsIam) DeleteCertificate(certName string) error {
	args := _f.Called(certName)
	return args.Error(0)
}

func StubAcmeClientProvider() *mocks.FakeAcmeClientProvider {
	acmeProviderMock := mocks.FakeAcmeClientProvider{}
	acmeProviderMock.GetDNS01ClientReturns(&mocks.FakeAcmeClient{}, nil)
	acmeProviderMock.GetHTTP01ClientReturns(&mocks.FakeAcmeClient{}, nil)
	return &acmeProviderMock
}

const (
	// hopefully the CDN broker is gone in 36500 days
	selfSignedCert = `-----BEGIN CERTIFICATE-----
MIIBzDCCAXYCCQDis4Zpma57yjANBgkqhkiG9w0BAQsFADBsMQswCQYDVQQGEwJH
QjEQMA4GA1UECAwHRW5nbGFuZDEPMA0GA1UEBwwGTG9uZG9uMQ0wCwYDVQQKDARD
QUJPMQwwCgYDVQQLDANHRFMxHTAbBgNVBAMMFGNsb3VkLnNlcnZpY2UuZ292LnVr
MCAXDTE5MDcyNTE1NDQ1M1oYDzIxMTkwNzAxMTU0NDUzWjBsMQswCQYDVQQGEwJH
QjEQMA4GA1UECAwHRW5nbGFuZDEPMA0GA1UEBwwGTG9uZG9uMQ0wCwYDVQQKDARD
QUJPMQwwCgYDVQQLDANHRFMxHTAbBgNVBAMMFGNsb3VkLnNlcnZpY2UuZ292LnVr
MFwwDQYJKoZIhvcNAQEBBQADSwAwSAJBALAAb/6B5tu6YHXj3fVBptvYjnnCLYjQ
MnoDJjHksKKS66pvu3P56Xr5usEsV3zA8hU9M5939LG9y39InfhWcpsCAwEAATAN
BgkqhkiG9w0BAQsFAANBAJCU4Mxpa+WvDe0vg/8l5Pk2zEDXQ6jw+KgW2aAOhcMH
VZZ3cRRY1RvyqlEyjRqlO9pJSp7RQYB4dwNg/MpXArE=
-----END CERTIFICATE-----`
)

var _ = Describe("Models", func() {

	It("should delete orphaned certs", func() {
		logger := lager.NewLogger("cdn-cron-test")
		logOutput := bytes.NewBuffer([]byte{})
		logger.RegisterSink(lager.NewWriterSink(logOutput, lager.ERROR))

		settings, _ := config.NewSettings()
		session := session.New(nil)

		//mock out the aws call to return a fixed list of certs, two of which should be deleted
		fakeiam := iam.New(session)
		fakeiam.Handlers.Clear()
		fakeiam.Handlers.Send.PushBack(func(r *request.Request) {
			//t.Log(r.Operation.Name)
			switch r.Operation.Name {
			case "ListServerCertificates":
				old := time.Now().AddDate(0, 0, -2)
				current := time.Now().AddDate(0, 0, 0)

				list := []*iam.ServerCertificateMetadata{
					&iam.ServerCertificateMetadata{
						Arn:                   aws.String("an-active-certificate"),
						ServerCertificateName: aws.String("an-active-certificate"),
						ServerCertificateId:   aws.String("an-active-certificate"),
						UploadDate:            &old,
					},
					&iam.ServerCertificateMetadata{
						Arn:                   aws.String("some-other-active-certificate"),
						ServerCertificateName: aws.String("some-other-active-certificate"),
						ServerCertificateId:   aws.String("some-other-active-certificate"),
						UploadDate:            &old,
					},
					&iam.ServerCertificateMetadata{
						Arn:                   aws.String("orphaned-but-not-old-enough"),
						ServerCertificateName: aws.String("orphaned-but-not-old-enough"),
						ServerCertificateId:   aws.String("this-cert-should-not-be-deleted"),
						UploadDate:            &current,
					},
					&iam.ServerCertificateMetadata{
						Arn:                   aws.String("some-orphaned-cert"),
						ServerCertificateName: aws.String("some-orphaned-cert"),
						ServerCertificateId:   aws.String("this-cert-should-be-deleted"),
						UploadDate:            &old,
					},
					&iam.ServerCertificateMetadata{
						Arn:                   aws.String("some-other-orphaned-cert"),
						ServerCertificateName: aws.String("some-other-orphaned-cert"),
						ServerCertificateId:   aws.String("this-cert-should-also-be-deleted"),
						UploadDate:            &old,
					},
				}
				data := r.Data.(*iam.ListServerCertificatesOutput)
				data.IsTruncated = aws.Bool(false)
				data.ServerCertificateMetadataList = list
			}
		})

		//mock out the aws call to return a fixed list of distributions
		fakecf := cloudfront.New(session)
		fakecf.Handlers.Clear()
		fakecf.Handlers.Send.PushBack(func(r *request.Request) {
			//t.Log(r.Operation.Name)
			switch r.Operation.Name {
			case "ListDistributions2019_03_26":
				list := []*cloudfront.DistributionSummary{
					&cloudfront.DistributionSummary{
						ARN: aws.String("some-distribution"),
						ViewerCertificate: &cloudfront.ViewerCertificate{
							IAMCertificateId: aws.String("an-active-certificate"),
						},
					},
					&cloudfront.DistributionSummary{
						ARN: aws.String("some-other-distribution"),
						ViewerCertificate: &cloudfront.ViewerCertificate{
							IAMCertificateId: aws.String("some-other-active-certificate"),
						},
					},
				}

				data := r.Data.(*cloudfront.ListDistributionsOutput)
				data.DistributionList = &cloudfront.DistributionList{
					IsTruncated: aws.Bool(false),
					Items:       list,
				}
			}
		})

		mui := new(MockUtilsIam)
		mui.Settings = settings
		mui.Service = fakeiam

		// expect the orphaned certs to be deleted
		mui.On("DeleteCertificate", "some-orphaned-cert").Return(nil)
		mui.On("DeleteCertificate", "some-other-orphaned-cert").Return(nil)

		acmeProviderMock := StubAcmeClientProvider()

		m := models.NewManager(
			logger,
			mui,
			&utils.Distribution{settings, fakecf},
			settings,
			acmeProviderMock,
			&mocks.FakeRouteStore{},
		)

		//run the test
		m.DeleteOrphanedCerts()

		//check our expectations
		mui.AssertExpectations(GinkgoT())
	})

	It("should handle AWS certificate deletion failure gracefully", func() {
		logger := lager.NewLogger("cdn-cron-test")
		logOutput := bytes.NewBuffer([]byte{})
		logger.RegisterSink(lager.NewWriterSink(logOutput, lager.ERROR))

		settings, _ := config.NewSettings()
		session := session.New(nil)

		//mock out the aws call to return a certificate to delete
		fakeiam := iam.New(session)
		fakeiam.Handlers.Clear()
		fakeiam.Handlers.Send.PushBack(func(r *request.Request) {
			//t.Log(r.Operation.Name)
			switch r.Operation.Name {
			case "ListServerCertificates":
				old := time.Now().AddDate(0, 0, -2)

				list := []*iam.ServerCertificateMetadata{
					&iam.ServerCertificateMetadata{
						Arn:                   aws.String("some-orphaned-cert"),
						ServerCertificateName: aws.String("some-orphaned-cert"),
						ServerCertificateId:   aws.String("this-cert-should-be-deleted"),
						UploadDate:            &old,
					},
				}
				data := r.Data.(*iam.ListServerCertificatesOutput)
				data.IsTruncated = aws.Bool(false)
				data.ServerCertificateMetadataList = list
			}
		})

		//mock out the aws call to return a fixed list of distributions
		fakecf := cloudfront.New(session)
		fakecf.Handlers.Clear()
		fakecf.Handlers.Send.PushBack(func(r *request.Request) {
			//t.Log(r.Operation.Name)
			switch r.Operation.Name {
			case "ListDistributions2019_03_26":
				list := []*cloudfront.DistributionSummary{}
				data := r.Data.(*cloudfront.ListDistributionsOutput)
				data.DistributionList = &cloudfront.DistributionList{
					IsTruncated: aws.Bool(false),
					Items:       list,
				}
			}
		})

		mui := new(MockUtilsIam)
		mui.Settings = settings
		mui.Service = fakeiam

		// expect the orphaned certs to fail to be deleted
		mui.On("DeleteCertificate", "some-orphaned-cert").Return(errors.New("DeleteCertificate error"))

		acmeProviderMock := StubAcmeClientProvider()

		m := models.NewManager(
			logger,
			mui,
			&utils.Distribution{settings, fakecf},
			settings,
			acmeProviderMock,
			&mocks.FakeRouteStore{},
		)

		//run the test
		m.DeleteOrphanedCerts()

		//check our expectations
		mui.AssertExpectations(GinkgoT())

		Expect(logOutput.String()).To(
			ContainSubstring("DeleteCertificate error"),
			"was expecting DeleteCertificate error to be logged",
		)
	})

	It("should handle AWS certificate deletion failure gracefully when listing certificates fails", func() {
		logger := lager.NewLogger("cdn-cron-test")
		logOutput := bytes.NewBuffer([]byte{})
		logger.RegisterSink(lager.NewWriterSink(logOutput, lager.ERROR))

		settings, _ := config.NewSettings()
		session := session.New(nil)

		//mock out the aws call to return a fixed list of distributions
		fakecf := cloudfront.New(session)
		fakecf.Handlers.Clear()
		fakecf.Handlers.Send.PushBack(func(r *request.Request) {
			//t.Log(r.Operation.Name)
			switch r.Operation.Name {
			case "ListDistributions2019_03_26":
				list := []*cloudfront.DistributionSummary{
					&cloudfront.DistributionSummary{
						ARN: aws.String("some-distribution"),
						ViewerCertificate: &cloudfront.ViewerCertificate{
							IAMCertificateId: aws.String("an-active-certificate"),
						},
					},
					&cloudfront.DistributionSummary{
						ARN: aws.String("some-other-distribution"),
						ViewerCertificate: &cloudfront.ViewerCertificate{
							IAMCertificateId: aws.String("some-other-active-certificate"),
						},
					},
				}

				data := r.Data.(*cloudfront.ListDistributionsOutput)
				data.DistributionList = &cloudfront.DistributionList{
					IsTruncated: aws.Bool(false),
					Items:       list,
				}
			}
		})

		//mock out the aws call to return a fixed list of certs, two of which should be deleted
		fakeiam := iam.New(session)
		fakeiam.Handlers.Clear()
		fakeiam.Handlers.Send.PushBack(func(r *request.Request) {
			r.Data = nil
			r.Error = errors.New("ListServerCertificates error")
		})

		mui := new(MockUtilsIam)
		mui.Settings = settings
		mui.Service = fakeiam

		acmeProviderMock := StubAcmeClientProvider()

		m := models.NewManager(
			logger,
			mui,
			&utils.Distribution{settings, fakecf},
			settings,
			acmeProviderMock,
			&mocks.FakeRouteStore{},
		)

		//run the test
		m.DeleteOrphanedCerts()

		//check our expectations
		mui.AssertNumberOfCalls(GinkgoT(), "DeleteCertificate", 0)
		mui.AssertExpectations(GinkgoT())

		Expect(logOutput.String()).To(
			ContainSubstring("ListServerCertificates error"),
			"was expecting ListServerCertificates error to be logged",
		)
	})

	It("should handle AWS certificate deletion failure gracefully when listing cloudfront distributions fails", func() {
		logger := lager.NewLogger("cdn-cron-test")
		logOutput := bytes.NewBuffer([]byte{})
		logger.RegisterSink(lager.NewWriterSink(logOutput, lager.ERROR))

		settings, _ := config.NewSettings()
		session := session.New(nil)

		//mock out the aws call to return a fixed list of distributions
		fakecf := cloudfront.New(session)
		fakecf.Handlers.Clear()
		fakecf.Handlers.Send.PushBack(func(r *request.Request) {
			r.Data = nil
			r.Error = errors.New("ListDistributions error")
		})

		//mock out the aws call to return one certificate that would be deleted but shoudln't if listing distributions fails
		fakeiam := iam.New(session)
		fakeiam.Handlers.Clear()
		fakeiam.Handlers.Send.PushBack(func(r *request.Request) {
			//t.Log(r.Operation.Name)
			switch r.Operation.Name {
			case "ListServerCertificates":
				old := time.Now().AddDate(0, 0, -2)

				list := []*iam.ServerCertificateMetadata{
					&iam.ServerCertificateMetadata{
						Arn:                   aws.String("some-orphaned-cert"),
						ServerCertificateName: aws.String("some-orphaned-cert"),
						ServerCertificateId:   aws.String("this-cert-should-be-deleted"),
						UploadDate:            &old,
					},
				}
				data := r.Data.(*iam.ListServerCertificatesOutput)
				data.IsTruncated = aws.Bool(false)
				data.ServerCertificateMetadataList = list
			}
		})

		mui := new(MockUtilsIam)
		mui.Settings = settings
		mui.Service = fakeiam

		acmeProviderMock := StubAcmeClientProvider()

		m := models.NewManager(
			logger,
			mui,
			&utils.Distribution{settings, fakecf},
			settings,
			acmeProviderMock,
			&mocks.FakeRouteStore{},
		)

		//run the test
		m.DeleteOrphanedCerts()

		//check our expectations
		mui.AssertNumberOfCalls(GinkgoT(), "DeleteCertificate", 0)
		mui.AssertExpectations(GinkgoT())

		Expect(logOutput.String()).To(
			ContainSubstring("ListDistributions error"),
			"was expecting ListDistributions error to be logged",
		)
	})

	Context("Create", func() {
		It("should perform only DNS01 challenges", func() {
			logger := lager.NewLogger("dns-01-test-only")
			logger.RegisterSink(lager.NewWriterSink(GinkgoWriter, lager.ERROR))

			instanceID := "cloudfoundry-instance-id"
			domain := "foo.paas.gov.uk"
			origin := "foo.cloudapps.digital"
			defaultTTL := int64(0)
			forwardedHeaders := utils.Headers{}
			forwardCookies := false
			tags := map[string]string{}

			settings, _ := config.NewSettings()
			awsSession := session.New(nil)

			fakecf := cloudfront.New(awsSession)
			fakecf.Handlers.Clear()
			fakecf.Handlers.Send.PushBack(func(r *request.Request) {
				switch r.Operation.Name {
				case "CreateDistributionWithTags2019_03_26":
					data := r.Data.(*cloudfront.CreateDistributionWithTagsOutput)
					data.Distribution = &cloudfront.Distribution{
						DomainName: aws.String("foo.paas.gov.uk"),
						Id:         aws.String("cloudfront-distribution-id"),
					}
				}
			})

			fakeiam := iam.New(awsSession)
			mui := new(MockUtilsIam)
			mui.Settings = settings
			mui.Service = fakeiam

			acmeProviderMock := StubAcmeClientProvider()

			routeStoreIfaceMock := mocks.FakeRouteStore{}
			routeStoreIfaceMock.CreateReturns(nil)

			manager := models.NewManager(
				logger,
				mui,
				&utils.Distribution{settings, fakecf},
				settings,
				acmeProviderMock,
				&routeStoreIfaceMock,
			)

			_, err := manager.Create(
				instanceID, domain, origin, defaultTTL,
				forwardedHeaders, forwardCookies, tags,
			)
			Expect(acmeProviderMock.GetHTTP01ClientCallCount()).To(
				Equal(0), "Creating a CDN service should never use HTTP challenges",
			)
			Expect(acmeProviderMock.GetDNS01ClientCallCount()).To(
				Equal(1), "Creating a CDN service should use only DNS challenges",
			)
			// We are just testing the newing up of the correct client
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Poll", func() {
		It("updateProvisioning works correctly", func() {
			logger := lager.NewLogger("dns-01-test-only")
			logger.RegisterSink(lager.NewWriterSink(GinkgoWriter, lager.ERROR))

			instanceID := "cloudfoundry-instance-id"
			domain := "foo.paas.gov.uk"
			origin := "foo.cloudapps.digital"
			defaultTTL := int64(0)

			settings, _ := config.NewSettings()
			awsSession := session.New(nil)

			fakecf := cloudfront.New(awsSession)
			fakecf.Handlers.Clear()
			fakecf.Handlers.Send.PushBack(func(r *request.Request) {
				distributionConfig := &cloudfront.DistributionConfig{
					Aliases: &cloudfront.Aliases{
						Quantity: aws.Int64(0),
						Items:    []*string{},
					},
					Enabled:           aws.Bool(true),
					ViewerCertificate: &cloudfront.ViewerCertificate{},
					DefaultCacheBehavior: &cloudfront.DefaultCacheBehavior{
						ForwardedValues: &cloudfront.ForwardedValues{
							Cookies: &cloudfront.CookiePreference{
								Forward: aws.String("all"),
							},
							QueryStringCacheKeys: &cloudfront.QueryStringCacheKeys{},
						},
						TrustedSigners: &cloudfront.TrustedSigners{},
						AllowedMethods: &cloudfront.AllowedMethods{
							CachedMethods: &cloudfront.CachedMethods{},
						},
					},
				}

				switch r.Operation.Name {
				case "GetDistribution2019_03_26":
					data := r.Data.(*cloudfront.GetDistributionOutput)
					data.Distribution = &cloudfront.Distribution{
						Id:                 aws.String("cloudfront-distribution-id"),
						DomainName:         aws.String("foo.paas.gov.uk"),
						Status:             aws.String("Deployed"),
						DistributionConfig: distributionConfig,
					}
					data.ETag = aws.String("etag")
				case "GetDistributionConfig2019_03_26":
					data := r.Data.(*cloudfront.GetDistributionConfigOutput)
					data.DistributionConfig = distributionConfig
				}
			})

			fakeiam := iam.New(awsSession)
			mui := new(MockUtilsIam)
			mui.Settings = settings
			mui.Service = fakeiam

			fakeDNS01Client := &mocks.FakeAcmeClient{}
			fakeDNS01Client.RequestCertificateReturns(
				acme.CertificateResource{Certificate: []byte(selfSignedCert)},
				nil,
			)
			fakeDNS01Client.GetChallengesReturns(
				[]acme.AuthorizationResource{},
				map[string]error{},
			)

			acmeProviderMock := mocks.FakeAcmeClientProvider{}
			acmeProviderMock.GetHTTP01ClientReturns(&mocks.FakeAcmeClient{}, nil)
			acmeProviderMock.GetDNS01ClientReturns(fakeDNS01Client, nil)

			routeStoreIface := mocks.FakeRouteStore{}

			manager := models.NewManager(
				logger,
				mui,
				&utils.Distribution{settings, fakecf},
				settings,
				&acmeProviderMock,
				&routeStoreIface,
			)

			route := &models.Route{
				InstanceId:     instanceID,
				State:          models.Provisioning,
				DomainExternal: domain,
				Origin:         origin,
				Path:           "",
				DefaultTTL:     defaultTTL,
				InsecureOrigin: false,
			}

			rsaTestKey, err := rsa.GenerateKey(rand.Reader, 2048)
			Expect(err).NotTo(HaveOccurred(), "Generating test key")
			pemdata := pem.EncodeToMemory(
				&pem.Block{
					Type:  "RSA PRIVATE KEY",
					Bytes: x509.MarshalPKCS1PrivateKey(rsaTestKey),
				},
			)

			route.UserData.Key = pemdata

			routeStoreIface.SaveReturns(nil)

			err = manager.Poll(route)
			Expect(err).NotTo(HaveOccurred())
			Expect(acmeProviderMock.GetDNS01ClientCallCount()).To(Equal(1))
			Expect(acmeProviderMock.GetHTTP01ClientCallCount()).To(Equal(0))
		})
	})

	Context("GetCurrentlyDeployedDomains", func() {
		setupCloudFrontMock := func(aliases []string, fakecf *cloudfront.CloudFront) {
			awsAliases := make([]*string, 0)
			for _, alias := range aliases {
				awsAliases = append(awsAliases, aws.String(alias))
			}

			aliasQuantity := len(awsAliases)

			fakecf.Handlers.Send.PushBack(func(r *request.Request) {
				distributionConfig := &cloudfront.DistributionConfig{
					Aliases: &cloudfront.Aliases{
						Quantity: aws.Int64(int64(aliasQuantity)),
						Items:    awsAliases,
					},
					Enabled:           aws.Bool(true),
					ViewerCertificate: &cloudfront.ViewerCertificate{},
					DefaultCacheBehavior: &cloudfront.DefaultCacheBehavior{
						ForwardedValues: &cloudfront.ForwardedValues{
							Cookies: &cloudfront.CookiePreference{
								Forward: aws.String("all"),
							},
							QueryStringCacheKeys: &cloudfront.QueryStringCacheKeys{},
						},
						TrustedSigners: &cloudfront.TrustedSigners{},
						AllowedMethods: &cloudfront.AllowedMethods{
							CachedMethods: &cloudfront.CachedMethods{},
						},
					},
				}

				switch r.Operation.Name {
				case "GetDistribution2019_03_26":
					data := r.Data.(*cloudfront.GetDistributionOutput)
					data.Distribution = &cloudfront.Distribution{
						Id:                 aws.String("cloudfront-distribution-id"),
						DomainName:         aws.String("foo.paas.gov.uk"),
						Status:             aws.String("Deployed"),
						DistributionConfig: distributionConfig,
					}
					data.ETag = aws.String("etag")
				case "GetDistributionConfig2019_03_26":
					data := r.Data.(*cloudfront.GetDistributionConfigOutput)
					data.DistributionConfig = distributionConfig
				}
			})
		}

		var (
			manager models.RouteManager
			route   *models.Route
			fakecf  *cloudfront.CloudFront
		)

		BeforeEach(func() {
			logger := lager.NewLogger("cdn-cron-test")
			logOutput := bytes.NewBuffer([]byte{})
			logger.RegisterSink(lager.NewWriterSink(logOutput, lager.ERROR))

			instanceID := "cloudfoundry-instance-id"
			domain := "foo.paas.gov.uk"
			origin := "foo.cloudapps.digital"
			defaultTTL := int64(0)

			settings, _ := config.NewSettings()
			awsSession := session.New(nil)

			fakecf = cloudfront.New(awsSession)
			fakecf.Handlers.Clear()

			fakeiam := iam.New(awsSession)
			mui := new(MockUtilsIam)
			mui.Settings = settings
			mui.Service = fakeiam

			manager = models.NewManager(
				logger,
				mui,
				&utils.Distribution{settings, fakecf},
				settings,
				StubAcmeClientProvider(),
				&mocks.FakeRouteStore{},
			)

			route = &models.Route{
				InstanceId:     instanceID,
				State:          models.Provisioning,
				DomainExternal: domain,
				Origin:         origin,
				Path:           "",
				DefaultTTL:     defaultTTL,
				InsecureOrigin: false,
			}
		})

		It("should return the domains correctly when only zero CNAMEs", func() {
			aliases := []string{}
			setupCloudFrontMock(aliases, fakecf)

			domains, err := manager.GetCurrentlyDeployedDomains(route)

			Expect(err).NotTo(HaveOccurred())
			Expect(domains).To(HaveLen(0))
		})

		It("should return the domains correctly when only one CNAME", func() {
			aliases := []string{"foo.cloudapps.digital"}
			setupCloudFrontMock(aliases, fakecf)

			domains, err := manager.GetCurrentlyDeployedDomains(route)

			Expect(err).NotTo(HaveOccurred())
			Expect(domains).To(ConsistOf("foo.cloudapps.digital"))
		})

		It("should return the domains correctly when many CNAMEs", func() {
			aliases := []string{"foo.cloudapps.digital", "bar.cloudapps.digital"}
			setupCloudFrontMock(aliases, fakecf)

			domains, err := manager.GetCurrentlyDeployedDomains(route)

			Expect(err).NotTo(HaveOccurred())
			Expect(domains).To(ConsistOf(
				"foo.cloudapps.digital",
				"bar.cloudapps.digital",
			))
		})
	})

	Context("Update", func() {
		var (
			cloudfrontDistID = "cloudfoundry-instance-id"
			domain           = "foo.paas.gov.uk"
			defaultTTL       = int64(0)
			forwardedHeaders = utils.Headers{"X-Forwarded-Five": true}
			forwardCookies   = false

			fakecf   *cloudfront.CloudFront
			fakeiam  *iam.IAM
			settings config.Settings
			mui      *MockUtilsIam
		)

		BeforeEach(func() {
			settings, _ = config.NewSettings()
			awsSession := session.New(nil)

			fakecf = cloudfront.New(awsSession)
			fakecf.Handlers.Clear()

			fakecf.Handlers.Send.PushBack(func(r *request.Request) {
				distributionConfig := &cloudfront.DistributionConfig{
					Aliases: &cloudfront.Aliases{
						Quantity: aws.Int64(1),
						Items:    []*string{aws.String("foo.paas.gov.uk")},
					},
					Enabled:           aws.Bool(true),
					ViewerCertificate: &cloudfront.ViewerCertificate{},
					CallerReference:   aws.String("hi mom"),
					DefaultCacheBehavior: &cloudfront.DefaultCacheBehavior{
						ForwardedValues: &cloudfront.ForwardedValues{
							Cookies: &cloudfront.CookiePreference{
								Forward: aws.String("all"),
							},
							QueryStringCacheKeys: &cloudfront.QueryStringCacheKeys{},
						},
						TrustedSigners: &cloudfront.TrustedSigners{},
						AllowedMethods: &cloudfront.AllowedMethods{
							CachedMethods: &cloudfront.CachedMethods{},
						},
					},
				}

				switch r.Operation.Name {
				case "GetDistribution2019_03_26":
					data := r.Data.(*cloudfront.GetDistributionOutput)
					data.Distribution = &cloudfront.Distribution{
						Id:                 aws.String("cloudfront-distribution-id"),
						DomainName:         aws.String("foo.paas.gov.uk"),
						Status:             aws.String("Deployed"),
						DistributionConfig: distributionConfig,
					}
					data.ETag = aws.String("etag")
				case "GetDistributionConfig2019_03_26":
					data := r.Data.(*cloudfront.GetDistributionConfigOutput)
					data.DistributionConfig = distributionConfig
				case "UpdateDistribution2019_03_26":
					data := r.Data.(*cloudfront.UpdateDistributionOutput)
					data.Distribution = &cloudfront.Distribution{
						DomainName:         aws.String("foo.paas.gov.uk"),
						Id:                 aws.String(cloudfrontDistID),
						DistributionConfig: distributionConfig,
					}
				}
			})

			fakeiam = iam.New(awsSession)
			mui = new(MockUtilsIam)
			mui.Settings = settings
			mui.Service = fakeiam
		})

		It("should not perform any ACME challenges when domains are not updated", func() {
			logger := lager.NewLogger("no-challenge-test-only")
			logger.RegisterSink(lager.NewWriterSink(GinkgoWriter, lager.ERROR))

			fakeDNS01Client := &mocks.FakeAcmeClient{}
			fakeDNS01Client.RequestCertificateReturns(
				acme.CertificateResource{Certificate: []byte(selfSignedCert)},
				nil,
			)
			fakeDNS01Client.GetChallengesReturns(
				[]acme.AuthorizationResource{},
				map[string]error{},
			)

			acmeProviderMock := StubAcmeClientProvider()

			fakeRoute := models.Route{
				InstanceId:     "route-one",
				State:          "provisioned",
				ChallengeJSON:  []byte(`{}`),
				DomainExternal: domain,
				DomainInternal: "foo.london.cloudapps.digital",
				DistId:         "cloudfront-dist-one",
				Origin:         "foo.london.cloudapps.digital",
				Path:           "",
				InsecureOrigin: false,
				UserDataID:     1,
				DefaultTTL:     0,
				Certificate:    models.Certificate{Domain: "foo.bar", CertURL: "this is a cert url"},
			}

			fakeUserData := models.UserData{
				Email: "foo@bar.org",
				Reg: []byte(`
					{
						"Email": "the-mocky-cloud-paas-team@digital.cabinet-office.gov.uk",
						"Registration": null
					}
				`),
				Key: generateKey(),
			}

			fakeRoute.UserData = fakeUserData

			fakeRouteStoreIface := mocks.FakeRouteStore{}

			fakeRouteStoreIface.SaveReturns(nil)
			fakeRouteStoreIface.FindOneMatchingReturns(fakeRoute, nil)

			// we are simulating that someone is updating the distribution, but does
			// not want to change the currently configured domain
			manager := models.NewManager(
				logger,
				mui,
				&utils.Distribution{settings, fakecf},
				settings,
				acmeProviderMock,
				&fakeRouteStoreIface,
			)

			brokerAPICallDomainArgument := ""

			performedAsynchronously, err := manager.Update(
				cloudfrontDistID,
				&brokerAPICallDomainArgument,
				&defaultTTL,
				&forwardedHeaders,
				&forwardCookies,
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(performedAsynchronously).To(Equal(false))
			Expect(acmeProviderMock.GetDNS01ClientCallCount()).To(Equal(0))
			Expect(acmeProviderMock.GetHTTP01ClientCallCount()).To(Equal(0))
		})

		It("should perform a DNS challenge when domains are updated", func() {
			logger := lager.NewLogger("dns-01-test-only")
			logger.RegisterSink(lager.NewWriterSink(GinkgoWriter, lager.ERROR))

			fakeDNS01Client := &mocks.FakeAcmeClient{}
			fakeDNS01Client.RequestCertificateReturns(
				acme.CertificateResource{Certificate: []byte(selfSignedCert)},
				nil,
			)
			fakeDNS01Client.GetChallengesReturns(
				[]acme.AuthorizationResource{},
				map[string]error{},
			)

			acmeProviderMock := mocks.FakeAcmeClientProvider{}
			acmeProviderMock.GetHTTP01ClientReturns(&mocks.FakeAcmeClient{}, nil)
			acmeProviderMock.GetDNS01ClientReturns(fakeDNS01Client, nil)

			fakeRoute := models.Route{
				InstanceId:     "route-one",
				State:          "provisioned",
				ChallengeJSON:  []byte(`{}`),
				DomainExternal: domain,
				DomainInternal: "foo.london.cloudapps.digital",
				DistId:         "cloudfront-dist-one",
				Origin:         "foo.london.cloudapps.digital",
				Path:           "",
				InsecureOrigin: false,
				UserDataID:     1,
				DefaultTTL:     0,
				Certificate:    models.Certificate{Domain: "foo.bar", CertURL: "this is a cert url"},
			}

			fakeUserData := models.UserData{
				Email: "foo@bar.org",
				Reg: []byte(`
					{
						"Email": "the-mocky-cloud-paas-team@digital.cabinet-office.gov.uk",
						"Registration": null
					}
				`),
				Key: generateKey(),
			}

			fakeRoute.UserData = fakeUserData

			fakeRouteStoreIface := mocks.FakeRouteStore{}

			fakeRouteStoreIface.SaveReturns(nil)
			fakeRouteStoreIface.FindOneMatchingReturns(fakeRoute, nil)

			// we are simulating that someone is updating the distribution, and DOES
			// want to change the currently configured domain
			manager := models.NewManager(
				logger,
				mui,
				&utils.Distribution{settings, fakecf},
				settings,
				&acmeProviderMock,
				&fakeRouteStoreIface,
			)

			brokerAPICallDomainArgument := "foo.paas.gov.uk,bar.paas.gov.uk"

			performedAsynchronously, err := manager.Update(
				cloudfrontDistID,
				&brokerAPICallDomainArgument,
				&defaultTTL,
				&forwardedHeaders,
				&forwardCookies,
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(performedAsynchronously).To(Equal(true))
			Expect(acmeProviderMock.GetDNS01ClientCallCount()).To(Equal(1))
			Expect(acmeProviderMock.GetHTTP01ClientCallCount()).To(Equal(0))
		})
	})

	Context("CheckRoutesToUpdate", func(){
		It("finds routes in both `Provisioning` and `Deprovisioning` states", func(){
			routeStoreFake := mocks.FakeRouteStore{}

			routeStoreFake.FindAllMatchingReturns([]models.Route{}, nil)

			settings, _ := config.NewSettings()
			manager := models.NewManager(
				lager.NewLogger("check-routes-to-update"),
				new(MockUtilsIam),
				&utils.Distribution{},
				settings,
				StubAcmeClientProvider(),
				&routeStoreFake,
			)

			manager.CheckRoutesToUpdate()

			Expect(routeStoreFake.FindAllMatchingCallCount()).To(Equal(2))

			fetchedProvisioning := false
			fetchedDeprovisioning := false

			firstCallExample := routeStoreFake.FindAllMatchingArgsForCall(0)
			secondCallExample := routeStoreFake.FindAllMatchingArgsForCall(1)

			if firstCallExample.State == models.Provisioning || secondCallExample.State == models.Provisioning {
				fetchedProvisioning = true
			}

			if firstCallExample.State == models.Deprovisioning || secondCallExample.State == models.Deprovisioning {
				fetchedDeprovisioning = true
			}

			Expect(fetchedProvisioning).To(BeTrue())
			Expect(fetchedDeprovisioning).To(BeTrue())
		})
	})
})
