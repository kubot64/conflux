package client

import (
	"net/http"
	"time"
)

// GetTransport はテスト用に Client の http.Transport を返す。
func GetTransport(c *Client) *http.Transport {
	if t, ok := c.httpClient.Transport.(*http.Transport); ok {
		return t
	}
	return nil
}

// NewWithResponseHeaderTimeout はテスト用に ResponseHeaderTimeout を短縮した Client を生成する。
func NewWithResponseHeaderTimeout(baseURL, token string, opts Options, d time.Duration) *Client {
	opts.ResponseHeaderTimeout = d
	return New(baseURL, token, opts)
}

// SetBackoffBase はテスト用にリトライ待ちの基準時間を差し替える。
func SetBackoffBase(d time.Duration) (restore func()) {
	old := backoffBase
	backoffBase = d
	return func() { backoffBase = old }
}

// SetMaxUploadBytes はテスト用にアップロード上限を差し替える。
func SetMaxUploadBytes(n int64) (restore func()) {
	old := maxUploadBytes
	maxUploadBytes = n
	return func() { maxUploadBytes = old }
}

// SetMaxResponseBytes はテスト用にレスポンス上限を差し替える。
func SetMaxResponseBytes(n int64) (restore func()) {
	old := maxResponseBytes
	maxResponseBytes = n
	return func() { maxResponseBytes = old }
}
