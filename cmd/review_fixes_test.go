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
		cmd := exec.Command(bin, "--insecure-skip-verify", "--json", "ping")
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

	t.Run("allow-insecure does not skip tls", func(t *testing.T) {
		cmd := exec.Command(bin, "--allow-insecure", "--json", "ping")
		cmd.Env = env
		if _, err := cmd.Output(); err == nil {
			t.Fatal("expected TLS verification failure with only --allow-insecure")
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

func TestPingTextAndUserAgent(t *testing.T) {
	var ua string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"version": "7.9.18"})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	cmd := exec.Command(bin, "ping")
	cmd.Env = testEnv(srv.URL)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ping: %v\n%s", err, out)
	}
	got := string(out)
	if strings.Contains(got, "map[") || !strings.Contains(got, "ok "+srv.URL+" server 7.9.18") {
		t.Fatalf("text ping: %q", got)
	}
	if ua != "conflux/dev" {
		t.Fatalf("User-Agent: %q", ua)
	}
}

func TestTimeoutMustBePositive(t *testing.T) {
	bin := buildBinary(t)
	cmd := exec.Command(bin, "--timeout", "0s", "--json", "version")
	_, err := cmd.Output()
	if err == nil {
		t.Fatal("expected validation error")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("exit: %v", err)
	}
}

func TestPageSearch_RejectsBadAfter(t *testing.T) {
	bin := buildBinary(t)
	cmd := exec.Command(bin, "--json", "page", "search", "--after", "yesterday")
	cmd.Env = testEnv("https://confluence.example.com")
	_, err := cmd.Output()
	if err == nil {
		t.Fatal("expected validation error")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("exit: %v stderr %s", err, func() string {
			if exitErr != nil {
				return string(exitErr.Stderr)
			}
			return ""
		}())
	}
}

func TestPageTree_TextIsPreorder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/api/space/TEAM/content/page":
			_, _ = ioWrite(w, `{"results":[
				{"id":"20","title":"Second","_links":{"webui":"/p/20"}},
				{"id":"10","title":"First","_links":{"webui":"/p/10"}}
			]}`)
		case "/rest/api/content/20/child/page":
			_, _ = ioWrite(w, `{"results":[{"id":"15","title":"Child","_links":{"webui":"/p/15"}}]}`)
		case "/rest/api/content/10/child/page", "/rest/api/content/15/child/page":
			_, _ = ioWrite(w, `{"results":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	bin := buildBinary(t)
	cmd := exec.Command(bin, "page", "tree", "--space", "TEAM", "--depth", "2")
	cmd.Env = testEnv(srv.URL)
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		t.Fatalf("tree: %v\nstderr: %s\nstdout: %s", err, stderr, out)
	}
	text := string(out)
	i10 := strings.Index(text, "10")
	i20 := strings.Index(text, "20")
	i15 := strings.Index(text, "15")
	if i10 < 0 || i20 < 0 || i15 < 0 || !(i10 < i20 && i20 < i15) {
		t.Fatalf("expected preorder 10, 20, 15:\n%s", text)
	}
	if !strings.Contains(text, "\n  15") {
		t.Fatalf("child should be indented:\n%s", text)
	}
}

func ioWrite(w http.ResponseWriter, body string) (int, error) {
	return w.Write([]byte(body))
}
