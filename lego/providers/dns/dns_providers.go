// Factory for DNS providers
package dns

import (
	"fmt"

	"github.com/18F/cf-cdn-service-broker/lego/acme"
	"github.com/18F/cf-cdn-service-broker/lego/providers/dns/auroradns"
	"github.com/18F/cf-cdn-service-broker/lego/providers/dns/cloudflare"
	"github.com/18F/cf-cdn-service-broker/lego/providers/dns/digitalocean"
	"github.com/18F/cf-cdn-service-broker/lego/providers/dns/dnsimple"
	"github.com/18F/cf-cdn-service-broker/lego/providers/dns/dnsmadeeasy"
	"github.com/18F/cf-cdn-service-broker/lego/providers/dns/dnspod"
	"github.com/18F/cf-cdn-service-broker/lego/providers/dns/dyn"
	"github.com/18F/cf-cdn-service-broker/lego/providers/dns/gandi"
	"github.com/18F/cf-cdn-service-broker/lego/providers/dns/googlecloud"
	"github.com/18F/cf-cdn-service-broker/lego/providers/dns/linode"
	"github.com/18F/cf-cdn-service-broker/lego/providers/dns/namecheap"
	"github.com/18F/cf-cdn-service-broker/lego/providers/dns/ns1"
	"github.com/18F/cf-cdn-service-broker/lego/providers/dns/ovh"
	"github.com/18F/cf-cdn-service-broker/lego/providers/dns/pdns"
	"github.com/18F/cf-cdn-service-broker/lego/providers/dns/rackspace"
	"github.com/18F/cf-cdn-service-broker/lego/providers/dns/rfc2136"
	"github.com/18F/cf-cdn-service-broker/lego/providers/dns/route53"
	"github.com/18F/cf-cdn-service-broker/lego/providers/dns/vultr"
)

func NewDNSChallengeProviderByName(name string) (acme.ChallengeProvider, error) {
	var err error
	var provider acme.ChallengeProvider
	switch name {
	case "auroradns":
		provider, err = auroradns.NewDNSProvider()
	case "cloudflare":
		provider, err = cloudflare.NewDNSProvider()
	case "digitalocean":
		provider, err = digitalocean.NewDNSProvider()
	case "dnsimple":
		provider, err = dnsimple.NewDNSProvider()
	case "dnsmadeeasy":
		provider, err = dnsmadeeasy.NewDNSProvider()
	case "dnspod":
		provider, err = dnspod.NewDNSProvider()
	case "dyn":
		provider, err = dyn.NewDNSProvider()
	case "gandi":
		provider, err = gandi.NewDNSProvider()
	case "gcloud":
		provider, err = googlecloud.NewDNSProvider()
	case "linode":
		provider, err = linode.NewDNSProvider()
	case "manual":
		provider, err = acme.NewDNSProviderManual()
	case "namecheap":
		provider, err = namecheap.NewDNSProvider()
	case "rackspace":
		provider, err = rackspace.NewDNSProvider()
	case "route53":
		provider, err = route53.NewDNSProvider()
	case "rfc2136":
		provider, err = rfc2136.NewDNSProvider()
	case "vultr":
		provider, err = vultr.NewDNSProvider()
	case "ovh":
		provider, err = ovh.NewDNSProvider()
	case "pdns":
		provider, err = pdns.NewDNSProvider()
	case "ns1":
		provider, err = ns1.NewDNSProvider()
	default:
		err = fmt.Errorf("Unrecognised DNS provider: %s", name)
	}
	return provider, err
}
