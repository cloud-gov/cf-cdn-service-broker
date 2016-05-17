package utils

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/aws/aws-sdk-go/service/iam"

	"github.com/xenolf/lego/acme"

	"github.com/18F/cf-cdn-service-broker/config"
)

func CreateDistribution(domain string) (*cloudfront.Distribution, error) {
	svc := cloudfront.New(session.New())

	params := &cloudfront.CreateDistributionInput{
		DistributionConfig: &cloudfront.DistributionConfig{
			CallerReference: aws.String(fmt.Sprintf("cdn-route:%s", domain)),
			Comment:         aws.String("cdn route service"),
			Enabled:         aws.Bool(true),
			DefaultCacheBehavior: &cloudfront.DefaultCacheBehavior{
				ForwardedValues: &cloudfront.ForwardedValues{
					Cookies: &cloudfront.CookiePreference{
						Forward: aws.String("none"),
					},
					QueryString: aws.Bool(false),
				},
				MinTTL:         aws.Int64(0),
				TargetOriginId: aws.String(fmt.Sprintf("s3-%s-%s", config.Bucket, domain)),
				TrustedSigners: &cloudfront.TrustedSigners{
					Enabled:  aws.Bool(false),
					Quantity: aws.Int64(0),
				},
				ViewerProtocolPolicy: aws.String("redirect-to-https"),
				AllowedMethods: &cloudfront.AllowedMethods{
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
					CachedMethods: &cloudfront.CachedMethods{
						Quantity: aws.Int64(2),
						Items: []*string{
							aws.String("GET"),
							aws.String("HEAD"),
						},
					},
				},
			},
			Origins: &cloudfront.Origins{
				Quantity: aws.Int64(1),
				Items: []*cloudfront.Origin{
					{
						DomainName: aws.String(fmt.Sprintf("%s.s3.amazonaws.com", config.Bucket)),
						Id:         aws.String(fmt.Sprintf("s3-%s-%s", config.Bucket, domain)),
						S3OriginConfig: &cloudfront.S3OriginConfig{
							OriginAccessIdentity: aws.String(""),
						},
					},
				},
			},
			Aliases: &cloudfront.Aliases{
				Quantity: aws.Int64(1),
				Items: []*string{
					aws.String(domain),
				},
			},
		},
	}
	resp, err := svc.CreateDistribution(params)

	if err != nil {
		return &cloudfront.Distribution{}, err
	}

	return resp.Distribution, nil
}

func DisableDistribution(distId string) error {
	svc := cloudfront.New(session.New())

	resp, err := svc.GetDistributionConfig(&cloudfront.GetDistributionConfigInput{
		Id: aws.String(distId),
	})
	if err != nil {
		return err
	}

	DistributionConfig, ETag := resp.DistributionConfig, resp.ETag
	DistributionConfig.Enabled = aws.Bool(false)

	_, err = svc.UpdateDistribution(&cloudfront.UpdateDistributionInput{
		Id:                 aws.String(distId),
		IfMatch:            ETag,
		DistributionConfig: DistributionConfig,
	})
	return err
}

func DeleteDistribution(distId string) (bool, error) {
	svc := cloudfront.New(session.New())

	resp, err := svc.GetDistribution(&cloudfront.GetDistributionInput{
		Id: aws.String(distId),
	})
	if err != nil {
		return false, err
	}

	if *resp.Distribution.Status != "Deployed" {
		return false, nil
	}

	_, err = svc.DeleteDistribution(&cloudfront.DeleteDistributionInput{
		Id:      aws.String(distId),
		IfMatch: resp.ETag,
	})
	return err == nil, err
}

func UploadCert(domain string, cert acme.CertificateResource) (id string, err error) {
	svc := iam.New(session.New())

	resp, err := svc.UploadServerCertificate(&iam.UploadServerCertificateInput{
		CertificateBody:       aws.String(string(cert.Certificate)),
		PrivateKey:            aws.String(string(cert.PrivateKey)),
		ServerCertificateName: aws.String(fmt.Sprintf("cdn-route-%s", domain)),
		Path: aws.String("/cloudfront/letsencrypt"),
	})
	if err != nil {
		return "", err
	}
	return *resp.ServerCertificateMetadata.ServerCertificateId, nil
}

func DeployCert(certId, distId string) error {
	svc := cloudfront.New(session.New())

	resp, err := svc.GetDistributionConfig(&cloudfront.GetDistributionConfigInput{
		Id: aws.String(distId),
	})
	if err != nil {
		return err
	}

	DistributionConfig, ETag := resp.DistributionConfig, resp.ETag

	DistributionConfig.ViewerCertificate.Certificate = aws.String(certId)
	DistributionConfig.ViewerCertificate.IAMCertificateId = aws.String(certId)
	DistributionConfig.ViewerCertificate.CertificateSource = aws.String("iam")
	DistributionConfig.ViewerCertificate.SSLSupportMethod = aws.String("sni-only")
	DistributionConfig.ViewerCertificate.MinimumProtocolVersion = aws.String("TLSv1")

	_, err = svc.UpdateDistribution(&cloudfront.UpdateDistributionInput{
		Id:                 aws.String(distId),
		IfMatch:            ETag,
		DistributionConfig: DistributionConfig,
	})
	return err
}

func BindHTTPOrigin(distId, domain string) error {
	svc := cloudfront.New(session.New())

	resp, err := svc.GetDistributionConfig(&cloudfront.GetDistributionConfigInput{
		Id: aws.String(distId),
	})
	if err != nil {
		return err
	}

	DistributionConfig, ETag := resp.DistributionConfig, resp.ETag
	Origins := DistributionConfig.Origins

	origin := &cloudfront.Origin{
		DomainName: aws.String(domain),
		Id:         aws.String(fmt.Sprintf("cdn-route:%s", domain)),
		OriginPath: aws.String(""),
		CustomHeaders: &cloudfront.CustomHeaders{
			Quantity: aws.Int64(0),
		},
		CustomOriginConfig: &cloudfront.CustomOriginConfig{
			HTTPPort:             aws.Int64(80),
			HTTPSPort:            aws.Int64(443),
			OriginProtocolPolicy: aws.String("https-only"),
			OriginSslProtocols: &cloudfront.OriginSslProtocols{
				Quantity: aws.Int64(3),
				Items: []*string{
					aws.String("TLSv1"),
					aws.String("TLSv1.1"),
					aws.String("TLSv1.2"),
				},
			},
		},
	}

	Origins.Quantity = aws.Int64(*Origins.Quantity + 1)
	Origins.Items = append(Origins.Items, origin)

	_, err = svc.UpdateDistribution(&cloudfront.UpdateDistributionInput{
		Id:                 aws.String(distId),
		IfMatch:            ETag,
		DistributionConfig: DistributionConfig,
	})
	return err
}

func UnbindHTTPOrigin(distId, domain string) error {
	svc := cloudfront.New(session.New())

	resp, err := svc.GetDistributionConfig(&cloudfront.GetDistributionConfigInput{
		Id: aws.String(distId),
	})
	if err != nil {
		return err
	}

	DistributionConfig, ETag := resp.DistributionConfig, resp.ETag
	Origins := DistributionConfig.Origins

	var items []*cloudfront.Origin
	for _, origin := range Origins.Items {
		if *origin.Id != fmt.Sprintf("cdn-route:%s", domain) {
			items = append(items, origin)
		}
	}

	Origins.Quantity = aws.Int64(int64(len(items)))
	Origins.Items = items

	_, err = svc.UpdateDistribution(&cloudfront.UpdateDistributionInput{
		Id:                 aws.String(distId),
		IfMatch:            ETag,
		DistributionConfig: DistributionConfig,
	})
	return err
}
