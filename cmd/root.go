package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kubot64/conflux/internal/apperror"
	"github.com/kubot64/conflux/internal/output"
	"github.com/spf13/cobra"
)

var (
	jsonFlag          bool
	timeoutFlag       string
	allowInsecureFlag bool
)

var rootCmd = &cobra.Command{
	Use:           "conflux",
	Short:         "Confluence CLI for AI agents",
	SilenceUsage:  true,
	SilenceErrors: true,
	// タイムアウトと json フラグを全サブコマンドに伝播させる
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := prepareConfig(cmd)
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
		commandTimeoutCancel = cancel
		cmd.SetContext(withConfig(ctx, cfg))
		return nil
	},
}

// Execute はルートコマンドを実行する。
func Execute() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer func() {
		if commandTimeoutCancel != nil {
			commandTimeoutCancel()
			commandTimeoutCancel = nil
		}
	}()

	executed, err := rootCmd.ExecuteContextC(ctx)
	if err != nil {
		newWriter().WriteError(jsonCommandName(executed), err)
		return err
	}
	return nil
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&jsonFlag, "json", false, "JSON 形式で出力する")
	rootCmd.PersistentFlags().StringVar(&timeoutFlag, "timeout", "", "コマンドタイムアウト（例: 30s, 2m）")
	rootCmd.PersistentFlags().BoolVar(&allowInsecureFlag, "allow-insecure", false, "http:// の使用を許可する")
}

// newWriter は --json フラグに基づいて output.Writer を生成する。
func newWriter() *output.Writer {
	return output.New(jsonFlag)
}
