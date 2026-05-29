// Package config loads and validates the application's configuration from a
// TOML file (hand-editable by a non-technical office admin) with optional
// environment-variable overrides.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/BurntSushi/toml"
)

// Config holds all runtime settings. See config.example.toml for documentation.
type Config struct {
	ListenAddr    string `toml:"listen_addr"`
	DBPath        string `toml:"db_path"`
	CaseFilesRoot string `toml:"case_files_root"`
	SessionHours  int    `toml:"session_hours"`
	LogPath       string `toml:"log_path"`

	// Optional TLS. When both are set, the server runs HTTPS instead of HTTP.
	TLSCert string `toml:"tls_cert"`
	TLSKey  string `toml:"tls_key"`
}

// Default values applied before a file or env overrides are read.
func defaults() Config {
	return Config{
		ListenAddr:   "0.0.0.0:8080",
		DBPath:       "crm.db",
		SessionHours: 12,
		LogPath:      "crm.log",
	}
}

// Load reads the config from path (if it exists), applies env overrides, then
// validates. A missing file is not an error — defaults plus env are used.
func Load(path string) (Config, error) {
	cfg := defaults()

	if path != "" {
		if _, err := os.Stat(path); err == nil {
			if _, err := toml.DecodeFile(path, &cfg); err != nil {
				return cfg, fmt.Errorf("reading config %q: %w", path, err)
			}
		}
	}

	applyEnv(&cfg)

	if err := cfg.validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// applyEnv lets CRM_* environment variables override file values. Handy for
// quick local dev and for setups that prefer env to a file.
func applyEnv(cfg *Config) {
	if v := os.Getenv("CRM_LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("CRM_DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("CRM_CASE_FILES_ROOT"); v != "" {
		cfg.CaseFilesRoot = v
	}
	if v := os.Getenv("CRM_SESSION_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.SessionHours = n
		}
	}
	if v := os.Getenv("CRM_LOG_PATH"); v != "" {
		cfg.LogPath = v
	}
	if v := os.Getenv("CRM_TLS_CERT"); v != "" {
		cfg.TLSCert = v
	}
	if v := os.Getenv("CRM_TLS_KEY"); v != "" {
		cfg.TLSKey = v
	}
}

func (c *Config) validate() error {
	if c.ListenAddr == "" {
		return fmt.Errorf("listen_addr must not be empty")
	}
	if c.DBPath == "" {
		return fmt.Errorf("db_path must not be empty")
	}
	if c.SessionHours <= 0 {
		c.SessionHours = 12
	}
	// TLS is all-or-nothing.
	if (c.TLSCert == "") != (c.TLSKey == "") {
		return fmt.Errorf("tls_cert and tls_key must both be set, or both empty")
	}
	// case_files_root is optional (file browsing is simply disabled when unset),
	// but if set it must exist and be a directory so we fail loudly at startup.
	if c.CaseFilesRoot != "" {
		abs, err := filepath.Abs(c.CaseFilesRoot)
		if err != nil {
			return fmt.Errorf("resolving case_files_root: %w", err)
		}
		c.CaseFilesRoot = filepath.Clean(abs)
		info, err := os.Stat(c.CaseFilesRoot)
		if err != nil {
			return fmt.Errorf("case_files_root %q is not accessible: %w", c.CaseFilesRoot, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("case_files_root %q is not a directory", c.CaseFilesRoot)
		}
	}
	return nil
}

// TLSEnabled reports whether HTTPS should be served.
func (c *Config) TLSEnabled() bool {
	return c.TLSCert != "" && c.TLSKey != ""
}

// SessionTTL returns the configured session lifetime as a duration.
func (c *Config) SessionTTL() time.Duration {
	return time.Duration(c.SessionHours) * time.Hour
}

// FileBrowsingEnabled reports whether a case-files root is configured.
func (c *Config) FileBrowsingEnabled() bool {
	return c.CaseFilesRoot != ""
}
