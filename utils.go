package utils

import (
	"fmt"
	"log"
	"net"
	"os"
	"path"
	"strings"

	"crypto"
	"crypto/rand"
	"crypto/rsa"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/xenolf/lego/acme"
)

var bucket string = os.Getenv("CDN_BUCKET")
var region string = os.Getenv("CDN_REGION")

func CreateDistribution(domain string) (id string, err error) {
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
				TargetOriginId: aws.String(fmt.Sprintf("s3-%s-%s", bucket, domain)),
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
						DomainName: aws.String(fmt.Sprintf("%s.s3.amazonaws.com", bucket)),
						Id:         aws.String(fmt.Sprintf("s3-%s-%s", bucket, domain)),
						S3OriginConfig: &cloudfront.S3OriginConfig{
							OriginAccessIdentity: aws.String(""),
						},
					},
				},
			},
		},
	}
	resp, err := svc.CreateDistribution(params)

	if err != nil {
		return "", err
	}

	return *resp.Distribution.Id, nil
}

type User struct {
	Email        string
	Registration *acme.RegistrationResource
	key          crypto.PrivateKey
}

func (u User) GetEmail() string {
	return u.Email
}

func (u User) GetRegistration() *acme.RegistrationResource {
	return u.Registration
}

func (u User) GetPrivateKey() crypto.PrivateKey {
	return u.key
}

type HTTPProvider struct{}

func (*HTTPProvider) Present(domain, token, keyAuth string) error {
	svc := s3.New(session.New(&aws.Config{Region: aws.String(region)}))

	_, err := svc.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Body:   strings.NewReader(keyAuth),
		Key:    aws.String(path.Join(".well-known", "acme-challenge", token)),
	})

	return err
}

func (*HTTPProvider) CleanUp(domain, token, keyAuth string) error {
	return nil
}

func ObtainCertificate(domain string) (certificates acme.CertificateResource, failures map[string]error) {
	keySize := 2048
	key, err := rsa.GenerateKey(rand.Reader, keySize)
	if err != nil {
		log.Fatal(err)
	}

	user := User{
		Email: os.Getenv("CDN_EMAIL"),
		key:   key,
	}
	client, err := acme.NewClient("https://acme-staging.api.letsencrypt.org/directory", &user, acme.RSA2048)

	client.SetChallengeProvider(acme.HTTP01, &HTTPProvider{})
	client.ExcludeChallenges([]acme.Challenge{acme.DNS01, acme.TLSSNI01})

	reg, err := client.Register()
	user.Registration = reg

	err = client.AgreeToTOS()

	domains := []string{domain}
	certificate, failures := client.ObtainCertificate(domains, false, nil)

	if len(failures) > 0 {
		return acme.CertificateResource{}, failures
	}

	return certificate, failures
}

func CheckCNAME(domain string) bool {
	cname, err := net.LookupCNAME(domain)
	if err != nil {
		return false
	}
	return strings.Contains(cname, ".cloudfront.net")
}

func CheckDistribution(distId string) bool {
	svc := cloudfront.New(session.New())
	resp, err := svc.GetDistribution(&cloudfront.GetDistributionInput{
		Id: aws.String(distId),
	})
	if err != nil {
		return false
	}
	return *resp.Distribution.Status == "Deployed" && *resp.Distribution.DistributionConfig.Enabled
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
	if err != nil {
		return err
	}

	return nil
}

// func main() {
// 	id, err := CreateDistribution("test.org")
// 	cert, failures := ObtainCertificate("test.org")
// }
