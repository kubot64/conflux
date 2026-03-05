package fileutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kubot64/conflux/internal/fileutil"
)

func TestAtomicWrite_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "test.json")

	data := []byte(`{"key":"value"}`)
	if err := fileutil.AtomicWrite(dest, data, 0600); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("got %q, want %q", got, data)
	}
}

func TestAtomicWrite_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "test.json")

	if err := fileutil.AtomicWrite(dest, []byte("hello"), 0600); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}

	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("permissions: got %o, want 0600", perm)
	}
}

func TestAtomicWrite_NoTmpFileLeft(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "test.json")

	if err := fileutil.AtomicWrite(dest, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "test.json" {
			t.Errorf("unexpected file: %s", e.Name())
		}
	}
}

func TestAtomicWrite_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "test.json")

	_ = fileutil.AtomicWrite(dest, []byte("old"), 0600)
	_ = fileutil.AtomicWrite(dest, []byte("new"), 0600)

	got, _ := os.ReadFile(dest)
	if string(got) != "new" {
		t.Errorf("got %q, want %q", got, "new")
	}
}
