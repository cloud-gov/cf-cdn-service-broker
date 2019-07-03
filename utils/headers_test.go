package utils

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Headers", func() {
	Context("Add", func() {
		It("adds", func() {
			headers := Headers{}
			headers.Add("abc-def")

			Expect(headers).To(Equal(Headers{"Abc-Def": true}))
		})
	})

	Context("Contains", func() {
		It("contains", func() {
			headers := Headers{"Abc-Def": true}

			Expect(headers.Contains("Abc-Def")).To(Equal(true))
			Expect(headers.Contains("Ghi-Jkl")).To(Equal(false))
		})
	})

	Context("Strings", func() {
		It("strings", func() {
			headers := Headers{"Abc-Def": true, "User-Agent": true}
			headerStrings := headers.Strings()

			Expect(headerStrings).To(ContainElement("Abc-Def"))
			Expect(headerStrings).To(ContainElement("User-Agent"))
			Expect(headerStrings).To(HaveLen(2))
		})
	})
})
