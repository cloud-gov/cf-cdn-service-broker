package broker_test

import (
	"context"
	"errors"
	"net/url"
	"reflect"

	"github.com/stretchr/testify/suite"

	"code.cloudfoundry.org/lager"
	"github.com/alphagov/paas-cdn-broker/broker"
	"github.com/alphagov/paas-cdn-broker/config"
	"github.com/alphagov/paas-cdn-broker/utils"
	"github.com/alphagov/paas-cdn-broker/models"
	cfmock "github.com/alphagov/paas-cdn-broker/cf/mocks"
	"github.com/cloudfoundry-community/go-cfclient"
	"github.com/alphagov/paas-cdn-broker/models/mocks"
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

// setup cf response for domain1.gov, directly owned by test org
func (s *ProvisionUpdateSuite) setupCFClientListV3DomainsDomain1() {
	q , _ := url.ParseQuery("names=domain1.gov")
	s.cfclient.On(
		"ListV3Domains",
		q,
	).Return([]cfclient.V3Domain{
		{
			Name: "domain.gov",
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

// setup cf response for domain2.gov, shared with test org
func (s *ProvisionUpdateSuite) setupCFClientListV3DomainsDomain2() {
	q , _ := url.ParseQuery("names=domain2.gov")
	s.cfclient.On(
		"ListV3Domains",
		q,
	).Return([]cfclient.V3Domain{
		{
			Name: "domain2.gov",
			Relationships: cfclient.DomainRelationships{
				Organization: cfclient.V3ToOneRelationship{
					Data: cfclient.V3Relationship{
						GUID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
					},
				},
				SharedOrganizations: cfclient.V3ToManyRelationships{
					Data: []cfclient.V3Relationship{
						{ GUID: "11111111-2222-3333-4444-555555555555" },
						{ GUID: "dfb39134-ab7d-489e-ae59-4ed5c6f42fb5" },
					},
				},
			},
		},
	}, nil)
}

// setup cf response for domain3.gov, not known
func (s *ProvisionUpdateSuite) setupCFClientListV3DomainsDomain3() {
	q , _ := url.ParseQuery("names=domain3.gov")
	s.cfclient.On(
		"ListV3Domains",
		q,
	).Return([]cfclient.V3Domain{}, nil)
}

// setup cf response for domain4.gov, not known
func (s *ProvisionUpdateSuite) setupCFClientListV3DomainsDomain4() {
	q , _ := url.ParseQuery("names=domain4.gov")
	s.cfclient.On(
		"ListV3Domains",
		q,
	).Return([]cfclient.V3Domain{}, nil)
}

// setup cf response for domain5.gov, shared but not with test org
func (s *ProvisionUpdateSuite) setupCFClientListV3DomainsDomain5() {
	q , _ := url.ParseQuery("names=domain5.gov")
	s.cfclient.On(
		"ListV3Domains",
		q,
	).Return([]cfclient.V3Domain{
		{
			Name: "domain5.gov",
			Relationships: cfclient.DomainRelationships{
				Organization: cfclient.V3ToOneRelationship{
					Data: cfclient.V3Relationship{
						GUID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
					},
				},
				SharedOrganizations: cfclient.V3ToManyRelationships{
					Data: []cfclient.V3Relationship{
						{ GUID: "11111111-2222-3333-4444-555555555555" },
					},
				},
			},
		},
	}, nil)
}

// setup cf response for domain6.gov, not owned by test org
func (s *ProvisionUpdateSuite) setupCFClientListV3DomainsDomain6() {
	q , _ := url.ParseQuery("names=domain6.gov")
	s.cfclient.On(
		"ListV3Domains",
		q,
	).Return([]cfclient.V3Domain{
		{
			Name: "domain6.gov",
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
