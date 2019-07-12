package healthchecks

import (
	"crypto"
	"github.com/18F/cf-cdn-service-broker/lego/acme"

	"github.com/18F/cf-cdn-service-broker/config"
)

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

func LetsEncrypt(settings config.Settings) error {
	user := &User{key: "cheese"}
	_, err := acme.NewClient("https://acme-v01.api.letsencrypt.org/directory", user, acme.RSA2048)
	return err
}
