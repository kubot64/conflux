package cmd

// attachment コマンド群（フェーズ2/3で実装）

import "github.com/spf13/cobra"

var attachmentCmd = &cobra.Command{
	Use:   "attachment",
	Short: "添付ファイル操作",
}

func init() {
	rootCmd.AddCommand(attachmentCmd)
}
