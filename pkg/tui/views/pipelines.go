package views

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
	"github.com/lsst25/lazybitbucket/pkg/tui/theme"
)

const pipelinesPanelTitle = "[2] Pipelines"

// minPipelineTargetWidth is how narrow the flexible target column may get
// before the row starts dropping its fixed columns instead. Below this a
// branch name is shortened past the point of telling two runs apart, which is
// the one thing the column exists for.
const minPipelineTargetWidth = 10

// PipelineSelectedMsg is emitted when the list cursor lands on a run. Like
// RepoHoveredMsg it is a cursor move rather than an activation: the app shows
// the run in the detail panel straight away, because the list payload is the
// whole run.
type PipelineSelectedMsg struct {
	Entry *bitbucket.PipelineEntry
}

// PipelineList is the [2] panel of the pipelines view: recent runs of every
// configured repository, newest first, one row per run.
type PipelineList struct {
	components.ListBase
	// entries holds every fetched run; visible holds the subset the search bar
	// leaves on screen. The cursor indexes visible, so the selection can never
	// point at a row the panel is not drawing.
	entries []bitbucket.PipelineEntry
	visible []bitbucket.PipelineEntry
	loading bool
	errMsg  string
	filter  string
	theme   *theme.Theme
}

// NewPipelineList constructs an empty list on the default theme.
func NewPipelineList() *PipelineList {
	return &PipelineList{theme: theme.Default}
}

// SetEntries replaces the runs and clears the loading and error state.
//
// The cursor stays on the run it was on when that run came back too — matched
// on repository and build number, because a refetch answers with fresh structs
// — so the periodic refetch behind a rerun or a stop does not throw the
// reader's place away. A selection that is gone falls back to the top.
func (p *PipelineList) SetEntries(entries []bitbucket.PipelineEntry) {
	prev := p.SelectedEntry()
	p.entries = entries
	p.loading = false
	p.errMsg = ""
	p.Cursor = 0
	p.Offset = 0
	p.applyFilter()
	if prev != nil {
		p.SelectByRun(prev.Repo, prev.Pipeline.BuildNumber)
	}
}

// SetLoading marks a fetch in flight.
func (p *PipelineList) SetLoading() {
	p.loading = true
	p.errMsg = ""
}

// SetError records a failed fetch, which the panel shows instead of its rows,
// exactly as the Builds panel does. It is only reached when every repository
// failed; a fetch that lost some of them reports that beside the list.
func (p *PipelineList) SetError(msg string) {
	p.errMsg = msg
	p.loading = false
}

// Entries returns the stored runs, newest first.
func (p *PipelineList) Entries() []bitbucket.PipelineEntry { return p.entries }

// SelectedEntry returns a copy of the run under the cursor, nil when there is
// none. The cursor indexes the filtered rows, so this is always the row the
// panel highlights.
func (p *PipelineList) SelectedEntry() *bitbucket.PipelineEntry {
	if p.Cursor < 0 || p.Cursor >= len(p.visible) {
		return nil
	}
	entry := p.visible[p.Cursor]
	return &entry
}

// SelectByRun moves the cursor to the visible run of that repository and build
// number, reporting whether it was found. It is how a caller keeps the
// selection on one run across a refetch.
func (p *PipelineList) SelectByRun(repo string, build int) bool {
	for i := range p.visible {
		if p.visible[i].Repo == repo && p.visible[i].Pipeline.BuildNumber == build {
			p.Cursor = i
			p.AdjustOffset()
			return true
		}
	}
	return false
}

// SetFilter applies the search bar query to the rows.
func (p *PipelineList) SetFilter(query string) {
	p.filter = query
	p.applyFilter()
}

// ClearFilter drops the search query.
func (p *PipelineList) ClearFilter() {
	p.filter = ""
	p.applyFilter()
}

// applyFilter recomputes the visible rows. The filter is panel state rather
// than a render-time slice so the cursor, the footer count and SelectedEntry
// all agree with what View draws.
func (p *PipelineList) applyFilter() {
	if p.filter == "" {
		p.visible = p.entries
		p.SetItemCount(len(p.visible))
		return
	}
	q := strings.ToLower(p.filter)
	visible := make([]bitbucket.PipelineEntry, 0, len(p.entries))
	for _, entry := range p.entries {
		if strings.Contains(pipelineHaystack(entry), q) {
			visible = append(visible, entry)
		}
	}
	p.visible = visible
	p.SetItemCount(len(p.visible))
}

// pipelineHaystack is what the search bar matches a run against: the fields the
// row draws, untruncated so a query still matches a branch the panel had to
// shorten to fit, plus the trigger and the creator, which are the two things a
// reader looks a run up by and the row has no room for.
func pipelineHaystack(entry bitbucket.PipelineEntry) string {
	p := entry.Pipeline
	parts := []string{
		"#" + strconv.Itoa(p.BuildNumber),
		entry.Repo,
		pipelineTargetCell(p.Target),
		p.State.Summary(),
		p.Trigger.Name,
	}
	if p.Creator != nil {
		parts = append(parts, p.Creator.Name())
	}
	return strings.ToLower(strings.Join(parts, " "))
}

// ContentHeight is the natural panel height for the accordion layout.
func (p *PipelineList) ContentHeight() int { return p.ListBase.ContentHeight(7) }

// Init satisfies the bubbletea component shape; the list has no startup work.
func (p *PipelineList) Init() tea.Cmd { return nil }

// Update moves the cursor and announces the run it landed on, which is what
// makes the detail panel follow the list without an extra keypress.
func (p *PipelineList) Update(msg tea.Msg) (*PipelineList, tea.Cmd) {
	if !p.Focused {
		return p, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		if p.KeyNav(key.String()) {
			if entry := p.SelectedEntry(); entry != nil {
				return p, func() tea.Msg { return PipelineSelectedMsg{Entry: entry} }
			}
		}
	}
	return p, nil
}

// View renders the panel, collapsing to a single title bar at height 1.
func (p *PipelineList) View() string {
	footer := ""
	if n := len(p.visible); n > 0 {
		footer = fmt.Sprintf("%d of %d", p.Cursor+1, n)
	}

	if p.Height <= 1 {
		return components.RenderCollapsedBar(pipelinesPanelTitle, footer, p.Width, p.Focused)
	}

	contentWidth, innerHeight := components.PanelDimensions(p.Width, p.Height)

	if placeholder, ok := p.placeholder(); ok {
		return components.RenderPanel(pipelinesPanelTitle, placeholder, p.Width, innerHeight, p.Focused)
	}

	styled, plain := p.rowPairs(contentWidth)

	var rendered []string
	end := min(p.Offset+innerHeight, len(plain))
	for i := p.Offset; i < end; i++ {
		if i == p.Cursor && p.Focused {
			rendered = append(rendered, p.theme.SelectedItem.Width(contentWidth).Render(plain[i]))
		} else {
			rendered = append(rendered, p.theme.NormalItem.Width(contentWidth).Render(styled[i]))
		}
	}

	scroll := &components.ScrollInfo{Total: len(plain), Visible: innerHeight, Offset: p.Offset}
	return components.RenderPanelFull(pipelinesPanelTitle, footer, strings.Join(rendered, "\n"),
		p.Width, innerHeight, p.Focused, scroll)
}

// placeholder returns the single-line body shown instead of rows, and whether
// there is one.
func (p *PipelineList) placeholder() (string, bool) {
	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)
	switch {
	case p.errMsg != "":
		return p.theme.ErrorText.Render(" " + p.errMsg), true
	case p.loading:
		return gray.Render(" Loading pipelines…"), true
	case len(p.entries) == 0:
		return gray.Render(" No pipeline runs"), true
	case len(p.visible) == 0:
		return gray.Render(" No matching runs"), true
	default:
		return "", false
	}
}

// rowPairs renders each visible run twice: once styled and once plain. The
// plain copy backs the selected row, whose background would clash with inline
// foreground colours.
func (p *PipelineList) rowPairs(width int) (styled, plain []string) {
	styled = make([]string, 0, len(p.visible))
	plain = make([]string, 0, len(p.visible))
	for _, entry := range p.visible {
		styled = append(styled, pipelineRow(entry, width, false))
		plain = append(plain, pipelineRow(entry, width, true))
	}
	return styled, plain
}

// pipelineRow renders " <glyph> #415  api-gateway  PR #103 branch  4m28s  2h ago".
// The target is the flexible column; when it would be squeezed below
// minPipelineTargetWidth the fixed columns are dropped for it, cheapest first:
// the age, then the duration, then the repository slug.
func pipelineRow(entry bitbucket.PipelineEntry, width int, plain bool) string {
	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)
	run := entry.Pipeline

	number := "#" + strconv.Itoa(run.BuildNumber)
	age := ""
	if ago := timeAgo(run.CreatedOn); ago != "" {
		age = ago + " ago"
	}
	// A run still going has no duration to report; Duration() would call it 0s.
	duration := ""
	if !run.CompletedOn.IsZero() {
		duration = formatDuration(run.Duration())
	}

	// " " + glyph + " " + number, then each remaining cell behind two spaces.
	repo := entry.Repo
	head := 1 + 1 + 1 + lipgloss.Width(number)
	for _, drop := range []*string{&age, &duration, &repo} {
		if width-head-cellsWidth(repo, duration, age) >= minPipelineTargetWidth {
			break
		}
		*drop = ""
	}

	target := components.TruncateEnd(pipelineTargetCell(run.Target),
		max(width-head-cellsWidth(repo, duration, age), 3))

	glyph, style := pipelineGlyph(run.State)
	if !plain {
		glyph = style.Render(glyph)
	}
	// The fixed columns are dimmed, the target is not: it is what the eye scans
	// the list by.
	dim := func(cell string) string {
		switch {
		case cell == "":
			return ""
		case plain:
			return "  " + cell
		default:
			return "  " + gray.Render(cell)
		}
	}
	return " " + glyph + " " + number + dim(repo) + "  " + target + dim(duration) + dim(age)
}

// cellsWidth is the room the given non-empty cells need, each with the two
// spaces that separate it from the one before.
func cellsWidth(cells ...string) int {
	total := 0
	for _, cell := range cells {
		if cell != "" {
			total += 2 + lipgloss.Width(cell)
		}
	}
	return total
}

// pipelineTargetCell is the row's description of what a run was for. It shares
// PipelineTargetLine's vocabulary but not its destination branch: a list row
// has to stay short, and the destination is the one part every run of a
// repository tends to agree on.
func pipelineTargetCell(target bitbucket.PipelineTarget) string {
	if pr := target.PullRequest; pr != nil {
		return strings.TrimSpace("PR #" + strconv.Itoa(pr.ID) + " " + target.Source)
	}
	if branch := target.BranchName(); branch != "" {
		return branch
	}
	return "target " + target.Type
}

// pipelineGlyph is the one-character state column of a run, in the Builds
// panel's vocabulary. A state the API grows later falls through to the neutral
// circle rather than claiming an outcome.
func pipelineGlyph(state bitbucket.PipelineState) (string, lipgloss.Style) {
	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)
	switch pipelineStateWord(state) {
	case "SUCCESSFUL":
		return "✔", lipgloss.NewStyle().Foreground(theme.ColorGreen)
	case "FAILED", "ERROR":
		return "✘", lipgloss.NewStyle().Foreground(theme.ColorRed)
	case "IN_PROGRESS", "PENDING":
		return "●", lipgloss.NewStyle().Foreground(theme.ColorYellow)
	case "STOPPED":
		return "■", gray
	default:
		return "○", gray
	}
}

// pipelineStateWord is the word the glyph switches on: the result of a finished
// run, and the state name otherwise. State.Summary() is not it — for a run in
// progress it answers the stage (RUNNING, PAUSED), which is a distinction the
// one-character column does not draw.
func pipelineStateWord(state bitbucket.PipelineState) string {
	if state.IsCompleted() && state.Result != nil && state.Result.Name != "" {
		return state.Result.Name
	}
	return state.Name
}
