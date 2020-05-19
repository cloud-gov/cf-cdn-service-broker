package utils_test

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	awsSession "github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/acm"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/18F/cf-cdn-service-broker/config"
	. "github.com/18F/cf-cdn-service-broker/utils"
)

var _ = Describe("Acm", func() {
	var (
		settings     config.Settings
		session      *awsSession.Session
		fakeacm      *acm.ACM
		certsManager CertificateManager
	)

	const ArnString string = "this is arn"
	const SBInstanceID string = "abcd-erts-32432-ksjdfs"

	BeforeEach(func() {
		//Setup an input for the RequestCertificate call with a single domain as a common name
		settings, _ = config.NewSettings()
		session = awsSession.New(nil)

		//mock out the aws call to return a fixed list of certs, two of which should be deleted
		fakeacm = acm.New(session)
		fakeacm.Handlers.Clear()

		certsManager = CertificateManager{Settings: settings, Service: fakeacm}

	})

	Context("Request Certificate", func() {
		var requestCertInput *acm.RequestCertificateInput

		BeforeEach(func() {
			fakeacm.Handlers.Send.PushBack(func(r *request.Request) {

				if r.Operation.Name == "RequestCertificate" {
					requestCertInput = r.Params.(*acm.RequestCertificateInput)

					data := r.Data.(*acm.RequestCertificateOutput)
					data.CertificateArn = aws.String(ArnString)
				}

			})
		})

		It("When requesting certs for a single domain, should specify only the common name", func() {

			inputDomains := []string{"example.com"}

			//Request a cert
			_, err := certsManager.RequestCertificate(inputDomains, SBInstanceID)

			Expect(err).NotTo(HaveOccurred())
			Expect(requestCertInput).NotTo(BeNil())
			Expect(*(requestCertInput.DomainName)).To(Equal(inputDomains[0]))
			Expect(requestCertInput.SubjectAlternativeNames).To(BeNil())

		})

		It("When requesting certs for multiple domains, should specify a common name and subject alternative names", func() {

			//the first element is the DomainName
			inputDomains := []string{"example.com"}

			subjectAlternativeNames := []string{"blog.example.com", "support.example.com"}

			inputDomains = append(inputDomains, subjectAlternativeNames...)
			//Request a cert
			_, err := certsManager.RequestCertificate(inputDomains, SBInstanceID)

			Expect(err).NotTo(HaveOccurred())
			Expect(requestCertInput).NotTo(BeNil())
			Expect(*(requestCertInput.DomainName)).To(Equal(inputDomains[0]))
			Expect(requestCertInput.SubjectAlternativeNames).NotTo(BeNil())
			Expect(requestCertInput.SubjectAlternativeNames).To(HaveLen(2))
			Expect((*requestCertInput.SubjectAlternativeNames[0])).To(Equal(subjectAlternativeNames[0]))
			Expect((*requestCertInput.SubjectAlternativeNames[1])).To(Equal(subjectAlternativeNames[1]))

		})

		It("When requesting certs from ACM, make sure that it is using the DNS validation", func() {
			inputDomains := []string{"example.com"}

			//Request a cert
			_, err := certsManager.RequestCertificate(inputDomains, SBInstanceID)

			Expect(err).NotTo(HaveOccurred())
			Expect(requestCertInput).NotTo(BeNil())
			// Expect(*(requestCertInput.DomainName)).To(Equal(inputDomains[0]))
			Expect(*(requestCertInput.ValidationMethod)).To(Equal(acm.ValidationMethodDns))
		})

		It("When requesting certs from ACM, make sure that it returns a valid ARN for the ACM Certificate object", func() {
			inputDomains := []string{"example.com"}

			//Request a cert
			res, err := certsManager.RequestCertificate(inputDomains, SBInstanceID)

			Expect(err).NotTo(HaveOccurred())
			Expect(requestCertInput).NotTo(BeNil())
			Expect(res).NotTo(BeNil())
			Expect(*res).To(Equal(ArnString))

		})

		It("When requesting certs from ACM, make sure that it has the idenpotency token set to concat of all domain names sorted", func() {
			inputDomains := []string{"b", "d", "a", "c"}

			localIdempotencyToken := "a-b-c-d"

			//Request a cert
			_, err := certsManager.RequestCertificate(inputDomains, SBInstanceID)

			Expect(err).NotTo(HaveOccurred())
			Expect(requestCertInput).NotTo(BeNil())
			Expect(*(requestCertInput.IdempotencyToken)).To(Equal(localIdempotencyToken))

		})

		It("When requesting certs from ACM, make sure that it has a tag created with Service Instance ID value", func() {
			inputDomains := []string{"example.com"}

			//Request a cert
			_, err := certsManager.RequestCertificate(inputDomains, SBInstanceID)

			Expect(err).NotTo(HaveOccurred())
			Expect(requestCertInput).NotTo(BeNil())
			Expect(requestCertInput.Tags).NotTo(BeNil())
			Expect(requestCertInput.Tags).To(ContainElement(&acm.Tag{Key: aws.String("ServiceInstance"), Value: aws.String(SBInstanceID)}))

		})

	})

	Context("Delete Certificate", func() {
		var deleteCertInput *acm.DeleteCertificateInput

		BeforeEach(func() {
			fakeacm.Handlers.Send.PushBack(func(r *request.Request) {

				if r.Operation.Name == "DeleteCertificate" {
					deleteCertInput = r.Params.(*acm.DeleteCertificateInput)
				}

			})
		})

		It("After calling DeleteCertificate, make sure that the correct method was called in ACM anymore", func() {

			err := certsManager.DeleteCertificate(ArnString)

			Expect(err).NotTo(HaveOccurred())
			Expect(deleteCertInput).NotTo(BeNil())
			Expect(*(deleteCertInput.CertificateArn)).To(Equal(ArnString))

		})

	})

	Context("Is Certificate Issued", func() {
		var describeCertInput *acm.DescribeCertificateInput
		var describeCertOutput *acm.DescribeCertificateOutput
		var certDetail *acm.CertificateDetail

		BeforeEach(func() {
			certDetail = &acm.CertificateDetail{}

			fakeacm.Handlers.Send.PushBack(func(r *request.Request) {

				if r.Operation.Name == "DescribeCertificate" {
					describeCertInput = r.Params.(*acm.DescribeCertificateInput)
				}

				describeCertOutput = r.Data.(*acm.DescribeCertificateOutput)

				describeCertOutput.Certificate = certDetail

			})
		})

		It("The call to isCertificateIssued is calling AWS DescribeCertificate function", func() {

			certDetail.SetCertificateArn(ArnString)
			certDetail.SetStatus(acm.CertificateStatusIssued)

			_, err := certsManager.IsCertificateIssued(ArnString)

			Expect(err).NotTo(HaveOccurred())
			Expect(describeCertInput).NotTo(BeNil())
			Expect(*(describeCertInput.CertificateArn)).To(Equal(ArnString))
		})

		It("the call to IsCertificateIssued returns 'true' when the status of the certificate is 'ISSUED'", func() {

			certDetail.SetCertificateArn(ArnString)
			certDetail.SetStatus(acm.CertificateStatusIssued)

			res, err := certsManager.IsCertificateIssued(ArnString)

			Expect(err).NotTo(HaveOccurred())
			Expect(describeCertInput).NotTo(BeNil())
			Expect(res).To(BeTrue())
		})

		It("the call to IsCertificateIssued returns 'false' when the status of the certificate is 'PENDING_VALIDATION'", func() {

			certDetail.SetCertificateArn(ArnString)
			certDetail.SetStatus(acm.CertificateStatusPendingValidation)

			res, err := certsManager.IsCertificateIssued(ArnString)

			Expect(err).NotTo(HaveOccurred())
			Expect(describeCertInput).NotTo(BeNil())
			Expect(res).To(BeFalse())
		})

		It("when the status of the certificate is 'FAILED', we can see the failure reason as part of the returned error details", func() {

			certDetail.SetCertificateArn(ArnString)
			certDetail.SetStatus(acm.CertificateStatusFailed)
			certDetail.SetFailureReason(acm.FailureReasonAdditionalVerificationRequired)

			res, err := certsManager.IsCertificateIssued(ArnString)

			Expect(err).To(HaveOccurred())
			Expect(describeCertInput).NotTo(BeNil())
			Expect(res).To(BeFalse())
			Expect(err.Error()).To(ContainSubstring(acm.FailureReasonAdditionalVerificationRequired))
		})

		It("when the status of the certificate is 'CertificateStatusValidationTimedOut', we can see a specific error triggered", func() {

			certDetail.SetCertificateArn(ArnString)
			certDetail.SetStatus(acm.CertificateStatusValidationTimedOut)

			res, err := certsManager.IsCertificateIssued(ArnString)

			Expect(err).To(HaveOccurred())
			Expect(describeCertInput).NotTo(BeNil())
			Expect(res).To(BeFalse())
			Expect(err).To(Equal(ErrValidationTimedOut))
		})

	})

	Context("GetDomainValidationChallenges", func() {
		var describeCertInput *acm.DescribeCertificateInput
		var describeCertOutput *acm.DescribeCertificateOutput
		var certDetail *acm.CertificateDetail

		BeforeEach(func() {
			certDetail = &acm.CertificateDetail{}

			fakeacm.Handlers.Send.PushBack(func(r *request.Request) {

				if r.Operation.Name == "DescribeCertificate" {
					describeCertInput = r.Params.(*acm.DescribeCertificateInput)
				}

				describeCertOutput = r.Data.(*acm.DescribeCertificateOutput)

				describeCertOutput.Certificate = certDetail

			})
		})

		It("The call to GetDomainValidationChallenges is calling AWS DescribeCertificate function", func() {

			certDetail.SetCertificateArn(ArnString)
			certDetail.SetStatus(acm.CertificateStatusIssued)

			_, err := certsManager.GetDomainValidationChallenges(ArnString)

			Expect(err).NotTo(HaveOccurred())
			Expect(describeCertInput).NotTo(BeNil())
			Expect(*(describeCertInput.CertificateArn)).To(Equal(ArnString))
		})

		It("The call to GetDomainValidationChallenges returns DomainChallenges struct with all the fields set to valid values", func() {

			certDetail.SetCertificateArn(ArnString)
			certDetail.SetStatus(acm.CertificateStatusIssued)
			domainValidationChallenge := DomainValidationChallenge{
				DomainName:       "example.com",
				RecordName:       "_blahblahblah.example.com",
				RecordType:       acm.RecordTypeCname,
				RecordValue:      "blahblahblha",
				ValidationStatus: acm.DomainStatusSuccess,
			}

			certDetail.DomainValidationOptions = []*acm.DomainValidation{
				&acm.DomainValidation{
					DomainName:       aws.String("example.com"),
					ValidationStatus: aws.String(acm.DomainStatusSuccess),
					ResourceRecord: &acm.ResourceRecord{
						Name:  aws.String("_blahblahblah.example.com"),
						Type:  aws.String(acm.RecordTypeCname),
						Value: aws.String("blahblahblha"),
					},
				},
			}

			res, err := certsManager.GetDomainValidationChallenges(ArnString)

			Expect(err).NotTo(HaveOccurred())
			Expect(describeCertInput).NotTo(BeNil())
			Expect(*(describeCertInput.CertificateArn)).To(Equal(ArnString))
			Expect(res).NotTo(BeNil())
			Expect(res[0]).To(Equal(domainValidationChallenge))
		})

	})

})
