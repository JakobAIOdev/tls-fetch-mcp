package fetch

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	defaultResponseLimit     int64 = 512 * 1024
	defaultTimeout                 = 30
	defaultMaxTimeout              = 120
	defaultMaxSessions             = 64
	defaultSessionTTL              = 30 * 60
	defaultMaxResponses            = 32
	defaultResponseTTL             = 5 * 60
	defaultResponseReadLimit       = 128 * 1024
)

// Config controls the server-side security and resource limits. Tool callers
// may choose stricter limits, but cannot exceed these values.
type Config struct {
	AllowPrivate     bool
	AllowProxy       bool
	AllowedHosts     []string
	MaxResponseBytes int64
	DefaultTimeout   int
	MaxTimeout       int
	MaxSessions      int
	SessionTTL       int
	MaxResponses     int
	ResponseTTL      int
	MaxReadBytes     int64
}

func ConfigFromEnv() (Config, error) {
	cfg := Config{
		AllowedHosts:     splitCSV(os.Getenv("MCP_TLS_FETCH_ALLOWED_HOSTS")),
		MaxResponseBytes: defaultResponseLimit,
		DefaultTimeout:   defaultTimeout,
		MaxTimeout:       defaultMaxTimeout,
		MaxSessions:      defaultMaxSessions,
		SessionTTL:       defaultSessionTTL,
		MaxResponses:     defaultMaxResponses,
		ResponseTTL:      defaultResponseTTL,
		MaxReadBytes:     defaultResponseReadLimit,
	}

	var err error
	if cfg.AllowPrivate, err = envBoolean("MCP_TLS_FETCH_ALLOW_PRIVATE"); err != nil {
		return Config{}, err
	}
	if cfg.AllowProxy, err = envBoolean("MCP_TLS_FETCH_ALLOW_PROXY"); err != nil {
		return Config{}, err
	}
	if cfg.MaxResponseBytes, err = envPositiveInt64("MCP_TLS_FETCH_MAX_RESPONSE_BYTES", cfg.MaxResponseBytes); err != nil {
		return Config{}, err
	}
	if cfg.DefaultTimeout, err = envPositiveInt("MCP_TLS_FETCH_DEFAULT_TIMEOUT_SECONDS", cfg.DefaultTimeout); err != nil {
		return Config{}, err
	}
	if cfg.MaxTimeout, err = envPositiveInt("MCP_TLS_FETCH_MAX_TIMEOUT_SECONDS", cfg.MaxTimeout); err != nil {
		return Config{}, err
	}
	if cfg.MaxSessions, err = envPositiveInt("MCP_TLS_FETCH_MAX_SESSIONS", cfg.MaxSessions); err != nil {
		return Config{}, err
	}
	if cfg.SessionTTL, err = envPositiveInt("MCP_TLS_FETCH_SESSION_TTL_SECONDS", cfg.SessionTTL); err != nil {
		return Config{}, err
	}
	if cfg.MaxResponses, err = envPositiveInt("MCP_TLS_FETCH_MAX_STORED_RESPONSES", cfg.MaxResponses); err != nil {
		return Config{}, err
	}
	if cfg.ResponseTTL, err = envPositiveInt("MCP_TLS_FETCH_RESPONSE_TTL_SECONDS", cfg.ResponseTTL); err != nil {
		return Config{}, err
	}
	if cfg.MaxReadBytes, err = envPositiveInt64("MCP_TLS_FETCH_MAX_RESPONSE_READ_BYTES", cfg.MaxReadBytes); err != nil {
		return Config{}, err
	}
	if cfg.DefaultTimeout > cfg.MaxTimeout {
		return Config{}, fmt.Errorf("MCP_TLS_FETCH_DEFAULT_TIMEOUT_SECONDS must not exceed MCP_TLS_FETCH_MAX_TIMEOUT_SECONDS")
	}

	return cfg, nil
}

func (cfg Config) withDefaults() Config {
	if cfg.MaxResponseBytes <= 0 {
		cfg.MaxResponseBytes = defaultResponseLimit
	}
	if cfg.DefaultTimeout <= 0 {
		cfg.DefaultTimeout = defaultTimeout
	}
	if cfg.MaxTimeout <= 0 {
		cfg.MaxTimeout = defaultMaxTimeout
	}
	if cfg.MaxSessions <= 0 {
		cfg.MaxSessions = defaultMaxSessions
	}
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = defaultSessionTTL
	}
	if cfg.MaxResponses <= 0 {
		cfg.MaxResponses = defaultMaxResponses
	}
	if cfg.ResponseTTL <= 0 {
		cfg.ResponseTTL = defaultResponseTTL
	}
	if cfg.MaxReadBytes <= 0 {
		cfg.MaxReadBytes = defaultResponseReadLimit
	}
	return cfg
}

func envBoolean(name string) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return false, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", name)
	}
	return value, nil
}

func envPositiveInt(name string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return value, nil
}

func envPositiveInt64(name string, fallback int64) (int64, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return value, nil
}

func splitCSV(value string) []string {
	var values []string
	for _, item := range strings.Split(value, ",") {
		item = strings.ToLower(strings.TrimSpace(item))
		if item != "" {
			values = append(values, item)
		}
	}
	return values
}
