package healthchecks

import (
	"crypto"

	"github.com/18F/cf-cdn-service-broker/config"
	"github.com/go-acme/lego/certcrypto"
	"github.com/go-acme/lego/lego"
	"github.com/go-acme/lego/registration"
)

type User struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}

func (u *User) GetEmail() string {
	return u.Email
}

func (u *User) GetRegistration() *registration.Resource {
	return u.Registration
}

func (u *User) GetPrivateKey() crypto.PrivateKey {
	return u.key
}

func LetsEncrypt(settings config.Settings) error {
	user := &User{key: "cheese"}
	_, err := lego.NewClient(&lego.Config{
		CADirURL: "",
		Certificate: lego.CertificateConfig{
			KeyType: certcrypto.RSA2048,
		},
		User: user,
	})
	return err
}
