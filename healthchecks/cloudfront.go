package healthchecks

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudfront"

	"github.com/alphagov/paas-cdn-broker/config"
)

func Cloudfront(settings config.Settings) error {
	session, err := session.NewSession(aws.NewConfig().WithRegion(settings.AwsDefaultRegion))
	if err != nil {
		return err
	}

	svc := cloudfront.New(session)

	params := &cloudfront.ListDistributionsInput{}
	_, err = svc.ListDistributions(params)
	if err != nil {
		return err
	}

	return nil
}
