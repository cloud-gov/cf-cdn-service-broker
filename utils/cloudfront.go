package utils

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudfront"

	"github.com/18F/cf-cdn-service-broker/config"
)

type DistributionIface interface {
	Create(callerReference string, domains []string, origin, path string, defaultTTL int64, insecureOrigin bool, forwardedHeaders Headers, forwardCookies bool, tags map[string]string) (*cloudfront.Distribution, error)
	Update(distId string, domains []string, origin, path string, defaultTTL int64, insecureOrigin bool, forwardedHeaders Headers, forwardCookies bool) (*cloudfront.Distribution, error)
	Get(distId string) (*cloudfront.Distribution, error)
	SetCertificateAndCname(distId, certId string, domains []string) error
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

// fillDistributionConfig is a wrapper function that will get all the common config settings for
// "cloudfront.DistributionConfig". This function is shared between "Create" and "Update".
// In order to maintain backwards compatibility with older versions of the code where the callerReference was derived
// from the domain(s), the callerReference has to be explicitly passed in. This is necessary because whenever we do an
// update, the domains could change but we need to treat the CallerReference like an ID because
// it can't be changed like the domains and instead the callerReference which was composed of the original domains must
// be passed in.
func (d *Distribution) fillDistributionConfig(config *cloudfront.DistributionConfig, origin, path string,
	insecureOrigin bool, callerReference *string, domains []string, forwardedHeaders []string, forwardCookies bool,
	defaultTTL int64) {
	config.CallerReference = callerReference
	config.Comment = aws.String("cdn route service")
	config.Enabled = aws.Bool(true)
	config.IsIPV6Enabled = aws.Bool(true)

	cookies := aws.String("all")
	if forwardCookies == false {
		cookies = aws.String("none")
	}

	config.DefaultCacheBehavior = &cloudfront.DefaultCacheBehavior{
		TargetOriginId: aws.String(*callerReference),
		ForwardedValues: &cloudfront.ForwardedValues{
			Headers: d.getHeaders(forwardedHeaders),
			Cookies: &cloudfront.CookiePreference{
				Forward: cookies,
			},
			QueryString: aws.Bool(true),
			QueryStringCacheKeys: &cloudfront.QueryStringCacheKeys{
				Quantity: aws.Int64(0),
			},
		},
		SmoothStreaming: aws.Bool(false),
		DefaultTTL:      aws.Int64(defaultTTL),
		MinTTL:          aws.Int64(0),
		MaxTTL:          aws.Int64(31622400),
		LambdaFunctionAssociations: &cloudfront.LambdaFunctionAssociations{
			Quantity: aws.Int64(0),
		},
		TrustedSigners: &cloudfront.TrustedSigners{
			Enabled:  aws.Bool(false),
			Quantity: aws.Int64(0),
		},
		ViewerProtocolPolicy: aws.String("redirect-to-https"),
		AllowedMethods: &cloudfront.AllowedMethods{
			CachedMethods: &cloudfront.CachedMethods{
				Quantity: aws.Int64(2),
				Items: []*string{
					aws.String("HEAD"),
					aws.String("GET"),
				},
			},
			Quantity: aws.Int64(7),
			Items: []*string{
				aws.String("HEAD"),
				aws.String("GET"),
				aws.String("OPTIONS"),
				aws.String("PUT"),
				aws.String("POST"),
				aws.String("PATCH"),
				aws.String("DELETE"),
			},
		},
		Compress: aws.Bool(false),
	}
	config.Origins = &cloudfront.Origins{
		Quantity: aws.Int64(2),
		Items: []*cloudfront.Origin{
			{
				DomainName: aws.String(origin),
				Id:         aws.String(*callerReference),
				OriginPath: aws.String(path),
				CustomHeaders: &cloudfront.CustomHeaders{
					Quantity: aws.Int64(0),
				},
				CustomOriginConfig: &cloudfront.CustomOriginConfig{
					HTTPPort:               aws.Int64(80),
					HTTPSPort:              aws.Int64(443),
					OriginReadTimeout:      aws.Int64(60),
					OriginKeepaliveTimeout: aws.Int64(5),
					OriginProtocolPolicy:   getOriginProtocolPolicy(insecureOrigin),
					OriginSslProtocols: &cloudfront.OriginSslProtocols{
						Quantity: aws.Int64(1),
						Items: []*string{
							aws.String("TLSv1.2"),
						},
					},
				},
			},
			{
				DomainName: aws.String(fmt.Sprintf("%s.s3.amazonaws.com", d.Settings.Bucket)),
				Id:         aws.String(fmt.Sprintf("s3-%s-%s", d.Settings.Bucket, *callerReference)),
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
	config.CacheBehaviors = &cloudfront.CacheBehaviors{
		Quantity: aws.Int64(1),
		Items: []*cloudfront.CacheBehavior{
			{
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
				Compress:       aws.Bool(false),
				PathPattern:    aws.String("/.well-known/acme-challenge/*"),
				TargetOriginId: aws.String(fmt.Sprintf("s3-%s-%s", d.Settings.Bucket, *callerReference)),
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
	config.Aliases = d.getAliases(domains)
	config.PriceClass = aws.String("PriceClass_100")
}

func (d *Distribution) Create(callerReference string, domains []string, origin, path string, defaultTTL int64, insecureOrigin bool, forwardedHeaders Headers, forwardCookies bool, tags map[string]string) (*cloudfront.Distribution, error) {
	distConfig := new(cloudfront.DistributionConfig)
	d.fillDistributionConfig(distConfig, origin, path, insecureOrigin,
		aws.String(callerReference), domains, forwardedHeaders.Strings(), forwardCookies,
		defaultTTL)
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

func (d *Distribution) Update(distId string, domains []string, origin, path string, defaultTTL int64, insecureOrigin bool, forwardedHeaders Headers, forwardCookies bool) (*cloudfront.Distribution, error) {
	// Get the current distribution
	dist, err := d.Service.GetDistributionConfig(&cloudfront.GetDistributionConfigInput{
		Id: aws.String(distId),
	})
	if err != nil {
		return nil, err
	}
	d.fillDistributionConfig(dist.DistributionConfig, origin, path, insecureOrigin,
		dist.DistributionConfig.CallerReference, domains, forwardedHeaders.Strings(), forwardCookies,
		defaultTTL)

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

func (d *Distribution) SetCertificateAndCname(distId, certId string, domains []string) error {
	resp, err := d.Service.GetDistributionConfig(&cloudfront.GetDistributionConfigInput{
		Id: aws.String(distId),
	})
	if err != nil {
		return err
	}

	aliases := d.getAliases(domains)
	DistributionConfig, ETag := resp.DistributionConfig, resp.ETag
	DistributionConfig.Aliases = aliases

	DistributionConfig.ViewerCertificate.Certificate = aws.String(certId)
	DistributionConfig.ViewerCertificate.IAMCertificateId = aws.String(certId)
	DistributionConfig.ViewerCertificate.CertificateSource = aws.String("iam")
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

func getOriginProtocolPolicy(insecure bool) *string {
	if insecure {
		return aws.String("http-only")
	}
	return aws.String("https-only")
}
