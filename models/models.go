package models

import (
	"net"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/xenolf/lego/acme"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudfront"

	"github.com/18F/cf-cdn-service-broker/utils"
)

type Route struct {
	gorm.Model
	InstanceId     string `gorm:"unique_index"`
	Pending        bool   `gorm:"index"`
	DomainExternal string
	DomainInternal string
	DistId         string
	Certificate    Certificate
}

func NewRoute(db *gorm.DB, instanceId, domain string) (Route, error) {
	dist, err := utils.CreateDistribution(domain)
	if err != nil {
		return Route{}, err
	}
	route := Route{
		InstanceId:     instanceId,
		DomainExternal: domain,
		DomainInternal: *dist.DomainName,
		DistId:         *dist.Id,
		Pending:        true,
	}
	db.Create(&route)
	return route, nil
}

func (r *Route) Update(db *gorm.DB) error {
	if !r.Pending {
		return nil
	}
	if r.checkCNAME() && r.checkDistribution() {
		certResource, err := r.provisionCert()
		if err != nil {
			return err
		}
		certRow := Certificate{
			CertURL:       certResource.CertURL,
			CertStableURL: certResource.CertStableURL,
		}
		db.Create(&certRow)
		r.Pending = false
		r.Certificate = certRow
		db.Save(r)
	}
	return nil
}

func (r *Route) provisionCert() (acme.CertificateResource, error) {
	cert, err := utils.ObtainCertificate(r.DomainExternal)
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
	return strings.Contains(cname, ".cloudfront.net")
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
	CertURL       string
	CertStableURL string
	Expires       time.Time `gorm:"index"`
}

func (c *Certificate) BeforeCreate(scope *gorm.Scope) error {
	scope.SetColumn("Expires", time.Now().AddDate(0, 0, 90))
	return nil
}
