// todo (mxplusb): simplify this. EXTENSIVELY
package models

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"database/sql/driver"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"strings"
	"time"

	"code.cloudfoundry.org/lager"
	"github.com/go-acme/lego/certcrypto"
	"github.com/go-acme/lego/certificate"
	"github.com/go-acme/lego/lego"
	"github.com/jinzhu/gorm"
	"github.com/pivotal-cf/brokerapi"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/18F/cf-cdn-service-broker/config"
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
	Email string `gorm:"not null"`
	Reg   []byte
	Key   []byte
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

func SaveUser(db *gorm.DB, user utils.User) (UserData, error) {
	var err error
	userData := UserData{Email: user.GetEmail()}

	userData.Key, err = savePrivateKey(user.GetPrivateKey())
	if err != nil {
		return userData, err
	}
	userData.Reg, err = json.Marshal(user)
	if err != nil {
		return userData, err
	}

	if err := db.Save(&userData).Error; err != nil {
		return userData, err
	}

	return userData, nil
}

func LoadUser(userData UserData) (utils.User, error) {
	var user utils.User
	if err := json.Unmarshal(userData.Reg, &user); err != nil {
		return user, err
	}
	key, err := loadPrivateKey(userData.Key)
	if err != nil {
		return user, err
	}
	user.SetPrivateKey(key)
	return user, nil
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
	DomainExternal string
	DomainInternal string
	DistId         string
	Origin         string
	Path           string
	InsecureOrigin bool
	Certificate    Certificate
	UserData       UserData
	UserDataID     int
}

func (r *Route) GetDomains() []string {
	return strings.Split(r.DomainExternal, ",")
}

func (r *Route) loadUser(db *gorm.DB) (utils.User, error) {
	var userData UserData
	if err := db.Model(r).Related(&userData).Error; err != nil {
		return utils.User{}, err
	}

	return LoadUser(userData)
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
	Create(instanceId, domain, origin, path string, insecureOrigin bool, forwardedHeaders utils.Headers, forwardCookies bool, tags map[string]string) (*Route, error)
	Update(instanceId string, domain, origin string, path string, insecureOrigin bool, forwardedHeaders utils.Headers, forwardCookies bool) error
	Get(instanceId string) (*Route, error)
	Poll(route *Route) error
	Disable(route *Route) error
	Renew(route *Route) error
	RenewAll()
	DeleteOrphanedCerts()
	GetDNSInstructions(route *Route) ([]string, error)
}

type RouteManager struct {
	logger       lager.Logger
	iam          utils.IamIface
	cloudFront   utils.DistributionIface
	settings     config.Settings
	dnsPresenter chan string
	db           *gorm.DB
}

func NewManager(
	logger lager.Logger,
	iam utils.IamIface,
	cloudFront utils.DistributionIface,
	settings config.Settings,
	db *gorm.DB,
) RouteManager {
	return RouteManager{
		logger:     logger,
		iam:        iam,
		cloudFront: cloudFront,
		settings:   settings,
		db:         db,
	}
}

func (m *RouteManager) Create(instanceId, domain, origin, path string, insecureOrigin bool, forwardedHeaders utils.Headers, forwardCookies bool, tags map[string]string) (*Route, error) {
	route := &Route{
		InstanceId:     instanceId,
		State:          Provisioning,
		DomainExternal: domain,
		Origin:         origin,
		Path:           path,
		InsecureOrigin: insecureOrigin,
	}

	user, err := CreateUser(m.settings.Email)
	if err != nil {
		return nil, err
	}

	userData, err := SaveUser(m.db, user)
	if err != nil {
		return nil, err
	}

	route.UserData = userData

	dist, err := m.cloudFront.Create(instanceId, make([]string, 0), origin, path, insecureOrigin, forwardedHeaders, forwardCookies, tags)
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

func (m *RouteManager) Update(instanceId, domain, origin string, path string, insecureOrigin bool, forwardedHeaders utils.Headers, forwardCookies bool) error {
	// Get current route
	route, err := m.Get(instanceId)
	if err != nil {
		return err
	}

	// When we update the CloudFront distribution we should use the old domains
	// until we have a valid certificate in IAM.
	// CloudFront gets updated when we receive new certificates during Poll
	oldDomainsForCloudFront := route.GetDomains()

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
	dist, err := m.cloudFront.Update(route.DistId, oldDomainsForCloudFront,
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

func (m *RouteManager) stillActive(r *Route) error {

	m.logger.Info("Starting canary check", lager.Data{
		"route":    r,
		"settings": m.settings,
	})

	localSession, err := session.NewSession(aws.NewConfig().WithRegion(m.settings.AwsDefaultRegion))
	if err != nil {
		return err
	}

	s3client := s3.New(localSession)

	target := path.Join(".well-known", "acme-challenge", "canary", r.InstanceId)

	input := s3.PutObjectInput{
		Bucket: aws.String(m.settings.Bucket),
		Key:    aws.String(target),
		Body:   strings.NewReader(r.InstanceId),
	}

	if m.settings.ServerSideEncryption != "" {
		input.ServerSideEncryption = aws.String(m.settings.ServerSideEncryption)
	}

	if _, err := s3client.PutObject(&input); err != nil {
		return err
	}

	insecureClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	for _, domain := range r.GetDomains() {
		// because we're deferring in a loop, we need to have a lambda/anonymous func or there will be a deferral leak.
		return func() error {
			resp, err := insecureClient.Get("https://" + path.Join(domain, target))
			if err != nil {
				return err
			}

			defer resp.Body.Close()
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return err
			}

			if string(body) != r.InstanceId {
				return fmt.Errorf("canary check failed for %s; expected %s, got %s", domain, r.InstanceId, string(body))
			}
			return nil
		}()
	}

	return nil
}

func (m *RouteManager) Renew(r *Route) error {
	err := m.stillActive(r)
	if err != nil {
		return fmt.Errorf("route is not active, skipping renewal: %v", err)
	}

	var certRow Certificate
	err = m.db.Model(r).Related(&certRow, "Certificate").Error
	if err != nil {
		return err
	}

	user, err := r.loadUser(m.db)
	if err != nil {
		return err
	}

	client, err := m.getClient(&user, m.settings)
	if err != nil {
		return err
	}

	certResource, err := client.Certificate.Obtain(certificate.ObtainRequest{
		Domains:    r.GetDomains(),
		Bundle:     true,
		PrivateKey: nil,
		MustStaple: false,
	})
	if err != nil {
		return fmt.Errorf("error obtaining certificate: %v", err)
	}

	cert, err := certcrypto.ParsePEMCertificate(certResource.Certificate)
	if err != nil {
		return err
	}

	if err := m.deployCertificate(*(r), *(certResource)); err != nil {
		return err
	}

	certRow.Domain = certResource.Domain
	certRow.CertURL = certResource.CertURL
	certRow.Certificate = certResource.Certificate
	certRow.Expires = cert.NotAfter
	return m.db.Save(&certRow).Error
}

func (m *RouteManager) DeleteOrphanedCerts() {
	// iterate over all distributions and record all certificates in-use by these distributions
	activeCerts := make(map[string]string)

	if err := m.cloudFront.ListDistributions(func(distro cloudfront.DistributionSummary) bool {
		if distro.ViewerCertificate.IAMCertificateId != nil {
			activeCerts[*distro.ViewerCertificate.IAMCertificateId] = *distro.ARN
		}
		return true
	}); err != nil {
		fmt.Printf("error retrieving cloudfront distributions: %s", err)
	}

	// iterate over all certificates
	if err := m.iam.ListCertificates(func(cert iam.ServerCertificateMetadata) bool {

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
	}); err != nil {
		fmt.Printf("error deleting orphaned certificates: %s", err)
	}
}

func (m *RouteManager) RenewAll() {
	routes := []Route{}

	m.logger.Info("Looking for routes that are expiring soon")

	// todo (mxplusb): this needs to use the orm, not raw sql
	m.db.Having(
		"max(expires) < now() + interval '30 days'",
	).Group(
		"routes.id",
	).Where(
		"state = ?", string(Provisioned),
	).Joins(
		"join certificates on routes.id = certificates.route_id",
	).Find(&routes)

	m.logger.Info("Found routes that need renewal", lager.Data{
		"num-routes": len(routes),
	})

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

func (m *RouteManager) getClient(user *utils.User, settings config.Settings) (*lego.Client, error) {
	localSession, err := session.NewSession(aws.NewConfig().WithRegion(settings.AwsDefaultRegion))
	if err != nil {
		return &lego.Client{}, err
	}

	client, presenter, err := utils.NewClient(settings, user, s3.New(localSession))
	if err != nil {
		return client, err
	}
	m.dnsPresenter = presenter

	return client, nil
}

func (m *RouteManager) updateProvisioning(r *Route) error {
	user, err := r.loadUser(m.db)
	if err != nil {
		return err
	}

	client, err := m.getClient(&user, m.settings)
	if err != nil {
		return err
	}

	if m.checkDistribution(r) {
		certResource, err := client.Certificate.Obtain(certificate.ObtainRequest{
			Domains:    r.GetDomains(),
			Bundle:     true,
			PrivateKey: nil,
			MustStaple: false,
		})
		if err != nil {
			return err
		}

		cert, err := certcrypto.ParsePEMCertificate(certResource.Certificate)
		if err != nil {
			return err
		}

		// dereference
		if err := m.deployCertificate(*(r), *(certResource)); err != nil {
			return err
		}

		certRow := Certificate{
			Domain:      certResource.Domain,
			Certificate: certResource.Certificate,
			Expires:     cert.NotAfter,
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

func (m *RouteManager) deployCertificate(route Route, certResource certificate.Resource) error {
	cert, err := certcrypto.ParsePEMCertificate(certResource.Certificate)
	if err != nil {
		return err
	}
	expires := cert.NotAfter

	name := fmt.Sprintf("cdn-route-%s-%s", route.InstanceId, expires.Format("2006-01-02_15-04-05"))

	m.logger.Info("Uploading certificate to IAM", lager.Data{"name": name})

	certId, err := m.iam.UploadCertificate(name, certResource)
	if err != nil {
		return err
	}

	return m.cloudFront.SetCertificateAndCname(route.DistId, certId, route.GetDomains())
}

// todo (mxplusb): find a better way to do this.
func (m *RouteManager) GetDNSInstructions(route *Route) ([]string, error) {
	// we need to set this to a length of 3 so we can put the instructions in the proper place.
	instructions := make([]string, 3)

	for key := range m.dnsPresenter {
		if strings.Contains(key, "domain") {
			instructions[0] = fmt.Sprintf("name: %s", strings.Split(key, "domain")[0])
		}
		if strings.Contains(key, "keyauth") {
			instructions[1] = fmt.Sprintf("value: %s", strings.Split(key, "keyauth")[0])
		}
	}

	instructions[2] = fmt.Sprintf("ttl: 120")

	return instructions, nil
}
