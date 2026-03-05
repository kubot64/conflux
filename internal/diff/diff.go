// Package diff は unified diff 生成を提供する。
package diff

import (
	"fmt"
	"strings"
)

const contextLines = 3

// Unified は before と after の unified diff 文字列を返す。
// nameA, nameB はファイル名ラベル（例: "before", "after"）。
// 差分がない場合は空文字列を返す。
func Unified(before, after, nameA, nameB string) string {
	aLines := splitLines(before)
	bLines := splitLines(after)
	edits := computeEdits(aLines, bLines)
	return formatUnified(edits, aLines, bLines, nameA, nameB)
}

type editKind int

const (
	editEqual  editKind = iota
	editDelete          // aLines のみに存在
	editInsert          // bLines のみに存在
)

type edit struct {
	kind editKind
	aIdx int // aLines インデックス（Equal/Delete）
	bIdx int // bLines インデックス（Equal/Insert）
}

// computeEdits は LCS を用いて編集操作列を返す。
func computeEdits(a, b []string) []edit {
	m, n := len(a), len(b)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}

	// バックトレースで編集列を構築（逆順で追加してから反転）
	var edits []edit
	i, j := m, n
	for i > 0 || j > 0 {
		switch {
		case i > 0 && j > 0 && a[i-1] == b[j-1]:
			edits = append(edits, edit{editEqual, i - 1, j - 1})
			i--
			j--
		case j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]):
			edits = append(edits, edit{editInsert, -1, j - 1})
			j--
		default:
			edits = append(edits, edit{editDelete, i - 1, -1})
			i--
		}
	}
	for l, r := 0, len(edits)-1; l < r; l, r = l+1, r-1 {
		edits[l], edits[r] = edits[r], edits[l]
	}
	return edits
}

// formatUnified は編集列を unified diff 形式の文字列に変換する。
func formatUnified(edits []edit, a, b []string, nameA, nameB string) string {
	n := len(edits)
	hasChange := false
	for _, e := range edits {
		if e.kind != editEqual {
			hasChange = true
			break
		}
	}
	if !hasChange {
		return ""
	}

	// 各編集が出力に含まれるか（変更行 ± contextLines の範囲）
	include := make([]bool, n)
	for i, e := range edits {
		if e.kind != editEqual {
			lo := i - contextLines
			if lo < 0 {
				lo = 0
			}
			hi := i + contextLines
			if hi >= n {
				hi = n - 1
			}
			for k := lo; k <= hi; k++ {
				include[k] = true
			}
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "--- %s\n+++ %s\n", nameA, nameB)

	i := 0
	for i < n {
		if !include[i] {
			i++
			continue
		}
		// ハンク開始: include が true の連続範囲を収集
		hunkStart := i
		hunkEnd := i
		for hunkEnd < n && include[hunkEnd] {
			hunkEnd++
		}

		// ハンク先頭の a/b オフセットを計算
		aStart, bStart := 0, 0
		for k := 0; k < hunkStart; k++ {
			switch edits[k].kind {
			case editEqual:
				aStart++
				bStart++
			case editDelete:
				aStart++
			case editInsert:
				bStart++
			}
		}

		// ハンク内の行を構築
		aCount, bCount := 0, 0
		var lines []string
		for k := hunkStart; k < hunkEnd; k++ {
			e := edits[k]
			switch e.kind {
			case editEqual:
				lines = append(lines, " "+a[e.aIdx])
				aCount++
				bCount++
			case editDelete:
				lines = append(lines, "-"+a[e.aIdx])
				aCount++
			case editInsert:
				lines = append(lines, "+"+b[e.bIdx])
				bCount++
			}
		}

		fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", aStart+1, aCount, bStart+1, bCount)
		for _, l := range lines {
			sb.WriteString(l)
			sb.WriteByte('\n')
		}

		i = hunkEnd
	}
	return sb.String()
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
