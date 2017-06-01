package healthchecks

import (
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/18F/cf-cdn-service-broker/config"
)

func S3(settings config.Settings) error {
	bucket := settings.Bucket
	key := "healthcheck-test-key"

	session := session.New(aws.NewConfig().WithRegion(settings.AwsDefaultRegion))
	svc := s3.New(session)

	input := s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   strings.NewReader("cheese"),
	}

	_, err := svc.PutObject(&input)
	if err != nil {
		return err
	}

	_, err = svc.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return err
	}

	return nil
}
