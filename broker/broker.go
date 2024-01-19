package broker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/pivotal-cf/brokerapi/v10/domain/apiresponses"

	"code.cloudfoundry.org/lager/v3"
	"github.com/pivotal-cf/brokerapi/v10/domain"

	"github.com/alphagov/paas-cdn-broker/cf"
	"github.com/alphagov/paas-cdn-broker/config"
	"github.com/alphagov/paas-cdn-broker/models"
	"github.com/alphagov/paas-cdn-broker/utils"
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
	MaxHeaderCount = 10

	// not very strict - only to prevent extreme mischief
	isValidDomain = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9.-]*[A-Za-z0-9]$").MatchString

	errDomainOrParentNotFound = errors.New("domain-or-parent-not-found")
)

func (b *CdnServiceBroker) GetBinding(ctx context.Context, first, second string, details domain.FetchBindingDetails) (domain.GetBindingSpec, error) {
	return domain.GetBindingSpec{}, fmt.Errorf("GetBinding method not implemented")
}

func (b *CdnServiceBroker) GetInstance(ctx context.Context, instanceID string, details domain.FetchInstanceDetails) (domain.GetInstanceDetailsSpec, error) {
	lsession := b.logger.Session("get-instance", lager.Data{
		"instance_id": instanceID,
	})

	lsession.Info("lookup-instance")
	route, err := b.manager.Get(instanceID)

	if err != nil {
		if err == apiresponses.ErrInstanceDoesNotExist {
			lsession.Error("instance-does-not-exist", err)
			return domain.GetInstanceDetailsSpec{}, apiresponses.ErrInstanceDoesNotExist
		} else {
			lsession.Error("lookup-instance", err)
			return domain.GetInstanceDetailsSpec{}, err
		}
	}

	lsession.Info("get-dns-challenges")
	challenges, err := b.manager.GetDNSChallenges(route, false)
	if err != nil {
		lsession.Error("get-dns-challenges", err)
		return domain.GetInstanceDetailsSpec{}, fmt.Errorf("could not get dns challenges for domain")
	}

	lsession.Info("get-cdn-configuration")
	distribution, err := b.manager.GetCDNConfiguration(route)
	if err != nil {
		lsession.Error("get-cdn-configuration", err)
		return domain.GetInstanceDetailsSpec{}, fmt.Errorf("could not get cdn configuration")
	}

	headers := []string{}
	for _, h := range distribution.DistributionConfig.DefaultCacheBehavior.ForwardedValues.Headers.Items {
		headers = append(headers, aws.StringValue(h))
	}

	forwardCookies := aws.StringValue(distribution.DistributionConfig.DefaultCacheBehavior.ForwardedValues.Cookies.Forward) == "all"
	cacheTTL := aws.Int64Value(distribution.DistributionConfig.DefaultCacheBehavior.DefaultTTL)
	distributionId := aws.StringValue(distribution.Id)

	instanceParams := map[string]interface{}{
		"cloudfront_distribution_id": distributionId,
		"cloudfront_domain":          route.DomainInternal,
		"dns_records":                challenges,
		"forwarded_headers":          headers,
		"forward_cookies":            forwardCookies,
		"cache_ttl":                  cacheTTL,
	}

	return domain.GetInstanceDetailsSpec{
		Parameters: instanceParams,
	}, nil
}

func (b *CdnServiceBroker) LastBindingOperation(ctx context.Context, first, second string, pollDetails domain.PollDetails) (domain.LastOperation, error) {
	return domain.LastOperation{}, fmt.Errorf("LastBindingOperation method not implemented")
}

func (b *CdnServiceBroker) Services(context context.Context) ([]domain.Service, error) {
	lsession := b.logger.Session("services")
	lsession.Info("start")

	var service domain.Service
	buf, err := ioutil.ReadFile("./catalog.json")
	if err != nil {
		lsession.Error("read-file", err)
		return []domain.Service{}, err
	}
	err = json.Unmarshal(buf, &service)
	if err != nil {
		lsession.Error("unmarshal", err)
		return []domain.Service{}, err
	}
	lsession.Info("ok", lager.Data{"service": service})
	return []domain.Service{service}, nil
}

func (b *CdnServiceBroker) Provision(
	context context.Context,
	instanceID string,
	details domain.ProvisionDetails,
	asyncAllowed bool,
) (domain.ProvisionedServiceSpec, error) {
	lsession := b.logger.Session("provision", lager.Data{
		"instance_id": instanceID,
		"details":     details,
	})
	lsession.Info("start")

	spec := domain.ProvisionedServiceSpec{}

	if !asyncAllowed {
		lsession.Error("async-not-allowed-err", apiresponses.ErrAsyncRequired)
		return spec, apiresponses.ErrAsyncRequired
	}

	options, err := b.parseProvisionDetails(lsession, details)
	if err != nil {
		lsession.Error("parse-options-err", err)
		return spec, err
	}
	lsession.Info("options", lager.Data{"options": options})

	_, err = b.manager.Get(instanceID)
	if err == nil {
		lsession.Error("manager-get-err", err)
		return spec, apiresponses.ErrInstanceAlreadyExists
	}

	headers, err := b.getHeaders(options.Headers)
	if err != nil {
		lsession.Error("get-headers-err", err)
		return spec, err
	}

	tags := map[string]string{
		"Organization":      details.OrganizationGUID,
		"Space":             details.SpaceGUID,
		"Service":           details.ServiceID,
		"ServiceInstance":   instanceID,
		"Plan":              details.PlanID,
		"chargeable_entity": instanceID,
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
	return domain.ProvisionedServiceSpec{IsAsync: true}, nil
}

func (b *CdnServiceBroker) LastOperation(
	context context.Context,
	instanceID string,
	pollDetails domain.PollDetails,
) (domain.LastOperation, error) {
	lsession := b.logger.Session("last-operation", lager.Data{
		"instance_id":    instanceID,
		"operation_data": pollDetails.OperationData,
	})
	lsession.Info("start")

	route, err := b.manager.Get(instanceID)
	if err != nil {
		lsession.Error("manager-get-err", err)
		return domain.LastOperation{
			State:       domain.Failed,
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
		challenges, err := b.manager.GetDNSChallenges(route, true)
		if err != nil {
			lsession.Error("get-dns-instructions-err", err, lager.Data{
				"domain": route.DomainExternal,
				"state":  route.State,
			})
			return domain.LastOperation{}, err
		}

		instructions := formatChallenges(challenges, route.DefaultTTL)

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
		return domain.LastOperation{
			State:       domain.InProgress,
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
		return domain.LastOperation{
			State:       domain.InProgress,
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
		return domain.LastOperation{
			State:       domain.Succeeded,
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
		return domain.LastOperation{
			State:       domain.Succeeded,
			Description: description,
		}, nil
	case models.Conflict:
		description := "One or more of the CNAMEs you provided are already associated with a different CDN"
		lsession.Info("conflict", lager.Data{
			"domain":      route.DomainExternal,
			"state":       route.State,
			"description": description,
		})
		return domain.LastOperation{
			State:       domain.Failed,
			Description: description,
		}, nil

	case models.TimedOut:
		lsession.Info("timed-out")
		description := fmt.Sprintf(
			`Create/update operation has timed out. Operations have %d hours to complete before expiring

Create/update operations usually expire because the domain validation DNS records have not been set.  
`,
			int(models.ProvisioningExpirationPeriodHours.Hours()),
		)

		return domain.LastOperation{
			State:       domain.Failed,
			Description: description,
		}, nil

	case models.Failed:
		fallthrough
	default:
		description := "Service instance stuck in unmanageable state."
		lsession.Info("unmanageable-state", lager.Data{
			"domain":      route.DomainExternal,
			"state":       route.State,
			"description": description,
		})
		return domain.LastOperation{
			State:       domain.Failed,
			Description: description,
		}, nil
	}
}

func formatChallenges(challenges []utils.DomainValidationChallenge, ttl int64) []string {
	instructions := []string{}
	for _, e := range challenges {

		if e.RecordName == "" {
			instructions = append(instructions, fmt.Sprintf(
				"Awaiting challenges for %s",
				e.DomainName,
			))
		} else {
			// Keep the new lines in this format
			format := `

For domain %s, set DNS record
    Name:  %s
    Type:  %s
    Value: %s
    TTL:   %d

Current validation status of %s: %s

`
			instructions = append(instructions, fmt.Sprintf(
				format,
				e.DomainName,
				e.RecordName,
				e.RecordType,
				strings.Trim(e.RecordValue, " "),
				ttl,
				e.DomainName,
				e.ValidationStatus,
			))
		}
	}

	return instructions
}

func (b *CdnServiceBroker) Deprovision(
	context context.Context,
	instanceID string,
	details domain.DeprovisionDetails,
	asyncAllowed bool,
) (domain.DeprovisionServiceSpec, error) {
	lsession := b.logger.Session("deprovision", lager.Data{
		"instance_id": instanceID,
		"details":     details,
	})
	lsession.Info("start")

	if !asyncAllowed {
		lsession.Error("async-not-allowed-err", apiresponses.ErrAsyncRequired)
		return domain.DeprovisionServiceSpec{}, apiresponses.ErrAsyncRequired
	}

	route, err := b.manager.Get(instanceID)
	if err != nil {
		lsession.Error("manager-get-err", err)
		return domain.DeprovisionServiceSpec{}, err
	}

	err = b.manager.Disable(route)
	if err != nil {
		lsession.Error("manager-disable-err", err, lager.Data{
			"domain": route.DomainExternal,
		})
		return domain.DeprovisionServiceSpec{}, nil
	}

	lsession.Info("ok", lager.Data{"domain": route.DomainExternal})
	return domain.DeprovisionServiceSpec{IsAsync: true}, nil
}

func (b *CdnServiceBroker) Bind(
	context context.Context,
	instanceID, bindingID string,
	details domain.BindDetails,
	asyncAllowed bool,
) (domain.Binding, error) {
	b.logger.Info("bind", lager.Data{
		"instance_id": instanceID,
		"binding_id":  bindingID,
		"details":     details,
	})

	return domain.Binding{}, errors.New("service does not support bind")
}

func (b *CdnServiceBroker) Unbind(
	context context.Context,
	instanceID, bindingID string,
	details domain.UnbindDetails,
	asyncAllowed bool,
) (domain.UnbindSpec, error) {
	b.logger.Info("unbind", lager.Data{
		"instance_id": instanceID,
		"binding_id":  bindingID,
		"details":     details,
	})

	return domain.UnbindSpec{}, errors.New("service does not support bind")
}

func (b *CdnServiceBroker) Update(
	context context.Context,
	instanceID string,
	details domain.UpdateDetails,
	asyncAllowed bool,
) (domain.UpdateServiceSpec, error) {
	b.logger.Info("update", lager.Data{
		"instance_id": instanceID,
		"details":     details,
	})

	if !asyncAllowed {
		return domain.UpdateServiceSpec{}, apiresponses.ErrAsyncRequired
	}

	options, err := b.parseUpdateDetails(b.logger, details)
	if err != nil {
		return domain.UpdateServiceSpec{}, err
	}
	b.logger.Info("update-options", lager.Data{"instance_id": instanceID, "options": options})

	var headers *utils.Headers

	if options.Headers != nil {
		parsedHeaders, err := b.getHeaders(*options.Headers)
		if err != nil {
			return domain.UpdateServiceSpec{}, err
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
		return domain.UpdateServiceSpec{}, err
	}

	return domain.UpdateServiceSpec{IsAsync: provisioningAsync}, nil
}

// parseProvisionDetails will attempt to parse the update details and then verify that BOTH least "domain" and "origin"
// are provided.
func (b *CdnServiceBroker) parseProvisionDetails(logger lager.Logger, details domain.ProvisionDetails) (CreateOptions, error) {
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

	err = b.checkDomains(logger, options.Domain, details.OrganizationGUID)
	if err != nil {
		return options, err
	}

	return options, err
}

// parseUpdateDetails will attempt to parse the update details and then verify that at least "domain" or "origin"
// are provided.
func (b *CdnServiceBroker) parseUpdateDetails(logger lager.Logger, details domain.UpdateDetails) (UpdateOptions, error) {
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
		err = b.checkDomains(logger, *options.Domain, details.PreviousValues.OrgID)
		if err != nil {
			return options, err
		}
	}

	return options, err
}

func (b *CdnServiceBroker) checkDomains(logger lager.Logger, domainsConcat, orgGUID string) error {
	// domain can be a comma separated list so we need to check each one individually
	domainStrings := strings.Split(domainsConcat, ",")
	var nonExistentDomains []string
	var nonOwnedDomains []string

	for i := range domainStrings {
		if !isValidDomain(domainStrings[i]) {
			return fmt.Errorf(
				"Domain %s doesn't look like a valid domain",
				domainStrings[i],
			)
		}

		v3domain, err := b.getDomainOrParent(logger, domainStrings[i])
		if errors.Is(err, errDomainOrParentNotFound) {
			logger.Info("cf-domain-not-found", lager.Data{
				"domain": domainStrings[i],
			})
			nonExistentDomains = append(nonExistentDomains, domainStrings[i])
			continue
		}
		if err != nil {
			return err
		}

		if v3domain.Relationships.Organization.Data.GUID != orgGUID {
			// check the SharedOrganizations
			// really, golang?.
			found := false
			for j := range v3domain.Relationships.SharedOrganizations.Data {
				if v3domain.Relationships.SharedOrganizations.Data[j].GUID == orgGUID {
					found = true
					break
				}
			}
			if !found {
				logger.Info("cf-domain-wrong-owner", lager.Data{
					"domain":                   domainStrings[i],
					"owning_organization_guid": v3domain.Relationships.Organization.Data.GUID,
				})
				nonOwnedDomains = append(nonOwnedDomains, domainStrings[i])
				continue
			}
		}
	}

	if len(nonExistentDomains) > 0 {
		orgName := "<organization>"
		org, err := b.cfclient.GetOrgByGuid(orgGUID)
		if err == nil {
			orgName = org.Name
		}

		var createDomainCommands []string
		for i := range nonExistentDomains {
			createDomainCommands = append(
				createDomainCommands,
				fmt.Sprintf("cf create-domain %s %s", nonExistentDomains[i], orgName),
			)
		}

		if len(createDomainCommands) == 1 {
			return fmt.Errorf(
				"Domain %s does not exist in CloudFoundry; create it with: %s",
				nonExistentDomains[0],
				createDomainCommands[0],
			)
		}

		return fmt.Errorf(
			"Multiple domains do not exist in CloudFoundry; create them with:\n%s",
			strings.Join(createDomainCommands, "\n"),
		)
	}

	if len(nonOwnedDomains) > 0 {
		if len(nonOwnedDomains) == 1 {
			return fmt.Errorf(
				"Domain %s is owned by a different organization in CloudFoundry",
				nonOwnedDomains[0],
			)
		}
		return fmt.Errorf(
			"Multiple domains are owned by a different organization in CloudFoundry: %s",
			strings.Join(nonOwnedDomains, ", "),
		)
	}

	return nil
}

func (b *CdnServiceBroker) getDomainOrParent(logger lager.Logger, domain string) (cf.V3Domain, error) {
	for {
		q := url.Values{}
		q.Set("names", domain)
		v3domains, err := b.cfclient.ListV3Domains(q)
		if err != nil {
			return cf.V3Domain{}, fmt.Errorf(
				"Error during CloudFoundry domain lookup of %s: %w",
				domain,
				err,
			)
		}
		if len(v3domains) == 1 {
			// found
			return v3domains[0], nil
		}
		if len(v3domains) > 1 {
			// shouldn't be possible?
			logger.Info("cf-domain-multiple-found", lager.Data{
				"domain": domain,
			})
			return cf.V3Domain{}, fmt.Errorf(
				"Domain %s matches multiple CloudFoundry domains",
				domain,
			)
		}

		// none found, try a parent domain
		splitDomain := strings.SplitN(domain, ".", 2)
		if len(splitDomain) != 2 || !strings.Contains(splitDomain[1], ".") {
			// no more parents or remaining parent not valid domain
			return cf.V3Domain{}, errDomainOrParentNotFound
		}
		domain = splitDomain[1]
	}
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

	if len(headers) > MaxHeaderCount {
		err = fmt.Errorf("must not set more than %d headers; got %d", MaxHeaderCount, len(headers))
		return headers, err
	}

	return headers, err
}
