package cmd

import "testing"

func TestApplyMaxChars_CutsOnParagraph(t *testing.T) {
	body := "para-one\n\npara-two"
	got, truncated, total := applyMaxChars(body, 12)
	if total != len([]rune(body)) {
		t.Fatalf("total: got %d", total)
	}
	if !truncated {
		t.Fatal("expected truncated")
	}
	if got != "para-one" {
		t.Fatalf("got %q", got)
	}
}

func TestApplyMaxChars_NoCutWhenShort(t *testing.T) {
	got, truncated, total := applyMaxChars("abc", 10)
	if truncated || got != "abc" || total != 3 {
		t.Fatalf("got %q truncated %v total %d", got, truncated, total)
	}
}
