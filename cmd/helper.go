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
// デフォルトでタイトルは redaction される（セキュリティの安全側に倒す）。
// 平文で保存したい場合は CONFLUENCE_CLI_REDACT_HISTORY=0 を明示する。
func newHistoryLogger() (*history.Logger, error) {
	var opts []history.Option
	if os.Getenv("CONFLUENCE_CLI_REDACT_HISTORY") == "0" {
		opts = append(opts, history.WithRedactTitle(false))
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
