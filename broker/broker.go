package broker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"

	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi"

	"github.com/18F/cf-cdn-service-broker/cf"
	"github.com/18F/cf-cdn-service-broker/config"
	"github.com/18F/cf-cdn-service-broker/models"
)

type Options struct {
	Domain         string `json:"domain"`
	Origin         string `json:"origin"`
	Path           string `json:"path"`
	InsecureOrigin bool   `json:"insecure_origin"`
	Cookies        bool   `json:"cookies"`
}

type CdnServiceBroker struct {
	manager  models.RouteManagerIface
	cfclient cf.Client
	settings config.Settings
	logger   lager.Logger
}

func New(
	manager models.RouteManagerIface,
	cfclient cf.Client,
	settings config.Settings,
	logger lager.Logger,
) *CdnServiceBroker {
	return &CdnServiceBroker{
		manager:  manager,
		cfclient: cfclient,
		settings: settings,
		logger:   logger,
	}
}

func (*CdnServiceBroker) Services(context context.Context) []brokerapi.Service {
	var service brokerapi.Service
	buf, err := ioutil.ReadFile("./catalog.json")
	if err != nil {
		return []brokerapi.Service{}
	}
	err = json.Unmarshal(buf, &service)
	if err != nil {
		return []brokerapi.Service{}
	}
	return []brokerapi.Service{service}
}

func (b *CdnServiceBroker) Provision(
	context context.Context,
	instanceID string,
	details brokerapi.ProvisionDetails,
	asyncAllowed bool,
) (brokerapi.ProvisionedServiceSpec, error) {
	spec := brokerapi.ProvisionedServiceSpec{}

	if !asyncAllowed {
		return spec, brokerapi.ErrAsyncRequired
	}

	options, err := b.parseProvisionDetails(details)
	if err != nil {
		return spec, err
	}

	_, err = b.manager.Get(instanceID)
	if err == nil {
		return spec, brokerapi.ErrInstanceAlreadyExists
	}

	headers := b.getHeaders(options)

	tags := map[string]string{
		"Organization": details.OrganizationGUID,
		"Space":        details.SpaceGUID,
		"Service":      details.ServiceID,
		"Plan":         details.PlanID,
	}

	_, err = b.manager.Create(instanceID, options.Domain, options.Origin, options.Path, options.InsecureOrigin, headers, options.Cookies, tags)
	if err != nil {
		return spec, err
	}

	return brokerapi.ProvisionedServiceSpec{IsAsync: true}, nil
}

func (b *CdnServiceBroker) LastOperation(
	context context.Context,
	instanceID, operationData string,
) (brokerapi.LastOperation, error) {
	route, err := b.manager.Get(instanceID)
	if err != nil {
		return brokerapi.LastOperation{
			State:       brokerapi.Failed,
			Description: "Service instance not found",
		}, nil
	}

	err = b.manager.Poll(route)
	if err != nil {
		b.logger.Error("Error during update", err, lager.Data{
			"domain": route.DomainExternal,
			"state":  route.State,
		})
	}

	switch route.State {
	case models.Provisioning:
		return brokerapi.LastOperation{
			State: brokerapi.InProgress,
			Description: fmt.Sprintf(
				"Provisioning in progress [%s => %s]; CNAME domain %s to %s.",
				route.DomainExternal, route.Origin, route.DomainExternal, route.DomainInternal,
			),
		}, nil
	case models.Deprovisioning:
		return brokerapi.LastOperation{
			State: brokerapi.InProgress,
			Description: fmt.Sprintf(
				"Deprovisioning in progress [%s => %s]",
				route.DomainExternal, route.Origin,
			),
		}, nil
	default:
		return brokerapi.LastOperation{
			State: brokerapi.Succeeded,
			Description: fmt.Sprintf(
				"Service instance provisioned [%s => %s]",
				route.DomainExternal, route.Origin,
			),
		}, nil
	}
}

func (b *CdnServiceBroker) Deprovision(
	context context.Context,
	instanceID string,
	details brokerapi.DeprovisionDetails,
	asyncAllowed bool,
) (brokerapi.DeprovisionServiceSpec, error) {
	if !asyncAllowed {
		return brokerapi.DeprovisionServiceSpec{}, brokerapi.ErrAsyncRequired
	}

	route, err := b.manager.Get(instanceID)
	if err != nil {
		return brokerapi.DeprovisionServiceSpec{}, err
	}

	err = b.manager.Disable(route)
	if err != nil {
		return brokerapi.DeprovisionServiceSpec{}, nil
	}

	return brokerapi.DeprovisionServiceSpec{IsAsync: true}, nil
}

func (b *CdnServiceBroker) Bind(
	context context.Context,
	instanceID, bindingID string,
	details brokerapi.BindDetails,
) (brokerapi.Binding, error) {
	return brokerapi.Binding{}, errors.New("service does not support bind")
}

func (b *CdnServiceBroker) Unbind(
	context context.Context,
	instanceID, bindingID string,
	details brokerapi.UnbindDetails,
) error {
	return errors.New("service does not support bind")
}

func (b *CdnServiceBroker) Update(
	context context.Context,
	instanceID string,
	details brokerapi.UpdateDetails,
	asyncAllowed bool,
) (brokerapi.UpdateServiceSpec, error) {
	if !asyncAllowed {
		return brokerapi.UpdateServiceSpec{}, brokerapi.ErrAsyncRequired
	}

	options, err := b.parseUpdateDetails(details)
	if err != nil {
		return brokerapi.UpdateServiceSpec{}, err
	}

	headers := b.getHeaders(options)

	err = b.manager.Update(instanceID, options.Domain, options.Origin, options.Path, options.InsecureOrigin, headers, options.Cookies)
	if err != nil {
		return brokerapi.UpdateServiceSpec{}, err
	}

	return brokerapi.UpdateServiceSpec{IsAsync: true}, nil
}

// createBrokerOptions will attempt to take raw json and convert it into the "Options" struct.
func (b *CdnServiceBroker) createBrokerOptions(details []byte) (options Options, err error) {
	if len(details) == 0 {
		err = errors.New("must be invoked with configuration parameters")
		return
	}
	options = Options{
		Origin:  b.settings.DefaultOrigin,
		Cookies: true,
	}
	err = json.Unmarshal(details, &options)
	if err != nil {
		return
	}
	return
}

// parseProvisionDetails will attempt to parse the update details and then verify that BOTH least "domain" and "origin"
// are provided.
func (b *CdnServiceBroker) parseProvisionDetails(details brokerapi.ProvisionDetails) (options Options, err error) {
	options, err = b.createBrokerOptions(details.RawParameters)
	if err != nil {
		return
	}
	if options.Domain == "" {
		err = errors.New("must pass non-empty `domain`")
		return
	}
	if options.Origin == b.settings.DefaultOrigin {
		err = b.checkDomain(options.Domain, details.OrganizationGUID)
		if err != nil {
			return
		}
	}
	return
}

// parseUpdateDetails will attempt to parse the update details and then verify that at least "domain" or "origin"
// are provided.
func (b *CdnServiceBroker) parseUpdateDetails(details brokerapi.UpdateDetails) (options Options, err error) {
	rawJSON, err := json.Marshal(details.Parameters)
	if err != nil {
		return
	}
	options, err = b.createBrokerOptions(rawJSON)
	if err != nil {
		return
	}
	if options.Domain == "" && options.Origin == "" {
		err = errors.New("must pass non-empty `domain` or `origin`")
		return
	}
	if options.Domain != "" && options.Origin == b.settings.DefaultOrigin {
		err = b.checkDomain(options.Domain, details.PreviousValues.OrgID)
		if err != nil {
			return
		}
	}
	return
}

func (b *CdnServiceBroker) checkDomain(domain, orgGUID string) error {
	// domain can be a comma separated list so we need to check each one individually
	domains := strings.Split(domain, ",")
	var errorlist []string

	var orgName string = "<organization>"

	for i := range domains {
		if _, err := b.cfclient.GetDomainByName(domains[i]); err != nil {

			if orgName == "<organization>" {
				org, err := b.cfclient.GetOrgByGuid(orgGUID)
				if err == nil {
					orgName = org.Name
				}
			}

			errorlist = append(errorlist, fmt.Sprintf("`cf create-domain %s %s`", orgName, domains[i]))
		}
	}

	if len(errorlist) > 0 {
		if len(errorlist) > 1 {
			return fmt.Errorf("Multiple domains do not exist; create them with:\n%s", strings.Join(errorlist, "\n"))
		} else {
			return fmt.Errorf("Domain does not exist; create it with %s", errorlist[0])
		}
	}

	return nil
}

func (b *CdnServiceBroker) getHeaders(options Options) []string {
	if options.Origin == b.settings.DefaultOrigin {
		return []string{"Host"}
	}
	return []string{}
}
