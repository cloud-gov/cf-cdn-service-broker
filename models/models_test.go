package models_test

import (
	"os"
	"testing"
	"time"

	"code.cloudfoundry.org/lager"
	"github.com/jinzhu/gorm"
	"github.com/xenolf/lego/acme"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/aws/aws-sdk-go/service/iam"

	"github.com/18F/cf-cdn-service-broker/config"
	"github.com/18F/cf-cdn-service-broker/models"
	"github.com/18F/cf-cdn-service-broker/utils"

	"github.com/stretchr/testify/mock"
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

func TestDeleteOrphanedCerts(t *testing.T) {
	logger := lager.NewLogger("cdn-cron-test")
	logger.RegisterSink(lager.NewWriterSink(os.Stderr, lager.INFO))

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
					Arn: aws.String("an-active-certificate"),
					ServerCertificateName: aws.String("an-active-certificate"),
					ServerCertificateId:   aws.String("an-active-certificate"),
					UploadDate:            &old,
				},
				&iam.ServerCertificateMetadata{
					Arn: aws.String("some-other-active-certificate"),
					ServerCertificateName: aws.String("some-other-active-certificate"),
					ServerCertificateId:   aws.String("some-other-active-certificate"),
					UploadDate:            &old,
				},
				&iam.ServerCertificateMetadata{
					Arn: aws.String("orphaned-but-not-old-enough"),
					ServerCertificateName: aws.String("orphaned-but-not-old-enough"),
					ServerCertificateId:   aws.String("this-cert-should-not-be-deleted"),
					UploadDate:            &current,
				},
				&iam.ServerCertificateMetadata{
					Arn: aws.String("some-orphaned-cert"),
					ServerCertificateName: aws.String("some-orphaned-cert"),
					ServerCertificateId:   aws.String("this-cert-should-be-deleted"),
					UploadDate:            &old,
				},
				&iam.ServerCertificateMetadata{
					Arn: aws.String("some-other-orphaned-cert"),
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

	m := models.NewManager(
		logger,
		mui,
		&utils.Distribution{settings, fakecf},
		settings,
		&gorm.DB{},
	)

	//run the test
	m.DeleteOrphanedCerts()

	//check our expectations
	mui.AssertExpectations(t)

}
