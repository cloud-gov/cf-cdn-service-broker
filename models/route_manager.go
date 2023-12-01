package models

import (
	"errors"
	"strings"
	"time"

	"code.cloudfoundry.org/lager/v3"
	"github.com/alphagov/paas-cdn-broker/config"
	"github.com/alphagov/paas-cdn-broker/utils"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/jinzhu/gorm"
	"github.com/pivotal-cf/brokerapi/v10"
)

//counterfeiter:generate -o mocks/RouteManagerIface.go --fake-name RouteManagerIface route_manager.go RouteManagerIface
type RouteManagerIface interface {
	Create(
		instanceId string,
		domain string,
		origin string,
		defaultTTL int64,
		forwardedHeaders utils.Headers,
		forwardCookies bool,
		tags map[string]string,
	) (*Route, error)

	Update(
		instanceId string,
		domain *string,
		defaultTTL *int64,
		forwardedHeaders *utils.Headers,
		forwardCookies *bool,
	) (bool, error)

	Get(instanceId string) (*Route, error)

	Poll(route *Route) error

	Disable(route *Route) error

	DeleteOrphanedCerts()

	GetDNSChallenges(route *Route, onlyValidatingCertificates bool) ([]utils.DomainValidationChallenge, error)

	GetCDNConfiguration(route *Route) (*cloudfront.Distribution, error)
}

type RouteManager struct {
	logger          lager.Logger
	cloudFront      utils.DistributionIface
	settings        config.Settings
	routeStoreIface RouteStoreInterface
	certsManager    utils.CertificateManagerInterface
}

func NewManager(
	logger lager.Logger,
	cloudFront utils.DistributionIface,
	settings config.Settings,
	routeStoreIface RouteStoreInterface,
	certsManager utils.CertificateManagerInterface,
) RouteManager {
	return RouteManager{
		logger:          logger,
		cloudFront:      cloudFront,
		settings:        settings,
		routeStoreIface: routeStoreIface,
		certsManager:    certsManager,
	}
}

func (m *RouteManager) Create(
	instanceId,
	domain string,
	origin string,
	defaultTTL int64,
	forwardedHeaders utils.Headers,
	forwardCookies bool,
	tags map[string]string,
) (*Route, error) {

	route := &Route{
		InstanceId:     instanceId,
		State:          Provisioning,
		DomainExternal: domain,
		Origin:         origin,
		Path:           "",
		DefaultTTL:     defaultTTL,
		InsecureOrigin: false,
		Certificates:   []Certificate{},
	}

	lsession := m.logger.Session("route-manager-create-route", lager.Data{
		"instance-id": instanceId,
	})

	//Creating Cloud Front Distribution
	lsession.Info("create-cloudfront-instance")
	dist, err := m.cloudFront.Create(
		instanceId,
		make([]string, 0),
		origin,
		defaultTTL,
		forwardedHeaders,
		forwardCookies,
		tags,
	)
	if err != nil {
		lsession.Error("create-cloudfront-instance", err)
		return nil, err
	}

	route.DomainInternal = *dist.DomainName
	route.DistId = *dist.Id

	//request a certificate from ACM.
	lsession.Info("certsmanager-request-certificate")
	certArn, err := m.certsManager.RequestCertificate(route.GetDomains(), instanceId)
	if err != nil {
		lsession.Error("certsmanager-request-certificate", err)
		return nil, err
	}
	lsession.Info("certsmanager-request-certificate-done", lager.Data{"certificate-arn": *certArn})

	newCert := Certificate{
		CertificateArn:    *certArn,
		CertificateStatus: CertificateStatusValidating,
	}

	route.Certificates = append(route.Certificates, newCert)

	//insert the route object into the database
	lsession.Info("create-route")
	if err := m.routeStoreIface.Create(route); err != nil {
		lsession.Error("create-route", err)
		return nil, err
	}

	return route, nil
}

// Get a Route from a database, by instanceId
func (m *RouteManager) Get(instanceId string) (*Route, error) {
	route := Route{}

	lsession := m.logger.Session("route-manager-get", lager.Data{
		"instance-id": instanceId,
	})

	lsession.Info("db-first-route")
	route, err := m.routeStoreIface.FindOneMatching(Route{
		InstanceId: instanceId,
	})

	if err == nil {
		return &route, nil
	} else if err == gorm.ErrRecordNotFound {
		lsession.Error("db-record-not-found", brokerapi.ErrInstanceDoesNotExist)
		return nil, brokerapi.ErrInstanceDoesNotExist
	} else {
		lsession.Error("db-generic-error", err)
		return nil, err
	}
}

// Update function updates the CDN route service and returns whether the update has been
// performed asynchronously or not
// this function is ONLY called when a tenant will issue 'cf service-update'
func (m *RouteManager) Update(
	instanceId string,
	domain *string,
	defaultTTL *int64,
	forwardedHeaders *utils.Headers,
	forwardCookies *bool,
) (bool, error) {

	lsession := m.logger.Session("route-manager-update", lager.Data{
		"instance-id": instanceId,
	})

	// Get current route
	lsession.Info("get-route")
	route, err := m.Get(instanceId)
	if err != nil {
		lsession.Error("get-route", err)
		return false, err
	}

	// Override DefaultTTL settings that are new or different.
	if defaultTTL != nil {
		lsession.Info("param-update-default-ttl")
		route.DefaultTTL = *defaultTTL
	}

	// Update the distribution with new TTL, forwardHeaders and forwardCookies settings
	lsession.Info("cloudfront-update-excluding-domains")
	dist, err := m.cloudFront.Update(
		route.DistId,
		nil,
		route.Origin,
		defaultTTL,
		forwardedHeaders,
		forwardCookies,
	)
	if err != nil {
		lsession.Error("cloudfront-update-excluding-domains", err)
		return false, err
	}

	// Get the updated domain name and dist id.
	route.DomainInternal = *dist.DomainName
	route.DistId = *dist.Id

	if domain == nil || *domain == "" {
		lsession.Info("set-state-provisioned")
		// CloudFront has been updated with all the configuration
		// The domains are not being updated so we do not need a new certificate
		// The Update step is therefore now finished
		route.State = Provisioned
	} else {
		lsession.Info("set-state-provisioning")
		route.State = Provisioning

		if domain != nil {
			lsession.Info("param-update-domain")
			route.DomainExternal = *domain
		}

		//request a certificate from ACM.
		lsession.Info("certsmanager-request-certificate")
		certArn, err := m.certsManager.RequestCertificate(route.GetDomains(), instanceId)
		if err != nil {
			lsession.Error("certsmanager-request-certificate", err)
			return false, err
		}
		lsession.Info("certsmanager-request-certificate-done", lager.Data{"certificate-arn": *certArn})

		newCert := Certificate{
			CertificateArn:    *certArn,
			CertificateStatus: CertificateStatusValidating,
		}

		route.Certificates = append(route.Certificates, newCert)
	}

	//save route object into the database
	lsession.Info("save-route")
	if err = m.routeStoreIface.Save(route); err != nil {
		lsession.Error("save-route", err)
		return false, err
	}

	performedAsynchronously := route.State == Provisioning
	return performedAsynchronously, nil
}

func (m *RouteManager) Poll(r *Route) error {
	lsession := m.logger.Session("route-manager-update", lager.Data{
		"instance-id": r.InstanceId,
	})
	switch r.State {
	case Provisioning:
		lsession.Info("update-provisioning")
		return m.updateProvisioning(r)
	case Deprovisioning:
		lsession.Info("update-deprovisioning")
		return m.updateDeprovisioning(r)
	default:
		lsession.Info("unexpected-state", lager.Data{"state": r.State})
		return nil
	}
}

func (m *RouteManager) Disable(r *Route) error {
	lsession := m.logger.Session("route-manager-disable", lager.Data{
		"instance-id": r.InstanceId,
	})

	lsession.Info("cloudfront-disable")
	err := m.cloudFront.Disable(r.DistId)
	if err != nil {
		lsession.Error("cloudfront-disable", err)
		return err
	}

	lsession.Info("save-route")
	r.State = Deprovisioning
	if err := m.routeStoreIface.Save(r); err != nil {
		lsession.Error("save-route", err)
	}

	return nil
}

func (m *RouteManager) DeleteOrphanedCerts() {
	lsession := m.logger.Session("delete-orphaned-certs")
	// First let us call the function that will clean up orphaned certs that
	// were issued by ACM.
	m.deleteOrphanedACMCerts()

	lsession.Info("finished")
}

func (m *RouteManager) CheckRoutesToUpdate() {
	lsession := m.logger.Session("check-routes-to-update")

	lsession.Info("fetch-routes-to-update")
	routes, err := m.fetchRoutesToUpdate(lsession)

	if err != nil {
		lsession.Error("fetch-routes-to-update", err)
		return
	}

	if len(routes) == 0 {
		return
	}

	for _, route := range routes {
		loopLog := lsession.WithData(lager.Data{
			"instance-id": route.InstanceId,
			"domains":     route.GetDomains(),
		})

		loopLog.Info("check")
		err := m.Poll(&route)
		if err != nil {
			loopLog.Info("check-failed")

			if strings.Contains(err.Error(), "CNAMEAlreadyExists") {
				loopLog.Info("cname-conflict")

				route.State = Conflict
				loopLog.Info("set-state", lager.Data{"state": route.State})
				err = m.routeStoreIface.Save(&route)
				if err != nil {
					loopLog.Error("route-save-failed", err)
					continue
				}

			}

			if err == utils.ErrValidationTimedOut {
				loopLog.Info("certificate-validation-timed-out")
				route.State = Failed
				loopLog.Info("set-state", lager.Data{"state": route.State})
				err = m.routeStoreIface.Save(&route)
				if err != nil {
					loopLog.Error("route-save-failed", err)
					continue
				}
			}
		}

		loopLog.Info("checking-provisioning-expiration", lager.Data{"provisioning_since": route.ProvisioningSince})
		if route.IsProvisioningExpired() {
			loopLog.Info("expiring-unprovisioned-instance", lager.Data{
				"domain":             route.DomainExternal,
				"state":              route.State,
				"created_at":         route.CreatedAt,
				"provisioning_since": route.ProvisioningSince,
			})

			route.State = TimedOut
			loopLog.Info("set-state", lager.Data{"state": route.State})
			err = m.routeStoreIface.Save(&route)
			if err != nil {
				loopLog.Error("route-save-failed", err)
				continue
			}
		}
	}
}

func (m *RouteManager) GetDNSChallenges(route *Route, onlyValidatingCertificates bool) ([]utils.DomainValidationChallenge, error) {
	lsession := m.logger.Session("get-dns-instructions", lager.Data{
		"instance-id":               route.InstanceId,
		"domains":                   route.GetDomains(),
		"only-get-validating-certs": onlyValidatingCertificates,
	})

	validatingCert, attachedCert := findValidatingAndAttachedCerts(route)

	certArnsToRequest := []string{}

	if onlyValidatingCertificates {
		if validatingCert == nil {
			err := errors.New("couldn't find the most recent validating certificate")
			lsession.Error("missing-validating-certificate", err)
			return nil, err
		}

		certArnsToRequest = append(certArnsToRequest, validatingCert.CertificateArn)
	} else {
		if validatingCert != nil {
			certArnsToRequest = append(certArnsToRequest, validatingCert.CertificateArn)
		}

		if attachedCert != nil {
			certArnsToRequest = append(certArnsToRequest, attachedCert.CertificateArn)
		}
	}

	validationChallenges := []utils.DomainValidationChallenge{}

	for _, arn := range certArnsToRequest {
		lsession.Info("certsmanager-get-validation-challenges", lager.Data{"certificate-arn": arn})
		challenges, err := m.certsManager.GetDomainValidationChallenges(arn)
		if err != nil {
			// Only handle a ResourceNotFound exception specially
			if awsErr, ok := err.(awserr.Error); ok {
				if awsErr.Code() == acm.ErrCodeResourceNotFoundException {
					lsession.Info("certsmanager-get-validation-certificate-not-found", lager.Data{"certificate-arn": arn})
					continue
				}
			} else {
				lsession.Error("certsmanager-get-validation-challenges", err, lager.Data{"certificate-arn": arn})
				return nil, err
			}
		}

		validationChallenges = append(validationChallenges, challenges...)
	}

	lsession.Info("finished")
	return validationChallenges, nil
}

func (m *RouteManager) GetCurrentlyDeployedDomains(r *Route) ([]string, error) {
	lsession := m.logger.Session("get-currently-deployed-domains")

	lsession.Info("cloudfront-get-start")
	dist, err := m.cloudFront.Get(r.DistId)
	if err != nil {
		lsession.Error("cloudfront-get-error", err)

		return []string{}, err
	}
	lsession.Info("cloudfront-get-done")

	deployedDomains := []string{}
	for _, domain := range dist.DistributionConfig.Aliases.Items {
		deployedDomains = append(deployedDomains, *domain)
	}

	lsession.Info("finished")
	return deployedDomains, nil
}

func (m *RouteManager) GetCDNConfiguration(route *Route) (*cloudfront.Distribution, error) {
	return m.cloudFront.Get(route.DistId)
}

func (m *RouteManager) deleteOrphanedACMCerts() {
	lsession := m.logger.Session("delete-acm-managed-orphaned-certs")

	lsession.Info("list-issued-certificates")
	certs, err := m.certsManager.ListIssuedCertificates()
	if err != nil {
		lsession.Error("list-issued-certificates", err)
		return
	}

	time24hAgo := time.Now().Add(-24 * time.Hour)

	for _, cert := range certs {
		managedByCdnBroker := false

		for _, tag := range cert.Tags {
			if *tag.Key == utils.ManagedByTagName && *tag.Value == utils.ManagedByTagValue {
				managedByCdnBroker = true
				break
			}
		}

		isIssued := *cert.Status == acm.CertificateStatusIssued
		isInUse := len(cert.InUseBy) > 0
		olderThan24h := cert.IssuedAt.Before(time24hAgo)

		if isIssued && !isInUse && managedByCdnBroker && olderThan24h {
			lsession.Info("deleting-orphaned-cert", lager.Data{"certificate-arn": cert.CertificateArn})
			err = m.certsManager.DeleteCertificate(*cert.CertificateArn)
			if err != nil {
				lsession.Error("deleting-orphaned-cert", err, lager.Data{"certificate-arn": cert.CertificateArn})
			}
		} else {
			lsession.Info("not-deleting-certificate", lager.Data{
				"certificate-arn":          *cert.CertificateArn,
				"is-issued":                isIssued,
				"is-in-use":                isInUse,
				"is-managed-by-cdn-broker": managedByCdnBroker,
				"is-older-than-24h":        olderThan24h,
			})
		}
	}

	lsession.Info("finished")
}

func (m *RouteManager) fetchRoutesToUpdate(lsession lager.Logger) ([]Route, error) {
	provisioning := []Route{}

	lsession.Info("find-provisioning-instances")
	provisioning, err := m.routeStoreIface.FindAllMatching(Route{State: Provisioning})
	if err != nil {
		lsession.Error("find-provisioning-instances", err)
		return []Route{}, err
	}

	deprovisioning := []Route{}

	lsession.Info("find-deprovisioning-instances")
	deprovisioning, err = m.routeStoreIface.FindAllMatching(Route{State: Deprovisioning})
	if err != nil {
		lsession.Error("find-deprovisioning-instances", err)
		return []Route{}, err
	}

	routes := []Route{}
	routes = append(routes, provisioning...)
	routes = append(routes, deprovisioning...)

	affectedDomains := []string{}
	for _, route := range routes {
		affectedDomains = append(affectedDomains, route.DomainExternal)
	}

	if len(routes) > 0 {
		lsession.Info("found-instances", lager.Data{"domains": affectedDomains})
	} else {
		lsession.Info("found-no-instances")
	}

	return routes, nil
}

func (m *RouteManager) updateProvisioning(r *Route) error {
	lsession := m.logger.Session("route-manager-update-provisioning", lager.Data{
		"instance-id": r.InstanceId,
		"domains":     r.GetDomains(),
		"state":       r.State,
	})

	lsession.Info("check-distribution")
	isDistributionDeployed := m.checkDistribution(r)

	if !isDistributionDeployed {
		//we are here when distribution provisioing
		lsession.Info("distribution-provisioning")
		return nil
	}

	desiredDomains := r.GetDomains()
	lsession.Info("get-currently-deployed-domains")
	deployedDomains, err := m.GetCurrentlyDeployedDomains(r)
	if err != nil {
		lsession.Error("get-currently-deployed-domains", err)
		return err
	}

	lsession.Info(
		"router-manager-update-provisioning",
		lager.Data{
			"deployed-domains": deployedDomains,
			"desired-domains":  desiredDomains,
		},
	)

	lsession.Info("find-the-most-recent-validating-certificate")
	theMostRecentCert, theCurrentAttached := findValidatingAndAttachedCerts(r)

	if theMostRecentCert == nil {
		err = errors.New("couldn't find the most recent certificate")
		lsession.Error("find-the-most-recent-validating-certificate-error", err)
		return err
	}

	lsession.Info("find-the-most-recent-validating-certificate-found", lager.Data{"CertificateArn": theMostRecentCert.CertificateArn})

	//we need to ensure that the certificate validation in ACM has finished and its status is 'ISSUED'
	lsession.Info("is-certificate-issued")
	issued, err := m.certsManager.IsCertificateIssued(theMostRecentCert.CertificateArn)
	if err != nil {
		lsession.Error("is-certificate-issued", err)
		if err == utils.ErrValidationTimedOut {
			theMostRecentCert.CertificateStatus = CertificateStatusFailed
		}
		return err
	}

	if !issued {
		lsession.Info("certificate-is-not-issued-yet")
		return nil
	}

	lsession.Info("deploy-certificate")
	if err := m.deployACMCertificate(*r, *theMostRecentCert); err != nil {
		lsession.Error("deploy-certificate", err)
		return err
	}

	//Swaping certificates from attached to detached
	if theCurrentAttached != nil {
		theCurrentAttached.CertificateStatus = CertificateStatusDeleted
	}

	//Set the new issued certificate as attached
	theMostRecentCert.CertificateStatus = CertificateStatusAttached

	lsession.Info("set-provisioned")
	r.State = Provisioned
	lsession.Info("save-route-provisioned")
	if err := m.routeStoreIface.Save(r); err != nil {
		lsession.Error("save-route-provisioned", err)
		return err
	}

	lsession.Info("finished")
	return nil
}

func findValidatingAndAttachedCerts(r *Route) (*Certificate, *Certificate) {
	var theMostRecentCert, theCurrentAttached *Certificate
	for i, e := range r.Certificates {
		if e.CertificateStatus == CertificateStatusValidating {
			if theMostRecentCert == nil {
				theMostRecentCert = &(r.Certificates[i])
			}

			if e.CreatedAt.After(theMostRecentCert.CreatedAt) {
				theMostRecentCert = &(r.Certificates[i])
			}
		}

		if e.CertificateStatus == CertificateStatusAttached {
			theCurrentAttached = &(r.Certificates[i])
		}
	}
	return theMostRecentCert, theCurrentAttached
}

func (m *RouteManager) updateDeprovisioning(r *Route) error {
	lsession := m.logger.Session("route-manager-update-deprovisioning", lager.Data{
		"instance-id": r.InstanceId,
		"domains":     r.GetDomains(),
		"state":       r.State,
	})

	lsession.Info("cloudfront-delete")
	deleted, err := m.cloudFront.Delete(r.DistId)
	if err != nil {
		lsession.Error("cloudfront-delete", err)
		return err
	}

	if deleted {
		r.State = Deprovisioned
		lsession.Info("save-route-deprovisioned")
		if err := m.routeStoreIface.Save(r); err != nil {
			lsession.Error("save-route-deprovisioned", err)
		}
	}

	lsession.Info("finished")
	return nil
}

func (m *RouteManager) checkDistribution(r *Route) bool {
	lsession := m.logger.Session("check-distribution", lager.Data{
		"instance-id": r.InstanceId,
	})

	lsession.Info("cloudfront-get")
	dist, err := m.cloudFront.Get(r.DistId)
	if err != nil {
		lsession.Error("cloudfront-get", err)
		return false
	}

	lsession.Info("finished", lager.Data{
		"status":  *dist.Status,
		"enabled": *dist.DistributionConfig.Enabled,
	})
	return *dist.Status == "Deployed" && *dist.DistributionConfig.Enabled
}

func (m *RouteManager) deployACMCertificate(
	route Route,
	cert Certificate,
) error {
	lsession := m.logger.Session("deploy-acm-certificate", lager.Data{
		"instance-id": route.InstanceId,
		"domains":     route.GetDomains(),
		"dist":        route.DistId,
	})

	lsession.Info("acm-set-certificate-and-cname", lager.Data{
		"cert_id": cert.CertificateArn,
	})
	err := m.cloudFront.SetCertificateAndCname(route.DistId, cert.CertificateArn, route.GetDomains())
	if err != nil {
		lsession.Error("acm-set-certificate-and-cname", err, lager.Data{
			"cert_id": cert.CertificateArn,
		})
		return err
	}

	lsession.Info("finished")
	return nil
}
