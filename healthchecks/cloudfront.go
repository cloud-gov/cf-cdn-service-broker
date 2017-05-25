package healthchecks

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudfront"

	"github.com/18F/cf-cdn-service-broker/config"
)

func Cloudfront(settings config.Settings) error {
	session := session.New(aws.NewConfig().WithRegion(settings.AwsDefaultRegion))
	svc := cloudfront.New(session)

	params := &cloudfront.ListDistributionsInput{}
	_, err := svc.ListDistributions(params)
	if err != nil {
		return err
	}

	return nil
}
