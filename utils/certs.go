package utils

import (
	"crypto"
	"fmt"
	"net"

	"github.com/18F/cf-cdn-service-broker/lego/acme"
)

func preCheckDNS(fqdn, value string) (bool, error) {
	record, err := net.LookupTXT(fqdn)
	if err != nil {
		return false, err
	}
	if len(record) == 1 && record[0] == value {
		return true, nil
	}
	return false, fmt.Errorf("DNS precheck failed on name %s, value %s", fqdn, value)
}

func init() {
	acme.PreCheckDNS = preCheckDNS
}

type User struct {
	Email        string
	Registration *acme.RegistrationResource
	key          crypto.PrivateKey
}

func (u *User) GetEmail() string {
	return u.Email
}

func (u *User) GetRegistration() *acme.RegistrationResource {
	return u.Registration
}

func (u *User) GetPrivateKey() crypto.PrivateKey {
	return u.key
}

func (u *User) SetPrivateKey(key crypto.PrivateKey) {
	u.key = key
}
