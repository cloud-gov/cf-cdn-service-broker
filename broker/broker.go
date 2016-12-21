package broker

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"

	"github.com/pivotal-cf/brokerapi"

	"github.com/18F/cf-cdn-service-broker/models"
)

type ProvisionOptions struct {
	Domain         string `json:"domain"`
	Origin         string `json:"origin"`
	Path           string `json:"path"`
	InsecureOrigin bool   `json:"insecure_origin"`
}

type CdnServiceBroker struct {
	Manager models.RouteManagerIface
}

func (*CdnServiceBroker) Services() []brokerapi.Service {
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

func parseProvisionOptions(details brokerapi.ProvisionDetails) (options ProvisionOptions, err error) {
	if len(details.RawParameters) == 0 {
		err = errors.New("must be invoked with configuration parameters")
		return
	}

	err = json.Unmarshal(details.RawParameters, &options)
	if err != nil {
		return
	}
	if options.Domain == "" || options.Origin == "" {
		err = errors.New("must be invoked with `domain` and `origin` keys")
		return
	}

	return
}

func (b *CdnServiceBroker) Provision(
	instanceId string,
	details brokerapi.ProvisionDetails,
	asyncAllowed bool,
) (brokerapi.ProvisionedServiceSpec, error) {
	spec := brokerapi.ProvisionedServiceSpec{}

	if !asyncAllowed {
		return spec, brokerapi.ErrAsyncRequired
	}

	options, err := parseProvisionOptions(details)
	if err != nil {
		return spec, err
	}

	_, err = b.Manager.Get(instanceId)
	if err == nil {
		return spec, brokerapi.ErrInstanceAlreadyExists
	}

	tags := map[string]string{
		"Organization": details.OrganizationGUID,
		"Space":        details.SpaceGUID,
		"Service":      details.ServiceID,
		"Plan":         details.PlanID,
	}

	_, err = b.Manager.Create(instanceId, options.Domain, options.Origin, options.Path, options.InsecureOrigin, tags)
	if err != nil {
		return spec, err
	}

	return brokerapi.ProvisionedServiceSpec{IsAsync: true}, nil
}

func (b *CdnServiceBroker) LastOperation(instanceId string) (brokerapi.LastOperation, error) {
	route, err := b.Manager.Get(instanceId)
	if err != nil {
		return brokerapi.LastOperation{
			State:       brokerapi.Failed,
			Description: "Service instance not found",
		}, nil
	}

	err = b.Manager.Update(route)
	if err != nil {
		log.Printf("[%s] Error during %s: %s", route.DomainExternal, route.State, err)
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

func (b *CdnServiceBroker) Deprovision(instanceId string, details brokerapi.DeprovisionDetails, asyncAllowed bool) (brokerapi.IsAsync, error) {
	if !asyncAllowed {
		return false, brokerapi.ErrAsyncRequired
	}

	route, err := b.Manager.Get(instanceId)
	if err != nil {
		return false, err
	}

	err = b.Manager.Disable(route)
	if err != nil {
		return false, nil
	}

	return true, nil
}

func (b *CdnServiceBroker) Bind(instanceId, bindingId string, details brokerapi.BindDetails) (brokerapi.Binding, error) {
	return brokerapi.Binding{}, errors.New("service does not support bind")
}

func (b *CdnServiceBroker) Unbind(instanceId, bindingId string, details brokerapi.UnbindDetails) error {
	return errors.New("service does not support bind")
}

func (b *CdnServiceBroker) Update(instanceId string, details brokerapi.UpdateDetails, asyncAllowed bool) (brokerapi.IsAsync, error) {
	if !asyncAllowed {
		return false, brokerapi.ErrAsyncRequired
	}

	return true, nil
}
