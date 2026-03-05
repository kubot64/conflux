package diff

import (
	"strings"
	"testing"
)

func TestUnified_NoChange(t *testing.T) {
	got := Unified("line1\nline2\n", "line1\nline2\n", "a", "b")
	if got != "" {
		t.Errorf("expected empty string for no change, got: %q", got)
	}
}

func TestUnified_SimpleDiff(t *testing.T) {
	before := "line1\nold line\nline3\n"
	after := "line1\nnew line\nline3\n"
	got := Unified(before, after, "before", "after")

	if !strings.HasPrefix(got, "--- before\n+++ after\n") {
		t.Errorf("missing header:\n%s", got)
	}
	if !strings.Contains(got, "-old line") {
		t.Errorf("missing deletion:\n%s", got)
	}
	if !strings.Contains(got, "+new line") {
		t.Errorf("missing insertion:\n%s", got)
	}
}

func TestUnified_Insert(t *testing.T) {
	before := "line1\nline2\n"
	after := "line1\nnew line\nline2\n"
	got := Unified(before, after, "a", "b")

	if !strings.Contains(got, "+new line") {
		t.Errorf("missing insertion:\n%s", got)
	}
}

func TestUnified_Delete(t *testing.T) {
	before := "line1\nremove me\nline2\n"
	after := "line1\nline2\n"
	got := Unified(before, after, "a", "b")

	if !strings.Contains(got, "-remove me") {
		t.Errorf("missing deletion:\n%s", got)
	}
}

func TestUnified_ContextLines(t *testing.T) {
	// 変更箇所が離れている場合、別ハンクになることを確認
	var bld strings.Builder
	for i := 0; i < 10; i++ {
		bld.WriteString("context\n")
	}
	before := bld.String()
	lines := strings.Split(before, "\n")
	lines[0] = "changed"
	lines[9] = "also changed"
	after := strings.Join(lines, "\n")

	got := Unified(before, after, "a", "b")
	// 2つの @@ ハンクが生成されるはず
	hunkCount := strings.Count(got, "@@")
	if hunkCount < 2 {
		t.Errorf("expected at least 2 hunks, got %d:\n%s", hunkCount/2, got)
	}
}
