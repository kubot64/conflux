package cmd

// helper.go にはコマンド横断ヘルパー関数を置く。

import (
	"os"
	"path/filepath"

	"github.com/kubot64/conflux/internal/apperror"
	"github.com/kubot64/conflux/internal/client"
	"github.com/kubot64/conflux/internal/config"
)

// newClient は設定から REST API クライアントを生成する。
func newClient(cfg *config.Config) *client.Client {
	return client.New(cfg.URL, cfg.Token)
}

// validateCredentials は URL と TOKEN が設定されているか確認する。
func validateCredentials(cfg *config.Config) error {
	if cfg.URL == "" {
		return apperror.New(apperror.KindValidation, "CONFLUENCE_URL is not set")
	}
	if cfg.Token == "" {
		return apperror.New(apperror.KindValidation, "CONFLUENCE_TOKEN is not set")
	}
	return nil
}

// cliHomeDir は CLI データディレクトリを返す（$CONFLUENCE_CLI_HOME > ~/.confluence-cli）。
func cliHomeDir() string {
	if home := os.Getenv("CONFLUENCE_CLI_HOME"); home != "" {
		return home
	}
	userHome, _ := os.UserHomeDir()
	return filepath.Join(userHome, ".confluence-cli")
}
