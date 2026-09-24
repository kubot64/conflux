package porttest

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/kubot64/conflux/internal/port"
)

func TestMocksSatisfyInterfaces(t *testing.T) {
	var (
		_ port.SpaceClient      = (*MockSpaceClient)(nil)
		_ port.PageClient       = (*MockPageClient)(nil)
		_ port.AttachmentClient = (*MockAttachmentClient)(nil)
		_ port.Converter        = (*MockConverter)(nil)
		_ port.AliasStore       = (*MockAliasStore)(nil)
		_ port.HistoryLogger    = (*MockHistoryLogger)(nil)
	)

	page, err := (&MockPageClient{
		GetPageFn: func(ctx context.Context, id string) (*port.Page, error) {
			return &port.Page{ID: id, Title: "t"}, nil
		},
	}).GetPage(context.Background(), "42")
	if err != nil || page.ID != "42" {
		t.Fatalf("page mock: %+v %v", page, err)
	}

	rc, err := (&MockAttachmentClient{
		DownloadAttachmentFn: func(ctx context.Context, attachmentID string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader([]byte(attachmentID))), nil
		},
	}).DownloadAttachment(context.Background(), "att")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != "att" {
		t.Fatalf("attachment mock: %q", got)
	}
}
