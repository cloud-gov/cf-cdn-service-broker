package models

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"

	"github.com/18F/cf-cdn-service-broker/utils"
	"github.com/jinzhu/gorm"
)

//counterfeiter:generate -o mocks/FakeRouteStore.go --fake-name FakeRouteStore data_store.go RouteStoreInterface
type RouteStoreInterface interface {
	Save(*Route) error
	Create(*Route) error
	FindOneMatching(Route) (Route, error)
	FindAllMatching(Route) ([]Route, error)
	FindWithExpiringCerts() ([]Route, error)
}

type RouteStore struct {
	Database *gorm.DB
}

func (r RouteStore) Save(route *Route) error {
	return r.Database.Save(route).Error
}

func (r RouteStore) Create(route *Route) error {
	err := r.Database.Create(route).Error

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

	for i, _ := range results {
		err = r.hydrateRoute(&results[i])
		if err != nil {
			return []Route{}, err
		}
	}

	return results, nil
}

func (r RouteStore) FindWithExpiringCerts() ([]Route, error) {
	routes := []Route{}

	err := r.Database.Preload("Certificates").Having(
		"max(expires) < now() + interval '30 days'",
	).Group(
		"routes.id",
	).Where(
		"state = ?", string(Provisioned),
	).Where(
		"is_certificate_managed_by_acm = ?", false,
	).Joins(
		"join certificates on routes.id = certificates.route_id",
	).Find(&routes).Error

	if err != nil {
		return []Route{}, err
	}

	for i, _ := range routes {
		err = r.hydrateRoute(&routes[i])
		if err != nil {
			return []Route{}, err
		}
	}

	return routes, nil
}

func (r *RouteStore) hydrateRoute(route *Route) error {
	err := r.populateUser(route)

	if err != nil {
		return err
	}

	return r.populateCertificate(route)
}

func (r *RouteStore) populateUser(route *Route) error {
	if route.UserDataID == 0 {
		return nil
	}

	var userData UserData
	if err := r.Database.Model(route).Related(&userData).Error; err != nil {
		return err
	}

	route.UserData = userData

	user, err := loadUser(userData)

	if err != nil {
		return err
	}
	route.User = user

	return nil
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

	if certificate.Model.ID > 0 {
		route.Certificate = certificate
	}

	return nil
}

func loadUser(userData UserData) (utils.User, error) {
	var user utils.User

	lsession := helperLogger.Session("load-user")

	if err := json.Unmarshal(userData.Reg, &user); err != nil {
		lsession.Error("json-unmarshal-user-data", err)
		return utils.User{}, err
	}
	keyBlock, _ := pem.Decode(userData.Key)

	switch keyBlock.Type {
	case "RSA PRIVATE KEY":
		key, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)

		if err != nil {
			return utils.User{}, err
		}
		user.SetPrivateKey(key)
	case "EC PRIVATE KEY":
		key, err := x509.ParseECPrivateKey(keyBlock.Bytes)

		if err != nil {
			return utils.User{}, err
		}
		user.SetPrivateKey(key)
	default:
		return utils.User{}, errors.New("unknown private key type")
	}

	return user, nil
}
