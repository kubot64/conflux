package cmd

// helper.go にはコマンド横断ヘルパー関数を置く。

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kubot64/conflux/internal/apperror"
	"github.com/kubot64/conflux/internal/client"
	"github.com/kubot64/conflux/internal/config"
	"github.com/kubot64/conflux/internal/history"
	"github.com/kubot64/conflux/internal/output"
	"github.com/kubot64/conflux/internal/port"
	"github.com/spf13/cobra"
)

type configKey struct{}

var (
	sessionID            = newSessionID()
	commandTimeoutCancel context.CancelFunc
)

func newSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// newClient は設定から REST API クライアントを生成する。
func newClient(cfg *config.Config) *client.Client {
	return client.New(cfg.URL, cfg.Token, cfg.Insecure)
}

func prepareConfig(cmd *cobra.Command) (*config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, apperror.New(apperror.KindValidation, err.Error())
	}
	cfg.Insecure = cfg.Insecure || allowInsecureFlag
	if !skipsRemoteConfig(cmd) {
		if err := cfg.Validate(); err != nil {
			return nil, apperror.New(apperror.KindValidation, err.Error())
		}
	}
	return cfg, nil
}

func skipsRemoteConfig(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		switch c.Name() {
		case "version", "alias", "history":
			return true
		}
	}
	if cmd.Run == nil && cmd.RunE == nil {
		return true
	}
	return false
}

func requireRemoteConfig(cmd *cobra.Command) (*config.Config, error) {
	if cfg, ok := cmd.Context().Value(configKey{}).(*config.Config); ok && cfg != nil {
		if err := cfg.Validate(); err != nil {
			return nil, apperror.New(apperror.KindValidation, err.Error())
		}
		return cfg, nil
	}
	cfg, err := prepareConfig(cmd)
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, apperror.New(apperror.KindValidation, err.Error())
	}
	return cfg, nil
}

func withConfig(ctx context.Context, cfg *config.Config) context.Context {
	return context.WithValue(ctx, configKey{}, cfg)
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

func recordHistory(w *output.Writer, command string, entry port.HistoryEntry) {
	if entry.SessionID == "" {
		entry.SessionID = sessionID
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}
	logger, err := newHistoryLogger()
	if err != nil {
		w.WriteWarning(command, "history_write_failed", "failed to write history: "+err.Error())
		return
	}
	if err := logger.Log(entry); err != nil {
		w.WriteWarning(command, "history_write_failed", "failed to write history: "+err.Error())
	}
}

func formatTimestamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func jsonCommandName(cmd *cobra.Command) string {
	if cmd == nil {
		return rootCmd.Name()
	}
	path := cmd.CommandPath()
	name := path
	if len(path) >= len(rootCmd.Name()) && path[:len(rootCmd.Name())] == rootCmd.Name() {
		name = path[len(rootCmd.Name()):]
	}
	name = trimSpace(name)
	if name == "" {
		return rootCmd.Name()
	}
	return name
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	return s
}

// cliHomeDir は CLI データディレクトリを返す（$CONFLUENCE_CLI_HOME > ~/.confluence-cli）。
func cliHomeDir() string {
	if home := os.Getenv("CONFLUENCE_CLI_HOME"); home != "" {
		return home
	}
	userHome, _ := os.UserHomeDir()
	return filepath.Join(userHome, ".confluence-cli")
}
