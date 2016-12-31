package broker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi"

	"github.com/18F/cf-cdn-service-broker/config"
	"github.com/18F/cf-cdn-service-broker/iamuser"
	"github.com/18F/cf-cdn-service-broker/models"
	"github.com/18F/cf-cdn-service-broker/utils"
)

type Options struct {
	Domain         string `json:"domain"`
	Origin         string `json:"origin"`
	Path           string `json:"path"`
	InsecureOrigin bool   `json:"insecure_origin"`
}

type CdnServiceBroker struct {
	logger       lager.Logger
	manager      models.RouteManagerIface
	distribution *utils.Distribution
	user         iamuser.User
	catalog      Catalog
	iamPath      string
	userPrefix   string
	policyPrefix string
}

func NewCdnServiceBroker(
	manager models.RouteManagerIface,
	distribution *utils.Distribution,
	user iamuser.User,
	catalog Catalog,
	settings config.Settings,
	logger lager.Logger,
) *CdnServiceBroker {
	return &CdnServiceBroker{
		manager:      manager,
		distribution: distribution,
		user:         user,
		catalog:      catalog,
		iamPath:      settings.IamPathPrefix,
		userPrefix:   settings.IamUserPrefix,
		policyPrefix: settings.IamPolicyPrefix,
		logger:       logger,
	}
}

func (b *CdnServiceBroker) Services(context context.Context) []brokerapi.Service {
	brokerCatalog, err := json.Marshal(b.catalog)
	if err != nil {
		b.logger.Error("marshal-error", err)
		return []brokerapi.Service{}
	}

	apiCatalog := CatalogExternal{}
	if err = json.Unmarshal(brokerCatalog, &apiCatalog); err != nil {
		b.logger.Error("unmarshal-error", err)
		return []brokerapi.Service{}
	}

	return apiCatalog.Services
}

// createBrokerOptions will attempt to take raw json and convert it into the "Options" struct.
func createBrokerOptions(details []byte) (options Options, err error) {
	if len(details) == 0 {
		err = errors.New("must be invoked with configuration parameters")
		return
	}
	err = json.Unmarshal(details, &options)
	if err != nil {
		return
	}
	return
}

// parseProvisionDetails will attempt to parse the update details and then verify that BOTH least "domain" and "origin"
// are provided.
func parseProvisionDetails(details brokerapi.ProvisionDetails) (options Options, err error) {
	options, err = createBrokerOptions(details.RawParameters)
	if err != nil {
		return
	}
	if options.Domain == "" || options.Origin == "" {
		err = errors.New("must be invoked with `domain` and `origin` keys")
		return
	}

	return
}

// parseUpdateDetails will attempt to parse the update details and then verify that at least "domain" or "origin"
// are provided.
func parseUpdateDetails(details map[string]interface{}) (options Options, err error) {
	// need to convert the map into raw JSON.
	rawJSON, err := json.Marshal(details)
	if err != nil {
		return
	}
	options, err = createBrokerOptions(rawJSON)
	if err != nil {
		return
	}
	if options.Domain == "" && options.Origin == "" {
		err = errors.New("must be invoked with `domain` or `origin` keys")
		return
	}
	return
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

	options, err := parseProvisionDetails(details)
	if err != nil {
		return spec, err
	}

	_, err = b.manager.Get(instanceID)
	if err == nil {
		return spec, brokerapi.ErrInstanceAlreadyExists
	}

	tags := map[string]string{
		"Organization": details.OrganizationGUID,
		"Space":        details.SpaceGUID,
		"Service":      details.ServiceID,
		"Plan":         details.PlanID,
	}

	_, err = b.manager.Create(instanceID, options.Domain, options.Origin, options.Path, options.InsecureOrigin, tags)
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
	b.logger.Debug("bind", lager.Data{
		"instance-id": instanceID,
		"binding-id":  bindingID,
		"details":     details,
	})

	binding := brokerapi.Binding{}

	var accessKeyID, secretAccessKey string
	var policyARN string
	var err error

	servicePlan, ok := b.catalog.FindServicePlan(details.PlanID)
	if !ok {
		return binding, fmt.Errorf("Service Plan '%s' not found", details.PlanID)
	}

	route, err := b.manager.Get(instanceID)
	if err != nil {
		return binding, err
	}

	dist, err := b.distribution.Get(route.DistId)
	if err != nil {
		return binding, err
	}

	if _, err = b.user.Create(b.userName(bindingID), b.iamPath); err != nil {
		return binding, err
	}
	defer func() {
		if err != nil {
			if policyARN != "" {
				b.user.DeletePolicy(policyARN)
			}
			if accessKeyID != "" {
				b.user.DeleteAccessKey(b.userName(bindingID), accessKeyID)
			}
			b.user.Delete(b.userName(bindingID))
		}
	}()

	accessKeyID, secretAccessKey, err = b.user.CreateAccessKey(b.userName(bindingID))
	if err != nil {
		return binding, err
	}

	policyARN, err = b.user.CreatePolicy(
		b.policyName(bindingID),
		b.iamPath,
		string(servicePlan.Properties.IamPolicy),
		*dist.ARN,
	)
	if err != nil {
		return binding, err
	}

	if err = b.user.AttachUserPolicy(b.userName(bindingID), policyARN); err != nil {
		return binding, err
	}

	binding.Credentials = map[string]string{
		"access_key_id":     accessKeyID,
		"secret_access_key": secretAccessKey,
		"distribution":      route.DistId,
	}

	return binding, nil
}

func (b *CdnServiceBroker) Unbind(
	context context.Context,
	instanceID, bindingID string,
	details brokerapi.UnbindDetails,
) error {
	b.logger.Debug("unbind", lager.Data{
		"instance-id": instanceID,
		"binding-id":  bindingID,
		"details":     details,
	})

	accessKeys, err := b.user.ListAccessKeys(b.userName(bindingID))
	if err != nil {
		return err
	}

	for _, accessKey := range accessKeys {
		if err := b.user.DeleteAccessKey(b.userName(bindingID), accessKey); err != nil {
			return err
		}
	}

	userPolicies, err := b.user.ListAttachedUserPolicies(b.userName(bindingID), b.iamPath)
	if err != nil {
		return err
	}

	for _, userPolicy := range userPolicies {
		if err := b.user.DetachUserPolicy(b.userName(bindingID), userPolicy); err != nil {
			return err
		}

		if err := b.user.DeletePolicy(userPolicy); err != nil {
			return err
		}
	}

	if err := b.user.Delete(b.userName(bindingID)); err != nil {
		return err
	}

	return nil
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

	options, err := parseUpdateDetails(details.Parameters)
	if err != nil {
		return brokerapi.UpdateServiceSpec{}, err
	}

	err = b.manager.Update(instanceID, options.Domain, options.Origin, options.Path, options.InsecureOrigin)
	if err != nil {
		return brokerapi.UpdateServiceSpec{}, err
	}

	return brokerapi.UpdateServiceSpec{IsAsync: true}, nil
}

func (b *CdnServiceBroker) userName(bindingID string) string {
	return fmt.Sprintf("%s-%s", b.userPrefix, bindingID)
}

func (b *CdnServiceBroker) policyName(bindingID string) string {
	return fmt.Sprintf("%s-%s", b.policyPrefix, bindingID)
}
