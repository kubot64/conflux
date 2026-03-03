package converter_test

import (
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

func TestStorageToMarkdown_MacroPreserved(t *testing.T) {
	c := newConverter()
	macro := `<ac:structured-macro ac:name="info"><ac:rich-text-body><p>note</p></ac:rich-text-body></ac:structured-macro>`
	out, err := c.StorageToMarkdown(macro)
	if err != nil {
		t.Fatal(err)
	}
	// マクロは <!-- macro: ... --> コメントとして保持される
	if !strings.Contains(out, "<!-- macro:") {
		t.Errorf("expected macro comment in output, got: %s", out)
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
