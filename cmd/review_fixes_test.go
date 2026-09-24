package cmd_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestJSONErrorEnvelope(t *testing.T) {
	bin := buildBinary(t)
	cmd := exec.Command(bin, "--json", "page", "get", "nope")
	cmd.Env = testEnv("https://confluence.example.com")

	_, err := cmd.Output()
	if err == nil {
		t.Fatal("expected non-zero exit")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("unexpected error %T", err)
	}
	if exitErr.ExitCode() != 1 {
		t.Fatalf("exit %d, stderr %s", exitErr.ExitCode(), exitErr.Stderr)
	}
	var resp struct {
		SchemaVersion int    `json:"schema_version"`
		Command       string `json:"command"`
		Error         struct {
			Code float64 `json:"code"`
			Kind string  `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal(exitErr.Stderr, &resp); err != nil {
		t.Fatalf("stderr is not JSON error envelope: %v\n%s", err, exitErr.Stderr)
	}
	if resp.SchemaVersion != 1 || resp.Command != "page get" || resp.Error.Kind != "validation_error" || resp.Error.Code != 1 {
		t.Fatalf("envelope: %+v", resp)
	}
}

func TestVersionWithoutCredentials(t *testing.T) {
	bin := buildBinary(t)
	cmd := exec.Command(bin, "version", "--json")
	cmd.Env = withEnv(os.Environ(), map[string]string{
		"CONFLUENCE_URL":        "",
		"CONFLUENCE_TOKEN":      "",
		"CONFLUENCE_TOKEN_FILE": "",
	})
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		t.Fatalf("version should not need Confluence credentials: %v\nstderr: %s\nstdout: %s", err, stderr, out)
	}
}

func TestAllowInsecureFlagReachesClient(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/serverInfo" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"version": "7.9.18"})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := withEnv(os.Environ(), map[string]string{
		"CONFLUENCE_URL":            srv.URL,
		"CONFLUENCE_TOKEN":          "test-token",
		"CONFLUENCE_ALLOW_INSECURE": "",
	})

	t.Run("without flag", func(t *testing.T) {
		cmd := exec.Command(bin, "--json", "ping")
		cmd.Env = env
		if _, err := cmd.Output(); err == nil {
			t.Fatal("expected TLS verification failure without --allow-insecure")
		}
	})

	t.Run("with flag", func(t *testing.T) {
		cmd := exec.Command(bin, "--allow-insecure", "--json", "ping")
		cmd.Env = env
		out, err := cmd.Output()
		if err != nil {
			stderr := ""
			if ee, ok := err.(*exec.ExitError); ok {
				stderr = string(ee.Stderr)
			}
			t.Fatalf("ping with --allow-insecure: %v\nstderr: %s\nstdout: %s", err, stderr, out)
		}
		var resp struct {
			Result struct {
				OK            bool   `json:"ok"`
				ServerVersion string `json:"server_version"`
			} `json:"result"`
		}
		if err := json.Unmarshal(out, &resp); err != nil {
			t.Fatal(err)
		}
		if !resp.Result.OK || resp.Result.ServerVersion != "7.9.18" {
			t.Fatalf("result: %+v raw %s", resp.Result, out)
		}
	})
}

func TestPageCreate_RecordsHistory(t *testing.T) {
	bin := buildBinary(t)
	srv := httptest.NewServer(pageCreateAPIHandler(t, 0))
	defer srv.Close()

	home := t.TempDir()
	mdFile := filepath.Join(t.TempDir(), "page.md")
	if err := os.WriteFile(mdFile, []byte("# Test Page\n\nContent."), 0o644); err != nil {
		t.Fatal(err)
	}

	env := withEnv(testEnv(srv.URL), map[string]string{
		"CONFLUENCE_CLI_HOME":           home,
		"CONFLUENCE_CLI_REDACT_HISTORY": "0",
	})

	create := exec.Command(bin, "page", "create", "--json", "--space", "TEAM", mdFile)
	create.Env = env
	out, err := create.Output()
	if err != nil {
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		t.Fatalf("create: %v\nstderr: %s\nstdout: %s", err, stderr, out)
	}

	list := exec.Command(bin, "history", "list", "--json")
	list.Env = env
	out, err = list.Output()
	if err != nil {
		t.Fatalf("history list: %v\nstdout: %s", err, out)
	}
	var resp struct {
		Result []struct {
			Action    string `json:"action"`
			PageID    string `json:"page_id"`
			SessionID string `json:"session_id"`
			Title     string `json:"title"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if len(resp.Result) != 1 {
		t.Fatalf("history entries: got %d (%s)", len(resp.Result), out)
	}
	got := resp.Result[0]
	if got.Action != "created" || got.PageID != "99001" || got.Title != "Test Page" || got.SessionID == "" {
		t.Fatalf("entry: %+v", got)
	}
}
