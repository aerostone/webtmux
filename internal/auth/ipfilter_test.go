package auth

import (
	"net"
	"net/http/httptest"
	"testing"
)

func TestIPFilterEmptyAllowsAll(t *testing.T) {
	f, err := NewIPFilter(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !f.Allowed(net.ParseIP("1.2.3.4")) {
		t.Error("empty filter should allow all")
	}
}

func TestClientIPUsesForwardedForOnlyFromTrustedProxy(t *testing.T) {
	trusted, _ := NewIPFilter([]string{"10.0.0.0/8"})

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.2:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.8, 10.0.0.3")
	if got := ClientIP(req, trusted).String(); got != "198.51.100.8" {
		t.Fatalf("trusted proxy client IP = %s, want 198.51.100.8", got)
	}

	req.RemoteAddr = "203.0.113.2:1234"
	if got := ClientIP(req, trusted).String(); got != "203.0.113.2" {
		t.Fatalf("untrusted peer client IP = %s, want 203.0.113.2", got)
	}
}

func TestRequestIsHTTPSOnlyTrustsForwardedProtoFromProxy(t *testing.T) {
	trusted, _ := NewIPFilter([]string{"10.0.0.0/8"})
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.2:1234"
	req.Header.Set("X-Forwarded-Proto", "https")
	if !RequestIsHTTPS(req, trusted) {
		t.Fatal("trusted proxy HTTPS should be detected")
	}
	req.RemoteAddr = "203.0.113.2:1234"
	if RequestIsHTTPS(req, trusted) {
		t.Fatal("untrusted forwarded proto must not be trusted")
	}
}

func TestIPFilterCIDR(t *testing.T) {
	f, err := NewIPFilter([]string{"192.168.1.0/24"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !f.Allowed(net.ParseIP("192.168.1.100")) {
		t.Error("should allow IP in range")
	}
	if f.Allowed(net.ParseIP("10.0.0.1")) {
		t.Error("should reject IP outside range")
	}
}

func TestIPFilterSingleIP(t *testing.T) {
	f, err := NewIPFilter([]string{"10.0.0.1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !f.Allowed(net.ParseIP("10.0.0.1")) {
		t.Error("should allow exact IP")
	}
	if f.Allowed(net.ParseIP("10.0.0.2")) {
		t.Error("should reject different IP")
	}
}
