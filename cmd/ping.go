package cmd

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/kubot64/conflux/internal/apperror"
	"github.com/kubot64/conflux/internal/config"
	"github.com/spf13/cobra"
)

var pingCmd = &cobra.Command{
	Use:   "ping",
	Short: "Confluence への疎通確認を行う",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		w := newWriter()
		ctx := cmd.Context()

		url := strings.TrimRight(cfg.URL, "/") + "/rest/api/serverInfo"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return apperror.New(apperror.KindServer, fmt.Sprintf("request: %v", err))
		}
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
		req.Header.Set("User-Agent", "conflux/1.0.0")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				if ctx.Err().Error() == "context deadline exceeded" {
					return apperror.New(apperror.KindTimeout, "request timed out")
				}
				return apperror.New(apperror.KindCanceled, "request canceled")
			}
			return apperror.New(apperror.KindServer, fmt.Sprintf("connection failed: %v", err))
		}
		defer resp.Body.Close()

		switch resp.StatusCode {
		case http.StatusOK:
			// ok
		case http.StatusUnauthorized, http.StatusForbidden:
			return apperror.New(apperror.KindAuth, "authentication failed")
		default:
			return apperror.New(apperror.KindServer, fmt.Sprintf("server returned %d", resp.StatusCode))
		}

		return w.Write("ping", map[string]any{
			"ok":  true,
			"url": cfg.URL,
		})
	},
}

func init() {
	rootCmd.AddCommand(pingCmd)
}
