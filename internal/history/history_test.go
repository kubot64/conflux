package history_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kubot64/conflux/internal/history"
	"github.com/kubot64/conflux/internal/port"
)

func newLogger(t *testing.T) (*history.Logger, string) {
	t.Helper()
	dir := t.TempDir()
	logger, err := history.NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	return logger, dir
}

func TestLog_And_List(t *testing.T) {
	logger, _ := newLogger(t)

	entry := port.HistoryEntry{
		Timestamp:     time.Now().UTC().Truncate(time.Second),
		SessionID:     "sess-001",
		Action:        "updated",
		PageID:        "12345",
		Title:         "テストページ",
		Space:         "DEV",
		VersionBefore: 3,
		VersionAfter:  4,
	}
	if err := logger.Log(entry); err != nil {
		t.Fatalf("Log: %v", err)
	}

	entries, err := logger.List("", "", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	got := entries[0]
	if got.PageID != entry.PageID {
		t.Errorf("PageID: got %q, want %q", got.PageID, entry.PageID)
	}
	if got.Action != entry.Action {
		t.Errorf("Action: got %q, want %q", got.Action, entry.Action)
	}
}

func TestList_FilterBySpace(t *testing.T) {
	logger, _ := newLogger(t)

	_ = logger.Log(port.HistoryEntry{SessionID: "s1", Action: "updated", PageID: "1", Space: "DEV"})
	_ = logger.Log(port.HistoryEntry{SessionID: "s1", Action: "updated", PageID: "2", Space: "OPS"})

	entries, err := logger.List("DEV", "", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry for space DEV, got %d", len(entries))
	}
}

func TestList_FilterBySession(t *testing.T) {
	logger, _ := newLogger(t)

	_ = logger.Log(port.HistoryEntry{SessionID: "sess-A", Action: "created", PageID: "1", Space: "DEV"})
	_ = logger.Log(port.HistoryEntry{SessionID: "sess-B", Action: "updated", PageID: "2", Space: "DEV"})

	entries, err := logger.List("", "sess-A", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry for session sess-A, got %d", len(entries))
	}
}

func TestList_Limit(t *testing.T) {
	logger, _ := newLogger(t)

	for i := 0; i < 5; i++ {
		_ = logger.Log(port.HistoryEntry{SessionID: "s", Action: "updated", PageID: "1", Space: "DEV"})
	}

	entries, err := logger.List("", "", 3)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries (limit), got %d", len(entries))
	}
}

func TestLog_Max1000(t *testing.T) {
	logger, dir := newLogger(t)

	for i := 0; i < 1005; i++ {
		_ = logger.Log(port.HistoryEntry{SessionID: "s", Action: "updated", PageID: "1", Space: "DEV"})
	}

	entries, err := logger.List("", "", 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) > 1000 {
		t.Errorf("expected at most 1000 entries, got %d", len(entries))
	}

	// history.json が存在することを確認
	if _, err := os.Stat(filepath.Join(dir, "history.json")); err != nil {
		t.Errorf("history.json not found: %v", err)
	}
}

func TestLog_Atomic(t *testing.T) {
	logger, dir := newLogger(t)
	_ = logger.Log(port.HistoryEntry{SessionID: "s", Action: "created", PageID: "1", Space: "DEV"})

	// 一時ファイルが残っていないことを確認
	tmpPath := filepath.Join(dir, ".history.json.tmp")
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("tmp file should not exist after successful write")
	}
}
