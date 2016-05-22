package models

import (
	"fmt"
	"net"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/pivotal-cf/brokerapi"
	"github.com/xenolf/lego/acme"

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

type Certificate struct {
	gorm.Model
	acme.CertificateResource
	RouteId uint
	Expires time.Time `gorm:"index"`
}

type RouteManagerIface interface {
	Create(instanceId, domain, origin string) (Route, error)
	Get(instanceId string) (Route, error)
	Update(route Route) error
	Disable(route Route) error
	Renew(route Route) error
	RenewAll()
}

type RouteManager struct {
	Iam        utils.IamIface
	CloudFront utils.DistributionIface
	Acme       utils.AcmeIface
	DB         *gorm.DB
}

func (m *RouteManager) Create(instanceId, domain, origin string) (Route, error) {
	dist, err := m.CloudFront.Create(domain, origin)
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

	m.DB.Create(&route)
	return route, nil
}

func (m *RouteManager) Get(instanceId string) (Route, error) {
	route := Route{}
	m.DB.First(&route, Route{InstanceId: instanceId})
	if route.InstanceId == instanceId {
		return route, nil
	}
	return Route{}, brokerapi.ErrInstanceDoesNotExist
}

func (m *RouteManager) Update(r Route) error {
	switch r.State {
	case "provisioning":
		return m.updateProvisioning(r)
	case "deprovisioning":
		return m.updateDeprovisioning(r)
	default:
		return nil
	}
}

func (m *RouteManager) Disable(r Route) error {
	err := m.CloudFront.Disable(r.DistId)
	if err != nil {
		return err
	}

	r.State = "deprovisioning"
	m.DB.Save(&r)

	return nil
}

func (m *RouteManager) Renew(r Route) error {
	var certRow Certificate

	m.DB.Model(&r).Related(&certRow, "Certificate")

	certResource, err := m.Acme.RenewCertificate(certRow.CertificateResource)
	if err != nil {
		return err
	}

	err = m.deployCertificate(r.DomainExternal, r.DistId, certResource)
	if err != nil {
		return err
	}

	expires, err := acme.GetPEMCertExpiration(certResource.Certificate)
	if err != nil {
		return err
	}

	certRow.CertificateResource = certResource
	certRow.Expires = expires
	m.DB.Save(&certRow)

	return nil
}

func (m *RouteManager) RenewAll() {
	routes := []Route{}

	m.DB.Where(
		"state = ? and expires < now() + interval '30 days'", "provisioned",
	).Joins(
		"join certificates on routes.id = certificates.route_id",
	).Preload(
		"Certificate",
	).Find(&routes)

	for _, route := range routes {
		m.Renew(route)
	}
}

func (m *RouteManager) updateProvisioning(r Route) error {
	if m.checkCNAME(r) && m.checkDistribution(r) {
		certResource, err := m.provisionCert(r)
		if err != nil {
			return err
		}

		expires, err := acme.GetPEMCertExpiration(certResource.Certificate)
		if err != nil {
			return err
		}

		certRow := Certificate{
			CertificateResource: certResource,
			Expires:             expires,
		}
		m.DB.Create(&certRow)

		r.State = "provisioned"
		r.Certificate = certRow
		m.DB.Save(&r)
	}

	return nil
}

func (m *RouteManager) updateDeprovisioning(r Route) error {
	deleted, err := m.CloudFront.Delete(r.DistId)
	if err != nil {
		return err
	}

	if deleted {
		err = m.Iam.DeleteCertificate(fmt.Sprintf("cdn-route-%s", r.DomainExternal))
		if err != nil {
			return err
		}

		r.State = "deprovisioned"
		m.DB.Save(&r)
	}

	return nil
}

func (m *RouteManager) provisionCert(r Route) (acme.CertificateResource, error) {
	cert, err := m.Acme.ObtainCertificate(r.DomainExternal)
	if err != nil {
		return acme.CertificateResource{}, err
	}

	err = m.deployCertificate(r.DomainExternal, r.DistId, cert)
	if err != nil {
		return acme.CertificateResource{}, err
	}

	return cert, nil
}

func (m *RouteManager) checkCNAME(r Route) bool {
	cname, err := net.LookupCNAME(r.DomainExternal)
	if err != nil {
		return false
	}

	return cname == fmt.Sprintf("%s.", r.DomainInternal)
}

func (m *RouteManager) checkDistribution(r Route) bool {
	dist, err := m.CloudFront.Get(r.DistId)
	if err != nil {
		return false
	}

	return *dist.Status == "Deployed" && *dist.DistributionConfig.Enabled
}

func (m *RouteManager) deployCertificate(domain, distId string, cert acme.CertificateResource) error {
	prev := fmt.Sprintf("cdn-route-%s-new", domain)
	next := fmt.Sprintf("cdn-route-%s", domain)

	certId, err := m.Iam.UploadCertificate(prev, cert)
	if err != nil {
		return err
	}

	err = m.CloudFront.SetCertificate(certId, distId)
	if err != nil {
		return err
	}

	return m.Iam.RenameCertificate(prev, next)
}
