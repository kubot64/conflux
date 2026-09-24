package client_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"title":"Test"`) {
				t.Errorf("attempt %d body missing title: %s", attempts, body)
			}
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

func TestNew_TransportTimeouts(t *testing.T) {
	c := client.New("https://localhost:1", "token", false)
	tr := client.GetTransport(c)
	if tr == nil {
		t.Fatal("expected explicit Transport, got nil")
	}
	if tr.DialContext == nil {
		t.Error("DialContext should be set to control dial timeout and keepalive")
	}
	if tr.TLSHandshakeTimeout <= 0 {
		t.Error("TLSHandshakeTimeout should be > 0")
	}
	if tr.ResponseHeaderTimeout <= 0 {
		t.Error("ResponseHeaderTimeout should be > 0")
	}
	if tr.IdleConnTimeout <= 0 {
		t.Error("IdleConnTimeout should be > 0")
	}
	if tr.ExpectContinueTimeout <= 0 {
		t.Error("ExpectContinueTimeout should be > 0")
	}
}

// スローレスポンス（ヘッダ送出が ResponseHeaderTimeout を超える）でエラー返す。
func TestResponseHeaderTimeout_TripsOnSlowHeaders(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow header timeout test in short mode")
	}
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	defer srv.Close()
	defer close(block)

	c := client.NewWithResponseHeaderTimeout(srv.URL, "test-token", true, 100*time.Millisecond)
	_, err := c.ListSpaces(context.Background())
	if err == nil {
		t.Fatal("expected timeout error, got nil")
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

// HTTP ステータスコードや内部ディテールはユーザ向けエラーメッセージに
// 露出させない。slog 側に逃がしているため、CLI 利用者はエラー Kind と
// 抽象的な説明だけを受け取る。
func TestErrorMessage_DoesNotLeakHTTPStatus(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{"server_500", http.StatusInternalServerError},
		{"server_503", http.StatusServiceUnavailable},
		{"unexpected_418", http.StatusTeapot},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			c := newTestClient(t, srv)
			_, err := c.CreatePage(context.Background(), "DEV", "Test", "<p>body</p>")
			if err == nil {
				t.Fatalf("expected error for status %d", tt.status)
			}
			msg := err.Error()
			if strings.Contains(msg, fmt.Sprintf("%d", tt.status)) {
				t.Errorf("error message must not contain status code %d, got: %s", tt.status, msg)
			}
		})
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

func TestListSpaces_FollowsNext(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("start") == "1" {
			fmt.Fprintf(w, `{"results":[{"key":"B","name":"Bee","_links":{"webui":"/display/B"}}],"_links":{"base":"%s"}}`, srv.URL)
			return
		}
		fmt.Fprintf(w, `{"results":[{"key":"A","name":"Aye","_links":{"webui":"/display/A"}}],"_links":{"base":"%s","next":"%s/rest/api/space?limit=1&start=1"}}`, srv.URL, srv.URL)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	spaces, err := c.ListSpaces(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(spaces) != 2 {
		t.Fatalf("spaces: got %d, want 2 (%v)", len(spaces), spaces)
	}
	if spaces[0].Key != "A" || spaces[1].Key != "B" {
		t.Fatalf("keys: %+v", spaces)
	}
	if !strings.HasPrefix(spaces[0].URL, "http") {
		t.Errorf("space URL should be absolute, got %q", spaces[0].URL)
	}
}

func TestGetPageTree_PaginatesAndOrdersByDepth(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("start") == "1" {
			fmt.Fprintf(w, `{"results":[{"id":"1","title":"Root","ancestors":[],"_links":{"webui":"/pages/1"}}],"_links":{"base":"%s"}}`, srv.URL)
			return
		}
		fmt.Fprintf(w, `{"results":[{"id":"2","title":"Child","ancestors":[{"id":"1"}],"_links":{"webui":"/pages/2"}}],"_links":{"next":"%s/rest/api/content?start=1","base":"%s"}}`, srv.URL, srv.URL)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	nodes, err := c.GetPageTree(context.Background(), "TEAM", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 {
		t.Fatalf("nodes: got %d, want 2", len(nodes))
	}
	if nodes[0].Depth != 0 || nodes[0].ID != "1" {
		t.Fatalf("first node should be root, got %+v", nodes[0])
	}
	if nodes[1].Depth != 1 || nodes[1].ParentID == nil || *nodes[1].ParentID != "1" {
		t.Fatalf("second node should be child, got %+v", nodes[1])
	}
}

func TestDownloadAttachment_FollowsDownloadLink(t *testing.T) {
	const body = "file-bytes"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/content/att-9":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"id":"att-9","title":"a.txt","extensions":{"mediaType":"text/plain","fileSize":%d},"_links":{"download":"/download/attachments/42/a.txt"}}`, len(body))
		case "/download/attachments/42/a.txt":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = io.WriteString(w, body)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	rc, err := c.DownloadAttachment(context.Background(), "att-9")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != body {
		t.Fatalf("body: got %q", got)
	}
}

func TestDownloadAttachment_RejectsOtherHost(t *testing.T) {
	var gotAuth string
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, "secret")
	}))
	defer evil.Close()

	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/content/att-9" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"id":"att-9","title":"a.txt","extensions":{"mediaType":"text/plain","fileSize":1},"_links":{"download":"%s/steal"}}`, evil.URL)
			return
		}
		http.NotFound(w, r)
	}))
	defer good.Close()

	c := newTestClient(t, good)
	if _, err := c.DownloadAttachment(context.Background(), "att-9"); err == nil {
		t.Fatal("expected error for off-host download url")
	}
	if gotAuth != "" {
		t.Fatalf("token was sent to another host: %q", gotAuth)
	}
}
