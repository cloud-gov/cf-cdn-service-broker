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
	"github.com/18F/cf-cdn-service-broker/utils"
)

type CreateOptions struct {
	Domain     string   `json:"domain"`
	DefaultTTL int64    `json:"default_ttl"`
	Cookies    bool     `json:"cookies"`
	Headers    []string `json:"headers"`
}

type UpdateOptions struct {
	Domain     *string   `json:"domain,omitempty"`
	DefaultTTL *int64    `json:"default_ttl,omitempty"`
	Cookies    *bool     `json:"cookies,omitempty"`
	Headers    *[]string `json:"headers,omitempty"`
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
	lsession := logger.Session("broker")
	return &CdnServiceBroker{
		manager:  manager,
		cfclient: cfclient,
		settings: settings,
		logger:   lsession,
	}
}

var (
	MAX_HEADER_COUNT = 10
)

func (b *CdnServiceBroker) GetBinding(ctx context.Context, first, second string) (brokerapi.GetBindingSpec, error) {
	return brokerapi.GetBindingSpec{}, fmt.Errorf("GetBinding method not implemented")
}

func (b *CdnServiceBroker) GetInstance(ctx context.Context, first string) (brokerapi.GetInstanceDetailsSpec, error) {
	return brokerapi.GetInstanceDetailsSpec{}, fmt.Errorf("GetInstance method not implemented")
}

func (b *CdnServiceBroker) LastBindingOperation(ctx context.Context, first, second string, pollDetails brokerapi.PollDetails) (brokerapi.LastOperation, error) {
	return brokerapi.LastOperation{}, fmt.Errorf("LastBindingOperation method not implemented")
}

func (b *CdnServiceBroker) Services(context context.Context) ([]brokerapi.Service, error) {
	lsession := b.logger.Session("provision")
	lsession.Info("start")

	var service brokerapi.Service
	buf, err := ioutil.ReadFile("./catalog.json")
	if err != nil {
		lsession.Error("read-file", err)
		return []brokerapi.Service{}, err
	}
	err = json.Unmarshal(buf, &service)
	if err != nil {
		lsession.Error("unmarshal", err)
		return []brokerapi.Service{}, err
	}
	lsession.Info("ok", lager.Data{"service": service})
	return []brokerapi.Service{service}, nil
}

func (b *CdnServiceBroker) Provision(
	context context.Context,
	instanceID string,
	details brokerapi.ProvisionDetails,
	asyncAllowed bool,
) (brokerapi.ProvisionedServiceSpec, error) {
	lsession := b.logger.Session("provision", lager.Data{
		"instance_id": instanceID,
		"details":     details,
	})
	lsession.Info("start")

	spec := brokerapi.ProvisionedServiceSpec{}

	if !asyncAllowed {
		lsession.Error("async-not-allowed-err", brokerapi.ErrAsyncRequired)
		return spec, brokerapi.ErrAsyncRequired
	}

	options, err := b.parseProvisionDetails(details)
	if err != nil {
		lsession.Error("parse-options-err", err)
		return spec, err
	}
	lsession.Info("options", lager.Data{"options": options})

	_, err = b.manager.Get(instanceID)
	if err == nil {
		lsession.Error("manager-get-err", err)
		return spec, brokerapi.ErrInstanceAlreadyExists
	}

	headers, err := b.getHeaders(options.Headers)
	if err != nil {
		lsession.Error("get-headers-err", err)
		return spec, err
	}

	tags := map[string]string{
		"Organization":    details.OrganizationGUID,
		"Space":           details.SpaceGUID,
		"Service":         details.ServiceID,
		"ServiceInstance": instanceID,
		"Plan":            details.PlanID,
	}

	_, err = b.manager.Create(
		instanceID,
		options.Domain,
		b.settings.DefaultOrigin,
		options.DefaultTTL,
		headers,
		options.Cookies,
		tags,
	)
	if err != nil {
		lsession.Info("manager-create-err", lager.Data{
			"options": options,
			"tags":    tags,
			"err":     err,
		})
		return spec, err
	}

	lsession.Info("ok")
	return brokerapi.ProvisionedServiceSpec{IsAsync: true}, nil
}

func (b *CdnServiceBroker) LastOperation(
	context context.Context,
	instanceID string,
	pollDetails brokerapi.PollDetails,
) (brokerapi.LastOperation, error) {
	lsession := b.logger.Session("last-operation", lager.Data{
		"instance_id":    instanceID,
		"operation_data": pollDetails.OperationData,
	})
	lsession.Info("start")

	route, err := b.manager.Get(instanceID)
	if err != nil {
		lsession.Error("manager-get-err", err)
		return brokerapi.LastOperation{
			State:       brokerapi.Failed,
			Description: "Service instance not found",
		}, nil
	}

	lsession.Info("route-state", lager.Data{
		"instance_id": route.InstanceId,
		"domain":      route.DomainExternal,
		"state":       route.State,
	})

	switch route.State {
	case models.Provisioning:
		instructions, err := b.manager.GetDNSInstructions(route)
		if err != nil {
			lsession.Error("get-dns-instructions-err", err, lager.Data{
				"domain": route.DomainExternal,
				"state":  route.State,
			})
			return brokerapi.LastOperation{}, err
		}
		if len(instructions) != len(route.GetDomains()) {
			err = fmt.Errorf("Expected to find %d tokens; found %d", len(route.GetDomains()), len(instructions))
			lsession.Error("too-few-dns-instructions", err, lager.Data{
				"domain": route.DomainExternal,
				"state":  route.State,
			})
			return brokerapi.LastOperation{}, err
		}
		var description string

		cloudFrontCNAMES := []string{}
		for _, tenantDomain := range route.GetDomains() {
			cloudFrontCNAMES = append(
				cloudFrontCNAMES,
				fmt.Sprintf("%s => %s", tenantDomain, route.DomainInternal),
			)
		}

		description = fmt.Sprintf(
			`
Provisioning in progress.

Create the following CNAME records to direct traffic from your domains to your CDN route

%s

To validate ownership of the domain, set the following DNS records

%s
`,
			strings.Join(cloudFrontCNAMES, "\n"),
			strings.Join(instructions, "\n"),
		)

		lsession.Info("provisioning-ok", lager.Data{
			"domain":      route.DomainExternal,
			"state":       route.State,
			"description": description,
		})
		return brokerapi.LastOperation{
			State:       brokerapi.InProgress,
			Description: description,
		}, nil
	case models.Deprovisioning:
		description := fmt.Sprintf(
			"Deprovisioning in progress [%s => %s]; CDN domain %s",
			route.DomainExternal, route.Origin, route.DomainInternal,
		)
		lsession.Info("deprovisioning-ok", lager.Data{
			"domain":      route.DomainExternal,
			"state":       route.State,
			"description": description,
		})
		return brokerapi.LastOperation{
			State:       brokerapi.InProgress,
			Description: description,
		}, nil
	case models.Provisioned:
		description := fmt.Sprintf(
			"Service instance provisioned [%s => %s]; CDN domain %s",
			route.DomainExternal, route.Origin, route.DomainInternal,
		)
		lsession.Info("ok", lager.Data{
			"domain":      route.DomainExternal,
			"state":       route.State,
			"description": description,
		})
		return brokerapi.LastOperation{
			State:       brokerapi.Succeeded,
			Description: description,
		}, nil
	case models.Deprovisioned:
		description := fmt.Sprintf(
			"Service instance deprovisioned [%s => %s]; CDN domain %s",
			route.DomainExternal, route.Origin, route.DomainInternal,
		)
		lsession.Info("ok", lager.Data{
			"domain":      route.DomainExternal,
			"state":       route.State,
			"description": description,
		})
		return brokerapi.LastOperation{
			State:       brokerapi.Succeeded,
			Description: description,
		}, nil
	case models.Conflict:
		description := "One or more of the CNAMEs you provided are already associated with a different CDN"
		lsession.Info("conflict", lager.Data{
			"domain":      route.DomainExternal,
			"state":       route.State,
			"description": description,
		})
		return brokerapi.LastOperation{
			State:       brokerapi.Failed,
			Description: description,
		}, nil

	case models.Failed:
		fallthrough
	default:
		description := "Service instance stuck in unmanagable state."
		if route.IsProvisioningExpired() {
			description = fmt.Sprintf("Couldn't verify in 24h time slot. %s", description)
		}
		lsession.Info("unmanagable-state", lager.Data{
			"domain":      route.DomainExternal,
			"state":       route.State,
			"description": description,
		})
		return brokerapi.LastOperation{
			State:       brokerapi.Failed,
			Description: description,
		}, nil
	}
}

func (b *CdnServiceBroker) Deprovision(
	context context.Context,
	instanceID string,
	details brokerapi.DeprovisionDetails,
	asyncAllowed bool,
) (brokerapi.DeprovisionServiceSpec, error) {
	lsession := b.logger.Session("deprovision", lager.Data{
		"instance_id": instanceID,
		"details":     details,
	})
	lsession.Info("start")

	if !asyncAllowed {
		lsession.Error("async-not-allowed-err", brokerapi.ErrAsyncRequired)
		return brokerapi.DeprovisionServiceSpec{}, brokerapi.ErrAsyncRequired
	}

	route, err := b.manager.Get(instanceID)
	if err != nil {
		lsession.Error("manager-get-err", err)
		return brokerapi.DeprovisionServiceSpec{}, err
	}

	err = b.manager.Disable(route)
	if err != nil {
		lsession.Error("manager-disable-err", err, lager.Data{
			"domain": route.DomainExternal,
		})
		return brokerapi.DeprovisionServiceSpec{}, nil
	}

	lsession.Info("ok", lager.Data{"domain": route.DomainExternal})
	return brokerapi.DeprovisionServiceSpec{IsAsync: true}, nil
}

func (b *CdnServiceBroker) Bind(
	context context.Context,
	instanceID, bindingID string,
	details brokerapi.BindDetails,
	asyncAllowed bool,
) (brokerapi.Binding, error) {
	b.logger.Info("bind", lager.Data{
		"instance_id": instanceID,
		"binding_id":  bindingID,
		"details":     details,
	})

	return brokerapi.Binding{}, errors.New("service does not support bind")
}

func (b *CdnServiceBroker) Unbind(
	context context.Context,
	instanceID, bindingID string,
	details brokerapi.UnbindDetails,
	asyncAllowed bool,
) (brokerapi.UnbindSpec, error) {
	b.logger.Info("unbind", lager.Data{
		"instance_id": instanceID,
		"binding_id":  bindingID,
		"details":     details,
	})

	return brokerapi.UnbindSpec{}, errors.New("service does not support bind")
}

func (b *CdnServiceBroker) Update(
	context context.Context,
	instanceID string,
	details brokerapi.UpdateDetails,
	asyncAllowed bool,
) (brokerapi.UpdateServiceSpec, error) {
	b.logger.Info("update", lager.Data{
		"instance_id": instanceID,
		"details":     details,
	})

	if !asyncAllowed {
		return brokerapi.UpdateServiceSpec{}, brokerapi.ErrAsyncRequired
	}

	options, err := b.parseUpdateDetails(details)
	if err != nil {
		return brokerapi.UpdateServiceSpec{}, err
	}
	b.logger.Info("update-options", lager.Data{"instance_id": instanceID, "options": options})

	var headers *utils.Headers

	if options.Headers != nil {
		parsedHeaders, err := b.getHeaders(*options.Headers)
		if err != nil {
			return brokerapi.UpdateServiceSpec{}, err
		}
		headers = &parsedHeaders
	}

	provisioningAsync, err := b.manager.Update(
		instanceID,
		options.Domain,
		options.DefaultTTL,
		headers,
		options.Cookies,
	)
	if err != nil {
		return brokerapi.UpdateServiceSpec{}, err
	}

	return brokerapi.UpdateServiceSpec{IsAsync: provisioningAsync}, nil
}

// parseProvisionDetails will attempt to parse the update details and then verify that BOTH least "domain" and "origin"
// are provided.
func (b *CdnServiceBroker) parseProvisionDetails(details brokerapi.ProvisionDetails) (CreateOptions, error) {
	var err error
	options := CreateOptions{
		Cookies:    true,
		Headers:    []string{},
		DefaultTTL: b.settings.DefaultDefaultTTL,
	}

	if len(details.RawParameters) == 0 {
		return options, errors.New("must be invoked with configuration parameters")
	}

	err = json.Unmarshal(details.RawParameters, &options)
	if err != nil {
		return options, err
	}

	if options.Domain == "" {
		err = errors.New("must pass non-empty `domain`")
		return options, err
	}

	err = b.checkDomain(options.Domain, details.OrganizationGUID)
	if err != nil {
		return options, err
	}

	return options, err
}

// parseUpdateDetails will attempt to parse the update details and then verify that at least "domain" or "origin"
// are provided.
func (b *CdnServiceBroker) parseUpdateDetails(details brokerapi.UpdateDetails) (UpdateOptions, error) {
	var err error
	options := UpdateOptions{}

	if len(details.RawParameters) == 0 {
		return options, errors.New("must be invoked with configuration parameters")
	}

	err = json.Unmarshal(details.RawParameters, &options)
	if err != nil {
		return options, err
	}

	if options.Domain != nil {
		err = b.checkDomain(*options.Domain, details.PreviousValues.OrgID)
		if err != nil {
			return options, err
		}
	}

	return options, err
}

func (b *CdnServiceBroker) checkDomain(domain, orgGUID string) error {
	// domain can be a comma separated list so we need to check each one individually
	domains := strings.Split(domain, ",")
	var errorlist []string

	orgName := "<organization>"

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
		}
		return fmt.Errorf("Domain does not exist; create it with %s", errorlist[0])
	}

	return nil
}

func (b *CdnServiceBroker) getHeaders(headerNames []string) (utils.Headers, error) {
	var err error
	headers := utils.Headers{}
	for _, header := range headerNames {
		if headers.Contains(header) {
			err = fmt.Errorf("must not pass duplicated header '%s'", header)
			return headers, err
		}
		headers.Add(header)
	}

	// Forbid accompanying a wildcard with specific headers.
	if headers.Contains("*") && len(headers) > 1 {
		err = errors.New("must not pass whitelisted headers alongside wildcard")
		return headers, err
	}

	// Ensure the Host header is forwarded if using a CloudFoundry origin.
	if !headers.Contains("*") {
		headers.Add("Host")
	}

	if len(headers) > MAX_HEADER_COUNT {
		err = fmt.Errorf("must not set more than %d headers; got %d", MAX_HEADER_COUNT, len(headers))
		return headers, err
	}

	return headers, err
}
