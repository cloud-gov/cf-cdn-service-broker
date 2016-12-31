package iamcerts

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/iam"

	"code.cloudfoundry.org/lager"
	"github.com/xenolf/lego/acme"

	"github.com/18F/cf-cdn-service-broker/config"
)

type IAMCerts struct {
	settings config.Settings
	service  *iam.IAM
	logger   lager.Logger
}

func NewIAMCerts(settings config.Settings, service *iam.IAM, logger lager.Logger) *IAMCerts {
	return &IAMCerts{
		settings: settings,
		service:  service,
		logger:   logger.Session("iam-certs"),
	}
}

func (i *IAMCerts) UploadCertificate(name string, cert acme.CertificateResource) (string, error) {
	input := &iam.UploadServerCertificateInput{
		CertificateBody:       aws.String(string(cert.Certificate)),
		PrivateKey:            aws.String(string(cert.PrivateKey)),
		ServerCertificateName: aws.String(name),
		Path: aws.String(fmt.Sprintf("/cloudfront/%s/", i.settings.IamPathPrefix)),
	}
	i.logger.Debug("upload-cert", lager.Data{"input": input})
	resp, err := i.service.UploadServerCertificate(input)

	if err != nil {
		i.logger.Error("aws-iam-error", err)
		return "", err
	}

	return *resp.ServerCertificateMetadata.ServerCertificateId, nil
}

func (i *IAMCerts) RenameCertificate(prev, next string) error {
	err := i.DeleteCertificate(next, true)
	if err != nil {
		return err
	}

	input := &iam.UpdateServerCertificateInput{
		ServerCertificateName:    aws.String(prev),
		NewServerCertificateName: aws.String(next),
	}
	i.logger.Debug("update-cert", lager.Data{"input": input})
	_, err = i.service.UpdateServerCertificate(input)
	if err != nil {
		i.logger.Error("aws-iam-error", err)
	}

	return err
}

func (i *IAMCerts) DeleteCertificate(name string, allowError bool) error {
	input := &iam.DeleteServerCertificateInput{
		ServerCertificateName: aws.String(name),
	}
	i.logger.Debug("delete-cert", lager.Data{"input": input})
	_, err := i.service.DeleteServerCertificate(input)

	// If caller passes `allowError`, ignore 403 and 404 errors;
	// deleting a non-existing certificate may throw either error
	// depending on permissions.
	if err != nil && allowError {
		code := err.(awserr.RequestFailure).StatusCode()
		if code != 403 && code != 404 {
			i.logger.Error("aws-iam-error", err)
			return err
		}
	}

	return nil
}
