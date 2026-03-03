package config

import (
	"fmt"
	"os"
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
}

// Load は環境変数から Config を読み込む。
func Load() (*Config, error) {
	cfg := &Config{
		URL:          os.Getenv("CONFLUENCE_URL"),
		Token:        os.Getenv("CONFLUENCE_TOKEN"),
		DefaultSpace: os.Getenv("CONFLUENCE_DEFAULT_SPACE"),
		LogPath:      os.Getenv("CONFLUENCE_CLI_LOG"),
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
