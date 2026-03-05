package client_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kubot64/conflux/internal/client"
)

func newTestClient(t *testing.T, srv *httptest.Server) *client.Client {
	t.Helper()
	return client.New(srv.URL, "test-token", true)
}

// --- リトライ: GET は 429/5xx で再試行 ---

func TestGet_Retry429(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 2 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.ListSpaces(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestGet_Retry5xx(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.ListSpaces(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestGet_MaxRetryExceeded(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.ListSpaces(context.Background())
	if err == nil {
		t.Fatal("expected error after max retries, got nil")
	}
	// 1回目 + 最大3回リトライ = 4回(1+3)
	if attempts != 4 {
		t.Errorf("expected 4 attempts, got %d", attempts)
	}
}

// --- POST は 429 のみ再試行（5xx は再試行しない）---

func TestPost_NoRetryOn5xx(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			attempts++
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.CreatePage(context.Background(), "DEV", "Test", "<p>body</p>")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if attempts != 1 {
		t.Errorf("POST 5xx should not retry: expected 1 attempt, got %d", attempts)
	}
}

func TestPost_Retry429(t *testing.T) {
	attempts := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			attempts++
			if attempts <= 2 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"id": "99", "title": "Test",
				"space":   map[string]any{"key": "DEV"},
				"version": map[string]any{"number": 1},
				"_links":  map[string]any{"base": srv.URL, "webui": "/pages/99"},
			})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.CreatePage(context.Background(), "DEV", "Test", "<p>body</p>")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

// --- PAT 認証ヘッダー ---

func TestPATAuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	c.ListSpaces(context.Background())

	want := "Bearer test-token"
	if gotAuth != want {
		t.Errorf("Authorization: got %q, want %q", gotAuth, want)
	}
}

// --- 401/403 は認証エラーで即終了 ---

func TestGet_AuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.ListSpaces(context.Background())
	if err == nil {
		t.Fatal("expected auth error")
	}
}

// --- ListSpaces ---

func TestListSpaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/space" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"results":[{"key":"DEV","name":"開発","_links":{"base":"%s","webui":"/display/DEV"}}],"size":1}`, r.Host)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	spaces, err := c.ListSpaces(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(spaces) != 1 {
		t.Fatalf("expected 1 space, got %d", len(spaces))
	}
	if spaces[0].Key != "DEV" {
		t.Errorf("key: got %q, want DEV", spaces[0].Key)
	}
}

// --- GetPage ---

func TestGetPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"id":"12345","title":"Test Page",
			"space":{"key":"DEV"},
			"version":{"number":3},
			"body":{"storage":{"value":"<p>hello</p>"}},
			"_links":{"base":"%s","webui":"/pages/12345"}
		}`, r.Host)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	page, err := c.GetPage(context.Background(), "12345")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.ID != "12345" {
		t.Errorf("ID: got %q", page.ID)
	}
	if page.StorageBody != "<p>hello</p>" {
		t.Errorf("StorageBody: got %q", page.StorageBody)
	}
}

// --- GetPage 404 ---

// --- TLS 設定の強化 ---

func TestNew_TLSMinVersion(t *testing.T) {
	// insecure=false の場合、TLS 1.2 未満は拒否される
	c := client.New("https://localhost:1", "token", false)
	tr := client.GetTransport(c)
	if tr == nil {
		t.Fatal("expected explicit Transport, got nil")
	}
	if tr.TLSClientConfig == nil {
		t.Fatal("expected TLSClientConfig, got nil")
	}
	// crypto/tls.VersionTLS12 = 0x0303
	if tr.TLSClientConfig.MinVersion != 0x0303 {
		t.Errorf("expected MinVersion TLS 1.2 (0x0303), got 0x%04x", tr.TLSClientConfig.MinVersion)
	}
	if tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be false when insecure=false")
	}
}

func TestNew_InsecureMode(t *testing.T) {
	c := client.New("https://localhost:1", "token", true)
	tr := client.GetTransport(c)
	if tr == nil {
		t.Fatal("expected explicit Transport, got nil")
	}
	if tr.TLSClientConfig == nil {
		t.Fatal("expected TLSClientConfig, got nil")
	}
	if !tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be true when insecure=true")
	}
	// insecure=true でも TLS 1.2 最低は維持
	if tr.TLSClientConfig.MinVersion != 0x0303 {
		t.Errorf("expected MinVersion TLS 1.2 even in insecure mode, got 0x%04x", tr.TLSClientConfig.MinVersion)
	}
}

func TestGetPage_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.GetPage(context.Background(), "99999")
	if err == nil {
		t.Fatal("expected not found error")
	}
}
