package broker_test

import (
	"context"
	"errors"
	"net/url"
	"reflect"

	"github.com/stretchr/testify/suite"

	"code.cloudfoundry.org/lager/v3"
	"github.com/alphagov/paas-cdn-broker/broker"
	cfmock "github.com/alphagov/paas-cdn-broker/cf/mocks"
	"github.com/alphagov/paas-cdn-broker/config"
	"github.com/alphagov/paas-cdn-broker/models"
	"github.com/alphagov/paas-cdn-broker/models/mocks"
	"github.com/alphagov/paas-cdn-broker/utils"
	"github.com/cloudfoundry-community/go-cfclient"
)

type ProvisionUpdateSuite struct {
	suite.Suite
	Manager  mocks.RouteManagerIface
	Broker   *broker.CdnServiceBroker
	cfclient cfmock.Client
	settings config.Settings
	logger   lager.Logger
	ctx      context.Context
}

func (s *ProvisionUpdateSuite) allowCreateWithExpectedHeaders(expectedHeaders utils.Headers) {
	route := &models.Route{State: models.Provisioning}
	s.Manager.CreateStub = func(
		_ string,
		_ string,
		_ string,
		_ int64,
		headers utils.Headers,
		_ bool,
		_ map[string]string) (*models.Route, error) {

		if reflect.DeepEqual(headers, expectedHeaders) {
			return route, nil
		}

		return nil, errors.New("unexpected header values")
	}
}

// setup cf response for domain1.cloud.gov, directly owned by test org
func (s *ProvisionUpdateSuite) setupCFClientListV3DomainsDomain1() {
	q, _ := url.ParseQuery("names=domain1.cloud.gov")
	s.cfclient.On(
		"ListV3Domains",
		q,
	).Return([]cfclient.V3Domain{
		{
			Name: "domain.cloud.gov",
			Relationships: cfclient.DomainRelationships{
				Organization: cfclient.V3ToOneRelationship{
					Data: cfclient.V3Relationship{
						GUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
					},
				},
			},
		},
	}, nil)
}

// setup cf response for domain2.cloud.gov, shared with test org
func (s *ProvisionUpdateSuite) setupCFClientListV3DomainsDomain2() {
	q, _ := url.ParseQuery("names=domain2.cloud.gov")
	s.cfclient.On(
		"ListV3Domains",
		q,
	).Return([]cfclient.V3Domain{
		{
			Name: "domain2.cloud.gov",
			Relationships: cfclient.DomainRelationships{
				Organization: cfclient.V3ToOneRelationship{
					Data: cfclient.V3Relationship{
						GUID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
					},
				},
				SharedOrganizations: cfclient.V3ToManyRelationships{
					Data: []cfclient.V3Relationship{
						{GUID: "11111111-2222-3333-4444-555555555555"},
						{GUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5"},
					},
				},
			},
		},
	}, nil)
}

// setup cf response for domain3.cloud.gov, not known
func (s *ProvisionUpdateSuite) setupCFClientListV3DomainsDomain3() {
	q, _ := url.ParseQuery("names=domain3.cloud.gov")
	s.cfclient.On(
		"ListV3Domains",
		q,
	).Return([]cfclient.V3Domain{}, nil)
	// followup query looking for parent
	q, _ = url.ParseQuery("names=cloud.gov")
	s.cfclient.On(
		"ListV3Domains",
		q,
	).Return([]cfclient.V3Domain{}, nil)
}

// setup cf response for domain4.four.cloud.gov, not known
func (s *ProvisionUpdateSuite) setupCFClientListV3DomainsDomain4() {
	q, _ := url.ParseQuery("names=domain4.four.cloud.gov")
	s.cfclient.On(
		"ListV3Domains",
		q,
	).Return([]cfclient.V3Domain{}, nil)
	// followup query looking for parent
	q, _ = url.ParseQuery("names=four.cloud.gov")
	s.cfclient.On(
		"ListV3Domains",
		q,
	).Return([]cfclient.V3Domain{}, nil)
	// followup query looking for parent
	q, _ = url.ParseQuery("names=cloud.gov")
	s.cfclient.On(
		"ListV3Domains",
		q,
	).Return([]cfclient.V3Domain{}, nil)
}

// setup cf response for domain5.cloud.gov, shared but not with test org
func (s *ProvisionUpdateSuite) setupCFClientListV3DomainsDomain5() {
	q, _ := url.ParseQuery("names=domain5.cloud.gov")
	s.cfclient.On(
		"ListV3Domains",
		q,
	).Return([]cfclient.V3Domain{
		{
			Name: "domain5.cloud.gov",
			Relationships: cfclient.DomainRelationships{
				Organization: cfclient.V3ToOneRelationship{
					Data: cfclient.V3Relationship{
						GUID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
					},
				},
				SharedOrganizations: cfclient.V3ToManyRelationships{
					Data: []cfclient.V3Relationship{
						{GUID: "11111111-2222-3333-4444-555555555555"},
					},
				},
			},
		},
	}, nil)
}

// setup cf response for domain6.cloud.gov, not owned by test org
func (s *ProvisionUpdateSuite) setupCFClientListV3DomainsDomain6() {
	q, _ := url.ParseQuery("names=domain6.cloud.gov")
	s.cfclient.On(
		"ListV3Domains",
		q,
	).Return([]cfclient.V3Domain{
		{
			Name: "domain6.cloud.gov",
			Relationships: cfclient.DomainRelationships{
				Organization: cfclient.V3ToOneRelationship{
					Data: cfclient.V3Relationship{
						GUID: "22222222-3333-4444-5555-666666666666",
					},
				},
			},
		},
	}, nil)
}

// setup cf response for domain7.rain.gov, rain.gov directly owned by test org
func (s *ProvisionUpdateSuite) setupCFClientListV3DomainsDomain7() {
	q, _ := url.ParseQuery("names=domain7.rain.gov")
	s.cfclient.On(
		"ListV3Domains",
		q,
	).Return([]cfclient.V3Domain{}, nil)
	q, _ = url.ParseQuery("names=rain.gov")
	s.cfclient.On(
		"ListV3Domains",
		q,
	).Return([]cfclient.V3Domain{
		{
			Name: "rain.gov",
			Relationships: cfclient.DomainRelationships{
				Organization: cfclient.V3ToOneRelationship{
					Data: cfclient.V3Relationship{
						GUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
					},
				},
			},
		},
	}, nil)
}

// setup cf response for domain8.wet.snow.gov, wet.snow.gov not shared but not with test org
func (s *ProvisionUpdateSuite) setupCFClientListV3DomainsDomain8() {
	q, _ := url.ParseQuery("names=domain8.wet.snow.gov")
	s.cfclient.On(
		"ListV3Domains",
		q,
	).Return([]cfclient.V3Domain{}, nil)
	q, _ = url.ParseQuery("names=wet.snow.gov")
	s.cfclient.On(
		"ListV3Domains",
		q,
	).Return([]cfclient.V3Domain{
		{
			Name: "wet.snow.gov",
			Relationships: cfclient.DomainRelationships{
				Organization: cfclient.V3ToOneRelationship{
					Data: cfclient.V3Relationship{
						GUID: "22222222-3333-4444-5555-666666666666",
					},
				},
				SharedOrganizations: cfclient.V3ToManyRelationships{
					Data: []cfclient.V3Relationship{
						{GUID: "11111111-2222-3333-4444-555555555555"},
					},
				},
			},
		},
	}, nil)
}

// setup successful cf response for test org
func (s *ProvisionUpdateSuite) setupCFClientOrg() {
	s.cfclient.On(
		"GetOrgByGuid",
		"dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
	).Return(cfclient.Org{Name: "my-org"}, nil)
}

// setup unsuccessful cf response for test org
func (s *ProvisionUpdateSuite) setupCFClientOrgErr() {
	s.cfclient.On(
		"GetOrgByGuid",
		"dfb39134-ab7d-489e-ae59-4ed5c6f42fb5",
	).Return(cfclient.Org{}, errors.New("nope"))
}
