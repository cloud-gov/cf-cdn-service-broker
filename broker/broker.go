package broker

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"

	"github.com/jinzhu/gorm"
	"github.com/pivotal-cf/brokerapi"

	"github.com/18F/cf-cdn-service-broker/models"
	"github.com/18F/cf-cdn-service-broker/utils"
)

type ProvisionOptions struct {
	Domain string `json:"domain"`
}

type CdnServiceBroker struct {
	DB *gorm.DB
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

func (b *CdnServiceBroker) Provision(
	instanceId string,
	details brokerapi.ProvisionDetails,
	asyncAllowed bool,
) (brokerapi.ProvisionedServiceSpec, error) {
	spec := brokerapi.ProvisionedServiceSpec{}

	if !asyncAllowed {
		return spec, errors.New("must be invoked with `asyncAllowed`")
	}

	if len(details.RawParameters) == 0 {
		return spec, errors.New("must be invoked with configuration parameters")
	}

	var options ProvisionOptions
	err := json.Unmarshal(details.RawParameters, &options)
	if err != nil {
		return spec, err
	}
	if options.Domain == "" {
		return spec, errors.New("must be invoked with `options` key")
	}

	_, err = models.NewRoute(b.DB, instanceId, options.Domain)
	if err != nil {
		return spec, err
	}

	return brokerapi.ProvisionedServiceSpec{IsAsync: true}, nil
}

func (b *CdnServiceBroker) LastOperation(instanceId string) (brokerapi.LastOperation, error) {
	route, err := b.getRoute(instanceId)
	if err != nil {
		return brokerapi.LastOperation{
			State:       brokerapi.Failed,
			Description: "Service instance not found",
		}, nil
	}
	err = route.Update(b.DB)
	if route.Pending || err != nil {
		return brokerapi.LastOperation{
			State: brokerapi.InProgress,
			Description: fmt.Sprintf(
				"Provisioning in progress; CNAME domain %s to %s",
				route.DomainExternal, route.DomainInternal,
			),
		}, nil
	}
	return brokerapi.LastOperation{
		State:       brokerapi.Succeeded,
		Description: "Service instance provisioned",
	}, nil
}

func (b *CdnServiceBroker) Deprovision(instanceId string, details brokerapi.DeprovisionDetails, asyncAllowed bool) (brokerapi.IsAsync, error) {
	return true, nil
}

func (b *CdnServiceBroker) Bind(instanceId, bindingId string, details brokerapi.BindDetails) (brokerapi.Binding, error) {
	route, err := b.getRoute(instanceId)
	if err != nil {
		return brokerapi.Binding{}, err
	}
	err = utils.BindHTTPOrigin(route.DistId, route.DomainExternal)
	if err != nil {
		return brokerapi.Binding{}, err
	}
	return brokerapi.Binding{}, nil
}

func (b *CdnServiceBroker) Unbind(instanceId, bindingId string, details brokerapi.UnbindDetails) error {
	route, err := b.getRoute(instanceId)
	if err != nil {
		return err
	}
	return utils.UnbindHTTPOrigin(route.DistId, route.DomainExternal)
}

func (b *CdnServiceBroker) Update(instanceId string, details brokerapi.UpdateDetails, asyncAllowed bool) (brokerapi.IsAsync, error) {
	return true, nil
}

func (b *CdnServiceBroker) getRoute(instanceId string) (models.Route, error) {
	route := models.Route{}
	b.DB.First(&route, models.Route{InstanceId: instanceId})
	if route.InstanceId == instanceId {
		return route, nil
	}
	return models.Route{}, fmt.Errorf("no route with InstanceId %s", instanceId)
}
