package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kubot64/conflux/internal/history"
	"github.com/spf13/cobra"
)

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "更新履歴管理",
}

var (
	historyLimitFlag   int
	historySpaceFlag   string
	historySessionFlag string
)

var historyListCmd = &cobra.Command{
	Use:   "list",
	Short: "更新履歴を表示する",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := historyDir()
		logger, err := history.NewLogger(dir)
		if err != nil {
			return err
		}

		entries, err := logger.List(historySpaceFlag, historySessionFlag, historyLimitFlag)
		if err != nil {
			return err
		}

		w := newWriter()
		type entryJSON struct {
			Timestamp     string `json:"timestamp"`
			SessionID     string `json:"session_id"`
			Action        string `json:"action"`
			PageID        string `json:"page_id"`
			Title         string `json:"title"`
			Space         string `json:"space"`
			VersionBefore int    `json:"version_before,omitempty"`
			VersionAfter  int    `json:"version_after,omitempty"`
		}
		result := make([]entryJSON, len(entries))
		for i, e := range entries {
			result[i] = entryJSON{
				Timestamp:     e.Timestamp.Format("2006-01-02T15:04:05Z"),
				SessionID:     e.SessionID,
				Action:        e.Action,
				PageID:        e.PageID,
				Title:         e.Title,
				Space:         e.Space,
				VersionBefore: e.VersionBefore,
				VersionAfter:  e.VersionAfter,
			}
		}
		return w.Write("history list", result)
	},
}

func historyDir() string {
	if home := os.Getenv("CONFLUENCE_CLI_HOME"); home != "" {
		return home
	}
	userHome, _ := os.UserHomeDir()
	return filepath.Join(userHome, ".confluence-cli")
}

func init() {
	historyListCmd.Flags().IntVar(&historyLimitFlag, "limit", 20, "表示件数")
	historyListCmd.Flags().StringVar(&historySpaceFlag, "space", "", "スペースキーでフィルタ")
	historyListCmd.Flags().StringVar(&historySessionFlag, "session", "", "セッションIDでフィルタ")
	historyCmd.AddCommand(historyListCmd)
	rootCmd.AddCommand(historyCmd)

	// historyDir をコマンドから参照するためのデバッグ用
	_ = fmt.Sprintf
}
