package broker

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"

	"github.com/pivotal-cf/brokerapi"
)

type CatalogExternal struct {
	Services []brokerapi.Service `json:"services"`
}

func LoadCatalog(path string) (Catalog, error) {
	raw, err := ioutil.ReadFile(path)
	if err != nil {
		return Catalog{}, err
	}

	var catalog Catalog
	err = json.Unmarshal(raw, &catalog)
	if err != nil {
		return Catalog{}, err
	}

	return catalog, nil
}

type Catalog struct {
	Services []Service `json:"services,omitempty"`
}

type Service struct {
	ID              string                            `json:"id"`
	Name            string                            `json:"name"`
	Description     string                            `json:"description"`
	Bindable        bool                              `json:"bindable"`
	Tags            []string                          `json:"tags,omitempty"`
	PlanUpdatable   bool                              `json:"plan_updateable"`
	Plans           []ServicePlan                     `json:"plans"`
	Requires        []brokerapi.RequiredPermission    `json:"requires,omitempty"`
	Metadata        *brokerapi.ServiceMetadata        `json:"metadata,omitempty"`
	DashboardClient *brokerapi.ServiceDashboardClient `json:"dashboard_client,omitempty"`
}

type ServicePlan struct {
	ID          string                         `json:"id"`
	Name        string                         `json:"name"`
	Description string                         `json:"description"`
	Free        bool                           `json:"free"`
	Metadata    *brokerapi.ServicePlanMetadata `json:"metadata,omitempty"`
	Properties  Properties                     `json:"properties,omitempty"`
}

type Properties struct {
	IamPolicy json.RawMessage `json:"iam_policy,omitempty"`
}

func (c Catalog) Validate() error {
	for _, service := range c.Services {
		if err := service.Validate(); err != nil {
			return fmt.Errorf("Validating Services configuration: %s", err)
		}
	}

	return nil
}

func (c Catalog) FindService(serviceID string) (service Service, found bool) {
	for _, service := range c.Services {
		if service.ID == serviceID {
			return service, true
		}
	}

	return service, false
}

func (c Catalog) FindServicePlan(planID string) (plan ServicePlan, found bool) {
	for _, service := range c.Services {
		for _, plan := range service.Plans {
			if plan.ID == planID {
				return plan, true
			}
		}
	}

	return plan, false
}

func (s Service) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("Must provide a non-empty ID (%+v)", s)
	}

	if s.Name == "" {
		return fmt.Errorf("Must provide a non-empty Name (%+v)", s)
	}

	if s.Description == "" {
		return fmt.Errorf("Must provide a non-empty Description (%+v)", s)
	}

	for _, servicePlan := range s.Plans {
		if err := servicePlan.Validate(); err != nil {
			return fmt.Errorf("Validating Plans configuration: %s", err)
		}
	}

	return nil
}

func (sp ServicePlan) Validate() error {
	if sp.ID == "" {
		return fmt.Errorf("Must provide a non-empty ID (%+v)", sp)
	}

	if sp.Name == "" {
		return fmt.Errorf("Must provide a non-empty Name (%+v)", sp)
	}

	if sp.Description == "" {
		return fmt.Errorf("Must provide a non-empty Description (%+v)", sp)
	}

	if err := sp.Properties.Validate(); err != nil {
		return fmt.Errorf("Validating S3 Properties configuration: %s", err)
	}

	return nil
}

func (eq Properties) Validate() error {
	if len(eq.IamPolicy) == 0 {
		return errors.New("Must provide a non-empty IAM Policy")
	}

	return nil
}
