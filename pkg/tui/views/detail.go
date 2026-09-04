package views

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
	"github.com/lsst25/lazybitbucket/pkg/tui/theme"
)

// DetailTab is a tab within the pull request view. Step 1 has a single one;
// step 2 adds Diff, Commits and Comments, at which point the tab bar appears.
type DetailTab int

// Tabs of the pull request view.
const (
	TabOverview DetailTab = iota
)

// MainMode is what the right-hand panel is showing. Unlike lazyjira's
// tab-driven detail view, the mode decides both the body and whether the panel
// is a scrolling text pane or a block list.
type MainMode int

// Modes of the detail panel.
const (
	ModePullRequest MainMode = iota
	ModePipeline
	ModeStepLog
	ModeRepo
	ModeSplash
)

// maxBlockLines is how many lines of a block are shown before it is collapsed
// behind an ellipsis and offered for expansion.
const maxBlockLines = 8

const (
	// notRunLabel is the duration cell of a step that never ran.
	notRunLabel = "not run"
	// resultNotRun is the step result Bitbucket reports for a step that was
	// skipped because an earlier one failed.
	resultNotRun = "NOT_RUN"
)

// SplashInfo is the startup screen's content.
type SplashInfo struct {
	Version    string
	AuthMethod string
	Workspace  string
	User       string
	RepoCount  int
	RepoNote   string
}

// ExpandBlockMsg asks the app to open a truncated block in a read-only modal.
type ExpandBlockMsg struct {
	Title string
	Lines []string
}

// StepSelectedMsg is emitted when the user activates a pipeline step, and asks
// the app to fetch that step's log.
type StepSelectedMsg struct {
	PipelineUUID string
	StepUUID     string
}

// DetailView is the [0] panel. It renders a pull request, one of its pipeline
// runs, a step log, a repository or the splash screen, depending on its mode.
type DetailView struct {
	pr             *bitbucket.PullRequest
	statuses       []bitbucket.CommitStatus
	statusesLoaded bool

	pipeline        *bitbucket.Pipeline
	steps           []bitbucket.PipelineStep
	pipelineBuild   int
	pipelineLoading bool
	pipelineErr     string

	stepUUID       string
	stepLog        string
	stepLogLoading bool
	stepLogErr     string

	repo   *RepoItem
	splash SplashInfo

	mode       MainMode
	activeTab  DetailTab
	scrollY    int
	listCursor int
	blocks     [][]string
	blockKeys  []string
	dblClick   components.DblClickDetector

	width   int
	height  int
	focused bool
	theme   *theme.Theme

	renderer MarkdownRenderer
	// ResolveNav is assigned by the app; the view is keyboard-inert until it is.
	ResolveNav components.NavResolver
}

// NewDetailView constructs a detail view rendering Markdown with renderer.
func NewDetailView(renderer MarkdownRenderer) *DetailView {
	return &DetailView{theme: theme.Default, mode: ModePullRequest, renderer: renderer}
}

// Mode reports what the panel is currently showing.
func (d *DetailView) Mode() MainMode { return d.mode }

// PullRequestKey is the key of the pull request in view, "" when the panel is
// showing something that is not tied to one.
func (d *DetailView) PullRequestKey() string {
	if d.pr == nil {
		return ""
	}
	switch d.mode {
	case ModePullRequest, ModePipeline, ModeStepLog:
		return d.pr.Key()
	default:
		return ""
	}
}

// SetPullRequest shows a pull request. Scroll and tab are only reset when the
// key changes, so a background refresh does not yank the user's place.
func (d *DetailView) SetPullRequest(pr *bitbucket.PullRequest) {
	changed := d.adoptPullRequest(pr)
	d.mode = ModePullRequest
	if changed {
		d.resetPullRequestView()
	}
}

// UpdatePullRequestData refreshes the pull request without changing the mode,
// which is what the background detail fetch does while the user reads a
// pipeline.
func (d *DetailView) UpdatePullRequestData(pr *bitbucket.PullRequest) {
	if d.adoptPullRequest(pr) {
		d.resetPullRequestView()
	}
}

// adoptPullRequest stores pr and reports whether it is a different one.
func (d *DetailView) adoptPullRequest(pr *bitbucket.PullRequest) bool {
	prevKey := ""
	if d.pr != nil {
		prevKey = d.pr.Key()
	}
	d.pr = pr
	if pr == nil {
		return prevKey != ""
	}
	return pr.Key() != prevKey
}

func (d *DetailView) resetPullRequestView() {
	d.scrollY = 0
	d.listCursor = 0
	d.activeTab = TabOverview
	d.statuses = nil
	d.statusesLoaded = false
	d.clearPipeline()
}

func (d *DetailView) clearPipeline() {
	d.pipeline = nil
	d.steps = nil
	d.pipelineBuild = 0
	d.pipelineLoading = false
	d.pipelineErr = ""
	d.clearStepLog()
}

func (d *DetailView) clearStepLog() {
	d.stepUUID = ""
	d.stepLog = ""
	d.stepLogLoading = false
	d.stepLogErr = ""
}

// SetStatuses feeds the overview's Builds line. Statuses for another pull
// request are dropped.
func (d *DetailView) SetStatuses(key string, statuses []bitbucket.CommitStatus) {
	if d.pr == nil || d.pr.Key() != key {
		return
	}
	d.statuses = bitbucket.LatestStatuses(statuses)
	d.statusesLoaded = true
}

// SetPipelineLoading switches to the pipeline view with a placeholder body
// while the run is fetched. A call for another pull request is dropped.
func (d *DetailView) SetPipelineLoading(key string, buildNumber int) {
	if d.pr == nil || d.pr.Key() != key {
		return
	}
	d.clearPipeline()
	d.mode = ModePipeline
	d.pipelineBuild = buildNumber
	d.pipelineLoading = true
	d.scrollY = 0
	d.listCursor = 0
}

// SetPipeline shows a pipeline run and its steps. A run for another pull
// request is dropped.
func (d *DetailView) SetPipeline(key string, p *bitbucket.Pipeline, steps []bitbucket.PipelineStep) {
	if d.pr == nil || d.pr.Key() != key {
		return
	}
	d.clearStepLog()
	d.mode = ModePipeline
	d.pipeline = p
	d.steps = steps
	d.pipelineLoading = false
	d.pipelineErr = ""
	if p != nil {
		d.pipelineBuild = p.BuildNumber
	}
	d.scrollY = 0
	d.listCursor = 0
}

// SetPipelineError shows why a pipeline run could not be fetched. An error for
// another pull request is dropped.
func (d *DetailView) SetPipelineError(key, msg string) {
	if d.pr == nil || d.pr.Key() != key {
		return
	}
	d.mode = ModePipeline
	d.pipelineErr = msg
	d.pipelineLoading = false
	d.scrollY = 0
}

// SetStepLogLoading switches to the step log view with a placeholder body. A
// call for another pipeline, or another step, is dropped.
func (d *DetailView) SetStepLogLoading(pipelineUUID, stepUUID string) {
	if !d.pipelineMatches(pipelineUUID) {
		return
	}
	d.mode = ModeStepLog
	d.stepUUID = stepUUID
	d.stepLog = ""
	d.stepLogErr = ""
	d.stepLogLoading = true
	d.scrollY = 0
}

// SetStepLog shows a step's log text. A log for another pipeline, or for a step
// the user has already navigated away from, is dropped.
func (d *DetailView) SetStepLog(pipelineUUID, stepUUID, text string) {
	if !d.stepMatches(pipelineUUID, stepUUID) {
		return
	}
	d.mode = ModeStepLog
	d.stepUUID = stepUUID
	d.stepLog = text
	d.stepLogErr = ""
	d.stepLogLoading = false
	d.scrollY = 0
}

// SetStepLogError shows why a step log could not be fetched.
func (d *DetailView) SetStepLogError(pipelineUUID, stepUUID, msg string) {
	if !d.stepMatches(pipelineUUID, stepUUID) {
		return
	}
	d.mode = ModeStepLog
	d.stepUUID = stepUUID
	d.stepLogErr = msg
	d.stepLogLoading = false
	d.scrollY = 0
}

func (d *DetailView) pipelineMatches(pipelineUUID string) bool {
	return d.pipeline != nil && d.pipeline.UUID == pipelineUUID
}

// stepMatches reports whether a step-log response is still the one the user is
// waiting on. The check is identity-based: Back() clears stepUUID and leaves
// ModePipeline, so a response that lands after the user walked out of the log
// is dropped instead of yanking them back into it.
func (d *DetailView) stepMatches(pipelineUUID, stepUUID string) bool {
	if !d.pipelineMatches(pipelineUUID) {
		return false
	}
	return d.mode == ModeStepLog && d.stepUUID != "" && d.stepUUID == stepUUID
}

// Pipeline returns the pipeline run in view, nil when there is none.
func (d *DetailView) Pipeline() *bitbucket.Pipeline { return d.pipeline }

// SelectedStep returns the step under the block cursor, nil outside the
// pipeline view.
func (d *DetailView) SelectedStep() *bitbucket.PipelineStep {
	if d.mode != ModePipeline {
		return nil
	}
	if d.listCursor < 0 || d.listCursor >= len(d.steps) {
		return nil
	}
	return &d.steps[d.listCursor]
}

// Back walks one step out of the drill-down: step log to pipeline, pipeline to
// pull request. It reports whether the mode changed.
func (d *DetailView) Back() bool {
	switch d.mode {
	case ModeStepLog:
		d.clearStepLog()
		d.mode = ModePipeline
		d.scrollY = 0
		return true
	case ModePipeline:
		d.clearPipeline()
		d.mode = ModePullRequest
		d.scrollY = 0
		d.listCursor = 0
		return true
	default:
		return false
	}
}

// SetRepo shows a repository's metadata.
func (d *DetailView) SetRepo(repo *RepoItem) {
	d.repo = repo
	d.mode = ModeRepo
	d.scrollY = 0
}

// SetSplash shows the startup screen.
func (d *DetailView) SetSplash(info SplashInfo) {
	d.splash = info
	d.mode = ModeSplash
	d.scrollY = 0
}

// SetSize records the panel geometry; it must precede View.
func (d *DetailView) SetSize(w, h int) { d.width = w; d.height = h }

// SetFocused toggles focus, resetting the block cursor when focus is lost.
func (d *DetailView) SetFocused(focused bool) {
	if d.focused && !focused {
		d.listCursor = 0
	}
	d.focused = focused
}

// ActiveTab returns the tab in view.
func (d *DetailView) ActiveTab() DetailTab { return d.activeTab }

// SetActiveTab switches tabs and rewinds the body.
func (d *DetailView) SetActiveTab(tab DetailTab) {
	d.activeTab = tab
	d.scrollY = 0
	d.listCursor = 0
}

// NextTab moves to the following visible tab.
func (d *DetailView) NextTab() { d.cycleTab(1) }

// PrevTab moves to the preceding visible tab.
func (d *DetailView) PrevTab() { d.cycleTab(-1) }

func (d *DetailView) cycleTab(delta int) {
	tabs := d.visibleTabs()
	if len(tabs) == 0 {
		return
	}
	for i, t := range tabs {
		if t == d.activeTab {
			d.SetActiveTab(tabs[((i+delta)%len(tabs)+len(tabs))%len(tabs)])
			return
		}
	}
	d.SetActiveTab(tabs[0])
}

func (d *DetailView) visibleTabs() []DetailTab {
	labels := d.tabLabels()
	tabs := make([]DetailTab, len(labels))
	for i, l := range labels {
		tabs[i] = l.tab
	}
	return tabs
}

type tabLabel struct {
	tab   DetailTab
	label string
}

// tabLabels is the tab set for the current mode. With a single tab the bar is
// not drawn at all, so the title stays the plain mode title until step 2 adds
// Diff, Commits and Comments here.
func (d *DetailView) tabLabels() []tabLabel {
	if d.mode != ModePullRequest || d.pr == nil {
		return nil
	}
	return []tabLabel{{tab: TabOverview, label: "Overview"}}
}

// ClickTab switches tabs from a click on the title bar. It is a no-op while the
// tab bar is not drawn.
func (d *DetailView) ClickTab(x int) {
	labels := d.tabLabels()
	if len(labels) < 2 {
		return
	}
	const sepW = 3 // " - "
	pos := lipgloss.Width(d.titlePrefix()) + sepW
	if x < pos {
		return
	}
	for i, tl := range labels {
		zoneEnd := pos + len(tl.label) + sepW
		if i == len(labels)-1 {
			zoneEnd = pos + len(tl.label) + 10
		}
		if x >= pos && x < zoneEnd {
			d.SetActiveTab(tl.tab)
			return
		}
		pos = zoneEnd
	}
}

// ScrollBy moves the block cursor in list modes and the line offset otherwise.
func (d *DetailView) ScrollBy(delta int) {
	if d.IsListTab() {
		d.listCursor = clampIndex(d.listCursor+delta, d.listTabItemCount())
		return
	}
	d.scrollY = max(d.scrollY+delta, 0)
}

// ClickItem selects the block under relY. A double-click on a truncated block
// expands it.
func (d *DetailView) ClickItem(relY int) tea.Cmd {
	if !d.IsListTab() {
		return nil
	}
	blockWidth := max(d.width-2, 10) - 1
	blocks := d.renderActiveBlocks(blockWidth)
	if len(blocks) == 0 {
		return nil
	}

	clickedLine := d.scrollY + relY - 1 - len(d.listModeHeader(max(d.width-2, 10)))
	if clickedLine < 0 {
		return nil
	}

	linePos := 0
	for i, block := range blocks {
		displayH := displayBlockHeight(block)
		if clickedLine >= linePos && clickedLine < linePos+displayH {
			d.listCursor = i
			if d.dblClick.Click(i) && len(block) > maxBlockLines {
				return func() tea.Msg {
					return ExpandBlockMsg{Title: "Details", Lines: block}
				}
			}
			return nil
		}
		linePos += displayH + 1
	}
	return nil
}

// IsListTab reports whether the panel is a block list rather than scrolling
// text. Only the pipeline view is.
func (d *DetailView) IsListTab() bool { return d.mode == ModePipeline }

// listTabItemCount is the number of blocks in list mode, 0 in text modes.
func (d *DetailView) listTabItemCount() int {
	if d.mode != ModePipeline {
		return 0
	}
	return len(d.steps)
}

// VisibleRows is the number of body lines the panel can show.
func (d *DetailView) VisibleRows() int { return max(d.height-2, 1) }

// Init satisfies the bubbletea component shape; the view has no startup work.
func (d *DetailView) Init() tea.Cmd { return nil }

// Update handles keys while the panel is focused.
func (d *DetailView) Update(msg tea.Msg) (*DetailView, tea.Cmd) {
	if !d.focused {
		return d, nil
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return d, nil
	}
	key := km.String()

	if key == "tab" {
		d.NextTab()
		return d, nil
	}
	if key == "enter" || key == " " {
		return d.handleActivation()
	}

	nav := components.NavNone
	if d.ResolveNav != nil {
		nav = d.ResolveNav(key)
	}
	switch nav {
	case components.NavNone:
	case components.NavDown:
		d.handleCursorDown()
	case components.NavUp:
		d.handleCursorUp()
	case components.NavHalfDown:
		d.handleHalfPage(d.VisibleRows() / 2)
	case components.NavHalfUp:
		d.handleHalfPage(-d.VisibleRows() / 2)
	case components.NavTop:
		d.scrollY = 0
		d.listCursor = 0
	case components.NavBottom:
		if count := d.listTabItemCount(); count > 0 {
			d.listCursor = count - 1
		} else {
			d.scrollY = math.MaxInt32
		}
	}
	return d, nil
}

func (d *DetailView) handleCursorDown() {
	count := d.listTabItemCount()
	if count == 0 {
		d.scrollY++
		return
	}
	if d.listCursor < count-1 {
		d.listCursor++
	} else {
		d.listCursor = 0
	}
}

func (d *DetailView) handleCursorUp() {
	count := d.listTabItemCount()
	if count == 0 {
		if d.scrollY > 0 {
			d.scrollY--
		}
		return
	}
	if d.listCursor > 0 {
		d.listCursor--
	} else {
		d.listCursor = count - 1
	}
}

func (d *DetailView) handleHalfPage(delta int) {
	if count := d.listTabItemCount(); count > 0 {
		d.listCursor = clampIndex(d.listCursor+delta, count)
		return
	}
	d.scrollY = max(d.scrollY+delta, 0)
}

// handleActivation implements the Enter precedence in the pipeline view: a step
// with a UUID opens its log, otherwise a truncated block expands.
func (d *DetailView) handleActivation() (*DetailView, tea.Cmd) {
	if !d.IsListTab() || d.listCursor < 0 || d.listCursor >= len(d.blocks) {
		return d, nil
	}
	if d.listCursor < len(d.blockKeys) && d.blockKeys[d.listCursor] != "" && d.pipeline != nil {
		pipelineUUID := d.pipeline.UUID
		stepUUID := d.blockKeys[d.listCursor]
		return d, func() tea.Msg {
			return StepSelectedMsg{PipelineUUID: pipelineUUID, StepUUID: stepUUID}
		}
	}
	block := d.blocks[d.listCursor]
	if len(block) > maxBlockLines {
		return d, func() tea.Msg {
			return ExpandBlockMsg{Title: "Details", Lines: block}
		}
	}
	return d, nil
}

// View renders the panel for the current mode.
func (d *DetailView) View() string {
	contentWidth, innerH := components.PanelDimensions(d.width, d.height)

	switch d.mode {
	case ModeSplash:
		return d.renderSplash(contentWidth, innerH)
	case ModeRepo:
		if d.repo != nil {
			return d.renderRepo(contentWidth, innerH)
		}
	case ModePullRequest:
		if d.pr == nil {
			placeholder := lipgloss.NewStyle().Foreground(theme.ColorGray).
				Render("Select a pull request to view details")
			return components.RenderPanel(d.buildTitle(titleBudget(contentWidth)),
				clampLine(placeholder, contentWidth), d.width, innerH, d.focused)
		}
	case ModePipeline, ModeStepLog:
	}

	visible := d.VisibleRows()
	contentLines := d.bodyLines(contentWidth, visible)
	totalLines := len(contentLines)
	contentLines = clampLines(d.clampAndSliceScroll(contentLines, visible), contentWidth)

	footer := ""
	if count := d.listTabItemCount(); count > 0 {
		footer = fmt.Sprintf("%d of %d", d.listCursor+1, count)
	}
	scroll := &components.ScrollInfo{Total: totalLines, Visible: visible, Offset: d.scrollY}
	return components.RenderPanelFull(d.buildTitle(titleBudget(contentWidth)), footer,
		strings.Join(contentLines, "\n"), d.width, innerH, d.focused, scroll)
}

// titleBudget is the room the top border leaves for the title: the panel's
// content width less the column the closing "╮" needs.
func titleBudget(contentWidth int) int { return max(contentWidth-1, 0) }

// clampLine truncates s to width display columns, keeping ANSI intact.
func clampLine(s string, width int) string {
	if ansi.StringWidth(s) > width {
		return ansi.Truncate(s, width, "")
	}
	return s
}

// clampLines truncates every body line to the panel's content width in place.
// renderPanelImpl pads short lines but never truncates long ones, so a single
// over-wide line (splash art, a placeholder, an unbreakable URL out of glamour)
// would widen the whole layout and push the neighbouring column off screen.
func clampLines(lines []string, width int) []string {
	for i, line := range lines {
		lines[i] = clampLine(line, width)
	}
	return lines
}

func (d *DetailView) bodyLines(contentWidth, visible int) []string {
	switch d.mode {
	case ModePipeline:
		header := d.listModeHeader(contentWidth)
		if d.pipelineLoading || d.pipelineErr != "" || d.pipeline == nil {
			return header
		}
		return append(header, d.renderBlockList(contentWidth, visible, len(header))...)
	case ModeStepLog:
		return d.renderStepLog(contentWidth)
	case ModePullRequest, ModeRepo, ModeSplash:
		return d.renderOverview(contentWidth)
	default:
		return []string{" No content."}
	}
}

// titlePrefix is the mode title, without any tab bar.
func (d *DetailView) titlePrefix() string {
	switch d.mode {
	case ModePullRequest:
		if d.pr == nil {
			return "[0] Detail"
		}
		return "[0] #" + strconv.Itoa(d.pr.ID) + " " + d.pr.RepoSlug()
	case ModePipeline:
		return "[0] Pipeline #" + strconv.Itoa(d.pipelineBuild)
	case ModeStepLog:
		return "[0] Pipeline #" + strconv.Itoa(d.pipelineBuild) + " · " + d.currentStepName()
	case ModeRepo:
		if d.repo == nil {
			return "[0] Repo"
		}
		return "[0] Repo: " + d.repo.Label()
	case ModeSplash:
		return "[0] lazybitbucket"
	default:
		return "[0] Detail"
	}
}

// buildTitle appends the tab bar to the mode title, but only once there are at
// least two tabs to choose between.
func (d *DetailView) buildTitle(maxWidth int) string {
	prefix := d.titlePrefix()
	labels := d.tabLabels()
	if len(labels) < 2 {
		return components.TruncateEnd(prefix, maxWidth)
	}

	activeStyle := lipgloss.NewStyle().Foreground(theme.ColorGreen).Bold(true)
	inactiveStyle := lipgloss.NewStyle().Foreground(theme.ColorWhite)
	sep := lipgloss.NewStyle().Foreground(theme.ColorGray).Render(" - ")

	parts := make([]string, 0, len(labels))
	for _, t := range labels {
		if t.tab == d.activeTab {
			parts = append(parts, activeStyle.Render(t.label))
		} else {
			parts = append(parts, inactiveStyle.Render(t.label))
		}
	}
	return prefix + sep + strings.Join(parts, sep)
}

func (d *DetailView) currentStepName() string {
	for i := range d.steps {
		if d.steps[i].UUID == d.stepUUID {
			return d.steps[i].DisplayName(i)
		}
	}
	return "step"
}

// ---------------------------------------------------------------------------
// Overview (pull request) rendering
// ---------------------------------------------------------------------------

func (d *DetailView) renderOverview(width int) []string {
	if d.pr == nil {
		return []string{" No content."}
	}
	pr := d.pr
	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)
	bold := lipgloss.NewStyle().Bold(true)

	var lines []string
	for _, l := range wrapText(pr.Title, width-2) {
		lines = append(lines, " "+bold.Render(l))
	}

	identity := "#" + strconv.Itoa(pr.ID) + " · " + pr.RepoFullName() + " · " +
		stateStyle(pr.State).Render(string(pr.State))
	if pr.Draft {
		identity += gray.Render(" · draft")
	}
	lines = append(lines, " "+identity)
	lines = append(lines, " "+components.TruncateEnd(pr.SourceBranch()+" → "+pr.DestinationBranch(), width-2))

	meta := fmt.Sprintf("Author: %s   Created %s ago · Updated %s ago · %d comments · %d tasks",
		theme.AuthorRender(pr.Author.Name()), timeAgo(pr.CreatedOn), timeAgo(pr.UpdatedOn),
		pr.CommentCount, pr.TaskCount)
	lines = append(lines, " "+meta)
	lines = append(lines, " "+d.reviewersLine())
	lines = append(lines, d.buildsLines(width)...)

	lines = append(lines, " "+gray.Render(strings.Repeat("─", max(width-2, 1))))
	lines = append(lines, d.renderDescription(width)...)
	return lines
}

func (d *DetailView) reviewersLine() string {
	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)
	green := lipgloss.NewStyle().Foreground(theme.ColorGreen)
	red := lipgloss.NewStyle().Foreground(theme.ColorRed)

	pr := d.pr
	if !pr.HasReviewData() {
		return "Reviewers: " + gray.Render("loading…")
	}

	seen := make(map[string]bool)
	var parts []string
	appendReviewer := func(acct bitbucket.Account, state bitbucket.ParticipantState, approved bool) {
		if seen[acct.UUID] {
			return
		}
		seen[acct.UUID] = true
		switch {
		case approved || state == bitbucket.ParticipantApproved:
			parts = append(parts, green.Render("✓ ")+theme.AuthorRender(acct.Name()))
		case state == bitbucket.ParticipantChangesRequested:
			parts = append(parts, red.Render("✗ ")+theme.AuthorRender(acct.Name()))
		default:
			parts = append(parts, gray.Render("· ")+theme.AuthorRender(acct.Name()))
		}
	}

	for _, p := range pr.Participants {
		if p.Role != bitbucket.RoleReviewer && !p.Approved && p.State == "" {
			continue
		}
		appendReviewer(p.User, p.State, p.Approved)
	}
	for _, reviewer := range pr.Reviewers {
		appendReviewer(reviewer, "", false)
	}

	if len(parts) == 0 {
		return "Reviewers: " + gray.Render("(none)")
	}
	return "Reviewers: " + strings.Join(parts, "   ")
}

func (d *DetailView) buildsLines(width int) []string {
	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)
	if !d.statusesLoaded {
		return []string{" Builds: " + gray.Render("loading…")}
	}
	if len(d.statuses) == 0 {
		return []string{" Builds: " + gray.Render("none")}
	}
	lines := make([]string, 0, len(d.statuses))
	for _, status := range d.statuses {
		name := status.Name
		if name == "" {
			name = status.Key
		}
		label := fmt.Sprintf("%s %s  %s (%s)",
			buildGlyph(status.State, false), string(status.State),
			components.TruncateEnd(name, max(width-30, 8)), timeAgo(status.UpdatedOn))
		lines = append(lines, " Builds: "+label)
	}
	return lines
}

func (d *DetailView) renderDescription(width int) []string {
	if d.pr.Description == "" {
		return []string{" " + lipgloss.NewStyle().Foreground(theme.ColorGray).Render("(no description)")}
	}
	rendered := d.renderer.Render(d.pr.Description, width-1)
	if len(rendered) == 0 {
		return []string{" " + lipgloss.NewStyle().Foreground(theme.ColorGray).Render("(no description)")}
	}
	lines := make([]string, 0, len(rendered))
	for _, l := range rendered {
		lines = append(lines, " "+l)
	}
	return lines
}

// ---------------------------------------------------------------------------
// Pipeline rendering
// ---------------------------------------------------------------------------

// listModeHeader is the fixed preamble drawn above the block list. Its height
// is what the block hit-test and auto-scroll offset by.
func (d *DetailView) listModeHeader(width int) []string {
	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)

	if d.pipelineErr != "" {
		return []string{" " + d.theme.ErrorText.Render(components.TruncateEnd(d.pipelineErr, width-2))}
	}
	if d.pipelineLoading || d.pipeline == nil {
		return []string{" " + gray.Render(fmt.Sprintf("Loading pipeline #%d…", d.pipelineBuild))}
	}

	p := d.pipeline
	summary := p.State.Summary()
	head := fmt.Sprintf("Pipeline #%d · %s · %s · %s",
		p.BuildNumber, pipelineResultStyle(summary).Render(summary),
		formatDuration(p.Duration()), p.Trigger.Name)
	if p.Creator != nil {
		head += " · by " + theme.AuthorRender(p.Creator.Name())
	}
	if age := timeAgo(p.CreatedOn); age != "" {
		head += " · " + age + " ago"
	}

	lines := []string{" " + head, " " + gray.Render(pipelineTargetLine(p.Target))}
	if p.State.Result != nil && p.State.Result.Error != nil {
		errText := p.State.Result.Error.Key
		if msg := p.State.Result.Error.Message; msg != "" {
			errText = strings.TrimPrefix(errText+": "+msg, ": ")
		}
		for _, l := range wrapText(errText, width-2) {
			lines = append(lines, " "+d.theme.ErrorText.Render(l))
		}
	}
	lines = append(lines, " "+gray.Render(strings.Repeat("─", max(width-2, 1))))
	return lines
}

func pipelineTargetLine(target bitbucket.PipelineTarget) string {
	if pr := target.PullRequest; pr != nil {
		return fmt.Sprintf("PR #%d %s → %s", pr.ID, target.Source, target.Destination)
	}
	if branch := target.BranchName(); branch != "" {
		return "branch " + branch
	}
	return "target " + target.Type
}

// renderStepBlocks builds one block per pipeline step: a headline with glyph,
// name and duration, plus the error message lines of a failed step.
func (d *DetailView) renderStepBlocks(width int) [][]string {
	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)
	blocks := make([][]string, 0, len(d.steps))

	nameWidth := max(width-14, 8)
	for i := range d.steps {
		step := d.steps[i]
		result := stepResultName(step)
		glyph, style := stepGlyph(step)

		duration := formatDuration(step.Duration())
		if result == resultNotRun {
			duration = notRunLabel
		}

		name := components.TruncateEnd(step.DisplayName(i), nameWidth)
		var headline string
		if result == resultNotRun || result == "EXPIRED" {
			// A step that never ran is dimmed whole, glyph included.
			headline = gray.Render(glyph + " " + padRight(name, nameWidth) + " " + duration)
		} else {
			headline = style.Render(glyph) + " " + padRight(name, nameWidth) + " " + gray.Render(duration)
		}

		block := []string{headline}
		if step.State.Result != nil && step.State.Result.Error != nil {
			errText := step.State.Result.Error.Key
			if msg := step.State.Result.Error.Message; msg != "" {
				errText = strings.TrimPrefix(errText+": "+msg, ": ")
			}
			for _, l := range wrapText(errText, width-4) {
				block = append(block, "   "+d.theme.ErrorText.Render(l))
			}
		}
		blocks = append(blocks, block)
	}
	return blocks
}

// stepResultName is the step's terminal result, "" while it has not finished.
func stepResultName(step bitbucket.PipelineStep) string {
	if step.State.Result == nil {
		return ""
	}
	return step.State.Result.Name
}

func stepGlyph(step bitbucket.PipelineStep) (string, lipgloss.Style) {
	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)
	switch stepResultName(step) {
	case "SUCCESSFUL":
		return "✔", lipgloss.NewStyle().Foreground(theme.ColorGreen)
	case "FAILED", "ERROR":
		return "✘", lipgloss.NewStyle().Foreground(theme.ColorRed)
	case "STOPPED":
		return "■", gray
	case resultNotRun, "EXPIRED":
		return "-", gray
	default:
		if step.State.Name == "IN_PROGRESS" {
			return "●", lipgloss.NewStyle().Foreground(theme.ColorYellow)
		}
		return "·", gray
	}
}

// pipelineResultStyle colours a pipeline or step outcome word.
func pipelineResultStyle(name string) lipgloss.Style {
	switch name {
	case "SUCCESSFUL":
		return lipgloss.NewStyle().Foreground(theme.ColorGreen)
	case "FAILED", "ERROR":
		return lipgloss.NewStyle().Foreground(theme.ColorRed)
	case "STOPPED", "EXPIRED", resultNotRun:
		return lipgloss.NewStyle().Foreground(theme.ColorGray)
	default:
		return lipgloss.NewStyle().Foreground(theme.ColorYellow)
	}
}

// formatDuration renders a run or step duration compactly: "35s", "9m46s",
// "1h05m".
func formatDuration(d time.Duration) string {
	secs := max(int(d.Seconds()), 0)
	switch {
	case secs < 60:
		return strconv.Itoa(secs) + "s"
	case secs < 3600:
		return fmt.Sprintf("%dm%02ds", secs/60, secs%60)
	default:
		return fmt.Sprintf("%dh%02dm", secs/3600, (secs%3600)/60)
	}
}

// ---------------------------------------------------------------------------
// Step log rendering
// ---------------------------------------------------------------------------

func (d *DetailView) renderStepLog(width int) []string {
	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)
	switch {
	case d.stepLogErr != "":
		lines := make([]string, 0, 2)
		for _, l := range wrapText(d.stepLogErr, width-2) {
			lines = append(lines, " "+d.theme.ErrorText.Render(l))
		}
		return lines
	case d.stepLogLoading:
		return []string{" " + gray.Render("Loading log…")}
	case strings.TrimSpace(d.stepLog) == "":
		return []string{" " + gray.Render("(empty log)")}
	}

	var lines []string
	for _, raw := range strings.Split(strings.ReplaceAll(d.stepLog, "\r\n", "\n"), "\n") {
		for _, chunk := range hardWrapLine(raw, width-2) {
			lines = append(lines, " "+chunk)
		}
	}
	return lines
}

// ---------------------------------------------------------------------------
// Repo and splash rendering
// ---------------------------------------------------------------------------

func (d *DetailView) renderRepo(contentWidth, innerH int) string {
	repo := d.repo
	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)
	val := d.theme.ValueStyle

	lines := []string{
		fmt.Sprintf(" %-12s %s", "Slug:", val.Render(repo.Label())),
		fmt.Sprintf(" %-12s %s", "Name:", val.Render(repo.Name)),
	}
	if repo.FullName != "" {
		lines = append(lines, fmt.Sprintf(" %-12s %s", "Full name:", val.Render(repo.FullName)))
	}
	if repo.MainBranch != "" {
		lines = append(lines, fmt.Sprintf(" %-12s %s", "Main branch:", val.Render(repo.MainBranch)))
	}
	if repo.OpenCount >= 0 {
		lines = append(lines, fmt.Sprintf(" %-12s %s", "Open PRs:", val.Render(strconv.Itoa(repo.OpenCount))))
	}
	if repo.URL != "" {
		lines = append(lines, fmt.Sprintf(" %-12s %s", "URL:", gray.Render(repo.URL)))
	}

	title := components.TruncateEnd(d.titlePrefix(), titleBudget(contentWidth))
	return components.RenderPanel(title, strings.Join(clampLines(lines, contentWidth), "\n"),
		d.width, innerH, d.focused)
}

// splashArtWidth is the block banner's 51 columns plus its two-column indent.
// A narrower panel gets a plain heading instead of a chopped-up banner.
const splashArtWidth = 53

// splashArt is "lazybitbucket" as a 51-column block banner.
const splashArt = `█   ███ ███ █ █ ██  ███ ███ ██  █ █ ███ █ █ ███ ███
█   █ █   █ █ █ █ █  █   █  █ █ █ █ █   █ █ █    █
█   ███  █  ███ ██   █   █  ██  █ █ █   ██  ██   █
█   █ █ █    █  █ █  █   █  █ █ █ █ █   █ █ █    █
███ █ █ ███  █  ██  ███  █  ██  ███ ███ █ █ ███  █`

func (d *DetailView) renderSplash(contentWidth, innerH int) string {
	green := lipgloss.NewStyle().Foreground(theme.ColorGreen).Bold(true)
	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)
	label := lipgloss.NewStyle().Foreground(theme.ColorGreen)
	val := lipgloss.NewStyle()

	artLines := strings.Split(splashArt, "\n")
	lines := make([]string, 0, len(artLines)+9)
	if contentWidth >= splashArtWidth {
		for _, l := range artLines {
			lines = append(lines, "  "+green.Render(l))
		}
	} else {
		lines = append(lines, "  "+green.Render("lazybitbucket"))
	}

	s := d.splash
	repos := strconv.Itoa(s.RepoCount)
	if s.RepoNote != "" {
		repos = s.RepoNote
	}

	lines = append(lines,
		"",
		gray.Render("  lazybitbucket "+s.Version),
		"",
		"  "+strings.Repeat("─", 30),
		"",
		fmt.Sprintf("  %s      %s", label.Render("Auth:"), val.Render(s.AuthMethod)),
		fmt.Sprintf("  %s %s", label.Render("Workspace:"), val.Render(s.Workspace)),
		fmt.Sprintf("  %s      %s", label.Render("User:"), val.Render(s.User)),
		fmt.Sprintf("  %s     %s", label.Render("Repos:"), val.Render(repos)),
	)

	title := components.TruncateEnd(d.titlePrefix(), titleBudget(contentWidth))
	return components.RenderPanel(title, strings.Join(clampLines(lines, contentWidth), "\n"),
		d.width, innerH, d.focused)
}

// ---------------------------------------------------------------------------
// Block list engine
// ---------------------------------------------------------------------------

// renderActiveBlocks dispatches block rendering for the current mode.
func (d *DetailView) renderActiveBlocks(width int) [][]string {
	if d.mode != ModePipeline {
		return nil
	}
	return d.renderStepBlocks(width)
}

// buildBlockKeys pairs each block with the step it renders, so activating a
// block can name the step to fetch a log for.
func (d *DetailView) buildBlockKeys(blocks [][]string) []string {
	keys := make([]string, len(blocks))
	for i := range blocks {
		if i < len(d.steps) {
			keys[i] = d.steps[i].UUID
		}
	}
	return keys
}

// renderBlockList draws the blocks with a cursor bar, collapsing anything
// longer than maxBlockLines. headerLines is how many lines precede the list, so
// auto-scroll keeps the selected block visible in the whole body.
func (d *DetailView) renderBlockList(contentWidth, visible, headerLines int) []string {
	blockWidth := contentWidth - 1
	blocks := d.renderActiveBlocks(blockWidth)

	d.listCursor = clampIndex(d.listCursor, len(blocks))
	d.blocks = blocks
	d.blockKeys = d.buildBlockKeys(blocks)

	bar := lipgloss.NewStyle().Foreground(theme.ColorBlue).Render("▎")
	ellipsis := lipgloss.NewStyle().Foreground(theme.ColorGray).Render("    ...")
	sep := strings.Repeat("─", max(blockWidth, 1))

	var lines []string
	for i, block := range blocks {
		displayBlock := block
		truncated := false
		if len(block) > maxBlockLines {
			displayBlock = block[:maxBlockLines]
			truncated = true
		}
		prefix := " "
		if i == d.listCursor && d.focused {
			prefix = bar
		}
		for _, line := range displayBlock {
			lines = append(lines, prefix+line)
		}
		if truncated {
			lines = append(lines, prefix+ellipsis)
		}
		if i < len(blocks)-1 {
			lines = append(lines, " "+sep)
		}
	}

	d.autoScrollToBlock(blocks, visible, headerLines)
	return lines
}

// displayBlockHeight is how many lines a block occupies once collapsed.
func displayBlockHeight(block []string) int {
	if len(block) > maxBlockLines {
		return maxBlockLines + 1
	}
	return len(block)
}

// autoScrollToBlock moves scrollY so the whole selected block stays visible,
// with one line of margin when the panel is tall enough to afford it.
func (d *DetailView) autoScrollToBlock(blocks [][]string, visible, headerLines int) {
	if len(blocks) == 0 {
		return
	}
	lineStart := headerLines
	for i := 0; i < d.listCursor && i < len(blocks); i++ {
		lineStart += displayBlockHeight(blocks[i]) + 1
	}
	margin := 1
	if visible <= 3 {
		margin = 0
	}
	if lineStart-margin < d.scrollY {
		d.scrollY = lineStart - margin
	}
	blockEnd := lineStart + displayBlockHeight(blocks[d.listCursor])
	if blockEnd+margin > d.scrollY+visible {
		d.scrollY = blockEnd + margin - visible
	}
}

// clampAndSliceScroll clamps scrollY to the content and returns the visible
// window of lines.
func (d *DetailView) clampAndSliceScroll(contentLines []string, visible int) []string {
	maxScroll := max(len(contentLines)-visible, 0)
	d.scrollY = min(max(d.scrollY, 0), maxScroll)
	scrolled := contentLines[min(d.scrollY, len(contentLines)):]
	if len(scrolled) > visible {
		scrolled = scrolled[:visible]
	}
	return scrolled
}

// clampIndex keeps i inside [0, count), returning 0 for an empty range.
func clampIndex(i, count int) int {
	if count <= 0 {
		return 0
	}
	return min(max(i, 0), count-1)
}
