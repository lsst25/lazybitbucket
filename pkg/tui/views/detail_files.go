package views

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
	"github.com/lsst25/lazybitbucket/pkg/tui/theme"
)

// FileSelectedMsg asks the app to open one changed file's diff. Path is the
// display path; the app maps it back to the diffstat entry, whose escaped path
// is what the diff endpoint wants.
type FileSelectedMsg struct {
	PullRequestKey string
	Path           string
}

// minusSign is the typographic minus the counts use, so "+12 −3" lines up with
// the plus rather than reading as a hyphenated range.
const minusSign = "−"

// renderFileBlocks builds one single-line block per changed file. The loading,
// error and empty states are the only block instead, which keeps the cursor,
// the footer and the hit-test honest without a header the other review tabs do
// not have.
func (d *DetailView) renderFileBlocks(width int) [][]string {
	if placeholder, ok := d.filesPlaceholder(); ok {
		return [][]string{{placeholder}}
	}

	counts := make([]string, len(d.files.items))
	countsWidth := 0
	for i := range d.files.items {
		counts[i] = fileCounts(d.files.items[i])
		countsWidth = max(countsWidth, lipgloss.Width(counts[i]))
	}

	blocks := make([][]string, 0, len(d.files.items))
	for i := range d.files.items {
		blocks = append(blocks, []string{fileLine(d.files.items[i], counts[i], countsWidth, width)})
	}
	return blocks
}

// filesPlaceholder is the single line the Files tab shows instead of a list,
// and whether there is one. A refetch over a loaded list is not one of the
// states: the list stays on screen with the cursor where the reader left it,
// the tab label carries the in-flight ellipsis and listModeHeader carries the
// error.
func (d *DetailView) filesPlaceholder() (string, bool) {
	if len(d.files.items) > 0 {
		return "", false
	}
	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)
	switch {
	case d.files.errMsg != "":
		return d.theme.ErrorText.Render(d.files.errMsg), true
	case d.files.loading || !d.files.loaded:
		return gray.Render("Loading files…"), true
	default:
		return gray.Render("No changes"), true
	}
}

// fileLine renders one changed file: a status glyph, the path (both paths on a
// rename), a binary marker and the added/removed counts, right-aligned so the
// numbers form a column.
func fileLine(entry bitbucket.DiffStatEntry, counts string, countsWidth, width int) string {
	glyph, style := fileGlyph(entry.Status)
	name := entry.Path()
	if entry.Status == bitbucket.DiffStatRenamed && entry.OldPath() != "" {
		name = entry.OldPath() + " → " + entry.Path()
	}
	if entry.IsBinary() {
		name += lipgloss.NewStyle().Foreground(theme.ColorGray).Render(" B")
	}

	// glyph, a space, the name, a space, the counts column.
	nameWidth := max(width-countsWidth-3, 8)
	return style.Render(glyph) + " " +
		padRight(components.TruncateEnd(name, nameWidth), nameWidth) + " " +
		padLeft(counts, countsWidth)
}

// fileCounts is the "+12 −3" cell, each side omitted when it is zero so an
// added file does not claim to have removed nothing.
func fileCounts(entry bitbucket.DiffStatEntry) string {
	green := lipgloss.NewStyle().Foreground(theme.ColorGreen)
	red := lipgloss.NewStyle().Foreground(theme.ColorRed)

	var parts []string
	if entry.LinesAdded > 0 {
		parts = append(parts, green.Render("+"+strconv.Itoa(entry.LinesAdded)))
	}
	if entry.LinesRemoved > 0 {
		parts = append(parts, red.Render(minusSign+strconv.Itoa(entry.LinesRemoved)))
	}
	return strings.Join(parts, " ")
}

// fileGlyph is the one-letter status column and its colour.
func fileGlyph(status string) (string, lipgloss.Style) {
	switch status {
	case bitbucket.DiffStatAdded:
		return "A", lipgloss.NewStyle().Foreground(theme.ColorGreen)
	case bitbucket.DiffStatModified:
		return "M", lipgloss.NewStyle().Foreground(theme.ColorYellow)
	case bitbucket.DiffStatRemoved:
		return "D", lipgloss.NewStyle().Foreground(theme.ColorRed)
	case bitbucket.DiffStatRenamed:
		return "R", lipgloss.NewStyle().Foreground(theme.ColorCyan)
	default:
		return "?", lipgloss.NewStyle().Foreground(theme.ColorGray)
	}
}

// activateFile asks the app to open the cursor file's diff.
func (d *DetailView) activateFile() tea.Cmd {
	entry := d.SelectedFile()
	if entry == nil || d.pr == nil {
		return nil
	}
	key, path := d.pr.Key(), entry.Path()
	return func() tea.Msg {
		return FileSelectedMsg{PullRequestKey: key, Path: path}
	}
}
