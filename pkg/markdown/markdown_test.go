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
			markdown: `🧑‍💻 @appleboy push code to repository {color:#ff8b00}**davinci/rag-service**{color} {color:#00875A}**refs/heads/GAIS-4223**{color} branch.\n\nSee the detailed information from [commit link](http://exampl.com).\n\nimprove logging and error handling for PDF page count validation`,
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

func TestConvertMentions(t *testing.T) {
	r := NewJiraRenderer()
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "mention with punctuation",
			text: "Hello @user! How are you?",
			want: "Hello [~user]! How are you?",
		},
		{
			name: "mention with multiple punctuation",
			text: "Hey @user!?",
			want: "Hey [~user]!?",
		},
		{
			name: "single mention",
			text: "Hello @user!",
			want: "Hello [~user]!",
		},
		{
			name: "multiple mentions",
			text: "@user1 and @user2 are here.",
			want: "[~user1] and [~user2] are here.",
		},
		{
			name: "mention with invalid characters",
			text: "Hello @user! and @user@name",
			want: "Hello [~user]! and [~user]@name",
		},
		{
			name: "mention with hyphen and underscore",
			text: "Hello @user-name and @user_name!",
			want: "Hello [~user-name] and [~user_name]!",
		},
		{
			name: "mention at the end",
			text: "This is a mention @user",
			want: "This is a mention [~user]",
		},
		{
			name: "mention with numbers",
			text: "Hello @user123!",
			want: "Hello [~user123]!",
		},
		{
			name: "no mentions",
			text: "Hello world!",
			want: "Hello world!",
		},
		{
			name: "mention with special characters",
			text: "Hello @user!@name",
			want: "Hello [~user]![~name]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.convertMentions(tt.text)
			if got != tt.want {
				t.Errorf("ConvertMentions() = %v, want %v", got, tt.want)
			}
		})
	}
}

func BenchmarkConvertMentions(b *testing.B) {
	r := NewJiraRenderer()
	b.Run("simple", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		text := "Hello @user! How are you?"
		for i := 0; i < b.N; i++ {
			_ = r.convertMentions(text)
		}
	})
	b.Run("complex", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		text := "Hello @user! How are you? How are you?How are you?How are you?How are you?How are you?How are you?"
		for i := 0; i < b.N; i++ {
			_ = r.convertMentions(text)
		}
	})
	b.Run("no mention", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		text := "Hello How are you? How are you?How are you?How are you?How are you?How are you?How are you?"
		for i := 0; i < b.N; i++ {
			_ = r.convertMentions(text)
		}
	})
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
