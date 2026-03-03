package cmd

import (
	"fmt"

	"github.com/kubot64/conflux/internal/apperror"
	"github.com/kubot64/conflux/internal/config"
	"github.com/kubot64/conflux/internal/validator"
	"github.com/spf13/cobra"
)

var attachmentCmd = &cobra.Command{
	Use:   "attachment",
	Short: "添付ファイル操作",
}

var attachmentListCmd = &cobra.Command{
	Use:   "list <page-ID>",
	Short: "指定ページの添付ファイル一覧を表示する",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		pageID := args[0]
		if err := validator.PageID(pageID); err != nil {
			return apperror.New(apperror.KindValidation, err.Error())
		}

		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if err := validateCredentials(cfg); err != nil {
			return err
		}

		c := newClient(cfg)
		attachments, err := c.ListAttachments(cmd.Context(), pageID)
		if err != nil {
			return err
		}

		type attachResult struct {
			ID        string `json:"id"`
			Filename  string `json:"filename"`
			Size      int64  `json:"size"`
			MediaType string `json:"media_type"`
			URL       string `json:"url"`
		}
		result := make([]attachResult, len(attachments))
		for i, a := range attachments {
			result[i] = attachResult{
				ID:        a.ID,
				Filename:  a.Filename,
				Size:      a.Size,
				MediaType: a.MediaType,
				URL:       a.URL,
			}
		}

		w := newWriter()
		if jsonFlag {
			return w.Write("attachment list", result)
		}
		for _, a := range attachments {
			fmt.Printf("%-10s %-40s %10d  %s\n", a.ID, a.Filename, a.Size, a.MediaType)
		}
		return nil
	},
}

func init() {
	attachmentCmd.AddCommand(attachmentListCmd)
	rootCmd.AddCommand(attachmentCmd)
}
