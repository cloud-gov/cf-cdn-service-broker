package healthchecks

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/jarcoal/httpmock"

	"github.com/18F/cf-cdn-service-broker/config"
)

var _ = Describe("LetsEncrypt", func() {

	BeforeSuite(func() {
		httpmock.Activate()
	})

	BeforeEach(func() {
		httpmock.Reset()
	})

	AfterSuite(func() {
		httpmock.DeactivateAndReset()
	})

	It("Can new up a client", func() {
		httpmock.RegisterResponder(
			"GET", "https://acme-v01.api.letsencrypt.org/directory",
			httpmock.NewStringResponder(
				200, `{
                "foo-bar-baz": "https://community.letsencrypt.org/t/adding-random-entries-to-the-directory/1",
                "key-change": "https://acme-v01.api.letsencrypt.org/acme/key-change",
                "meta": {
                  "caaIdentities": ["letsencrypt.org"],
                  "terms-of-service": "https://letsencrypt.org/documents/LE-SA-v1.2-November-15-2017.pdf",
                  "website": "https://letsencrypt.org"
                },
                "new-authz": "https://acme-v01.api.letsencrypt.org/acme/new-authz",
                "new-cert": "https://acme-v01.api.letsencrypt.org/acme/new-cert",
                "new-reg": "https://acme-v01.api.letsencrypt.org/acme/new-reg",
                "revoke-cert": "https://acme-v01.api.letsencrypt.org/acme/revoke-cert"
              }`,
			),
		)

		err := LetsEncrypt(config.Settings{})

		Expect(err).NotTo(HaveOccurred())
	})
})
