package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kubot64/conflux/internal/apperror"
	"github.com/kubot64/conflux/internal/config"
	"github.com/kubot64/conflux/internal/converter"
	"github.com/kubot64/conflux/internal/diff"
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

var (
	pageCreateSpaceFlag    string
	pageCreateTitleFlag    string
	pageCreateDryRun       bool
	pageCreateIfExistsFlag string
)

var pageCreateCmd = &cobra.Command{
	Use:   "create [file]",
	Short: "ページを作成する",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch pageCreateIfExistsFlag {
		case "skip", "error", "update":
		default:
			return apperror.New(apperror.KindValidation, "--if-exists must be 'skip', 'error', or 'update'")
		}

		markdown, err := readMarkdownInput(args)
		if err != nil {
			return err
		}

		title := pageCreateTitleFlag
		if title == "" {
			title = extractTitleFromMarkdown(markdown)
		}
		if title == "" {
			return apperror.New(apperror.KindValidation, "title required: use --title or add '# Heading' to the file")
		}

		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if err := validateCredentials(cfg); err != nil {
			return err
		}

		space := pageCreateSpaceFlag
		if space == "" {
			space = cfg.DefaultSpace
		}
		if space == "" {
			return apperror.New(apperror.KindValidation, "--space or CONFLUENCE_DEFAULT_SPACE is required")
		}
		if err := validator.SpaceKey(space); err != nil {
			return apperror.New(apperror.KindValidation, err.Error())
		}

		conv := converter.New()
		storageBody, err := conv.MarkdownToStorage(markdown)
		if err != nil {
			return apperror.New(apperror.KindValidation, fmt.Sprintf("markdown convert: %v", err))
		}

		c := newClient(cfg)
		w := newWriter()

		existing, err := c.FindPagesByTitle(cmd.Context(), space, title)
		if err != nil {
			return err
		}

		type createResult struct {
			Action       string `json:"action"`
			ID           string `json:"id,omitempty"`
			Title        string `json:"title"`
			Space        string `json:"space"`
			VersionAfter int    `json:"version_after,omitempty"`
			URL          string `json:"url,omitempty"`
		}

		if len(existing) >= 2 {
			return apperror.New(apperror.KindConflict,
				fmt.Sprintf("ambiguous: %d pages with title %q exist in space %q", len(existing), title, space))
		}

		if len(existing) == 0 {
			if pageCreateDryRun {
				return w.Write("page create", createResult{Action: "would_create", Title: title, Space: space})
			}
			page, err := c.CreatePage(cmd.Context(), space, title, storageBody)
			if err != nil {
				return err
			}
			r := createResult{
				Action:       "created",
				ID:           page.ID,
				Title:        page.Title,
				Space:        page.Space,
				VersionAfter: page.Version,
				URL:          page.URL,
			}
			if jsonFlag {
				return w.Write("page create", r)
			}
			fmt.Printf("Created: %s (%s)\n", page.Title, page.ID)
			return nil
		}

		// len(existing) == 1
		found := existing[0]
		switch pageCreateIfExistsFlag {
		case "error":
			return apperror.New(apperror.KindConflict,
				fmt.Sprintf("page with title %q already exists (id=%s)", title, found.ID))
		case "skip":
			if pageCreateDryRun {
				return w.Write("page create", createResult{Action: "would_skip", Title: title, Space: space})
			}
			r := createResult{Action: "skipped", ID: found.ID, Title: title, Space: space, URL: found.URL}
			if jsonFlag {
				return w.Write("page create", r)
			}
			fmt.Printf("Skipped: %s (%s)\n", title, found.ID)
			return nil
		case "update":
			existingPage, err := c.GetPage(cmd.Context(), found.ID)
			if err != nil {
				return err
			}
			if pageCreateDryRun {
				return w.Write("page create", createResult{Action: "would_update", ID: existingPage.ID, Title: title, Space: space})
			}
			updated, err := c.UpdatePage(cmd.Context(), existingPage.ID, existingPage.Version+1, title, storageBody)
			if err != nil {
				return err
			}
			r := createResult{
				Action:       "updated",
				ID:           updated.ID,
				Title:        updated.Title,
				Space:        updated.Space,
				VersionAfter: updated.Version,
				URL:          updated.URL,
			}
			if jsonFlag {
				return w.Write("page create", r)
			}
			fmt.Printf("Updated: %s (%s) v%d\n", updated.Title, updated.ID, updated.Version)
			return nil
		}
		return nil
	},
}

// readMarkdownInput はファイル引数または stdin から Markdown を読み込む。
func readMarkdownInput(args []string) (string, error) {
	if len(args) > 0 {
		data, err := os.ReadFile(args[0])
		if err != nil {
			return "", apperror.New(apperror.KindValidation, fmt.Sprintf("read file: %v", err))
		}
		return string(data), nil
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", apperror.New(apperror.KindValidation, fmt.Sprintf("read stdin: %v", err))
	}
	return string(data), nil
}

// extractTitleFromMarkdown は Markdown の先頭 `# Heading` からタイトルを取得する。
func extractTitleFromMarkdown(markdown string) string {
	for _, line := range strings.Split(markdown, "\n") {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return ""
}

var (
	pageUpdateDryRun bool
)

var pageUpdateCmd = &cobra.Command{
	Use:   "update <ID> [file]",
	Short: "ページを更新する",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		if err := validator.PageID(id); err != nil {
			return apperror.New(apperror.KindValidation, err.Error())
		}

		markdown, err := readMarkdownInput(args[1:])
		if err != nil {
			return err
		}

		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if err := validateCredentials(cfg); err != nil {
			return err
		}

		conv := converter.New()
		storageBody, err := conv.MarkdownToStorage(markdown)
		if err != nil {
			return apperror.New(apperror.KindValidation, fmt.Sprintf("markdown convert: %v", err))
		}

		c := newClient(cfg)
		w := newWriter()

		// 現在のページ情報を取得して、バージョンとタイトルを確認
		existing, err := c.GetPage(cmd.Context(), id)
		if err != nil {
			return err
		}

		type updateResult struct {
			Action       string `json:"action"`
			ID           string `json:"id"`
			Title        string `json:"title"`
			VersionAfter int    `json:"version_after,omitempty"`
			Diff         string `json:"diff,omitempty"`
			URL          string `json:"url,omitempty"`
		}

		if pageUpdateDryRun {
			currentMD, _ := conv.StorageToMarkdown(existing.StorageBody)
			udiff := diff.Unified(currentMD, markdown, "current", "new")
			return w.Write("page update", updateResult{
				Action: "would_update",
				ID:     id,
				Title:  existing.Title,
				Diff:   udiff,
			})
		}

		updated, err := c.UpdatePage(cmd.Context(), id, existing.Version+1, existing.Title, storageBody)
		if err != nil {
			return err
		}

		r := updateResult{
			Action:       "updated",
			ID:           updated.ID,
			Title:        updated.Title,
			VersionAfter: updated.Version,
			URL:          updated.URL,
		}
		if jsonFlag {
			return w.Write("page update", r)
		}
		fmt.Printf("Updated: %s (%s) v%d\n", updated.Title, updated.ID, updated.Version)
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

	pageCreateCmd.Flags().StringVar(&pageCreateSpaceFlag, "space", "", "スペースキー（省略時: CONFLUENCE_DEFAULT_SPACE）")
	pageCreateCmd.Flags().StringVar(&pageCreateTitleFlag, "title", "", "ページタイトル（省略時: 先頭 # 見出し）")
	pageCreateCmd.Flags().BoolVar(&pageCreateDryRun, "dry-run", false, "実際には作成しない")
	pageCreateCmd.Flags().StringVar(&pageCreateIfExistsFlag, "if-exists", "error", "既存ページがある場合の動作 (skip|error|update)")
	pageCmd.AddCommand(pageCreateCmd)

	pageUpdateCmd.Flags().BoolVar(&pageUpdateDryRun, "dry-run", false, "実際には更新せず差分を表示する")
	pageCmd.AddCommand(pageUpdateCmd)

	rootCmd.AddCommand(pageCmd)
}
