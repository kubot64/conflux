package converter

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// macroTokenRE は Markdown 変換後に残るマクロ印を探す。
// 段落だけに印がある場合は <p> ごとマクロ要素へ戻す。
var macroTokenRE = regexp.MustCompile(`(?i)<p>\s*%%conflux-macro:([0-9a-f]+)%%\s*</p>|%%conflux-macro:([0-9a-f]+)%%`)

// Converter は port.Converter を実装する。
type Converter struct {
	md goldmark.Markdown
}

// New は Converter を生成する。
func New() *Converter {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
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
	return restoreMacroTokens(buf.String()), nil
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

	root := doc.Find("div").First()
	md, err := contentsToMarkdown(root)
	if err != nil {
		return "", err
	}
	md = strings.TrimSpace(md)
	if err := ensureMacrosPreserved(root, md); err != nil {
		return "", err
	}
	return md, nil
}

// contentsToMarkdown は要素の直下（テキストノードを含む）を Markdown にする。
func contentsToMarkdown(s *goquery.Selection) (string, error) {
	var sb strings.Builder
	var convErr error
	s.Contents().Each(func(_ int, n *goquery.Selection) {
		if convErr != nil {
			return
		}
		if goquery.NodeName(n) == "#text" {
			if t := strings.TrimSpace(n.Text()); t != "" {
				sb.WriteString(t)
				sb.WriteString("\n\n")
			}
			return
		}
		piece, err := nodeToMarkdown(n)
		if err != nil {
			convErr = err
			return
		}
		sb.WriteString(piece)
	})
	return sb.String(), convErr
}

// nodeToMarkdown は goquery Selection を Markdown 文字列に変換する。
func nodeToMarkdown(s *goquery.Selection) (string, error) {
	tag := strings.ToLower(goquery.NodeName(s))
	switch tag {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(tag[1] - '0')
		inner, err := inlineToMarkdown(s)
		if err != nil {
			return "", err
		}
		return strings.Repeat("#", level) + " " + strings.TrimSpace(inner) + "\n\n", nil
	case "p":
		inner, err := inlineToMarkdown(s)
		if err != nil {
			return "", err
		}
		return inner + "\n\n", nil
	case "ul":
		return listToMarkdown(s, false)
	case "ol":
		return listToMarkdown(s, true)
	case "pre":
		return codeBlockToMarkdown(s), nil
	case "blockquote":
		inner, err := contentsToMarkdown(s)
		if err != nil {
			return "", err
		}
		return prefixLines("> ", inner) + "\n", nil
	case "hr":
		return "---\n\n", nil
	case "table":
		return tableToMarkdown(s)
	case "div", "section", "article", "thead", "tbody", "tfoot":
		return contentsToMarkdown(s)
	default:
		if isMacroTag(tag) {
			return macroToken(s)
		}
		if s.Children().Length() > 0 {
			return contentsToMarkdown(s)
		}
		text := strings.TrimSpace(s.Text())
		if text == "" {
			return "", nil
		}
		return text + "\n\n", nil
	}
}

func listToMarkdown(s *goquery.Selection, ordered bool) (string, error) {
	var sb strings.Builder
	var convErr error
	idx := 0
	s.Children().Each(func(_ int, li *goquery.Selection) {
		if convErr != nil || goquery.NodeName(li) != "li" {
			return
		}
		idx++
		head, nested, err := liBody(li)
		if err != nil {
			convErr = err
			return
		}
		prefix := "- "
		if ordered {
			prefix = fmt.Sprintf("%d. ", idx)
		}
		sb.WriteString(prefix + head + "\n")
		if nested != "" {
			sb.WriteString(nested)
		}
	})
	if convErr != nil {
		return "", convErr
	}
	sb.WriteString("\n")
	return sb.String(), nil
}

func liBody(li *goquery.Selection) (string, string, error) {
	var head strings.Builder
	var nested strings.Builder
	var convErr error
	li.Contents().Each(func(_ int, n *goquery.Selection) {
		if convErr != nil {
			return
		}
		tag := strings.ToLower(goquery.NodeName(n))
		switch tag {
		case "ul", "ol":
			block, err := listToMarkdown(n, tag == "ol")
			if err != nil {
				convErr = err
				return
			}
			nested.WriteString(indentBlock(block, "  "))
		case "#text":
			head.WriteString(n.Text())
		default:
			if tag == "p" || tag == "div" {
				inner, err := inlineToMarkdown(n)
				if err != nil {
					convErr = err
					return
				}
				if head.Len() > 0 && strings.TrimSpace(inner) != "" {
					head.WriteByte(' ')
				}
				head.WriteString(strings.TrimSpace(inner))
				return
			}
			piece, err := inlineNode(n)
			if err != nil {
				convErr = err
				return
			}
			head.WriteString(piece)
		}
	})
	return strings.TrimSpace(head.String()), nested.String(), convErr
}

func codeBlockToMarkdown(s *goquery.Selection) string {
	code := s.Find("code").First()
	lang := ""
	text := s.Text()
	if code.Length() > 0 {
		text = code.Text()
		lang = codeLanguage(code)
		if lang == "" {
			lang = codeLanguage(s)
		}
	}
	return "```" + lang + "\n" + text + "\n```\n\n"
}

func codeLanguage(s *goquery.Selection) string {
	class, _ := s.Attr("class")
	const marker = "language-"
	if i := strings.Index(class, marker); i >= 0 {
		lang := class[i+len(marker):]
		if sp := strings.IndexAny(lang, " \t"); sp >= 0 {
			lang = lang[:sp]
		}
		return lang
	}
	return ""
}

func macroToken(s *goquery.Selection) (string, error) {
	html, err := goquery.OuterHtml(s)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(html) == "" {
		return "", nil
	}
	// goldmark は HTML コメントを落とす。プレーンテキストの印なら往復でき、
	// hex なので Markdown 上に script やコメント終端は出ない。
	return "%%conflux-macro:" + hex.EncodeToString([]byte(html)) + "%%\n\n", nil
}

func isMacroTag(tag string) bool {
	tag = strings.ToLower(tag)
	return strings.HasPrefix(tag, "ac:") || strings.HasPrefix(tag, "ri:")
}

func indentBlock(block, prefix string) string {
	if block == "" {
		return ""
	}
	lines := strings.Split(block, "\n")
	var sb strings.Builder
	for _, line := range lines {
		if line == "" {
			continue
		}
		sb.WriteString(prefix)
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	return sb.String()
}

func prefixLines(prefix, block string) string {
	block = strings.TrimSpace(block)
	if block == "" {
		return ""
	}
	lines := strings.Split(block, "\n")
	var sb strings.Builder
	for _, line := range lines {
		sb.WriteString(prefix)
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// inlineToMarkdown はインライン要素を含む Selection を Markdown に変換する。
func inlineToMarkdown(s *goquery.Selection) (string, error) {
	var sb strings.Builder
	var convErr error
	s.Contents().Each(func(_ int, n *goquery.Selection) {
		if convErr != nil {
			return
		}
		piece, err := inlineNode(n)
		if err != nil {
			convErr = err
			return
		}
		sb.WriteString(piece)
	})
	return sb.String(), convErr
}

func inlineNode(n *goquery.Selection) (string, error) {
	tag := strings.ToLower(goquery.NodeName(n))
	switch tag {
	case "#text":
		return n.Text(), nil
	case "strong", "b":
		inner, err := inlineToMarkdown(n)
		if err != nil {
			return "", err
		}
		return "**" + inner + "**", nil
	case "em", "i":
		inner, err := inlineToMarkdown(n)
		if err != nil {
			return "", err
		}
		return "*" + inner + "*", nil
	case "del", "s", "strike":
		inner, err := inlineToMarkdown(n)
		if err != nil {
			return "", err
		}
		return "~~" + inner + "~~", nil
	case "code":
		return "`" + n.Text() + "`", nil
	case "a":
		href, _ := n.Attr("href")
		inner, err := inlineToMarkdown(n)
		if err != nil {
			return "", err
		}
		return "[" + inner + "](" + href + ")", nil
	case "br":
		return "  \n", nil
	case "span":
		return inlineToMarkdown(n)
	default:
		if isMacroTag(tag) {
			tok, err := macroToken(n)
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(tok), nil
		}
		if n.Children().Length() > 0 {
			return inlineToMarkdown(n)
		}
		return n.Text(), nil
	}
}

func escapeTableCell(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

// tableToMarkdown は HTML テーブルを GFM テーブルに変換する。
func tableToMarkdown(s *goquery.Selection) (string, error) {
	var rows [][]string
	var convErr error
	s.Find("tr").Each(func(_ int, tr *goquery.Selection) {
		if convErr != nil {
			return
		}
		var row []string
		tr.Find("th, td").Each(func(_ int, cell *goquery.Selection) {
			if convErr != nil {
				return
			}
			inner, err := inlineToMarkdown(cell)
			if err != nil {
				convErr = err
				return
			}
			row = append(row, escapeTableCell(inner))
		})
		if len(row) > 0 {
			rows = append(rows, row)
		}
	})
	if convErr != nil {
		return "", convErr
	}
	if len(rows) == 0 {
		return "", nil
	}

	var sb strings.Builder
	sb.WriteString("| " + strings.Join(rows[0], " | ") + " |\n")
	seps := make([]string, len(rows[0]))
	for i := range seps {
		seps[i] = "---"
	}
	sb.WriteString("| " + strings.Join(seps, " | ") + " |\n")
	for _, row := range rows[1:] {
		sb.WriteString("| " + strings.Join(row, " | ") + " |\n")
	}
	sb.WriteString("\n")
	return sb.String(), nil
}

func ensureMacrosPreserved(root *goquery.Selection, markdown string) error {
	want := countExposedMacros(root)
	got := len(macroTokenRE.FindAllString(markdown, -1))
	if want != got {
		return fmt.Errorf("storage to markdown: %d macro element(s) would be dropped", want-got)
	}
	return nil
}

func countExposedMacros(s *goquery.Selection) int {
	n := 0
	s.Children().Each(func(_ int, c *goquery.Selection) {
		if isMacroTag(goquery.NodeName(c)) {
			n++
			return
		}
		n += countExposedMacros(c)
	})
	return n
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

func restoreMacroTokens(html string) string {
	return macroTokenRE.ReplaceAllStringFunc(html, func(match string) string {
		sub := macroTokenRE.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		encoded := sub[1]
		if encoded == "" {
			encoded = sub[2]
		}
		restored, ok := sanitizeMacroHTML(encoded)
		if !ok {
			return match
		}
		return restored
	})
}

// sanitizeMacroHTML は印の中身を ac:/ri: 要素だけに戻す。
// 兄弟要素の script やイベント属性は storage に載せない。
func sanitizeMacroHTML(encoded string) (string, bool) {
	raw, err := hex.DecodeString(encoded)
	if err != nil {
		return "", false
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(raw)))
	if err != nil {
		return "", false
	}
	var b strings.Builder
	doc.Find("body").Children().Each(func(_ int, s *goquery.Selection) {
		tag := strings.ToLower(goquery.NodeName(s))
		if !strings.HasPrefix(tag, "ac:") && !strings.HasPrefix(tag, "ri:") {
			return
		}
		s.Find("script,style,iframe,object,embed,link,meta").Remove()
		s.Find("*").Each(func(_ int, n *goquery.Selection) {
			if len(n.Nodes) == 0 {
				return
			}
			var drop []string
			for _, a := range n.Nodes[0].Attr {
				if strings.HasPrefix(strings.ToLower(a.Key), "on") {
					drop = append(drop, a.Key)
				}
			}
			for _, key := range drop {
				n.RemoveAttr(key)
			}
		})
		fragment, err := goquery.OuterHtml(s)
		if err == nil {
			b.WriteString(fragment)
		}
	})
	if b.Len() == 0 {
		return "", false
	}
	return b.String(), true
}

func isHeading(tag string) bool {
	switch tag {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		return true
	}
	return false
}
