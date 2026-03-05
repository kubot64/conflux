package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kubot64/conflux/internal/apperror"
	"github.com/kubot64/conflux/internal/port"
)

const (
	maxRetries    = 3
	backoffBase   = 1 * time.Second
	backoffMax    = 8 * time.Second
)

// Client は Confluence REST API クライアント。
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	userAgent  string
}

// New は Client を生成する。
func New(baseURL, token string, insecure bool) *Client {
	c := &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		token:     token,
		userAgent: "conflux/1.0.0", // TODO: バージョン情報を cmd から渡せるようにする
	}
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: insecure,
		},
	}
	c.httpClient = &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			// HTTPS から HTTP へのダウングレードを禁止
			if !insecure && via[0].URL.Scheme == "https" && req.URL.Scheme == "http" {
				return fmt.Errorf("insecure redirect from https to http")
			}
			return nil
		},
	}
	return c
}

// --- 内部ヘルパー ---

func (c *Client) newReq(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	return req, nil
}

// do はリトライ付きでリクエストを実行する。
// isWrite=true の場合、POST/PUT は 429 のみ再試行（5xx は再試行しない）。
func (c *Client) do(req *http.Request, isWrite bool) (*http.Response, error) {
	var (
		resp *http.Response
		err  error
	)
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// リクエストボディを再生成できないため、リトライ不可の場合は即リターン
			// (本実装では body を都度生成しているので問題なし)
		}
		resp, err = c.httpClient.Do(req)
		if err != nil {
			// ネットワークエラーはコンテキストチェック
			if req.Context().Err() != nil {
				return nil, contextError(req.Context().Err())
			}
			return nil, apperror.New(apperror.KindServer, fmt.Sprintf("request failed: %v", err))
		}

		// 認証エラーは即終了
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			resp.Body.Close()
			return nil, apperror.New(apperror.KindAuth, fmt.Sprintf("authentication failed: %d", resp.StatusCode))
		}

		// 404 は即終了（リトライしない）
		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			return nil, apperror.New(apperror.KindNotFound, "resource not found")
		}

		// 成功
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, nil
		}

		// 429 はリトライ対象（GET も POST も）
		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			if attempt == maxRetries {
				break
			}
			wait := retryAfterDuration(resp)
			select {
			case <-req.Context().Done():
				return nil, contextError(req.Context().Err())
			case <-time.After(wait):
			}
			// リクエストを再生成（同じコンテキスト・メソッド・パスで）
			newReq, rerr := http.NewRequestWithContext(req.Context(), req.Method, req.URL.String(), req.Body)
			if rerr != nil {
				return nil, apperror.New(apperror.KindServer, rerr.Error())
			}
			newReq.Header = req.Header.Clone()
			req = newReq
			continue
		}

		// 5xx: GET は再試行、POST/PUT は再試行しない
		if resp.StatusCode >= 500 {
			resp.Body.Close()
			if isWrite || attempt == maxRetries {
				return nil, apperror.New(apperror.KindServer, fmt.Sprintf("server error: %d", resp.StatusCode))
			}
			wait := jitterBackoff(attempt)
			select {
			case <-req.Context().Done():
				return nil, contextError(req.Context().Err())
			case <-time.After(wait):
			}
			newReq, rerr := http.NewRequestWithContext(req.Context(), req.Method, req.URL.String(), req.Body)
			if rerr != nil {
				return nil, apperror.New(apperror.KindServer, rerr.Error())
			}
			newReq.Header = req.Header.Clone()
			req = newReq
			continue
		}

		// その他エラー
		resp.Body.Close()
		return nil, apperror.New(apperror.KindServer, fmt.Sprintf("unexpected status: %d", resp.StatusCode))
	}
	return nil, apperror.New(apperror.KindServer, "max retries exceeded")
}

func retryAfterDuration(resp *http.Response) time.Duration {
	if v := resp.Header.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil {
			return time.Duration(secs) * time.Second
		}
	}
	return jitterBackoff(0)
}

func jitterBackoff(attempt int) time.Duration {
	exp := backoffBase * (1 << uint(attempt))
	if exp > backoffMax {
		exp = backoffMax
	}
	jitter := time.Duration(rand.Int63n(int64(exp / 2)))
	return exp + jitter
}

func contextError(err error) error {
	if err == context.DeadlineExceeded {
		return apperror.New(apperror.KindTimeout, "request timed out")
	}
	return apperror.New(apperror.KindCanceled, "request canceled")
}

// --- SpaceClient ---

type spaceListResponse struct {
	Results []struct {
		Key   string `json:"key"`
		Name  string `json:"name"`
		Links struct {
			Base  string `json:"base"`
			WebUI string `json:"webui"`
		} `json:"_links"`
	} `json:"results"`
}

func (c *Client) ListSpaces(ctx context.Context) ([]port.Space, error) {
	req, err := c.newReq(ctx, http.MethodGet, "/rest/api/space?limit=200", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req, false)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result spaceListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, apperror.New(apperror.KindServer, fmt.Sprintf("decode: %v", err))
	}

	spaces := make([]port.Space, len(result.Results))
	for i, r := range result.Results {
		spaces[i] = port.Space{
			Key:  r.Key,
			Name: r.Name,
			URL:  r.Links.Base + r.Links.WebUI,
		}
	}
	return spaces, nil
}

// --- PageClient ---

type pageResponse struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Space struct {
		Key string `json:"key"`
	} `json:"space"`
	Version struct {
		Number int `json:"number"`
	} `json:"version"`
	Body struct {
		Storage struct {
			Value string `json:"value"`
		} `json:"storage"`
	} `json:"body"`
	History struct {
		LastUpdated struct {
			When time.Time `json:"when"`
		} `json:"lastUpdated"`
	} `json:"history"`
	Links struct {
		Base  string `json:"base"`
		WebUI string `json:"webui"`
	} `json:"_links"`
}

func toPage(r pageResponse) *port.Page {
	return &port.Page{
		ID:           r.ID,
		Title:        r.Title,
		Space:        r.Space.Key,
		Version:      r.Version.Number,
		StorageBody:  r.Body.Storage.Value,
		LastModified: r.History.LastUpdated.When,
		URL:          r.Links.Base + r.Links.WebUI,
	}
}

func (c *Client) GetPage(ctx context.Context, id string) (*port.Page, error) {
	path := fmt.Sprintf("/rest/api/content/%s?expand=body.storage,version,history.lastUpdated,space", id)
	req, err := c.newReq(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req, false)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var r pageResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, apperror.New(apperror.KindServer, fmt.Sprintf("decode: %v", err))
	}
	return toPage(r), nil
}

type searchResponse struct {
	Results []struct {
		ID      string `json:"id"`
		Title   string `json:"title"`
		Space   struct{ Key string } `json:"space"`
		History struct {
			LastUpdated struct {
				When time.Time `json:"when"`
			} `json:"lastUpdated"`
		} `json:"history"`
		Links struct {
			Base  string `json:"base"`
			WebUI string `json:"webui"`
		} `json:"_links"`
	} `json:"results"`
}

// escapeCQL は Confluence CQL クエリの文字列値をエスケープする。
func escapeCQL(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func (c *Client) SearchPages(ctx context.Context, keyword, space, after string) ([]port.PageSearchResult, error) {
	cql := `type=page`
	if space != "" {
		cql += fmt.Sprintf(` AND space="%s"`, escapeCQL(space))
	}
	if keyword != "" {
		cql += fmt.Sprintf(` AND text~"%s"`, escapeCQL(keyword))
	}
	if after != "" {
		cql += fmt.Sprintf(` AND lastModified>"%s"`, escapeCQL(after))
	}
	path := fmt.Sprintf("/rest/api/content/search?cql=%s&expand=history.lastUpdated,space&limit=50",
		urlEncode(cql))
	req, err := c.newReq(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req, false)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, apperror.New(apperror.KindServer, fmt.Sprintf("decode: %v", err))
	}
	pages := make([]port.PageSearchResult, len(result.Results))
	for i, r := range result.Results {
		pages[i] = port.PageSearchResult{
			ID:           r.ID,
			Title:        r.Title,
			Space:        r.Space.Key,
			LastModified: r.History.LastUpdated.When,
			URL:          r.Links.Base + r.Links.WebUI,
		}
	}
	return pages, nil
}

func (c *Client) FindPagesByTitle(ctx context.Context, space, title string) ([]port.PageSearchResult, error) {
	cql := fmt.Sprintf(`type=page AND space="%s" AND title="%s"`, escapeCQL(space), escapeCQL(title))
	path := fmt.Sprintf("/rest/api/content/search?cql=%s&expand=history.lastUpdated,space&limit=10", urlEncode(cql))
	req, err := c.newReq(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req, false)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, apperror.New(apperror.KindServer, fmt.Sprintf("decode: %v", err))
	}
	pages := make([]port.PageSearchResult, len(result.Results))
	for i, r := range result.Results {
		pages[i] = port.PageSearchResult{
			ID:           r.ID,
			Title:        r.Title,
			Space:        r.Space.Key,
			LastModified: r.History.LastUpdated.When,
			URL:          r.Links.Base + r.Links.WebUI,
		}
	}
	return pages, nil
}

func (c *Client) GetPageTree(ctx context.Context, space string, depth int) ([]port.PageTreeNode, error) {
	// ルートページを取得してから再帰的に子ページを取得する実装（フェーズ2で詳細実装）
	path := fmt.Sprintf("/rest/api/content?spaceKey=%s&type=page&expand=ancestors&limit=200", space)
	req, err := c.newReq(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req, false)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Results []struct {
			ID        string `json:"id"`
			Title     string `json:"title"`
			Ancestors []struct {
				ID string `json:"id"`
			} `json:"ancestors"`
			Links struct {
				Base  string `json:"base"`
				WebUI string `json:"webui"`
			} `json:"_links"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, apperror.New(apperror.KindServer, fmt.Sprintf("decode: %v", err))
	}

	nodes := make([]port.PageTreeNode, 0, len(result.Results))
	for _, r := range result.Results {
		var parentID *string
		d := len(r.Ancestors)
		if d > 0 && d <= depth {
			pid := r.Ancestors[len(r.Ancestors)-1].ID
			parentID = &pid
		} else if d > depth {
			continue // depth 超えはスキップ
		}
		nodes = append(nodes, port.PageTreeNode{
			ID:       r.ID,
			Title:    r.Title,
			ParentID: parentID,
			Depth:    d,
			URL:      r.Links.Base + r.Links.WebUI,
		})
	}
	return nodes, nil
}

func (c *Client) CreatePage(ctx context.Context, space, title, storageBody string) (*port.Page, error) {
	payload := map[string]any{
		"type":  "page",
		"title": title,
		"space": map[string]string{"key": space},
		"body": map[string]any{
			"storage": map[string]string{
				"value":          storageBody,
				"representation": "storage",
			},
		},
	}
	b, _ := json.Marshal(payload)
	req, err := c.newReq(ctx, http.MethodPost, "/rest/api/content", strings.NewReader(string(b)))
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req, true)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var r pageResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, apperror.New(apperror.KindServer, fmt.Sprintf("decode: %v", err))
	}
	return toPage(r), nil
}

func (c *Client) UpdatePage(ctx context.Context, id string, version int, title, storageBody string) (*port.Page, error) {
	payload := map[string]any{
		"type":    "page",
		"title":   title,
		"version": map[string]int{"number": version},
		"body": map[string]any{
			"storage": map[string]string{
				"value":          storageBody,
				"representation": "storage",
			},
		},
	}
	b, _ := json.Marshal(payload)
	path := fmt.Sprintf("/rest/api/content/%s", id)
	req, err := c.newReq(ctx, http.MethodPut, path, strings.NewReader(string(b)))
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req, true)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var r pageResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, apperror.New(apperror.KindServer, fmt.Sprintf("decode: %v", err))
	}
	return toPage(r), nil
}

// --- AttachmentClient ---

func (c *Client) ListAttachments(ctx context.Context, pageID string) ([]port.Attachment, error) {
	path := fmt.Sprintf("/rest/api/content/%s/child/attachment", pageID)
	req, err := c.newReq(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req, false)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Results []struct {
			ID         string `json:"id"`
			Title      string `json:"title"`
			Extensions struct {
				MediaType string `json:"mediaType"`
				FileSize  int64  `json:"fileSize"`
			} `json:"extensions"`
			Links struct {
				Base     string `json:"base"`
				Download string `json:"download"`
			} `json:"_links"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, apperror.New(apperror.KindServer, fmt.Sprintf("decode: %v", err))
	}

	attachments := make([]port.Attachment, len(result.Results))
	for i, r := range result.Results {
		attachments[i] = port.Attachment{
			ID:        r.ID,
			Filename:  r.Title,
			Size:      r.Extensions.FileSize,
			MediaType: r.Extensions.MediaType,
			URL:       r.Links.Base + r.Links.Download,
		}
	}
	return attachments, nil
}

func (c *Client) UploadAttachment(ctx context.Context, pageID, filename string, r io.Reader) (*port.Attachment, error) {
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)

	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return nil, apperror.New(apperror.KindServer, fmt.Sprintf("create form file: %v", err))
	}
	if _, err := io.Copy(part, r); err != nil {
		return nil, apperror.New(apperror.KindServer, fmt.Sprintf("copy file content: %v", err))
	}
	mw.Close()

	path := fmt.Sprintf("/rest/api/content/%s/child/attachment", pageID)
	req, err := c.newReq(ctx, http.MethodPost, path, &b)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Atlassian-Token", "nocheck")

	resp, err := c.do(req, true)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Results []struct {
			ID         string `json:"id"`
			Title      string `json:"title"`
			Extensions struct {
				MediaType string `json:"mediaType"`
				FileSize  int64  `json:"fileSize"`
			} `json:"extensions"`
			Links struct {
				Base     string `json:"base"`
				Download string `json:"download"`
			} `json:"_links"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, apperror.New(apperror.KindServer, fmt.Sprintf("decode: %v", err))
	}

	if len(result.Results) == 0 {
		return nil, apperror.New(apperror.KindServer, "no results returned after upload")
	}

	r0 := result.Results[0]
	return &port.Attachment{
		ID:        r0.ID,
		Filename:  r0.Title,
		Size:      r0.Extensions.FileSize,
		MediaType: r0.Extensions.MediaType,
		URL:       r0.Links.Base + r0.Links.Download,
	}, nil
}

func (c *Client) DownloadAttachment(ctx context.Context, attachmentID string) (io.ReadCloser, error) {
	path := fmt.Sprintf("/download/attachments/%s", attachmentID)
	req, err := c.newReq(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req, false)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (c *Client) GetAttachment(ctx context.Context, attachmentID string) (*port.Attachment, error) {
	path := fmt.Sprintf("/rest/api/content/%s", attachmentID)
	req, err := c.newReq(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req, false)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var r struct {
		ID         string `json:"id"`
		Title      string `json:"title"`
		Extensions struct {
			MediaType string `json:"mediaType"`
			FileSize  int64  `json:"fileSize"`
		} `json:"extensions"`
		Links struct {
			Base     string `json:"base"`
			Download string `json:"download"`
		} `json:"_links"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, apperror.New(apperror.KindServer, fmt.Sprintf("decode: %v", err))
	}
	return &port.Attachment{
		ID:        r.ID,
		Filename:  r.Title,
		Size:      r.Extensions.FileSize,
		MediaType: r.Extensions.MediaType,
		URL:       r.Links.Base + r.Links.Download,
	}, nil
}

func urlEncode(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if isUnreserved(c) {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func isUnreserved(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~'
}
