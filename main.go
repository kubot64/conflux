package main

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"

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
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
		return
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
		return
	}

	// トークンを取得してマスキング対象とする
	token := os.Getenv("CONFLUENCE_TOKEN")

	opts := &slog.HandlerOptions{
		Level: slog.LevelDebug,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// キー名で判定
			key := strings.ToLower(a.Key)
			if key == "token" || key == "authorization" || key == "password" {
				return slog.String(a.Key, "[MASKED]")
			}
			// 値で判定
			if token != "" && a.Value.Kind() == slog.KindString && a.Value.String() == token {
				return slog.String(a.Key, "[MASKED]")
			}
			return a
		},
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(f, opts)))
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
