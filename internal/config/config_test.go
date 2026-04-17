package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestLoad_TokenFile_Mode0600_OK(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file modes are not meaningful on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("secret-token\n"), 0600); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	t.Setenv("CONFLUENCE_TOKEN", "")
	t.Setenv("CONFLUENCE_TOKEN_FILE", path)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Token != "secret-token" {
		t.Errorf("Token: got %q, want %q", cfg.Token, "secret-token")
	}
}

func TestLoad_TokenFile_WorldReadable_Rejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file modes are not meaningful on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("secret-token"), 0644); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	t.Setenv("CONFLUENCE_TOKEN", "")
	t.Setenv("CONFLUENCE_TOKEN_FILE", path)

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for world-readable token file, got nil")
	}
	if !strings.Contains(err.Error(), "permission") {
		t.Errorf("error should mention permissions, got: %v", err)
	}
}

func TestLoad_TokenFile_GroupReadable_Rejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file modes are not meaningful on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("secret-token"), 0640); err != nil {
		t.Fatalf("write token file: %v", err)
	}

	t.Setenv("CONFLUENCE_TOKEN", "")
	t.Setenv("CONFLUENCE_TOKEN_FILE", path)

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for group-readable token file, got nil")
	}
}
