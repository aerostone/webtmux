package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

func TestTOTPGenerateAndVerify(t *testing.T) {
	secret, err := GenerateTOTPSecret("test@example.com")
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if secret == "" {
		t.Fatal("expected non-empty secret")
	}

	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("generate code failed: %v", err)
	}
	if !VerifyTOTP(secret, code) {
		t.Error("valid code should pass verification")
	}
}

func TestTOTPInvalidCode(t *testing.T) {
	secret, err := GenerateTOTPSecret("test@example.com")
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if VerifyTOTP(secret, "000000") {
		t.Error("invalid code should fail verification")
	}
}

func TestMiddlewareAllowsWhitelistedIP(t *testing.T) {
	filter, _ := NewIPFilter([]string{"192.168.1.0/24"})
	mw := Middleware(filter)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestMiddlewareBlocksNonWhitelistedIP(t *testing.T) {
	filter, _ := NewIPFilter([]string{"192.168.1.0/24"})
	mw := Middleware(filter)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestMiddlewareEmptyWhitelistAllowsAll(t *testing.T) {
	filter, _ := NewIPFilter(nil)
	mw := Middleware(filter)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:12345"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
