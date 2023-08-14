package utils_test

import (
	"errors"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudfront"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/alphagov/paas-cdn-broker/config"
	"github.com/alphagov/paas-cdn-broker/utils"
	"github.com/alphagov/paas-cdn-broker/utils/mocks"
)

var _ = Describe("Distribution", func() {
	var distribution utils.Distribution
	var fakeCloudFront mocks.FakeCloudfront

	BeforeEach(func() {
		fakeCloudFront = mocks.FakeCloudfront{}
		distribution = utils.Distribution{
			Settings: config.Settings{},
			Service: &fakeCloudFront,
		}
	})

	distributionConfigCommonAssertions := func(dc *cloudfront.DistributionConfig) {
		Expect(dc).ToNot(BeNil())
		Expect(dc.Aliases).ToNot(BeNil())

		Expect(dc.CacheBehaviors).To(BeNil())

		Expect(dc.Comment).To(Equal(aws.String("cdn route service")))

		Expect(dc.Enabled).To(Equal(aws.Bool(true)))
		Expect(dc.IsIPV6Enabled).To(Equal(aws.Bool(true)))
		Expect(dc.PriceClass).To(Equal(aws.String("PriceClass_100")))

		Expect(dc.Origins).ToNot(BeNil())
		Expect(dc.Origins.Quantity).To(Equal(aws.Int64(1)))
		Expect(dc.Origins.Items).To(HaveLen(1))
		Expect(dc.Origins.Items[0]).ToNot(BeNil())
		Expect(dc.Origins.Items[0].OriginPath).To(Equal(aws.String("")))

		Expect(dc.Origins.Items[0].CustomHeaders).ToNot(BeNil())

		Expect(dc.Origins.Items[0].CustomOriginConfig).ToNot(BeNil())
		Expect(dc.Origins.Items[0].CustomOriginConfig.HTTPPort).To(Equal(aws.Int64(80)))
		Expect(dc.Origins.Items[0].CustomOriginConfig.HTTPSPort).To(Equal(aws.Int64(443)))
		Expect(dc.Origins.Items[0].CustomOriginConfig.OriginProtocolPolicy).To(Equal(aws.String("https-only")))
		Expect(dc.Origins.Items[0].CustomOriginConfig.OriginReadTimeout).To(Equal(aws.Int64(60)))
		Expect(dc.Origins.Items[0].CustomOriginConfig.OriginKeepaliveTimeout).To(Equal(aws.Int64(5)))
		Expect(dc.Origins.Items[0].CustomOriginConfig.OriginSslProtocols).ToNot(BeNil())
		Expect(dc.Origins.Items[0].CustomOriginConfig.OriginSslProtocols.Quantity).To(Equal(aws.Int64(1)))
		Expect(dc.Origins.Items[0].CustomOriginConfig.OriginSslProtocols.Items).To(HaveLen(1))
		Expect(dc.Origins.Items[0].CustomOriginConfig.OriginSslProtocols.Items[0]).To(Equal(aws.String("TLSv1.2")))


		Expect(dc.DefaultCacheBehavior).ToNot(BeNil())

		Expect(dc.DefaultCacheBehavior.ForwardedValues).ToNot(BeNil())
		Expect(dc.DefaultCacheBehavior.ForwardedValues.Cookies).ToNot(BeNil())
		Expect(dc.DefaultCacheBehavior.ForwardedValues.QueryString).To(Equal(aws.Bool(true)))
		Expect(dc.DefaultCacheBehavior.ForwardedValues.QueryStringCacheKeys).ToNot(BeNil())
		Expect(dc.DefaultCacheBehavior.ForwardedValues.QueryStringCacheKeys.Quantity).To(Equal(aws.Int64(0)))
		Expect(dc.DefaultCacheBehavior.ForwardedValues.QueryStringCacheKeys.Items).To(HaveLen(0))
		Expect(dc.DefaultCacheBehavior.ForwardedValues.Headers).ToNot(BeNil())
		Expect(dc.DefaultCacheBehavior.SmoothStreaming).To(Equal(aws.Bool(false)))
		Expect(dc.DefaultCacheBehavior.MinTTL).To(Equal(aws.Int64(0)))
		Expect(dc.DefaultCacheBehavior.MaxTTL).To(Equal(aws.Int64(31622400)))
		Expect(dc.DefaultCacheBehavior.TrustedSigners).ToNot(BeNil())
		Expect(dc.DefaultCacheBehavior.TrustedSigners.Enabled).To(Equal(aws.Bool(false)))
		Expect(dc.DefaultCacheBehavior.ViewerProtocolPolicy).To(Equal(aws.String("redirect-to-https")))

		Expect(dc.DefaultCacheBehavior.AllowedMethods).ToNot(BeNil())
		Expect(dc.DefaultCacheBehavior.AllowedMethods.CachedMethods).ToNot(BeNil())
		Expect(dc.DefaultCacheBehavior.AllowedMethods.CachedMethods.Quantity).To(Equal(aws.Int64(2)))
		Expect(dc.DefaultCacheBehavior.AllowedMethods.CachedMethods.Items).To(ConsistOf(aws.String("HEAD"),aws.String("GET") ))
		Expect(dc.DefaultCacheBehavior.AllowedMethods.Quantity).To(Equal(aws.Int64(7)))
		Expect(dc.DefaultCacheBehavior.AllowedMethods.Items).To(ConsistOf(
			aws.String("HEAD"),
			aws.String("GET"),
			aws.String("OPTIONS"),
			aws.String("PUT"),
			aws.String("POST"),
			aws.String("PATCH"),
			aws.String("DELETE"),
		))
	}

	Context("Create", func() {
		createDistributionWithTagsInputCommonAssertions := func(cdti *cloudfront.CreateDistributionWithTagsInput) {
			Expect(cdti).ToNot(BeNil())

			Expect(cdti.DistributionConfigWithTags).ToNot(BeNil())
			Expect(cdti.DistributionConfigWithTags.Tags).ToNot(BeNil())

			distributionConfigCommonAssertions(cdti.DistributionConfigWithTags.DistributionConfig)
		}

		JustBeforeEach(func() {
			fakeCloudFront.CreateDistributionWithTagsReturns(
				&cloudfront.CreateDistributionWithTagsOutput{
					Distribution: &cloudfront.Distribution{
						ARN: aws.String("some:created:resource/id"),
					},
				},
				nil,
			)
		})

		It("creates a distribution without forwarding cookies", func() {
			retDist, err := distribution.Create(
				"foo777",
				[]string{
					"foo.example.com",
					"bar.example.net",
				},
				"some-origin",
				1234,
				utils.Headers{
					"X-Foo": true,
					"X-Baz": true,
				},
				false,
				map[string]string{
					"blahkey": "blabvalue",
				},
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(retDist.ARN).To(Equal(aws.String("some:created:resource/id")))

			Expect(fakeCloudFront.CreateDistributionWithTagsCallCount()).To(Equal(1))
			cdti := fakeCloudFront.CreateDistributionWithTagsArgsForCall(0)

			createDistributionWithTagsInputCommonAssertions(cdti)

			Expect(cdti.DistributionConfigWithTags.DistributionConfig.Aliases.Quantity).To(Equal(aws.Int64(2)))
			Expect(cdti.DistributionConfigWithTags.DistributionConfig.Aliases.Items).To(HaveLen(2))
			Expect(cdti.DistributionConfigWithTags.DistributionConfig.Aliases.Items).To(
				ContainElement(Equal(aws.String("foo.example.com"))),
			)
			Expect(cdti.DistributionConfigWithTags.DistributionConfig.Aliases.Items).To(
				ContainElement(Equal(aws.String("bar.example.net"))),
			)

			Expect(cdti.DistributionConfigWithTags.Tags.Items).To(HaveLen(1))
			Expect(cdti.DistributionConfigWithTags.Tags.Items[0]).ToNot(BeNil())
			Expect(cdti.DistributionConfigWithTags.Tags.Items[0].Key).To(Equal(aws.String("blahkey")))
			Expect(cdti.DistributionConfigWithTags.Tags.Items[0].Value).To(Equal(aws.String("blabvalue")))

			Expect(cdti.DistributionConfigWithTags.DistributionConfig.CallerReference).To(Equal(aws.String("foo777")))
			Expect(cdti.DistributionConfigWithTags.DistributionConfig.Origins.Items[0].Id).To(Equal(aws.String("foo777")))

			Expect(cdti.DistributionConfigWithTags.DistributionConfig.Origins.Items[0].CustomHeaders.Quantity).To(Equal(aws.Int64(0)))
			Expect(cdti.DistributionConfigWithTags.DistributionConfig.Origins.Items[0].CustomHeaders.Items).To(HaveLen(0))

			Expect(cdti.DistributionConfigWithTags.DistributionConfig.Origins.Items[0].DomainName).To(Equal(aws.String("some-origin")))
			Expect(cdti.DistributionConfigWithTags.DistributionConfig.DefaultCacheBehavior.TargetOriginId).To(Equal(aws.String("foo777")))

			Expect(cdti.DistributionConfigWithTags.DistributionConfig.DefaultCacheBehavior.ForwardedValues.Cookies.Forward).To(Equal(aws.String("none")))

			Expect(cdti.DistributionConfigWithTags.DistributionConfig.DefaultCacheBehavior.ForwardedValues.Headers.Quantity).To(Equal(aws.Int64(2)))
			Expect(cdti.DistributionConfigWithTags.DistributionConfig.DefaultCacheBehavior.ForwardedValues.Headers.Items).To(ConsistOf(aws.String("X-Foo"), aws.String("X-Baz")))
			Expect(cdti.DistributionConfigWithTags.DistributionConfig.DefaultCacheBehavior.DefaultTTL).To(Equal(aws.Int64(1234)))
		})

		It("creates a distribution with forwarding cookies", func() {
			retDist, err := distribution.Create(
				"foo777",
				[]string{
					"foo.example.com",
					"bar.example.net",
				},
				"some-origin",
				1234,
				utils.Headers{
					"X-Foo": true,
					"X-Baz": true,
				},
				true,
				map[string]string{
					"blahkey": "blabvalue",
				},
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(retDist.ARN).To(Equal(aws.String("some:created:resource/id")))

			Expect(fakeCloudFront.CreateDistributionWithTagsCallCount()).To(Equal(1))
			cdti := fakeCloudFront.CreateDistributionWithTagsArgsForCall(0)

			createDistributionWithTagsInputCommonAssertions(cdti)

			Expect(cdti.DistributionConfigWithTags.DistributionConfig.Aliases.Quantity).To(Equal(aws.Int64(2)))
			Expect(cdti.DistributionConfigWithTags.DistributionConfig.Aliases.Items).To(HaveLen(2))
			Expect(cdti.DistributionConfigWithTags.DistributionConfig.Aliases.Items).To(
				ContainElement(Equal(aws.String("foo.example.com"))),
			)
			Expect(cdti.DistributionConfigWithTags.DistributionConfig.Aliases.Items).To(
				ContainElement(Equal(aws.String("bar.example.net"))),
			)

			Expect(cdti.DistributionConfigWithTags.Tags.Items).To(HaveLen(1))
			Expect(cdti.DistributionConfigWithTags.Tags.Items[0]).ToNot(BeNil())
			Expect(cdti.DistributionConfigWithTags.Tags.Items[0].Key).To(Equal(aws.String("blahkey")))
			Expect(cdti.DistributionConfigWithTags.Tags.Items[0].Value).To(Equal(aws.String("blabvalue")))

			Expect(cdti.DistributionConfigWithTags.DistributionConfig.CallerReference).To(Equal(aws.String("foo777")))
			Expect(cdti.DistributionConfigWithTags.DistributionConfig.Origins.Items[0].Id).To(Equal(aws.String("foo777")))

			Expect(cdti.DistributionConfigWithTags.DistributionConfig.Origins.Items[0].CustomHeaders.Quantity).To(Equal(aws.Int64(0)))
			Expect(cdti.DistributionConfigWithTags.DistributionConfig.Origins.Items[0].CustomHeaders.Items).To(HaveLen(0))

			Expect(cdti.DistributionConfigWithTags.DistributionConfig.Origins.Items[0].DomainName).To(Equal(aws.String("some-origin")))
			Expect(cdti.DistributionConfigWithTags.DistributionConfig.DefaultCacheBehavior.TargetOriginId).To(Equal(aws.String("foo777")))

			Expect(cdti.DistributionConfigWithTags.DistributionConfig.DefaultCacheBehavior.ForwardedValues.Cookies.Forward).To(Equal(aws.String("all")))

			Expect(cdti.DistributionConfigWithTags.DistributionConfig.DefaultCacheBehavior.ForwardedValues.Headers.Quantity).To(Equal(aws.Int64(2)))
			Expect(cdti.DistributionConfigWithTags.DistributionConfig.DefaultCacheBehavior.ForwardedValues.Headers.Items).To(ConsistOf(aws.String("X-Foo"), aws.String("X-Baz")))
			Expect(cdti.DistributionConfigWithTags.DistributionConfig.DefaultCacheBehavior.DefaultTTL).To(Equal(aws.Int64(1234)))
		})

		Context("when settings specify ExtraRequestHeaders", func() {
			BeforeEach(func() {
				distribution.Settings.ExtraRequestHeaders = map[string]string{
					"x-qux": "qux-value",
					"x-qux-2": "qux-2-value",
				}
			})

			It("creates a distribution with custom headers set appropriately", func() {
				retDist, err := distribution.Create(
					"foo777",
					[]string{
						"foo.example.com",
						"bar.example.net",
					},
					"some-origin",
					1234,
					utils.Headers{
						"X-Foo": true,
						"X-Baz": true,
					},
					false,
					map[string]string{
						"blahkey": "blabvalue",
					},
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(retDist.ARN).To(Equal(aws.String("some:created:resource/id")))

				Expect(fakeCloudFront.CreateDistributionWithTagsCallCount()).To(Equal(1))
				cdti := fakeCloudFront.CreateDistributionWithTagsArgsForCall(0)

				createDistributionWithTagsInputCommonAssertions(cdti)

				Expect(cdti.DistributionConfigWithTags.DistributionConfig.Aliases.Quantity).To(Equal(aws.Int64(2)))
				Expect(cdti.DistributionConfigWithTags.DistributionConfig.Aliases.Items).To(HaveLen(2))
				Expect(cdti.DistributionConfigWithTags.DistributionConfig.Aliases.Items).To(
					ContainElement(Equal(aws.String("foo.example.com"))),
				)
				Expect(cdti.DistributionConfigWithTags.DistributionConfig.Aliases.Items).To(
					ContainElement(Equal(aws.String("bar.example.net"))),
				)

				Expect(cdti.DistributionConfigWithTags.Tags.Items).To(HaveLen(1))
				Expect(cdti.DistributionConfigWithTags.Tags.Items[0]).ToNot(BeNil())
				Expect(cdti.DistributionConfigWithTags.Tags.Items[0].Key).To(Equal(aws.String("blahkey")))
				Expect(cdti.DistributionConfigWithTags.Tags.Items[0].Value).To(Equal(aws.String("blabvalue")))

				Expect(cdti.DistributionConfigWithTags.DistributionConfig.CallerReference).To(Equal(aws.String("foo777")))
				Expect(cdti.DistributionConfigWithTags.DistributionConfig.Origins.Items[0].Id).To(Equal(aws.String("foo777")))

				Expect(cdti.DistributionConfigWithTags.DistributionConfig.Origins.Items[0].CustomHeaders.Quantity).To(Equal(aws.Int64(2)))
				Expect(cdti.DistributionConfigWithTags.DistributionConfig.Origins.Items[0].CustomHeaders.Items).To(ConsistOf(
					SatisfyAll(HaveField("HeaderName", aws.String("x-qux")), HaveField("HeaderValue", aws.String("qux-value"))),
					SatisfyAll(HaveField("HeaderName", aws.String("x-qux-2")), HaveField("HeaderValue", aws.String("qux-2-value"))),
				))

				Expect(cdti.DistributionConfigWithTags.DistributionConfig.Origins.Items[0].DomainName).To(Equal(aws.String("some-origin")))
				Expect(cdti.DistributionConfigWithTags.DistributionConfig.DefaultCacheBehavior.TargetOriginId).To(Equal(aws.String("foo777")))

				Expect(cdti.DistributionConfigWithTags.DistributionConfig.DefaultCacheBehavior.ForwardedValues.Cookies.Forward).To(Equal(aws.String("none")))

				Expect(cdti.DistributionConfigWithTags.DistributionConfig.DefaultCacheBehavior.ForwardedValues.Headers.Quantity).To(Equal(aws.Int64(2)))
				Expect(cdti.DistributionConfigWithTags.DistributionConfig.DefaultCacheBehavior.ForwardedValues.Headers.Items).To(ConsistOf(aws.String("X-Foo"), aws.String("X-Baz")))
				Expect(cdti.DistributionConfigWithTags.DistributionConfig.DefaultCacheBehavior.DefaultTTL).To(Equal(aws.Int64(1234)))
			})
		})
	})

	Context("Update", func() {
		var existingDistributionConfig cloudfront.DistributionConfig

		BeforeEach(func() {
			// simulate a config that has drifted
			existingDistributionConfig = cloudfront.DistributionConfig{
				CallerReference: aws.String("foo777"),
			}
		})

		JustBeforeEach(func() {
			fakeCloudFront.GetDistributionConfigReturns(&cloudfront.GetDistributionConfigOutput{
				DistributionConfig: &existingDistributionConfig,
				ETag: aws.String("wat987"),
			}, nil)
			fakeCloudFront.UpdateDistributionReturns(
				&cloudfront.UpdateDistributionOutput{
					Distribution: &cloudfront.Distribution{
						ARN: aws.String("some:updated:resource/id"),
					},
				},
				nil,
			)
		})

		It("updates the cloudfront distribution", func() {
			retDist, err := distribution.Update(
				"foo222",
				&[]string{
					"foo.example.com",
					"bar.example.net",
				},
				"some-origin",
				aws.Int64(1234),
				&utils.Headers{
					"X-Foo": true,
					"X-Baz": true,
				},
				aws.Bool(false),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(retDist.ARN).To(Equal(aws.String("some:updated:resource/id")))

			Expect(fakeCloudFront.GetDistributionConfigCallCount()).To(Equal(1))
			Expect(fakeCloudFront.GetDistributionConfigArgsForCall(0).Id).To(Equal(aws.String("foo222")))

			Expect(fakeCloudFront.UpdateDistributionCallCount()).To(Equal(1))
			udi := fakeCloudFront.UpdateDistributionArgsForCall(0)

			Expect(udi).ToNot(BeNil())
			Expect(udi.Id).To(Equal(aws.String("foo222")))
			Expect(udi.IfMatch).To(Equal(aws.String("wat987")))

			distributionConfigCommonAssertions(udi.DistributionConfig)

			Expect(udi.DistributionConfig.Aliases.Quantity).To(Equal(aws.Int64(2)))
			Expect(udi.DistributionConfig.Aliases.Items).To(HaveLen(2))
			Expect(udi.DistributionConfig.Aliases.Items).To(
				ContainElement(Equal(aws.String("foo.example.com"))),
			)
			Expect(udi.DistributionConfig.Aliases.Items).To(
				ContainElement(Equal(aws.String("bar.example.net"))),
			)

			Expect(udi.DistributionConfig.CallerReference).To(Equal(aws.String("foo777")))
			Expect(udi.DistributionConfig.Origins.Items[0].Id).To(Equal(aws.String("foo777")))

			Expect(udi.DistributionConfig.Origins.Items[0].CustomHeaders.Quantity).To(Equal(aws.Int64(0)))
			Expect(udi.DistributionConfig.Origins.Items[0].CustomHeaders.Items).To(HaveLen(0))

			Expect(udi.DistributionConfig.Origins.Items[0].DomainName).To(Equal(aws.String("some-origin")))
			Expect(udi.DistributionConfig.DefaultCacheBehavior.TargetOriginId).To(Equal(aws.String("foo777")))

			Expect(udi.DistributionConfig.DefaultCacheBehavior.ForwardedValues.Cookies.Forward).To(Equal(aws.String("none")))

			Expect(udi.DistributionConfig.DefaultCacheBehavior.ForwardedValues.Headers.Quantity).To(Equal(aws.Int64(2)))
			Expect(udi.DistributionConfig.DefaultCacheBehavior.ForwardedValues.Headers.Items).To(ConsistOf(aws.String("X-Foo"), aws.String("X-Baz")))
			Expect(udi.DistributionConfig.DefaultCacheBehavior.DefaultTTL).To(Equal(aws.Int64(1234)))
		})

		It("updates the cloudfront distribution with forwarding cookies", func() {
			retDist, err := distribution.Update(
				"foo222",
				&[]string{
					"foo.example.com",
					"bar.example.net",
				},
				"some-origin",
				aws.Int64(1234),
				&utils.Headers{
					"X-Foo": true,
					"X-Baz": true,
				},
				aws.Bool(true),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(retDist.ARN).To(Equal(aws.String("some:updated:resource/id")))

			Expect(fakeCloudFront.GetDistributionConfigCallCount()).To(Equal(1))
			Expect(fakeCloudFront.GetDistributionConfigArgsForCall(0).Id).To(Equal(aws.String("foo222")))

			Expect(fakeCloudFront.UpdateDistributionCallCount()).To(Equal(1))
			udi := fakeCloudFront.UpdateDistributionArgsForCall(0)

			Expect(udi).ToNot(BeNil())
			Expect(udi.Id).To(Equal(aws.String("foo222")))
			Expect(udi.IfMatch).To(Equal(aws.String("wat987")))

			distributionConfigCommonAssertions(udi.DistributionConfig)

			Expect(udi.DistributionConfig.Aliases.Quantity).To(Equal(aws.Int64(2)))
			Expect(udi.DistributionConfig.Aliases.Items).To(HaveLen(2))
			Expect(udi.DistributionConfig.Aliases.Items).To(
				ContainElement(Equal(aws.String("foo.example.com"))),
			)
			Expect(udi.DistributionConfig.Aliases.Items).To(
				ContainElement(Equal(aws.String("bar.example.net"))),
			)

			Expect(udi.DistributionConfig.CallerReference).To(Equal(aws.String("foo777")))
			Expect(udi.DistributionConfig.Origins.Items[0].Id).To(Equal(aws.String("foo777")))

			Expect(udi.DistributionConfig.Origins.Items[0].CustomHeaders.Quantity).To(Equal(aws.Int64(0)))
			Expect(udi.DistributionConfig.Origins.Items[0].CustomHeaders.Items).To(HaveLen(0))

			Expect(udi.DistributionConfig.Origins.Items[0].DomainName).To(Equal(aws.String("some-origin")))
			Expect(udi.DistributionConfig.DefaultCacheBehavior.TargetOriginId).To(Equal(aws.String("foo777")))

			Expect(udi.DistributionConfig.DefaultCacheBehavior.ForwardedValues.Cookies.Forward).To(Equal(aws.String("all")))

			Expect(udi.DistributionConfig.DefaultCacheBehavior.ForwardedValues.Headers.Quantity).To(Equal(aws.Int64(2)))
			Expect(udi.DistributionConfig.DefaultCacheBehavior.ForwardedValues.Headers.Items).To(ConsistOf(aws.String("X-Foo"), aws.String("X-Baz")))
			Expect(udi.DistributionConfig.DefaultCacheBehavior.DefaultTTL).To(Equal(aws.Int64(1234)))
		})

		Context("when GetDistributionConfig errors", func() {
			JustBeforeEach(func() {
				fakeCloudFront.GetDistributionConfigReturns(
					nil,
					errors.New("some terrible error"),
				)
			})

			It("aborts early", func() {
				_, err := distribution.Update(
					"foo222",
					&[]string{
						"foo.example.com",
						"bar.example.net",
					},
					"some-origin",
					aws.Int64(1234),
					&utils.Headers{
						"X-Foo": true,
						"X-Baz": true,
					},
					aws.Bool(true),
				)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("some terrible error"))

				Expect(fakeCloudFront.GetDistributionConfigCallCount()).To(Equal(1))
				Expect(fakeCloudFront.GetDistributionConfigArgsForCall(0).Id).To(Equal(aws.String("foo222")))

				Expect(fakeCloudFront.UpdateDistributionCallCount()).To(Equal(0))
			})
		})

		Context("when the distribution's config has drifted", func() {
			BeforeEach(func() {
				// a config that has drifted to almost the exact opposite configuration
				// it should have
				existingDistributionConfig = cloudfront.DistributionConfig{
					Aliases: &cloudfront.Aliases{
						Quantity: aws.Int64(1),
						Items: []*string{
							aws.String("before.example.com"),
						},
					},
					Comment: aws.String("cdn route service before"),
					Enabled: aws.Bool(false),
					CallerReference: aws.String("blah888"),
					IsIPV6Enabled: aws.Bool(false),
					PriceClass: aws.String("PriceClass_Before"),
					Origins: &cloudfront.Origins{
						Quantity: aws.Int64(2),
						Items: []*cloudfront.Origin{
							&cloudfront.Origin{
								Id: aws.String("blah999"),
								OriginPath: aws.String("nonempty"),
								DomainName: aws.String("flim.before.example.com"),
								CustomOriginConfig: &cloudfront.CustomOriginConfig{
									HTTPPort:               aws.Int64(111),
									HTTPSPort:              aws.Int64(222),
									OriginReadTimeout:      aws.Int64(333),
									OriginKeepaliveTimeout: aws.Int64(444),
								},
								CustomHeaders: &cloudfront.CustomHeaders{
									Quantity: aws.Int64(1),
									Items: []*cloudfront.OriginCustomHeader{
										&cloudfront.OriginCustomHeader{
											HeaderName: aws.String("existing-custom"),
											HeaderValue: aws.String("header"),
										},
									},
								},
							},
							&cloudfront.Origin{
								Id: aws.String("blab333"),
								OriginPath: aws.String("nonempty"),
								DomainName: aws.String("flam.before.example.com"),
							},
						},
					},
					DefaultCacheBehavior: &cloudfront.DefaultCacheBehavior{
						TargetOriginId: aws.String("blub123"),
						ForwardedValues: &cloudfront.ForwardedValues{
							Cookies: &cloudfront.CookiePreference{
								Forward: aws.String("impossible-value"),
							},
							QueryString: aws.Bool(false),
							QueryStringCacheKeys: &cloudfront.QueryStringCacheKeys{
								Quantity: aws.Int64(1),
								Items: []*string{
									aws.String("flump"),
								},
							},
							Headers: &cloudfront.Headers{
								Quantity: aws.Int64(2),
								Items: []*string{
									aws.String("bump"),
									aws.String("burp"),
								},
							},
						},
						SmoothStreaming: aws.Bool(true),
						MinTTL: aws.Int64(123),
						MaxTTL: aws.Int64(456),
						Compress: aws.Bool(true),
						TrustedSigners: &cloudfront.TrustedSigners{
							Enabled: aws.Bool(true),
							Quantity: aws.Int64(1),
							Items: []*string{
								aws.String("slump"),
							},
						},
						ViewerProtocolPolicy: aws.String("some-other-policy"),
						AllowedMethods: &cloudfront.AllowedMethods{
							CachedMethods: nil,
							Quantity: aws.Int64(8),
							Items: []*string{
								aws.String("ONE"),
								aws.String("TWO"),
								aws.String("THREE"),
								aws.String("FOUR"),
								aws.String("FIVE"),
								aws.String("SIX"),
								aws.String("SEVEN"),
								aws.String("EIGHT"),
							},
						},
						DefaultTTL: aws.Int64(333),
					},
				}
			})

			It("updates all values we care about back to the appropriate values", func() {
				retDist, err := distribution.Update(
					"foo222",
					&[]string{
						"foo.example.com",
						"bar.example.net",
					},
					"some-origin",
					aws.Int64(1234),
					&utils.Headers{
						"X-Foo": true,
						"X-Baz": true,
					},
					aws.Bool(false),
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(retDist.ARN).To(Equal(aws.String("some:updated:resource/id")))

				Expect(fakeCloudFront.GetDistributionConfigCallCount()).To(Equal(1))
				Expect(fakeCloudFront.GetDistributionConfigArgsForCall(0).Id).To(Equal(aws.String("foo222")))

				Expect(fakeCloudFront.UpdateDistributionCallCount()).To(Equal(1))
				udi := fakeCloudFront.UpdateDistributionArgsForCall(0)

				Expect(udi).ToNot(BeNil())
				Expect(udi.Id).To(Equal(aws.String("foo222")))
				Expect(udi.IfMatch).To(Equal(aws.String("wat987")))

				distributionConfigCommonAssertions(udi.DistributionConfig)

				Expect(udi.DistributionConfig.Aliases.Quantity).To(Equal(aws.Int64(2)))
				Expect(udi.DistributionConfig.Aliases.Items).To(HaveLen(2))
				Expect(udi.DistributionConfig.Aliases.Items).To(
					ContainElement(Equal(aws.String("foo.example.com"))),
				)
				Expect(udi.DistributionConfig.Aliases.Items).To(
					ContainElement(Equal(aws.String("bar.example.net"))),
				)

				Expect(udi.DistributionConfig.CallerReference).To(Equal(aws.String("blah888")))
				Expect(udi.DistributionConfig.Origins.Items[0].Id).To(Equal(aws.String("blah888")))

				Expect(udi.DistributionConfig.Origins.Items[0].CustomHeaders.Quantity).To(Equal(aws.Int64(0)))
				Expect(udi.DistributionConfig.Origins.Items[0].CustomHeaders.Items).To(HaveLen(0))

				Expect(udi.DistributionConfig.Origins.Items[0].DomainName).To(Equal(aws.String("some-origin")))
				Expect(udi.DistributionConfig.DefaultCacheBehavior.TargetOriginId).To(Equal(aws.String("blah888")))

				Expect(udi.DistributionConfig.DefaultCacheBehavior.ForwardedValues.Cookies.Forward).To(Equal(aws.String("none")))

				Expect(udi.DistributionConfig.DefaultCacheBehavior.ForwardedValues.Headers.Quantity).To(Equal(aws.Int64(2)))
				Expect(udi.DistributionConfig.DefaultCacheBehavior.ForwardedValues.Headers.Items).To(ConsistOf(aws.String("X-Foo"), aws.String("X-Baz")))
				Expect(udi.DistributionConfig.DefaultCacheBehavior.DefaultTTL).To(Equal(aws.Int64(1234)))
			})
		})

		Context("when settings specify ExtraRequestHeaders", func() {
			BeforeEach(func() {
				distribution.Settings.ExtraRequestHeaders = map[string]string{
					"x-qux": "qux-value",
					"x-qux-2": "qux-2-value",
				}
			})

			It("updates the cloudfront distribution with the custom headers set appropriately", func() {
				retDist, err := distribution.Update(
					"foo222",
					&[]string{
						"foo.example.com",
						"bar.example.net",
					},
					"some-origin",
					aws.Int64(1234),
					&utils.Headers{
						"X-Foo": true,
						"X-Baz": true,
					},
					aws.Bool(false),
				)
				Expect(err).ToNot(HaveOccurred())
				Expect(retDist.ARN).To(Equal(aws.String("some:updated:resource/id")))

				Expect(fakeCloudFront.GetDistributionConfigCallCount()).To(Equal(1))
				Expect(fakeCloudFront.GetDistributionConfigArgsForCall(0).Id).To(Equal(aws.String("foo222")))

				Expect(fakeCloudFront.UpdateDistributionCallCount()).To(Equal(1))
				udi := fakeCloudFront.UpdateDistributionArgsForCall(0)

				Expect(udi).ToNot(BeNil())
				Expect(udi.Id).To(Equal(aws.String("foo222")))
				Expect(udi.IfMatch).To(Equal(aws.String("wat987")))

				distributionConfigCommonAssertions(udi.DistributionConfig)

				Expect(udi.DistributionConfig.Aliases.Quantity).To(Equal(aws.Int64(2)))
				Expect(udi.DistributionConfig.Aliases.Items).To(HaveLen(2))
				Expect(udi.DistributionConfig.Aliases.Items).To(
					ContainElement(Equal(aws.String("foo.example.com"))),
				)
				Expect(udi.DistributionConfig.Aliases.Items).To(
					ContainElement(Equal(aws.String("bar.example.net"))),
				)

				Expect(udi.DistributionConfig.CallerReference).To(Equal(aws.String("foo777")))
				Expect(udi.DistributionConfig.Origins.Items[0].Id).To(Equal(aws.String("foo777")))

				Expect(udi.DistributionConfig.Origins.Items[0].CustomHeaders.Quantity).To(Equal(aws.Int64(2)))
				Expect(udi.DistributionConfig.Origins.Items[0].CustomHeaders.Items).To(ConsistOf(
					SatisfyAll(HaveField("HeaderName", aws.String("x-qux")), HaveField("HeaderValue", aws.String("qux-value"))),
					SatisfyAll(HaveField("HeaderName", aws.String("x-qux-2")), HaveField("HeaderValue", aws.String("qux-2-value"))),
				))

				Expect(udi.DistributionConfig.Origins.Items[0].DomainName).To(Equal(aws.String("some-origin")))
				Expect(udi.DistributionConfig.DefaultCacheBehavior.TargetOriginId).To(Equal(aws.String("foo777")))

				Expect(udi.DistributionConfig.DefaultCacheBehavior.ForwardedValues.Cookies.Forward).To(Equal(aws.String("none")))

				Expect(udi.DistributionConfig.DefaultCacheBehavior.ForwardedValues.Headers.Quantity).To(Equal(aws.Int64(2)))
				Expect(udi.DistributionConfig.DefaultCacheBehavior.ForwardedValues.Headers.Items).To(ConsistOf(aws.String("X-Foo"), aws.String("X-Baz")))
				Expect(udi.DistributionConfig.DefaultCacheBehavior.DefaultTTL).To(Equal(aws.Int64(1234)))
			})
		})
	})
})
