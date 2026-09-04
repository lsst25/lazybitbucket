package views

import (
	"strings"

	"github.com/charmbracelet/glamour"
)

// MarkdownRenderer turns a Markdown document into terminal lines that fit
// within width columns. Bitbucket pull request descriptions and comments are
// Markdown, so this is the only body renderer the TUI needs.
type MarkdownRenderer interface {
	Render(markdown string, width int) []string
}

// GlamourRenderer renders Markdown with glamour using one of its standard
// styles. Style is the value returned by ResolveGlamourStyle ("dark", "light"
// or "notty"); "notty" produces unstyled output, which is what tests use.
type GlamourRenderer struct {
	Style string
}

// Render renders md and returns its lines with trailing blank lines removed.
// A renderer or render failure is reported inline rather than dropped, so a
// broken style never silently blanks the detail pane.
func (g GlamourRenderer) Render(markdown string, width int) []string {
	if markdown == "" {
		return nil
	}
	if width <= 0 {
		width = defaultWrapWidth
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(g.Style),
		glamour.WithWordWrap(width),
		glamour.WithEmoji(),
	)
	if err != nil {
		return []string{"[glamour: " + err.Error() + "]"}
	}

	out, err := renderer.Render(markdown)
	if err != nil {
		return []string{"[glamour: " + err.Error() + "]"}
	}

	lines := strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

// PlainRenderer word-wraps Markdown as plain text without any styling. It is
// deterministic and allocation-cheap, which makes it the renderer tests and
// headless code paths inject.
type PlainRenderer struct{}

// Render wraps markdown to width columns, leaving the source markup intact.
func (PlainRenderer) Render(markdown string, width int) []string {
	if markdown == "" {
		return nil
	}
	return wrapText(markdown, width)
}
