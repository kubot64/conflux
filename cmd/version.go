package cmd

import (
	"github.com/spf13/cobra"
)

// ビルド時に ldflags で埋め込む。
var (
	Version   = "dev"
	Commit    = "unknown"
	BuiltAt   = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "バージョンを表示する",
	RunE: func(cmd *cobra.Command, args []string) error {
		w := newWriter()
		return w.Write("version", map[string]string{
			"version":  Version,
			"commit":   Commit,
			"built_at": BuiltAt,
		})
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
