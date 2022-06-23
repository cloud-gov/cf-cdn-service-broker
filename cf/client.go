package cf

import (
	"net/url"

	"github.com/cloudfoundry-community/go-cfclient"
)

type Client interface {
	ListV3Domains(query url.Values) ([]cfclient.V3Domain, error)
	GetOrgByGuid(guid string) (cfclient.Org, error)
}
