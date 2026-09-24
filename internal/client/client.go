package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kubot64/conflux/internal/apperror"
	"github.com/kubot64/conflux/internal/port"
)

const (
	maxRetries  = 3
	backoffBase = 1 * time.Second
	backoffMax  = 8 * time.Second
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
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		ForceAttemptHTTP2:     true,
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
		savedGetBody := req.GetBody
		savedLen := req.ContentLength
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
			slog.Debug("auth failed", "status", resp.StatusCode, "method", req.Method, "path", req.URL.Path)
			return nil, apperror.New(apperror.KindAuth, "authentication failed")
		}

		// 404 は即終了（リトライしない）
		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			slog.Debug("not found", "method", req.Method, "path", req.URL.Path)
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
			newReq, rerr := cloneForRetry(req, savedGetBody, savedLen)
			if rerr != nil {
				return nil, rerr
			}
			req = newReq
			continue
		}

		// 5xx: GET は再試行、POST/PUT は再試行しない
		if resp.StatusCode >= 500 {
			status := resp.StatusCode
			resp.Body.Close()
			if isWrite || attempt == maxRetries {
				slog.Debug("server error", "status", status, "method", req.Method, "path", req.URL.Path, "attempt", attempt)
				return nil, apperror.New(apperror.KindServer, "server error")
			}
			wait := jitterBackoff(attempt)
			select {
			case <-req.Context().Done():
				return nil, contextError(req.Context().Err())
			case <-time.After(wait):
			}
			newReq, rerr := cloneForRetry(req, savedGetBody, savedLen)
			if rerr != nil {
				return nil, rerr
			}
			req = newReq
			continue
		}

		// その他エラー
		status := resp.StatusCode
		resp.Body.Close()
		slog.Debug("unexpected response", "status", status, "method", req.Method, "path", req.URL.Path)
		return nil, apperror.New(apperror.KindServer, "unexpected response from server")
	}
	return nil, apperror.New(apperror.KindServer, "max retries exceeded")
}

func cloneForRetry(req *http.Request, getBody func() (io.ReadCloser, error), contentLen int64) (*http.Request, error) {
	var body io.Reader
	if getBody != nil {
		rc, err := getBody()
		if err != nil {
			return nil, apperror.New(apperror.KindServer, fmt.Sprintf("retry body: %v", err))
		}
		body = rc
	}
	cloned, err := http.NewRequestWithContext(req.Context(), req.Method, req.URL.String(), body)
	if err != nil {
		return nil, apperror.New(apperror.KindServer, err.Error())
	}
	cloned.Header = req.Header.Clone()
	cloned.Header.Del("Content-Length")
	cloned.GetBody = getBody
	if contentLen > 0 {
		cloned.ContentLength = contentLen
	}
	return cloned, nil
}

const maxPages = 100

func (c *Client) forEachPage(ctx context.Context, path string, handle func(raw []byte) (string, error)) error {
	seen := map[string]struct{}{}
	for i := 0; i < maxPages; i++ {
		if _, ok := seen[path]; ok {
			return apperror.New(apperror.KindServer, "pagination loop")
		}
		seen[path] = struct{}{}
		req, err := c.newReq(ctx, http.MethodGet, path, nil)
		if err != nil {
			return err
		}
		resp, err := c.do(req, false)
		if err != nil {
			return err
		}
		raw, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return apperror.New(apperror.KindServer, fmt.Sprintf("read body: %v", err))
		}
		next, err := handle(raw)
		if err != nil {
			return err
		}
		if next == "" {
			return nil
		}
		path, err = c.normalizeNext(next)
		if err != nil {
			return err
		}
	}
	return apperror.New(apperror.KindServer, "too many result pages")
}

func (c *Client) normalizeNext(next string) (string, error) {
	if strings.HasPrefix(next, "/") {
		return next, nil
	}
	u, err := url.Parse(next)
	if err != nil {
		return "", apperror.New(apperror.KindServer, fmt.Sprintf("pagination link: %v", err))
	}
	if u.Host != "" {
		base, err := url.Parse(c.baseURL)
		if err != nil || !strings.EqualFold(u.Host, base.Host) {
			return "", apperror.New(apperror.KindServer, "pagination link points to another host")
		}
	}
	if u.RequestURI() == "" {
		return "", apperror.New(apperror.KindServer, "pagination link is empty")
	}
	return u.RequestURI(), nil
}

func (c *Client) ensureSameOrigin(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return apperror.New(apperror.KindServer, "download url is invalid")
	}
	base, err := url.Parse(c.baseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return apperror.New(apperror.KindServer, "base url is invalid")
	}
	if !strings.EqualFold(u.Scheme, base.Scheme) || !strings.EqualFold(u.Host, base.Host) {
		return apperror.New(apperror.KindServer, "download url points to another host")
	}
	return nil
}

func (c *Client) absoluteLink(linkBase, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref
	}
	base := strings.TrimRight(linkBase, "/")
	if base == "" {
		base = c.baseURL
	}
	return base + "/" + strings.TrimLeft(ref, "/")
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
	jitter := time.Duration(rand.Int64N(int64(exp / 2)))
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
	Links struct {
		Base string `json:"base"`
		Next string `json:"next"`
	} `json:"_links"`
}

func (c *Client) ListSpaces(ctx context.Context) ([]port.Space, error) {
	var spaces []port.Space
	err := c.forEachPage(ctx, "/rest/api/space?limit=100", func(raw []byte) (string, error) {
		var result spaceListResponse
		if err := json.Unmarshal(raw, &result); err != nil {
			return "", apperror.New(apperror.KindServer, fmt.Sprintf("decode: %v", err))
		}
		for _, r := range result.Results {
			base := r.Links.Base
			if base == "" {
				base = result.Links.Base
			}
			spaces = append(spaces, port.Space{
				Key:  r.Key,
				Name: r.Name,
				URL:  c.absoluteLink(base, r.Links.WebUI),
			})
		}
		return result.Links.Next, nil
	})
	return spaces, err
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

func (c *Client) toPage(r pageResponse) *port.Page {
	return &port.Page{
		ID:           r.ID,
		Title:        r.Title,
		Space:        r.Space.Key,
		Version:      r.Version.Number,
		StorageBody:  r.Body.Storage.Value,
		LastModified: r.History.LastUpdated.When,
		URL:          c.absoluteLink(r.Links.Base, r.Links.WebUI),
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
	return c.toPage(r), nil
}

func (c *Client) ServerInfo(ctx context.Context) (string, error) {
	req, err := c.newReq(ctx, http.MethodGet, "/rest/api/serverInfo", nil)
	if err != nil {
		return "", err
	}
	resp, err := c.do(req, false)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var body struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", apperror.New(apperror.KindServer, fmt.Sprintf("decode: %v", err))
	}
	return body.Version, nil
}

type searchResponse struct {
	Results []struct {
		ID      string               `json:"id"`
		Title   string               `json:"title"`
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
	Links struct {
		Base string `json:"base"`
		Next string `json:"next"`
	} `json:"_links"`
}

// escapeCQL は Confluence CQL クエリの文字列値をエスケープする。
func escapeCQL(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func (c *Client) searchResultFrom(r struct {
	ID      string               `json:"id"`
	Title   string               `json:"title"`
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
}, pageBase string) port.PageSearchResult {
	base := r.Links.Base
	if base == "" {
		base = pageBase
	}
	return port.PageSearchResult{
		ID:           r.ID,
		Title:        r.Title,
		Space:        r.Space.Key,
		LastModified: r.History.LastUpdated.When,
		URL:          c.absoluteLink(base, r.Links.WebUI),
	}
}

func (c *Client) collectSearch(ctx context.Context, path string) ([]port.PageSearchResult, error) {
	var pages []port.PageSearchResult
	err := c.forEachPage(ctx, path, func(raw []byte) (string, error) {
		var result searchResponse
		if err := json.Unmarshal(raw, &result); err != nil {
			return "", apperror.New(apperror.KindServer, fmt.Sprintf("decode: %v", err))
		}
		for _, r := range result.Results {
			pages = append(pages, c.searchResultFrom(r, result.Links.Base))
		}
		return result.Links.Next, nil
	})
	return pages, err
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
	return c.collectSearch(ctx, path)
}

func (c *Client) FindPagesByTitle(ctx context.Context, space, title string) ([]port.PageSearchResult, error) {
	cql := fmt.Sprintf(`type=page AND space="%s" AND title="%s"`, escapeCQL(space), escapeCQL(title))
	path := fmt.Sprintf("/rest/api/content/search?cql=%s&expand=history.lastUpdated,space&limit=25", urlEncode(cql))
	return c.collectSearch(ctx, path)
}

func (c *Client) GetPageTree(ctx context.Context, space string, depth int) ([]port.PageTreeNode, error) {
	path := fmt.Sprintf("/rest/api/content?spaceKey=%s&type=page&expand=ancestors&limit=100", urlEncode(space))
	var nodes []port.PageTreeNode
	seen := map[string]struct{}{}
	err := c.forEachPage(ctx, path, func(raw []byte) (string, error) {
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
			Links struct {
				Base string `json:"base"`
				Next string `json:"next"`
			} `json:"_links"`
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			return "", apperror.New(apperror.KindServer, fmt.Sprintf("decode: %v", err))
		}
		for _, r := range result.Results {
			if _, ok := seen[r.ID]; ok {
				continue
			}
			d := len(r.Ancestors)
			if d > depth {
				continue
			}
			var parentID *string
			if d > 0 {
				pid := r.Ancestors[len(r.Ancestors)-1].ID
				parentID = &pid
			}
			base := r.Links.Base
			if base == "" {
				base = result.Links.Base
			}
			seen[r.ID] = struct{}{}
			nodes = append(nodes, port.PageTreeNode{
				ID:       r.ID,
				Title:    r.Title,
				ParentID: parentID,
				Depth:    d,
				URL:      c.absoluteLink(base, r.Links.WebUI),
			})
		}
		return result.Links.Next, nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].Depth != nodes[j].Depth {
			return nodes[i].Depth < nodes[j].Depth
		}
		return nodes[i].ID < nodes[j].ID
	})
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
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, apperror.New(apperror.KindValidation, fmt.Sprintf("encode: %v", err))
	}
	req, err := c.newReq(ctx, http.MethodPost, "/rest/api/content", bytes.NewReader(b))
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
	return c.toPage(r), nil
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
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, apperror.New(apperror.KindValidation, fmt.Sprintf("encode: %v", err))
	}
	path := fmt.Sprintf("/rest/api/content/%s", id)
	req, err := c.newReq(ctx, http.MethodPut, path, bytes.NewReader(b))
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
	return c.toPage(r), nil
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
			URL:       c.absoluteLink(r.Links.Base, r.Links.Download),
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
		URL:       c.absoluteLink(r0.Links.Base, r0.Links.Download),
	}, nil
}

func (c *Client) DownloadAttachment(ctx context.Context, attachmentID string) (io.ReadCloser, error) {
	meta, err := c.GetAttachment(ctx, attachmentID)
	if err != nil {
		return nil, err
	}
	if meta.URL == "" {
		return nil, apperror.New(apperror.KindServer, "attachment has no download url")
	}
	if err := c.ensureSameOrigin(meta.URL); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, meta.URL, nil)
	if err != nil {
		return nil, apperror.New(apperror.KindServer, fmt.Sprintf("download request: %v", err))
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.do(req, false)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (c *Client) GetAttachment(ctx context.Context, attachmentID string) (*port.Attachment, error) {
	path := fmt.Sprintf("/rest/api/content/%s?expand=extensions", urlEncode(attachmentID))
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
		URL:       c.absoluteLink(r.Links.Base, r.Links.Download),
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
