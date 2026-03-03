package config_test

import (
	"testing"
	"time"

	"github.com/kubot64/conflux/internal/config"
)

func TestLoad(t *testing.T) {
	t.Setenv("CONFLUENCE_URL", "https://confluence.example.com")
	t.Setenv("CONFLUENCE_TOKEN", "mytoken")
	t.Setenv("CONFLUENCE_DEFAULT_SPACE", "DEV")
	t.Setenv("CONFLUENCE_CLI_LOG", "/tmp/test.log")
	t.Setenv("CONFLUENCE_CLI_TIMEOUT", "45s")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.URL != "https://confluence.example.com" {
		t.Errorf("URL: got %q, want %q", cfg.URL, "https://confluence.example.com")
	}
	if cfg.Token != "mytoken" {
		t.Errorf("Token mismatch")
	}
	if cfg.DefaultSpace != "DEV" {
		t.Errorf("DefaultSpace mismatch")
	}
	if cfg.LogPath != "/tmp/test.log" {
		t.Errorf("LogPath mismatch")
	}
	if cfg.Timeout != 45*time.Second {
		t.Errorf("Timeout: got %v, want %v", cfg.Timeout, 45*time.Second)
	}
}

func TestLoad_DefaultsWhenEnvUnset(t *testing.T) {
	t.Setenv("CONFLUENCE_URL", "")
	t.Setenv("CONFLUENCE_TOKEN", "")
	t.Setenv("CONFLUENCE_DEFAULT_SPACE", "")
	t.Setenv("CONFLUENCE_CLI_LOG", "")
	t.Setenv("CONFLUENCE_CLI_TIMEOUT", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Timeout != 0 {
		t.Errorf("Timeout should be 0 when unset, got %v", cfg.Timeout)
	}
}

func TestLoad_InvalidTimeout(t *testing.T) {
	t.Setenv("CONFLUENCE_CLI_TIMEOUT", "invalid")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for invalid timeout, got nil")
	}
}
