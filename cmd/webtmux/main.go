package main

import (
	"crypto/tls"
	"fmt"
	"os"

	"github.com/aerostone/webtmux/internal/config"
	"github.com/aerostone/webtmux/internal/logger"
	"github.com/aerostone/webtmux/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logLevel := logger.ParseLevel(cfg.LogLevel)
	if cfg.LogLevel == "" {
		logLevel = logger.LevelInfo
	}
	maxAge := cfg.LogMaxAgeDays
	if maxAge <= 0 {
		maxAge = 3
	}
	if err := logger.Init(cfg.LogDir, logLevel, maxAge); err != nil {
		fmt.Fprintf(os.Stderr, "logger init error: %v\n", err)
		os.Exit(1)
	}
	logger.Infof("log level=%s dir=%s max_age=%d days", cfg.LogLevel, logger.LogDir(), maxAge)

	var tlsConfig *tls.Config
	if cfg.TLSEnabled {
		tlsConfig, err = server.TLSConfig(cfg.TLSCertFile, cfg.TLSKeyFile, cfg.TLSClientCAFile, cfg.TLSClientAuth)
		if err != nil {
			logger.Fatalf("TLS configuration error: %v", err)
		}
	}
	srv := server.New(cfg)
	srv.TLSConfig = tlsConfig

	if cfg.TLSEnabled {
		logger.Infof("webtmux listening on %s (TLS) cert=%s", cfg.ListenAddr, cfg.TLSCertFile)
		if err := srv.ListenAndServeTLS("", ""); err != nil {
			logger.Fatalf("server error: %v", err)
		}
	} else {
		logger.Infof("webtmux listening on %s (no TLS)", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil {
			logger.Fatalf("server error: %v", err)
		}
	}
}
