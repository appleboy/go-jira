package markdown

import (
	"testing"
)

func TestMarkdownToJira(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		want     string
	}{
		{
			name:     "simple paragraph",
			markdown: "This is a simple paragraph.",
			want:     "This is a simple paragraph.",
		},
		{
			name:     "heading",
			markdown: "# Heading 1",
			want:     "h1. Heading 1",
		},
		{
			name:     "bold text",
			markdown: "**bold text**",
			want:     "*bold text*",
		},
		{
			name:     "italic text",
			markdown: "*italic text*",
			want:     "_italic text_",
		},
		{
			name:     "link",
			markdown: "[example](http://example.com)",
			want:     "[example|http://example.com]",
		},
		{
			name:     "unordered list",
			markdown: "- item 1\n- item 2",
			want:     "* item 1\n* item 2",
		},
		{
			name:     "code block",
			markdown: "```go\ncode block\n```",
			want:     "{code:language=go}\ncode block\n{code}",
		},
		{
			name:     "inline code",
			markdown: "`inline code`",
			want:     "{{inline code}}",
		},
		{
			name:     "strikethrough",
			markdown: "~~strikethrough~~",
			want:     "-strikethrough-",
		},
		{
			name:     "custom comment",
			markdown: `🧑‍💻 [~appleboy] push code to repository {color:#ff8b00}**davinci/rag-service**{color} {color:#00875A}**refs/heads/GAIS-4223**{color} branch.\n\nSee the detailed information from [commit link](http://exampl.com).\n\nimprove logging and error handling for PDF page count validation`,
			want:     `🧑‍💻 [~appleboy] push code to repository {color:#ff8b00}*davinci/rag-service*{color} {color:#00875A}*refs/heads/GAIS-4223*{color} branch.\n\nSee the detailed information from [commit link|http://exampl.com].\n\nimprove logging and error handling for PDF page count validation`,
		},
		{
			name:     "code block and item list",
			markdown: "* item with [link](http://example.com)\n* item with `code`",
			want:     "* item with [link|http://example.com]\n* item with {{code}}",
		},
		{
			name:     "blod and item list",
			markdown: "* item with **bold** text\n* item with _italic_ text",
			want:     "* item with *bold* text\n* item with _italic_ text",
		},
		{
			name:     "nested list",
			markdown: "* item 1\n  * nested item\n* item 2",
			want:     "* item 1\n** nested item\n* item 2",
		},
		{
			name:     "item list",
			markdown: "* item 1\n* item 2",
			want:     "* item 1\n* item 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToJira(tt.markdown)
			if got != tt.want {
				t.Errorf("MarkdownToJira() = %v, want %v", got, tt.want)
			}
		})
	}
}

// BenchmarkMarkdownToJira benchmarks the MarkdownToJira function.
func BenchmarkMarkdownToJira(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	markdown := `
# Heading 1

This is a paragraph with **bold text** and _italic text_.

* List item 1
* List item 2
* List item 3

[Link](http://example.com)

> Blockquote

` + "```go" + `
func main() {
	fmt.Println("Hello, World!")
}
` + "```" + `
`
	for i := 0; i < b.N; i++ {
		ToJira(markdown)
	}
}
