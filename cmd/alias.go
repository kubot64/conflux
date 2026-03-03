package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/kubot64/conflux/internal/alias"
	"github.com/kubot64/conflux/internal/apperror"
	"github.com/kubot64/conflux/internal/port"
	"github.com/spf13/cobra"
)

var aliasCmd = &cobra.Command{
	Use:   "alias",
	Short: "エイリアス管理",
}

var aliasTypeFlag string

var aliasSetCmd = &cobra.Command{
	Use:   "set <name> <target>",
	Short: "エイリアスを設定する",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name, target := args[0], args[1]

		var t port.AliasType
		switch aliasTypeFlag {
		case "page", "":
			t = port.AliasPage
		case "space":
			t = port.AliasSpace
		default:
			return apperror.New(apperror.KindValidation, "--type must be 'page' or 'space'")
		}

		store, err := newAliasStore()
		if err != nil {
			return err
		}
		if err := store.Set(name, target, t); err != nil {
			return err
		}

		w := newWriter()
		if jsonFlag {
			return w.Write("alias set", map[string]any{"name": name, "target": target, "type": string(t)})
		}
		fmt.Printf("alias '%s' -> %s (%s)\n", name, target, t)
		return nil
	},
}

var aliasGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "エイリアスを取得する",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := newAliasStore()
		if err != nil {
			return err
		}
		a, err := store.Get(args[0])
		if err != nil {
			return err
		}

		w := newWriter()
		if jsonFlag {
			return w.Write("alias get", map[string]any{"name": a.Name, "target": a.Target, "type": string(a.Type)})
		}
		fmt.Printf("%s -> %s (%s)\n", a.Name, a.Target, a.Type)
		return nil
	},
}

var aliasListCmd = &cobra.Command{
	Use:   "list",
	Short: "エイリアス一覧を表示する",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := newAliasStore()
		if err != nil {
			return err
		}
		aliases, err := store.List()
		if err != nil {
			return err
		}

		type aliasResult struct {
			Name   string `json:"name"`
			Target string `json:"target"`
			Type   string `json:"type"`
		}
		result := make([]aliasResult, len(aliases))
		for i, a := range aliases {
			result[i] = aliasResult{Name: a.Name, Target: a.Target, Type: string(a.Type)}
		}

		w := newWriter()
		if jsonFlag {
			return w.Write("alias list", result)
		}
		for _, a := range aliases {
			fmt.Printf("%-20s %-30s %s\n", a.Name, a.Target, a.Type)
		}
		return nil
	},
}

var aliasDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "エイリアスを削除する",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := newAliasStore()
		if err != nil {
			return err
		}
		if err := store.Delete(args[0]); err != nil {
			return err
		}

		w := newWriter()
		if jsonFlag {
			return w.Write("alias delete", map[string]any{"deleted": args[0]})
		}
		fmt.Printf("alias '%s' deleted\n", args[0])
		return nil
	},
}

func newAliasStore() (*alias.Store, error) {
	return alias.NewStore(filepath.Join(cliHomeDir(), "alias.json"))
}

func init() {
	aliasSetCmd.Flags().StringVar(&aliasTypeFlag, "type", "page", "エイリアス種別 (page|space)")
	aliasCmd.AddCommand(aliasSetCmd)
	aliasCmd.AddCommand(aliasGetCmd)
	aliasCmd.AddCommand(aliasListCmd)
	aliasCmd.AddCommand(aliasDeleteCmd)
	rootCmd.AddCommand(aliasCmd)
}
