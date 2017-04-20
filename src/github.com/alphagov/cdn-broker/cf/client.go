package cf

import "github.com/cloudfoundry-community/go-cfclient"

type Client interface {
	GetDomainByName(name string) (cfclient.Domain, error)
	GetOrgByGuid(guid string) (cfclient.Org, error)
}
