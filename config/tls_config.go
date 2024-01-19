package config

import (
	"crypto/tls"
	"fmt"
)

type TLSConfig struct {
	Certificate string `json:"certificate"`
	PrivateKey  string `json:"private_key"`
}

// GenerateTLSConfig produces a tls.Config structure out of TLSConfig.
// Aiming to be used while configuring a TLS client or server.
func (t *TLSConfig) GenerateTLSConfig() (*tls.Config, error) {
	certificate, err := tls.X509KeyPair([]byte(t.Certificate), []byte(t.PrivateKey))
	if err != nil {
		return nil, fmt.Errorf("error loading certicate and private key: %v", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{certificate},

		MinVersion:       tls.VersionTLS12,
		CurvePreferences: []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},

		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		},
	}, nil
}

func (t *TLSConfig) validate() error {
	if t.Certificate == "" {
		return fmt.Errorf("Config error: TLS certificate required")
	}
	if t.PrivateKey == "" {
		return fmt.Errorf("Config error: TLS private key required")
	}
	return nil
}
