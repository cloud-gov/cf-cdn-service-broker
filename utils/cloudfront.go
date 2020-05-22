package utils

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudfront"

	"github.com/18F/cf-cdn-service-broker/config"
)

//counterfeiter:generate -o mocks/FakeDistribution.go --fake-name FakeDistribution cloudfront.go DistributionIface

type DistributionIface interface {
	Create(callerReference string, domains []string, origin string, defaultTTL int64, forwardedHeaders Headers, forwardCookies bool, tags map[string]string) (*cloudfront.Distribution, error)
	Update(distId string, domains *[]string, origin string, defaultTTL *int64, forwardedHeaders *Headers, forwardCookies *bool) (*cloudfront.Distribution, error)
	Get(distId string) (*cloudfront.Distribution, error)
	SetCertificateAndCname(distId, certId string, domains []string, acmCert bool) error
	Disable(distId string) error
	Delete(distId string) (bool, error)
	ListDistributions(callback func(cloudfront.DistributionSummary) bool) error
}

type Distribution struct {
	Settings config.Settings
	Service  *cloudfront.CloudFront
}

func (d *Distribution) getAliases(domains []string) *cloudfront.Aliases {
	var items []*string
	for _, d := range domains {
		items = append(items, aws.String(d))
	}
	return &cloudfront.Aliases{
		Quantity: aws.Int64(int64(len(domains))),
		Items:    items,
	}
}

func (d *Distribution) getTags(tags map[string]string) *cloudfront.Tags {
	items := []*cloudfront.Tag{}
	for key, value := range tags {
		items = append(items, &cloudfront.Tag{
			Key:   aws.String(key),
			Value: aws.String(value),
		})
	}
	return &cloudfront.Tags{Items: items}
}

func (d *Distribution) getHeaders(headers []string) *cloudfront.Headers {
	items := make([]*string, len(headers))
	for idx, header := range headers {
		items[idx] = aws.String(header)
	}
	return &cloudfront.Headers{
		Quantity: aws.Int64(int64(len(headers))),
		Items:    items,
	}
}

func (d *Distribution) setDistributionConfigDefaults(config *cloudfront.DistributionConfig, callerReference string) {
	config.CallerReference = aws.String(callerReference)
	config.Comment = aws.String("cdn route service")
	config.Enabled = aws.Bool(true)
	config.IsIPV6Enabled = aws.Bool(true)
	config.PriceClass = aws.String("PriceClass_100")
}

func (d *Distribution) setDistributionConfigCacheBehaviors(config *cloudfront.DistributionConfig, callerReference string) {
	config.CacheBehaviors = &cloudfront.CacheBehaviors{
		Quantity: aws.Int64(1),
		Items: []*cloudfront.CacheBehavior{
			{
				TargetOriginId:         aws.String(fmt.Sprintf("s3-%s-%s", d.Settings.Bucket, callerReference)),
				FieldLevelEncryptionId: aws.String(""),
				AllowedMethods: &cloudfront.AllowedMethods{
					CachedMethods: &cloudfront.CachedMethods{
						Quantity: aws.Int64(2),
						Items: []*string{
							aws.String("HEAD"),
							aws.String("GET"),
						},
					},
					Items: []*string{
						aws.String("HEAD"),
						aws.String("GET"),
					},
					Quantity: aws.Int64(2),
				},
				Compress:    aws.Bool(false),
				PathPattern: aws.String("/.well-known/acme-challenge/*"),
				ForwardedValues: &cloudfront.ForwardedValues{
					Headers: &cloudfront.Headers{
						Quantity: aws.Int64(0),
					},
					QueryString: aws.Bool(false),
					Cookies: &cloudfront.CookiePreference{
						Forward: aws.String("none"),
					},
					QueryStringCacheKeys: &cloudfront.QueryStringCacheKeys{
						Quantity: aws.Int64(0),
					},
				},
				SmoothStreaming: aws.Bool(false),
				DefaultTTL:      aws.Int64(86400),
				MinTTL:          aws.Int64(0),
				MaxTTL:          aws.Int64(31536000),
				LambdaFunctionAssociations: &cloudfront.LambdaFunctionAssociations{
					Quantity: aws.Int64(0),
				},
				TrustedSigners: &cloudfront.TrustedSigners{
					Enabled:  aws.Bool(false),
					Quantity: aws.Int64(0),
				},
				ViewerProtocolPolicy: aws.String("allow-all"),
			},
		},
	}
}

func (d *Distribution) setDistributionConfigOrigins(config *cloudfront.DistributionConfig, callerReference string) {
	config.Origins = &cloudfront.Origins{
		Quantity: aws.Int64(2),
		Items: []*cloudfront.Origin{
			{
				Id:         aws.String(callerReference),
				OriginPath: aws.String(""),
				CustomHeaders: &cloudfront.CustomHeaders{
					Quantity: aws.Int64(0),
				},
				CustomOriginConfig: &cloudfront.CustomOriginConfig{
					HTTPPort:               aws.Int64(80),
					HTTPSPort:              aws.Int64(443),
					OriginReadTimeout:      aws.Int64(60),
					OriginKeepaliveTimeout: aws.Int64(5),
					OriginSslProtocols: &cloudfront.OriginSslProtocols{
						Quantity: aws.Int64(1),
						Items: []*string{
							aws.String("TLSv1.2"),
						},
					},
				},
			},
			{
				Id:         aws.String(fmt.Sprintf("s3-%s-%s", d.Settings.Bucket, callerReference)),
				DomainName: aws.String(fmt.Sprintf("%s.s3.amazonaws.com", d.Settings.Bucket)),
				OriginPath: aws.String(""),
				CustomHeaders: &cloudfront.CustomHeaders{
					Quantity: aws.Int64(0),
				},
				S3OriginConfig: &cloudfront.S3OriginConfig{
					OriginAccessIdentity: aws.String(""),
				},
			},
		},
	}
}

func (d *Distribution) setDistributionConfigDefaultCacheBehavior(config *cloudfront.DistributionConfig, callerReference string) {
	config.DefaultCacheBehavior.TargetOriginId = aws.String(callerReference)

	config.DefaultCacheBehavior.ForwardedValues.Cookies.Forward = aws.String("all")
	config.DefaultCacheBehavior.ForwardedValues.QueryString = aws.Bool(true)
	config.DefaultCacheBehavior.ForwardedValues.QueryStringCacheKeys.Quantity = aws.Int64(0)

	config.DefaultCacheBehavior.SmoothStreaming = aws.Bool(false)
	config.DefaultCacheBehavior.MinTTL = aws.Int64(0)
	config.DefaultCacheBehavior.MaxTTL = aws.Int64(31622400)
	config.DefaultCacheBehavior.Compress = aws.Bool(false)

	config.DefaultCacheBehavior.TrustedSigners.Enabled = aws.Bool(false)
	config.DefaultCacheBehavior.TrustedSigners.Quantity = aws.Int64(0)

	config.DefaultCacheBehavior.ViewerProtocolPolicy = aws.String("redirect-to-https")

	config.DefaultCacheBehavior.AllowedMethods.CachedMethods.Quantity = aws.Int64(2)
	config.DefaultCacheBehavior.AllowedMethods.CachedMethods.Items = []*string{
		aws.String("HEAD"),
		aws.String("GET"),
	}

	config.DefaultCacheBehavior.AllowedMethods.Quantity = aws.Int64(7)
	config.DefaultCacheBehavior.AllowedMethods.Items = []*string{
		aws.String("HEAD"),
		aws.String("GET"),
		aws.String("OPTIONS"),
		aws.String("PUT"),
		aws.String("POST"),
		aws.String("PATCH"),
		aws.String("DELETE"),
	}
}

func (d *Distribution) Create(callerReference string, domains []string, origin string, defaultTTL int64, forwardedHeaders Headers, forwardCookies bool, tags map[string]string) (*cloudfront.Distribution, error) {
	distConfig := new(cloudfront.DistributionConfig)

	distConfig.DefaultCacheBehavior = &cloudfront.DefaultCacheBehavior{
		FieldLevelEncryptionId: aws.String(""),
		ForwardedValues: &cloudfront.ForwardedValues{
			Cookies:              &cloudfront.CookiePreference{},
			QueryStringCacheKeys: &cloudfront.QueryStringCacheKeys{},
		},
		LambdaFunctionAssociations: &cloudfront.LambdaFunctionAssociations{
			Quantity: aws.Int64(0),
		},
		TrustedSigners: &cloudfront.TrustedSigners{},
		AllowedMethods: &cloudfront.AllowedMethods{
			CachedMethods: &cloudfront.CachedMethods{},
		},
	}

	d.setDistributionConfigDefaults(distConfig, callerReference)
	d.setDistributionConfigCacheBehaviors(distConfig, callerReference)
	d.setDistributionConfigDefaultCacheBehavior(distConfig, callerReference)
	d.setDistributionConfigOrigins(distConfig, callerReference)

	if forwardCookies == false {
		distConfig.DefaultCacheBehavior.ForwardedValues.Cookies.Forward = aws.String("none")
	}

	distConfig.Origins.Items[0].DomainName = aws.String(origin)
	distConfig.Origins.Items[0].CustomOriginConfig.OriginProtocolPolicy = aws.String("https-only")

	distConfig.Aliases = d.getAliases(domains)

	distConfig.DefaultCacheBehavior.ForwardedValues.Headers = d.getHeaders(forwardedHeaders.Strings())
	distConfig.DefaultCacheBehavior.DefaultTTL = aws.Int64(defaultTTL)

	resp, err := d.Service.CreateDistributionWithTags(&cloudfront.CreateDistributionWithTagsInput{
		DistributionConfigWithTags: &cloudfront.DistributionConfigWithTags{
			DistributionConfig: distConfig,
			Tags:               d.getTags(tags),
		},
	})

	if err != nil {
		return &cloudfront.Distribution{}, err
	}

	return resp.Distribution, nil
}

func (d *Distribution) Update(
	distId string,
	domains *[]string,
	origin string,
	defaultTTL *int64,
	forwardedHeaders *Headers,
	forwardCookies *bool,
) (*cloudfront.Distribution, error) {
	// Get the current distribution
	dist, err := d.Service.GetDistributionConfig(&cloudfront.GetDistributionConfigInput{
		Id: aws.String(distId),
	})
	if err != nil {
		return nil, err
	}

	callerReference := *dist.DistributionConfig.CallerReference

	d.setDistributionConfigDefaults(dist.DistributionConfig, callerReference)
	d.setDistributionConfigCacheBehaviors(dist.DistributionConfig, callerReference)
	d.setDistributionConfigDefaultCacheBehavior(dist.DistributionConfig, callerReference)
	d.setDistributionConfigOrigins(dist.DistributionConfig, callerReference)

	dist.DistributionConfig.Origins.Items[0].DomainName = aws.String(origin)
	dist.DistributionConfig.Origins.Items[0].CustomOriginConfig.OriginProtocolPolicy = aws.String("https-only")

	if forwardCookies != nil {
		if *forwardCookies {
			dist.DistributionConfig.DefaultCacheBehavior.ForwardedValues.Cookies.Forward = aws.String("all")
		} else {
			dist.DistributionConfig.DefaultCacheBehavior.ForwardedValues.Cookies.Forward = aws.String("none")
		}
	}

	if domains != nil {
		dist.DistributionConfig.Aliases = d.getAliases(*domains)
	}

	if forwardedHeaders != nil {
		dist.DistributionConfig.DefaultCacheBehavior.ForwardedValues.Headers = d.getHeaders(
			(*forwardedHeaders).Strings(),
		)
	}

	if defaultTTL != nil {
		dist.DistributionConfig.DefaultCacheBehavior.DefaultTTL = aws.Int64(*defaultTTL)
	}

	// Call the UpdateDistribution function
	resp, err := d.Service.UpdateDistribution(&cloudfront.UpdateDistributionInput{
		Id:                 aws.String(distId),
		IfMatch:            dist.ETag,
		DistributionConfig: dist.DistributionConfig,
	})
	if err != nil {
		return &cloudfront.Distribution{}, err
	}
	return resp.Distribution, nil
}

func (d *Distribution) Get(distId string) (*cloudfront.Distribution, error) {
	resp, err := d.Service.GetDistribution(&cloudfront.GetDistributionInput{
		Id: aws.String(distId),
	})
	if err != nil {
		return &cloudfront.Distribution{}, err
	}
	return resp.Distribution, nil
}

func (d *Distribution) SetCertificateAndCname(distId, certId string, domains []string, acmCert bool) error {
	resp, err := d.Service.GetDistributionConfig(&cloudfront.GetDistributionConfigInput{
		Id: aws.String(distId),
	})
	if err != nil {
		return err
	}

	aliases := d.getAliases(domains)
	DistributionConfig, ETag := resp.DistributionConfig, resp.ETag
	DistributionConfig.Aliases = aliases

	if acmCert {
		DistributionConfig.ViewerCertificate.ACMCertificateArn = aws.String(certId)
		DistributionConfig.ViewerCertificate.IAMCertificateId = nil
	} else {
		DistributionConfig.ViewerCertificate.IAMCertificateId = aws.String(certId)
		DistributionConfig.ViewerCertificate.ACMCertificateArn = nil
	}

	DistributionConfig.ViewerCertificate.SSLSupportMethod = aws.String("sni-only")
	DistributionConfig.ViewerCertificate.MinimumProtocolVersion = aws.String("TLSv1.2_2018")
	DistributionConfig.ViewerCertificate.CloudFrontDefaultCertificate = aws.Bool(false)

	_, err = d.Service.UpdateDistribution(&cloudfront.UpdateDistributionInput{
		Id:                 aws.String(distId),
		IfMatch:            ETag,
		DistributionConfig: DistributionConfig,
	})

	return err
}

func (d *Distribution) Disable(distId string) error {
	resp, err := d.Service.GetDistributionConfig(&cloudfront.GetDistributionConfigInput{
		Id: aws.String(distId),
	})
	if err != nil {
		return err
	}

	DistributionConfig, ETag := resp.DistributionConfig, resp.ETag
	DistributionConfig.Enabled = aws.Bool(false)

	_, err = d.Service.UpdateDistribution(&cloudfront.UpdateDistributionInput{
		Id:                 aws.String(distId),
		IfMatch:            ETag,
		DistributionConfig: DistributionConfig,
	})

	return err
}

func (d *Distribution) Delete(distId string) (bool, error) {
	resp, err := d.Service.GetDistribution(&cloudfront.GetDistributionInput{
		Id: aws.String(distId),
	})
	if err != nil {
		return false, err
	}

	if *resp.Distribution.Status != "Deployed" {
		return false, nil
	}

	_, err = d.Service.DeleteDistribution(&cloudfront.DeleteDistributionInput{
		Id:      aws.String(distId),
		IfMatch: resp.ETag,
	})

	return err == nil, err
}

func (d *Distribution) ListDistributions(callback func(cloudfront.DistributionSummary) bool) error {
	params := &cloudfront.ListDistributionsInput{}

	return d.Service.ListDistributionsPages(params,
		func(page *cloudfront.ListDistributionsOutput, lastPage bool) bool {
			for _, v := range page.DistributionList.Items {
				// stop iteration if the callback tells us to
				if callback(*v) == false {
					return false
				}
			}

			return true
		})
}
