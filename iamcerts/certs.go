package iamcerts

import (
	"github.com/xenolf/lego/acme"
)

type Certs interface {
	UploadCertificate(name string, cert acme.CertificateResource) (string, error)
	RenameCertificate(prev, next string) error
	DeleteCertificate(name string, allowError bool) error
}
