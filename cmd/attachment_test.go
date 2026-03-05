package cmd_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// attachmentAPIHandler は attachment upload/download テスト用のモック API ハンドラ。
func attachmentAPIHandler(t *testing.T) http.Handler {
	t.Helper()
	const pageID = "12345"
	const attachmentID = "att-001"
	const fileContent = "hello attachment content"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/rest/api/serverInfo":
			json.NewEncoder(w).Encode(map[string]any{"version": "7.9.18"})

		case strings.HasSuffix(r.URL.Path, "/child/attachment") && r.Method == http.MethodPost:
			// upload: multipart form-data
			if err := r.ParseMultipartForm(10 << 20); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"results": []any{
					map[string]any{
						"id":    attachmentID,
						"title": "test.txt",
						"extensions": map[string]any{
							"mediaType": "text/plain",
							"fileSize":  int64(len(fileContent)),
						},
						"_links": map[string]any{
							"base":     "",
							"download": "/download/attachments/" + pageID + "/test.txt",
						},
					},
				},
			})

		case strings.HasPrefix(r.URL.Path, "/download/attachments/") && r.Method == http.MethodGet:
			// download
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte(fileContent))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func TestAttachmentUpload(t *testing.T) {
	bin := buildBinary(t)

	t.Run("upload_success", func(t *testing.T) {
		srv := httptest.NewServer(attachmentAPIHandler(t))
		defer srv.Close()

		// 一時ファイル作成
		tmpFile := filepath.Join(t.TempDir(), "test.txt")
		if err := os.WriteFile(tmpFile, []byte("hello attachment content"), 0644); err != nil {
			t.Fatal(err)
		}

		cmd := exec.Command(bin, "attachment", "upload", "--json", "12345", tmpFile)
		cmd.Env = append(os.Environ(),
			"CONFLUENCE_URL="+srv.URL,
			"CONFLUENCE_TOKEN=test-token",
		)

		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("expected exit 0: %v\nstdout: %s", err, out)
		}

		var resp map[string]any
		if err := json.Unmarshal(out, &resp); err != nil {
			t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
		}
		result, ok := resp["result"].(map[string]any)
		if !ok {
			t.Fatalf("result is not object: %T\nraw: %s", resp["result"], out)
		}
		if result["id"] != "att-001" {
			t.Errorf("id: got %v, want att-001", result["id"])
		}
		if result["filename"] != "test.txt" {
			t.Errorf("filename: got %v, want test.txt", result["filename"])
		}
	})

	t.Run("file_not_found_exit_1", func(t *testing.T) {
		srv := httptest.NewServer(attachmentAPIHandler(t))
		defer srv.Close()

		cmd := exec.Command(bin, "attachment", "upload", "--json", "12345", "/nonexistent/file.txt")
		cmd.Env = append(os.Environ(),
			"CONFLUENCE_URL="+srv.URL,
			"CONFLUENCE_TOKEN=test-token",
		)

		out, err := cmd.Output()
		if err == nil {
			t.Fatalf("expected non-zero exit, got 0\nstdout: %s", out)
		}
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("unexpected error type: %T", err)
		}
		if exitErr.ExitCode() != 1 {
			t.Errorf("exit code: got %d, want 1\nstderr: %s", exitErr.ExitCode(), exitErr.Stderr)
		}
	})
}

func TestAttachmentDownload(t *testing.T) {
	bin := buildBinary(t)
	const fileContent = "hello attachment content"

	t.Run("download_to_stdout", func(t *testing.T) {
		srv := httptest.NewServer(attachmentAPIHandler(t))
		defer srv.Close()

		cmd := exec.Command(bin, "attachment", "download", "--output", "-", "att-001")
		cmd.Env = append(os.Environ(),
			"CONFLUENCE_URL="+srv.URL,
			"CONFLUENCE_TOKEN=test-token",
		)

		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("expected exit 0: %v\nstdout: %s", err, out)
		}
		if string(out) != fileContent {
			t.Errorf("stdout: got %q, want %q", string(out), fileContent)
		}
	})

	t.Run("download_to_file", func(t *testing.T) {
		srv := httptest.NewServer(attachmentAPIHandler(t))
		defer srv.Close()

		outFile := filepath.Join(t.TempDir(), "downloaded.txt")
		cmd := exec.Command(bin, "attachment", "download", "--output", outFile, "att-001")
		cmd.Env = append(os.Environ(),
			"CONFLUENCE_URL="+srv.URL,
			"CONFLUENCE_TOKEN=test-token",
		)

		if out, err := cmd.Output(); err != nil {
			t.Fatalf("expected exit 0: %v\nstdout: %s", err, out)
		}

		data, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("read output file: %v", err)
		}
		if string(data) != fileContent {
			t.Errorf("file content: got %q, want %q", string(data), fileContent)
		}
	})

	t.Run("download_stdin_flag_writes_only_content", func(t *testing.T) {
		// --output - の場合、ファイル内容のみが stdout に出力される（メッセージなし）
		srv := httptest.NewServer(attachmentAPIHandler(t))
		defer srv.Close()

		cmd := exec.Command(bin, "attachment", "download", "-o", "-", "att-001")
		cmd.Env = append(os.Environ(),
			"CONFLUENCE_URL="+srv.URL,
			"CONFLUENCE_TOKEN=test-token",
		)

		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("expected exit 0: %v\nstdout: %s", err, out)
		}

		r := strings.NewReader(string(out))
		all, _ := io.ReadAll(r)
		if string(all) != fileContent {
			t.Errorf("stdout should contain only file content, got %q", string(all))
		}
	})
}
