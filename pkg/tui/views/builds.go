package views

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
	"github.com/lsst25/lazybitbucket/pkg/tui/theme"
)

const buildsPanelTitle = "[3] Builds"

// BuildsPanel is the [3] panel: the build statuses posted against the selected
// pull request's head commit, newest first, one row per status key.
type BuildsPanel struct {
	components.ListBase
	key string
	// statuses holds every status of the pull request; visible holds the subset
	// the search bar leaves on screen. The cursor indexes visible, so the
	// selection can never point at a row the panel is not drawing.
	statuses []bitbucket.CommitStatus
	visible  []bitbucket.CommitStatus
	loading  bool
	errMsg   string
	filter   string
	theme    *theme.Theme
}

// NewBuildsPanel constructs an empty panel on the default theme.
func NewBuildsPanel() *BuildsPanel {
	return &BuildsPanel{theme: theme.Default}
}

// SetPullRequest points the panel at a pull request key. Statuses, the error
// and the cursor are only reset when the key actually changes, so a background
// refresh of the same pull request does not blank the panel.
func (b *BuildsPanel) SetPullRequest(key string) {
	if key != b.key {
		b.key = key
		b.statuses = nil
		b.errMsg = ""
		b.Cursor = 0
		b.Offset = 0
		b.applyFilter()
	}
	b.loading = false
}

// SetLoading marks a fetch in flight. A response for a pull request the user
// has already moved off is dropped.
func (b *BuildsPanel) SetLoading(key string) {
	if key != b.key {
		return
	}
	b.loading = true
	b.errMsg = ""
}

// SetStatuses stores the newest status per key. A response for another pull
// request is dropped.
func (b *BuildsPanel) SetStatuses(key string, statuses []bitbucket.CommitStatus) {
	if key != b.key {
		return
	}
	b.statuses = bitbucket.LatestStatuses(statuses)
	b.loading = false
	b.errMsg = ""
	b.applyFilter()
}

// SetError records a failed fetch. An error for another pull request is
// dropped.
func (b *BuildsPanel) SetError(key, msg string) {
	if key != b.key {
		return
	}
	b.errMsg = msg
	b.loading = false
}

// Key returns the pull request key the panel is showing builds for.
func (b *BuildsPanel) Key() string { return b.key }

// Statuses returns the stored statuses, newest first.
func (b *BuildsPanel) Statuses() []bitbucket.CommitStatus { return b.statuses }

// SelectedStatus returns a copy of the status under the cursor, nil when there
// is none. The cursor indexes the filtered rows, so this is always the row the
// panel highlights.
func (b *BuildsPanel) SelectedStatus() *bitbucket.CommitStatus {
	if b.Cursor < 0 || b.Cursor >= len(b.visible) {
		return nil
	}
	status := b.visible[b.Cursor]
	return &status
}

// SelectByKey moves the cursor to the visible status with the given key and
// reports whether it was found. It is how a caller keeps the selection on the
// build the user searched for after the filter is dropped.
func (b *BuildsPanel) SelectByKey(key string) bool {
	for i := range b.visible {
		if b.visible[i].Key == key {
			b.Cursor = i
			b.AdjustOffset()
			return true
		}
	}
	return false
}

// SetFilter applies the search bar query to the rows.
func (b *BuildsPanel) SetFilter(query string) {
	b.filter = query
	b.applyFilter()
}

// ClearFilter drops the search query.
func (b *BuildsPanel) ClearFilter() {
	b.filter = ""
	b.applyFilter()
}

// applyFilter recomputes the visible rows. The filter is panel state rather
// than a render-time slice so the cursor, the footer count and SelectedStatus
// all agree with what View draws.
func (b *BuildsPanel) applyFilter() {
	if b.filter == "" {
		b.visible = b.statuses
		b.SetItemCount(len(b.visible))
		return
	}
	q := strings.ToLower(b.filter)
	visible := make([]bitbucket.CommitStatus, 0, len(b.statuses))
	for _, status := range b.statuses {
		if strings.Contains(statusHaystack(status), q) {
			visible = append(visible, status)
		}
	}
	b.visible = visible
	b.SetItemCount(len(b.visible))
}

// statusHaystack is what the search bar matches a build against: the same
// fields buildRow draws, untruncated so a query still matches a name the panel
// had to shorten to fit.
func statusHaystack(status bitbucket.CommitStatus) string {
	name := status.Name
	if name == "" {
		name = status.Key
	}
	return strings.ToLower(strings.Join([]string{
		buildGlyph(status.State, true), name, status.RefName, timeAgo(status.UpdatedOn),
	}, " "))
}

// ContentHeight is the natural panel height for the accordion layout.
func (b *BuildsPanel) ContentHeight() int { return b.ListBase.ContentHeight(3) }

// Init satisfies the bubbletea component shape; the panel has no startup work.
func (b *BuildsPanel) Init() tea.Cmd { return nil }

// Update moves the cursor. Selecting a build emits nothing: the app reads
// SelectedStatus when the user opens one.
func (b *BuildsPanel) Update(msg tea.Msg) (*BuildsPanel, tea.Cmd) {
	if !b.Focused {
		return b, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		b.KeyNav(key.String())
	}
	return b, nil
}

// View renders the panel, collapsing to a single title bar at height 1.
func (b *BuildsPanel) View() string {
	if b.Height <= 1 {
		footer := ""
		if n := len(b.visible); n > 0 {
			footer = fmt.Sprintf("%d of %d", b.Cursor+1, n)
		}
		return components.RenderCollapsedBar(buildsPanelTitle, footer, b.Width, b.Focused)
	}

	contentWidth, innerHeight := components.PanelDimensions(b.Width, b.Height)

	if placeholder, ok := b.placeholder(); ok {
		return components.RenderPanel(buildsPanelTitle, placeholder, b.Width, innerHeight, b.Focused)
	}

	styled, plain := b.rowPairs(contentWidth)

	var rendered []string
	end := min(b.Offset+innerHeight, len(plain))
	for i := b.Offset; i < end; i++ {
		if i == b.Cursor && b.Focused {
			rendered = append(rendered, b.theme.SelectedItem.Width(contentWidth).Render(plain[i]))
		} else {
			rendered = append(rendered, b.theme.NormalItem.Width(contentWidth).Render(styled[i]))
		}
	}

	footer := ""
	if len(plain) > 0 {
		footer = fmt.Sprintf("%d of %d", b.Cursor+1, len(plain))
	}
	scroll := &components.ScrollInfo{Total: len(plain), Visible: innerHeight, Offset: b.Offset}
	return components.RenderPanelFull(buildsPanelTitle, footer, strings.Join(rendered, "\n"),
		b.Width, innerHeight, b.Focused, scroll)
}

// placeholder returns the single-line body shown instead of rows, and whether
// there is one.
func (b *BuildsPanel) placeholder() (string, bool) {
	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)
	switch {
	case b.key == "":
		return gray.Render(" No pull request selected"), true
	case b.errMsg != "":
		return b.theme.ErrorText.Render(" " + b.errMsg), true
	case b.loading:
		return gray.Render(" Loading builds…"), true
	case len(b.statuses) == 0:
		return gray.Render(" No builds"), true
	case len(b.visible) == 0:
		return gray.Render(" No matching builds"), true
	default:
		return "", false
	}
}

// rowPairs renders each visible status twice: once styled and once plain. The
// plain copy backs the selected row, whose background would clash with inline
// foreground colours.
func (b *BuildsPanel) rowPairs(width int) (styled, plain []string) {
	styled = make([]string, 0, len(b.visible))
	plain = make([]string, 0, len(b.visible))
	for _, status := range b.visible {
		styled = append(styled, buildRow(status, width, false))
		plain = append(plain, buildRow(status, width, true))
	}
	return styled, plain
}

// buildRow renders " <glyph> <name>  <refname>  <age>", trimming the name first
// when the row does not fit.
func buildRow(status bitbucket.CommitStatus, width int, plain bool) string {
	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)

	age := timeAgo(status.UpdatedOn)
	refName := status.RefName
	name := status.Name
	if name == "" {
		name = status.Key
	}

	// " " + glyph + " " + name + "  " + ref + "  " + age
	fixed := 1 + 1 + 1 + 2 + lipgloss.Width(refName) + 2 + lipgloss.Width(age)
	name = components.TruncateEnd(name, max(width-fixed, 3))

	glyph := buildGlyph(status.State, plain)
	if plain {
		return " " + glyph + " " + name + "  " + refName + "  " + age
	}
	return " " + glyph + " " + name + "  " + gray.Render(refName) + "  " + gray.Render(age)
}
