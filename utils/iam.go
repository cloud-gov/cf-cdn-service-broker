package utils

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/iam"

	"github.com/xenolf/lego/acme"
)

type IamIface interface {
	UploadCertificate(name string, cert acme.CertificateResource) (string, error)
	RenameCertificate(prev, next string) error
	DeleteCertificate(name string) error
}

type Iam struct {
	Service *iam.IAM
}

func (i *Iam) UploadCertificate(name string, cert acme.CertificateResource) (string, error) {
	resp, err := i.Service.UploadServerCertificate(&iam.UploadServerCertificateInput{
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

func (i *Iam) RenameCertificate(prev, next string) error {
	err := i.DeleteCertificate(next)
	if err != nil {
		return err
	}

	_, err = i.Service.UpdateServerCertificate(&iam.UpdateServerCertificateInput{
		ServerCertificateName:    aws.String(prev),
		NewServerCertificateName: aws.String(next),
	})

	return err
}

func (i *Iam) DeleteCertificate(name string) error {
	_, err := i.Service.DeleteServerCertificate(&iam.DeleteServerCertificateInput{
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
