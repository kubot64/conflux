package cmd

// alias コマンド群（フェーズ2で実装）

import "github.com/spf13/cobra"

var aliasCmd = &cobra.Command{
	Use:   "alias",
	Short: "エイリアス管理",
}

func init() {
	rootCmd.AddCommand(aliasCmd)
}
