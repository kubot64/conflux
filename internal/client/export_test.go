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
func NewWithResponseHeaderTimeout(baseURL, token string, insecure bool, d time.Duration) *Client {
	c := New(baseURL, token, insecure)
	if tr, ok := c.httpClient.Transport.(*http.Transport); ok {
		tr.ResponseHeaderTimeout = d
	}
	return c
}
