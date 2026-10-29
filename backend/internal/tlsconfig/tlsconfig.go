// Package tlsconfig builds the TLS settings for calling another service.
package tlsconfig

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
)

func Client(certFile, keyFile, caFile string) (*tls.Config, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS13}

	if caFile != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("read CA file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("no certificates found in " + caFile)
		}
		cfg.RootCAs = pool
	}
	if certFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("load client certificate: %w", err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return cfg, nil
}
