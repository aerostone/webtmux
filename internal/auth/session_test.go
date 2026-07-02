package auth

import (
	"net/http/httptest"
	"testing"
)

func TestSessionSetAndValidate(t *testing.T) {
	sm := NewSessionManager("test-secret-key")

	w := httptest.NewRecorder()
	sm.SetCookie(w, false)

	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	if cookies[0].Name != cookieName {
		t.Errorf("expected cookie name %q, got %q", cookieName, cookies[0].Name)
	}
	if !cookies[0].HttpOnly {
		t.Error("cookie should be HttpOnly")
	}

	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(cookies[0])
	if !sm.Validate(r) {
		t.Error("valid cookie should pass validation")
	}
}

func TestSessionRejectsInvalid(t *testing.T) {
	sm := NewSessionManager("test-secret-key")

	r := httptest.NewRequest("GET", "/", nil)
	if sm.Validate(r) {
		t.Error("no cookie should fail validation")
	}
}

func TestSessionRejectsTampered(t *testing.T) {
	sm := NewSessionManager("test-secret-key")

	w := httptest.NewRecorder()
	sm.SetCookie(w, false)
	cookie := w.Result().Cookies()[0]

	cookie.Value = cookie.Value[:len(cookie.Value)-2] + "ff"
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(cookie)
	if sm.Validate(r) {
		t.Error("tampered cookie should fail validation")
	}
}

func TestSessionDifferentSecrets(t *testing.T) {
	sm1 := NewSessionManager("secret-1")
	sm2 := NewSessionManager("secret-2")

	w := httptest.NewRecorder()
	sm1.SetCookie(w, false)
	cookie := w.Result().Cookies()[0]

	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(cookie)
	if sm2.Validate(r) {
		t.Error("cookie from different secret should fail")
	}
}

func TestSessionClearCookie(t *testing.T) {
	sm := NewSessionManager("test-secret")

	w := httptest.NewRecorder()
	sm.ClearCookie(w)

	cookie := w.Result().Cookies()[0]
	if cookie.MaxAge != -1 {
		t.Errorf("expected MaxAge=-1, got %d", cookie.MaxAge)
	}
}
