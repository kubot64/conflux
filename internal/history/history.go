package history

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kubot64/conflux/internal/fileutil"
	"github.com/kubot64/conflux/internal/port"
)

const maxEntries = 1000

// Logger は port.HistoryLogger を実装する。
type Logger struct {
	dir         string
	redactTitle bool
}

// Option は Logger のオプション関数型。
type Option func(*Logger)

// WithRedactTitle はタイトルを SHA-256 ハッシュで置換するオプション。
func WithRedactTitle(v bool) Option {
	return func(l *Logger) {
		l.redactTitle = v
	}
}

// NewLogger は指定ディレクトリに history.json を管理する Logger を生成する。
func NewLogger(dir string, opts ...Option) (*Logger, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	l := &Logger{dir: dir}
	for _, opt := range opts {
		opt(l)
	}
	return l, nil
}

type historyFile struct {
	Entries []historyEntry `json:"entries"`
}

type historyEntry struct {
	Timestamp     time.Time `json:"timestamp"`
	SessionID     string    `json:"session_id"`
	Action        string    `json:"action"`
	PageID        string    `json:"page_id"`
	Title         string    `json:"title"`
	Space         string    `json:"space"`
	VersionBefore int       `json:"version_before,omitempty"`
	VersionAfter  int       `json:"version_after,omitempty"`
}

func (l *Logger) path() string { return filepath.Join(l.dir, "history.json") }

func (l *Logger) load() (*historyFile, error) {
	data, err := os.ReadFile(l.path())
	if os.IsNotExist(err) {
		return &historyFile{}, nil
	}
	if err != nil {
		return nil, err
	}
	var hf historyFile
	if err := json.Unmarshal(data, &hf); err != nil {
		return nil, err
	}
	return &hf, nil
}

func (l *Logger) save(hf *historyFile) error {
	// 最大 1000 件を超えた場合は古いエントリを削除
	if len(hf.Entries) > maxEntries {
		hf.Entries = hf.Entries[len(hf.Entries)-maxEntries:]
	}

	data, err := json.MarshalIndent(hf, "", "  ")
	if err != nil {
		return err
	}

	return fileutil.AtomicWrite(l.path(), data, 0600)
}

// Log はエントリを history.json に記録する。
func (l *Logger) Log(entry port.HistoryEntry) error {
	hf, err := l.load()
	if err != nil {
		return err
	}

	title := entry.Title
	if l.redactTitle && title != "" {
		h := sha256.Sum256([]byte(title))
		title = fmt.Sprintf("sha256:%x", h[:8])
	}

	hf.Entries = append(hf.Entries, historyEntry{
		Timestamp:     entry.Timestamp,
		SessionID:     entry.SessionID,
		Action:        entry.Action,
		PageID:        entry.PageID,
		Title:         title,
		Space:         entry.Space,
		VersionBefore: entry.VersionBefore,
		VersionAfter:  entry.VersionAfter,
	})

	return l.save(hf)
}

// List はフィルタ条件に合うエントリを返す。limit=0 は全件。
func (l *Logger) List(space, sessionID string, limit int) ([]port.HistoryEntry, error) {
	hf, err := l.load()
	if err != nil {
		return nil, err
	}

	var result []port.HistoryEntry
	// 新しい順（末尾から）に走査
	for i := len(hf.Entries) - 1; i >= 0; i-- {
		e := hf.Entries[i]
		if space != "" && e.Space != space {
			continue
		}
		if sessionID != "" && e.SessionID != sessionID {
			continue
		}
		result = append(result, port.HistoryEntry{
			Timestamp:     e.Timestamp,
			SessionID:     e.SessionID,
			Action:        e.Action,
			PageID:        e.PageID,
			Title:         e.Title,
			Space:         e.Space,
			VersionBefore: e.VersionBefore,
			VersionAfter:  e.VersionAfter,
		})
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result, nil
}
