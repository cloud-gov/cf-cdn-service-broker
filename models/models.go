package models

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql/driver"
	"encoding/json"
	"encoding/pem"
	"errors"
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

type UserData struct {
	gorm.Model
	Email string `gorm:"not null;unique_index"`
	Reg   []byte
	Key   []byte
}

func GetOrCreateUser(db *gorm.DB, email string) (utils.User, UserData, error) {
	var user utils.User
	userData := UserData{Email: email}
	if res := db.First(&userData, &userData); res.Error != nil {
		if res.RecordNotFound() {
			user, err := CreateUser(email)
			return user, userData, err
		}
		return user, userData, res.Error
	}
	if err := json.Unmarshal(userData.Reg, &user); err != nil {
		return user, userData, err
	}
	key, err := loadPrivateKey(userData.Key)
	user.SetPrivateKey(key)
	return user, userData, err
}

func CreateUser(email string) (utils.User, error) {
	user := utils.User{Email: email}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return user, err
	}
	user.SetPrivateKey(key)
	return user, nil
}

func SaveUser(db *gorm.DB, user utils.User, userData UserData) error {
	var err error
	userData.Key, err = savePrivateKey(user.GetPrivateKey())
	if err != nil {
		return err
	}
	userData.Reg, err = json.Marshal(user)
	if err != nil {
		return err
	}
	if userData.ID == 0 {
		return db.Create(&userData).Error
	}
	return db.Save(&userData).Error
}

// loadPrivateKey loads a PEM-encoded ECC/RSA private key from an array of bytes.
func loadPrivateKey(keyBytes []byte) (crypto.PrivateKey, error) {
	keyBlock, _ := pem.Decode(keyBytes)

	switch keyBlock.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	case "EC PRIVATE KEY":
		return x509.ParseECPrivateKey(keyBlock.Bytes)
	}

	return nil, errors.New("unknown private key type")
}

// savePrivateKey saves a PEM-encoded ECC/RSA private key to an array of bytes.
func savePrivateKey(key crypto.PrivateKey) ([]byte, error) {
	var pemType string
	var keyBytes []byte
	switch key := key.(type) {
	case *ecdsa.PrivateKey:
		var err error
		pemType = "EC"
		keyBytes, err = x509.MarshalECPrivateKey(key)
		if err != nil {
			return nil, err
		}
	case *rsa.PrivateKey:
		pemType = "RSA"
		keyBytes = x509.MarshalPKCS1PrivateKey(key)
	}

	pemKey := pem.Block{Type: pemType + " PRIVATE KEY", Bytes: keyBytes}
	return pem.EncodeToMemory(&pemKey), nil
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
	GetDNSInstructions([]byte) ([]string, error)
}

type RouteManager struct {
	logger      lager.Logger
	iam         utils.IamIface
	cloudFront  utils.DistributionIface
	acmeUser    utils.User
	acmeClient  *acme.Client
	acmeClients map[acme.Challenge]*acme.Client
	db          *gorm.DB
}

func NewManager(
	logger lager.Logger,
	iam utils.IamIface,
	cloudFront utils.DistributionIface,
	acmeUser utils.User,
	acmeClients map[acme.Challenge]*acme.Client,
	db *gorm.DB,
) RouteManager {
	return RouteManager{
		logger:      logger,
		iam:         iam,
		cloudFront:  cloudFront,
		acmeUser:    acmeUser,
		acmeClient:  acmeClients[acme.HTTP01],
		acmeClients: acmeClients,
		db:          db,
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

	if err := m.ensureChallenges(route, false); err != nil {
		return nil, err
	}

	dist, err := m.cloudFront.Create(instanceId, route.GetDomains(), origin, path, insecureOrigin, forwardedHeaders, forwardCookies, tags)
	if err != nil {
		return nil, err
	}

	route.DomainInternal = *dist.DomainName
	route.DistId = *dist.Id

	if err := m.db.Create(route).Error; err != nil {
        return nil, err
    }

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

	if domain != "" {
		route.ChallengeJSON = []byte("")
		if err := m.ensureChallenges(route, false); err != nil {
			return err
		}
	}

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
	// Handle provisioning instances created before DNS challenge
	if err := m.ensureChallenges(r, true); err != nil {
		return err
	}

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
	errs := make(chan map[string]error)

	for _, client := range m.acmeClients {
		go func(client *acme.Client) {
			errs <- client.SolveChallenges(challenges)
		}(client)
	}

	var failures map[string]error
	for challenge, _ := range m.acmeClients {
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

func (m *RouteManager) ensureChallenges(route *Route, update bool) error {
	if len(route.ChallengeJSON) == 0 {
		challenges, errs := m.acmeClient.GetChallenges(route.GetDomains())
		if len(errs) > 0 {
			return fmt.Errorf("Error(s) getting challenges: %v", errs)
		}

		var err error
		route.ChallengeJSON, err = json.Marshal(challenges)
		if err != nil {
			return err
		}

		if update {
			return m.db.Save(route).Error
		}
		return nil
	}

	return nil
}

func (m *RouteManager) GetDNSInstructions(data []byte) ([]string, error) {
	var instructions []string
	var challenges []acme.AuthorizationResource
	if err := json.Unmarshal(data, &challenges); err != nil {
		return instructions, err
	}
	for _, auth := range challenges {
		for _, challenge := range auth.Body.Challenges {
			if challenge.Type == acme.DNS01 {
				keyAuth, err := acme.GetKeyAuthorization(challenge.Token, m.acmeUser.GetPrivateKey())
				if err != nil {
					return instructions, err
				}
				fqdn, value, ttl := acme.DNS01Record(auth.Domain, keyAuth)
				instructions = append(instructions, fmt.Sprintf("name: %s, value: %s, ttl: %d", fqdn, value, ttl))
			}
		}
	}
	return instructions, nil
}
