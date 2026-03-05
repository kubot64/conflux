package cmd

import (
	"fmt"

	"github.com/kubot64/conflux/internal/config"
	"github.com/spf13/cobra"
)

var spaceCmd = &cobra.Command{
	Use:   "space",
	Short: "スペース操作",
}

var spaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "スペース一覧を表示する",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		c := newClient(cfg)
		spaces, err := c.ListSpaces(cmd.Context())
		if err != nil {
			return err
		}

		type spaceResult struct {
			Key  string `json:"key"`
			Name string `json:"name"`
			URL  string `json:"url"`
		}
		result := make([]spaceResult, len(spaces))
		for i, s := range spaces {
			result[i] = spaceResult{Key: s.Key, Name: s.Name, URL: s.URL}
		}

		w := newWriter()
		if jsonFlag {
			return w.Write("space list", result)
		}
		for _, s := range spaces {
			fmt.Printf("%-20s %-40s %s\n", s.Key, s.Name, s.URL)
		}
		return nil
	},
}

func init() {
	spaceCmd.AddCommand(spaceListCmd)
	rootCmd.AddCommand(spaceCmd)
}
