package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"code.cloudfoundry.org/lager"
	"github.com/jinzhu/gorm"
	"github.com/pivotal-cf/brokerapi"
	"github.com/xenolf/lego/acme"

	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/aws/aws-sdk-go/service/iam"

	"github.com/18F/cf-cdn-service-broker/utils"
)

type State string

const (
	Provisioning   State = "provisioning"
	Provisioned          = "provisioned"
	Deprovisioning       = "deprovisioning"
	Deprovisioned        = "deprovisioned"
)

// Marshal a `State` to a `string` when saving to the database
func (s State) Value() (driver.Value, error) {
	return string(s), nil
}

// Unmarshal an `interface{}` to a `State` when reading from the database
func (s *State) Scan(value interface{}) error {
	switch value.(type) {
	case string:
		*s = State(value.(string))
	case []byte:
		*s = State(value.([]byte))
	default:
		return fmt.Errorf("Incompatible type for %s", value)
	}
	return nil
}

type Route struct {
	gorm.Model
	InstanceId     string `gorm:"not null;unique_index"`
	State          State  `gorm:"not null;index"`
	ChallengeJSON  []byte
	DomainExternal string
	DomainInternal string
	DistId         string
	Origin         string
	Path           string
	InsecureOrigin bool
	Certificate    Certificate
}

func (r *Route) GetDomains() []string {
	return strings.Split(r.DomainExternal, ",")
}

type Certificate struct {
	gorm.Model
	RouteId     uint
	Domain      string
	CertURL     string
	Certificate []byte
	Expires     time.Time `gorm:"index"`
}

type RouteManagerIface interface {
	Create(instanceId, domain, origin, path string, insecureOrigin bool, forwardedHeaders []string, forwardCookies bool, tags map[string]string) (*Route, error)
	Update(instanceId string, domain, origin string, path string, insecureOrigin bool, forwardedHeaders []string, forwardCookies bool) error
	Get(instanceId string) (*Route, error)
	Poll(route *Route) error
	Disable(route *Route) error
	Renew(route *Route) error
	RenewAll()
	DeleteOrphanedCerts()
}

type RouteManager struct {
	logger     lager.Logger
	iam        utils.IamIface
	cloudFront utils.DistributionIface
	acmeClient *acme.Client
	db         *gorm.DB
}

func NewManager(
	logger lager.Logger,
	iam utils.IamIface,
	cloudFront utils.DistributionIface,
	acmeClient *acme.Client,
	db *gorm.DB,
) RouteManager {
	return RouteManager{
		logger:     logger,
		iam:        iam,
		cloudFront: cloudFront,
		acmeClient: acmeClient,
		db:         db,
	}
}

func (m *RouteManager) Create(instanceId, domain, origin, path string, insecureOrigin bool, forwardedHeaders []string, forwardCookies bool, tags map[string]string) (*Route, error) {
	route := &Route{
		InstanceId:     instanceId,
		State:          Provisioning,
		DomainExternal: domain,
		Origin:         origin,
		Path:           path,
		InsecureOrigin: insecureOrigin,
	}

	dist, err := m.cloudFront.Create(instanceId, route.GetDomains(), origin, path, insecureOrigin, forwardedHeaders, forwardCookies, tags)
	if err != nil {
		return nil, err
	}

	route.DomainInternal = *dist.DomainName
	route.DistId = *dist.Id

	challenges, errs := m.acmeClient.GetChallenges(route.GetDomains())
	if len(errs) > 0 {
		return nil, fmt.Errorf("Error(s) getting challenges: %v", errs)
	}

	route.ChallengeJSON, err = json.Marshal(challenges)
	if err != nil {
		return nil, err
	}

	m.db.Create(route)
	return route, nil
}

func (m *RouteManager) Get(instanceId string) (*Route, error) {
	route := Route{}
	result := m.db.First(&route, Route{InstanceId: instanceId})
	if result.Error == nil {
		return &route, nil
	} else if result.RecordNotFound() {
		return nil, brokerapi.ErrInstanceDoesNotExist
	} else {
		return nil, result.Error
	}
}

func (m *RouteManager) Update(instanceId, domain, origin string, path string, insecureOrigin bool, forwardedHeaders []string, forwardCookies bool) error {
	// Get current route
	route, err := m.Get(instanceId)
	if err != nil {
		return err
	}

	// Override any settings that are new or different.
	if domain != "" {
		route.DomainExternal = domain
	}
	if origin != "" {
		route.Origin = origin
	}
	if path != route.Path {
		route.Path = path
	}
	if insecureOrigin != route.InsecureOrigin {
		route.InsecureOrigin = insecureOrigin
	}

	// Update the distribution
	dist, err := m.cloudFront.Update(route.DistId, route.GetDomains(),
		route.Origin, route.Path, route.InsecureOrigin, forwardedHeaders, forwardCookies)
	if err != nil {
		return err
	}
	route.State = Provisioning

	// Get the updated domain name and dist id.
	route.DomainInternal = *dist.DomainName
	route.DistId = *dist.Id

	// Save the database.
	result := m.db.Save(route)
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (m *RouteManager) Poll(r *Route) error {
	switch r.State {
	case Provisioning:
		return m.updateProvisioning(r)
	case Deprovisioning:
		return m.updateDeprovisioning(r)
	default:
		return nil
	}
}

func (m *RouteManager) Disable(r *Route) error {
	err := m.cloudFront.Disable(r.DistId)
	if err != nil {
		return err
	}

	r.State = Deprovisioning
	m.db.Save(r)

	return nil
}

func (m *RouteManager) Renew(r *Route) error {
	var certRow Certificate
	err := m.db.Model(r).Related(&certRow, "Certificate").Error
	if err != nil {
		return err
	}

	certResource, errs := m.acmeClient.ObtainCertificate(r.GetDomains(), true, nil, false)
	if len(errs) > 0 {
		return fmt.Errorf("Error(s) obtaining certificate: %v", errs)
	}
	expires, err := acme.GetPEMCertExpiration(certResource.Certificate)
	if err != nil {
		return err
	}

	if err := m.deployCertificate(r.InstanceId, r.DistId, certResource); err != nil {
		return err
	}

	certRow.Domain = certResource.Domain
	certRow.CertURL = certResource.CertURL
	certRow.Certificate = certResource.Certificate
	certRow.Expires = expires
	return m.db.Save(&certRow).Error
}

func (m *RouteManager) DeleteOrphanedCerts() {
	// iterate over all distributions and record all certificates in-use by these distributions
	activeCerts := make(map[string]string)

	m.cloudFront.ListDistributions(func(distro cloudfront.DistributionSummary) bool {
		if distro.ViewerCertificate.IAMCertificateId != nil {
			activeCerts[*distro.ViewerCertificate.IAMCertificateId] = *distro.ARN
		}
		return true
	})

	// iterate over all certificates
	m.iam.ListCertificates(func(cert iam.ServerCertificateMetadata) bool {

		// delete any certs not attached to a distribution that are older than 24 hours
		_, active := activeCerts[*cert.ServerCertificateId]
		if !active && time.Since(*cert.UploadDate).Hours() > 24 {
			m.logger.Info("Deleting orphaned certificate", lager.Data{
				"cert": cert,
			})

			err := m.iam.DeleteCertificate(*cert.ServerCertificateName)
			if err != nil {
				m.logger.Error("Error deleting certificate", err, lager.Data{
					"cert": cert,
				})
			}
		}

		return true
	})
}

func (m *RouteManager) RenewAll() {
	routes := []Route{}

	m.db.Where(
		"state = ? and expires < now() + interval '30 days'", string(Provisioned),
	).Joins(
		"join certificates on routes.id = certificates.route_id",
	).Preload(
		"Certificate",
	).Find(&routes)

	for _, route := range routes {
		err := m.Renew(&route)
		if err != nil {
			m.logger.Error("Error Renewing certificate", err, lager.Data{
				"domain": route.DomainExternal,
				"origin": route.Origin,
			})
		} else {
			m.logger.Info("Successfully Renewed certificate", lager.Data{
				"domain": route.DomainExternal,
				"origin": route.Origin,
			})
		}
	}
}

func (m *RouteManager) updateProvisioning(r *Route) error {
	if m.checkDistribution(r) {
		var challenges []acme.AuthorizationResource
		if err := json.Unmarshal(r.ChallengeJSON, &challenges); err != nil {
			return err
		}
		if errs := m.solveChallenges(challenges); len(errs) > 0 {
			return fmt.Errorf("Error(s) solving challenges: %v", errs)
		}

		cert, err := m.acmeClient.RequestCertificate(challenges, true, nil, false)
		if err != nil {
			return err
		}

		expires, err := acme.GetPEMCertExpiration(cert.Certificate)
		if err != nil {
			return err
		}
		if err := m.deployCertificate(r.InstanceId, r.DistId, cert); err != nil {
			return err
		}

		certRow := Certificate{
			Domain:      cert.Domain,
			CertURL:     cert.CertURL,
			Certificate: cert.Certificate,
			Expires:     expires,
		}
		if err := m.db.Create(&certRow).Error; err != nil {
			return err
		}

		r.State = Provisioned
		r.Certificate = certRow
		return m.db.Save(r).Error
	}

	m.logger.Info("distribution-provisioning", lager.Data{"instance_id": r.InstanceId})
	return nil
}

func (m *RouteManager) updateDeprovisioning(r *Route) error {
	deleted, err := m.cloudFront.Delete(r.DistId)
	if err != nil {
		return err
	}

	if deleted {
		r.State = Deprovisioned
		m.db.Save(r)
	}

	return nil
}

func (m *RouteManager) checkDistribution(r *Route) bool {
	dist, err := m.cloudFront.Get(r.DistId)
	if err != nil {
		return false
	}

	return *dist.Status == "Deployed" && *dist.DistributionConfig.Enabled
}

func (m *RouteManager) solveChallenges(challenges []acme.AuthorizationResource) map[string]error {
	fmt.Println(fmt.Sprintf("CHALLENGES: %+v", challenges))
	clients := map[acme.Challenge]*acme.Client{
		acme.DNS01:  &acme.Client{},
		acme.HTTP01: &acme.Client{},
	}
	*clients[acme.DNS01] = *m.acmeClient
	*clients[acme.HTTP01] = *m.acmeClient
	clients[acme.DNS01].ExcludeChallenges([]acme.Challenge{acme.HTTP01})
	clients[acme.HTTP01].ExcludeChallenges([]acme.Challenge{acme.DNS01})

	errs := make(chan map[string]error)

	for _, client := range clients {
		go func(client *acme.Client) {
			errs <- client.SolveChallenges(challenges)
		}(client)
	}

	var failures map[string]error
	for challenge, _ := range clients {
		failures = <-errs
		m.logger.Info("solve-challenges", lager.Data{
			"challenge": challenge,
			"failures":  failures,
		})
		if len(failures) == 0 {
			return failures
		}
	}

	return failures
}

func (m *RouteManager) deployCertificate(instanceId, distId string, cert acme.CertificateResource) error {
	expires, err := acme.GetPEMCertExpiration(cert.Certificate)
	if err != nil {
		return err
	}

	name := fmt.Sprintf("cdn-route-%s-%s", instanceId, expires.Format("2006-01-02_15-04-05"))

	m.logger.Info("Uploading certificate to IAM", lager.Data{"name": name})

	certId, err := m.iam.UploadCertificate(name, cert)
	if err != nil {
		return err
	}

	return m.cloudFront.SetCertificate(distId, certId)
}
