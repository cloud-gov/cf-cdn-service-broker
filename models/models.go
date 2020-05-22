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
	"github.com/aws/aws-sdk-go/service/acm"
	"io/ioutil"
	"net/http"
	"path"
	"strings"
	"time"

	"code.cloudfoundry.org/lager"
	"github.com/18F/cf-cdn-service-broker/lego/acme"
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
	Provisioned    State = "provisioned"
	Deprovisioning State = "deprovisioning"
	Deprovisioned  State = "deprovisioned"
	Conflict       State = "conflict"
	Failed         State = "failed"
)

var (
	helperLogger = lager.NewLogger("helper-logger")
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
		err := fmt.Errorf("%s-is-incompatible", value)
		helperLogger.Session("state-scan").Error("scan-switch", err)
		return err
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
		helperLogger.Session("create-user").Error("rsa-generate-key", err)
		return user, err
	}
	user.SetPrivateKey(key)

	return user, nil
}

func (r *Route) SetUser(user utils.User) error {
	var err error
	userData := UserData{Email: user.GetEmail()}

	lsession := helperLogger.Session("save-user")

	userData.Key, err = savePrivateKey(user.GetPrivateKey())
	if err != nil {
		lsession.Error("save-private-key", err)
		return err
	}
	userData.Reg, err = json.Marshal(user)
	if err != nil {
		lsession.Error("json-marshal-user", err)
		return err
	}

	r.UserData = userData

	return nil
}

func LoadUser(userData UserData) (utils.User, error) {
	var user utils.User

	lsession := helperLogger.Session("load-user")

	if err := json.Unmarshal(userData.Reg, &user); err != nil {
		lsession.Error("json-unmarshal-user-data", err)
		return user, err
	}
	key, err := loadPrivateKey(userData.Key)
	if err != nil {
		lsession.Error("load-private-key", err)
		return user, err
	}
	user.SetPrivateKey(key)
	return user, nil
}

func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(&Route{}, &Certificate{}, &UserData{}).Error; err != nil {
		return err
	}
	db.Model(&UserData{}).RemoveIndex("uix_user_data_email")
	return nil
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
			helperLogger.Session("save-private-key").Error("marshal-ec-private-key", err)
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
	InstanceId                string `gorm:"not null;unique_index"`
	State                     State  `gorm:"not null;index"`
	ChallengeJSON             []byte
	DomainExternal            string
	DomainInternal            string
	DistId                    string
	Origin                    string
	Path                      string      // Always empty, should not remove because it is in DB
	InsecureOrigin            bool        // Always false, should not remove because it is in DB
	Certificate               Certificate //this is used by letsencrypt certs and will be removed once all our certs are migrated to ACM
	UserData                  UserData
	UserDataID                int
	User                      utils.User `gorm:"-"`
	DefaultTTL                int64      `gorm:"default:86400"`
	ProvisioningSince         *time.Time //using the same field to measure a time for Provisioning or Deprovisioning
	IsCertificateManagedByACM bool       `gorm:"default:false"` //using false as default value, so all the existing records will default to managed by LE
	Certificates              []Certificate
}

// BeforeCreate hook will change the value of Provisioning_since field to time.Now()
// if creating a route in 'Provisioning' state
// if the state is 'Provisioned' then the value should be 'nil'
func (r *Route) BeforeCreate(tx *gorm.DB) error {

	if r.State == Provisioning {
		t := time.Now()
		r.ProvisioningSince = &t
	}
	return nil
}

func (r *Route) BeforeUpdate(tx *gorm.DB) error {
	var originalStates []State
	err := tx.Find(&Route{InstanceId: r.InstanceId}).Pluck("state", &originalStates).Error

	if err != nil {
		return err
	}

	originalState := originalStates[0]

	if isActivelyChanging(originalState) && !isActivelyChanging(r.State) {
		r.ProvisioningSince = nil
	} else if !isActivelyChanging(originalState) && isActivelyChanging(r.State) {
		t := time.Now()
		r.ProvisioningSince = &t
	}

	return nil
}

func isActivelyChanging(st State) bool {
	return st == Provisioning || st == Deprovisioning
}

func (r *Route) GetDomains() []string {
	return strings.Split(r.DomainExternal, ",")
}

const ProvisioningExpirationPeriodHours time.Duration = 84 * time.Hour

//the issue of a certificate in ACM has a time out limit of 72 hours
//plus few (12) hours to account for any unexpected delays
//we've got to the magic number of 84 hours represented by the const - ProvisioningExpirationPeriodHours
func (r *Route) IsProvisioningExpired() bool {
	var provisioningSince *time.Time

	if r.ProvisioningSince != nil {
		provisioningSince = r.ProvisioningSince
	}
	return r.State == Provisioning && provisioningSince != nil &&
		(*provisioningSince).Before(time.Now().Add(-1*ProvisioningExpirationPeriodHours))
}

const (
	CertificateStatusAttached   string = "attached"
	CertificateStatusValidating string = "validating"
	CertificateStatusDeleted    string = "deleted"
	CertificateStatusFailed     string = "failed"
	CertificateStatusLE         string = "letsencrypt"
)

type Certificate struct {
	gorm.Model
	RouteId     uint
	Domain      string
	CertURL     string
	Certificate []byte
	Expires     time.Time `gorm:"index"`
	//adding a certificateArn to this struct, so we can truck the requested/provisioned certificates by ACM
	CertificateArn    string `gorm:"not null;default:'managedbyletsencrypt'"`
	CertificateStatus string `gorm:"not null;default:'letsencrypt'"` //(Attached, Validating, Detached, failed, letsencrypt)
}

type RouteManagerIface interface {
	Create(
		instanceId string,
		domain string,
		origin string,
		defaultTTL int64,
		forwardedHeaders utils.Headers,
		forwardCookies bool,
		tags map[string]string,
	) (*Route, error)

	Update(
		instanceId string,
		domain *string,
		defaultTTL *int64,
		forwardedHeaders *utils.Headers,
		forwardCookies *bool,
	) (bool, error)

	Get(instanceId string) (*Route, error)

	Poll(route *Route) error

	Disable(route *Route) error

	Renew(route *Route) error

	RenewAll()

	DeleteOrphanedCerts()

	GetDNSInstructions(route *Route) ([]string, error)
}

type RouteManager struct {
	logger             lager.Logger
	iam                utils.IamIface
	cloudFront         utils.DistributionIface
	settings           config.Settings
	acmeClientProvider AcmeClientProviderInterface
	routeStoreIface    RouteStoreInterface
	certsManager       utils.CertificateManagerInterface
}

func NewManager(
	logger lager.Logger,
	iam utils.IamIface,
	cloudFront utils.DistributionIface,
	settings config.Settings,
	acmeClientProvider AcmeClientProviderInterface,
	routeStoreIface RouteStoreInterface,
	certsManager utils.CertificateManagerInterface,
) RouteManager {
	return RouteManager{
		logger:             logger,
		iam:                iam,
		cloudFront:         cloudFront,
		settings:           settings,
		acmeClientProvider: acmeClientProvider,
		routeStoreIface:    routeStoreIface,
		certsManager:       certsManager,
	}
}

func (m *RouteManager) Create(
	instanceId,
	domain string,
	origin string,
	defaultTTL int64,
	forwardedHeaders utils.Headers,
	forwardCookies bool,
	tags map[string]string,
) (*Route, error) {

	route := &Route{
		InstanceId:                instanceId,
		State:                     Provisioning,
		DomainExternal:            domain,
		Origin:                    origin,
		Path:                      "",
		DefaultTTL:                defaultTTL,
		InsecureOrigin:            false,
		IsCertificateManagedByACM: true,
		Certificates:              []Certificate{},
	}

	lsession := m.logger.Session("route-manager-create-route", lager.Data{
		"instance-id": instanceId,
	})

	//Creating Cloud Front Distribution
	lsession.Info("create-cloudfront-instance")
	dist, err := m.cloudFront.Create(
		instanceId,
		make([]string, 0),
		origin,
		defaultTTL,
		forwardedHeaders,
		forwardCookies,
		tags,
	)
	if err != nil {
		lsession.Error("create-cloudfront-instance", err)
		return nil, err
	}

	route.DomainInternal = *dist.DomainName
	route.DistId = *dist.Id

	//request a certificate from ACM.
	lsession.Info("certsmanager-request-certificate")
	certArn, err := m.certsManager.RequestCertificate(route.GetDomains(), instanceId)
	if err != nil {
		lsession.Error("certsmanager-request-certificate", err)
		return nil, err
	}
	lsession.Info("certsmanager-request-certificate-done", lager.Data{"certificate-arn": *certArn})

	//WIP - it looks that the only data about ACM cert we need to persist is the ARN
	// everything else we can retrieve
	newCert := Certificate{
		CertificateArn:    *certArn,
		CertificateStatus: CertificateStatusValidating,
	}

	route.Certificates = append(route.Certificates, newCert)

	//insert the route object into the database
	lsession.Info("create-route")
	if err := m.routeStoreIface.Create(route); err != nil {
		lsession.Error("create-route", err)
		return nil, err
	}

	return route, nil
}

//Get a Route from a database, by instanceId
func (m *RouteManager) Get(instanceId string) (*Route, error) {
	route := Route{}

	lsession := m.logger.Session("route-manager-get", lager.Data{
		"instance-id": instanceId,
	})

	lsession.Info("db-first-route")
	route, err := m.routeStoreIface.FindOneMatching(Route{
		InstanceId: instanceId,
	})

	if err == nil {
		return &route, nil
	} else if err == gorm.ErrRecordNotFound {
		lsession.Error("db-record-not-found", brokerapi.ErrInstanceDoesNotExist)
		return nil, brokerapi.ErrInstanceDoesNotExist
	} else {
		lsession.Error("db-generic-error", err)
		return nil, err
	}
}

// Update function updates the CDN route service and returns whether the update has been
// performed asynchronously or not
// this function is ONLY called when a tenant will issue 'cf service-update'
func (m *RouteManager) Update(
	instanceId string,
	domain *string,
	defaultTTL *int64,
	forwardedHeaders *utils.Headers,
	forwardCookies *bool,
) (bool, error) {

	lsession := m.logger.Session("route-manager-update", lager.Data{
		"instance-id": instanceId,
	})

	// Get current route
	lsession.Info("get-route")
	route, err := m.Get(instanceId)
	if err != nil {
		lsession.Error("get-route", err)
		return false, err
	}

	// Override DefaultTTL settings that are new or different.
	if defaultTTL != nil {
		lsession.Info("param-update-default-ttl")
		route.DefaultTTL = *defaultTTL
	}

	// Update the distribution with new TTL, forwardHeaders and forwardCookies settings
	lsession.Info("cloudfront-update-excluding-domains")
	dist, err := m.cloudFront.Update(
		route.DistId,
		nil,
		route.Origin,
		defaultTTL,
		forwardedHeaders,
		forwardCookies,
	)
	if err != nil {
		lsession.Error("cloudfront-update-excluding-domains", err)
		return false, err
	}

	// Get the updated domain name and dist id.
	route.DomainInternal = *dist.DomainName
	route.DistId = *dist.Id

	if domain == nil || *domain == "" {
		lsession.Info("set-state-provisioned")
		// CloudFront has been updated with all the configuration
		// The domains are not being updated so we do not need a new certificate
		// The Update step is therefore now finished
		route.State = Provisioned
	} else {
		lsession.Info("set-state-provisioning")
		route.State = Provisioning

		if domain != nil {
			lsession.Info("param-update-domain")
			route.DomainExternal = *domain
		}
		//At this point we can assume that we will kick-off the Certificate Provisioning with ACM (even if it was provisioned with LE before)
		route.IsCertificateManagedByACM = true

		//request a certificate from ACM.
		lsession.Info("certsmanager-request-certificate")
		certArn, err := m.certsManager.RequestCertificate(route.GetDomains(), instanceId)
		if err != nil {
			lsession.Error("certsmanager-request-certificate", err)
			return false, err
		}
		lsession.Info("certsmanager-request-certificate-done", lager.Data{"certificate-arn": *certArn})

		//WIP - it looks that the only data about ACM cert we need to persist is the ARN
		// everything else we can retrieve
		newCert := Certificate{
			CertificateArn:    *certArn,
			CertificateStatus: CertificateStatusValidating,
		}

		route.Certificates = append(route.Certificates, newCert)
	}

	//save route object into the database
	lsession.Info("save-route")
	if err = m.routeStoreIface.Save(route); err != nil {
		lsession.Error("save-route", err)
		return false, err
	}

	performedAsynchronously := route.State == Provisioning
	return performedAsynchronously, nil
}

func (m *RouteManager) Poll(r *Route) error {
	lsession := m.logger.Session("route-manager-update", lager.Data{
		"instance-id": r.InstanceId,
	})
	switch r.State {
	case Provisioning:
		lsession.Info("update-provisioning")
		return m.updateProvisioning(r)
	case Deprovisioning:
		lsession.Info("update-deprovisioning")
		return m.updateDeprovisioning(r)
	default:
		lsession.Info("unexpected-state", lager.Data{"state": r.State})
		return nil
	}
}

func (m *RouteManager) Disable(r *Route) error {
	lsession := m.logger.Session("route-manager-disable", lager.Data{
		"instance-id": r.InstanceId,
	})

	lsession.Info("cloudfront-disable")
	err := m.cloudFront.Disable(r.DistId)
	if err != nil {
		lsession.Error("cloudfront-disable", err)
		return err
	}

	lsession.Info("save-route")
	r.State = Deprovisioning
	if err := m.routeStoreIface.Save(r); err != nil {
		lsession.Error("save-route", err)
	}

	return nil
}

func (m *RouteManager) stillActive(r *Route) error {
	lsession := m.logger.Session("route-manager-still-active", lager.Data{
		"instance-id": r.InstanceId,
	})

	lsession.Info("starting-canary-check", lager.Data{
		"settings":    m.settings,
		"instance-id": r.InstanceId,
	})

	session := session.New(aws.NewConfig().WithRegion(m.settings.AwsDefaultRegion))

	s3client := s3.New(session)

	target := path.Join(".well-known", "acme-challenge", "canary", r.InstanceId)

	input := s3.PutObjectInput{
		Bucket: aws.String(m.settings.Bucket),
		Key:    aws.String(target),
		Body:   strings.NewReader(r.InstanceId),
	}

	if m.settings.ServerSideEncryption != "" {
		input.ServerSideEncryption = aws.String(m.settings.ServerSideEncryption)
	}

	lsession.Info("s3-put-object", lager.Data{
		"bucket": m.settings.Bucket,
		"key":    target,
	})
	if _, err := s3client.PutObject(&input); err != nil {
		lsession.Error("s3-put-object", err)
		return err
	}

	insecureClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	lsession.Info("get-domains")
	for _, domain := range r.GetDomains() {
		lsession.Info("insecure-client-get", lager.Data{
			"domain": domain,
			"target": target,
		})
		resp, err := insecureClient.Get("https://" + path.Join(domain, target))
		if err != nil {
			lsession.Error("insecure-client-get", err)
			return err
		}

		defer resp.Body.Close()
		lsession.Info("read-response-body", lager.Data{
			"domain": domain,
			"target": target,
		})
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			lsession.Error("read-response-body", err)
			return err
		}

		lsession.Info("canary-check", lager.Data{
			"domain": domain,
			"target": target,
		})
		if string(body) != r.InstanceId {
			err := fmt.Errorf(
				"Canary check failed for %s; expected %s, got %s",
				domain, r.InstanceId, string(body),
			)
			lsession.Error("canary-check-failed", err)
			return err
		}
	}

	lsession.Info("finished")
	return nil
}

func (m *RouteManager) Renew(r *Route) error {
	lsession := m.logger.Session("route-manager-renew", lager.Data{
		"instance-id": r.InstanceId,
	})

	lsession.Info("check-still-active")
	err := m.stillActive(r)
	if err != nil {
		err := fmt.Errorf("Route is not active, skipping renewal: %v", err)
		lsession.Error("still-active", err)
		return err
	}

	// During Renew of the certificate we are using HTTP challange since we already
	// have control over the path used to prove the validity and 'ownership'  of the
	// domain (e.g. on behalf of the tenant)
	lsession.Info("get-http01-client")
	client, err := m.getHTTP01Client(&r.User, m.settings)
	if err != nil {
		lsession.Error("get-http01-client", err)
		return err
	}

	lsession.Info("obtain-certificate")
	certResource, errs := client.ObtainCertificate(r.GetDomains(), true, nil, false)
	if len(errs) > 0 {
		err := fmt.Errorf("Error(s) obtaining certificate: %v", errs)
		lsession.Error("obtain-certificate", err)
		return err
	}

	lsession.Info("get-pem-cert-expiry")
	expires, err := acme.GetPEMCertExpiration(certResource.Certificate)
	if err != nil {
		lsession.Error("get-pem-cert-expiry", err)
		return err
	}

	lsession.Info("deploy-certificate")
	if err := m.deployCertificate(*r, certResource); err != nil {
		lsession.Error("deploy-certificate", err)
		return err
	}

	r.Certificate.Domain = certResource.Domain
	r.Certificate.CertURL = certResource.CertURL
	r.Certificate.Certificate = certResource.Certificate
	r.Certificate.Expires = expires

	lsession.Info("save-route-cert", lager.Data{
		"domain":   r.Certificate.Domain,
		"cert-url": r.Certificate.CertURL,
		"expires":  r.Certificate.Expires,
	})
	if err := m.routeStoreIface.Save(r); err != nil {
		lsession.Error("save-route-cert", err)
		return err
	}

	lsession.Info("finished")
	return nil
}

func (m *RouteManager) DeleteOrphanedCerts() {
	lsession := m.logger.Session("delete-orphaned-certs")
	//first let us call the function that will clean up orphaned certs that
	//were issued by letsencrypt
	m.deleteOrphanedLetsEncryptCerts()

	m.deleteOrphanedACMCerts()

	lsession.Info("finished")
}

func (m *RouteManager) deleteOrphanedACMCerts() {
	lsession := m.logger.Session("delete-acm-managed-orphaned-certs")

	lsession.Info("list-issued-certificates")
	certs, err := m.certsManager.ListIssuedCertificates()
	if err != nil {
		lsession.Error("list-issued-certificates", err)
		return
	}

	time24hAgo := time.Now().Add(-24 * time.Hour)

	for _, cert := range certs {
		managedByCdnBroker := false

		for _, tag := range cert.Tags {
			if *tag.Key == utils.ManagedByTagName && *tag.Value == utils.ManagedByTagValue {
				managedByCdnBroker = true
				break
			}
		}

		isIssued := *cert.Status == acm.CertificateStatusIssued
		isInUse := len(cert.InUseBy) > 0
		olderThan24h := cert.IssuedAt.Before(time24hAgo)


		if isIssued && !isInUse &&  managedByCdnBroker && olderThan24h {
			lsession.Info("deleting-orphaned-cert", lager.Data{"certificate-arn": cert.CertificateArn})
			err = m.certsManager.DeleteCertificate(*cert.CertificateArn)
			if err != nil {
				lsession.Error("deleting-orphaned-cert", err, lager.Data{"certificate-arn": cert.CertificateArn})
			}
		} else {
			lsession.Info("not-deleting-certificate", lager.Data{
				"certificate-arn": *cert.CertificateArn,
				"is-issued": isIssued,
				"is-in-use": isInUse,
				"is-managed-by-cdn-broker": managedByCdnBroker,
				"is-older-than-24h": olderThan24h,
			})
		}
	}

	lsession.Info("finished")
}

func (m *RouteManager) deleteOrphanedLetsEncryptCerts() {
	lsession := m.logger.Session("delete-le-managed-orphaned-certs")
	// iterate over all distributions and record all certificates in-use by these distributions
	activeCerts := make(map[string]string)

	lsession.Info("list-distributions")
	err := m.cloudFront.ListDistributions(func(distro cloudfront.DistributionSummary) bool {
		if distro.ViewerCertificate.IAMCertificateId != nil {
			activeCerts[*distro.ViewerCertificate.IAMCertificateId] = *distro.ARN
		}
		return true
	})

	if err != nil {
		lsession.Error("cloudfront-list-distributions", err)
		return
	}

	// iterate over all certificates
	lsession.Info("list-certificates")
	err = m.iam.ListCertificates(func(cert iam.ServerCertificateMetadata) bool {

		// delete any certs not attached to a distribution that are older than 24 hours
		_, active := activeCerts[*cert.ServerCertificateId]
		if !active && time.Since(*cert.UploadDate).Hours() > 24 {
			lsession.Info("cleaning-orphaned-certificate", lager.Data{
				"cert": cert,
			})

			err := m.iam.DeleteCertificate(*cert.ServerCertificateName)
			if err != nil {
				lsession.Error("iam-delete-certificate", err, lager.Data{
					"cert": cert,
				})
			}
		} else {
			lsession.Info("skipping", lager.Data{
				"cert": cert,
			})
		}

		return true
	})

	if err != nil {
		lsession.Error("iam_list_certificates", err)
	}

	lsession.Info("finished")
}

func (m *RouteManager) RenewAll() {
	lsession := m.logger.Session("route-manager-renew-all")

	routes := []Route{}

	lsession.Info("Find routes that are expiring soon")

	routes, err := m.routeStoreIface.FindWithExpiringCerts()

	if err != nil {
		lsession.Error("find-certs-expiring-soon-error", err)
		return
	}

	lsession.Info("routes-needing-renewal", lager.Data{
		"num-routes": len(routes),
	})

	for _, route := range routes {
		err := m.Renew(&route)
		if err != nil {
			lsession.Error("renew-error", err, lager.Data{
				"domain": route.DomainExternal,
				"origin": route.Origin,
			})
		} else {
			lsession.Info("renew-success", lager.Data{
				"domain": route.DomainExternal,
				"origin": route.Origin,
			})
		}
	}
	lsession.Info("finished")
}

func (m *RouteManager) CheckRoutesToUpdate() {
	lsession := m.logger.Session("check-routes-to-update")

	lsession.Info("fetch-routes-to-update")
	routes, err := m.fetchRoutesToUpdate(lsession)

	if err != nil {
		lsession.Error("fetch-routes-to-update", err)
		return
	}

	if len(routes) == 0 {
		return
	}

	for _, route := range routes {
		lsession.Info("check", lager.Data{"instance_id": route.InstanceId})
		err := m.Poll(&route)
		if err != nil {
			lsession.Info("check-failed", lager.Data{"instance_id": route.InstanceId})

			if strings.Contains(err.Error(), "CNAMEAlreadyExists") {
				lsession.Info("cname-conflict", lager.Data{"instance_id": route.InstanceId, "domains": route.GetDomains()})

				route.State = Conflict
				lsession.Info("set-state", lager.Data{"instance_id": route.InstanceId, "state": route.State})
				err = m.routeStoreIface.Save(&route)
				if err != nil {
					lsession.Error("route-save-failed", err)
					continue
				}

			}

			if err == utils.ErrValidationTimedOut {
				lsession.Info("certificate-validation-timed-out", lager.Data{"instance_id": route.InstanceId, "domains": route.GetDomains()})
				route.State = Failed
				lsession.Info("set-state", lager.Data{"instance_id": route.InstanceId, "state": route.State})
				err = m.routeStoreIface.Save(&route)
				if err != nil {
					lsession.Error("route-save-failed", err)
					continue
				}
			}
		}

		lsession.Info("checking-provisioning-expiration", lager.Data{"instance_id": route.InstanceId, "provisioning_since": route.ProvisioningSince})
		if route.IsProvisioningExpired() {
			lsession.Info("expiring-unprovisioned-instance", lager.Data{
				"domain":             route.DomainExternal,
				"state":              route.State,
				"created_at":         route.CreatedAt,
				"provisioning_since": route.ProvisioningSince,
			})

			err = m.Disable(&route)
			if err != nil {
				lsession.Error("unable-to-expire-unprovisioned-instance", err, lager.Data{
					"domain":             route.DomainExternal,
					"state":              route.State,
					"created_at":         route.CreatedAt,
					"provisioning_since": route.ProvisioningSince,
				})

				route.State = Failed
				lsession.Info("set-state", lager.Data{"instance_id": route.InstanceId, "state": route.State})
				err = m.routeStoreIface.Save(&route)
				if err != nil {
					lsession.Error("route-save-failed", err)
					continue
				}
			}
		}
	}
}

func (m *RouteManager) fetchRoutesToUpdate(lsession lager.Logger) ([]Route, error) {
	provisioning := []Route{}

	lsession.Info("find-provisioning-instances")
	provisioning, err := m.routeStoreIface.FindAllMatching(Route{State: Provisioning})
	if err != nil {
		lsession.Error("find-provisioning-instances", err)
		return []Route{}, err
	}

	deprovisioning := []Route{}

	lsession.Info("find-deprovisioning-instances")
	deprovisioning, err = m.routeStoreIface.FindAllMatching(Route{State: Deprovisioning})
	if err != nil {
		lsession.Error("find-deprovisioning-instances", err)
		return []Route{}, err
	}

	routes := []Route{}
	routes = append(routes, provisioning...)
	routes = append(routes, deprovisioning...)

	affectedDomains := []string{}
	for _, route := range routes {
		affectedDomains = append(affectedDomains, route.DomainExternal)
	}

	if len(routes) > 0 {
		lsession.Info("found-instances", lager.Data{"domains": affectedDomains})
	} else {
		lsession.Info("found-no-instances")
	}

	return routes, nil
}

func (m *RouteManager) getDNS01Client(
	user *utils.User,
	settings config.Settings,
) (acme.ClientInterface, error) {

	lsession := m.logger.Session("route-manager-get-dns01-client")

	client, err := m.acmeClientProvider.GetDNS01Client(user, settings)

	if err != nil {
		lsession.Error("new-client-dns-builder", err)
		return client, err
	}

	lsession.Info("new-client-dns-builder-success")
	return client, nil
}

func (m *RouteManager) getHTTP01Client(
	user *utils.User,
	settings config.Settings,
) (acme.ClientInterface, error) {

	lsession := m.logger.Session("route-manager-get-http01-client")

	client, err := m.acmeClientProvider.GetHTTP01Client(user, settings)

	if err != nil {
		lsession.Error("new-client-http-builder", err)
		return client, err
	}

	lsession.Info("new-client-http-builder-success")
	return client, nil
}

func (m *RouteManager) updateProvisioning(r *Route) error {
	lsession := m.logger.Session("route-manager-update-provisioning", lager.Data{
		"instance-id": r.InstanceId,
	})

	lsession.Info("check-distribution")
	isDistributionDeployed := m.checkDistribution(r)

	if !isDistributionDeployed {
		//we are here when distribution provisioing
		lsession.Info("distribution-provisioning")
		return nil
	}

	desiredDomains := r.GetDomains()
	lsession.Info("get-currently-deployed-domains")
	deployedDomains, err := m.GetCurrentlyDeployedDomains(r)
	if err != nil {
		lsession.Error("get-currently-deployed-domains", err)
		return err
	}

	lsession.Info(
		"router-manager-update-provisioning",
		lager.Data{
			"deployed-domains": deployedDomains,
			"desired-domains":  desiredDomains,
		},
	)

	lsession.Info("find-the-most-recent-validating-certificate")
	var theMostRecentCert, theCurrentAttached *Certificate
	for i, e := range r.Certificates {
		if e.CertificateStatus == CertificateStatusValidating {
			if theMostRecentCert == nil {
				theMostRecentCert = &(r.Certificates[i])
			}

			if e.CreatedAt.After(theMostRecentCert.CreatedAt) {
				theMostRecentCert = &(r.Certificates[i])
			}
		}

		if e.CertificateStatus == CertificateStatusAttached {
			theCurrentAttached = &(r.Certificates[i])
		}
	}

	if theMostRecentCert == nil {
		err = errors.New("couldn't find the most recent certificate")
		lsession.Error("find-the-most-recent-validating-certificate-error", err)
		return err
	}

	lsession.Info("find-the-most-recent-validating-certificate-found", lager.Data{"CertificateArn": theMostRecentCert.CertificateArn})

	//we need to ensure that the certificate validation in ACM has finished and its status is 'ISSUED'
	lsession.Info("is-certificate-issued")
	issued, err := m.certsManager.IsCertificateIssued(theMostRecentCert.CertificateArn)
	if err != nil {
		lsession.Error("is-certificate-issued", err)
		if err == utils.ErrValidationTimedOut {
			theMostRecentCert.CertificateStatus = CertificateStatusFailed
		}
		return err
	}

	if !issued {
		lsession.Info("certificate-is-not-issued-yet")
		return nil
	}

	lsession.Info("deploy-certificate")
	if err := m.deployACMCertificate(*r, *theMostRecentCert); err != nil {
		lsession.Error("deploy-certificate", err)
		return err
	}

	//Swaping certificates from attached to detached
	if theCurrentAttached != nil {
		theCurrentAttached.CertificateStatus = CertificateStatusDeleted
	}

	//Set the new issued certificate as attached
	theMostRecentCert.CertificateStatus = CertificateStatusAttached

	lsession.Info("set-provisioned")
	r.State = Provisioned
	lsession.Info("save-route-provisioned")
	if err := m.routeStoreIface.Save(r); err != nil {
		lsession.Error("save-route-provisioned", err)
		return err
	}

	lsession.Info("finished")
	return nil
}

func (m *RouteManager) updateDeprovisioning(r *Route) error {
	lsession := m.logger.Session("route-manager-update-deprovisioning")

	lsession.Info("cloudfront-delete")
	deleted, err := m.cloudFront.Delete(r.DistId)
	if err != nil {
		lsession.Error("cloudfront-delete", err)
		return err
	}

	if deleted {
		r.State = Deprovisioned
		lsession.Info("save-route-deprovisioned")
		if err := m.routeStoreIface.Save(r); err != nil {
			lsession.Error("save-route-deprovisioned", err)
		}
	}

	lsession.Info("finished")
	return nil
}

func (m *RouteManager) checkDistribution(r *Route) bool {
	lsession := m.logger.Session("check-distribution", lager.Data{
		"instance-id": r.InstanceId,
	})

	lsession.Info("cloudfront-get")
	dist, err := m.cloudFront.Get(r.DistId)
	if err != nil {
		lsession.Error("cloudfront-get", err)
		return false
	}

	lsession.Info("finished", lager.Data{
		"status":  *dist.Status,
		"enabled": *dist.DistributionConfig.Enabled,
	})
	return *dist.Status == "Deployed" && *dist.DistributionConfig.Enabled
}

func (m *RouteManager) solveChallenges(
	logger lager.Logger,
	client acme.ClientInterface,
	challenges []acme.AuthorizationResource,
) map[string]error {

	startTime := time.Now()
	lsession := logger.Session("solve-challenge", lager.Data{
		"start_time": startTime.String(),
	})
	lsession.Info("solve-challenge-start")

	failures := client.SolveChallenges(challenges)
	endTime := time.Now()

	if len(failures) > 0 {
		lsession.Error("solve-challenges-errors", fmt.Errorf(
			"Encountered non-zero number of failures solving challenges",
		), lager.Data{
			"failures": failures,
			"end_time": endTime.String(),
			"duration": endTime.Sub(startTime).String(),
		})
	} else {
		lsession.Info("solve-challenges-success", lager.Data{
			"end_time": endTime.String(),
			"duration": endTime.Sub(startTime).String(),
		})
	}

	lsession.Info("finished", lager.Data{
		"failures": failures,
	})
	return failures
}

func (m *RouteManager) deployACMCertificate(
	route Route,
	cert Certificate,
) error {
	lsession := m.logger.Session("deploy-acm-certificate", lager.Data{
		"instance-id": route.InstanceId,
		"domains":     route.GetDomains(),
		"dist":        route.DistId,
	})

	lsession.Info("acm-set-certificate-and-cname", lager.Data{
		"cert_id": cert.CertificateArn,
	})
	err := m.cloudFront.SetCertificateAndCname(route.DistId, cert.CertificateArn, route.GetDomains(), true)
	if err != nil {
		lsession.Error("acm-set-certificate-and-cname", err, lager.Data{
			"cert_id": cert.CertificateArn,
		})
		return err
	}

	lsession.Info("finished")
	return nil
}

func (m *RouteManager) deployCertificate(
	route Route,
	cert acme.CertificateResource,
) error {

	lsession := m.logger.Session("deploy-certificate", lager.Data{
		"instance-id": route.InstanceId,
		"domains":     route.GetDomains(),
		"dist":        route.DistId,
	})

	lsession.Info("get-cert-expiry")
	expires, err := acme.GetPEMCertExpiration(cert.Certificate)
	if err != nil {
		lsession.Error("get-cert-expiry-err", err)
		return err
	}

	name := fmt.Sprintf(
		"cdn-route-%s-%s",
		route.InstanceId,
		expires.Format("2006-01-02_15-04-05"),
	)
	lsession.Info("iam-upload-certificate", lager.Data{"name": name})
	certId, err := m.iam.UploadCertificate(name, cert)
	if err != nil {
		lsession.Error("iam-upload-certificate-err", err, lager.Data{
			"name": name,
		})
		return err
	}

	lsession.Info("iam-set-certificate-and-cname", lager.Data{
		"name":    name,
		"cert_id": certId,
	})
	err = m.cloudFront.SetCertificateAndCname(route.DistId, certId, route.GetDomains(), false)
	if err != nil {
		lsession.Error("iam-set-certificate-and-cname-err", err, lager.Data{
			"name":    name,
			"cert_id": certId,
		})
		return err
	}

	lsession.Info("finished")
	return nil
}

//ToDo - will not need that with ACM (probably)
//since we aren't going to save challenges in the database
//we can always ask for them from ACM.
//WIP - looks like if we don't have the challenges persisted in the database (1st time we call this function)
//we will extract these and then save them in the DB.
//worth checking when and where are we using this JSON again.
//
func (m *RouteManager) ensureChallenges(
	route *Route,
	client acme.ClientInterface,
) error {

	lsession := m.logger.Session("ensure-challenges", lager.Data{
		"instance-id": route.InstanceId,
		"domains":     route.GetDomains(),
	})

	if len(route.ChallengeJSON) > 0 {
		lsession.Info("challenge-json-was-already-present")
		return nil
	}

	lsession.Info("get-challenges")
	challenges, errs := client.GetChallenges(route.GetDomains())
	if len(errs) > 0 {
		err := fmt.Errorf("Error(s) getting challenges: %v", errs)
		lsession.Error("get-challenges-err", err)
		return err
	}

	lsession.Info("unmarshal-challenges")
	var err error
	route.ChallengeJSON, err = json.Marshal(challenges)
	if err != nil {
		lsession.Error("json-marshal-challenge-err", err)
		return err
	}

	lsession.Info("finished")
	return nil
}

//This function will have a fork/dual behaviour for certs that were provisioned by LE or ACM
//LE certs challenges are kept in the DB
//ACM certs challenges are aquired dynamically via an API call
func (m *RouteManager) GetDNSInstructions(route *Route) ([]string, error) {
	var instructions []string

	lsession := m.logger.Session("get-dns-instructions", lager.Data{
		"instance-id": route.InstanceId,
		"domains":     route.GetDomains(),
	})

	if route.IsCertificateManagedByACM {
		lsession.Info("certsmanager-get-validation-challenges")
		validationChallenges, err := m.certsManager.GetDomainValidationChallenges(route.Certificate.CertificateArn)
		if err != nil {
			lsession.Error("certsmanager-get-validation-challenges", err)
			return []string{}, err
		}

		for _, e := range validationChallenges {



			if e.RecordName == "" {
				instructions = append(instructions, fmt.Sprintf(
					"Awaiting challenges for %s",
					e.DomainName,
				))
			} else {
				// Keep the new lines in this format
				format := `

For domain %s, set DNS record
    Name:  %s
    Type:  %s
    Value: %s
    TTL:   %d

Current validation status of %s: %s

`
				instructions = append(instructions, fmt.Sprintf(
					format,
					e.DomainName,
					e.RecordName,
					e.RecordType,
					strings.Trim(e.RecordValue, " "),
					route.DefaultTTL,
					e.DomainName,
					e.ValidationStatus,
				))
			}
		}
	} else {
		var challenges []acme.AuthorizationResource

		lsession.Info("json-unmarshal-challenge")
		if err := json.Unmarshal(route.ChallengeJSON, &challenges); err != nil {
			lsession.Error("json-unmarshal-challenge", err)
			return instructions, err
		}

		lsession.Info("get-key-authorization")
		for _, auth := range challenges {
			for _, challenge := range auth.Body.Challenges {
				if challenge.Type == acme.DNS01 {
					lsession.Info(
						"get-key-authorization-for-a-dns-challenge",
						lager.Data{
							"domain": auth.Domain,
						},
					)
					keyAuth, err := acme.GetKeyAuthorization(
						challenge.Token,
						route.User.GetPrivateKey(),
					)
					if err != nil {
						lsession.Error("get-key-authorization-err", err)
						return instructions, err
					}
					fqdn, value, ttl := acme.DNS01Record(auth.Domain, keyAuth)
					instructions = append(instructions, fmt.Sprintf(
						"name: %s, value: %s, ttl: %d, type: TXT",
						fqdn, value, ttl,
					))
				}
			}
		}

	}

	lsession.Info("finished")
	return instructions, nil
}

func (m *RouteManager) GetCurrentlyDeployedDomains(r *Route) ([]string, error) {
	lsession := m.logger.Session("get-currently-deployed-domains")

	lsession.Info("cloudfront-get-start")
	dist, err := m.cloudFront.Get(r.DistId)
	if err != nil {
		lsession.Error("cloudfront-get-error", err)

		return []string{}, err
	}
	lsession.Info("cloudfront-get-done")

	deployedDomains := []string{}
	for _, domain := range dist.DistributionConfig.Aliases.Items {
		deployedDomains = append(deployedDomains, *domain)
	}

	lsession.Info("finished")
	return deployedDomains, nil
}
