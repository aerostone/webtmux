package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := loadWithArgs(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ListenAddr != ":8080" {
		t.Errorf("expected :8080, got %s", cfg.ListenAddr)
	}
	if cfg.TLSEnabled {
		t.Error("TLS should be disabled by default")
	}
	if cfg.BasePath != "" {
		t.Errorf("expected empty base path, got %q", cfg.BasePath)
	}
}

func TestNormalizeBasePath(t *testing.T) {
	for input, want := range map[string]string{
		"": "", "/": "", "/webtmux/": "/webtmux", "/terminal": "/terminal",
	} {
		got, err := NormalizeBasePath(input)
		if err != nil || got != want {
			t.Errorf("NormalizeBasePath(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"webtmux", "/../admin", "/a//b", "/a?b", "/a#b", "/a%2fb"} {
		if _, err := NormalizeBasePath(input); err == nil {
			t.Errorf("NormalizeBasePath(%q) should fail", input)
		}
	}
}

func TestTLSConfigValidation(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{name: "requires cert and key", cfg: Config{TLSEnabled: true}},
		{name: "mTLS requires CA", cfg: Config{TLSEnabled: true, TLSCertFile: "cert", TLSKeyFile: "key", TLSClientAuth: "require_and_verify"}},
		{name: "reject unknown client auth", cfg: Config{TLSClientAuth: "optional"}},
		{name: "reject mTLS without TLS", cfg: Config{TLSClientAuth: "require_and_verify"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.cfg.MaxLoginAttempts = 5
			tc.cfg.LoginWindowSec = 300
			tc.cfg.LoginLockoutSec = 900
			if err := tc.cfg.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestTLSConfigAllowsValidModes(t *testing.T) {
	for _, cfg := range []Config{
		{},
		{TLSClientAuth: "none"},
		{TLSEnabled: true, TLSCertFile: "cert", TLSKeyFile: "key"},
		{TLSEnabled: true, TLSCertFile: "cert", TLSKeyFile: "key", TLSClientCAFile: "ca", TLSClientAuth: "require_and_verify"},
	} {
		cfg.MaxLoginAttempts = 5
		cfg.LoginWindowSec = 300
		cfg.LoginLockoutSec = 900
		if err := cfg.Validate(); err != nil {
			t.Errorf("unexpected validation error for %+v: %v", cfg, err)
		}
	}
}

func TestLoadFromYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := "listen_addr: \":7070\"\nauth_user: \"admin\"\nip_whitelist:\n  - \"10.0.0.0/8\"\n"
	os.WriteFile(cfgPath, []byte(content), 0644)

	cfg, err := loadWithArgs([]string{"-config", cfgPath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ListenAddr != ":7070" {
		t.Errorf("expected :7070, got %s", cfg.ListenAddr)
	}
	if cfg.AuthUser != "admin" {
		t.Errorf("expected 'admin', got %q", cfg.AuthUser)
	}
	if len(cfg.IPWhitelist) != 1 {
		t.Errorf("expected 1 IP, got %d", len(cfg.IPWhitelist))
	}
}

func TestEnvOverridesYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte("listen_addr: \":7070\"\n"), 0644)

	os.Setenv("WEBTMUX_LISTEN", ":5050")
	defer os.Unsetenv("WEBTMUX_LISTEN")

	cfg, err := loadWithArgs([]string{"-config", cfgPath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ListenAddr != ":5050" {
		t.Errorf("env should override YAML: expected :5050, got %s", cfg.ListenAddr)
	}
}

func TestFlagOverridesEnv(t *testing.T) {
	os.Setenv("WEBTMUX_LISTEN", ":5050")
	defer os.Unsetenv("WEBTMUX_LISTEN")

	cfg, err := loadWithArgs([]string{"-listen", ":3030"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ListenAddr != ":3030" {
		t.Errorf("flag should override env: expected :3030, got %s", cfg.ListenAddr)
	}
}

func TestSecurityConfigLayers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("base_path: /yaml\ntls_enabled: true\ntls_cert_file: cert.pem\ntls_key_file: key.pem\nmax_login_attempts: 4\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WEBTMUX_BASE_PATH", "/env/")
	t.Setenv("WEBTMUX_TLS_ENABLED", "true")
	t.Setenv("WEBTMUX_MAX_LOGIN_ATTEMPTS", "6")
	cfg, err := loadWithArgs([]string{"-config", path, "-tls=false", "-max-login-attempts", "8"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BasePath != "/env" || cfg.TLSEnabled || cfg.MaxLoginAttempts != 8 {
		t.Fatalf("unexpected configuration: base_path=%q tls=%v attempts=%d", cfg.BasePath, cfg.TLSEnabled, cfg.MaxLoginAttempts)
	}
}

func TestSecurityEnvValidation(t *testing.T) {
	for _, name := range []string{"WEBTMUX_TLS_ENABLED", "WEBTMUX_NO_TLS", "WEBTMUX_MAX_LOGIN_ATTEMPTS", "WEBTMUX_LOGIN_WINDOW_SEC", "WEBTMUX_LOGIN_LOCKOUT_SEC"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, "invalid")
			if _, err := loadWithArgs(nil); err == nil {
				t.Fatal("expected invalid configuration to return an error")
			}
		})
	}
}

func TestLoadConfigExample(t *testing.T) {
	cfg, err := loadWithArgs([]string{"-config", "../../config.yaml.example"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TLSEnabled || cfg.ListenAddr != "127.0.0.1:3400" {
		t.Fatalf("unexpected example defaults: TLS=%v listen=%s", cfg.TLSEnabled, cfg.ListenAddr)
	}
}

func TestUnknownYAMLFieldIsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("unknown_setting: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadWithArgs([]string{"-config", path}); err == nil {
		t.Fatal("expected unknown configuration field to return an error")
	}
}

func TestYAMLDocumentLoading(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		wantError     bool
	}{
		{name: "empty file keeps defaults"},
		{name: "multiple documents rejected", content: "base_path: /tmux\n---\ntls_enabled: false\n", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tc.content), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadWithArgs([]string{"-config", path}); (err != nil) != tc.wantError {
				t.Fatalf("configuration error = %v, want error %v", err, tc.wantError)
			}
		})
	}
}
