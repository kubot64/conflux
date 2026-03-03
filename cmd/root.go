package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/kubot64/conflux/internal/apperror"
	"github.com/kubot64/conflux/internal/config"
	"github.com/kubot64/conflux/internal/output"
	"github.com/spf13/cobra"
)

var (
	jsonFlag    bool
	timeoutFlag string
)

var rootCmd = &cobra.Command{
	Use:          "conflux",
	Short:        "Confluence CLI for AI agents",
	SilenceUsage: true,
	// タイムアウトと json フラグを全サブコマンドに伝播させる
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		// タイムアウト優先順位: --timeout > CONFLUENCE_CLI_TIMEOUT > 30s
		timeout := 30 * time.Second
		if cfg.Timeout > 0 {
			timeout = cfg.Timeout
		}
		if timeoutFlag != "" {
			d, err := time.ParseDuration(timeoutFlag)
			if err != nil {
				return apperror.New(apperror.KindValidation, fmt.Sprintf("--timeout: %v", err))
			}
			timeout = d
		}

		ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
		_ = cancel // cobra がコマンド終了時にクリーンアップする
		cmd.SetContext(ctx)
		return nil
	},
}

// Execute はルートコマンドを実行する。
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&jsonFlag, "json", false, "JSON 形式で出力する")
	rootCmd.PersistentFlags().StringVar(&timeoutFlag, "timeout", "", "コマンドタイムアウト（例: 30s, 2m）")
}

// newWriter は --json フラグに基づいて output.Writer を生成する。
func newWriter() *output.Writer {
	return output.New(jsonFlag)
}

// exitWithError は AppError に対応する終了コードで exit する。
func exitWithError(w *output.Writer, command string, err error) {
	w.WriteError(command, err)
	var appErr *apperror.AppError
	if errors.As(err, &appErr) {
		os.Exit(int(appErr.Code()))
	}
	os.Exit(1)
}
