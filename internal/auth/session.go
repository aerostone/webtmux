package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	cookieName = "webtmux_session"
	cookieTTL  = 24 * time.Hour // browser cookie max lifetime (hard limit)
)

var (
	// SessionTimeout is the sliding window timeout (default 2h, configurable)
	SessionTimeout = 2 * time.Hour

	// serverStartTime is set once at process start. Changing it invalidates all old cookies.
	serverStartTime = time.Now().Unix()
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

// SetCookie signs and sets a session cookie with the current timestamp.
func (sm *SessionManager) SetCookie(w http.ResponseWriter, secure bool) {
	sig := sm.signNow()
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    sig,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(cookieTTL.Seconds()),
	})
}

// Validate checks the cookie signature and expiry.
// Returns true only if the cookie is valid AND not expired.
func (sm *SessionManager) Validate(r *http.Request) bool {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return false
	}
	return sm.verifyWithExpiry(cookie.Value)
}

// RefreshIfStale extends the cookie if more than half the timeout has passed.
// Call this after successful validation to implement sliding window.
func (sm *SessionManager) RefreshIfStale(w http.ResponseWriter, r *http.Request, secure bool) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return
	}
	ts := sm.extractTimestamp(cookie.Value)
	if ts <= 0 {
		return
	}
	elapsed := time.Since(time.Unix(ts, 0))
	// Refresh if more than half the timeout has passed
	if elapsed > SessionTimeout/2 {
		sm.SetCookie(w, secure)
	}
}

// ClearCookie removes the session cookie.
func (sm *SessionManager) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:   cookieName,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
}

// signNow creates a signature over "authenticated:<timestamp>:<server_start>".
func (sm *SessionManager) signNow() string {
	ts := time.Now().Unix()
	payload := fmt.Sprintf("authenticated:%d:%d", ts, serverStartTime)
	mac := hmac.New(sha256.New, sm.secret)
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("%s.%d.%d", sig, ts, serverStartTime)
}

// verifyWithExpiry checks both the HMAC signature and the timestamp expiry.
func (sm *SessionManager) verifyWithExpiry(token string) bool {
	sig, ts, start, ok := parseToken(token)
	if !ok {
		return false
	}

	// Reject cookies from previous server instance
	if start != serverStartTime {
		return false
	}

	// Check expiry
	age := time.Since(time.Unix(ts, 0))
	if age > SessionTimeout {
		return false
	}

	// Verify HMAC
	payload := fmt.Sprintf("authenticated:%d:%d", ts, start)
	mac := hmac.New(sha256.New, sm.secret)
	mac.Write([]byte(payload))
	expected := mac.Sum(nil)

	got, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}

	return hmac.Equal(expected, got)
}

// extractTimestamp returns the unix timestamp from a token, or -1 if invalid.
func (sm *SessionManager) extractTimestamp(token string) int64 {
	_, ts, _, ok := parseToken(token)
	if !ok {
		return -1
	}
	return ts
}

// parseToken splits "sig.timestamp.serverStart" into (sig, timestamp, serverStart, ok).
func parseToken(token string) (string, int64, int64, bool) {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return "", 0, 0, false
	}
	ts, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || ts <= 0 {
		return "", 0, 0, false
	}
	start, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || start <= 0 {
		return "", 0, 0, false
	}
	return parts[0], ts, start, true
}
