package server

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aerostone/webtmux/internal/config"
	"github.com/aerostone/webtmux/internal/tmux"
	"github.com/gorilla/websocket"

	authpkg "github.com/aerostone/webtmux/internal/auth"
)

func TestHealthEndpoint(t *testing.T) {
	cfg := &config.Config{
		ListenAddr:       ":0",
		AuthPass:         "test",
		MaxLoginAttempts: 5,
		LoginWindowSec:   300,
		LoginLockoutSec:  900,
	}
	srv := New(cfg)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestBasePathRoutesAndInjectsPrefix(t *testing.T) {
	cfg := &config.Config{
		ListenAddr:       ":0",
		BasePath:         "/tmux",
		AuthPass:         "test",
		MaxLoginAttempts: 5,
		LoginWindowSec:   300,
		LoginLockoutSec:  900,
	}
	srv := New(cfg)

	root := httptest.NewRecorder()
	srv.Handler.ServeHTTP(root, httptest.NewRequest(http.MethodGet, "/tmux/", nil))
	if root.Code != http.StatusOK || !strings.Contains(root.Body.String(), `window.WEBTMUX_BASE_PATH="/tmux"`) {
		t.Fatalf("base path root response = %d, body missing prefix", root.Code)
	}

	api := httptest.NewRecorder()
	srv.Handler.ServeHTTP(api, httptest.NewRequest(http.MethodGet, "/tmux/api/config", nil))
	if api.Code != http.StatusOK {
		t.Fatalf("base path API status = %d, want 200", api.Code)
	}

	outside := httptest.NewRecorder()
	srv.Handler.ServeHTTP(outside, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if outside.Code != http.StatusNotFound {
		t.Fatalf("outside base path status = %d, want 404", outside.Code)
	}
	for _, path := range []string{"/?view=sessions", "/tmux?view=sessions"} {
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusPermanentRedirect || w.Header().Get("Location") != "/tmux/?view=sessions" {
			t.Fatalf("redirect %q: status=%d location=%q", path, w.Code, w.Header().Get("Location"))
		}
	}
}

func TestBasePathLoginLogoutBehindProxy(t *testing.T) {
	cfg := &config.Config{
		BasePath: "/tmux", AuthPass: "test",
		MaxLoginAttempts: 5, LoginWindowSec: 300, LoginLockoutSec: 900,
		TrustedProxyCIDRs: []string{"127.0.0.1/32"},
	}
	srv := New(cfg)
	login := httptest.NewRequest(http.MethodPost, "/tmux/api/login", strings.NewReader(`{"password":"test"}`))
	login.RemoteAddr = "127.0.0.1:12345"
	login.Header.Set("X-Forwarded-For", "192.0.2.1")
	login.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, login)
	if w.Code != http.StatusOK {
		t.Fatalf("login status = %d", w.Code)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Path != "/tmux/" || !cookies[0].Secure {
		t.Fatalf("unexpected login cookies: %v", cookies)
	}
	logout := httptest.NewRequest(http.MethodGet, "/tmux/api/logout", nil)
	logout.AddCookie(cookies[0])
	clear := httptest.NewRecorder()
	srv.Handler.ServeHTTP(clear, logout)
	if clear.Code != http.StatusOK || len(clear.Result().Cookies()) != 1 {
		t.Fatalf("logout status=%d cookies=%v", clear.Code, clear.Result().Cookies())
	}
	deleted := clear.Result().Cookies()[0]
	if deleted.Path != "/tmux/" || deleted.MaxAge != -1 {
		t.Fatalf("unexpected logout cookie: %v", deleted)
	}
}

func TestBasePathWebAuthnChallengeCookie(t *testing.T) {
	store, err := authpkg.NewCredentialStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	trusted, err := authpkg.NewIPFilter([]string{"127.0.0.1/32"})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{cfg: &config.Config{BasePath: "/tmux"}, credStore: store, trustedProxies: trusted}
	register := httptest.NewRequest(http.MethodGet, "http://example.com/api/webauthn/register/start", nil)
	register.RemoteAddr = "127.0.0.1:12345"
	register.Header.Set("X-Forwarded-Proto", "https")
	challenge := httptest.NewRecorder()
	s.handleWebAuthnRegisterStart(challenge, register)
	cookies := challenge.Result().Cookies()
	if challenge.Code != http.StatusOK || len(cookies) != 1 || cookies[0].Path != "/tmux/" || !cookies[0].Secure {
		t.Fatalf("registration challenge: status=%d cookies=%v", challenge.Code, cookies)
	}
}

func TestBasePathWebSocketConnection(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	// Use an isolated tmux server and unique session; never attach to user sessions.
	socket := fmt.Sprintf("webtmux-base-path-%d", time.Now().UnixNano())
	name := socket
	manager := tmux.NewManager(socket)
	if err := manager.CreateSession(name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { manager.KillSession(name) })
	cfg := &config.Config{
		BasePath: "/tmux", AuthPass: "test", TmuxSocket: socket,
		MaxLoginAttempts: 5, LoginWindowSec: 300, LoginLockoutSec: 900,
	}
	srv := httptest.NewServer(New(cfg).Handler)
	defer srv.Close()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, Timeout: 5 * time.Second}
	login, err := client.Post(srv.URL+"/tmux/api/login", "application/json", strings.NewReader(`{"password":"test"}`))
	if err != nil {
		t.Fatal(err)
	}
	login.Body.Close()
	if login.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d", login.StatusCode)
	}
	dialer := websocket.Dialer{Jar: jar, HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/tmux/ws?session="+url.QueryEscape(name), http.Header{"Origin": []string{srv.URL}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("terminal output: %v", err)
	}
	if err := conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "test complete")); err != nil {
		t.Fatal(err)
	}
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				t.Fatalf("close terminal connection: %v", err)
			}
			break
		}
	}
}

func TestSessionsRequiresAuth(t *testing.T) {
	cfg := &config.Config{
		ListenAddr:       ":0",
		AuthPass:         "test",
		MaxLoginAttempts: 5,
		LoginWindowSec:   300,
		LoginLockoutSec:  900,
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
		LoginWindowSec:   300,
		LoginLockoutSec:  900,
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
		LoginWindowSec:   300,
		LoginLockoutSec:  900,
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
	if seconds, err := strconv.Atoi(w.Header().Get("Retry-After")); err != nil || seconds <= 0 || seconds > cfg.LoginLockoutSec {
		t.Errorf("expected remaining lockout seconds, got %q", w.Header().Get("Retry-After"))
	}
	for _, endpoint := range []struct{ method, path string }{
		{http.MethodGet, "/api/webauthn/login/start"},
		{http.MethodPost, "/api/webauthn/login/finish"},
	} {
		req := httptest.NewRequest(endpoint.method, endpoint.path, nil)
		req.RemoteAddr = "10.0.0.1:12345"
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)
		if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
			t.Errorf("%s: expected shared lockout and Retry-After, got %d", endpoint.path, w.Code)
		}
	}
}

func TestCreateSessionAPI(t *testing.T) {
	cfg := &config.Config{
		ListenAddr:       ":0",
		AuthPass:         "test",
		MaxLoginAttempts: 5,
		LoginWindowSec:   300,
		LoginLockoutSec:  900,
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
