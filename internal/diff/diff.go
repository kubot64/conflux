// Package diff は unified diff 生成を提供する。
// github.com/sergi/go-diff/diffmatchpatch を使い行単位の差分を計算する。
package diff

import (
	"fmt"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
)

const contextLines = 3

// Unified は before と after の unified diff 文字列を返す。
// nameA, nameB はファイル名ラベル（例: "current", "new"）。
// 差分がない場合は空文字列を返す。
func Unified(before, after, nameA, nameB string) string {
	dmp := diffmatchpatch.New()

	// 行単位の diff を計算
	aChars, bChars, lineArray := dmp.DiffLinesToChars(before, after)
	diffs := dmp.DiffMain(aChars, bChars, false)
	diffs = dmp.DiffCharsToLines(diffs, lineArray)

	// 行単位の操作リストに展開
	type lineOp struct {
		op   diffmatchpatch.Operation
		text string
	}
	var lines []lineOp
	for _, d := range diffs {
		parts := strings.Split(d.Text, "\n")
		for _, p := range parts {
			if p == "" {
				continue
			}
			lines = append(lines, lineOp{op: d.Type, text: p})
		}
	}

	n := len(lines)
	hasChange := false
	for _, l := range lines {
		if l.op != diffmatchpatch.DiffEqual {
			hasChange = true
			break
		}
	}
	if !hasChange {
		return ""
	}

	// 変更行 ± contextLines の範囲を include フラグで管理
	include := make([]bool, n)
	for i, l := range lines {
		if l.op != diffmatchpatch.DiffEqual {
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

		hunkStart := i
		hunkEnd := i
		for hunkEnd < n && include[hunkEnd] {
			hunkEnd++
		}

		// ハンク先頭の a/b オフセットを計算
		aStart, bStart := 0, 0
		for k := 0; k < hunkStart; k++ {
			switch lines[k].op {
			case diffmatchpatch.DiffEqual:
				aStart++
				bStart++
			case diffmatchpatch.DiffDelete:
				aStart++
			case diffmatchpatch.DiffInsert:
				bStart++
			}
		}

		aCount, bCount := 0, 0
		var hunkLines []string
		for k := hunkStart; k < hunkEnd; k++ {
			l := lines[k]
			switch l.op {
			case diffmatchpatch.DiffEqual:
				hunkLines = append(hunkLines, " "+l.text)
				aCount++
				bCount++
			case diffmatchpatch.DiffDelete:
				hunkLines = append(hunkLines, "-"+l.text)
				aCount++
			case diffmatchpatch.DiffInsert:
				hunkLines = append(hunkLines, "+"+l.text)
				bCount++
			}
		}

		fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", aStart+1, aCount, bStart+1, bCount)
		for _, l := range hunkLines {
			sb.WriteString(l)
			sb.WriteByte('\n')
		}

		i = hunkEnd
	}
	return sb.String()
}
