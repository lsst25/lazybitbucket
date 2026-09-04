package views

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// Chroma style names paired with the glamour styles the renderer uses, so the
// diff's code column and the Markdown bodies above it agree about the
// background they are drawn on.
const (
	chromaStyleDark  = "monokai"
	chromaStyleLight = "github"
)

// maxHighlightBytes is the longest line the highlighter will tokenise. A
// minified bundle or an embedded data URI is one enormous line whose
// highlighting nobody reads, and the diff view renders every visible line on
// every keystroke, so those are passed through as plain text.
const maxHighlightBytes = 2000

// ChromaStyleFor maps a resolved glamour style (as ResolveGlamourStyle returns
// it) to the chroma style the diff view highlights code with. It returns "" for
// "notty" and for anything unrecognised, which disables highlighting: a
// terminal that cannot render glamour's colours cannot render chroma's either.
func ChromaStyleFor(glamourStyle string) string {
	switch glamourStyle {
	case glamourStyleDark:
		return chromaStyleDark
	case glamourStyleLight:
		return chromaStyleLight
	case glamourStyleNoTTY:
		return ""
	default:
		return ""
	}
}

// highlighter colours the code column of one file's diff. It is built once per
// file — picking a lexer means sniffing the content, which is far too expensive
// to repeat per line — and is safe to leave nil, in which case Line is the
// identity function.
type highlighter struct {
	lexer     chroma.Lexer
	style     *chroma.Style
	formatter chroma.Formatter
}

// newHighlighter builds the highlighter for one file. path picks the lexer by
// extension, content is the fallback the lexer is sniffed from when the
// extension says nothing, and styleName is a ChromaStyleFor result: "" returns
// nil, which turns highlighting off.
func newHighlighter(path, content, styleName string) *highlighter {
	if styleName == "" {
		return nil
	}
	lexer := lexers.Match(path)
	if lexer == nil {
		lexer = lexers.Analyse(content)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	return &highlighter{
		// Coalesce merges runs of same-type tokens, which cuts the number of
		// escape sequences a line of code costs.
		lexer:     chroma.Coalesce(lexer),
		style:     styles.Get(styleName),
		formatter: formatters.TTY256,
	}
}

// Line highlights one line of code. Lines are tokenised independently, so a
// line inside a block comment or a raw string is coloured as if it stood alone;
// that is the price of not re-lexing the whole file on every scroll. A
// tokenising or formatting failure yields the line unchanged rather than an
// error, because a diff that renders wrongly still beats one that does not
// render.
func (h *highlighter) Line(code string) string {
	if h == nil || code == "" || len(code) > maxHighlightBytes {
		return code
	}
	iterator, err := h.lexer.Tokenise(nil, code)
	if err != nil {
		return code
	}
	var out strings.Builder
	if err := h.formatter.Format(&out, h.style, iterator); err != nil {
		return code
	}
	// Lexers normalise their input to end in a newline; the formatter passes
	// that through and a newline inside a rendered row would break the layout.
	return strings.ReplaceAll(out.String(), "\n", "")
}
