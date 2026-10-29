package config

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

func expandHome(path string) string {
	if len(path) >= 1 && path[0] == '~' {
		if home, err := os.UserHomeDir(); err == nil {
			return home + path[1:]
		}
	}
	return path
}

type Config struct {
	ListenAddr        string   `json:"listen_addr" yaml:"listen_addr"`
	BasePath          string   `json:"base_path" yaml:"base_path"`
	TLSCertFile       string   `json:"tls_cert_file" yaml:"tls_cert_file"`
	TLSKeyFile        string   `json:"tls_key_file" yaml:"tls_key_file"`
	TLSEnabled        bool     `json:"tls_enabled" yaml:"tls_enabled"`
	TLSClientCAFile   string   `json:"tls_client_ca_file" yaml:"tls_client_ca_file"`
	TLSClientAuth     string   `json:"tls_client_auth" yaml:"tls_client_auth"`
	NoTLS             bool     `json:"no_tls" yaml:"no_tls"`
	TrustedProxyCIDRs []string `json:"trusted_proxy_cidrs" yaml:"trusted_proxy_cidrs"`
	TmuxSocket        string   `json:"tmux_socket" yaml:"tmux_socket"`
	IPWhitelist       []string `json:"ip_whitelist" yaml:"ip_whitelist"`
	TOTPEnabled       bool     `json:"totp_enabled" yaml:"totp_enabled"`
	TOTPSecret        string   `json:"totp_secret" yaml:"totp_secret"`
	AuthUser          string   `json:"auth_user" yaml:"auth_user"`
	AuthPass          string   `json:"auth_pass" yaml:"auth_pass"`
	SessionSecret     string   `json:"session_secret" yaml:"session_secret"`
	MaxLoginAttempts  int      `json:"max_login_attempts" yaml:"max_login_attempts"`
	LoginWindowSec    int      `json:"login_window_sec" yaml:"login_window_sec"`
	LoginLockoutSec   int      `json:"login_lockout_sec" yaml:"login_lockout_sec"`
	WSTimeoutSec      int      `json:"ws_timeout_sec" yaml:"ws_timeout_sec"`
	AllowAllOrigins   bool     `json:"allow_all_origins" yaml:"allow_all_origins"`
	WebAuthnRPID      string   `json:"webauthn_rpid" yaml:"webauthn_rpid"`
	WebAuthnOrigin    string   `json:"webauthn_origin" yaml:"webauthn_origin"`
	WebAuthnDir       string   `json:"webauthn_dir" yaml:"webauthn_dir"`
	FileRoots         []string `json:"file_roots" yaml:"file_roots"`
	SentryDSN         string   `json:"sentry_dsn" yaml:"sentry_dsn"`
	SessionTimeoutSec int      `json:"session_timeout_sec" yaml:"session_timeout_sec"`
	LogLevel          string   `json:"log_level" yaml:"log_level"`               // DEBUG, INFO, WARN, ERROR
	LogDir            string   `json:"log_dir" yaml:"log_dir"`                   // log file directory, empty = temp
	LogMaxAgeDays     int      `json:"log_max_age_days" yaml:"log_max_age_days"` // days to keep logs (default 3)
}

func Load() (*Config, error) {
	return loadWithArgs(os.Args[1:])
}

func loadWithArgs(args []string) (*Config, error) {
	cfg := &Config{
		ListenAddr:        ":8080",
		TLSClientAuth:     "none",
		MaxLoginAttempts:  5,
		LoginWindowSec:    300,
		LoginLockoutSec:   900,
		WSTimeoutSec:      0,
		WebAuthnRPID:      "localhost",
		WebAuthnOrigin:    "http://localhost:8080",
		WebAuthnDir:       "~/.webtmux",
		SessionTimeoutSec: 7200,
	}

	fs := flag.NewFlagSet("webtmux", flag.ContinueOnError)
	configFile := fs.String("config", "", "path to YAML config file")
	_ = fs.String("listen", "", "listen address")
	_ = fs.String("base-path", "", "URL base path (for example /webtmux)")
	_ = fs.Bool("tls", false, "enable TLS")
	_ = fs.String("tls-cert", "", "TLS certificate chain PEM file (required with -tls)")
	_ = fs.String("tls-key", "", "TLS private key PEM file (required with -tls)")
	_ = fs.String("tls-client-ca", "", "CA bundle for verifying TLS client certificates")
	_ = fs.String("tls-client-auth", "", "TLS client authentication mode")
	_ = fs.Bool("no-tls", false, "disable TLS, use plain HTTP/WS")
	_ = fs.String("trusted-proxy-cidrs", "", "CIDRs allowed to provide forwarded HTTPS headers")
	_ = fs.String("socket", "", "tmux socket path")
	_ = fs.Bool("totp", false, "enable TOTP authentication")
	_ = fs.Int("max-login-attempts", 0, "authentication attempt budget per client IP")
	_ = fs.Int("login-window", 0, "authentication attempt window in seconds")
	_ = fs.Int("login-lockout", 0, "authentication lockout in seconds")
	_ = fs.String("ip-whitelist", "", "comma-separated IP whitelist (CIDR)")
	_ = fs.String("user", "", "basic auth username")
	_ = fs.String("pass", "", "basic auth password")
	_ = fs.Int("ws-timeout", 0, "WebSocket idle timeout seconds (0=server default)")
	_ = fs.String("file-roots", "", "comma-separated additional file manager root directories")
	_ = fs.Int("session-timeout", 0, "session timeout seconds (0=server default 7200)")
	_ = fs.String("log-level", "", "log level: DEBUG, INFO, WARN, ERROR")
	_ = fs.String("log-dir", "", "log file directory (default: temp dir)")
	_ = fs.Int("log-max-age", 0, "days to keep log files (default: 3)")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	// Layer 1: YAML file (lowest priority)
	if path := resolveConfigFile(*configFile); path != "" {
		if err := loadYAML(path, cfg); err != nil {
			return nil, err
		}
	}

	// Layer 2: Environment variables (override YAML)
	if err := applyEnv(cfg); err != nil {
		return nil, err
	}

	// Layer 3: CLI flags (highest priority, only if explicitly set)
	applyFlag(fs, "listen", func(v string) { cfg.ListenAddr = v })
	applyFlag(fs, "base-path", func(v string) { cfg.BasePath = v })
	applyFlag(fs, "tls-cert", func(v string) { cfg.TLSCertFile = v })
	applyFlag(fs, "tls-key", func(v string) { cfg.TLSKeyFile = v })
	applyFlag(fs, "tls-client-ca", func(v string) { cfg.TLSClientCAFile = v })
	applyFlag(fs, "tls-client-auth", func(v string) { cfg.TLSClientAuth = v })
	applyFlag(fs, "trusted-proxy-cidrs", func(v string) { cfg.TrustedProxyCIDRs = splitCSV(v) })
	applyFlag(fs, "socket", func(v string) { cfg.TmuxSocket = v })
	applyFlag(fs, "user", func(v string) { cfg.AuthUser = v })
	applyFlag(fs, "pass", func(v string) { cfg.AuthPass = v })
	applyFlag(fs, "ip-whitelist", func(v string) { cfg.IPWhitelist = strings.Split(v, ",") })
	applyFlag(fs, "file-roots", func(v string) { cfg.FileRoots = strings.Split(v, ",") })
	applyFlag(fs, "log-level", func(v string) { cfg.LogLevel = v })
	applyFlag(fs, "log-dir", func(v string) { cfg.LogDir = v })

	fs.Visit(func(f *flag.Flag) {
		if f.Name == "totp" {
			cfg.TOTPEnabled = f.Value.(flag.Getter).Get().(bool)
		}
		if f.Name == "no-tls" {
			cfg.NoTLS = f.Value.(flag.Getter).Get().(bool)
		}
		if f.Name == "tls" {
			cfg.TLSEnabled = f.Value.(flag.Getter).Get().(bool)
		}
		if f.Name == "max-login-attempts" {
			cfg.MaxLoginAttempts = f.Value.(flag.Getter).Get().(int)
		}
		if f.Name == "login-window" {
			cfg.LoginWindowSec = f.Value.(flag.Getter).Get().(int)
		}
		if f.Name == "login-lockout" {
			cfg.LoginLockoutSec = f.Value.(flag.Getter).Get().(int)
		}
		if f.Name == "ws-timeout" {
			cfg.WSTimeoutSec = f.Value.(flag.Getter).Get().(int)
		}
		if f.Name == "session-timeout" {
			if v := f.Value.(flag.Getter).Get().(int); v > 0 {
				cfg.SessionTimeoutSec = v
			}
		}
		if f.Name == "log-max-age" {
			if v := f.Value.(flag.Getter).Get().(int); v > 0 {
				cfg.LogMaxAgeDays = v
			}
		}
	})

	// Expand ~ in WebAuthnDir
	cfg.WebAuthnDir = expandHome(cfg.WebAuthnDir)
	cfg.TLSCertFile = expandHome(cfg.TLSCertFile)
	cfg.TLSKeyFile = expandHome(cfg.TLSKeyFile)
	cfg.TLSClientCAFile = expandHome(cfg.TLSClientCAFile)
	if basePath, err := NormalizeBasePath(cfg.BasePath); err != nil {
		return nil, err
	} else {
		cfg.BasePath = basePath
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func applyFlag(fs *flag.FlagSet, name string, setter func(string)) {
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			setter(f.Value.String())
		}
	})
}

func resolveConfigFile(flagPath string) string {
	if flagPath != "" {
		return flagPath
	}
	return os.Getenv("WEBTMUX_CONFIG")
}

func loadYAML(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(cfg); err != nil && err != io.EOF {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return err
		}
		return fmt.Errorf("config file must contain a single YAML document")
	}
	return nil
}

// NormalizeBasePath returns a safe URL path prefix without a trailing slash.
// The empty string means the application is mounted at the domain root.
func NormalizeBasePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" || path == "/" {
		return "", nil
	}
	if !strings.HasPrefix(path, "/") || strings.Contains(path, "//") {
		return "", fmt.Errorf("base_path must be a URL path beginning with /")
	}
	path = strings.TrimRight(path, "/")
	if path == "" || path == "/" {
		return "", nil
	}
	for _, segment := range strings.Split(path, "/")[1:] {
		if segment == "." || segment == ".." || segment == "" {
			return "", fmt.Errorf("base_path contains an invalid path segment")
		}
		for _, r := range segment {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("-._~", r)) {
				return "", fmt.Errorf("base_path contains an invalid character")
			}
		}
	}
	return path, nil
}

func (c *Config) Validate() error {
	if c.TLSClientAuth == "" {
		c.TLSClientAuth = "none"
	}
	if c.TLSClientAuth != "none" && c.TLSClientAuth != "require_and_verify" {
		return fmt.Errorf("tls_client_auth must be none or require_and_verify")
	}
	if (c.TLSCertFile == "") != (c.TLSKeyFile == "") {
		return fmt.Errorf("tls_cert_file and tls_key_file must be configured together")
	}
	if c.TLSEnabled {
		if c.NoTLS {
			return fmt.Errorf("tls_enabled cannot be combined with no_tls")
		}
		if c.TLSCertFile == "" {
			return fmt.Errorf("tls_enabled requires tls_cert_file and tls_key_file")
		}
	}
	if c.TLSClientAuth == "require_and_verify" {
		if !c.TLSEnabled || c.NoTLS {
			return fmt.Errorf("tls_client_auth requires enabled TLS")
		}
		if c.TLSClientCAFile == "" {
			return fmt.Errorf("tls_client_auth=require_and_verify requires tls_client_ca_file")
		}
	}
	if c.TLSClientCAFile != "" && c.TLSClientAuth == "none" {
		return fmt.Errorf("tls_client_ca_file requires tls_client_auth=require_and_verify")
	}
	for _, raw := range c.TrustedProxyCIDRs {
		if _, _, err := net.ParseCIDR(raw); err != nil && net.ParseIP(raw) == nil {
			return fmt.Errorf("invalid trusted proxy address %q", raw)
		}
	}
	if c.MaxLoginAttempts <= 0 {
		return fmt.Errorf("max_login_attempts must be positive")
	}
	for name, seconds := range map[string]int{"login_window_sec": c.LoginWindowSec, "login_lockout_sec": c.LoginLockoutSec} {
		if seconds <= 0 || int64(seconds) > int64(time.Duration(1<<63-1)/time.Second) {
			return fmt.Errorf("%s must be a positive duration in seconds", name)
		}
	}
	return nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

func applyEnv(cfg *Config) error {
	if v := os.Getenv("WEBTMUX_LISTEN"); v != "" {
		cfg.ListenAddr = v
	}
	if v, ok := os.LookupEnv("WEBTMUX_BASE_PATH"); ok {
		cfg.BasePath = v
	}
	if v := os.Getenv("WEBTMUX_SOCKET"); v != "" {
		cfg.TmuxSocket = v
	}
	if v := os.Getenv("WEBTMUX_USER"); v != "" {
		cfg.AuthUser = v
	}
	if v := os.Getenv("WEBTMUX_PASS"); v != "" {
		cfg.AuthPass = v
	}
	if v := os.Getenv("WEBTMUX_IP_WHITELIST"); v != "" {
		cfg.IPWhitelist = strings.Split(v, ",")
	}
	if v := os.Getenv("WEBTMUX_SESSION_SECRET"); v != "" {
		cfg.SessionSecret = v
	}
	if v := os.Getenv("WEBTMUX_TLS_CERT"); v != "" {
		cfg.TLSCertFile = v
	}
	if v := os.Getenv("WEBTMUX_TLS_KEY"); v != "" {
		cfg.TLSKeyFile = v
	}
	if v := os.Getenv("WEBTMUX_TLS_CLIENT_CA"); v != "" {
		cfg.TLSClientCAFile = v
	}
	if v := os.Getenv("WEBTMUX_TLS_CLIENT_AUTH"); v != "" {
		cfg.TLSClientAuth = v
	}
	if v, ok := os.LookupEnv("WEBTMUX_TRUSTED_PROXY_CIDRS"); ok {
		cfg.TrustedProxyCIDRs = splitCSV(v)
	}
	if v := os.Getenv("WEBTMUX_TOTP_SECRET"); v != "" {
		cfg.TOTPSecret = v
	}
	for name, target := range map[string]*bool{
		"WEBTMUX_TLS_ENABLED":  &cfg.TLSEnabled,
		"WEBTMUX_NO_TLS":       &cfg.NoTLS,
		"WEBTMUX_TOTP_ENABLED": &cfg.TOTPEnabled,
	} {
		if v, ok := os.LookupEnv(name); ok {
			parsed, err := strconv.ParseBool(v)
			if err != nil {
				return fmt.Errorf("%s must be a boolean", name)
			}
			*target = parsed
		}
	}
	for name, target := range map[string]*int{
		"WEBTMUX_MAX_LOGIN_ATTEMPTS": &cfg.MaxLoginAttempts,
		"WEBTMUX_LOGIN_WINDOW_SEC":   &cfg.LoginWindowSec,
		"WEBTMUX_LOGIN_LOCKOUT_SEC":  &cfg.LoginLockoutSec,
	} {
		if v, ok := os.LookupEnv(name); ok {
			parsed, err := strconv.Atoi(v)
			if err != nil {
				return fmt.Errorf("%s must be an integer", name)
			}
			*target = parsed
		}
	}
	if v := os.Getenv("WEBTMUX_WS_TIMEOUT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.WSTimeoutSec = n
		}
	}
	if v := os.Getenv("WEBTMUX_SESSION_TIMEOUT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.SessionTimeoutSec = n
		}
	}
	if v := os.Getenv("WEBTMUX_WEBAUTHN_RPID"); v != "" {
		cfg.WebAuthnRPID = v
	}
	if v := os.Getenv("WEBTMUX_WEBAUTHN_ORIGIN"); v != "" {
		cfg.WebAuthnOrigin = v
	}
	if v := os.Getenv("WEBTMUX_WEBAUTHN_DIR"); v != "" {
		cfg.WebAuthnDir = v
	}
	if v := os.Getenv("WEBTMUX_FILE_ROOTS"); v != "" {
		cfg.FileRoots = strings.Split(v, ",")
	}
	if v := os.Getenv("WEBTMUX_SENTRY_DSN"); v != "" {
		cfg.SentryDSN = v
	}
	if v := os.Getenv("WEBTMUX_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("WEBTMUX_LOG_DIR"); v != "" {
		cfg.LogDir = v
	}
	return nil
}
