package alias_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kubot64/conflux/internal/alias"
	"github.com/kubot64/conflux/internal/apperror"
	"github.com/kubot64/conflux/internal/port"
)

func newStore(t *testing.T) *alias.Store {
	t.Helper()
	store, err := alias.NewStore(filepath.Join(t.TempDir(), "alias.json"))
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestSet_Get(t *testing.T) {
	store := newStore(t)

	if err := store.Set("home", "12345", port.AliasPage); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get("home")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "home" || got.Target != "12345" || got.Type != port.AliasPage {
		t.Errorf("unexpected alias: %+v", got)
	}
}

func TestSet_Overwrite(t *testing.T) {
	store := newStore(t)

	if err := store.Set("myspace", "OLD", port.AliasSpace); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("myspace", "NEW", port.AliasSpace); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Get("myspace")
	if got.Target != "NEW" {
		t.Errorf("expected target NEW, got %s", got.Target)
	}
}

func TestGet_NotFound(t *testing.T) {
	store := newStore(t)

	_, err := store.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var appErr *apperror.AppError
	if e, ok := err.(*apperror.AppError); !ok || e.Kind != apperror.KindNotFound {
		t.Errorf("expected KindNotFound error, got %v", appErr)
	}
}

func TestList(t *testing.T) {
	store := newStore(t)

	if err := store.Set("a", "111", port.AliasPage); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("b", "MS", port.AliasSpace); err != nil {
		t.Fatal(err)
	}

	aliases, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 2 {
		t.Fatalf("expected 2 aliases, got %d", len(aliases))
	}
}

func TestList_Empty(t *testing.T) {
	store := newStore(t)

	aliases, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 0 {
		t.Errorf("expected 0 aliases, got %d", len(aliases))
	}
}

func TestDelete(t *testing.T) {
	store := newStore(t)

	if err := store.Set("a", "111", port.AliasPage); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("b", "222", port.AliasPage); err != nil {
		t.Fatal(err)
	}

	if err := store.Delete("a"); err != nil {
		t.Fatal(err)
	}

	aliases, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 1 || aliases[0].Name != "b" {
		t.Errorf("expected only alias 'b', got %+v", aliases)
	}
}

func TestDelete_NotFound(t *testing.T) {
	store := newStore(t)

	err := store.Delete("nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if e, ok := err.(*apperror.AppError); !ok || e.Kind != apperror.KindNotFound {
		t.Errorf("expected KindNotFound error, got %v", err)
	}
}

func TestSave_NoPredictableTmpFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "alias.json")

	store, _ := alias.NewStore(path)
	if err := store.Set("a", "111", port.AliasPage); err != nil {
		t.Fatal(err)
	}

	// predictable な .tmp ファイルが残っていないこと
	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("predictable .tmp file should not exist")
	}

	// alias.json は存在する
	if _, err := os.Stat(path); err != nil {
		t.Errorf("alias.json should exist: %v", err)
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "alias.json")

	s1, _ := alias.NewStore(path)
	if err := s1.Set("home", "12345", port.AliasPage); err != nil {
		t.Fatal(err)
	}

	// 別インスタンスで読み込み
	s2, _ := alias.NewStore(path)
	got, err := s2.Get("home")
	if err != nil {
		t.Fatal(err)
	}
	if got.Target != "12345" {
		t.Errorf("expected target 12345, got %s", got.Target)
	}
}
