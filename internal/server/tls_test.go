package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTLSAndMutualTLSConnection(t *testing.T) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "webtmux test CA"},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, err = x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0600); err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	certPath, keyPath := testTLSCertificate(t, ca, caKey, 2, x509.ExtKeyUsageServerAuth)
	clientCertPath, clientKeyPath := testTLSCertificate(t, ca, caKey, 3, x509.ExtKeyUsageClientAuth)
	clientCert, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
	if err != nil {
		t.Fatal(err)
	}

	for _, mutual := range []bool{false, true} {
		name, mode, clientCA := "TLS", "none", ""
		if mutual {
			name, mode, clientCA = "mTLS", "require_and_verify", caPath
		}
		t.Run(name, func(t *testing.T) {
			cfg, err := TLSConfig(certPath, keyPath, clientCA, mode)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.MinVersion != tls.VersionTLS12 {
				t.Fatal("expected TLS 1.2 minimum")
			}
			if mutual && (cfg.ClientAuth != tls.RequireAndVerifyClientCert || cfg.ClientCAs == nil) {
				t.Fatal("expected mandatory verified client certificates")
			}
			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mutual && len(r.TLS.VerifiedChains) == 0 {
					t.Error("client certificate was not verified")
				}
				w.WriteHeader(http.StatusOK)
			}))
			srv.TLS = cfg
			srv.StartTLS()
			defer srv.Close()
			clientTLS := &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
			if mutual {
				clientTLS.Certificates = []tls.Certificate{clientCert}
			}
			transport := &http.Transport{TLSClientConfig: clientTLS}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
			res, err := client.Get(srv.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusOK || len(res.TLS.VerifiedChains) == 0 {
				t.Fatal("expected successful connection with verified server certificate")
			}
		})
	}
}

func testTLSCertificate(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, serial int64, usage x509.ExtKeyUsage) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		NotBefore:    time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, cert, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certPath, keyPath := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0600); err != nil {
		t.Fatal(err)
	}
	return certPath, keyPath
}
