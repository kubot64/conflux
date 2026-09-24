package cmd

import (
	"strings"
	"unicode/utf8"
)

// applyMaxChars は本文を maxChars 以内に収める。
// 収まる場合はそのまま返す。超える場合は上限より前の最後の段落区切りで切る。
func applyMaxChars(body string, maxChars int) (string, bool, int) {
	total := utf8.RuneCountInString(body)
	if maxChars <= 0 || total <= maxChars {
		return body, false, total
	}
	runes := []rune(body)
	window := string(runes[:maxChars])
	cut := maxChars
	if idx := strings.LastIndex(window, "\n\n"); idx > 0 {
		cut = utf8.RuneCountInString(window[:idx])
	}
	if cut <= 0 {
		cut = maxChars
	}
	return string(runes[:cut]), true, total
}
