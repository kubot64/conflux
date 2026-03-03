package validator

import (
	"fmt"
	"regexp"
	"strconv"
)

var spaceKeyRe = regexp.MustCompile(`^\S+$`)

// PageID は Confluence ページ ID の形式を検証する（正整数のみ）。
func PageID(id string) error {
	if id == "" {
		return fmt.Errorf("page ID must not be empty")
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return fmt.Errorf("page ID must be a numeric string, got %q", id)
	}
	if n <= 0 {
		return fmt.Errorf("page ID must be positive, got %q", id)
	}
	return nil
}

// SpaceKey はスペースキーの形式を検証する（空白を含まない非空文字列）。
func SpaceKey(key string) error {
	if key == "" {
		return fmt.Errorf("space key must not be empty")
	}
	if !spaceKeyRe.MatchString(key) {
		return fmt.Errorf("space key must not contain whitespace, got %q", key)
	}
	return nil
}
