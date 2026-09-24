package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var pingCmd = &cobra.Command{
	Use:   "ping",
	Short: "Confluence への疎通確認を行う",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := requireRemoteConfig(cmd)
		if err != nil {
			return err
		}

		w := newWriter()
		version, err := newClient(cfg).ServerInfo(cmd.Context())
		if err != nil {
			return err
		}

		result := map[string]any{
			"ok":             true,
			"url":            cfg.URL,
			"server_version": version,
		}
		if jsonFlag {
			return w.Write("ping", result)
		}
		fmt.Printf("ok %s server %s\n", cfg.URL, version)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(pingCmd)
}
