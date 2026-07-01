package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aerostone/webtmux/internal/config"
)

func TestHealthEndpoint(t *testing.T) {
	cfg := &config.Config{
		ListenAddr:       ":0",
		AuthPass:         "test",
		MaxLoginAttempts: 5,
		LoginWindowSec:  300,
		LoginLockoutSec: 900,
	}
	srv := New(cfg)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestSessionsRequiresAuth(t *testing.T) {
	cfg := &config.Config{
		ListenAddr:       ":0",
		AuthPass:         "test",
		MaxLoginAttempts: 5,
		LoginWindowSec:  300,
		LoginLockoutSec: 900,
	}
	srv := New(cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestLoginAndAccess(t *testing.T) {
	cfg := &config.Config{
		ListenAddr:       ":0",
		AuthPass:         "secret123",
		MaxLoginAttempts: 5,
		LoginWindowSec:  300,
		LoginLockoutSec: 900,
	}
	srv := New(cfg)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/login",
		strings.NewReader(`{"password":"secret123"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, loginReq)

	if w.Code != http.StatusOK {
		t.Fatalf("login failed: %d", w.Code)
	}

	cookies := w.Result().Cookies()
	sessReq := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	for _, c := range cookies {
		sessReq.AddCookie(c)
	}
	w2 := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w2, sessReq)

	if w2.Code != http.StatusOK {
		t.Errorf("expected 200 with cookie, got %d", w2.Code)
	}
}

func TestLoginRateLimit(t *testing.T) {
	cfg := &config.Config{
		ListenAddr:       ":0",
		AuthPass:         "secret",
		MaxLoginAttempts: 2,
		LoginWindowSec:  300,
		LoginLockoutSec: 900,
	}
	srv := New(cfg)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/login",
			strings.NewReader(`{"password":"wrong"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "10.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/login",
		strings.NewReader(`{"password":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w.Code)
	}
}

func TestCreateSessionAPI(t *testing.T) {
	cfg := &config.Config{
		ListenAddr:       ":0",
		AuthPass:         "test",
		MaxLoginAttempts: 5,
		LoginWindowSec:  300,
		LoginLockoutSec: 900,
	}
	srv := New(cfg)

	// Login first
	loginReq := httptest.NewRequest(http.MethodPost, "/api/login",
		strings.NewReader(`{"password":"test"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	srv.Handler.ServeHTTP(loginW, loginReq)

	cookies := loginW.Result().Cookies()

	// Create session
	createReq := httptest.NewRequest(http.MethodPost, "/api/sessions/create",
		strings.NewReader(`{"name":"test-api-session"}`))
	createReq.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		createReq.AddCookie(c)
	}
	createW := httptest.NewRecorder()
	srv.Handler.ServeHTTP(createW, createReq)

	if createW.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", createW.Code)
	}

	// Kill session
	killReq := httptest.NewRequest(http.MethodPost, "/api/sessions/kill",
		strings.NewReader(`{"name":"test-api-session"}`))
	killReq.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		killReq.AddCookie(c)
	}
	killW := httptest.NewRecorder()
	srv.Handler.ServeHTTP(killW, killReq)

	if killW.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", killW.Code)
	}
}
