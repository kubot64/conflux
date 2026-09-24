package converter_test

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/kubot64/conflux/internal/converter"
)

func newConverter() *converter.Converter {
	return converter.New()
}

// --- MarkdownToStorage ---

func TestMarkdownToStorage_Paragraph(t *testing.T) {
	c := newConverter()
	out, err := c.MarkdownToStorage("Hello world")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "<p>Hello world</p>") {
		t.Errorf("expected <p>Hello world</p>, got: %s", out)
	}
}

func TestMarkdownToStorage_Heading(t *testing.T) {
	c := newConverter()
	out, err := c.MarkdownToStorage("# Title\n\nparagraph")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "<h1>Title</h1>") {
		t.Errorf("expected <h1>Title</h1>, got: %s", out)
	}
}

func TestMarkdownToStorage_Bold(t *testing.T) {
	c := newConverter()
	out, err := c.MarkdownToStorage("**bold** text")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "<strong>bold</strong>") {
		t.Errorf("expected <strong>bold</strong>, got: %s", out)
	}
}

func TestMarkdownToStorage_CodeBlock(t *testing.T) {
	c := newConverter()
	md := "```go\nfmt.Println(\"hello\")\n```"
	out, err := c.MarkdownToStorage(md)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "code") {
		t.Errorf("expected code block in output, got: %s", out)
	}
}

func TestMarkdownToStorage_Link(t *testing.T) {
	c := newConverter()
	out, err := c.MarkdownToStorage("[Click](https://example.com)")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `href="https://example.com"`) {
		t.Errorf("expected href, got: %s", out)
	}
}

// --- StorageToMarkdown ---

func TestStorageToMarkdown_Paragraph(t *testing.T) {
	c := newConverter()
	out, err := c.StorageToMarkdown("<p>Hello world</p>")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Hello world") {
		t.Errorf("expected 'Hello world', got: %s", out)
	}
}

func TestStorageToMarkdown_Heading(t *testing.T) {
	c := newConverter()
	out, err := c.StorageToMarkdown("<h1>Title</h1><p>body</p>")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "# Title") {
		t.Errorf("expected '# Title', got: %s", out)
	}
}

func TestStorageToMarkdown_Bold(t *testing.T) {
	c := newConverter()
	out, err := c.StorageToMarkdown("<p><strong>bold</strong> text</p>")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "**bold**") {
		t.Errorf("expected '**bold**', got: %s", out)
	}
}

func TestStorageToMarkdown_Link(t *testing.T) {
	c := newConverter()
	out, err := c.StorageToMarkdown(`<p><a href="https://example.com">Click</a></p>`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[Click](https://example.com)") {
		t.Errorf("expected markdown link, got: %s", out)
	}
}

func TestMacroRoundTrip_RestoresStructuredMacro(t *testing.T) {
	c := newConverter()
	macro := `<ac:structured-macro ac:name="info"><ac:rich-text-body><p>note</p></ac:rich-text-body></ac:structured-macro>`
	md, err := c.StorageToMarkdown(macro)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(md, "<script") || strings.Contains(md, "onerror") {
		t.Fatalf("markdown must not contain raw markup, got: %s", md)
	}
	storage, err := c.MarkdownToStorage(md)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(storage, `ac:name="info"`) {
		t.Fatalf("round-trip dropped macro, storage: %s\nmarkdown: %s", storage, md)
	}
}

func TestMarkdownToStorage_MacroToken_DropsForeignHTML(t *testing.T) {
	c := newConverter()
	payload := `<ac:structured-macro ac:name="info"><ac:rich-text-body><p>note</p><script>alert(1)</script><img src=x onerror="alert(1)"></ac:rich-text-body></ac:structured-macro><script>alert(2)</script>`
	md := "%%conflux-macro:" + hex.EncodeToString([]byte(payload)) + "%%"
	out, err := c.MarkdownToStorage(md)
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(out)
	if strings.Contains(lower, "<script") || strings.Contains(lower, "onerror") {
		t.Fatalf("restored macro must not contain script or onerror, got: %s", out)
	}
	if !strings.Contains(out, `ac:name="info"`) || !strings.Contains(out, "note") {
		t.Fatalf("expected macro body to remain, got: %s", out)
	}
}

func TestStorageToMarkdown_MacroPreserved(t *testing.T) {
	c := newConverter()
	macro := `<ac:structured-macro ac:name="info"><ac:rich-text-body><p>note</p></ac:rich-text-body></ac:structured-macro>`
	out, err := c.StorageToMarkdown(macro)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "conflux-macro:") {
		t.Errorf("expected macro token in output, got: %s", out)
	}
}

// 悪意のある Confluence ページが script タグ入りの疑似マクロを返した場合、
// 生の <script> タグが出力マークダウンに残ってはならない。
func TestStorageToMarkdown_Macro_StripsScriptTag(t *testing.T) {
	c := newConverter()
	macro := `<ac:structured-macro ac:name="info"><ac:rich-text-body><script>alert(1)</script></ac:rich-text-body></ac:structured-macro>`
	out, err := c.StorageToMarkdown(macro)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "<script>") || strings.Contains(strings.ToLower(out), "<script") {
		t.Errorf("sanitized macro must not contain <script>, got: %s", out)
	}
}

// HTML コメント中で "--" が現れるとコメント終端 "-->" を偽装してブレークアウト
// できるため、マクロ内容の "--" は無害化されなければならない。
func TestStorageToMarkdown_Macro_NeutralizesCommentBreakout(t *testing.T) {
	c := newConverter()
	macro := `<ac:structured-macro ac:name="x">payload --> <script>alert(1)</script></ac:structured-macro>`
	out, err := c.StorageToMarkdown(macro)
	if err != nil {
		t.Fatal(err)
	}
	// --> を含む文字列がマクロコメント内に残ってはならない
	if strings.Contains(out, "-->") {
		// 最後の1件（自前のコメント終端）は許容されるため、
		// 終端以外に "-->" が存在する場合のみエラー
		if strings.Count(out, "-->") > 1 {
			t.Errorf("macro content must not introduce '-->' sequence, got: %s", out)
		}
	}
}

func TestStorageToMarkdown_Macro_StripsOnErrorAttribute(t *testing.T) {
	c := newConverter()
	macro := `<ac:structured-macro ac:name="info"><ac:rich-text-body><img src=x onerror="alert(1)"></ac:rich-text-body></ac:structured-macro>`
	out, err := c.StorageToMarkdown(macro)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(out), "onerror") {
		t.Errorf("sanitized macro must not retain onerror attribute, got: %s", out)
	}
}

// --- XSS 対策: raw HTML はエスケープされる ---

func TestMarkdownToStorage_RawHTML_Escaped(t *testing.T) {
	c := newConverter()
	// raw HTML (script タグ) がそのまま出力されないこと
	md := `Hello <script>alert("xss")</script> world`
	out, err := c.MarkdownToStorage(md)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "<script>") {
		t.Errorf("raw <script> tag should be escaped, got: %s", out)
	}
	// テキスト部分は保持される
	if !strings.Contains(out, "Hello") || !strings.Contains(out, "world") {
		t.Errorf("expected text preserved, got: %s", out)
	}
}

func TestMarkdownToStorage_RawHTML_ImgOnError_Escaped(t *testing.T) {
	c := newConverter()
	md := `<img src=x onerror=alert(1)>`
	out, err := c.MarkdownToStorage(md)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "onerror") {
		t.Errorf("onerror attribute should be escaped, got: %s", out)
	}
}

func TestMarkdownToStorage_RawHTML_Iframe_Escaped(t *testing.T) {
	c := newConverter()
	md := `<iframe src="https://evil.com"></iframe>`
	out, err := c.MarkdownToStorage(md)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "<iframe") {
		t.Errorf("iframe tag should be escaped, got: %s", out)
	}
}

// --- ExtractSection ---

func TestExtractSection_ByHeading(t *testing.T) {
	c := newConverter()
	storage := `<h1>Introduction</h1><p>intro text</p><h1>Details</h1><p>detail text</p>`
	out, err := c.ExtractSection(storage, "Introduction")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "intro text") {
		t.Errorf("expected 'intro text', got: %s", out)
	}
	if strings.Contains(out, "detail text") {
		t.Errorf("should not contain 'detail text', got: %s", out)
	}
}

func TestExtractSection_NotFound(t *testing.T) {
	c := newConverter()
	_, err := c.ExtractSection("<h1>Title</h1><p>text</p>", "NonExistent")
	if err == nil {
		t.Fatal("expected error for missing section, got nil")
	}
}

func TestStorageToMarkdown_HeadingKeepsInline(t *testing.T) {
	c := newConverter()
	out, err := c.StorageToMarkdown(`<h2>Hello <strong>world</strong></h2>`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "## Hello **world**") {
		t.Fatalf("heading dropped inline mark: %s", out)
	}
}

func TestStorageToMarkdown_NestedList(t *testing.T) {
	c := newConverter()
	out, err := c.StorageToMarkdown(`<ul><li>a<ul><li>b</li></ul></li></ul>`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "- a\n  - b") {
		t.Fatalf("nested list flattened: %q", out)
	}
}

func TestStorageToMarkdown_CodeLanguage(t *testing.T) {
	c := newConverter()
	out, err := c.StorageToMarkdown(`<pre><code class="language-go">fmt.Println()</code></pre>`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "```go\nfmt.Println()") {
		t.Fatalf("code language dropped: %q", out)
	}
}

func TestStorageToMarkdown_TableKeepsCellMarkup(t *testing.T) {
	c := newConverter()
	out, err := c.StorageToMarkdown(`<table><tr><th>a|b</th></tr><tr><td><strong>c</strong></td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `a\|b`) || !strings.Contains(out, "**c**") {
		t.Fatalf("table cell lost markup or pipe: %q", out)
	}
}

func TestMacroInsideParagraph_RoundTrip(t *testing.T) {
	c := newConverter()
	storage := `<p>before <ac:structured-macro ac:name="status"></ac:structured-macro> after</p>`
	md, err := c.StorageToMarkdown(storage)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md, "conflux-macro:") || !strings.Contains(md, "before") || !strings.Contains(md, "after") {
		t.Fatalf("inline macro dropped: %s", md)
	}
	back, err := c.MarkdownToStorage(md)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(back, `ac:name="status"`) {
		t.Fatalf("inline macro not restored: %s\nmarkdown: %s", back, md)
	}
}
