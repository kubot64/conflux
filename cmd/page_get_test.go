package cmd_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildBinary はテスト用バイナリをビルドして一時ディレクトリのパスを返す。
func buildBinary(t *testing.T) string {
	t.Helper()
	binPath := filepath.Join(t.TempDir(), "conflux")
	cmd := exec.Command("go", "build", "-o", binPath, ".")
	// cmd/ ではなくモジュールルートで build
	cmd.Dir = filepath.Join("..", ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return binPath
}

// withEnv は base の環境変数を overrides で置き換える。
func withEnv(base []string, overrides map[string]string) []string {
	skip := make(map[string]bool, len(overrides))
	for k := range overrides {
		skip[k] = true
	}
	out := make([]string, 0, len(base)+len(overrides))
	for _, e := range base {
		k, _, ok := strings.Cut(e, "=")
		if ok && skip[k] {
			continue
		}
		out = append(out, e)
	}
	for k, v := range overrides {
		out = append(out, k+"="+v)
	}
	return out
}

// testEnv は httptest サーバー URL とトークンを含むテスト用環境変数を返す。
// CONFLUENCE_ALLOW_INSECURE=true を含み、http:// URL を許可する。
func testEnv(srvURL string) []string {
	return withEnv(os.Environ(), map[string]string{
		"CONFLUENCE_URL":            srvURL,
		"CONFLUENCE_TOKEN":          "test-token",
		"CONFLUENCE_ALLOW_INSECURE": "true",
		"CONFLUENCE_CLI_HOME":       filepath.Join(os.TempDir(), "conflux-cli-test-home"),
	})
}

// pageAPIHandler は page get テスト用のモック API ハンドラを返す。
// validIDs に含まれる ID は 200、それ以外は 404 を返す。
func pageAPIHandler(t *testing.T, validIDs map[string]bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// serverInfo (ping 用)
		if r.URL.Path == "/rest/api/serverInfo" {
			json.NewEncoder(w).Encode(map[string]any{"version": "7.9.18"})
			return
		}

		// /rest/api/content/<id>?expand=...
		if strings.HasPrefix(r.URL.Path, "/rest/api/content/") {
			id := strings.TrimPrefix(r.URL.Path, "/rest/api/content/")
			if idx := strings.Index(id, "?"); idx != -1 {
				id = id[:idx]
			}
			if validIDs[id] {
				json.NewEncoder(w).Encode(map[string]any{
					"id":      id,
					"title":   "Page " + id,
					"space":   map[string]any{"key": "TEAM"},
					"version": map[string]any{"number": 1},
					"body": map[string]any{
						"storage": map[string]any{"value": "<p>content of " + id + "</p>"},
					},
					"history": map[string]any{
						"lastUpdated": map[string]any{"when": "2024-01-01T00:00:00.000Z"},
					},
					"_links": map[string]any{"base": "", "webui": "/pages/" + id},
				})
			} else {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]any{"message": "Not Found"})
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
}

// TestPageGet_PartialFailure は複数 ID 指定時に一部 404 でも exit 0 になり、
// JSON 出力の errors[] に失敗分が入ることを検証する（回帰テスト）。
func TestPageGet_PartialFailure(t *testing.T) {
	validIDs := map[string]bool{"123": true}
	srv := httptest.NewServer(pageAPIHandler(t, validIDs))
	defer srv.Close()

	bin := buildBinary(t)

	tests := []struct {
		name        string
		ids         []string
		wantResults int
		wantErrors  int
	}{
		{
			name:        "one success one not-found",
			ids:         []string{"123", "999"},
			wantResults: 1,
			wantErrors:  1,
		},
		{
			name:        "all success",
			ids:         []string{"123"},
			wantResults: 1,
			wantErrors:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"page", "get", "--json", "--format", "storage"}, tt.ids...)
			cmd := exec.Command(bin, args...)
			cmd.Env = testEnv(srv.URL)

			out, err := cmd.Output()
			// 一部でも成功すれば exit 0。失敗分は errors[] に入る。
			if err != nil {
				t.Fatalf("expected exit 0, got error: %v\nstdout: %s", err, out)
			}

			var resp map[string]any
			if err := json.Unmarshal(out, &resp); err != nil {
				t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
			}

			// result は常に配列（nil でなく空配列）
			result, ok := resp["result"]
			if !ok {
				t.Fatal("result field missing from response")
			}
			if tt.wantResults == 0 {
				if result != nil {
					arr, ok := result.([]any)
					if ok && len(arr) != 0 {
						t.Errorf("expected empty result array, got: %v", result)
					}
				}
			} else {
				arr, ok := result.([]any)
				if !ok {
					t.Fatalf("result is not array: %T", result)
				}
				if len(arr) != tt.wantResults {
					t.Errorf("result length: got %d, want %d", len(arr), tt.wantResults)
				}
			}

			// errors フィールドの確認
			if tt.wantErrors > 0 {
				errField, ok := resp["errors"]
				if !ok {
					t.Fatal("errors field missing from response")
				}
				arr, ok := errField.([]any)
				if !ok {
					t.Fatalf("errors is not array: %T", errField)
				}
				if len(arr) != tt.wantErrors {
					t.Errorf("errors length: got %d, want %d", len(arr), tt.wantErrors)
				}
				// 各エラーに id フィールドがあること
				for i, e := range arr {
					errMap, ok := e.(map[string]any)
					if !ok {
						t.Errorf("error[%d] is not object", i)
						continue
					}
					if _, ok := errMap["id"]; !ok {
						t.Errorf("error[%d] missing id field", i)
					}
					if _, ok := errMap["error"]; !ok {
						t.Errorf("error[%d] missing error field", i)
					}
				}
			} else {
				// エラーなし時は errors フィールド自体が存在しないこと
				if _, ok := resp["errors"]; ok {
					t.Error("errors field should not be present when there are no errors")
				}
			}
		})
	}
}

func TestPageGet_AllFailExitCode(t *testing.T) {
	srv := httptest.NewServer(pageAPIHandler(t, map[string]bool{}))
	defer srv.Close()
	bin := buildBinary(t)

	cmd := exec.Command(bin, "page", "get", "--json", "999", "888")
	cmd.Env = testEnv(srv.URL)
	_, err := cmd.Output()
	if err == nil {
		t.Fatal("expected non-zero exit when every page fails")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatal(err)
	}
	if exitErr.ExitCode() != 4 {
		t.Fatalf("exit %d, stderr %s", exitErr.ExitCode(), exitErr.Stderr)
	}
	var resp struct {
		Error struct {
			Kind string `json:"kind"`
			Code int    `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(exitErr.Stderr, &resp); err != nil {
		t.Fatalf("stderr: %v\n%s", err, exitErr.Stderr)
	}
	if resp.Error.Kind != "not_found" || resp.Error.Code != 4 {
		t.Fatalf("error envelope: %+v", resp.Error)
	}
}

func TestPageGet_TextErrorsGoToStderr(t *testing.T) {
	srv := httptest.NewServer(pageAPIHandler(t, map[string]bool{"123": true}))
	defer srv.Close()
	bin := buildBinary(t)

	cmd := exec.Command(bin, "page", "get", "123", "999")
	cmd.Env = testEnv(srv.URL)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("partial success should exit 0: %v\nstderr: %s", err, stderr.String())
	}
	if strings.Contains(string(out), "ERROR") {
		t.Fatalf("errors leaked to stdout: %s", out)
	}
	if !strings.Contains(string(out), "Page 123") {
		t.Fatalf("stdout missing page: %s", out)
	}
	if !strings.Contains(stderr.String(), "ERROR [999]") {
		t.Fatalf("stderr: %s", stderr.String())
	}
}
