package client

import "net/http"

// GetTransport はテスト用に Client の http.Transport を返す。
func GetTransport(c *Client) *http.Transport {
	if t, ok := c.httpClient.Transport.(*http.Transport); ok {
		return t
	}
	return nil
}
