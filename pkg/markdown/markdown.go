package markdown

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/appleboy/com/bytesconv"
	bf "github.com/russross/blackfriday/v2"
)

type JiraRenderer struct {
	builder     strings.Builder
	inList      bool
	listDepth   int
	inCodeBlock bool
}

func NewJiraRenderer() *JiraRenderer {
	return &JiraRenderer{
		builder: strings.Builder{},
	}
}

func (r *JiraRenderer) RenderNode(w *bytes.Buffer, node *bf.Node, entering bool) bf.WalkStatus {
	switch node.Type {
	case bf.BlockQuote:
		r.renderBlockQuote(w, node, entering)
	case bf.HorizontalRule:
		r.renderHorizontalRule(w, node, entering)
	case bf.Image:
		r.renderImage(w, node, entering)
	case bf.HTMLBlock, bf.HTMLSpan:
		// Ignore HTML blocks and spans
	case bf.Softbreak:
		r.renderSoftbreak(w, node, entering)
	case bf.Table, bf.TableCell, bf.TableHead, bf.TableBody, bf.TableRow:
		// Handle tables if needed
	case bf.Document:
		return bf.GoToNext
	case bf.Paragraph:
		r.renderParagraph(w, node, entering)
	case bf.Heading:
		r.renderHeading(w, node, entering)
	case bf.Text:
		r.renderText(w, node, entering)
	case bf.Strong:
		r.renderStrong(w, node, entering)
	case bf.Emph:
		r.renderEmph(w, node, entering)
	case bf.Link:
		r.renderLink(w, node, entering)
	case bf.List:
		r.renderList(w, node, entering)
	case bf.Item:
		r.renderItem(w, node, entering)
	case bf.Code:
		r.renderCode(w, node, entering)
	case bf.CodeBlock:
		r.renderCodeBlock(w, node, entering)
	case bf.Hardbreak:
		r.renderHardbreak(w, node, entering)
	case bf.Del:
		r.renderDel(w, node, entering)
	}
	return bf.GoToNext
}

func (r *JiraRenderer) renderBlockQuote(_ *bytes.Buffer, _ *bf.Node, _ bool) {
	// Handle BlockQuote
}

func (r *JiraRenderer) renderHorizontalRule(w *bytes.Buffer, _ *bf.Node, _ bool) {
	w.WriteString("----\n")
}

func (r *JiraRenderer) renderImage(w *bytes.Buffer, node *bf.Node, entering bool) {
	if entering {
		w.WriteString("!")
		return
	}
	w.WriteString("|")
	w.Write(node.Destination)
	w.WriteString("!")
}

func (r *JiraRenderer) renderSoftbreak(w *bytes.Buffer, _ *bf.Node, _ bool) {
	w.WriteString(" ")
}

func (r *JiraRenderer) renderParagraph(w *bytes.Buffer, _ *bf.Node, entering bool) {
	if entering && !r.inList && w.Len() > 0 {
		w.WriteString("\n")
		return
	}
	if !entering {
		w.WriteString("\n")
	}
}

func (r *JiraRenderer) renderHeading(w *bytes.Buffer, node *bf.Node, entering bool) {
	if entering {
		if w.Len() > 0 {
			w.WriteString("\n")
		}
		w.WriteString("h")
		w.WriteString(strconv.Itoa(node.Level))
		w.WriteString(". ")
		return
	}
	w.WriteString("\n")
}

func (r *JiraRenderer) renderText(w *bytes.Buffer, node *bf.Node, _ bool) {
	text := r.convertMentions(bytesconv.BytesToStr(node.Literal))
	w.WriteString(text)
}

func (r *JiraRenderer) renderStrong(w *bytes.Buffer, _ *bf.Node, _ bool) {
	w.WriteString("*")
}

func (r *JiraRenderer) renderEmph(w *bytes.Buffer, _ *bf.Node, _ bool) {
	w.WriteString("_")
}

func (r *JiraRenderer) renderLink(w *bytes.Buffer, node *bf.Node, entering bool) {
	if entering {
		w.WriteString("[")
		return
	}
	w.WriteString("|")
	w.Write(node.Destination)
	w.WriteString("]")
}

func (r *JiraRenderer) renderList(w *bytes.Buffer, _ *bf.Node, entering bool) {
	if entering {
		r.inList = true
		r.listDepth++
		return
	}
	r.listDepth--
	if r.listDepth == 0 {
		r.inList = false
		w.WriteString("\n")
	}
}

func (r *JiraRenderer) renderItem(w *bytes.Buffer, _ *bf.Node, entering bool) {
	if !entering {
		return
	}
	indent := strings.Repeat("*", r.listDepth)
	if w.Len() > 0 && !strings.HasSuffix(w.String(), "\n") {
		w.WriteString("\n")
	}
	w.WriteString(indent + " ")
}

func (r *JiraRenderer) renderCode(w *bytes.Buffer, node *bf.Node, _ bool) {
	w.WriteString("{{")
	w.Write(node.Literal)
	w.WriteString("}}")
}

func (r *JiraRenderer) renderCodeBlock(w *bytes.Buffer, node *bf.Node, entering bool) {
	if entering {
		r.inCodeBlock = true
		language := string(node.Info)
		if language == "" {
			language = "java"
		}
		if w.Len() > 0 {
			w.WriteString("\n")
		}
		w.WriteString("{code:language=")
		w.WriteString(language)
		w.WriteString("}\n")
		w.Write(node.Literal)
		w.WriteString("{code}")
	}
	r.inCodeBlock = false
}

func (r *JiraRenderer) renderHardbreak(w *bytes.Buffer, _ *bf.Node, _ bool) {
	w.WriteString("\n")
}

func (r *JiraRenderer) renderDel(w *bytes.Buffer, _ *bf.Node, _ bool) {
	w.WriteString("-")
}

func (r *JiraRenderer) convertMentions(text string) string {
	// check the text include @ syntax
	if !strings.Contains(text, "@") {
		return text
	}

	count := strings.Count(text, "@")
	length := len(text)
	r.builder.Reset()
	r.builder.Grow(length + count*2) // Preallocate buffer with an initial capacity
	for i := 0; i < length; i++ {
		if text[i] == '@' && i+1 < length && isValidMentionChar(text[i+1]) {
			r.builder.WriteString("[~")
			i++
			for i < length && isValidMentionChar(text[i]) {
				r.builder.WriteByte(text[i])
				i++
			}
			r.builder.WriteString("]")
			if i < length {
				r.builder.WriteByte(text[i])
			}
			continue
		}
		// copy the character
		r.builder.WriteByte(text[i])
	}
	return r.builder.String()
}

// MarkdownToJira converts a given Markdown string to Jira markup format.
// It uses the blackfriday library to parse the Markdown and a custom Jira renderer
// to generate the corresponding Jira markup.
//
// Parameters:
//   - markdown: A string containing the Markdown content to be converted.
//
// Returns:
//
//	A string containing the converted content in Jira markup format.
func ToJira(markdown string) string {
	extensions := bf.CommonExtensions | bf.AutoHeadingIDs
	md := bf.New(bf.WithExtensions(extensions))

	ast := md.Parse(bytesconv.StrToBytes(markdown))

	buf := bytes.NewBuffer(make([]byte, 0, 512)) // Preallocate buffer with an initial capacity
	renderer := NewJiraRenderer()
	ast.Walk(func(node *bf.Node, entering bool) bf.WalkStatus {
		return renderer.RenderNode(buf, node, entering)
	})

	return strings.TrimSpace(bytesconv.BytesToStr(buf.Bytes()))
}

var validMentionChars [256]bool

func init() {
	for c := 'a'; c <= 'z'; c++ {
		validMentionChars[c] = true
	}
	for c := 'A'; c <= 'Z'; c++ {
		validMentionChars[c] = true
	}
	for c := '0'; c <= '9'; c++ {
		validMentionChars[c] = true
	}
	validMentionChars['-'] = true
	validMentionChars['_'] = true
}

// isValidMentionChar checks if the given byte character is a valid mention character.
// It returns true if the character is valid, otherwise false.
func isValidMentionChar(c byte) bool {
	return validMentionChars[c]
}
