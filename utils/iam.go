package utils

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/iam"

	"github.com/xenolf/lego/acme"

	"github.com/18F/cf-cdn-service-broker/config"
)

type IamIface interface {
	UploadCertificate(name string, cert acme.CertificateResource) (string, error)
	RenameCertificate(prev, next string) error
	DeleteCertificate(name string, allowError bool) error
}

type Iam struct {
	Settings config.Settings
	Service  *iam.IAM
}

func (i *Iam) UploadCertificate(name string, cert acme.CertificateResource) (string, error) {
	resp, err := i.Service.UploadServerCertificate(&iam.UploadServerCertificateInput{
		CertificateBody:       aws.String(string(cert.Certificate)),
		PrivateKey:            aws.String(string(cert.PrivateKey)),
		ServerCertificateName: aws.String(name),
		Path: aws.String(fmt.Sprintf("/cloudfront/%s/", i.Settings.IamPathPrefix)),
	})
	if err != nil {
		return "", err
	}

	return *resp.ServerCertificateMetadata.ServerCertificateId, nil
}

func (i *Iam) RenameCertificate(prev, next string) error {
	err := i.DeleteCertificate(next, true)
	if err != nil {
		return err
	}

	_, err = i.Service.UpdateServerCertificate(&iam.UpdateServerCertificateInput{
		ServerCertificateName:    aws.String(prev),
		NewServerCertificateName: aws.String(next),
	})

	return err
}

func (i *Iam) DeleteCertificate(name string, allowError bool) error {
	_, err := i.Service.DeleteServerCertificate(&iam.DeleteServerCertificateInput{
		ServerCertificateName: aws.String(name),
	})

	// If caller passes `allowError`, ignore 403 and 404 errors;
	// deleting a non-existing certificate may throw either error
	// depending on permissions.
	if err != nil && allowError {
		code := err.(awserr.RequestFailure).StatusCode()
		if code != 403 && code != 404 {
			return err
		}
	}

	return nil
}
