package cmd_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// pageCreateAPIHandler はページ作成テスト用のモック API ハンドラ。
// searchCount: タイトル検索で返すページ数。
func pageCreateAPIHandler(t *testing.T, searchCount int) http.Handler {
	t.Helper()
	const createdID = "99001"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/rest/api/serverInfo":
			json.NewEncoder(w).Encode(map[string]any{"version": "7.9.18"})

		case r.URL.Path == "/rest/api/content/search":
			// FindPagesByTitle 用検索
			results := make([]any, searchCount)
			for i := range results {
				id := fmt.Sprintf("1000%d", i+1)
				results[i] = map[string]any{
					"id":    id,
					"title": "Test Page",
					"space": map[string]any{"key": "TEAM"},
					"history": map[string]any{
						"lastUpdated": map[string]any{"when": "2024-01-01T00:00:00.000Z"},
					},
					"_links": map[string]any{"base": "", "webui": "/pages/" + id},
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"results": results})

		case r.URL.Path == "/rest/api/content" && r.Method == http.MethodPost:
			// 作成
			json.NewEncoder(w).Encode(map[string]any{
				"id":      createdID,
				"title":   "Test Page",
				"space":   map[string]any{"key": "TEAM"},
				"version": map[string]any{"number": 1},
				"body":    map[string]any{"storage": map[string]any{"value": "<p>Content.</p>"}},
				"history": map[string]any{"lastUpdated": map[string]any{"when": "2024-01-02T00:00:00.000Z"}},
				"_links":  map[string]any{"base": "", "webui": "/pages/" + createdID},
			})

		case strings.HasPrefix(r.URL.Path, "/rest/api/content/") && r.Method == http.MethodGet:
			// page get（更新前のバージョン取得）
			id := strings.TrimPrefix(r.URL.Path, "/rest/api/content/")
			if idx := strings.Index(id, "?"); idx != -1 {
				id = id[:idx]
			}
			json.NewEncoder(w).Encode(map[string]any{
				"id":      id,
				"title":   "Test Page",
				"space":   map[string]any{"key": "TEAM"},
				"version": map[string]any{"number": 1},
				"body":    map[string]any{"storage": map[string]any{"value": "<p>old</p>"}},
				"history": map[string]any{"lastUpdated": map[string]any{"when": "2024-01-01T00:00:00.000Z"}},
				"_links":  map[string]any{"base": "", "webui": "/pages/" + id},
			})

		case strings.HasPrefix(r.URL.Path, "/rest/api/content/") && r.Method == http.MethodPut:
			// 更新
			id := strings.TrimPrefix(r.URL.Path, "/rest/api/content/")
			json.NewEncoder(w).Encode(map[string]any{
				"id":      id,
				"title":   "Test Page",
				"space":   map[string]any{"key": "TEAM"},
				"version": map[string]any{"number": 2},
				"body":    map[string]any{"storage": map[string]any{"value": "<p>Content.</p>"}},
				"history": map[string]any{"lastUpdated": map[string]any{"when": "2024-01-02T00:00:00.000Z"}},
				"_links":  map[string]any{"base": "", "webui": "/pages/" + id},
			})

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func TestPageCreate_IfExists(t *testing.T) {
	bin := buildBinary(t)

	tests := []struct {
		name         string
		searchCount  int    // CQL 検索ヒット件数
		ifExists     string // skip|error|update (省略=デフォルト)
		dryRun       bool
		wantExitCode int
		wantAction   string
	}{
		{
			name:         "0件_create",
			searchCount:  0,
			wantExitCode: 0,
			wantAction:   "created",
		},
		{
			name:         "1件_if-exists_error",
			searchCount:  1,
			ifExists:     "error",
			wantExitCode: 5, // KindConflict → ExitConflict=5
		},
		{
			name:         "1件_if-exists_skip",
			searchCount:  1,
			ifExists:     "skip",
			wantExitCode: 0,
			wantAction:   "skipped",
		},
		{
			name:         "1件_if-exists_update",
			searchCount:  1,
			ifExists:     "update",
			wantExitCode: 0,
			wantAction:   "updated",
		},
		{
			name:         "2件以上_if-exists_update_ambiguous",
			searchCount:  2,
			ifExists:     "update",
			wantExitCode: 5, // KindConflict → ExitConflict=5
		},
		{
			name:         "dry-run_0件_would_create",
			searchCount:  0,
			dryRun:       true,
			wantExitCode: 0,
			wantAction:   "would_create",
		},
		{
			name:         "dry-run_1件_if-exists_update_would_update",
			searchCount:  1,
			ifExists:     "update",
			dryRun:       true,
			wantExitCode: 0,
			wantAction:   "would_update",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(pageCreateAPIHandler(t, tt.searchCount))
			defer srv.Close()

			// 一時 Markdown ファイル作成
			mdFile := filepath.Join(t.TempDir(), "page.md")
			if err := os.WriteFile(mdFile, []byte("# Test Page\n\nContent."), 0644); err != nil {
				t.Fatal(err)
			}

			args := []string{"page", "create", "--json", "--space", "TEAM", mdFile}
			if tt.ifExists != "" {
				args = append(args, "--if-exists", tt.ifExists)
			}
			if tt.dryRun {
				args = append(args, "--dry-run")
			}

			cmd := exec.Command(bin, args...)
			cmd.Env = append(os.Environ(),
				"CONFLUENCE_URL="+srv.URL,
				"CONFLUENCE_TOKEN=test-token",
			)

			out, err := cmd.Output()
			if tt.wantExitCode == 0 {
				if err != nil {
					t.Fatalf("expected exit 0, got: %v\nstdout: %s", err, out)
				}
				var resp map[string]any
				if err := json.Unmarshal(out, &resp); err != nil {
					t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
				}
				result, ok := resp["result"].(map[string]any)
				if !ok {
					t.Fatalf("result is not object: %T\nraw: %s", resp["result"], out)
				}
				if result["action"] != tt.wantAction {
					t.Errorf("action: got %v, want %s\nraw: %s", result["action"], tt.wantAction, out)
				}
			} else {
				if err == nil {
					t.Fatalf("expected exit %d, got 0\nstdout: %s", tt.wantExitCode, out)
				}
				exitErr, ok := err.(*exec.ExitError)
				if !ok {
					t.Fatalf("unexpected error type: %T", err)
				}
				if exitErr.ExitCode() != tt.wantExitCode {
					t.Errorf("exit code: got %d, want %d\nstderr: %s", exitErr.ExitCode(), tt.wantExitCode, exitErr.Stderr)
				}
			}
		})
	}
}
