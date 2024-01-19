package models

import (
	"code.cloudfoundry.org/lager/v3"

	"github.com/jinzhu/gorm"
)

var (
	helperLogger = lager.NewLogger("helper-logger")
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate -o mocks/FakeRouteStore.go --fake-name FakeRouteStore data_store.go RouteStoreInterface
type RouteStoreInterface interface {
	Save(*Route) error
	Create(*Route) error
	FindOneMatching(Route) (Route, error)
	FindAllMatching(Route) ([]Route, error)
}

type RouteStore struct {
	Database *gorm.DB
	Logger   lager.Logger
}

func (r RouteStore) Save(route *Route) error {
	r.Logger.Info("pre-save", lager.Data{"route": route})
	err := r.Database.Save(route).Error
	r.Logger.Info("post-save", lager.Data{"route": route})
	return err
}

func (r RouteStore) Create(route *Route) error {
	r.Logger.Info("pre-create", lager.Data{"route": route})
	err := r.Database.Create(route).Error
	r.Logger.Info("post-create", lager.Data{"route": route})
	if err != nil {
		return err
	}

	return r.hydrateRoute(route)
}

func (r RouteStore) FindOneMatching(route Route) (Route, error) {
	output := Route{}
	err := r.Database.Preload("Certificates").First(&output, route).Error

	if err != nil {
		return Route{}, err
	}

	err = r.hydrateRoute(&output)
	if err != nil {
		return Route{}, err
	}

	return output, nil
}

func (r RouteStore) FindAllMatching(route Route) ([]Route, error) {
	results := []Route{}
	err := r.Database.Preload("Certificates").Find(&results, route).Error

	if err != nil {
		return []Route{}, err
	}

	for i := range results {
		err = r.hydrateRoute(&results[i])
		if err != nil {
			return []Route{}, err
		}
	}

	return results, nil
}

func (r *RouteStore) hydrateRoute(route *Route) error {
	return r.populateCertificate(route)
}

func (r RouteStore) populateCertificate(route *Route) error {
	var certificate Certificate
	r.Database.Find(&certificate, Certificate{RouteId: route.Model.ID})

	if r.Database.RecordNotFound() {
		return nil
	}

	if r.Database.Error != nil {
		return r.Database.Error
	}

	return nil
}
