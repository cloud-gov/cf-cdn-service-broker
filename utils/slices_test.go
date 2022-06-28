package utils_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/alphagov/paas-cdn-broker/utils"
)

var _ = Describe("Slices", func() {
	Context("String slices equal", func() {
		It("should return false when not equal", func() {
			Expect(utils.StrSlicesAreEqual(
				[]string{"foo"}, []string{},
			)).To(Equal(false), "Slice 2 is empty")

			Expect(utils.StrSlicesAreEqual(
				[]string{}, []string{"foo"},
			)).To(Equal(false), "Slice 1 is empty")

			Expect(utils.StrSlicesAreEqual(
				[]string{"Foo"}, []string{"foo"},
			)).To(Equal(false), "Different cases")

			Expect(utils.StrSlicesAreEqual(
				[]string{"foo", "bar"}, []string{"foo"},
			)).To(Equal(false), "Slice1 superset of Slice2")

			Expect(utils.StrSlicesAreEqual(
				[]string{"foo"}, []string{"bar", "foo"},
			)).To(Equal(false), "Slice2 superset of Slice1")

			Expect(utils.StrSlicesAreEqual(
				[]string{"foo", "bar"}, []string{"123", "456"},
			)).To(Equal(false), "Completely different")
		})

		It("should return true when equal", func() {
			Expect(utils.StrSlicesAreEqual(
				[]string{}, []string{},
			)).To(Equal(true))

			Expect(utils.StrSlicesAreEqual(
				[]string{"foo"}, []string{"foo"},
			)).To(Equal(true))

			Expect(utils.StrSlicesAreEqual(
				[]string{"foo", "bar"}, []string{"foo", "bar"},
			)).To(Equal(true))

			Expect(utils.StrSlicesAreEqual(
				[]string{"foo", "bar"}, []string{"bar", "foo"},
			)).To(Equal(true))
		})
	})
})
