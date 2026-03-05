package cmd

// helper.go にはコマンド横断ヘルパー関数を置く。

import (
	"os"
	"path/filepath"

	"github.com/kubot64/conflux/internal/client"
	"github.com/kubot64/conflux/internal/config"
	"github.com/kubot64/conflux/internal/history"
)

// newClient は設定から REST API クライアントを生成する。
func newClient(cfg *config.Config) *client.Client {
	return client.New(cfg.URL, cfg.Token, cfg.Insecure)
}

// newHistoryLogger は CONFLUENCE_CLI_REDACT_HISTORY を反映した Logger を生成する。
func newHistoryLogger() (*history.Logger, error) {
	var opts []history.Option
	if os.Getenv("CONFLUENCE_CLI_REDACT_HISTORY") == "1" {
		opts = append(opts, history.WithRedactTitle(true))
	}
	return history.NewLogger(cliHomeDir(), opts...)
}

// cliHomeDir は CLI データディレクトリを返す（$CONFLUENCE_CLI_HOME > ~/.confluence-cli）。
func cliHomeDir() string {
	if home := os.Getenv("CONFLUENCE_CLI_HOME"); home != "" {
		return home
	}
	userHome, _ := os.UserHomeDir()
	return filepath.Join(userHome, ".confluence-cli")
}
