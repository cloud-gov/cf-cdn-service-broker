package models

import (
	"fmt"
	"net"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/xenolf/lego/acme"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudfront"

	"github.com/18F/cf-cdn-service-broker/config"
	"github.com/18F/cf-cdn-service-broker/utils"
)

type Route struct {
	gorm.Model
	InstanceId     string `gorm:"unique_index"`
	State          string `gorm:"index"`
	DomainExternal string
	DomainInternal string
	DistId         string
	Origin         string
	Certificate    Certificate
}

func NewRoute(settings config.Settings, db *gorm.DB, instanceId, domain, origin string) (Route, error) {
	dist, err := utils.CreateDistribution(settings, domain, origin)
	if err != nil {
		return Route{}, err
	}
	route := Route{
		InstanceId:     instanceId,
		State:          "provisioning",
		DomainExternal: domain,
		DomainInternal: *dist.DomainName,
		DistId:         *dist.Id,
		Origin:         origin,
	}
	db.Create(&route)
	return route, nil
}

func (r *Route) IsPending() bool {
	return r.State == "provisioning" || r.State == "deprovisioning"
}

func (r *Route) Update(settings config.Settings, db *gorm.DB) error {
	switch r.State {
	case "provisioning":
		return r.updateProvisioning(settings, db)
	case "deprovisioning":
		return r.updateDeprovisioning(db)
	}
	return nil
}

func (r *Route) Disable(db *gorm.DB) error {
	err := utils.DisableDistribution(r.DistId)
	if err != nil {
		return err
	}
	r.State = "deprovisioning"
	db.Save(&r)
	return nil
}

func (r *Route) updateProvisioning(settings config.Settings, db *gorm.DB) error {
	if r.checkCNAME() && r.checkDistribution() {
		certResource, err := r.provisionCert(settings)
		if err != nil {
			return err
		}
		expires, err := acme.GetPEMCertExpiration(certResource.Certificate)
		certRow := Certificate{
			CertURL:       certResource.CertURL,
			CertStableURL: certResource.CertStableURL,
			Expires:       expires,
		}
		db.Create(&certRow)
		r.State = "provisioned"
		r.Certificate = certRow
		db.Save(&r)
	}
	return nil
}

func (r *Route) updateDeprovisioning(db *gorm.DB) error {
	deleted, err := utils.DeleteDistribution(r.DomainExternal, r.DistId)
	if err != nil {
		return err
	}
	if deleted {
		r.State = "deprovisioned"
		db.Save(&r)
	}
	return nil
}

func (r *Route) provisionCert(settings config.Settings) (acme.CertificateResource, error) {
	cert, err := utils.ObtainCertificate(settings, r.DomainExternal)
	if err != nil {
		return acme.CertificateResource{}, err
	}
	certId, err := utils.UploadCert(r.DomainExternal, cert)
	if err != nil {
		return acme.CertificateResource{}, err
	}
	err = utils.DeployCert(certId, r.DistId)
	if err != nil {
		return acme.CertificateResource{}, err
	}
	return cert, nil
}

func (r *Route) checkCNAME() bool {
	cname, err := net.LookupCNAME(r.DomainExternal)
	if err != nil {
		return false
	}
	return cname == fmt.Sprintf("%s.", r.DomainInternal)
}

func (r *Route) checkDistribution() bool {
	svc := cloudfront.New(session.New())
	resp, err := svc.GetDistribution(&cloudfront.GetDistributionInput{
		Id: aws.String(r.DistId),
	})
	if err != nil {
		return false
	}
	return *resp.Distribution.Status == "Deployed" && *resp.Distribution.DistributionConfig.Enabled
}

type Certificate struct {
	gorm.Model
	RouteId       uint
	CertURL       string
	CertStableURL string
	Expires       time.Time `gorm:"index"`
}
