package port

import (
	"context"
	"io"
	"time"
)

// --- Space ---

type Space struct {
	Key  string
	Name string
	URL  string
}

type SpaceClient interface {
	ListSpaces(ctx context.Context) ([]Space, error)
}

// --- Page ---

type Page struct {
	ID           string
	Title        string
	Space        string
	Version      int
	StorageBody  string // XHTML (storage format)
	LastModified time.Time
	URL          string
}

type PageSearchResult struct {
	ID           string
	Title        string
	Space        string
	LastModified time.Time
	URL          string
}

type PageTreeNode struct {
	ID       string
	Title    string
	ParentID *string
	Depth    int
	URL      string
}

type PageClient interface {
	GetPage(ctx context.Context, id string) (*Page, error)
	SearchPages(ctx context.Context, keyword, space, after string) ([]PageSearchResult, error)
	FindPagesByTitle(ctx context.Context, space, title string) ([]PageSearchResult, error)
	GetPageTree(ctx context.Context, space string, depth int) ([]PageTreeNode, error)
	CreatePage(ctx context.Context, space, title, storageBody string) (*Page, error)
	UpdatePage(ctx context.Context, id string, version int, title, storageBody string) (*Page, error)
}

// --- Attachment ---

type Attachment struct {
	ID       string
	Filename string
	Size     int64
	MediaType string
	URL      string
}

type AttachmentClient interface {
	ListAttachments(ctx context.Context, pageID string) ([]Attachment, error)
	UploadAttachment(ctx context.Context, pageID, filename string, r io.Reader) (*Attachment, error)
	DownloadAttachment(ctx context.Context, attachmentID string) (io.ReadCloser, error)
}

// --- Converter ---

type Converter interface {
	MarkdownToStorage(markdown string) (string, error)
	StorageToMarkdown(storage string) (string, error)
	ExtractSection(storage, sectionID string) (string, error)
}

// --- Alias ---

type AliasType string

const (
	AliasPage  AliasType = "page"
	AliasSpace AliasType = "space"
)

type Alias struct {
	Name   string
	Target string
	Type   AliasType
}

type AliasStore interface {
	Set(name, target string, t AliasType) error
	Get(name string) (*Alias, error)
	List() ([]Alias, error)
	Delete(name string) error
}

// --- HistoryLogger ---

type HistoryEntry struct {
	Timestamp     time.Time
	SessionID     string
	Action        string // "created" | "updated" | "uploaded"
	PageID        string
	Title         string
	Space         string
	VersionBefore int
	VersionAfter  int
}

type HistoryLogger interface {
	Log(entry HistoryEntry) error
	List(space, sessionID string, limit int) ([]HistoryEntry, error)
}
