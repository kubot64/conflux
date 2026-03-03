package cmd

// space コマンド群（フェーズ2で実装）

import "github.com/spf13/cobra"

var spaceCmd = &cobra.Command{
	Use:   "space",
	Short: "スペース操作",
}

func init() {
	rootCmd.AddCommand(spaceCmd)
}
