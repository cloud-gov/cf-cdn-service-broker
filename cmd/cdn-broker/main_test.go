package main_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"time"

	"code.cloudfoundry.org/lager/v3"
	"github.com/alphagov/paas-cdn-broker/broker"
	cfmock "github.com/alphagov/paas-cdn-broker/cf/mocks"
	. "github.com/alphagov/paas-cdn-broker/cmd/cdn-broker"
	"github.com/alphagov/paas-cdn-broker/config"
	"github.com/alphagov/paas-cdn-broker/models"
	"github.com/alphagov/paas-cdn-broker/utils"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudfront"
	"github.com/jinzhu/gorm"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var err error
var tempDir string

var _ = Describe("TLS Configuration", func() {
	var (
		db        gorm.DB
		logger    lager.Logger
		CdnBroker *broker.CdnServiceBroker
		cfg       config.Settings
		server    http.Handler
		tlsConfig *tls.Config
	)

	const (
		mockPort     = "8443"
		mockHost     = "0.0.0.0"
		mockEndpoint = "/healthcheck/https"
	)

	BeforeEach(func() {
		logger = lager.NewLogger("cdn-service-broker")
		logger.RegisterSink(lager.NewWriterSink(os.Stderr, lager.INFO))
		cfclient := cfmock.Client{}

		database, err := gorm.Open("postgres", os.Getenv("POSTGRES_URL"))
		Expect(err).ToNot(HaveOccurred())

		db = *database

		session, err := session.NewSession(aws.NewConfig().WithRegion(cfg.AwsDefaultRegion))
		Expect(err).NotTo(HaveOccurred())

		testConfig, caCertPEM := createCredentials()
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCertPEM)

		cfg.Tls = testConfig
		cfg.Host = mockHost
		cfg.Port = mockPort

		manager := models.NewManager(
			logger,
			&utils.Distribution{Settings: cfg, Service: cloudfront.New(session)},
			cfg,
			models.RouteStore{Database: &db, Logger: logger.Session("route-store", lager.Data{"entry-point": "broker"})},
			utils.NewCertificateManager(logger, cfg, session),
		)
		Expect(manager).NotTo(BeNil())

		CdnBroker = broker.New(
			&manager,
			&cfclient,
			cfg,
			logger,
		)
		Expect(CdnBroker).NotTo(BeNil())

		tlsConfig, err = testConfig.GenerateTLSConfig()
		Expect(err).NotTo(HaveOccurred())
		Expect(tlsConfig).NotTo(BeNil())

		tlsConfig.RootCAs = caCertPool
		Expect(tlsConfig.RootCAs).ToNot(BeNil())

		server = BuildHTTPHandler(CdnBroker, logger, &cfg, &db)
		Expect(server).ToNot(BeNil())

		go func() {
			err = StartHTTPServer(&cfg, server, logger)
			Expect(err).NotTo(HaveOccurred())
		}()
	})

	AfterEach(func() {
		// Nothing useful to be done here
	})

	It("should use TLS", func() {
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
		}
		resp, err := client.Get(fmt.Sprintf("https://localhost:%s%s", mockPort, mockEndpoint))
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})
})

func createCredentials() (*config.TLSConfig, []byte) {
	// Generate a CA private key
	caPriv, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())

	// Create a template for the CA certificate
	caTemplate := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{Organization: []string{"My CA"}},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	// Create and sign the CA certificate
	caCertDER, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, &caPriv.PublicKey, caPriv)
	Expect(err).NotTo(HaveOccurred())

	// Encode the CA private key to PEM format
	//caPrivBytes, err := x509.MarshalECPrivateKey(caPriv)
	//Expect(err).NotTo(HaveOccurred())
	//caPrivBlock := &pem.Block{Type: "EC PRIVATE KEY", Bytes: caPrivBytes}
	//caPrivPEM := pem.EncodeToMemory(caPrivBlock)

	// Encode the CA certificate to PEM format
	caCertBlock := &pem.Block{Type: "CERTIFICATE", Bytes: caCertDER}
	caCertPEM := pem.EncodeToMemory(caCertBlock)

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	serverTemplate := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Acme Co"},
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(time.Hour),
		IsCA:        true,
		KeyUsage:    x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    []string{"localhost"},
	}

	//derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	derBytes, err := x509.CreateCertificate(rand.Reader, &serverTemplate, &caTemplate, &priv.PublicKey, caPriv)
	Expect(err).NotTo(HaveOccurred())

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	privBytes := x509.MarshalPKCS1PrivateKey(priv)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privBytes})

	localTlsConfig := &config.TLSConfig{
		Certificate: string(certPEM),
		PrivateKey:  string(keyPEM),
	}

	return localTlsConfig, caCertPEM
}
