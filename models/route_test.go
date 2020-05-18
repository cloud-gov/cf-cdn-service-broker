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
			time32hoursBefore time.Time
		)
		BeforeEach(func() {

			time32hoursBefore = time.Now().Add(-32 * time.Hour)
			route = models.Route{
				Model: gorm.Model{
					CreatedAt: time32hoursBefore,
					UpdatedAt: time32hoursBefore,
				},
			}

			route.ProvisioningSince = &time32hoursBefore
		})

		It("is expired when the last update time is >24h ago", func() {

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
})
