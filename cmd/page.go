package cmd

import (
	"fmt"

	"github.com/kubot64/conflux/internal/apperror"
	"github.com/kubot64/conflux/internal/config"
	"github.com/kubot64/conflux/internal/converter"
	"github.com/kubot64/conflux/internal/validator"
	"github.com/spf13/cobra"
)

var pageCmd = &cobra.Command{
	Use:   "page",
	Short: "ページ操作",
}

var (
	pageSearchSpaceFlag string
	pageSearchAfterFlag string
)

var pageSearchCmd = &cobra.Command{
	Use:   "search [keyword]",
	Short: "ページを検索する",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if err := validateCredentials(cfg); err != nil {
			return err
		}

		keyword := ""
		if len(args) > 0 {
			keyword = args[0]
		}
		space := pageSearchSpaceFlag
		if space == "" {
			space = cfg.DefaultSpace
		}

		c := newClient(cfg)
		pages, err := c.SearchPages(cmd.Context(), keyword, space, pageSearchAfterFlag)
		if err != nil {
			return err
		}

		type pageResult struct {
			ID           string `json:"id"`
			Title        string `json:"title"`
			Space        string `json:"space"`
			LastModified string `json:"last_modified"`
			URL          string `json:"url"`
		}
		result := make([]pageResult, len(pages))
		for i, p := range pages {
			result[i] = pageResult{
				ID:           p.ID,
				Title:        p.Title,
				Space:        p.Space,
				LastModified: p.LastModified.Format("2006-01-02T15:04:05Z"),
				URL:          p.URL,
			}
		}

		w := newWriter()
		if jsonFlag {
			return w.Write("page search", result)
		}
		for _, p := range pages {
			fmt.Printf("%-10s %-50s %s\n", p.ID, p.Title, p.Space)
		}
		return nil
	},
}

var (
	pageGetFormatFlag  string
	pageGetSectionFlag string
	pageGetMaxChars    int
)

var pageGetCmd = &cobra.Command{
	Use:   "get <page-ID> [page-ID ...]",
	Short: "ページ内容を取得する",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// 引数バリデーション
		for _, id := range args {
			if err := validator.PageID(id); err != nil {
				return apperror.New(apperror.KindValidation, err.Error())
			}
		}
		switch pageGetFormatFlag {
		case "markdown", "html", "storage":
		default:
			return apperror.New(apperror.KindValidation, "--format must be 'markdown', 'html', or 'storage'")
		}

		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if err := validateCredentials(cfg); err != nil {
			return err
		}

		c := newClient(cfg)
		conv := converter.New()

		type pageResult struct {
			ID      string `json:"id"`
			Title   string `json:"title"`
			Space   string `json:"space"`
			Version int    `json:"version"`
			Body    string `json:"body"`
			URL     string `json:"url"`
		}
		type pageError struct {
			ID    string `json:"id"`
			Error string `json:"error"`
		}

		var results []pageResult
		var errors []pageError

		for _, id := range args {
			page, err := c.GetPage(cmd.Context(), id)
			if err != nil {
				errors = append(errors, pageError{ID: id, Error: err.Error()})
				continue
			}

			body, err := formatBody(conv, page.StorageBody, pageGetFormatFlag, pageGetSectionFlag)
			if err != nil {
				errors = append(errors, pageError{ID: id, Error: err.Error()})
				continue
			}

			if pageGetMaxChars > 0 && len([]rune(body)) > pageGetMaxChars {
				runes := []rune(body)
				body = string(runes[:pageGetMaxChars])
			}

			results = append(results, pageResult{
				ID:      page.ID,
				Title:   page.Title,
				Space:   page.Space,
				Version: page.Version,
				Body:    body,
				URL:     page.URL,
			})
		}

		w := newWriter()
		if jsonFlag {
			if len(errors) > 0 {
				return w.WriteWithErrors("page get", results, errors)
			}
			return w.Write("page get", results)
		}

		for _, p := range results {
			fmt.Printf("=== %s (%s) ===\n%s\n\n", p.Title, p.ID, p.Body)
		}
		for _, e := range errors {
			fmt.Printf("ERROR [%s]: %s\n", e.ID, e.Error)
		}
		return nil
	},
}

// formatBody は指定フォーマットでページ本文を変換する。
func formatBody(conv *converter.Converter, storageBody, format, section string) (string, error) {
	switch format {
	case "storage":
		if section != "" {
			return conv.ExtractSection(storageBody, section)
		}
		return storageBody, nil
	case "html":
		if section != "" {
			return conv.ExtractSection(storageBody, section)
		}
		return storageBody, nil
	default: // markdown
		if section != "" {
			extracted, err := conv.ExtractSection(storageBody, section)
			if err != nil {
				return "", err
			}
			return conv.StorageToMarkdown(extracted)
		}
		return conv.StorageToMarkdown(storageBody)
	}
}

var (
	pageTreeSpaceFlag string
	pageTreeDepth     int
)

var pageTreeCmd = &cobra.Command{
	Use:   "tree",
	Short: "スペースのページツリーを取得する",
	RunE: func(cmd *cobra.Command, args []string) error {
		if pageTreeDepth < 1 || pageTreeDepth > 10 {
			return apperror.New(apperror.KindValidation, "--depth must be between 1 and 10")
		}

		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if err := validateCredentials(cfg); err != nil {
			return err
		}

		space := pageTreeSpaceFlag
		if space == "" {
			space = cfg.DefaultSpace
		}
		if space == "" {
			return apperror.New(apperror.KindValidation, "--space or CONFLUENCE_DEFAULT_SPACE is required")
		}

		c := newClient(cfg)
		nodes, err := c.GetPageTree(cmd.Context(), space, pageTreeDepth)
		if err != nil {
			return err
		}

		type nodeResult struct {
			ID       string  `json:"id"`
			Title    string  `json:"title"`
			ParentID *string `json:"parent_id"`
			Depth    int     `json:"depth"`
			URL      string  `json:"url"`
		}
		result := make([]nodeResult, len(nodes))
		for i, n := range nodes {
			result[i] = nodeResult{
				ID:       n.ID,
				Title:    n.Title,
				ParentID: n.ParentID,
				Depth:    n.Depth,
				URL:      n.URL,
			}
		}

		w := newWriter()
		if jsonFlag {
			return w.Write("page tree", result)
		}
		for _, n := range nodes {
			indent := ""
			for j := 0; j < n.Depth; j++ {
				indent += "  "
			}
			fmt.Printf("%s%-10s %s\n", indent, n.ID, n.Title)
		}
		return nil
	},
}

func init() {
	pageSearchCmd.Flags().StringVar(&pageSearchSpaceFlag, "space", "", "スペースキー（省略時: デフォルトスペース or 全スペース）")
	pageSearchCmd.Flags().StringVar(&pageSearchAfterFlag, "after", "", "この日付以降で絞り込む（YYYY-MM-DD）")
	pageCmd.AddCommand(pageSearchCmd)

	pageGetCmd.Flags().StringVar(&pageGetFormatFlag, "format", "markdown", "出力フォーマット (markdown|html|storage)")
	pageGetCmd.Flags().StringVar(&pageGetSectionFlag, "section", "", "抽出するセクション名")
	pageGetCmd.Flags().IntVar(&pageGetMaxChars, "max-chars", 0, "本文の最大文字数（0=制限なし）")
	pageCmd.AddCommand(pageGetCmd)

	pageTreeCmd.Flags().StringVar(&pageTreeSpaceFlag, "space", "", "スペースキー（省略時: CONFLUENCE_DEFAULT_SPACE）")
	pageTreeCmd.Flags().IntVar(&pageTreeDepth, "depth", 3, "取得する深さ（1-10）")
	pageCmd.AddCommand(pageTreeCmd)

	rootCmd.AddCommand(pageCmd)
}
