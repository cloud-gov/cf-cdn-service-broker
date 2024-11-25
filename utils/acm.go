package utils

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"code.cloudfoundry.org/lager/v3"
	"github.com/alphagov/paas-cdn-broker/config"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
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

	ListIssuedCertificates() ([]CertificateDetails, error)
}

type CertificateDetails struct {
	CertificateArn *string
	Status         *string
	InUseBy        []*string
	IssuedAt       *time.Time
	Tags           []*acm.Tag
}

type DomainValidationChallenge struct {
	DomainName string `json:"validating_domain_name"`

	RecordName string `json:"challenge_dns_record"`
	// RecordType can be only of CNAME type
	// const acm.RecordTypeCname represents that
	RecordType string `json:"challenges_dns_record_type"`

	RecordValue string `json:"challenges_dns_record_value"`
	// The validation status of the domain name. This can be one of the following
	// values:
	//
	//    * PENDING_VALIDATION
	//
	//    * SUCCESS
	//
	//    * FAILED
	ValidationStatus string `json:"status"`
}

type CertificateManager struct {
	Logger   lager.Logger
	Settings config.Settings
	Service  *acm.ACM
}

const CertificateTagName string = "ServiceInstance"
const ManagedByTagName string = "ManagedBy"
const ManagedByTagValue string = "cdn-broker"

// ErrValidationTimedOut is the error that we return when the validation of the certificate
// has timed out, no further explanation is offered by the ACM API.
var ErrValidationTimedOut = errors.New("validation timed out")

// NewCertificateManager retruns the NewCertificateManagerInterface,
// forceing ACM to be in Virginia (us-east-1) region, becuase CloudFront only supports reading certs from
// that region ONLY
// for more details - https://docs.aws.amazon.com/cloudfront/latest/APIReference/API_ViewerCertificate.html#cloudfront-Type-ViewerCertificate-ACMCertificateArn
func NewCertificateManager(logger lager.Logger, settings config.Settings, session *session.Session) CertificateManagerInterface {

	copySession := session.Copy()

	//Setting to Virginia region
	copySession.Config.WithRegion("us-east-1")

	return &CertificateManager{Logger: logger.Session("certificate-manager"), Settings: settings, Service: acm.New(copySession)}
}

func (cm *CertificateManager) RequestCertificate(ds []string, instanceID string) (*string, error) {
	lsession := cm.Logger.Session("request-certificate")

	if len(ds) == 0 {
		err := errors.New("the domain can't be empty")
		lsession.Error("domains-empty", err)
		return nil, err
	}

	domainName := ds[0]
	var subjectAlternativeNames []*string

	if len(ds) > 1 {
		for _, e := range ds[1:] {
			subjectAlternativeNames = append(subjectAlternativeNames, aws.String(e))
		}
	}

	lsession.Info("domains", lager.Data{"common-name": domainName, "subject-alternative-names": subjectAlternativeNames})

	idempotencyToken := createIdempotencyToken(ds)

	lsession.Info("request-certificate-from-acm")
	requestCertInput := acm.RequestCertificateInput{
		DomainName:              aws.String(domainName),
		SubjectAlternativeNames: subjectAlternativeNames,
		ValidationMethod:        aws.String(acm.ValidationMethodDns),
		IdempotencyToken:        aws.String(idempotencyToken),
		Tags: []*acm.Tag{
			&acm.Tag{
				Key:   aws.String(CertificateTagName),
				Value: aws.String(instanceID),
			},
			&acm.Tag{
				Key:   aws.String(ManagedByTagName),
				Value: aws.String(ManagedByTagValue),
			},
		},
	}

	res, err := cm.Service.RequestCertificate(&requestCertInput)

	if err != nil {
		lsession.Error("request-certificate-from-acm", err)
		return nil, err
	}

	return res.CertificateArn, err
}

func (cm *CertificateManager) DeleteCertificate(arn string) error {
	lsession := cm.Logger.Session("delete-certificate", lager.Data{"certificate-arn": arn})
	deleteCertificateInput := acm.DeleteCertificateInput{
		CertificateArn: &arn,
	}

	lsession.Info("delete-certificate-in-acm")
	_, err := cm.Service.DeleteCertificate(&deleteCertificateInput)

	if err != nil {
		lsession.Error("delete-certificate-in-acm", err)
		return err
	}

	return nil
}

func (cm *CertificateManager) IsCertificateIssued(arn string) (bool, error) {
	lsession := cm.Logger.Session("is-certificate-issued", lager.Data{"certificate-arn": arn})
	describeCertificateInput := acm.DescribeCertificateInput{
		CertificateArn: &arn,
	}

	lsession.Info("describe-certificate")
	res, err := cm.Service.DescribeCertificate(&describeCertificateInput)

	if err != nil {
		lsession.Error("describe-certificate", err)
		return false, err
	}

	lsession.Info("certificate-status", lager.Data{"status": *(res.Certificate.Status)})
	switch *(res.Certificate.Status) {
	case acm.CertificateStatusFailed:
		lsession.Info("certificate-failure", lager.Data{"reason": *res.Certificate.FailureReason})
		return false, fmt.Errorf("the certificate issue has failed, due to %s", *res.Certificate.FailureReason)

	case acm.CertificateStatusInactive,
		acm.CertificateStatusExpired,
		acm.CertificateStatusRevoked:
		return false, fmt.Errorf("the certificate status is %s", *res.Certificate.Status)

	case acm.CertificateStatusValidationTimedOut:
		lsession.Info("certificate-validation-timeout")
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
	lsession := cm.Logger.Session("get-domain-validation-challenges", lager.Data{"certificate-arn": arn})
	lsession.Info("start")

	describeCertificateInput := acm.DescribeCertificateInput{
		CertificateArn: &arn,
	}

	domainValidationChallenges := []DomainValidationChallenge{}

	lsession.Info("acm-describe-certificate")
	res, err := cm.Service.DescribeCertificate(&describeCertificateInput)

	if err != nil {
		lsession.Error("acm-describe-certificate", err)
		return domainValidationChallenges, err
	}

	for _, e := range res.Certificate.DomainValidationOptions {
		if e == nil {
			continue
		}

		var recordName = ""
		var recordType = ""
		var recordValue = ""

		if e.ResourceRecord != nil {
			recordName = *e.ResourceRecord.Name
			recordType = *e.ResourceRecord.Type
			recordValue = *e.ResourceRecord.Value
		}

		domainValidationChallengeElement := DomainValidationChallenge{
			DomainName:       *e.DomainName,
			ValidationStatus: *e.ValidationStatus,
			RecordName:       recordName,
			RecordType:       recordType,
			RecordValue:      recordValue,
		}

		domainValidationChallenges = append(domainValidationChallenges, domainValidationChallengeElement)
	}

	lsession.Info("finish")
	return domainValidationChallenges, nil
}

func (cm *CertificateManager) ListIssuedCertificates() ([]CertificateDetails, error) {
	lsession := cm.Logger.Session("list-issued-certificates")

	lsession.Info("start")

	certsDetails := []CertificateDetails{}

	input := acm.ListCertificatesInput{
		CertificateStatuses: aws.StringSlice([]string{acm.CertificateStatusIssued}),
	}

	lsession.Info("acm-list-certificates")
	listCertsOutput, err := cm.Service.ListCertificates(&input)

	if err != nil {
		lsession.Error("acm-list-certificates", err)
		return []CertificateDetails{}, err
	}

	for _, e := range listCertsOutput.CertificateSummaryList {

		describeCertificateInput := acm.DescribeCertificateInput{
			CertificateArn: e.CertificateArn,
		}

		lsession.Info("acm-describe-certificate", lager.Data{
			"certificate-arn": *(describeCertificateInput.CertificateArn),
		})
		describeCertOutput, err := cm.Service.DescribeCertificate(&describeCertificateInput)

		if err != nil {
			lsession.Error("acm-describe-certificate", err)
			return []CertificateDetails{}, err
		}

		lsession.Info("acm-list-tags-for-certificate", lager.Data{
			"certificate-arn": *(describeCertOutput.Certificate.CertificateArn),
		})
		listTagsForCertsOutput, err := cm.Service.ListTagsForCertificate(&acm.ListTagsForCertificateInput{CertificateArn: describeCertOutput.Certificate.CertificateArn})

		if err != nil {
			lsession.Error("acm-list-tags-for-certificate", err)
			return []CertificateDetails{}, err
		}

		certsDetails = append(certsDetails, CertificateDetails{
			CertificateArn: describeCertOutput.Certificate.CertificateArn,
			Status:         describeCertOutput.Certificate.Status,
			InUseBy:        describeCertOutput.Certificate.InUseBy,
			IssuedAt:       describeCertOutput.Certificate.IssuedAt,
			Tags:           listTagsForCertsOutput.Tags,
		})
	}

	lsession.Info("finish")
	return certsDetails, nil
}

func createIdempotencyToken(s []string) string {
	var cp []string
	cp = append([]string{}, s...)
	sort.Strings(cp)
	hashInput := strings.Join(cp, "-")

	// We take the hash of the value in order to get a unique (enough)
	// value that fits the ACM API's "\w+" regex validation pattern
	sha := sha1.New()
	sha.Write([]byte(hashInput))
	bytes := sha.Sum(nil)
	hashOutput := fmt.Sprintf("%x", bytes)

	// The ACM API has a hard limit of 32 characters for the idempotency token
	return hashOutput[:32]
}
