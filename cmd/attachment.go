package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kubot64/conflux/internal/apperror"
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

		cfg, err := requireRemoteConfig(cmd)
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

		cfg, err := requireRemoteConfig(cmd)
		if err != nil {
			return err
		}

		c := newClient(cfg)
		uploaded, err := c.UploadAttachment(cmd.Context(), pageID, filepath.Base(filename), f)
		if err != nil {
			return err
		}

		w := newWriter()
		recordHistory(w, "attachment upload", port.HistoryEntry{
			Action: "uploaded",
			PageID: pageID,
			Title:  uploaded.Filename,
		})

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

		cfg, err := requireRemoteConfig(cmd)
		if err != nil {
			return err
		}

		c := newClient(cfg)

		// --output 未指定時は API のファイル名のベース名だけを使う。
		// 絶対パスや ../ をそのまま書くと、作業ディレクトリの外へ出る。
		destFilename := attachmentDownloadOutputFlag
		if destFilename == "" {
			meta, err := c.GetAttachment(cmd.Context(), attachmentID)
			if err != nil {
				return err
			}
			destFilename = filepath.Base(meta.Filename)
			if destFilename == "" || destFilename == "." || destFilename == ".." || destFilename == string(filepath.Separator) {
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

		if destFilename == "-" {
			if _, err := io.Copy(os.Stdout, rc); err != nil {
				return apperror.New(apperror.KindServer, fmt.Sprintf("download failed: %v", err))
			}
			return nil
		}
		// 一時ファイルへ書いてから rename する。途中失敗で宛先を壊さず、
		// 既存のシンボリックリンク先も上書きしない。
		if err := writeDownloadFile(destFilename, rc); err != nil {
			return apperror.New(apperror.KindServer, fmt.Sprintf("download failed: %v", err))
		}

		if destFilename != "-" {
			fmt.Printf("Downloaded to %s\n", destFilename)
		}
		return nil
	},
}

func writeDownloadFile(dest string, r io.Reader) error {
	dir := filepath.Dir(dest)
	if dir == "" {
		dir = "."
	}
	f, err := os.CreateTemp(dir, ".conflux-download-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	done := false
	defer func() {
		if !done {
			f.Close()
			os.Remove(tmp)
		}
	}()
	if _, err := io.Copy(f, r); err != nil {
		return err
	}
	if err := f.Chmod(0o600); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return err
	}
	done = true
	return nil
}

func init() {
	attachmentCmd.AddCommand(attachmentListCmd)
	attachmentCmd.AddCommand(attachmentUploadCmd)
	attachmentDownloadCmd.Flags().StringVarP(&attachmentDownloadOutputFlag, "output", "o", "", "出力先ファイルパス（'-' で標準出力）")
	attachmentCmd.AddCommand(attachmentDownloadCmd)
	rootCmd.AddCommand(attachmentCmd)
}
