package server

import (
	"bufio"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/gorilla/websocket"

	authpkg "github.com/aerostone/webtmux/internal/auth"
	"github.com/aerostone/webtmux/internal/config"
	"github.com/aerostone/webtmux/internal/tmux"
)

//go:embed web
var webFS embed.FS

type Server struct {
	cfg       *config.Config
	mux       *http.ServeMux
	tmux      *tmux.Manager
	filter    *authpkg.IPFilter
	session   *authpkg.SessionManager
	rateLimit *authpkg.RateLimiter
	upgrade   websocket.Upgrader
	credStore *authpkg.CredentialStore
	webAuthn  *webauthn.WebAuthn // fallback when origin not detected
	waCache   map[string]*webauthn.WebAuthn
	waMu      sync.RWMutex
	wsConns   map[string]*websocket.Conn // session name → active WS
	wsConnMu  sync.Mutex
}

type loginRequest struct {
	Password string `json:"password"`
	TOTPCode string `json:"totp_code,omitempty"`
}

type createSessionReq struct {
	Name string `json:"name"`
}

func New(cfg *config.Config) *http.Server {
	filter, _ := authpkg.NewIPFilter(cfg.IPWhitelist)
	session := authpkg.NewSessionManager(cfg.SessionSecret)
	rateLimit := authpkg.NewRateLimiter(
		cfg.MaxLoginAttempts,
		time.Duration(cfg.LoginWindowSec)*time.Second,
		time.Duration(cfg.LoginLockoutSec)*time.Second,
	)

	// WebAuthn setup
	credStore, err := authpkg.NewCredentialStore(cfg.WebAuthnDir)
	if err != nil {
		log.Printf("webauthn: credential store error: %v (fingerprint disabled)", err)
	}
	wa, err := authpkg.NewWebAuthn(cfg.WebAuthnRPID, cfg.WebAuthnOrigin)
	if err != nil {
		log.Printf("webauthn: init error: %v (fingerprint disabled)", err)
	}

	s := &Server{
		cfg:       cfg,
		mux:       http.NewServeMux(),
		tmux:      tmux.NewManager(cfg.TmuxSocket),
		filter:    filter,
		session:   session,
		rateLimit: rateLimit,
		credStore: credStore,
		webAuthn:  wa,
		wsConns:   make(map[string]*websocket.Conn),
		upgrade: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				if cfg.AllowAllOrigins {
					return true
				}
				origin := r.Header.Get("Origin")
				if origin == "" {
					return true
				}
				originURL, err := url.Parse(origin)
				if err != nil {
					return false
				}
				return originURL.Host == r.Host
			},
		},
	}
	s.routes()

	handler := loggingMiddleware(recoveryMiddleware(securityHeaders(
		authpkg.Middleware(filter)(
			authpkg.LoginRequired(&authpkg.AuthConfig{
				Password:    cfg.AuthPass,
				TOTPSecret:  cfg.TOTPSecret,
				TOTPEnabled: cfg.TOTPEnabled,
				Session:     session,
				RateLimiter: rateLimit,
			})(s.mux)))))

	log.Printf("server config: listen=%s ip_whitelist=%v totp=%v",
		cfg.ListenAddr, len(cfg.IPWhitelist) > 0, cfg.TOTPEnabled)

	return &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

// responseWriter wraps http.ResponseWriter to capture status code
// while preserving http.Hijacker (needed for WebSocket upgrades)
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		log.Printf("%s %s %d %s %s",
			r.Method, r.URL.Path, rw.status,
			time.Since(start).Round(time.Millisecond),
			r.RemoteAddr)
	})
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("PANIC [%s %s]: %v", r.Method, r.URL.Path, err)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) routes() {
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/api/login", s.handleLogin)
	s.mux.HandleFunc("/api/logout", s.handleLogout)
	s.mux.HandleFunc("/api/config", s.handleConfig)
	s.mux.HandleFunc("/api/sessions", s.handleSessions)
	s.mux.HandleFunc("/api/sessions/create", s.handleCreateSession)
	s.mux.HandleFunc("/api/sessions/kill", s.handleKillSession)
	s.mux.HandleFunc("/api/webauthn/status", s.handleWebAuthnStatus)
	s.mux.HandleFunc("/api/webauthn/register/start", s.handleWebAuthnRegisterStart)
	s.mux.HandleFunc("/api/webauthn/register/finish", s.handleWebAuthnRegisterFinish)
	s.mux.HandleFunc("/api/webauthn/login/start", s.handleWebAuthnLoginStart)
	s.mux.HandleFunc("/api/webauthn/login/finish", s.handleWebAuthnLoginFinish)
	s.mux.HandleFunc("/api/webauthn/remove", s.handleWebAuthnRemove)
	s.mux.HandleFunc("/ws", s.handleWebSocket)

	webContent, _ := fs.Sub(webFS, "web")
	s.mux.Handle("/", http.FileServer(http.FS(webContent)))
}

// getWebAuthn returns a WebAuthn instance for the request's origin.
// Falls back to the default instance if origin cannot be detected.
func (s *Server) getWebAuthn(r *http.Request) *webauthn.WebAuthn {
	origin := r.Header.Get("Origin")
	if origin == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		origin = fmt.Sprintf("%s://%s", scheme, r.Host)
	}

	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		if s.webAuthn != nil {
			return s.webAuthn
		}
		return nil
	}

	rpID := strings.Split(u.Host, ":")[0] // strip port
	originKey := u.Scheme + "://" + u.Host

	s.waMu.RLock()
	wa, ok := s.waCache[originKey]
	s.waMu.RUnlock()
	if ok {
		return wa
	}

	wa, err = authpkg.NewWebAuthn(rpID, origin)
	if err != nil {
		log.Printf("webauthn: failed to create instance for %s: %v", origin, err)
		return s.webAuthn
	}

	s.waMu.Lock()
	if s.waCache == nil {
		s.waCache = make(map[string]*webauthn.WebAuthn)
	}
	s.waCache[originKey] = wa
	s.waMu.Unlock()

	log.Printf("webauthn: created instance for rpID=%s origin=%s", rpID, origin)
	return wa
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{
		"ws_timeout_sec": s.cfg.WSTimeoutSec,
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	key := s.rateLimit.KeyFromRequest(r.RemoteAddr)
	if !s.rateLimit.Allow(key) {
		http.Error(w, "too many attempts, try later", http.StatusTooManyRequests)
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if req.Password == "" {
		http.Error(w, "password required", http.StatusBadRequest)
		return
	}

	if req.Password != s.cfg.AuthPass {
		s.rateLimit.RecordFailure(key)
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	if s.cfg.TOTPEnabled && s.cfg.TOTPSecret != "" {
		if !authpkg.VerifyTOTP(s.cfg.TOTPSecret, req.TOTPCode) {
			s.rateLimit.RecordFailure(key)
			http.Error(w, "invalid TOTP code", http.StatusUnauthorized)
			return
		}
	}

	s.rateLimit.Reset(key)
	s.session.SetCookie(w)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.session.ClearCookie(w)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.tmux.ListSessions()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if sessions == nil {
		sessions = []tmux.Session{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req createSessionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	if err := s.tmux.CreateSession(req.Name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(`{"status":"created"}`))
}

func (s *Server) handleKillSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req createSessionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	if err := s.tmux.KillSession(req.Name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write([]byte(`{"status":"killed"}`))
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	sessionName := r.URL.Query().Get("session")
	if sessionName == "" {
		http.Error(w, "session parameter required", http.StatusBadRequest)
		return
	}

	log.Printf("ws connect: session=%s remote=%s", sessionName, r.RemoteAddr)

	conn, err := s.upgrade.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// Close old connection for same session (exclusive)
	s.wsConnMu.Lock()
	if old, ok := s.wsConns[sessionName]; ok {
		old.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "replaced by new connection"))
		old.Close()
		log.Printf("ws replaced old connection: session=%s", sessionName)
	}
	s.wsConns[sessionName] = conn
	s.wsConnMu.Unlock()
	defer func() {
		s.wsConnMu.Lock()
		if s.wsConns[sessionName] == conn {
			delete(s.wsConns, sessionName)
		}
		s.wsConnMu.Unlock()
	}()

	ptmx, err := s.tmux.AttachSession(sessionName)
	if err != nil {
		log.Printf("ws attach failed: session=%s err=%v", sessionName, err)
		conn.WriteMessage(websocket.TextMessage,
			[]byte("error: cannot attach to session"))
		return
	}
	defer ptmx.Close()

	log.Printf("ws attached: session=%s", sessionName)

	// Handle resize messages from client
	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				break
			}
			// Try to parse as resize JSON
			var resizeMsg struct {
				Cols int `json:"cols"`
				Rows int `json:"rows"`
			}
			if json.Unmarshal(msg, &resizeMsg) == nil && resizeMsg.Cols > 0 && resizeMsg.Rows > 0 {
				if err := ptmx.Resize(resizeMsg.Cols, resizeMsg.Rows); err != nil {
					log.Printf("ws resize error: session=%s err=%v", sessionName, err)
				} else {
					log.Printf("ws resize: session=%s cols=%d rows=%d", sessionName, resizeMsg.Cols, resizeMsg.Rows)
				}
				continue
			}
			// Regular input
			if _, err := ptmx.Write(msg); err != nil {
				break
			}
		}
	}()

	// Read from tmux, write to ws
	buf := make([]byte, 4096)
	for {
		n, err := ptmx.Read(buf)
		if err != nil {
			log.Printf("ws tmux read error: session=%s err=%v", sessionName, err)
			break
		}
		if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
			log.Printf("ws write error: session=%s err=%v", sessionName, err)
			break
		}
	}

	log.Printf("ws disconnect: session=%s", sessionName)
}

// ─── WebAuthn handlers ───

func (s *Server) handleWebAuthnStatus(w http.ResponseWriter, r *http.Request) {
	wa := s.getWebAuthn(r)
	if wa == nil || s.credStore == nil {
		http.Error(w, "webauthn not available", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{
		"enabled":    true,
		"registered": s.credStore.HasCredentials(),
	})
}

func (s *Server) handleWebAuthnRegisterStart(w http.ResponseWriter, r *http.Request) {
	wa := s.getWebAuthn(r)
	if wa == nil || s.credStore == nil {
		http.Error(w, "webauthn not available", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := authpkg.NewWebAuthnUser(s.credStore)
	credCreation, sessionData, err := wa.BeginRegistration(
		user,
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			AuthenticatorAttachment: protocol.Platform,
			UserVerification:        protocol.VerificationPreferred,
		}),
		webauthn.WithConveyancePreference(protocol.PreferNoAttestation),
	)
	if err != nil {
		log.Printf("webauthn register start error: %v", err)
		http.Error(w, "registration failed", http.StatusInternalServerError)
		return
	}

	// Store session data in a temporary cookie (base64 encoded)
	sessionJSON, _ := json.Marshal(sessionData)
	cookie := &http.Cookie{
		Name:     "webauthn_session",
		Value:    base64.StdEncoding.EncodeToString(sessionJSON),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   300,
	}
	http.SetCookie(w, cookie)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(credCreation)
}

func (s *Server) handleWebAuthnRegisterFinish(w http.ResponseWriter, r *http.Request) {
	wa := s.getWebAuthn(r)
	if wa == nil || s.credStore == nil {
		http.Error(w, "webauthn not available", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Recover session data from cookie
	sessionCookie, err := r.Cookie("webauthn_session")
	if err != nil {
		http.Error(w, "session expired", http.StatusBadRequest)
		return
	}
	decodedCookie, err := base64.StdEncoding.DecodeString(sessionCookie.Value)
	if err != nil {
		http.Error(w, "invalid session", http.StatusBadRequest)
		return
	}
	var sessionData webauthn.SessionData
	if err := json.Unmarshal(decodedCookie, &sessionData); err != nil {
		http.Error(w, "invalid session", http.StatusBadRequest)
		return
	}

	user := authpkg.NewWebAuthnUser(s.credStore)
	cred, err := wa.FinishRegistration(user, sessionData, r)
	if err != nil {
		log.Printf("webauthn register finish error: %v", err)
		http.Error(w, "registration failed", http.StatusBadRequest)
		return
	}

	if err := s.credStore.SaveCredential(cred); err != nil {
		log.Printf("webauthn save credential error: %v", err)
		http.Error(w, "save failed", http.StatusInternalServerError)
		return
	}

	log.Printf("webauthn: credential registered")
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleWebAuthnLoginStart(w http.ResponseWriter, r *http.Request) {
	wa := s.getWebAuthn(r)
	if wa == nil || s.credStore == nil {
		http.Error(w, "webauthn not available", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.credStore.HasCredentials() {
		http.Error(w, "no credentials registered", http.StatusNotFound)
		return
	}

	user := authpkg.NewWebAuthnUser(s.credStore)
	assertion, sessionData, err := wa.BeginLogin(user)
	if err != nil {
		log.Printf("webauthn login start error: %v", err)
		http.Error(w, "login failed", http.StatusInternalServerError)
		return
	}

	sessionJSON, _ := json.Marshal(sessionData)
	cookie := &http.Cookie{
		Name:     "webauthn_session",
		Value:    base64.StdEncoding.EncodeToString(sessionJSON),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   300,
	}
	http.SetCookie(w, cookie)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(assertion)
}

func (s *Server) handleWebAuthnLoginFinish(w http.ResponseWriter, r *http.Request) {
	wa := s.getWebAuthn(r)
	if wa == nil || s.credStore == nil {
		http.Error(w, "webauthn not available", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionCookie, err := r.Cookie("webauthn_session")
	if err != nil {
		http.Error(w, "session expired", http.StatusBadRequest)
		return
	}
	decodedCookie, err := base64.StdEncoding.DecodeString(sessionCookie.Value)
	if err != nil {
		http.Error(w, "invalid session", http.StatusBadRequest)
		return
	}
	var sessionData webauthn.SessionData
	if err := json.Unmarshal(decodedCookie, &sessionData); err != nil {
		http.Error(w, "invalid session", http.StatusBadRequest)
		return
	}

	user := authpkg.NewWebAuthnUser(s.credStore)
	cred, err := wa.FinishLogin(user, sessionData, r)
	if err != nil {
		log.Printf("webauthn login finish error: %v", err)
		http.Error(w, "authentication failed", http.StatusUnauthorized)
		return
	}

	// Update sign count
	if cred != nil {
		s.credStore.UpdateSignCount(cred.ID, cred.Authenticator.SignCount)
	}

	s.session.SetCookie(w)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleWebAuthnRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.credStore == nil {
		http.Error(w, "webauthn not available", http.StatusServiceUnavailable)
		return
	}
	if err := s.credStore.Clear(); err != nil {
		http.Error(w, "remove failed", http.StatusInternalServerError)
		return
	}
	log.Printf("webauthn: all credentials removed")
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}
