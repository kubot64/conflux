package converter

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

// Converter は port.Converter を実装する。
type Converter struct {
	md goldmark.Markdown
}

// New は Converter を生成する。
func New() *Converter {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Table,
		),
		goldmark.WithRendererOptions(
			html.WithUnsafe(),
		),
	)
	return &Converter{md: md}
}

// MarkdownToStorage は GFM を Confluence XHTML storage 形式に変換する。
func (c *Converter) MarkdownToStorage(markdown string) (string, error) {
	var buf bytes.Buffer
	if err := c.md.Convert([]byte(markdown), &buf); err != nil {
		return "", fmt.Errorf("markdown to storage: %w", err)
	}
	return buf.String(), nil
}

// StorageToMarkdown は Confluence XHTML storage 形式を GFM に変換する。
// Confluence マクロ（ac:structured-macro）は <!-- macro: ... --> コメントとして保持する。
func (c *Converter) StorageToMarkdown(storage string) (string, error) {
	// ac:* 要素は goquery が扱えるようラップしてパース
	wrapped := "<div>" + storage + "</div>"
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(wrapped))
	if err != nil {
		return "", fmt.Errorf("storage to markdown: %w", err)
	}

	var sb strings.Builder
	doc.Find("div").First().Children().Each(func(_ int, s *goquery.Selection) {
		sb.WriteString(nodeToMarkdown(s))
	})
	return strings.TrimSpace(sb.String()), nil
}

// nodeToMarkdown は goquery Selection を Markdown 文字列に変換する。
func nodeToMarkdown(s *goquery.Selection) string {
	tag := goquery.NodeName(s)
	switch tag {
	case "h1":
		return "# " + s.Text() + "\n\n"
	case "h2":
		return "## " + s.Text() + "\n\n"
	case "h3":
		return "### " + s.Text() + "\n\n"
	case "h4":
		return "#### " + s.Text() + "\n\n"
	case "h5":
		return "##### " + s.Text() + "\n\n"
	case "h6":
		return "###### " + s.Text() + "\n\n"
	case "p":
		return inlineToMarkdown(s) + "\n\n"
	case "ul":
		var sb strings.Builder
		s.Children().Each(func(_ int, li *goquery.Selection) {
			sb.WriteString("- " + inlineToMarkdown(li) + "\n")
		})
		sb.WriteString("\n")
		return sb.String()
	case "ol":
		var sb strings.Builder
		s.Children().Each(func(i int, li *goquery.Selection) {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, inlineToMarkdown(li)))
		})
		sb.WriteString("\n")
		return sb.String()
	case "pre":
		code := s.Find("code")
		if code.Length() > 0 {
			return "```\n" + code.Text() + "\n```\n\n"
		}
		return "```\n" + s.Text() + "\n```\n\n"
	case "blockquote":
		lines := strings.Split(strings.TrimSpace(s.Text()), "\n")
		var sb strings.Builder
		for _, l := range lines {
			sb.WriteString("> " + l + "\n")
		}
		sb.WriteString("\n")
		return sb.String()
	case "hr":
		return "---\n\n"
	case "table":
		return tableToMarkdown(s)
	default:
		// Confluence マクロ（ac:structured-macro, ac:image など）はコメントとして保持
		if strings.HasPrefix(tag, "ac:") || strings.HasPrefix(tag, "ri:") {
			html, _ := goquery.OuterHtml(s)
			return "<!-- macro: " + strings.TrimSpace(html) + " -->\n\n"
		}
		// その他はテキストのみ抽出
		text := strings.TrimSpace(s.Text())
		if text == "" {
			return ""
		}
		return text + "\n\n"
	}
}

// inlineToMarkdown はインライン要素を含む Selection を Markdown に変換する。
func inlineToMarkdown(s *goquery.Selection) string {
	var sb strings.Builder
	s.Contents().Each(func(_ int, n *goquery.Selection) {
		tag := goquery.NodeName(n)
		switch tag {
		case "#text":
			sb.WriteString(n.Text())
		case "strong", "b":
			sb.WriteString("**" + n.Text() + "**")
		case "em", "i":
			sb.WriteString("*" + n.Text() + "*")
		case "code":
			sb.WriteString("`" + n.Text() + "`")
		case "a":
			href, _ := n.Attr("href")
			sb.WriteString("[" + n.Text() + "](" + href + ")")
		case "br":
			sb.WriteString("  \n")
		default:
			sb.WriteString(n.Text())
		}
	})
	return sb.String()
}

// tableToMarkdown は HTML テーブルを GFM テーブルに変換する。
func tableToMarkdown(s *goquery.Selection) string {
	var rows [][]string
	s.Find("tr").Each(func(_ int, tr *goquery.Selection) {
		var row []string
		tr.Find("th,td").Each(func(_ int, cell *goquery.Selection) {
			row = append(row, strings.TrimSpace(cell.Text()))
		})
		if len(row) > 0 {
			rows = append(rows, row)
		}
	})
	if len(rows) == 0 {
		return ""
	}

	var sb strings.Builder
	// ヘッダ行
	sb.WriteString("| " + strings.Join(rows[0], " | ") + " |\n")
	// セパレータ
	seps := make([]string, len(rows[0]))
	for i := range seps {
		seps[i] = "---"
	}
	sb.WriteString("| " + strings.Join(seps, " | ") + " |\n")
	// データ行
	for _, row := range rows[1:] {
		sb.WriteString("| " + strings.Join(row, " | ") + " |\n")
	}
	sb.WriteString("\n")
	return sb.String()
}

// ExtractSection は storage XHTML から指定ヘッダ ID のセクションを抽出する。
// sectionID はヘッダテキストと一致させる。
func (c *Converter) ExtractSection(storage, sectionID string) (string, error) {
	wrapped := "<div>" + storage + "</div>"
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(wrapped))
	if err != nil {
		return "", fmt.Errorf("extract section: %w", err)
	}

	headingTags := []string{"h1", "h2", "h3", "h4", "h5", "h6"}

	// sectionID に一致するヘッダを探す
	var startSel *goquery.Selection
	var startLevel int
	doc.Find(strings.Join(headingTags, ",")).Each(func(_ int, s *goquery.Selection) {
		if startSel != nil {
			return
		}
		if strings.TrimSpace(s.Text()) == sectionID {
			startSel = s
			tag := goquery.NodeName(s)
			fmt.Sscanf(tag[1:], "%d", &startLevel)
		}
	})

	if startSel == nil {
		return "", fmt.Errorf("section %q not found", sectionID)
	}

	// セクション内容を収集（次の同レベル以上のヘッダまで）
	var sb strings.Builder
	for cur := startSel.Next(); cur.Length() > 0; cur = cur.Next() {
		tag := goquery.NodeName(cur)
		if isHeading(tag) {
			level := 0
			fmt.Sscanf(tag[1:], "%d", &level)
			if level <= startLevel {
				break
			}
		}
		html, _ := goquery.OuterHtml(cur)
		sb.WriteString(html)
	}

	return sb.String(), nil
}

func isHeading(tag string) bool {
	switch tag {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		return true
	}
	return false
}
