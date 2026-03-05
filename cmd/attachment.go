package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/kubot64/conflux/internal/apperror"
	"github.com/kubot64/conflux/internal/config"
	"github.com/kubot64/conflux/internal/history"
	"github.com/kubot64/conflux/internal/port"
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

var attachmentUploadCmd = &cobra.Command{
	Use:   "upload <page-ID> <file>",
	Short: "指定ページにファイルを添付する",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		pageID, filename := args[0], args[1]
		if err := validator.PageID(pageID); err != nil {
			return apperror.New(apperror.KindValidation, err.Error())
		}

		f, err := os.Open(filename)
		if err != nil {
			return apperror.New(apperror.KindValidation, fmt.Sprintf("open file: %v", err))
		}
		defer f.Close()

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		c := newClient(cfg)
		uploaded, err := c.UploadAttachment(cmd.Context(), pageID, filepath.Base(filename), f)
		if err != nil {
			return err
		}

		// 履歴の記録
		logger, _ := history.NewLogger(cliHomeDir())
		if logger != nil {
			_ = logger.Log(port.HistoryEntry{
				Timestamp: time.Now(),
				SessionID: os.Getenv("CONFLUENCE_CLI_SESSION_ID"),
				Action:    "uploaded",
				PageID:    pageID,
				Title:     uploaded.Filename,
			})
		}

		type uploadResult struct {
			ID       string `json:"id"`
			Filename string `json:"filename"`
			Size     int64  `json:"size"`
			URL      string `json:"url"`
		}
		r := uploadResult{
			ID:       uploaded.ID,
			Filename: uploaded.Filename,
			Size:     uploaded.Size,
			URL:      uploaded.URL,
		}

		w := newWriter()
		if jsonFlag {
			return w.Write("attachment upload", r)
		}
		fmt.Printf("Uploaded: %s (%s) %d bytes\n", uploaded.Filename, uploaded.ID, uploaded.Size)
		return nil
	},
}

var attachmentDownloadOutputFlag string

var attachmentDownloadCmd = &cobra.Command{
	Use:   "download <attachment-ID>",
	Short: "添付ファイルをダウンロードする",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		attachmentID := args[0]

		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if err := cfg.Validate(); err != nil {
			return err
		}

		c := newClient(cfg)

		// --output 未指定時は API でファイル名を取得
		destFilename := attachmentDownloadOutputFlag
		if destFilename == "" {
			meta, err := c.GetAttachment(cmd.Context(), attachmentID)
			if err != nil {
				return err
			}
			destFilename = meta.Filename
			if destFilename == "" {
				destFilename = attachmentID
			}
		}

		// path traversal 対策: 相対パスが上位ディレクトリへ抜けることを禁止
		if destFilename != "-" && !filepath.IsAbs(destFilename) {
			cleaned := filepath.Clean(destFilename)
			if len(cleaned) >= 2 && cleaned[:2] == ".." {
				return apperror.New(apperror.KindValidation, "output path must not escape the current directory; use an absolute path")
			}
			destFilename = cleaned
		}

		rc, err := c.DownloadAttachment(cmd.Context(), attachmentID)
		if err != nil {
			return err
		}
		defer rc.Close()

		var out io.Writer
		if destFilename == "-" {
			out = os.Stdout
		} else {
			f, err := os.Create(destFilename)
			if err != nil {
				return apperror.New(apperror.KindServer, fmt.Sprintf("create file: %v", err))
			}
			defer f.Close()
			out = f
		}

		if _, err := io.Copy(out, rc); err != nil {
			return apperror.New(apperror.KindServer, fmt.Sprintf("download failed: %v", err))
		}

		if destFilename != "-" {
			fmt.Printf("Downloaded to %s\n", destFilename)
		}
		return nil
	},
}

func init() {
	attachmentCmd.AddCommand(attachmentListCmd)
	attachmentCmd.AddCommand(attachmentUploadCmd)
	attachmentDownloadCmd.Flags().StringVarP(&attachmentDownloadOutputFlag, "output", "o", "", "出力先ファイルパス（'-' で標準出力）")
	attachmentCmd.AddCommand(attachmentDownloadCmd)
	rootCmd.AddCommand(attachmentCmd)
}
