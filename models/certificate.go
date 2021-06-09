package models

import (
	"github.com/jinzhu/gorm"
	"time"
)

const (
	CertificateStatusAttached   string = "attached"
	CertificateStatusValidating string = "validating"
	CertificateStatusDeleted    string = "deleted"
	CertificateStatusFailed     string = "failed"
)

type Certificate struct {
	gorm.Model
	RouteId     uint
	Domain      string
	CertURL     string
	Certificate []byte
	Expires     time.Time `gorm:"index"`
	// adding a certificateArn to this struct, so we can truck the requested/provisioned certificates by ACM
	CertificateArn    string `gorm:"not null"`
	CertificateStatus string `gorm:"not null"` //(Attached, Validating, Deleted, failed)
}
