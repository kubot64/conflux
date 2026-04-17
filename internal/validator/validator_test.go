package validator_test

import (
	"testing"

	"github.com/kubot64/conflux/internal/validator"
)

func TestPageID(t *testing.T) {
	tests := []struct {
		id      string
		wantErr bool
	}{
		{"12345", false},
		{"1", false},
		{"", true},
		{"abc", true},
		{"123abc", true},
		{"0", true},
	}
	for _, tt := range tests {
		err := validator.PageID(tt.id)
		if (err != nil) != tt.wantErr {
			t.Errorf("PageID(%q): wantErr=%v, got err=%v", tt.id, tt.wantErr, err)
		}
	}
}

func TestTitle(t *testing.T) {
	tests := []struct {
		name    string
		title   string
		wantErr bool
	}{
		{"normal ascii", "Hello World", false},
		{"japanese", "テストページ", false},
		{"max length 255", string(make([]rune, 255)) + "", false},
		{"empty", "", true},
		{"only whitespace", "   ", true},
		{"too long 256", string(make([]rune, 256)) + "a", true},
		{"contains newline", "title\nwith newline", true},
		{"contains tab", "title\twith tab", true},
		{"contains null", "title\x00null", true},
		{"contains carriage return", "title\rwith CR", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// "max length 255" は 255 個の \x00 になってしまうので作り直す
			title := tt.title
			if tt.name == "max length 255" {
				title = ""
				for i := 0; i < 255; i++ {
					title += "a"
				}
			}
			err := validator.Title(title)
			if (err != nil) != tt.wantErr {
				t.Errorf("Title(%q): wantErr=%v, got err=%v", tt.title, tt.wantErr, err)
			}
		})
	}
}

func TestSpaceKey(t *testing.T) {
	tests := []struct {
		key     string
		wantErr bool
	}{
		{"DEV", false},
		{"MYSPACE", false},
		{"my-space", false},
		{"~user", false},
		{"", true},
		{"has space", true},
	}
	for _, tt := range tests {
		err := validator.SpaceKey(tt.key)
		if (err != nil) != tt.wantErr {
			t.Errorf("SpaceKey(%q): wantErr=%v, got err=%v", tt.key, tt.wantErr, err)
		}
	}
}
