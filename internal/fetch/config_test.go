package fetch

import "testing"

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("MCP_TLS_FETCH_ALLOW_PRIVATE", "true")
	t.Setenv("MCP_TLS_FETCH_ALLOW_PROXY", "false")
	t.Setenv("MCP_TLS_FETCH_ALLOWED_HOSTS", "example.com, *.example.org")
	t.Setenv("MCP_TLS_FETCH_MAX_RESPONSE_BYTES", "4096")
	t.Setenv("MCP_TLS_FETCH_DEFAULT_TIMEOUT_SECONDS", "10")
	t.Setenv("MCP_TLS_FETCH_MAX_TIMEOUT_SECONDS", "20")
	t.Setenv("MCP_TLS_FETCH_MAX_SESSIONS", "4")
	t.Setenv("MCP_TLS_FETCH_SESSION_TTL_SECONDS", "600")
	t.Setenv("MCP_TLS_FETCH_MAX_STORED_RESPONSES", "7")
	t.Setenv("MCP_TLS_FETCH_RESPONSE_TTL_SECONDS", "90")
	t.Setenv("MCP_TLS_FETCH_MAX_RESPONSE_READ_BYTES", "2048")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}
	if !cfg.AllowPrivate || cfg.AllowProxy {
		t.Fatalf("boolean config = private:%v proxy:%v", cfg.AllowPrivate, cfg.AllowProxy)
	}
	if cfg.MaxResponseBytes != 4096 || cfg.DefaultTimeout != 10 || cfg.MaxTimeout != 20 || cfg.MaxSessions != 4 {
		t.Fatalf("numeric config = %+v", cfg)
	}
	if cfg.SessionTTL != 600 || cfg.MaxResponses != 7 || cfg.ResponseTTL != 90 || cfg.MaxReadBytes != 2048 {
		t.Fatalf("storage config = %+v", cfg)
	}
	if len(cfg.AllowedHosts) != 2 || cfg.AllowedHosts[1] != "*.example.org" {
		t.Fatalf("AllowedHosts = %v", cfg.AllowedHosts)
	}
}

func TestConfigFromEnvRejectsInvalidValues(t *testing.T) {
	t.Setenv("MCP_TLS_FETCH_ALLOW_PRIVATE", "sometimes")
	if _, err := ConfigFromEnv(); err == nil {
		t.Error("ConfigFromEnv() accepted an invalid boolean")
	}
}
