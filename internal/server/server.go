package server

import (
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"

	"github.com/aerostone/webtmux/internal/auth"
	"github.com/aerostone/webtmux/internal/config"
	"github.com/aerostone/webtmux/internal/tmux"
)

//go:embed web
var webFS embed.FS

type Server struct {
	cfg       *config.Config
	mux       *http.ServeMux
	tmux      *tmux.Manager
	filter    *auth.IPFilter
	session   *auth.SessionManager
	rateLimit *auth.RateLimiter
	upgrade   websocket.Upgrader
}

type loginRequest struct {
	Password string `json:"password"`
	TOTPCode string `json:"totp_code,omitempty"`
}

type createSessionReq struct {
	Name string `json:"name"`
}

func New(cfg *config.Config) *http.Server {
	filter, _ := auth.NewIPFilter(cfg.IPWhitelist)
	session := auth.NewSessionManager(cfg.SessionSecret)
	rateLimit := auth.NewRateLimiter(
		cfg.MaxLoginAttempts,
		time.Duration(cfg.LoginWindowSec)*time.Second,
		time.Duration(cfg.LoginLockoutSec)*time.Second,
	)

	s := &Server{
		cfg:       cfg,
		mux:       http.NewServeMux(),
		tmux:      tmux.NewManager(cfg.TmuxSocket),
		filter:    filter,
		session:   session,
		rateLimit: rateLimit,
		upgrade: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				if cfg.AllowAllOrigins {
					return true
				}
				origin := r.Header.Get("Origin")
				if origin == "" {
					return true
				}
				// Allow same-origin: compare host portion of Origin with Host header
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
		auth.Middleware(s.filter)(
			auth.LoginRequired(&auth.AuthConfig{
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
	s.mux.HandleFunc("/ws", s.handleWebSocket)

	webContent, _ := fs.Sub(webFS, "web")
	s.mux.Handle("/", http.FileServer(http.FS(webContent)))
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
		if !auth.VerifyTOTP(s.cfg.TOTPSecret, req.TOTPCode) {
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
