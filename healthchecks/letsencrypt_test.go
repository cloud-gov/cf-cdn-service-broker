package healthchecks

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/18F/cf-cdn-service-broker/config"
)

var _ = Describe("LetsEncrypt", func() {
	It("Can new up a client", func() {
		err := LetsEncrypt(config.Settings{})

		Expect(err).NotTo(HaveOccurred())
	})
})
