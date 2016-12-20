package broker_test

import "github.com/18F/cf-cdn-service-broker/models"

type FakeRouteManager struct {
	RouteCreate models.Route
	ErrorCreate error
	RouteGet    models.Route
	ErrorGet    error
}

func (m *FakeRouteManager) Create(instanceId, domain, origin, path string, insecure_origin bool) (models.Route, error) {
	return m.RouteCreate, m.ErrorCreate
}

func (m *FakeRouteManager) Get(instanceId string) (models.Route, error) {
	return m.RouteGet, m.ErrorGet
}

func (m *FakeRouteManager) Update(models.Route) error {
	return nil
}

func (m *FakeRouteManager) Disable(models.Route) error {
	return nil
}

func (m *FakeRouteManager) Renew(models.Route) error {
	return nil
}

func (m *FakeRouteManager) RenewAll() {
	return
}
