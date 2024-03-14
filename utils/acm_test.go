package utils_test

import (
	"code.cloudfoundry.org/lager/v3"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	awsSession "github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/acm"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/alphagov/paas-cdn-broker/config"
	"github.com/alphagov/paas-cdn-broker/utils"
	. "github.com/alphagov/paas-cdn-broker/utils"
)

var _ = Describe("Acm", func() {
	var (
		settings     config.Settings
		fakeacm      *acm.ACM
		certsManager CertificateManager
	)

	const ArnString string = "this is arn"
	const SBInstanceID string = "abcd-erts-32432-ksjdfs"

	BeforeEach(func() {
		//Setup an input for the RequestCertificate call with a single domain as a common name
		session, err := awsSession.NewSession(nil)
		Expect(err).NotTo(HaveOccurred())

		//mock out the aws call to return a fixed list of certs, two of which should be deleted
		fakeacm = acm.New(session)
		fakeacm.Handlers.Clear()

		logger := lager.NewLogger("test")

		logger.RegisterSink(lager.NewWriterSink(GinkgoWriter, lager.INFO))

		certsManager = CertificateManager{Logger: logger, Settings: settings, Service: fakeacm}

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

		Context("when the domain's validation challenges have not been set by AWS yet", func() {
			It("sets the challenge values to the empty string", func() {
				certDetail.SetCertificateArn(ArnString)
				certDetail.SetStatus(acm.CertificateStatusIssued)

				certDetail.DomainValidationOptions = []*acm.DomainValidation{
					&acm.DomainValidation{
						DomainName:       aws.String("example.com"),
						ValidationStatus: aws.String(acm.DomainStatusSuccess),
						ResourceRecord:   nil,
					},
				}

				res, err := certsManager.GetDomainValidationChallenges(ArnString)

				Expect(err).NotTo(HaveOccurred())
				Expect(describeCertInput).NotTo(BeNil())
				Expect(*(describeCertInput.CertificateArn)).To(Equal(ArnString))
				Expect(res).NotTo(BeNil())
				Expect(res[0].RecordName).To(BeEmpty())
				Expect(res[0].RecordType).To(BeEmpty())
				Expect(res[0].RecordValue).To(BeEmpty())
			})
		})

	})

	Context("ListIssuedCertificates", func() {

		It("Looks up a full description of each certificate in the ISSUED state", func() {

			var listCertInput *acm.ListCertificatesInput
			var listCertOutput *acm.ListCertificatesOutput
			var listCertCallCount = 0

			var describeCertInput *acm.DescribeCertificateInput
			var describeCertOutput *acm.DescribeCertificateOutput
			var describeCertCallCount = 0

			var listTagsForCertInput *acm.ListTagsForCertificateInput
			var listTagsForCertOuput *acm.ListTagsForCertificateOutput
			var listTagsForCertCallCount = 0

			var certDescriptions map[string]acm.CertificateDetail

			certDescriptions = map[string]acm.CertificateDetail{}

			const IssuedCertificateArn1 string = "IssuedCertificateArn1"
			const IssuedCertificateArn2 string = "IssuedCertificateArn2"

			var tags map[string][]*acm.Tag
			tags = map[string][]*acm.Tag{}

			fakeacm.Handlers.Send.PushBack(func(r *request.Request) {
				if r.Operation.Name == "ListCertificates" {
					listCertInput = r.Params.(*acm.ListCertificatesInput)
					listCertOutput = r.Data.(*acm.ListCertificatesOutput)
					listCertOutput.CertificateSummaryList = []*acm.CertificateSummary{
						{CertificateArn: aws.String(IssuedCertificateArn1)},
						{CertificateArn: aws.String(IssuedCertificateArn2)},
					}
					listCertCallCount++
				}

				if r.Operation.Name == "DescribeCertificate" {
					describeCertInput = r.Params.(*acm.DescribeCertificateInput)
					describeCertOutput = r.Data.(*acm.DescribeCertificateOutput)
					tempCert := certDescriptions[*describeCertInput.CertificateArn]
					describeCertOutput.Certificate = &tempCert
					describeCertCallCount++
				}

				if r.Operation.Name == "ListTagsForCertificate" {

					listTagsForCertInput = r.Params.(*acm.ListTagsForCertificateInput)
					listTagsForCertOuput = r.Data.(*acm.ListTagsForCertificateOutput)
					listTagsForCertOuput.Tags = tags[*listTagsForCertInput.CertificateArn]
					listTagsForCertCallCount++
				}

			})

			certDescriptions[IssuedCertificateArn1] = acm.CertificateDetail{CertificateArn: aws.String(IssuedCertificateArn1), Status: aws.String(acm.CertificateStatusIssued)}
			certDescriptions[IssuedCertificateArn2] = acm.CertificateDetail{CertificateArn: aws.String(IssuedCertificateArn2), Status: aws.String(acm.CertificateStatusIssued)}

			tags[IssuedCertificateArn1] = []*acm.Tag{&acm.Tag{Key: aws.String(utils.CertificateTagName), Value: aws.String("cloudfoundry-service-instance-id-1")}}
			tags[IssuedCertificateArn2] = []*acm.Tag{&acm.Tag{Key: aws.String(utils.CertificateTagName), Value: aws.String("cloudfoundry-service-instance-id-2")}}

			_, err := certsManager.ListIssuedCertificates()

			Expect(err).NotTo(HaveOccurred())
			Expect(listCertInput.CertificateStatuses).To(ContainElement(aws.String(acm.CertificateStatusIssued)))
			Expect(describeCertInput).ToNot(BeNil())
			Expect(listTagsForCertInput).ToNot(BeNil())
			Expect(listCertCallCount).To(Equal(1))
			Expect(describeCertCallCount).To(Equal(2))
			Expect(listTagsForCertCallCount).To(Equal(2))
		})

	})

})
