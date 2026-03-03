package main

import (
	"errors"
	"io"
	"log/slog"
	"os"

	"github.com/kubot64/conflux/cmd"
	"github.com/kubot64/conflux/internal/apperror"
)

func main() {
	setupLogger()
	os.Exit(run())
}

func setupLogger() {
	logPath := os.Getenv("CONFLUENCE_CLI_LOG")
	if logPath == "" {
		// ログ無効: discard ハンドラを設定
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
		return
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		// ログファイルを開けない場合は discard にフォールバック
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
		return
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(f, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))
}

func run() int {
	if err := cmd.Execute(); err != nil {
		var appErr *apperror.AppError
		if errors.As(err, &appErr) {
			return int(appErr.Code())
		}
		return 1
	}
	return 0
}
