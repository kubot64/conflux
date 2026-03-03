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
