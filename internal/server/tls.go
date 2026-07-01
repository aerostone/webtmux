package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

const (
	defaultDir  = ".webtmux"
	certFile    = "cert.pem"
	keyFile     = "key.pem"
	certDays    = 365 * 10 // 10 years
)

// EnsureTLS returns cert/key paths. If neither is configured, it auto-generates
// a self-signed ECDSA certificate persisted under ~/.webtmux/.
// Returns (certPath, keyPath, error).
func EnsureTLS(certPath, keyPath string) (string, string, error) {
	if certPath != "" && keyPath != "" {
		return certPath, keyPath, nil
	}

	dir, err := defaultTLSDir()
	if err != nil {
		return "", "", err
	}

	certPath = filepath.Join(dir, certFile)
	keyPath = filepath.Join(dir, keyFile)

	if fileExists(certPath) && fileExists(keyPath) {
		return certPath, keyPath, nil
	}

	if err := generateCert(certPath, keyPath); err != nil {
		return "", "", err
	}
	return certPath, keyPath, nil
}

func defaultTLSDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, defaultDir)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func generateCert(certPath, keyPath string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}

	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "webtmux",
			Organization: []string{"webtmux"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(0, 0, certDays),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Add SANs for common local addresses
	tmpl.IPAddresses = []net.IP{
		net.ParseIP("127.0.0.1"),
		net.ParseIP("::1"),
		net.ParseIP("0.0.0.0"),
	}
	tmpl.DNSNames = []string{"localhost"}

	// Add all non-loopback interface IPs
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && !ip.IsLoopback() {
				tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
			}
		}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}

	// Write cert
	certOut, err := os.OpenFile(certPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer certOut.Close()
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	// Write key
	keyOut, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer keyOut.Close()
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return nil
}
