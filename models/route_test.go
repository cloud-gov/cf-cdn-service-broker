package models_test

import (
	"github.com/jinzhu/gorm"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"time"

	"github.com/18F/cf-cdn-service-broker/models"
)

var _ = Describe("Route", func() {
 	Describe("IsProvisioningExpired", func(){
 		It("is expired when the last update time is >24h ago", func(){
 			route := models.Route{
				Model: gorm.Model{
					CreatedAt: time.Now().Add(-32 * time.Hour),
				},
				State: models.Provisioning,
			}

			Expect(route.IsProvisioningExpired()).To(BeTrue())
 		})

		It("is not expired if the state is not `Provisioning`", func(){
			route := models.Route{
				Model: gorm.Model{
					CreatedAt: time.Now().Add(-32 * time.Hour),
				},
				State: models.Provisioned,
			}

			Expect(route.IsProvisioningExpired()).To(BeFalse())
		})
	})
})
