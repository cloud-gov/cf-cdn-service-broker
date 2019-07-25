package models_test

import (
	"bytes"
	"errors"
	"time"

	"code.cloudfoundry.org/lager"
	"github.com/18F/cf-cdn-service-broker/lego/acme"
	"github.com/jinzhu/gorm"

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
			case "ListDistributions2016_11_25":
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
			&gorm.DB{},
			acmeProviderMock,
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
			case "ListDistributions2016_11_25":
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
			&gorm.DB{},
			acmeProviderMock,
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
			case "ListDistributions2016_11_25":
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
			&gorm.DB{},
			acmeProviderMock,
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
			&gorm.DB{},
			acmeProviderMock,
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
			path := "/"
			insecureOrigin := false
			forwardedHeaders := utils.Headers{}
			forwardCookies := false
			tags := map[string]string{}

			settings, _ := config.NewSettings()
			awsSession := session.New(nil)

			fakecf := cloudfront.New(awsSession)
			fakecf.Handlers.Clear()
			fakecf.Handlers.Send.PushBack(func(r *request.Request) {
				switch r.Operation.Name {
				case "ListDistributions2016_11_25":
					list := []*cloudfront.DistributionSummary{
						{
							Aliases: &cloudfront.Aliases{
								Items: []*string{
									aws.String("bar.paas.gov.uk"),
									aws.String("baz.paas.gov.uk"),
								},
							},
							DomainName: aws.String("foo.paas.gov.uk"),
							Id:         aws.String("some-cloudfront-id"),
						},
					}
					data := r.Data.(*cloudfront.ListDistributionsOutput)
					data.DistributionList = &cloudfront.DistributionList{
						IsTruncated: aws.Bool(false),
						Items:       list,
					}
				}
			})

			fakeiam := iam.New(awsSession)
			mui := new(MockUtilsIam)
			mui.Settings = settings
			mui.Service = fakeiam

			acmeProviderMock := StubAcmeClientProvider()

			manager := models.NewManager(
				logger,
				mui,
				&utils.Distribution{settings, fakecf},
				settings,
				&gorm.DB{},
				acmeProviderMock,
			)

			_, err := manager.Create(
				instanceID, domain, origin, path,
				insecureOrigin, forwardedHeaders, forwardCookies, tags,
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

})
