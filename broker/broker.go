package broker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"

	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi"

	"github.com/18F/cf-cdn-service-broker/models"
)

type Options struct {
	Domain         string `json:"domain"`
	Origin         string `json:"origin"`
	Path           string `json:"path"`
	InsecureOrigin bool   `json:"insecure_origin"`
}

type CdnServiceBroker struct {
	Manager models.RouteManagerIface
	Logger  lager.Logger
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

	_, err = b.Manager.Get(instanceID)
	if err == nil {
		return spec, brokerapi.ErrInstanceAlreadyExists
	}

	tags := map[string]string{
		"Organization": details.OrganizationGUID,
		"Space":        details.SpaceGUID,
		"Service":      details.ServiceID,
		"Plan":         details.PlanID,
	}

	_, err = b.Manager.Create(instanceID, options.Domain, options.Origin, options.Path, options.InsecureOrigin, tags)
	if err != nil {
		return spec, err
	}

	return brokerapi.ProvisionedServiceSpec{IsAsync: true}, nil
}

func (b *CdnServiceBroker) LastOperation(
	context context.Context,
	instanceID, operationData string,
) (brokerapi.LastOperation, error) {
	route, err := b.Manager.Get(instanceID)
	if err != nil {
		return brokerapi.LastOperation{
			State:       brokerapi.Failed,
			Description: "Service instance not found",
		}, nil
	}

	err = b.Manager.Poll(route)
	if err != nil {
		b.Logger.Error("Error during update", err, lager.Data{
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

	route, err := b.Manager.Get(instanceID)
	if err != nil {
		return brokerapi.DeprovisionServiceSpec{}, err
	}

	err = b.Manager.Disable(route)
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

	options, err := parseUpdateDetails(details.Parameters)
	if err != nil {
		return brokerapi.UpdateServiceSpec{}, err
	}

	err = b.Manager.Update(instanceID, options.Domain, options.Origin, options.Path, options.InsecureOrigin)
	if err != nil {
		return brokerapi.UpdateServiceSpec{}, err
	}

	return brokerapi.UpdateServiceSpec{IsAsync: true}, nil
}
