package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoginRequiredBlocksAPI(t *testing.T) {
	sm := NewSessionManager("test")
	cfg := &AuthConfig{Session: sm}
	mw := LoginRequired(cfg)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/sessions", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestLoginRequiredBlocksWebSocket(t *testing.T) {
	sm := NewSessionManager("test")
	cfg := &AuthConfig{Session: sm}
	mw := LoginRequired(cfg)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/ws?session=test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestLoginRequiredAllowsWithCookie(t *testing.T) {
	sm := NewSessionManager("test")
	cfg := &AuthConfig{Session: sm}
	mw := LoginRequired(cfg)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w1 := httptest.NewRecorder()
	sm.SetCookie(w1, false)
	cookie := w1.Result().Cookies()[0]

	req := httptest.NewRequest("GET", "/api/sessions", nil)
	req.AddCookie(cookie)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req)

	if w2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w2.Code)
	}
}

func TestLoginRequiredAllowsPublicPaths(t *testing.T) {
	sm := NewSessionManager("test")
	cfg := &AuthConfig{Session: sm}
	mw := LoginRequired(cfg)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	paths := []string{"/", "/health", "/api/login", "/index.html", "/style.css"}
	for _, path := range paths {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("path %s: expected 200, got %d", path, w.Code)
		}
	}
}
