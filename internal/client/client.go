package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
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

var (
	maxRetries       = 3
	backoffBase      = 1 * time.Second
	backoffMax       = 8 * time.Second
	maxResponseBytes = int64(32 << 20)
	maxUploadBytes   = int64(32 << 20)
)

// errRedirectRejected は別ホストや HTTPS→HTTP へのリダイレクトを拒否した印。
var errRedirectRejected = errors.New("redirect rejected")

// Options は Client の接続方針。
type Options struct {
	// AllowInsecureHTTP は http:// へのダウングレードリダイレクトを許す。
	AllowInsecureHTTP bool
	// SkipTLSVerify は TLS 証明書の検証を省略する。
	SkipTLSVerify bool
	// UserAgent が空なら conflux/dev を使う。
	UserAgent string
	// ResponseHeaderTimeout が 0 なら 30s。
	ResponseHeaderTimeout time.Duration
}

// Client は Confluence REST API クライアント。
type Client struct {
	baseURL           string
	token             string
	httpClient        *http.Client
	userAgent         string
	allowInsecureHTTP bool
}

// New は Client を生成する。
func New(baseURL, token string, opts Options) *Client {
	ua := opts.UserAgent
	if ua == "" {
		ua = "conflux/dev"
	}
	headerTimeout := opts.ResponseHeaderTimeout
	if headerTimeout <= 0 {
		headerTimeout = 30 * time.Second
	}
	c := &Client{
		baseURL:           strings.TrimRight(baseURL, "/"),
		token:             token,
		userAgent:         ua,
		allowInsecureHTTP: opts.AllowInsecureHTTP,
	}
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: headerTimeout,
		IdleConnTimeout:       90 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		ForceAttemptHTTP2:     true,
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: opts.SkipTLSVerify,
		},
	}
	c.httpClient = &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			base, err := url.Parse(c.baseURL)
			if err != nil || !sameOrigin(base, req.URL) {
				return errRedirectRejected
			}
			if !c.allowInsecureHTTP && via[0].URL.Scheme == "https" && req.URL.Scheme == "http" {
				return errRedirectRejected
			}
			return nil
		},
	}
	return c
}

func sameOrigin(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
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
// isWrite=true の場合、POST/PUT は 429 のみ再試行（5xx とネットワークエラーは再試行しない）。
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
			if req.Context().Err() != nil {
				return nil, contextError(req.Context().Err())
			}
			if errors.Is(err, errRedirectRejected) {
				return nil, apperror.New(apperror.KindServer, "redirect rejected")
			}
			if isWrite || attempt == maxRetries {
				if isNetTimeout(err) {
					return nil, apperror.New(apperror.KindTimeout, "request timed out")
				}
				return nil, apperror.New(apperror.KindServer, fmt.Sprintf("request failed: %v", err))
			}
			if rerr := waitRetry(req, jitterBackoff(attempt)); rerr != nil {
				return nil, rerr
			}
			newReq, rerr := cloneForRetry(req, savedGetBody, savedLen)
			if rerr != nil {
				return nil, rerr
			}
			req = newReq
			continue
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

		if resp.StatusCode == http.StatusBadRequest {
			resp.Body.Close()
			slog.Debug("bad request", "method", req.Method, "path", req.URL.Path)
			return nil, apperror.New(apperror.KindValidation, "request rejected")
		}

		if resp.StatusCode == http.StatusConflict {
			resp.Body.Close()
			slog.Debug("conflict", "method", req.Method, "path", req.URL.Path)
			return nil, apperror.New(apperror.KindConflict, "conflict")
		}

		// 成功
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, nil
		}

		// 429 はリトライ対象（GET も POST も）
		if resp.StatusCode == http.StatusTooManyRequests {
			wait := retryAfterDuration(resp)
			resp.Body.Close()
			if attempt == maxRetries {
				break
			}
			if rerr := waitRetry(req, wait); rerr != nil {
				return nil, rerr
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
			if rerr := waitRetry(req, jitterBackoff(attempt)); rerr != nil {
				return nil, rerr
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

func waitRetry(req *http.Request, wait time.Duration) error {
	select {
	case <-req.Context().Done():
		return contextError(req.Context().Err())
	case <-time.After(wait):
		return nil
	}
}

func isNetTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

func cloneForRetry(req *http.Request, getBody func() (io.ReadCloser, error), contentLen int64) (*http.Request, error) {
	if contentLen > 0 && getBody == nil {
		return nil, apperror.New(apperror.KindServer, "cannot retry request")
	}
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
		raw, err := readLimited(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
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

func readLimited(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxResponseBytes+1))
	if err != nil {
		return nil, apperror.New(apperror.KindServer, fmt.Sprintf("read body: %v", err))
	}
	if int64(len(b)) > maxResponseBytes {
		return nil, apperror.New(apperror.KindServer, "response too large")
	}
	return b, nil
}

func decodeLimited(r io.Reader, dest any) error {
	b, err := readLimited(r)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, dest); err != nil {
		return apperror.New(apperror.KindServer, fmt.Sprintf("decode: %v", err))
	}
	return nil
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
	if err := decodeLimited(resp.Body, &r); err != nil {
		return nil, err
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
	if err := decodeLimited(resp.Body, &body); err != nil {
		return "", err
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

type contentPage struct {
	ID    string
	Title string
	Base  string
	WebUI string
}

func (c *Client) listContent(ctx context.Context, path string) ([]contentPage, error) {
	var pages []contentPage
	err := c.forEachPage(ctx, path, func(raw []byte) (string, error) {
		var result struct {
			Results []struct {
				ID    string `json:"id"`
				Title string `json:"title"`
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
			base := r.Links.Base
			if base == "" {
				base = result.Links.Base
			}
			pages = append(pages, contentPage{ID: r.ID, Title: r.Title, Base: base, WebUI: r.Links.WebUI})
		}
		return result.Links.Next, nil
	})
	return pages, err
}

func (c *Client) GetPageTree(ctx context.Context, space string, depth int) ([]port.PageTreeNode, error) {
	if depth < 0 {
		return nil, apperror.New(apperror.KindValidation, "depth must be >= 0")
	}
	rootPath := fmt.Sprintf("/rest/api/space/%s/content/page?depth=root&limit=100", urlEncode(space))
	roots, err := c.listContent(ctx, rootPath)
	if err != nil {
		return nil, err
	}

	type queued struct {
		page   contentPage
		depth  int
		parent *string
	}
	queue := make([]queued, 0, len(roots))
	for _, p := range roots {
		queue = append(queue, queued{page: p, depth: 0})
	}

	var nodes []port.PageTreeNode
	seen := map[string]struct{}{}
	const maxTreeNodes = 10000
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if _, ok := seen[cur.page.ID]; ok {
			continue
		}
		if len(nodes) >= maxTreeNodes {
			return nil, apperror.New(apperror.KindServer, "too many pages in tree")
		}
		seen[cur.page.ID] = struct{}{}
		var parentID *string
		if cur.parent != nil {
			pid := *cur.parent
			parentID = &pid
		}
		nodes = append(nodes, port.PageTreeNode{
			ID:       cur.page.ID,
			Title:    cur.page.Title,
			ParentID: parentID,
			Depth:    cur.depth,
			URL:      c.absoluteLink(cur.page.Base, cur.page.WebUI),
		})
		if cur.depth >= depth {
			continue
		}
		childPath := fmt.Sprintf("/rest/api/content/%s/child/page?limit=100", urlEncode(cur.page.ID))
		children, err := c.listContent(ctx, childPath)
		if err != nil {
			return nil, err
		}
		parent := cur.page.ID
		for _, ch := range children {
			queue = append(queue, queued{page: ch, depth: cur.depth + 1, parent: &parent})
		}
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
	if err := decodeLimited(resp.Body, &r); err != nil {
		return nil, err
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
	if err := decodeLimited(resp.Body, &r); err != nil {
		return nil, err
	}
	return c.toPage(r), nil
}

// --- AttachmentClient ---

func (c *Client) ListAttachments(ctx context.Context, pageID string) ([]port.Attachment, error) {
	path := fmt.Sprintf("/rest/api/content/%s/child/attachment?limit=100", urlEncode(pageID))
	var attachments []port.Attachment
	err := c.forEachPage(ctx, path, func(raw []byte) (string, error) {
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
			Links struct {
				Base string `json:"base"`
				Next string `json:"next"`
			} `json:"_links"`
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			return "", apperror.New(apperror.KindServer, fmt.Sprintf("decode: %v", err))
		}
		for _, r := range result.Results {
			base := r.Links.Base
			if base == "" {
				base = result.Links.Base
			}
			attachments = append(attachments, port.Attachment{
				ID:        r.ID,
				Filename:  r.Title,
				Size:      r.Extensions.FileSize,
				MediaType: r.Extensions.MediaType,
				URL:       c.absoluteLink(base, r.Links.Download),
			})
		}
		return result.Links.Next, nil
	})
	return attachments, err
}

func (c *Client) UploadAttachment(ctx context.Context, pageID, filename string, r io.Reader) (*port.Attachment, error) {
	body, getBody, contentType, contentLen, err := buildMultipart(filename, r)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/rest/api/content/%s/child/attachment", urlEncode(pageID))
	req, err := c.newReq(ctx, http.MethodPost, path, body)
	if err != nil {
		return nil, err
	}
	req.GetBody = getBody
	req.ContentLength = contentLen
	req.Header.Set("Content-Type", contentType)
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
	if err := decodeLimited(resp.Body, &result); err != nil {
		return nil, err
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
	if err := decodeLimited(resp.Body, &r); err != nil {
		return nil, err
	}
	return &port.Attachment{
		ID:        r.ID,
		Filename:  r.Title,
		Size:      r.Extensions.FileSize,
		MediaType: r.Extensions.MediaType,
		URL:       c.absoluteLink(r.Links.Base, r.Links.Download),
	}, nil
}

func escapeQuotes(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

// buildMultipart はファイル本体をメモリに溜めず multipart 本文を作る。
// ReadSeeker ならサイズを確かめてから都度シークして再送できる。
func buildMultipart(filename string, r io.Reader) (io.Reader, func() (io.ReadCloser, error), string, int64, error) {
	boundary := fmt.Sprintf("conflux%x", rand.Uint64())
	contentType := "multipart/form-data; boundary=" + boundary
	header := fmt.Sprintf("--%s\r\nContent-Disposition: form-data; name=\"file\"; filename=\"%s\"\r\nContent-Type: application/octet-stream\r\n\r\n", boundary, escapeQuotes(filename))
	footer := fmt.Sprintf("\r\n--%s--\r\n", boundary)

	if rs, ok := r.(io.ReadSeeker); ok {
		size, err := rs.Seek(0, io.SeekEnd)
		if err != nil {
			return nil, nil, "", 0, apperror.New(apperror.KindValidation, fmt.Sprintf("attachment size: %v", err))
		}
		if _, err := rs.Seek(0, io.SeekStart); err != nil {
			return nil, nil, "", 0, apperror.New(apperror.KindValidation, fmt.Sprintf("attachment rewind: %v", err))
		}
		if size > maxUploadBytes {
			return nil, nil, "", 0, apperror.New(apperror.KindValidation, "attachment exceeds size limit")
		}
		getBody := func() (io.ReadCloser, error) {
			if _, err := rs.Seek(0, io.SeekStart); err != nil {
				return nil, err
			}
			return io.NopCloser(io.MultiReader(
				strings.NewReader(header),
				io.LimitReader(rs, size),
				strings.NewReader(footer),
			)), nil
		}
		rc, err := getBody()
		if err != nil {
			return nil, nil, "", 0, apperror.New(apperror.KindValidation, err.Error())
		}
		return rc, getBody, contentType, int64(len(header)+len(footer)) + size, nil
	}

	payload, err := io.ReadAll(io.LimitReader(r, maxUploadBytes+1))
	if err != nil {
		return nil, nil, "", 0, apperror.New(apperror.KindServer, fmt.Sprintf("read attachment: %v", err))
	}
	if int64(len(payload)) > maxUploadBytes {
		return nil, nil, "", 0, apperror.New(apperror.KindValidation, "attachment exceeds size limit")
	}
	snap := append([]byte(header), payload...)
	snap = append(snap, footer...)
	getBody := func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(snap)), nil
	}
	return bytes.NewReader(snap), getBody, contentType, int64(len(snap)), nil
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
