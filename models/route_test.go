package models_test

import (
	"time"

	"github.com/jinzhu/gorm"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/18F/cf-cdn-service-broker/models"
)

var _ = Describe("Route", func() {
	Describe("IsProvisioningExpired", func() {
		var (
			route             models.Route
			time96hoursBefore time.Time
		)
		BeforeEach(func() {

			time96hoursBefore = time.Now().Add(-96 * time.Hour)
			route = models.Route{
				Model: gorm.Model{
					CreatedAt: time96hoursBefore,
					UpdatedAt: time96hoursBefore,
				},
			}

			route.ProvisioningSince = &time96hoursBefore
		})

		It("is expired when the ProvisioningSince time is >84h ago", func() {

			route.State = models.Provisioning

			Expect(route.IsProvisioningExpired()).To(BeTrue())
		})

		It("is not expired if the state is not `Provisioning`", func() {
			route.State = models.Provisioned

			Expect(route.IsProvisioningExpired()).To(BeFalse())
		})

		It("is not expired if the ProvisioningSince is `nil`", func() {

			route.ProvisioningSince = nil

			Expect(route.IsProvisioningExpired()).To(BeFalse())

		})

	})

	Context("IsCertificateManagedByACM", func() {
		var (
			route models.Route
		)

		BeforeEach(func() {

			route = models.Route{}
		})

		It("Make sure that the default value is 'false'", func() {

			Expect(route.IsCertificateManagedByACM).To(BeFalse())

		})

		It("Make sure that the value is 'true', after setting it to 'true'", func() {

			route.IsCertificateManagedByACM = true

			Expect(route.IsCertificateManagedByACM).To(BeTrue())

		})

	})

})
