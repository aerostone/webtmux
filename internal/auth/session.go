package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"
)

const (
	cookieName = "webtmux_session"
	cookieTTL  = 24 * time.Hour
	payload    = "authenticated"
)

type SessionManager struct {
	secret []byte
}

func NewSessionManager(secret string) *SessionManager {
	if secret == "" {
		b := make([]byte, 32)
		rand.Read(b)
		secret = hex.EncodeToString(b)
	}
	return &SessionManager{secret: []byte(secret)}
}

func (sm *SessionManager) SetCookie(w http.ResponseWriter) {
	sig := sm.sign()
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    sig,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(cookieTTL.Seconds()),
	})
}

func (sm *SessionManager) Validate(r *http.Request) bool {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return false
	}
	return sm.verify(cookie.Value)
}

func (sm *SessionManager) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:   cookieName,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
}

func (sm *SessionManager) sign() string {
	mac := hmac.New(sha256.New, sm.secret)
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func (sm *SessionManager) verify(token string) bool {
	expected, err := hex.DecodeString(sm.sign())
	if err != nil {
		return false
	}
	got, err := hex.DecodeString(token)
	if err != nil {
		return false
	}
	return hmac.Equal(expected, got)
}
