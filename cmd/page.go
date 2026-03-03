package cmd

// page コマンド群（フェーズ2/3で実装）

import "github.com/spf13/cobra"

var pageCmd = &cobra.Command{
	Use:   "page",
	Short: "ページ操作",
}

func init() {
	rootCmd.AddCommand(pageCmd)
}
