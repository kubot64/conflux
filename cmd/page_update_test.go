package cmd_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// pageUpdateAPIHandler は page update テスト用のモック API ハンドラ。
func pageUpdateAPIHandler(t *testing.T) http.Handler {
	t.Helper()
	const pageID = "12345"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/rest/api/serverInfo":
			json.NewEncoder(w).Encode(map[string]any{"version": "7.9.18"})

		case strings.HasPrefix(r.URL.Path, "/rest/api/content/") && r.Method == http.MethodGet:
			// page get
			id := strings.TrimPrefix(r.URL.Path, "/rest/api/content/")
			if idx := strings.Index(id, "?"); idx != -1 {
				id = id[:idx]
			}
			if id != pageID {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]any{"message": "Not Found"})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"id":    pageID,
				"title": "Existing Page",
				"space": map[string]any{"key": "TEAM"},
				"version": map[string]any{"number": 3},
				"body": map[string]any{
					"storage": map[string]any{"value": "<p>old content</p>"},
				},
				"history": map[string]any{
					"lastUpdated": map[string]any{"when": "2024-01-01T00:00:00.000Z"},
				},
				"_links": map[string]any{"base": "", "webui": "/pages/" + pageID},
			})

		case strings.HasPrefix(r.URL.Path, "/rest/api/content/") && r.Method == http.MethodPut:
			// page update
			id := strings.TrimPrefix(r.URL.Path, "/rest/api/content/")
			json.NewEncoder(w).Encode(map[string]any{
				"id":    id,
				"title": "Existing Page",
				"space": map[string]any{"key": "TEAM"},
				"version": map[string]any{"number": 4},
				"body": map[string]any{
					"storage": map[string]any{"value": "<p>new content</p>"},
				},
				"history": map[string]any{
					"lastUpdated": map[string]any{"when": "2024-01-02T00:00:00.000Z"},
				},
				"_links": map[string]any{"base": "", "webui": "/pages/" + id},
			})

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func TestPageUpdate(t *testing.T) {
	bin := buildBinary(t)
	const pageID = "12345"

	t.Run("normal_update", func(t *testing.T) {
		srv := httptest.NewServer(pageUpdateAPIHandler(t))
		defer srv.Close()

		mdFile := filepath.Join(t.TempDir(), "page.md")
		if err := os.WriteFile(mdFile, []byte("# Existing Page\n\nnew content"), 0644); err != nil {
			t.Fatal(err)
		}

		cmd := exec.Command(bin, "page", "update", "--json", pageID, mdFile)
		cmd.Env = testEnv(srv.URL)

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
		if result["action"] != "updated" {
			t.Errorf("action: got %v, want 'updated'", result["action"])
		}
		if result["version_after"] != float64(4) {
			t.Errorf("version_after: got %v, want 4", result["version_after"])
		}
	})

	t.Run("dry_run_returns_diff", func(t *testing.T) {
		srv := httptest.NewServer(pageUpdateAPIHandler(t))
		defer srv.Close()

		mdFile := filepath.Join(t.TempDir(), "page.md")
		if err := os.WriteFile(mdFile, []byte("# Existing Page\n\nnew content"), 0644); err != nil {
			t.Fatal(err)
		}

		cmd := exec.Command(bin, "page", "update", "--json", "--dry-run", pageID, mdFile)
		cmd.Env = testEnv(srv.URL)

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
		if result["action"] != "would_update" {
			t.Errorf("action: got %v, want 'would_update'", result["action"])
		}
		// dry-run では diff フィールドが存在するはず
		if _, ok := result["diff"]; !ok {
			t.Errorf("diff field missing from result\nraw: %s", out)
		}
	})

	t.Run("not_found_exits_4", func(t *testing.T) {
		srv := httptest.NewServer(pageUpdateAPIHandler(t))
		defer srv.Close()

		cmd := exec.Command(bin, "page", "update", "--json", "99999")
		cmd.Env = testEnv(srv.URL)

		out, err := cmd.Output()
		if err == nil {
			t.Fatalf("expected exit 4, got 0\nstdout: %s", out)
		}
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("unexpected error type: %T", err)
		}
		if exitErr.ExitCode() != 4 {
			t.Errorf("exit code: got %d, want 4\nstderr: %s", exitErr.ExitCode(), exitErr.Stderr)
		}
	})
}
