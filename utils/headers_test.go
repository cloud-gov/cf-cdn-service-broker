package utils_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/alphagov/paas-cdn-broker/utils"
)

var _ = Describe("Headers", func() {
	Context("Add", func() {
		It("adds", func() {
			headers := utils.Headers{}
			headers.Add("abc-def")

			Expect(headers).To(Equal(utils.Headers{"Abc-Def": true}))
		})
	})

	Context("Contains", func() {
		It("contains", func() {
			headers := utils.Headers{"Abc-Def": true}

			Expect(headers.Contains("Abc-Def")).To(Equal(true))
			Expect(headers.Contains("Ghi-Jkl")).To(Equal(false))
		})
	})

	Context("Strings", func() {
		It("strings", func() {
			headers := utils.Headers{"Abc-Def": true, "User-Agent": true}
			headerStrings := headers.Strings()

			Expect(headerStrings).To(ContainElement("Abc-Def"))
			Expect(headerStrings).To(ContainElement("User-Agent"))
			Expect(headerStrings).To(HaveLen(2))
		})
	})
})
