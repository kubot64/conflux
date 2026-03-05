//go:build !production

package porttest

import (
	"context"
	"io"

	"github.com/kubot64/conflux/internal/port"
)

// MockSpaceClient はテスト用 SpaceClient モック。
type MockSpaceClient struct {
	ListSpacesFn func(ctx context.Context) ([]port.Space, error)
}

func (m *MockSpaceClient) ListSpaces(ctx context.Context) ([]port.Space, error) {
	return m.ListSpacesFn(ctx)
}

// MockPageClient はテスト用 PageClient モック。
type MockPageClient struct {
	GetPageFn           func(ctx context.Context, id string) (*port.Page, error)
	SearchPagesFn       func(ctx context.Context, keyword, space, after string) ([]port.PageSearchResult, error)
	FindPagesByTitleFn  func(ctx context.Context, space, title string) ([]port.PageSearchResult, error)
	GetPageTreeFn       func(ctx context.Context, space string, depth int) ([]port.PageTreeNode, error)
	CreatePageFn        func(ctx context.Context, space, title, storageBody string) (*port.Page, error)
	UpdatePageFn        func(ctx context.Context, id string, version int, title, storageBody string) (*port.Page, error)
}

func (m *MockPageClient) GetPage(ctx context.Context, id string) (*port.Page, error) {
	return m.GetPageFn(ctx, id)
}
func (m *MockPageClient) SearchPages(ctx context.Context, keyword, space, after string) ([]port.PageSearchResult, error) {
	return m.SearchPagesFn(ctx, keyword, space, after)
}
func (m *MockPageClient) FindPagesByTitle(ctx context.Context, space, title string) ([]port.PageSearchResult, error) {
	return m.FindPagesByTitleFn(ctx, space, title)
}
func (m *MockPageClient) GetPageTree(ctx context.Context, space string, depth int) ([]port.PageTreeNode, error) {
	return m.GetPageTreeFn(ctx, space, depth)
}
func (m *MockPageClient) CreatePage(ctx context.Context, space, title, storageBody string) (*port.Page, error) {
	return m.CreatePageFn(ctx, space, title, storageBody)
}
func (m *MockPageClient) UpdatePage(ctx context.Context, id string, version int, title, storageBody string) (*port.Page, error) {
	return m.UpdatePageFn(ctx, id, version, title, storageBody)
}

// MockAttachmentClient はテスト用 AttachmentClient モック。
type MockAttachmentClient struct {
	ListAttachmentsFn    func(ctx context.Context, pageID string) ([]port.Attachment, error)
	UploadAttachmentFn   func(ctx context.Context, pageID, filename string, r io.Reader) (*port.Attachment, error)
	DownloadAttachmentFn func(ctx context.Context, attachmentID string) (io.ReadCloser, error)
	GetAttachmentFn      func(ctx context.Context, attachmentID string) (*port.Attachment, error)
}

func (m *MockAttachmentClient) ListAttachments(ctx context.Context, pageID string) ([]port.Attachment, error) {
	return m.ListAttachmentsFn(ctx, pageID)
}
func (m *MockAttachmentClient) UploadAttachment(ctx context.Context, pageID, filename string, r io.Reader) (*port.Attachment, error) {
	return m.UploadAttachmentFn(ctx, pageID, filename, r)
}
func (m *MockAttachmentClient) DownloadAttachment(ctx context.Context, attachmentID string) (io.ReadCloser, error) {
	return m.DownloadAttachmentFn(ctx, attachmentID)
}
func (m *MockAttachmentClient) GetAttachment(ctx context.Context, attachmentID string) (*port.Attachment, error) {
	return m.GetAttachmentFn(ctx, attachmentID)
}

// MockConverter はテスト用 Converter モック。
type MockConverter struct {
	MarkdownToStorageFn func(markdown string) (string, error)
	StorageToMarkdownFn func(storage string) (string, error)
	ExtractSectionFn    func(storage, sectionID string) (string, error)
}

func (m *MockConverter) MarkdownToStorage(markdown string) (string, error) {
	return m.MarkdownToStorageFn(markdown)
}
func (m *MockConverter) StorageToMarkdown(storage string) (string, error) {
	return m.StorageToMarkdownFn(storage)
}
func (m *MockConverter) ExtractSection(storage, sectionID string) (string, error) {
	return m.ExtractSectionFn(storage, sectionID)
}

// MockAliasStore はテスト用 AliasStore モック。
type MockAliasStore struct {
	SetFn    func(name, target string, t port.AliasType) error
	GetFn    func(name string) (*port.Alias, error)
	ListFn   func() ([]port.Alias, error)
	DeleteFn func(name string) error
}

func (m *MockAliasStore) Set(name, target string, t port.AliasType) error {
	return m.SetFn(name, target, t)
}
func (m *MockAliasStore) Get(name string) (*port.Alias, error) { return m.GetFn(name) }
func (m *MockAliasStore) List() ([]port.Alias, error)         { return m.ListFn() }
func (m *MockAliasStore) Delete(name string) error            { return m.DeleteFn(name) }

// MockHistoryLogger はテスト用 HistoryLogger モック。
type MockHistoryLogger struct {
	LogFn  func(entry port.HistoryEntry) error
	ListFn func(space, sessionID string, limit int) ([]port.HistoryEntry, error)
}

func (m *MockHistoryLogger) Log(entry port.HistoryEntry) error { return m.LogFn(entry) }
func (m *MockHistoryLogger) List(space, sessionID string, limit int) ([]port.HistoryEntry, error) {
	return m.ListFn(space, sessionID, limit)
}
