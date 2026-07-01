package auth

import (
	"net"
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
