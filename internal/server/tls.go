package server

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// TLSConfig loads the configured server certificate and optional client CA.
// It never generates certificates and fails closed when TLS is explicitly enabled.
func TLSConfig(certFile, keyFile, clientCAFile, clientAuth string) (*tls.Config, error) {
	if clientAuth != "" && clientAuth != "none" && clientAuth != "require_and_verify" {
		return nil, fmt.Errorf("unsupported TLS client authentication mode %q", clientAuth)
	}
	if clientCAFile != "" && clientAuth != "require_and_verify" {
		return nil, fmt.Errorf("TLS client CA requires require_and_verify client authentication")
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load TLS certificate/key: %w", err)
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}}
	if clientAuth == "require_and_verify" {
		data, err := os.ReadFile(clientCAFile)
		if err != nil {
			return nil, fmt.Errorf("load TLS client CA: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(data) {
			return nil, fmt.Errorf("no certificates found in TLS client CA file")
		}
		cfg.ClientCAs = pool
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return cfg, nil
}
