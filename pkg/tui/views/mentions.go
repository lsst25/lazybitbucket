package views

import (
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
)

// A resolved mention is marked before the Markdown is rendered and painted
// afterwards: the Markdown renderer decides the wrapping and the styling of
// the body, and the markers are how a mention is found again in what it
// produced. They are U+FFF9 INTERLINEAR ANNOTATION ANCHOR and U+FFFB
// INTERLINEAR ANNOTATION TERMINATOR — zero-width for every width measure in
// the pipeline (go-runewidth under glamour, lipgloss under the plain wrapper),
// so a marked line wraps, pads and lines its table columns up exactly as an
// unmarked one does, and they are characters no comment carries.
const (
	mentionOpen  = "\ufff9"
	mentionClose = "\ufffb"
)

// styleProbe is the text a style is rendered around to learn the escape
// sequences it opens and closes with. It cannot occur inside a sequence.
const styleProbe = "\x00"

// markMention wraps one resolved mention in the markers.
func markMention(mention string) string { return mentionOpen + mention + mentionClose }

// readableMarkdown is raw with its mentions resolved through rendered — the
// html Bitbucket served next to it — and marked for paintMentions.
func readableMarkdown(raw, rendered string) string {
	return bitbucket.ResolveMentionsFunc(raw, rendered, markMention)
}

// renderBody renders a Markdown body — a description or a comment — with its
// mentions resolved to display names and painted in the theme's mention style.
func (d *DetailView) renderBody(raw, rendered string, width int) []string {
	return paintMentions(d.renderer.Render(readableMarkdown(raw, rendered), width), d.theme.Mention)
}

// paintMentions paints every marked mention in lines with style and removes
// the markers. Lines without a marker are returned as they are.
func paintMentions(lines []string, style lipgloss.Style) []string {
	if !slices.ContainsFunc(lines, hasMentionMarker) {
		return lines
	}
	open, closing := styleSequences(style)
	return paintMarked(lines, open, closing)
}

// hasMentionMarker reports whether a rendered line carries either marker.
func hasMentionMarker(line string) bool {
	return strings.Contains(line, mentionOpen) || strings.Contains(line, mentionClose)
}

// styleSequences is what style writes before and after the text it renders —
// nothing at all when the renderer has no colours to write with.
func styleSequences(style lipgloss.Style) (string, string) {
	before, after, _ := strings.Cut(style.Render(styleProbe), styleProbe)
	return before, after
}

// paintMarked replaces the markers in lines with open and closing. A mention
// the renderer wrapped across lines is painted on every line it touches, and
// open is written again after every sequence the renderer put inside it, so
// the renderer's own styling cannot take the colour back; once the mention
// closes, the renderer's state is restored by replaying the sequences it had
// in effect.
func paintMarked(lines []string, open, closing string) []string {
	painter := mentionPainter{open: open, closing: closing, inside: false}
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = painter.paint(line)
	}
	return out
}

// mentionPainter walks rendered lines one after the other, carrying "inside a
// mention" from each line into the next.
type mentionPainter struct {
	open    string
	closing string
	inside  bool
}

// paint repaints one line.
func (p *mentionPainter) paint(line string) string {
	var out strings.Builder
	out.Grow(len(line) + 2*len(p.open) + len(p.closing))

	// active is what the renderer has set since its last reset: what has to be
	// replayed once a mention closes.
	var active []string
	if p.inside {
		out.WriteString(p.open)
	}

	for i := 0; i < len(line); {
		switch {
		case strings.HasPrefix(line[i:], mentionOpen):
			p.inside = true
			out.WriteString(p.open)
			i += len(mentionOpen)
		case strings.HasPrefix(line[i:], mentionClose):
			p.inside = false
			out.WriteString(p.closing)
			out.WriteString(strings.Join(active, ""))
			i += len(mentionClose)
		case strings.HasPrefix(line[i:], csiPrefix):
			seq, ok := leadingSGR(line[i:])
			if !ok {
				// Not an SGR sequence: ordinary bytes, as far as painting goes.
				out.WriteString(csiPrefix)
				i += len(csiPrefix)
				continue
			}
			active = noteSGR(active, seq)
			out.WriteString(seq)
			if p.inside {
				out.WriteString(p.open)
			}
			i += len(seq)
		default:
			out.WriteByte(line[i])
			i++
		}
	}
	return out.String()
}

// noteSGR folds one of the renderer's sequences into the set in effect: a reset
// empties it, and anything else joins it. A sequence that resets before it sets
// ("0;1") does both.
func noteSGR(active []string, seq string) []string {
	params := strings.TrimSuffix(strings.TrimPrefix(seq, csiPrefix), "m")
	if params == "" || params == "0" {
		return active[:0]
	}
	if strings.HasPrefix(params, "0;") {
		active = active[:0]
	}
	return append(active, seq)
}
