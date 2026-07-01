package auth

import (
	"net/http"
	"strings"
)

type AuthConfig struct {
	Password    string
	TOTPSecret  string
	TOTPEnabled bool
	Session     *SessionManager
	RateLimiter *RateLimiter
}

func LoginRequired(cfg *AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublicPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			if cfg.Session.Validate(r) {
				next.ServeHTTP(w, r)
				return
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		})
	}
}

func isPublicPath(path string) bool {
	if path == "/" || path == "/health" || path == "/api/login" || path == "/api/config" {
		return true
	}
	// WebAuthn login endpoints are public (replaces password login)
	if path == "/api/webauthn/login/start" || path == "/api/webauthn/login/finish" || path == "/api/webauthn/status" {
		return true
	}
	if strings.HasPrefix(path, "/login") {
		return true
	}
	exts := []string{".html", ".css", ".js", ".ico", ".png", ".svg"}
	for _, ext := range exts {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}
