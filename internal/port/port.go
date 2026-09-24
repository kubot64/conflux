package port

import "time"

// --- Space ---

type Space struct {
	Key  string
	Name string
	URL  string
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

// --- Attachment ---

type Attachment struct {
	ID        string
	Filename  string
	Size      int64
	MediaType string
	URL       string
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

// --- History ---

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
