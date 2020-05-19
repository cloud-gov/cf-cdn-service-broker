package utils

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/18F/cf-cdn-service-broker/config"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/acm"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate -o mocks/FakeCertificateManager.go --fake-name FakeCertificateManager acm.go CertificateManagerInterface

type CertificateManagerInterface interface {
	//the original is func (c *ACM) RequestCertificate(input *RequestCertificateInput) (*RequestCertificateOutput, error)
	//we only need to supply the DomainName and SubjectAlternativeNames, everything else can be derived from these.
	//we will be using only DNS validation
	//CertificateArn - *string
	RequestCertificate(ds []string, instanceID string) (*string, error)

	DeleteCertificate(arn string) error

	IsCertificateIssued(arn string) (bool, error)

	GetDomainValidationChallenges(arn string) ([]DomainValidationChallenge, error)
}

type DomainValidationChallenge struct {
	DomainName string

	RecordName string
	// RecordType can be only of CNAME type
	// const acm.RecordTypeCname represents that
	RecordType string

	RecordValue string
	// The validation status of the domain name. This can be one of the following
	// values:
	//
	//    * PENDING_VALIDATION
	//
	//    * SUCCESS
	//
	//    * FAILED
	ValidationStatus string
}

type CertificateManager struct {
	Settings config.Settings
	Service  *acm.ACM
}

func (cm *CertificateManager) RequestCertificate(ds []string, instanceID string) (*string, error) {

	if len(ds) == 0 {
		return nil, errors.New("the domain can't be empty")
	}

	domainName := ds[0]
	var subjectAlternativeNames []*string

	if len(ds) > 1 {
		for _, e := range ds[1:] {
			subjectAlternativeNames = append(subjectAlternativeNames, aws.String(e))
		}
	}

	idempotencyToken := createIdempotencyToken(ds)

	requestCertInput := acm.RequestCertificateInput{
		DomainName:              aws.String(domainName),
		SubjectAlternativeNames: subjectAlternativeNames,
		ValidationMethod:        aws.String(acm.ValidationMethodDns),
		IdempotencyToken:        aws.String(idempotencyToken),
		Tags: []*acm.Tag{
			&acm.Tag{
				Key:   aws.String("ServiceInstance"),
				Value: aws.String(instanceID),
			},
		},
	}

	res, err := cm.Service.RequestCertificate(&requestCertInput)

	if err != nil {
		return nil, err
	}

	return res.CertificateArn, err
}

func createIdempotencyToken(s []string) string {
	var copy []string
	copy = append([]string{}, s...)
	sort.Strings(copy)
	return strings.Join(copy, "-")
}

func (cm *CertificateManager) DeleteCertificate(arn string) error {
	deleteCertificateInput := acm.DeleteCertificateInput{
		CertificateArn: &arn,
	}

	_, err := cm.Service.DeleteCertificate(&deleteCertificateInput)
	//We do not process errors from DeleteCertificate for now, so just forwarding it up
	return err
}

// ErrValidationTimedOut is the error that we return when the validation of the certificate
// has timed out, no further explanation is offered by the ACM API.
var ErrValidationTimedOut = errors.New("validation timed out")

func (cm *CertificateManager) IsCertificateIssued(arn string) (bool, error) {
	describeCertificateInput := acm.DescribeCertificateInput{
		CertificateArn: &arn,
	}

	res, err := cm.Service.DescribeCertificate(&describeCertificateInput)

	if err != nil {
		return false, err
	}

	switch *(res.Certificate.Status) {
	case acm.CertificateStatusFailed:
		return false, fmt.Errorf("the certificate issue has failed, due to %s", *res.Certificate.FailureReason)

	case acm.CertificateStatusInactive,
		acm.CertificateStatusExpired,
		acm.CertificateStatusRevoked:
		return false, fmt.Errorf("the certificate status is %s", *res.Certificate.Status)

	case acm.CertificateStatusValidationTimedOut:
		return false, ErrValidationTimedOut

	case acm.CertificateStatusPendingValidation:
		return false, nil

	case acm.CertificateStatusIssued:
		return true, nil

	default:
		return false, fmt.Errorf("unknown status %s", *res.Certificate.Status)

	}
}

func (cm *CertificateManager) GetDomainValidationChallenges(arn string) ([]DomainValidationChallenge, error) {
	describeCertificateInput := acm.DescribeCertificateInput{
		CertificateArn: &arn,
	}

	domainValidationChallenges := []DomainValidationChallenge{}

	res, err := cm.Service.DescribeCertificate(&describeCertificateInput)

	if err != nil {
		//in case of an error, return the empty slice and the error itself
		return domainValidationChallenges, err
	}

	for _, e := range res.Certificate.DomainValidationOptions {
		if e == nil {
			continue
		}

		domainValidationChallengeElement := DomainValidationChallenge{
			DomainName:       *e.DomainName,
			ValidationStatus: *e.ValidationStatus,
			RecordName:       *e.ResourceRecord.Name,
			RecordType:       *e.ResourceRecord.Type,
			RecordValue:      *e.ResourceRecord.Value,
		}

		domainValidationChallenges = append(domainValidationChallenges, domainValidationChallengeElement)
	}

	return domainValidationChallenges, nil
}
