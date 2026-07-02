package config

import (
	"flag"
	"os"
	"strconv"
	"strings"

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
	ListenAddr       string   `json:"listen_addr" yaml:"listen_addr"`
	TLSCertFile      string   `json:"tls_cert_file" yaml:"tls_cert_file"`
	TLSKeyFile       string   `json:"tls_key_file" yaml:"tls_key_file"`
	NoTLS            bool     `json:"no_tls" yaml:"no_tls"`
	TmuxSocket       string   `json:"tmux_socket" yaml:"tmux_socket"`
	IPWhitelist      []string `json:"ip_whitelist" yaml:"ip_whitelist"`
	TOTPEnabled      bool     `json:"totp_enabled" yaml:"totp_enabled"`
	TOTPSecret       string   `json:"totp_secret" yaml:"totp_secret"`
	AuthUser         string   `json:"auth_user" yaml:"auth_user"`
	AuthPass         string   `json:"auth_pass" yaml:"auth_pass"`
	SessionSecret    string   `json:"session_secret" yaml:"session_secret"`
	MaxLoginAttempts int      `json:"max_login_attempts" yaml:"max_login_attempts"`
	LoginWindowSec  int      `json:"login_window_sec" yaml:"login_window_sec"`
	LoginLockoutSec int      `json:"login_lockout_sec" yaml:"login_lockout_sec"`
	WSTimeoutSec    int      `json:"ws_timeout_sec" yaml:"ws_timeout_sec"`
	AllowAllOrigins bool     `json:"allow_all_origins" yaml:"allow_all_origins"`
	WebAuthnRPID    string   `json:"webauthn_rpid" yaml:"webauthn_rpid"`
	WebAuthnOrigin  string   `json:"webauthn_origin" yaml:"webauthn_origin"`
	WebAuthnDir     string   `json:"webauthn_dir" yaml:"webauthn_dir"`
	FileRoots       []string `json:"file_roots" yaml:"file_roots"`
}

func Load() (*Config, error) {
	return loadWithArgs(os.Args[1:])
}

func loadWithArgs(args []string) (*Config, error) {
	cfg := &Config{
		ListenAddr:       ":8080",
		MaxLoginAttempts: 5,
		LoginWindowSec:  300,
		LoginLockoutSec: 900,
		WSTimeoutSec:    300,
		WebAuthnRPID:    "localhost",
		WebAuthnOrigin:  "http://localhost:8080",
		WebAuthnDir:     "~/.webtmux",
	}

	fs := flag.NewFlagSet("webtmux", flag.ContinueOnError)
	configFile := fs.String("config", "", "path to YAML config file")
	_ = fs.String("listen", "", "listen address")
	_ = fs.String("tls-cert", "", "TLS certificate file (auto-generated if empty)")
	_ = fs.String("tls-key", "", "TLS private key file (auto-generated if empty)")
	_ = fs.Bool("no-tls", false, "disable TLS, use plain HTTP/WS")
	_ = fs.String("socket", "", "tmux socket path")
	_ = fs.Bool("totp", false, "enable TOTP authentication")
	_ = fs.String("ip-whitelist", "", "comma-separated IP whitelist (CIDR)")
	_ = fs.String("user", "", "basic auth username")
	_ = fs.String("pass", "", "basic auth password")
	_ = fs.Int("ws-timeout", 0, "WebSocket idle timeout seconds (0=server default)")
	_ = fs.String("file-roots", "", "comma-separated additional file manager root directories")

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
	applyEnv(cfg)

	// Layer 3: CLI flags (highest priority, only if explicitly set)
	applyFlag(fs, "listen", func(v string) { cfg.ListenAddr = v })
	applyFlag(fs, "tls-cert", func(v string) { cfg.TLSCertFile = v })
	applyFlag(fs, "tls-key", func(v string) { cfg.TLSKeyFile = v })
	applyFlag(fs, "socket", func(v string) { cfg.TmuxSocket = v })
	applyFlag(fs, "user", func(v string) { cfg.AuthUser = v })
	applyFlag(fs, "pass", func(v string) { cfg.AuthPass = v })
	applyFlag(fs, "ip-whitelist", func(v string) { cfg.IPWhitelist = strings.Split(v, ",") })
	applyFlag(fs, "file-roots", func(v string) { cfg.FileRoots = strings.Split(v, ",") })

	fs.Visit(func(f *flag.Flag) {
		if f.Name == "totp" {
			cfg.TOTPEnabled = true
		}
		if f.Name == "no-tls" {
			cfg.NoTLS = true
		}
		if f.Name == "ws-timeout" {
			cfg.WSTimeoutSec = f.Value.(flag.Getter).Get().(int)
		}
	})

	// Expand ~ in WebAuthnDir
	cfg.WebAuthnDir = expandHome(cfg.WebAuthnDir)

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
	return yaml.Unmarshal(data, cfg)
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("WEBTMUX_LISTEN"); v != "" {
		cfg.ListenAddr = v
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
	if v := os.Getenv("WEBTMUX_WS_TIMEOUT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.WSTimeoutSec = n
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
}
