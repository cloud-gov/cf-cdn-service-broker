package models

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"github.com/18F/cf-cdn-service-broker/utils"
	"github.com/jinzhu/gorm"
	"strings"
	"time"
)

const ProvisioningExpirationPeriodHours time.Duration = 84 * time.Hour

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

func (r *Route) GetDomains() []string {
	return strings.Split(r.DomainExternal, ",")
}

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

func isActivelyChanging(st State) bool {
	return st == Provisioning || st == Deprovisioning
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
