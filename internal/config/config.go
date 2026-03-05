package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config はアプリケーション設定を保持する。
type Config struct {
	URL          string
	Token        string
	DefaultSpace string
	LogPath      string
	// Timeout は CONFLUENCE_CLI_TIMEOUT の値。未指定時は 0（未設定を意味する）。
	// タイムアウトの優先順位解決（--timeout フラグ > env > デフォルト 30s）は cmd/root.go の責務。
	Timeout time.Duration
	// Insecure は http:// の使用を許可するかどうか。
	Insecure bool
}

// Load は環境変数から Config を読み込む。
func Load() (*Config, error) {
	cfg := &Config{
		URL:          os.Getenv("CONFLUENCE_URL"),
		Token:        os.Getenv("CONFLUENCE_TOKEN"),
		DefaultSpace: os.Getenv("CONFLUENCE_DEFAULT_SPACE"),
		LogPath:      os.Getenv("CONFLUENCE_CLI_LOG"),
	}

	// Token が空の場合、TokenFile からの読み取りを試みる
	if cfg.Token == "" {
		if tokenFile := os.Getenv("CONFLUENCE_TOKEN_FILE"); tokenFile != "" {
			data, err := os.ReadFile(tokenFile)
			if err != nil {
				return nil, fmt.Errorf("read CONFLUENCE_TOKEN_FILE: %w", err)
			}
			cfg.Token = strings.TrimSpace(string(data))
		}
	}

	if os.Getenv("CONFLUENCE_ALLOW_INSECURE") == "true" {
		cfg.Insecure = true
	}

	if raw := os.Getenv("CONFLUENCE_CLI_TIMEOUT"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return nil, fmt.Errorf("CONFLUENCE_CLI_TIMEOUT: %w", err)
		}
		cfg.Timeout = d
	}

	return cfg, nil
}

// Validate は設定内容を検証する。
func (cfg *Config) Validate() error {
	if cfg.URL == "" {
		return fmt.Errorf("CONFLUENCE_URL is not set")
	}
	if !cfg.Insecure && !strings.HasPrefix(cfg.URL, "https://") {
		return fmt.Errorf("insecure URL: CONFLUENCE_URL must start with https:// (or use --allow-insecure)")
	}
	if cfg.Token == "" {
		return fmt.Errorf("CONFLUENCE_TOKEN or CONFLUENCE_TOKEN_FILE is not set")
	}
	return nil
}
