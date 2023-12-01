package broker_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/alphagov/paas-cdn-broker/models"
	"github.com/alphagov/paas-cdn-broker/utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/pivotal-cf/brokerapi/v10/domain"
	"github.com/pivotal-cf/brokerapi/v10/domain/apiresponses"
	"github.com/stretchr/testify/suite"

	"code.cloudfoundry.org/lager/v3"
	"github.com/alphagov/paas-cdn-broker/broker"
	cfmock "github.com/alphagov/paas-cdn-broker/cf/mocks"
	"github.com/alphagov/paas-cdn-broker/config"
	"github.com/alphagov/paas-cdn-broker/models/mocks"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type GetInstanceSuite struct {
	suite.Suite
	Manager  mocks.RouteManagerIface
	Broker   *broker.CdnServiceBroker
	cfclient cfmock.Client
	settings config.Settings
	logger   lager.Logger
	ctx      context.Context
}

var _ = Describe("GetInstance", func() {

	s := &GetInstanceSuite{}

	BeforeEach(func() {
		s.Manager = mocks.RouteManagerIface{}
		s.cfclient = cfmock.Client{}
		s.logger = lager.NewLogger("test")
		s.settings = config.Settings{
			DefaultOrigin:     "origin.cloudapps.digital",
			DefaultDefaultTTL: int64(0),
		}
		s.Broker = broker.New(
			&s.Manager,
			&s.cfclient,
			s.settings,
			s.logger,
		)
		s.ctx = context.Background()
	})

	It("should error when the instance can't be found", func() {
		instanceId := "some-instance-id"
		s.Manager.GetReturns(nil, apiresponses.ErrInstanceDoesNotExist)

		_, err := s.Broker.GetInstance(s.ctx, instanceId, domain.FetchInstanceDetails{})
		Expect(err).To(HaveOccurred())
		Expect(err).To(MatchError(apiresponses.ErrInstanceDoesNotExist))
	})

	It("should error when the DNS challenges can't be found", func() {
		instanceId := "some-instance-id"
		s.Manager.GetReturns(&models.Route{}, nil)
		s.Manager.GetDNSChallengesReturns(nil, fmt.Errorf("can't get DNS challenges"))

		_, err := s.Broker.GetInstance(s.ctx, instanceId, domain.FetchInstanceDetails{})
		Expect(err).To(HaveOccurred())
	})

	Describe("when the instance is found", func() {
		var route *models.Route
		var instanceId = "instance-id"

		BeforeEach(func() {
			route = &models.Route{
				InstanceId:        instanceId,
				State:             models.Provisioning,
				DomainExternal:    "domain1.cloudapps.digital,domain2.cloudapps.digital",
				DomainInternal:    "xyz.cloudfront.net",
				DistId:            "distribution-1",
				Origin:            "",
				Path:              "",
				InsecureOrigin:    false,
				DefaultTTL:        0,
				ProvisioningSince: nil,
				Certificates:      nil,
			}
			s.Manager.GetReturns(route, nil)

			s.Manager.GetDNSChallengesReturns([]utils.DomainValidationChallenge{
				{
					DomainName:       "domain1.cloudapps.digital",
					RecordName:       "_validation.domain1.cloudapps.digital",
					RecordType:       "CNAME",
					RecordValue:      "abc123",
					ValidationStatus: "PENDING",
				},
				{
					DomainName:       "domain2.cloudapps.digital",
					RecordName:       "_validation.domain2.cloudapps.digital",
					RecordType:       "CNAME",
					RecordValue:      "def456",
					ValidationStatus: "PENDING",
				},
			}, nil)

			s.Manager.GetCDNConfigurationReturns(&cloudfront.Distribution{
				DomainName: aws.String("xyz.cloudfront.net"),
				Id:         aws.String("distribution-1"),
				Status:     aws.String("Deployed"),
				DistributionConfig: &cloudfront.DistributionConfig{
					Aliases: &cloudfront.Aliases{
						Quantity: aws.Int64(2),
						Items: []*string{
							aws.String("domain1.cloudapps.digital"),
							aws.String("domain2.cloudapps.digital"),
						},
					},
					DefaultCacheBehavior: &cloudfront.DefaultCacheBehavior{
						ForwardedValues: &cloudfront.ForwardedValues{
							Headers: &cloudfront.Headers{
								Items: []*string{
									aws.String("Host"),
									aws.String("Authorization"),
								},
								Quantity: aws.Int64(2),
							},
							Cookies: &cloudfront.CookiePreference{
								Forward: aws.String("all"),
							},
						},
						DefaultTTL: aws.Int64(1000),
					},
				},
			}, nil)
		})

		It("should return the CloudFront domain in the instance parameters", func() {
			instance, err := s.Broker.GetInstance(s.ctx, instanceId, domain.FetchInstanceDetails{})
			Expect(err).ToNot(HaveOccurred())

			Expect(instance.Parameters).ToNot(BeNil())
			params, err := instanceParamsToMap(instance.Parameters)
			Expect(err).ToNot(HaveOccurred())
			Expect(params).To(HaveKeyWithValue("cloudfront_domain", route.DomainInternal))
		})

		It("should return each of the DNS records that need setting in the instance parameters", func() {
			instance, err := s.Broker.GetInstance(s.ctx, instanceId, domain.FetchInstanceDetails{})
			Expect(err).ToNot(HaveOccurred())

			Expect(instance.Parameters).ToNot(BeNil())
			params, err := instanceParamsToMap(instance.Parameters)
			Expect(err).ToNot(HaveOccurred())

			Expect(params).To(HaveKey("dns_records"))
			Expect(params["dns_records"]).To(ConsistOf(
				map[string]interface{}{
					"validating_domain_name":      "domain1.cloudapps.digital",
					"challenge_dns_record":        "_validation.domain1.cloudapps.digital",
					"challenges_dns_record_type":  "CNAME",
					"challenges_dns_record_value": "abc123",
					"status":                      "PENDING",
				},
				map[string]interface{}{
					"validating_domain_name":      "domain2.cloudapps.digital",
					"challenge_dns_record":        "_validation.domain2.cloudapps.digital",
					"challenges_dns_record_type":  "CNAME",
					"challenges_dns_record_value": "def456",
					"status":                      "PENDING",
				},
			))
		})

		It("should return the headers being forwarded in the instance parameters", func() {
			instance, err := s.Broker.GetInstance(s.ctx, instanceId, domain.FetchInstanceDetails{})
			Expect(err).ToNot(HaveOccurred())

			Expect(instance.Parameters).ToNot(BeNil())
			params, err := instanceParamsToMap(instance.Parameters)
			Expect(err).ToNot(HaveOccurred())

			Expect(params).To(HaveKey("forwarded_headers"))
			Expect(params["forwarded_headers"]).To(ConsistOf("Host", "Authorization"))
		})

		It("should return the cloudfront distribution id in the instance parameters", func() {
			instance, err := s.Broker.GetInstance(s.ctx, instanceId, domain.FetchInstanceDetails{})
			Expect(err).ToNot(HaveOccurred())

			Expect(instance.Parameters).ToNot(BeNil())
			params, err := instanceParamsToMap(instance.Parameters)
			Expect(err).ToNot(HaveOccurred())
			Expect(params).To(HaveKeyWithValue("cloudfront_distribution_id", "distribution-1"))
		})

		It("should return the cookie forwarding configuration in the instance parameters", func() {
			instance, err := s.Broker.GetInstance(s.ctx, instanceId, domain.FetchInstanceDetails{})
			Expect(err).ToNot(HaveOccurred())

			Expect(instance.Parameters).ToNot(BeNil())
			params, err := instanceParamsToMap(instance.Parameters)
			Expect(err).ToNot(HaveOccurred())
			Expect(params).To(HaveKeyWithValue("forward_cookies", true))
		})

		It("should return the TTL configuration in the instance parameters", func() {
			instance, err := s.Broker.GetInstance(s.ctx, instanceId, domain.FetchInstanceDetails{})
			Expect(err).ToNot(HaveOccurred())

			Expect(instance.Parameters).ToNot(BeNil())
			params, err := instanceParamsToMap(instance.Parameters)
			Expect(err).ToNot(HaveOccurred())
			Expect(params).To(HaveKeyWithValue("cache_ttl", float64(1000)))
		})
	})
})

// Converting to JSON and back so that we can test what it would look like to consumers
func instanceParamsToMap(params interface{}) (map[string]interface{}, error) {
	jsonBytes, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	out := map[string]interface{}{}
	err = json.Unmarshal(jsonBytes, &out)
	if err != nil {
		return nil, err
	}

	return out, nil
}
