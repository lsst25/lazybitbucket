package views

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/diff"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
	"github.com/lsst25/lazybitbucket/pkg/tui/theme"
)

// DetailTab is a tab within the pull request view: the four faces of one pull
// request, each backed by its own request.
type DetailTab int

// Tabs of the pull request view.
const (
	TabOverview DetailTab = iota
	TabFiles
	TabCommits
	TabComments
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
	ModeFileDiff
)

// listKind is which block list the panel is drawing, resolved from the mode and
// the active tab together. It is the single place the two axes meet: every
// body, cursor, activation and hit-test decision switches on this rather than
// re-deriving the combination.
type listKind int

// The block lists the detail panel can draw. listNone means the body is
// scrolling text rather than a list.
const (
	listNone listKind = iota
	listSteps
	listFiles
	listCommits
	listThreads
	listDiffLines
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
	// pipelineOwner is who the run in view belongs to: a pull request key in
	// the pull request view, an opaque run key in the pipelines view. A late
	// response is matched against it rather than against the pull request,
	// because a standalone run has no pull request to match.
	pipelineOwner     string
	pipelineOwnerIsPR bool
	// stepsPending marks a standalone run whose steps are still being fetched,
	// and stepsErr why that fetch failed. Neither applies to the pull request
	// view, which fetches the run and its steps in one go.
	stepsPending bool
	stepsErr     string

	stepUUID       string
	stepLog        string
	stepLogLoading bool
	stepLogErr     string

	files    tabData[bitbucket.DiffStatEntry]
	commits  tabData[bitbucket.Commit]
	threads  tabData[bitbucket.Thread]
	fileDiff fileDiffState

	repo   *RepoItem
	splash SplashInfo

	mode       MainMode
	activeTab  DetailTab
	scrollY    int
	listCursor int
	blocks     [][]string
	blockKeys  []string
	dblClick   components.DblClickDetector
	// blockCache memoises the rendered blocks of the active list. Rendering a
	// diff runs a syntax highlighter and a thread runs glamour, neither of
	// which may happen once per keystroke.
	blockCache blockCache
	// tabZones is where each tab label landed in the title the last View drew,
	// so a click hit-tests against what is on screen rather than against a
	// recomputed guess.
	tabZones []tabZone

	width   int
	height  int
	focused bool
	theme   *theme.Theme

	renderer MarkdownRenderer
	// codeStyle is the chroma style name the diff's code column is highlighted
	// with, "" to leave it plain. See ChromaStyleFor.
	codeStyle string
	// ResolveNav is assigned by the app; the view is keyboard-inert until it is.
	ResolveNav components.NavResolver
}

// tabData is one review tab's payload plus the three states it can be in
// before the payload arrives, which the tab label and the body both read.
type tabData[T any] struct {
	items   []T
	loaded  bool
	loading bool
	errMsg  string
}

// set stores a loaded payload, clearing the loading and error states.
func (t *tabData[T]) set(items []T) {
	t.items = items
	t.loaded = true
	t.loading = false
	t.errMsg = ""
}

// setLoading marks a fetch in flight without discarding what is on screen.
func (t *tabData[T]) setLoading() {
	t.loading = true
	t.errMsg = ""
}

// setError records a failed fetch.
func (t *tabData[T]) setError(msg string) {
	t.errMsg = msg
	t.loading = false
}

// reset forgets everything, which is what selecting another pull request does.
func (t *tabData[T]) reset() { *t = tabData[T]{items: nil, loaded: false, loading: false, errMsg: ""} }

// fileDiffState is the ModeFileDiff payload: the file whose diff is on screen,
// the parsed diff, the comment markers per line and the rendered line list the
// cursor indexes.
type fileDiffState struct {
	path    string
	file    *diff.File
	markers map[int]int
	loading bool
	errMsg  string
	// lines is one entry per rendered row, hunk headers included, so the
	// cursor can be resolved to a diff line before the first View call.
	lines []diff.Line
	// highlighter is built once per file, in SetFileDiff. It is nil whenever
	// rendered is not: an external diff renderer owns the whole code column.
	highlighter *highlighter
	// rendered is the external diff renderer's finished code cells, one per
	// line of the diff document this file was parsed out of and indexed by
	// diff.Line.Source, or nil when the built-in colouring is in charge.
	rendered []string
}

// tabZone is one tab label's column span in the rendered title.
type tabZone struct {
	tab   DetailTab
	start int
	end   int
}

// blockCache memoises one list rendering. A cache entry is only valid for the
// list kind and width it was built at.
type blockCache struct {
	blocks [][]string
	kind   listKind
	width  int
	valid  bool
	// rendered marks which blocks of a lazily built list already carry their
	// text. It is nil for the lists that are rendered whole.
	rendered []bool
	// renderRow builds block i of a lazily built list, which is always a
	// single line. nil when the list was rendered whole.
	renderRow func(i int) string
}

// blockOverscan is how many blocks either side of the visible window a lazily
// built list renders anyway, so a scroll clamp a line either way cannot leave
// a blank row on screen.
const blockOverscan = 4

// NewDetailView constructs a detail view rendering Markdown with renderer and
// highlighting diff code with the chroma style codeStyle names; "" leaves the
// diff unhighlighted. See ChromaStyleFor for the mapping from the configured
// renderer style.
func NewDetailView(renderer MarkdownRenderer, codeStyle string) *DetailView {
	return &DetailView{
		theme:     theme.Default,
		mode:      ModePullRequest,
		renderer:  renderer,
		codeStyle: codeStyle,
	}
}

// listKind reports which block list the panel is drawing.
func (d *DetailView) listKind() listKind {
	switch d.mode {
	case ModePipeline:
		return listSteps
	case ModeFileDiff:
		return listDiffLines
	case ModePullRequest:
		if d.pr == nil {
			return listNone
		}
		switch d.activeTab {
		case TabFiles:
			return listFiles
		case TabCommits:
			return listCommits
		case TabComments:
			return listThreads
		case TabOverview:
			return listNone
		default:
			return listNone
		}
	case ModeStepLog, ModeRepo, ModeSplash:
		return listNone
	default:
		return listNone
	}
}

// invalidateBlocks drops the rendered-block cache. Every mutation that can
// change what a block says must call it, or the panel keeps drawing the old
// list.
func (d *DetailView) invalidateBlocks() { d.blockCache.valid = false }

// invalidateBlocksFor drops the cache only when what it holds is one of kinds.
// The cache keys on the list kind anyway, so another kind's entry is rebuilt
// the moment it is asked for; dropping it early only throws away work — a
// review fetch landing while the reader is inside a file diff would re-run the
// highlighter over that diff for nothing.
func (d *DetailView) invalidateBlocksFor(kinds ...listKind) {
	if slices.Contains(kinds, d.blockCache.kind) {
		d.invalidateBlocks()
	}
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
	case ModePipeline, ModeStepLog:
		// A standalone run may sit over a pull request the other view still
		// holds; it is not what the panel is showing.
		if !d.pipelineOwnerIsPR {
			return ""
		}
		return d.pr.Key()
	case ModePullRequest, ModeFileDiff:
		return d.pr.Key()
	default:
		return ""
	}
}

// SetPullRequest shows a pull request. Scroll, tab, mode and the per-tab data
// are only reset when the key changes, so a background refresh does not yank
// the user out of a file diff or a pipeline they are reading.
func (d *DetailView) SetPullRequest(pr *bitbucket.PullRequest) {
	if d.adoptPullRequest(pr) {
		d.mode = ModePullRequest
		d.resetPullRequestView()
		return
	}
	// Same pull request. The repository and splash screens are not this pull
	// request's own drill-down — the app calls SetPullRequest there precisely
	// to come back — so they return to the overview, while a pipeline, a step
	// log or a file diff is left alone: not yanking a reader out of what they
	// are reading is the whole point of the guard.
	switch d.mode {
	case ModeRepo, ModeSplash:
		d.mode = ModePullRequest
		d.resetPullRequestView()
	case ModePipeline, ModeStepLog:
		// A standalone run belongs to the pipelines view, not to this pull
		// request, so coming back to the pull request has to leave it — the
		// guard is only there to protect a reader inside this pull request's
		// own drill-down.
		if !d.pipelineOwnerIsPR {
			d.mode = ModePullRequest
			d.resetPullRequestView()
		}
	case ModePullRequest, ModeFileDiff:
	}
}

// UpdatePullRequestData refreshes the pull request without changing the mode,
// which is what the background detail fetch does while the user reads a
// pipeline.
func (d *DetailView) UpdatePullRequestData(pr *bitbucket.PullRequest) {
	if !d.adoptPullRequest(pr) {
		return
	}
	// A standalone run is not this pull request's drill-down: it belongs to the
	// pipelines view, which may be the one on screen while the pull request
	// list refreshes behind it. The new data is adopted — it is newer — but the
	// run stays, and re-entering the pull request view resets the panel through
	// SetPullRequest anyway.
	switch d.mode {
	case ModePipeline, ModeStepLog:
		if !d.pipelineOwnerIsPR {
			return
		}
	case ModePullRequest, ModeFileDiff, ModeRepo, ModeSplash:
	}
	d.resetPullRequestView()
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
	d.files.reset()
	d.commits.reset()
	d.threads.reset()
	d.clearFileDiff()
	d.clearPipeline()
	// The pull request in view owns any run drilled into from here, so the
	// owner token follows it. That is what used to be the d.pr comparison the
	// pipeline entry points made.
	d.pipelineOwner = ""
	if d.pr != nil {
		d.pipelineOwner = d.pr.Key()
	}
	d.pipelineOwnerIsPR = true
}

// clearPipeline forgets the run and its steps. The owner token is deliberately
// left alone: walking back out of a run and drilling into another one of the
// same pull request must not start dropping its own responses.
func (d *DetailView) clearPipeline() {
	d.pipeline = nil
	d.steps = nil
	d.pipelineBuild = 0
	d.pipelineLoading = false
	d.pipelineErr = ""
	d.stepsPending = false
	d.stepsErr = ""
	d.invalidateBlocks()
	d.clearStepLog()
}

// clearFileDiff forgets the file diff, so a response that lands after the user
// walked out of ModeFileDiff cannot pull them back into it.
func (d *DetailView) clearFileDiff() {
	d.fileDiff = fileDiffState{
		path: "", file: nil, markers: nil, loading: false, errMsg: "",
		lines: nil, highlighter: nil, rendered: nil,
	}
	d.invalidateBlocks()
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
// while the run is fetched. A call for another owner is dropped.
func (d *DetailView) SetPipelineLoading(key string, buildNumber int) {
	if key != d.pipelineOwner {
		return
	}
	d.clearPipeline()
	d.pipelineOwnerIsPR = true
	d.clearFileDiff()
	d.mode = ModePipeline
	d.pipelineBuild = buildNumber
	d.pipelineLoading = true
	d.scrollY = 0
	d.listCursor = 0
}

// SetPipeline shows a pipeline run and its steps. A run for another owner is
// dropped.
//
// A refetch of the run already in view — the read-back after a stop — keeps the
// user's place: the cursor stays on the step it was on, clamped to the steps
// that came back, and so does the scroll offset. Any other run starts at the top.
func (d *DetailView) SetPipeline(key string, p *bitbucket.Pipeline, steps []bitbucket.PipelineStep) {
	if key != d.pipelineOwner {
		return
	}
	d.pipelineOwnerIsPR = true
	d.showRun(p, steps)
}

// showRun is what SetPipeline and SetRun share: the run, its steps and the
// place in the step list they leave the reader at.
func (d *DetailView) showRun(p *bitbucket.Pipeline, steps []bitbucket.PipelineStep) {
	sameRun := d.mode == ModePipeline && d.pipeline != nil && p != nil && d.pipeline.UUID == p.UUID
	d.clearStepLog()
	d.clearFileDiff()
	d.mode = ModePipeline
	d.pipeline = p
	d.steps = steps
	d.pipelineLoading = false
	d.pipelineErr = ""
	d.stepsErr = ""
	d.invalidateBlocks()
	if p != nil {
		d.pipelineBuild = p.BuildNumber
	}
	if sameRun {
		d.listCursor = min(d.listCursor, max(len(steps)-1, 0))
		return
	}
	d.scrollY = 0
	d.listCursor = 0
}

// SetPipelineError shows why a pipeline run could not be fetched. An error for
// another owner is dropped.
func (d *DetailView) SetPipelineError(key, msg string) {
	if key != d.pipelineOwner {
		return
	}
	d.pipelineOwnerIsPR = true
	d.clearFileDiff()
	d.mode = ModePipeline
	d.pipelineErr = msg
	d.pipelineLoading = false
	d.scrollY = 0
	d.invalidateBlocks()
}

// SetRun shows a pipeline run that stands on its own — the pipelines view,
// where there is no pull request behind it. owner is the token later step
// responses are matched against. Nil steps mean they are still being fetched,
// which the body says instead of claiming the run has none.
func (d *DetailView) SetRun(owner string, run *bitbucket.Pipeline, steps []bitbucket.PipelineStep) {
	d.pipelineOwner = owner
	d.pipelineOwnerIsPR = false
	d.showRun(run, steps)
	d.stepsPending = steps == nil
}

// SetRunSteps fills in the steps of a standalone run. Steps for another owner
// are dropped, and the block cursor stays where it is when the run in view is
// the one they belong to — the same rule SetPipeline follows for a refetch.
func (d *DetailView) SetRunSteps(owner string, steps []bitbucket.PipelineStep) {
	if owner != d.pipelineOwner || d.pipelineOwnerIsPR {
		return
	}
	d.steps = steps
	d.stepsPending = false
	d.stepsErr = ""
	d.listCursor = min(d.listCursor, max(len(steps)-1, 0))
	d.invalidateBlocks()
}

// SetRunStepsError reports why a standalone run's steps could not be fetched.
// The run header stays: it came from the list payload and is still true, so
// only the step list is replaced.
func (d *DetailView) SetRunStepsError(owner, msg string) {
	if owner != d.pipelineOwner || d.pipelineOwnerIsPR {
		return
	}
	d.steps = nil
	d.stepsPending = false
	d.stepsErr = msg
	d.invalidateBlocks()
}

// PipelineOwner is the token of whoever the run in view belongs to, so the app
// can tell which run the panel is showing.
func (d *DetailView) PipelineOwner() string { return d.pipelineOwner }

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

// PipelineOwnerIsPR reports whether the run in view is a pull request's own
// drill-down rather than a standalone run of the pipelines view. The two carry
// different owner tokens — a pull request key and a run key — and only the
// second one names a repository and a build number a caller can act on.
func (d *DetailView) PipelineOwnerIsPR() bool { return d.pipelineOwnerIsPR }

// PipelineBuild is the build number of the run the pipeline view shows, 0
// outside it. It is tracked separately from Pipeline() because it is set the
// moment the drill-down starts: a key pressed while the run is still loading
// still has to know which run it is about.
func (d *DetailView) PipelineBuild() int { return d.pipelineBuild }

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

// Back walks one step out of the drill-down: file diff to the Files tab (with
// the cursor left on the file just read), step log to pipeline, pipeline to
// pull request. It reports whether the mode changed.
func (d *DetailView) Back() bool {
	switch d.mode {
	case ModeFileDiff:
		path := d.fileDiff.path
		d.clearFileDiff()
		d.mode = ModePullRequest
		d.activeTab = TabFiles
		d.scrollY = 0
		d.listCursor = max(d.fileIndex(path), 0)
		d.invalidateBlocks()
		return true
	case ModeStepLog:
		d.clearStepLog()
		d.mode = ModePipeline
		d.scrollY = 0
		d.invalidateBlocks()
		return true
	case ModePipeline:
		// A standalone run is not a drill-down out of anything: there is
		// nowhere behind it to go back to, so the key falls through to the
		// app's own handling instead.
		if !d.pipelineOwnerIsPR {
			return false
		}
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
	d.invalidateBlocks()
}

// SetSplash shows the startup screen.
func (d *DetailView) SetSplash(info SplashInfo) {
	d.splash = info
	d.mode = ModeSplash
	d.scrollY = 0
	d.invalidateBlocks()
}

// SetSize records the panel geometry; it must precede View. A width change
// invalidates the rendered blocks, which are laid out for one width.
func (d *DetailView) SetSize(w, h int) {
	if w != d.width {
		d.invalidateBlocks()
	}
	d.width = w
	d.height = h
}

// SetFocused toggles focus. The block cursor is deliberately kept across a
// blur: the cursor bar is only drawn while the panel is focused, so there is
// nothing to hide, and the app recomputes focus on every panel switch — which
// used to throw away the reader's place in a diff or a comment thread each
// time the detail panel merely kept focus.
func (d *DetailView) SetFocused(focused bool) { d.focused = focused }

// ActiveTab returns the tab in view.
func (d *DetailView) ActiveTab() DetailTab { return d.activeTab }

// SetActiveTab switches tabs and rewinds the body. It leaves ModeFileDiff
// first, so a tab key pressed inside a diff lands on the tab rather than on a
// diff wearing another tab's title.
func (d *DetailView) SetActiveTab(tab DetailTab) {
	if d.mode == ModeFileDiff {
		d.clearFileDiff()
		d.mode = ModePullRequest
	}
	d.activeTab = tab
	d.scrollY = 0
	d.listCursor = 0
	d.invalidateBlocks()
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
	// short is the label without its count, which is what the title falls back
	// to when the bar does not fit.
	short string
}

// tabLabels is the tab set for the current mode: the four pull request tabs,
// each carrying the item count once its data has arrived — and, for Comments,
// from the start, because the pull request payload counts its comments. Every
// other mode has no tabs, so the title stays the plain mode title.
func (d *DetailView) tabLabels() []tabLabel {
	if d.mode != ModePullRequest || d.pr == nil {
		return nil
	}
	// The Comments tab counts comments, not threads: comment_count is what the
	// pull request knows in advance, and roots plus replies is the same number
	// once the threads are in hand.
	comments := d.threads.counts()
	comments.known = d.pr.CommentCount
	comments.count = bitbucket.CommentTotal(d.threads.items)
	return []tabLabel{
		{tab: TabOverview, label: "Overview", short: "Overview"},
		countedTab(TabFiles, "Files", d.files.counts()),
		countedTab(TabCommits, "Commits", d.commits.counts()),
		countedTab(TabComments, "Comments", comments),
	}
}

// tabCounts is what a tab label knows about its number: the state of the fetch,
// the count of what it loaded, and the count the pull request itself already
// carries, if any.
type tabCounts struct {
	loading bool
	loaded  bool
	count   int
	// known is the count shown before and instead of the ellipsis while the
	// fetch runs — the pull request payload counts its comments but neither
	// its files nor its commits. unknownCount for a tab that has none.
	known int
}

// counts is the label state of one review tab, before anything the pull request
// payload may already know about it.
func (t *tabData[T]) counts() tabCounts {
	return tabCounts{loading: t.loading, loaded: t.loaded, count: len(t.items), known: unknownCount}
}

// countedTab builds a tab label that shows its count: the number the pull
// request itself carries while there is no answer of the tab's own — before the
// fetch and while one is in flight — the number of what the tab loaded once it
// has landed, and, for a tab the pull request cannot count, an ellipsis while
// the fetch runs and the bare name before it.
func countedTab(tab DetailTab, name string, counts tabCounts) tabLabel {
	label := name
	switch {
	case counts.loading && counts.known >= 0:
		label = withCount(name, counts.known)
	case counts.loading:
		label = name + " (…)"
	case counts.loaded:
		label = withCount(name, counts.count)
	case counts.known >= 0:
		label = withCount(name, counts.known)
	}
	return tabLabel{tab: tab, label: label, short: name}
}

// withCount is a tab name carrying its count.
func withCount(name string, count int) string {
	return name + " (" + strconv.Itoa(count) + ")"
}

// ClickTab switches tabs from a click on the title bar and reports whether the
// tab changed. It hit-tests against the zones the last View recorded, so a
// click always lands on the label the user could see; before the first View, or
// with no tab bar drawn, there is nothing to hit and it is a no-op.
func (d *DetailView) ClickTab(x int) bool {
	for _, zone := range d.tabZones {
		if x < zone.start || x >= zone.end {
			continue
		}
		if zone.tab == d.activeTab {
			return false
		}
		d.SetActiveTab(zone.tab)
		return true
	}
	return false
}

// ScrollBy moves the block cursor in list modes and the line offset otherwise.
func (d *DetailView) ScrollBy(delta int) {
	if d.IsListTab() {
		d.listCursor = clampIndex(d.listCursor+delta, d.listTabItemCount())
		return
	}
	d.scrollY = max(d.scrollY+delta, 0)
}

// ClickItem selects the block under relY. A double-click activates it, exactly
// as Enter would: it opens a file diff, a step log, or expands a block that is
// too long to show whole.
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
			if !d.dblClick.Click(i) {
				return nil
			}
			// handleActivation reads the block list, which a click can reach
			// before the first View of a resized panel has filled it in.
			d.blocks = blocks
			d.blockKeys = d.buildBlockKeys(blocks)
			_, cmd := d.handleActivation()
			return cmd
		}
		linePos += displayH + d.blockGap()
	}
	return nil
}

// IsListTab reports whether the panel is a block list rather than scrolling
// text.
func (d *DetailView) IsListTab() bool { return d.listKind() != listNone }

// listTabItemCount is the number of blocks in list mode, 0 in text modes. It is
// counted from the data rather than from the rendered blocks, so moving the
// cursor never renders a diff or a Markdown body.
func (d *DetailView) listTabItemCount() int {
	switch d.listKind() {
	case listNone:
		return 0
	case listSteps:
		return len(d.steps)
	case listFiles:
		return placeholderOr(d.filesPlaceholder, len(d.files.items))
	case listCommits:
		return placeholderOr(d.commitsPlaceholder, len(d.commits.items))
	case listThreads:
		return placeholderOr(d.threadsPlaceholder, len(d.threads.items))
	case listDiffLines:
		return placeholderOr(d.fileDiffPlaceholder, len(d.fileDiff.lines))
	default:
		return 0
	}
}

// placeholderOr is the block count of a review list: one, when the list is
// standing in a loading, error or empty state, and otherwise the item count.
func placeholderOr(placeholder func() (string, bool), count int) int {
	if _, ok := placeholder(); ok {
		return 1
	}
	return count
}

// blockGap is the number of separator rows drawn after each block. A file diff
// has none: its blocks are single lines that must sit directly under one
// another to read as a diff. The file list is one line per file too, so it
// reads as a plain list; multi-line blocks (steps, commits, threads) keep the
// rule between them.
func (d *DetailView) blockGap() int {
	switch d.listKind() { //nolint:exhaustive // every other kind keeps the separator
	case listDiffLines, listFiles:
		return 0
	}
	return 1
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

	// Tab cycling is the app's: ActNextTab/ActPrevTab pair it with the review
	// fetches the new tab needs, which a view-local cycle would skip. A "tab"
	// that reaches here (universal.switchPanel rebound elsewhere) is inert.
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

// handleActivation implements Enter for every list: the cursor's own action
// first — open a step log, open a file diff, expand a commit message or a
// thread, jump to the thread on a commented diff line — and a truncated block
// expansion when the list has no action to offer.
func (d *DetailView) handleActivation() (*DetailView, tea.Cmd) {
	kind := d.listKind()
	if kind == listNone || d.listCursor < 0 || d.listCursor >= len(d.blocks) {
		return d, nil
	}
	if cmd := d.activateBlock(kind); cmd != nil {
		return d, cmd
	}
	block := d.blocks[d.listCursor]
	if len(block) > maxBlockLines {
		return d, func() tea.Msg {
			return ExpandBlockMsg{Title: "Details", Lines: block}
		}
	}
	return d, nil
}

// Activate runs the focused list's activation, exactly as Enter does: open a
// file's diff, expand a commit message or a thread, jump to the thread on a
// marked diff line, open a pipeline step's log. It is the app's entry point, so
// the configured activation key resolves through ActOpen/ActSelect rather than
// through the literals Update matches.
func (d *DetailView) Activate() tea.Cmd {
	_, cmd := d.handleActivation()
	return cmd
}

// activateBlock is the cursor block's own Enter action, nil when it has none.
func (d *DetailView) activateBlock(kind listKind) tea.Cmd {
	switch kind {
	case listSteps:
		return d.activateStep()
	case listFiles:
		return d.activateFile()
	case listCommits:
		return d.activateCommit()
	case listThreads:
		return d.activateThread()
	case listDiffLines:
		return d.activateDiffLine()
	case listNone:
		return nil
	default:
		return nil
	}
}

// activateStep opens the cursor step's log.
func (d *DetailView) activateStep() tea.Cmd {
	if d.listCursor >= len(d.blockKeys) || d.blockKeys[d.listCursor] == "" || d.pipeline == nil {
		return nil
	}
	pipelineUUID := d.pipeline.UUID
	stepUUID := d.blockKeys[d.listCursor]
	return func() tea.Msg {
		return StepSelectedMsg{PipelineUUID: pipelineUUID, StepUUID: stepUUID}
	}
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
	case ModePipeline, ModeStepLog, ModeFileDiff:
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
	switch d.listKind() {
	case listSteps:
		header := d.listModeHeader(contentWidth)
		if d.pipelineLoading || d.pipelineErr != "" || d.pipeline == nil {
			return header
		}
		if lines, ok := d.stepsPlaceholder(contentWidth); ok {
			return append(header, lines...)
		}
		return append(header, d.renderBlockList(contentWidth, visible, len(header))...)
	case listFiles, listCommits, listThreads, listDiffLines:
		header := d.listModeHeader(contentWidth)
		return append(header, d.renderBlockList(contentWidth, visible, len(header))...)
	case listNone:
		return d.textBodyLines(contentWidth)
	default:
		return d.textBodyLines(contentWidth)
	}
}

// textBodyLines is the body of the scrolling-text modes.
func (d *DetailView) textBodyLines(contentWidth int) []string {
	switch d.mode {
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
	case ModeFileDiff:
		return "[0] " + d.fileDiff.path
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

// tabBarSep separates the mode title from the tab bar and the labels from each
// other.
const tabBarSep = " - "

// buildTitle appends the tab bar to the mode title, within maxWidth display
// columns. Room is made in the order that costs the least information: the item
// counts go first, then the repository slug, and only a title still too wide
// for the border is truncated. It records where each label landed, so ClickTab
// hit-tests what was drawn.
func (d *DetailView) buildTitle(maxWidth int) string {
	prefix := d.titlePrefix()
	labels := d.tabLabels()
	if len(labels) < 2 {
		d.tabZones = nil
		return components.TruncateEnd(prefix, maxWidth)
	}

	candidates := []struct {
		head  string
		texts []string
	}{
		{head: prefix, texts: tabTexts(labels, false)},
		{head: prefix, texts: tabTexts(labels, true)},
		{head: d.shortTitlePrefix(), texts: tabTexts(labels, true)},
	}
	pick := candidates[len(candidates)-1]
	for _, candidate := range candidates {
		if titleWidth(candidate.head, candidate.texts) <= maxWidth {
			pick = candidate
			break
		}
	}
	title := d.renderTabBar(labels, pick.head, pick.texts)
	d.clipTabZones(titleBorderW + maxWidth)
	return components.TruncateEnd(title, maxWidth)
}

// tabTexts is the label text of every tab, with or without its item count.
func tabTexts(labels []tabLabel, short bool) []string {
	texts := make([]string, len(labels))
	for i, t := range labels {
		texts[i] = t.label
		if short {
			texts[i] = t.short
		}
	}
	return texts
}

// titleWidth is how many display columns a title of prefix plus these labels
// occupies, separators included.
func titleWidth(prefix string, texts []string) int {
	total := lipgloss.Width(prefix)
	for _, text := range texts {
		total += len(tabBarSep) + lipgloss.Width(text)
	}
	return total
}

// shortTitlePrefix is the mode title with the least informative part dropped —
// for a pull request, the repository slug, which the [2] panel also shows.
func (d *DetailView) shortTitlePrefix() string {
	if d.mode == ModePullRequest && d.pr != nil {
		return "[0] #" + strconv.Itoa(d.pr.ID)
	}
	return d.titlePrefix()
}

// renderTabBar styles the labels onto prefix and records their column spans.
func (d *DetailView) renderTabBar(labels []tabLabel, prefix string, texts []string) string {
	activeStyle := lipgloss.NewStyle().Foreground(theme.ColorAccent).Bold(true)
	inactiveStyle := lipgloss.NewStyle().Foreground(theme.ColorWhite)
	sep := lipgloss.NewStyle().Foreground(theme.ColorGray).Render(tabBarSep)

	// The top border prepends "╭─" before the title, so the first title column
	// is titleBorderW in from the panel's left edge.
	pos := titleBorderW + lipgloss.Width(prefix)
	zones := make([]tabZone, 0, len(labels))
	parts := make([]string, 0, len(labels))
	for i, t := range labels {
		style := inactiveStyle
		if t.tab == d.activeTab {
			style = activeStyle
		}
		parts = append(parts, style.Render(texts[i]))
		start := pos + len(tabBarSep)
		end := start + lipgloss.Width(texts[i]) + len(tabBarSep)
		zones = append(zones, tabZone{tab: t.tab, start: start, end: end})
		pos = end - len(tabBarSep)
	}
	d.tabZones = zones
	return prefix + sep + strings.Join(parts, sep)
}

// clipTabZones drops the zones the title had no room to draw, so a click past
// the end of the bar selects nothing.
func (d *DetailView) clipTabZones(limit int) {
	kept := d.tabZones[:0]
	for _, zone := range d.tabZones {
		if zone.start >= limit {
			continue
		}
		zone.end = min(zone.end, limit)
		kept = append(kept, zone)
	}
	d.tabZones = kept
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

// renderDescription is the Overview tab's body: the pull request description
// as Markdown. A mention Bitbucket resolved reads as "@Grace Hopper", in the
// theme's mention colour, rather than as the account id the raw stores; the
// presence test stays on the raw description, which is what a pull request
// without one has empty.
func (d *DetailView) renderDescription(width int) []string {
	if d.pr.Description == "" {
		return []string{" " + lipgloss.NewStyle().Foreground(theme.ColorGray).Render("(no description)")}
	}
	rendered := d.renderBody(d.pr.Description, d.pr.Summary.HTML, width-1)
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
// is what the block hit-test and auto-scroll offset by. A review list has none
// until a refetch fails over data already on screen: its loading, error and
// empty states are the single block instead.
func (d *DetailView) listModeHeader(width int) []string {
	switch d.listKind() {
	case listSteps:
		return d.pipelineHeader(width)
	case listDiffLines:
		return d.fileDiffHeader(width)
	case listFiles:
		return d.reviewHeader(d.files.errMsg, len(d.files.items), width)
	case listCommits:
		return d.reviewHeader(d.commits.errMsg, len(d.commits.items), width)
	case listThreads:
		return d.reviewHeader(d.threads.errMsg, len(d.threads.items), width)
	case listNone:
		return nil
	default:
		return nil
	}
}

// reviewHeader is the error line drawn above a review list whose refetch failed
// while an earlier result is still on screen. Keeping the list means the
// failure has nowhere else to go — with nothing loaded the error is the list's
// placeholder instead, and there is no header.
func (d *DetailView) reviewHeader(errMsg string, count, width int) []string {
	if errMsg == "" || count == 0 {
		return nil
	}
	return []string{" " + d.theme.ErrorText.Render(components.TruncateEnd(errMsg, max(width-2, 8)))}
}

// stepsPlaceholder is what a standalone run draws instead of its step list
// while the steps are still on their way, or when they could not be fetched,
// and whether there is one. The run header is already on screen above it: only
// the list below it is missing.
func (d *DetailView) stepsPlaceholder(width int) ([]string, bool) {
	switch {
	case d.stepsErr != "":
		lines := make([]string, 0, 2)
		for _, l := range wrapText(d.stepsErr, max(width-2, 8)) {
			lines = append(lines, " "+d.theme.ErrorText.Render(l))
		}
		return lines, true
	case d.stepsPending:
		gray := lipgloss.NewStyle().Foreground(theme.ColorGray)
		return []string{" " + gray.Render("Loading steps…")}, true
	default:
		return nil, false
	}
}

// pipelineHeader is the run summary drawn above the step list.
func (d *DetailView) pipelineHeader(width int) []string {
	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)

	if d.pipelineErr != "" {
		return []string{" " + d.theme.ErrorText.Render(components.TruncateEnd(d.pipelineErr, width-2))}
	}
	if d.pipelineLoading || d.pipeline == nil {
		return []string{" " + gray.Render(fmt.Sprintf("Loading pipeline #%d…", d.pipelineBuild))}
	}

	p := d.pipeline
	summary := p.State.Summary()
	// A standalone run names its repository: the pipelines list spans every
	// repository, so unlike the pull request view the panel title does not say
	// which one this run is from.
	prefix := ""
	if !d.pipelineOwnerIsPR && p.Repository != nil && p.Repository.Slug() != "" {
		prefix = p.Repository.Slug() + " · "
	}
	head := fmt.Sprintf("%sPipeline #%d · %s · %s · %s",
		prefix, p.BuildNumber, pipelineResultStyle(summary).Render(summary),
		formatDuration(p.Duration()), p.Trigger.Name)
	if p.Creator != nil {
		head += " · by " + theme.AuthorRender(p.Creator.Name())
	}
	if age := timeAgo(p.CreatedOn); age != "" {
		head += " · " + age + " ago"
	}

	lines := []string{" " + head, " " + gray.Render(PipelineTargetLine(p.Target))}
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

// PipelineTargetLine is the one-line description of what a run was for: the
// pull request it ran on, the branch it ran for, or — when the target is a
// shape this line has no words for — its bare type. It is exported because the
// rerun and stop dialogs name the run with the same sentence the pipeline view
// heads it with; a dialog that described the run differently would read as
// being about another one.
func PipelineTargetLine(target bitbucket.PipelineTarget) string {
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
	accent := lipgloss.NewStyle().Foreground(theme.ColorAccent).Bold(true)
	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)
	label := lipgloss.NewStyle().Foreground(theme.ColorAccent)
	val := lipgloss.NewStyle()

	artLines := strings.Split(splashArt, "\n")
	lines := make([]string, 0, len(artLines)+9)
	if contentWidth >= splashArtWidth {
		for _, l := range artLines {
			lines = append(lines, "  "+accent.Render(l))
		}
	} else {
		lines = append(lines, "  "+accent.Render("lazybitbucket"))
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

// renderActiveBlocks dispatches block rendering for the active list, memoising
// the result: chroma and glamour are far too expensive to re-run for every
// cursor move, and the blocks only depend on the list kind, the data and the
// width.
func (d *DetailView) renderActiveBlocks(width int) [][]string {
	kind := d.listKind()
	if d.blockCache.valid && d.blockCache.kind == kind && d.blockCache.width == width {
		return d.blockCache.blocks
	}

	var (
		blocks    [][]string
		rendered  []bool
		renderRow func(i int) string
	)
	switch kind {
	case listNone:
	case listSteps:
		blocks = d.renderStepBlocks(width)
	case listFiles:
		blocks = d.renderFileBlocks(width)
	case listCommits:
		blocks = d.renderCommitBlocks(width)
	case listThreads:
		blocks = d.renderThreadBlocks(width)
	case listDiffLines:
		blocks, rendered, renderRow = d.newDiffLineBlocks(width)
	default:
	}

	d.blockCache = blockCache{
		blocks:    blocks,
		kind:      kind,
		width:     width,
		valid:     true,
		rendered:  rendered,
		renderRow: renderRow,
	}
	return blocks
}

// fillVisibleBlocks builds the text of a lazily rendered list's blocks inside
// the window the panel can draw, and leaves the rest empty: syntax-highlighting
// a whole file costs the best part of a second on a big one, and no more than a
// screenful of it is ever visible. Lists rendered whole are a no-op.
func (d *DetailView) fillVisibleBlocks(start, visible int) {
	if d.blockCache.renderRow == nil {
		return
	}
	from := max(start-blockOverscan, 0)
	to := min(start+visible+blockOverscan, len(d.blockCache.rendered))
	for i := from; i < to; i++ {
		if d.blockCache.rendered[i] {
			continue
		}
		d.blockCache.blocks[i][0] = d.blockCache.renderRow(i)
		d.blockCache.rendered[i] = true
	}
}

// buildBlockKeys pairs each block with the thing it renders — a step UUID, a
// file path, a commit hash, a thread root id, a diff line index — so activating
// a block can name its subject without re-deriving the cursor.
func (d *DetailView) buildBlockKeys(blocks [][]string) []string {
	keys := make([]string, len(blocks))
	for i := range blocks {
		switch d.listKind() {
		case listNone:
		case listSteps:
			if i < len(d.steps) {
				keys[i] = d.steps[i].UUID
			}
		case listFiles:
			if i < len(d.files.items) {
				keys[i] = d.files.items[i].Path()
			}
		case listCommits:
			if i < len(d.commits.items) {
				keys[i] = d.commits.items[i].Hash
			}
		case listThreads:
			if i < len(d.threads.items) {
				keys[i] = strconv.FormatInt(d.threads.items[i].Root.ID, 10)
			}
		case listDiffLines:
			keys[i] = "L" + strconv.Itoa(i)
		default:
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

	// Auto-scroll before rendering: it only needs the block heights, and the
	// window it settles on is the one a lazily rendered list has to fill in.
	d.autoScrollToBlock(blocks, visible, headerLines)
	d.fillVisibleBlocks(max(d.scrollY-headerLines, 0), visible)

	bar := lipgloss.NewStyle().Foreground(theme.ColorBlue).Render("▎")
	ellipsis := lipgloss.NewStyle().Foreground(theme.ColorGray).Render("    ...")
	sep := strings.Repeat("─", max(blockWidth, 1))
	gap := d.blockGap()

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
		if gap > 0 && i < len(blocks)-1 {
			lines = append(lines, " "+sep)
		}
	}

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
	gap := d.blockGap()
	lineStart := headerLines
	for i := 0; i < d.listCursor && i < len(blocks); i++ {
		lineStart += displayBlockHeight(blocks[i]) + gap
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
