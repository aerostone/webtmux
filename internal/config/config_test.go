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
