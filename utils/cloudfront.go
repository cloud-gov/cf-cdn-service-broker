package utils

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/aws/aws-sdk-go/service/iam"

	"github.com/xenolf/lego/acme"

	"github.com/18F/cf-cdn-service-broker/config"
)

func CreateDistribution(settings config.Settings, domain, origin string) (*cloudfront.Distribution, error) {
	svc := cloudfront.New(session.New())

	resp, err := svc.CreateDistribution(&cloudfront.CreateDistributionInput{
		DistributionConfig: &cloudfront.DistributionConfig{
			CallerReference: aws.String(fmt.Sprintf("cdn-route-%s", domain)),
			Comment:         aws.String("cdn route service"),
			Enabled:         aws.Bool(true),
			DefaultCacheBehavior: &cloudfront.DefaultCacheBehavior{
				TargetOriginId: aws.String(fmt.Sprintf("cdn-route-%s", domain)),
				ForwardedValues: &cloudfront.ForwardedValues{
					Cookies: &cloudfront.CookiePreference{
						Forward: aws.String("all"),
					},
					QueryString: aws.Bool(true),
				},
				MinTTL: aws.Int64(0),
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
				},
			},
			Origins: &cloudfront.Origins{
				Quantity: aws.Int64(2),
				Items: []*cloudfront.Origin{
					{
						DomainName: aws.String(origin),
						Id:         aws.String(fmt.Sprintf("cdn-route-%s", domain)),
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
					},
					{
						DomainName: aws.String(fmt.Sprintf("%s.s3.amazonaws.com", settings.Bucket)),
						Id:         aws.String(fmt.Sprintf("s3-%s-%s", settings.Bucket, domain)),
						S3OriginConfig: &cloudfront.S3OriginConfig{
							OriginAccessIdentity: aws.String(""),
						},
					},
				},
			},
			CacheBehaviors: &cloudfront.CacheBehaviors{
				Quantity: aws.Int64(1),
				Items: []*cloudfront.CacheBehavior{
					{
						PathPattern:    aws.String("/.well-known/acme-challenge/*"),
						TargetOriginId: aws.String(fmt.Sprintf("s3-%s-%s", settings.Bucket, domain)),
						ForwardedValues: &cloudfront.ForwardedValues{
							QueryString: aws.Bool(false),
							Cookies: &cloudfront.CookiePreference{
								Forward: aws.String("none"),
							},
						},
						MinTTL: aws.Int64(0),
						TrustedSigners: &cloudfront.TrustedSigners{
							Enabled:  aws.Bool(false),
							Quantity: aws.Int64(0),
						},
						ViewerProtocolPolicy: aws.String("allow-all"),
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
	})

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

func DeleteDistribution(domain, distId string) (bool, error) {
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

	err = deleteCert(fmt.Sprintf("cdn-route-%s", domain))
	if err != nil {
		return false, err
	}

	return err == nil, err
}

// Deploy certificate to distribution without downtime: upload to IAM with a
// "new" prefix, add to CloudFront by ID, delete existing certificate if exists,
// and rename new certificate.
func DeployCert(domain, distId string, cert acme.CertificateResource) error {
	prev := fmt.Sprintf("cdn-route-%s-new", domain)
	next := fmt.Sprintf("cdn-route-%s", domain)

	certId, err := uploadCert(prev, cert)
	if err != nil {
		return err
	}

	err = setDistributionCert(certId, distId)
	if err != nil {
		return err
	}

	return renameCert(prev, next)
}

func uploadCert(name string, cert acme.CertificateResource) (string, error) {
	svc := iam.New(session.New())

	err := deleteCert(name)
	if err != nil {
		return "", err
	}

	resp, err := svc.UploadServerCertificate(&iam.UploadServerCertificateInput{
		CertificateBody:       aws.String(string(cert.Certificate)),
		PrivateKey:            aws.String(string(cert.PrivateKey)),
		ServerCertificateName: aws.String(name),
		Path: aws.String("/cloudfront/letsencrypt/"),
	})
	if err != nil {
		return "", err
	}

	return *resp.ServerCertificateMetadata.ServerCertificateId, nil
}

func setDistributionCert(certId, distId string) error {
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
	DistributionConfig.ViewerCertificate.CloudFrontDefaultCertificate = aws.Bool(false)

	_, err = svc.UpdateDistribution(&cloudfront.UpdateDistributionInput{
		Id:                 aws.String(distId),
		IfMatch:            ETag,
		DistributionConfig: DistributionConfig,
	})

	return err
}

func renameCert(prev, next string) error {
	svc := iam.New(session.New())

	err := deleteCert(next)
	if err != nil {
		return err
	}

	_, err = svc.UpdateServerCertificate(&iam.UpdateServerCertificateInput{
		ServerCertificateName:    aws.String(prev),
		NewServerCertificateName: aws.String(next),
	})

	return err
}

func deleteCert(name string) error {
	svc := iam.New(session.New())

	_, err := svc.DeleteServerCertificate(&iam.DeleteServerCertificateInput{
		ServerCertificateName: aws.String(name),
	})

	if err != nil {
		failure := err.(awserr.RequestFailure)
		if failure.StatusCode() != 404 {
			return err
		}
	}

	return nil
}
