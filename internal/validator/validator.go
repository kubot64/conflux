package validator

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxTitleLength は Confluence ページタイトルの最大文字数。
// Confluence Server は 255 文字を上限としている。
const MaxTitleLength = 255

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

// Title はページタイトルの形式を検証する（最大 255 文字、制御文字禁止）。
// 改行・タブ・NUL 等の制御文字は Confluence の REST API 側で 400 を引き起こすほか、
// JSON envelope 出力やログ整形で破損の原因になるためクライアント側で拒否する。
func Title(title string) error {
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return fmt.Errorf("title must not be empty or whitespace only")
	}
	if utf8.RuneCountInString(title) > MaxTitleLength {
		return fmt.Errorf("title must be at most %d characters, got %d", MaxTitleLength, utf8.RuneCountInString(title))
	}
	for _, r := range title {
		if unicode.IsControl(r) {
			return fmt.Errorf("title must not contain control characters (e.g. newline, tab, null)")
		}
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
