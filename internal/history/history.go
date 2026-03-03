package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/kubot64/conflux/internal/port"
)

const maxEntries = 1000

// Logger は port.HistoryLogger を実装する。
type Logger struct {
	dir string
}

// NewLogger は指定ディレクトリに history.json を管理する Logger を生成する。
func NewLogger(dir string) (*Logger, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return &Logger{dir: dir}, nil
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

func (l *Logger) path() string    { return filepath.Join(l.dir, "history.json") }
func (l *Logger) tmpPath() string { return filepath.Join(l.dir, ".history.json.tmp") }

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

	// アトミック書き込み: temp ファイルに書き出してから rename
	if err := os.WriteFile(l.tmpPath(), data, 0644); err != nil {
		return err
	}
	return os.Rename(l.tmpPath(), l.path())
}

// Log はエントリを history.json に記録する。
func (l *Logger) Log(entry port.HistoryEntry) error {
	hf, err := l.load()
	if err != nil {
		return err
	}

	hf.Entries = append(hf.Entries, historyEntry{
		Timestamp:     entry.Timestamp,
		SessionID:     entry.SessionID,
		Action:        entry.Action,
		PageID:        entry.PageID,
		Title:         entry.Title,
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
