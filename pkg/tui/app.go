package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/config"
	"github.com/lsst25/lazybitbucket/pkg/git"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
	"github.com/lsst25/lazybitbucket/pkg/tui/theme"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
	"github.com/lsst25/lazybitbucket/pkg/viewed"
)

// Version is stamped by cmd/lazybitbucket at build time.
var Version = "dev"

type focusPanel int

const (
	focusStatus focusPanel = iota
	focusPullRequests
	focusBuilds
	focusRepos
	// focusPipelines is the pipelines view's [2] panel. It is last rather than
	// beside the pull request list because the two never share a screen: a
	// view draws one left column or the other.
	focusPipelines
)

type focusSide int

const (
	sideLeft focusSide = iota
	sideRight
)

// onSelectFunc is the callback the modal fires when the user picks an item:
// the URL picker's target, and the title/description chooser behind the edit
// key.
type onSelectFunc func(components.ModalItem) tea.Cmd

// onChecklistFunc is the callback the modal fires when a checklist is
// confirmed. The reviewer picker is its only user.
type onChecklistFunc func([]components.ModalItem) tea.Cmd

// onFormFunc is the callback the form modal fires on confirm; the message
// carries the fields with their final values.
type onFormFunc func(components.FormConfirmedMsg) tea.Cmd

// onInputFunc is the callback the input modal fires on confirm, carrying the
// text as typed.
type onInputFunc func(string) tea.Cmd

// statusEntry is one pull request's cached build statuses plus the two
// timestamps the staleness rule reads: the pull request's UpdatedOn as it was
// when the statuses were fetched, and when that fetch happened.
type statusEntry struct {
	statuses  []bitbucket.CommitStatus
	updatedOn time.Time
	fetchedAt time.Time
}

// pipelineEntry is a pipeline run and its steps, cached by "ws/slug#build".
type pipelineEntry struct {
	pipeline *bitbucket.Pipeline
	steps    []bitbucket.PipelineStep
}

// App is the bubbletea model: five panels, an overlay stack and the Bitbucket
// domain state feeding them.
type App struct {
	cfg        *config.Config
	client     bitbucket.ClientInterface
	splashInfo views.SplashInfo

	statusPanel  *views.StatusPanel
	prList       *views.PullRequestList
	buildsPanel  *views.BuildsPanel
	pipelineList *views.PipelineList
	repoList     *views.RepoList
	detailView   *views.DetailView
	logPanel     *views.LogPanel

	keymap    Keymap
	helpBar   components.HelpBar
	searchBar components.SearchBar
	modal     components.Modal
	form      components.FormModal
	input     components.InputModal
	overlays  components.OverlayStack

	// The four result slots, one per overlay shape. Each is armed from the
	// moment its overlay opens until that overlay's result message is
	// dispatched — a hop longer than the overlay is on screen — and nilled on
	// use and on every cancel, so a stale callback can never fire against a
	// pull request the user has left. See App.overlayArmed.
	onSelect    onSelectFunc
	onChecklist onChecklistFunc
	onForm      onFormFunc
	onInput     onInputFunc
	// pendingComment is set while the comment popup is open: what the confirmed
	// text will be posted as. See comments.go.
	pendingComment *pendingComment
	// pendingCreate is the pull request being opened, from the moment N reads
	// the branch until the POST answers. See create.go.
	pendingCreate *pendingCreate

	// onFormID is the id of the dialog onForm was armed for. A form result
	// carries the id of the form that produced it, so one that outlived its
	// dialog is dropped rather than answered by whatever is armed now.
	onFormID string

	// readOnly mirrors cfg.Bitbucket.ReadOnly: the write keys refuse to open
	// anything and the status panel says so.
	readOnly bool

	// writesInFlight maps a pull request key to the verb of the write running
	// against it, so a second M or D on the same pull request is refused rather
	// than racing the first.
	writesInFlight map[string]string

	// pipelineWrites is the same guard for the two pipeline writes, keyed by
	// pipelineCacheKey. It is a separate map because a pull request key and a
	// pipeline key share the "ws/slug#N" shape: pull request #412 and build
	// #412 of the same repository would otherwise block each other.
	pipelineWrites map[string]string

	// descriptionEditPath is the temp file the description editor is working
	// on, held across the suspension so the finish handler reads back a path
	// this process wrote rather than one that travelled through a message.
	descriptionEditPath string

	side      focusSide
	leftFocus focusPanel
	// view is the screen the app is on; see appView. It starts from
	// gui.startView and the nextView key cycles it.
	view appView
	// screen is the current screen mode; see screenMode. It starts from
	// gui.screenMode and the nextScreenMode / prevScreenMode keys cycle it.
	screen screenMode

	showHelp      bool
	helpCursor    int
	helpSearching bool
	helpSearch    components.TextInput
	helpFilter    string

	logFlag  *atomic.Bool
	demoMode bool

	// exec is the external-process seam (browser, clipboard); tests replace it.
	exec externalRunner

	// statusWarning mirrors the last StatusPanel.SetWarning so the layout can
	// reserve a row for it; the panel exposes its error but not its warning.
	statusWarning string

	// Domain state.
	workspace   string
	repoSlugs   []string
	repos       []bitbucket.Repository
	currentUser *bitbucket.Account

	// repoDiscoveryFailed records that repository discovery errored, so the
	// refresh paths re-run it — without it every later fetch is a no-op,
	// because repoSlugs was never populated.
	repoDiscoveryFailed bool

	// merged holds the unfiltered MergePullRequests result per fetch key
	// ("<state>|<query>"). Every tab sharing a key re-filters from here rather
	// than costing another request.
	merged   map[string][]bitbucket.PullRequest
	inFlight map[string]bool
	// loadedRepos records, per fetch key, the repositories whose pull requests
	// are in merged. A start inside a checkout fetches only that repository, so
	// merged is usually a part of the whole until the user widens the view.
	loadedRepos map[string]map[string]bool

	prCache       map[string]*bitbucket.PullRequest
	statusCache   map[string]statusEntry
	pipelineCache map[string]*pipelineEntry

	// The review caches, all keyed by pull request key (the file diffs by key
	// plus path) and all stamped with the source commit they describe, so a
	// push drops them. See review.go.
	diffStatCache map[string]reviewEntry[[]bitbucket.DiffStatEntry]
	commitsCache  map[string]reviewEntry[[]bitbucket.Commit]
	threadsCache  map[string]reviewEntry[[]bitbucket.Thread]
	fileDiffCache map[string]reviewEntry[fileDiffData]

	// reviewInFlight marks the review fetches that have been issued and not yet
	// answered, keyed by reviewFetchKey, so a tab entered while its prefetch is
	// still in flight waits for that answer instead of asking again.
	reviewInFlight map[string]bool

	// viewed holds the files marked as viewed, nil until SetViewedStore or
	// the first use creates an in-memory one. See viewed.go.
	viewed *viewed.Store

	// diffRendererDisabled turns the configured external diff renderer off for
	// the rest of the session, which is what a program that cannot be run at
	// all or whose output does not line up with the diff earns: neither gets
	// better by being retried on the next file. A timeout or a non-zero exit
	// costs only the file it happened on. lastRendererError is the last failure
	// already reported, so one broken renderer does not repaint the status
	// panel once per file.
	diffRendererDisabled bool
	lastRendererError    string

	// prefetchQueue is the remaining pull request keys to warm, head first.
	// prefetchEpoch is bumped whenever the queue is refilled so that in-flight
	// results from a superseded tab or repository filter are dropped.
	prefetchQueue []string
	prefetchEpoch int

	// previewKey is the pull request shown on the right; previewEpoch is bumped
	// on every selection so debounce ticks and responses from a superseded
	// selection can be dropped.
	previewKey   string
	previewEpoch int

	// The pipelines view's own state. pipelineEntries is the list as it was
	// last fetched, pipelinesLoaded records that the fetch has happened at all
	// (the first entry into the view is what pays for it), pipelinesEpoch drops
	// responses from a superseded fetch, and runKey is the run the detail panel
	// is showing there — the same "ws/slug#build" shape the pipeline caches and
	// the detail panel's owner token use.
	pipelineEntries []bitbucket.PipelineEntry
	pipelinesLoaded bool
	pipelinesEpoch  int
	runKey          string

	repoFilter   string
	refreshEvery time.Duration

	// rateLimit stores remaining+1 so the zero value reads as "unknown". It is
	// written from the client's request callback, which runs on whatever
	// goroutine executes the command, and read in View.
	rateLimit atomic.Int64

	// gitRepoPath is the cwd when it is inside a repository; gitRepoRoot is
	// that repository's working tree root, which is what a diff path has to be
	// resolved against. Neither is checked against the pull request's own
	// repository — nothing maps one to the other.
	gitRepoPath string
	gitRepoRoot string
	gitBranch   string
	// gitRemoteRepos is the lower-cased "workspace/slug" of every remote of the
	// checkout lazybitbucket was started in; autoScoped records that the list
	// was narrowed to one of them once, so the user's later choice is kept.
	gitRemoteRepos []string
	autoScoped     bool
	gitAutoSelect  bool
	// inTmux records that the session runs inside tmux ($TMUX was set at
	// start-up), where the editor key can open a window instead of suspending.
	// tmuxPath is the tmux executable to run; empty means "tmux" on $PATH, and
	// tests point it at a stand-in.
	inTmux   bool
	tmuxPath string

	renderer views.MarkdownRenderer
	// now is the clock and debounce the selection debounce window. Both are
	// zero in production and only set by tests that need a fixed clock or a
	// synchronously drainable message chain.
	now      func() time.Time
	debounce time.Duration

	customCmds []config.ResolvedCustomCommand

	// pendingCmd is the one custom command waiting for the diffstat its
	// {{.Files}} needs; nil the rest of the time. See awaitChangedFiles.
	pendingCmd *pendingCustomCommand

	panelSideW      int
	panelStatusH    int
	panelPRH        int
	panelBuildsH    int
	panelPipelinesH int
	panelReposH     int
	panelDetailH    int
	panelLogH       int
	width, height   int

	ctx    context.Context
	cancel context.CancelFunc
	cmdWg  sync.WaitGroup
}

// AuthMethod describes how the user authenticated.
type AuthMethod string

// The credential sources cmd/lazybitbucket reports on the splash screen.
const (
	AuthSaved  AuthMethod = "Saved credentials (auth.json)"
	AuthEnv    AuthMethod = "Environment variables"
	AuthWizard AuthMethod = "Setup wizard"
	AuthDemo   AuthMethod = "Demo mode"
)

// NewApp builds the app with the environment-credentials label.
func NewApp(cfg *config.Config, client bitbucket.ClientInterface) *App {
	return NewAppWithAuth(cfg, client, AuthEnv)
}

// NewAppWithAuth builds the app and records how the user authenticated.
// theme.Init must already have run: the views capture theme.Default when they
// are constructed.
func NewAppWithAuth(cfg *config.Config, client bitbucket.ClientInterface, authMethod AuthMethod) *App {
	workspace := cfg.Bitbucket.Workspace

	statusPanel := views.NewStatusPanel(scopeLabel(""), cfg.Bitbucket.Email, workspace)
	prList := views.NewPullRequestList()
	if len(cfg.GUI.PullRequestListFields) > 0 {
		prList.SetFields(cfg.GUI.PullRequestListFields)
	}
	prList.SetTabs(cfg.PullRequestTabs)
	prList.SetFocused(true)

	// One resolved style feeds both renderers: glamour for Markdown bodies and
	// the chroma style it maps to for the diff view's code column.
	glamourStyle := views.ResolveGlamourStyle(cfg.RendererStyle)
	renderer := views.MarkdownRenderer(views.GlamourRenderer{Style: glamourStyle})

	logPanel := views.NewLogPanel()
	logFlag := new(atomic.Bool)

	app := &App{
		cfg:            cfg,
		client:         client,
		keymap:         KeymapFromConfig(cfg.Keybinding),
		statusPanel:    statusPanel,
		prList:         prList,
		buildsPanel:    views.NewBuildsPanel(),
		pipelineList:   views.NewPipelineList(),
		repoList:       views.NewRepoList(),
		detailView:     views.NewDetailView(renderer, views.ChromaStyleFor(glamourStyle)),
		logPanel:       logPanel,
		helpBar:        components.NewHelpBar(nil),
		searchBar:      components.NewSearchBar(),
		modal:          components.NewModal(),
		form:           components.NewFormModal(),
		input:          components.NewInputModal(),
		readOnly:       cfg.Bitbucket.ReadOnly,
		side:           sideLeft,
		leftFocus:      focusPullRequests,
		screen:         screenModeFromConfig(cfg.GUI.ScreenMode),
		view:           viewFromConfig(cfg.GUI.StartView),
		logFlag:        logFlag,
		demoMode:       authMethod == AuthDemo,
		exec:           execExternal,
		workspace:      workspace,
		repoSlugs:      append([]string(nil), cfg.Bitbucket.Repositories...),
		merged:         make(map[string][]bitbucket.PullRequest),
		inFlight:       make(map[string]bool),
		prCache:        make(map[string]*bitbucket.PullRequest),
		statusCache:    make(map[string]statusEntry),
		pipelineCache:  make(map[string]*pipelineEntry),
		diffStatCache:  make(map[string]reviewEntry[[]bitbucket.DiffStatEntry]),
		commitsCache:   make(map[string]reviewEntry[[]bitbucket.Commit]),
		threadsCache:   make(map[string]reviewEntry[[]bitbucket.Thread]),
		fileDiffCache:  make(map[string]reviewEntry[fileDiffData]),
		reviewInFlight: make(map[string]bool),
		writesInFlight: make(map[string]string),
		pipelineWrites: make(map[string]string),
		renderer:       renderer,
	}
	// The callback runs on whatever goroutine executes the command, and several
	// request goroutines may run it at once, so it may only touch the log panel
	// (which locks internally), the log-flag atomic and the rate-limit atomic.
	rateTarget := &app.rateLimit
	client.SetOnRequest(func(rl bitbucket.RequestLog) {
		if rl.RateLimitRemaining >= 0 {
			rateTarget.Store(int64(rl.RateLimitRemaining) + 1)
		}
		if logFlag.Load() {
			logPanel.AddEntry(views.LogEntry{
				Time:    time.Now(),
				Method:  rl.Method,
				Path:    rl.Path,
				Status:  rl.Status,
				Elapsed: rl.Elapsed,
			})
		}
	})

	app.splashInfo = views.SplashInfo{
		Version:    Version,
		AuthMethod: string(authMethod),
		Workspace:  workspace,
		User:       cfg.Bitbucket.Email,
		RepoCount:  len(app.repoSlugs),
		RepoNote:   "",
	}

	app.ctx, app.cancel = context.WithCancel(context.Background()) //nolint:gosec // cancel is called in Shutdown()

	navResolver := app.keymap.MatchNav
	app.prList.ResolveNav = navResolver
	app.buildsPanel.ResolveNav = navResolver
	app.pipelineList.ResolveNav = navResolver
	app.repoList.ResolveNav = navResolver
	app.detailView.ResolveNav = navResolver
	// A conversation that is done is out of the way from the start; f asks for
	// them back, and gui.showResolvedComments starts the other way round.
	app.detailView.SetShowResolvedThreads(cfg.GUI.ShowsResolvedComments())

	app.initCustomCommands()
	if warning := quitReachableWarning(app.keymap, app.customCmds); warning != "" {
		fmt.Fprintln(os.Stderr, "lazybitbucket:", warning)
		app.statusPanel.SetError(warning)
	}

	// Order is priority: the form and the input are only ever open on their
	// own, and the modal is what the other two are opened from.
	app.overlays = components.OverlayStack{&app.modal, &app.form, &app.input}
	app.statusPanel.SetReadOnly(app.readOnly)

	if interval, err := cfg.ResolveRefreshInterval(); err == nil {
		app.refreshEvery = interval
	} else {
		app.setWarning("refresh interval: " + err.Error())
	}
	if warning := refreshRateWarning(len(app.repoSlugs), app.refreshEvery); warning != "" {
		app.setWarning(warning)
	}

	app.inTmux = os.Getenv("TMUX") != ""

	if git.GitAvailable() {
		cwd, _ := os.Getwd()
		if git.IsRepo(cwd) {
			app.gitRepoPath = cwd
			if root, err := git.TopLevel(cwd); err == nil && root != "" {
				app.gitRepoRoot = root
			}
			if branch, err := git.CurrentBranch(cwd); err == nil && branch != "" {
				app.gitBranch = branch
			}
			app.gitRemoteRepos = git.RemoteRepositories(cwd)
		}
	}

	// The configured start view decides which left column is drawn and which
	// panel the focus starts on, so it has to be applied before the first
	// render rather than on the first switch.
	app.leftFocus = app.mainPanel()
	app.applyView()
	app.prList.SetFocused(app.view == viewPullRequests)
	app.pipelineList.SetFocused(app.view == viewPipelines)

	app.detailView.SetSplash(app.splashInfo)
	app.helpBar.SetItems(app.helpBarItems())
	app.helpBar.SetVersion(Version)
	return app
}

// refreshRateWarning warns when auto-refresh alone would spend more than about
// 600 of the ~1000 requests Bitbucket allows per hour.
func refreshRateWarning(repos int, every time.Duration) string {
	if repos == 0 || every <= 0 {
		return ""
	}
	if float64(repos)*3600/every.Seconds() > 600 {
		return fmt.Sprintf("refresh interval too short for %d repositories", repos)
	}
	return ""
}

// rateLimitNoteBelow mirrors the threshold views.StatusPanel renders its
// rate-limit note under. Keep the two in sync: the app reserves the row.
const rateLimitNoteBelow = 200

// setWarning records a non-error hint and shows it on the status panel.
func (a *App) setWarning(text string) {
	a.statusWarning = text
	a.statusPanel.SetWarning(text)
}

// rateLimitLeft is the remaining request budget, or -1 when unknown. The
// atomic stores remaining+1 so a zero value reads as unknown.
func (a *App) rateLimitLeft() int {
	return int(a.rateLimit.Load()) - 1
}

// statusPanelHeight is three rows plus one for every extra line the status
// panel has to show.
func (a *App) statusPanelHeight() int {
	extra := 0
	if a.statusPanel.ErrorMessage() != "" || a.statusWarning != "" {
		extra++
	}
	if left := a.rateLimitLeft(); left >= 0 && left < rateLimitNoteBelow {
		extra++
	}
	return 3 + extra
}

// scopeLabel is what the status panel shows as the list scope.
func scopeLabel(slug string) string {
	if slug == "" {
		return "all repos"
	}
	return slug
}

// nowFn is the clock: time.Now unless a test injected one.
func (a *App) nowFn() time.Time {
	if a.now != nil {
		return a.now()
	}
	return time.Now()
}

// context is the app-lifetime context, or a background one for the hand-built
// App values tests use.
func (a *App) context() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

// Shutdown cancels the app-lifetime context, signalling background processes
// to terminate, and waits up to three seconds for them. Safe to call twice.
func (a *App) Shutdown() {
	if a.cancel != nil {
		a.cancel()
	}
	done := make(chan struct{})
	go func() { a.cmdWg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
}

// Init loads the current user and the repository list; the repository response
// is what triggers the first pull request fetch. The auto-refresh tick is only
// armed when refreshInterval is enabled.
func (a *App) Init() tea.Cmd {
	cmds := []tea.Cmd{a.fetchCurrentUser(), a.fetchRepos()}
	if a.refreshEvery > 0 {
		cmds = append(cmds, autoFetchTick(a.refreshEvery))
	}
	return tea.Batch(cmds...)
}

// Update routes a message to the search bar, the overlays, a handler, or the
// focused panel, in that order.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if a.searchBar.IsActive() {
		if km, ok := msg.(tea.KeyMsg); ok &&
			km.Type != tea.KeyUp && km.Type != tea.KeyDown &&
			km.Type != tea.KeyCtrlJ && km.Type != tea.KeyCtrlK {
			updated, cmd := a.searchBar.Update(msg)
			a.searchBar = updated
			return a, cmd
		}
	}

	// The comment popup's own key, answered before the overlay stack: the input
	// modal reads ctrl+e as "end of line", which is what it goes on meaning for
	// the title editor.
	if km, ok := msg.(tea.KeyMsg); ok {
		if cmd, handled := a.commentInputKey(km); handled {
			return a, cmd
		}
	}

	if cmd, ok := a.overlays.Intercept(msg); ok {
		return a, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return a.handleResize(msg)
	case tea.MouseMsg:
		return a.handleMouse(msg)
	case tea.KeyMsg:
		if m, cmd := a.handleKeyMsg(msg); m != nil {
			return m, cmd
		}

	case components.SearchChangedMsg:
		return a.handleSearchChanged(msg)
	case components.SearchConfirmedMsg:
		return a.handleSearchConfirmed()
	case components.SearchCancelledMsg:
		return a.handleSearchCancelled()

	case autoFetchTickMsg:
		return a.handleAutoFetch()
	case currentUserLoadedMsg:
		return a.handleCurrentUserLoaded(msg)
	case reposLoadedMsg:
		return a.handleReposLoaded(msg)
	case pullRequestsLoadedMsg:
		return a.handlePullRequestsLoaded(msg)
	case pullRequestDetailLoadedMsg:
		return a.handlePullRequestDetailLoaded(msg)
	case statusesLoadedMsg:
		return a.handleStatusesLoaded(msg)
	case previewDebounceMsg:
		return a.handlePreviewDebounce(msg)
	case prefetchedMsg:
		return a.handlePrefetched(msg)
	case pipelineLoadedMsg:
		return a.handlePipelineLoaded(msg)
	case pipelinesLoadedMsg:
		return a.handlePipelinesLoaded(msg)
	case pipelineRunDebounceMsg:
		return a.handlePipelineRunDebounce(msg)
	case pipelineStepsLoadedMsg:
		return a.handlePipelineStepsLoaded(msg)
	case stepLogLoadedMsg:
		return a.handleStepLogLoaded(msg)
	case diffStatLoadedMsg:
		return a.handleDiffStatLoaded(msg)
	case commitsLoadedMsg:
		return a.handleCommitsLoaded(msg)
	case threadsLoadedMsg:
		return a.handleThreadsLoaded(msg)
	case fileDiffLoadedMsg:
		return a.handleFileDiffLoaded(msg)
	case viewedComparedMsg:
		return a.handleViewedCompared(msg)

	case views.PullRequestSelectedMsg:
		// The list emits this only when the user moves its cursor: the app's
		// own previews call the handler directly.
		a.userTookTheCursor()
		return a.handlePullRequestSelected(msg)
	case views.PipelineSelectedMsg:
		return a.handlePipelineSelected(msg)
	case views.RepoHoveredMsg:
		if msg.Repo != nil && a.side == sideLeft && a.leftFocus == focusRepos {
			a.detailView.SetRepo(msg.Repo)
		}
		return a, nil
	case views.StepSelectedMsg:
		return a.handleStepSelected(msg)
	case views.ExpandBlockMsg:
		return a.handleExpandBlock(msg)
	case views.FileSelectedMsg:
		return a.handleFileSelected(msg)
	case views.ThreadSelectedMsg:
		return a.handleThreadSelected(msg)

	case components.ModalSelectedMsg:
		return a.handleModalSelected(msg)
	case components.ModalCancelledMsg:
		return a.handleModalCancelled()
	case components.ChecklistConfirmedMsg:
		return a.handleChecklistConfirmed(msg)
	case components.FormConfirmedMsg:
		return a.handleFormConfirmed(msg)
	case components.FormCancelledMsg:
		return a.handleFormCancelled(msg)
	case components.InputConfirmedMsg:
		return a.handleInputConfirmed(msg)
	case components.InputCancelledMsg:
		return a.handleInputCancelled()

	case pullRequestWrittenMsg:
		return a.handlePullRequestWritten(msg)
	case mergeQueuedMsg:
		return a.handleMergeQueued(msg)
	case reviewerCandidatesMsg:
		return a.handleReviewerCandidates(msg)
	case approvalStateMsg:
		return a.handleApprovalState(msg)
	case pipelineActionStateMsg:
		return a.handlePipelineActionState(msg)
	case pipelineWrittenMsg:
		return a.handlePipelineWritten(msg)
	case descriptionEditedMsg:
		return a.handleDescriptionEdited(msg)
	case commentPostedMsg:
		return a.handleCommentPosted(msg)
	case createPrefillMsg:
		return a.handleCreatePrefill(msg)
	case createReviewersMsg:
		return a.handleCreateReviewers(msg)
	case branchPushedMsg:
		return a.handleBranchPushed(msg)
	case createDescriptionEditedMsg:
		return a.handleCreateDescriptionEdited(msg)
	case pullRequestCreatedMsg:
		return a.handlePullRequestCreated(msg)
	case commentResolvedMsg:
		return a.handleCommentResolved(msg)
	case commentEditedMsg:
		return a.handleCommentEdited(msg)
	case syncDoneMsg:
		return a.handleSyncDone(msg)

	case customCommandFinishedMsg:
		return a.handleCustomCommandFinished(msg)
	case tmuxWindowMsg:
		return a.handleTmuxWindow(msg)
	case editorFinishedMsg:
		return a.handleEditorFinished(msg)

	case gitCheckoutDoneMsg:
		return a.handleGitBranchSwitch(msg.name, msg.note)
	case gitErrorMsg:
		a.statusPanel.SetError(msg.err.Error())
		return a, nil
	case errorMsg:
		errText := msg.err.Error()
		a.statusPanel.SetOnline(false)
		a.statusPanel.SetError(errText)
		a.modal.ShowError("Error", []components.ModalItem{{Label: errText}})
		return a, nil
	}

	return a, a.routeToPanel(msg)
}

// View renders the whole screen.
func (a *App) View() string {
	if a.width == 0 {
		return "Loading..."
	}

	a.statusPanel.SetRateLimit(a.rateLimitLeft())
	// The status panel grows with its content, so re-layout before rendering:
	// a warning or the rate-limit note must never be clipped.
	a.layoutPanels()

	var content string
	if panel, ok := a.fullScreenPanel(); ok {
		content = a.panelView(panel)
	} else if a.isVerticalLayout() {
		content = lipgloss.JoinVertical(lipgloss.Left,
			append(a.leftColumnViews(), a.detailView.View(), a.logPanel.View())...,
		)
	} else {
		leftCol := lipgloss.JoinVertical(lipgloss.Left, a.leftColumnViews()...)
		rightCol := lipgloss.JoinVertical(lipgloss.Left,
			a.detailView.View(),
			a.logPanel.View(),
		)
		content = lipgloss.JoinHorizontal(lipgloss.Top, leftCol, rightCol)
	}

	a.helpBar.SetItems(a.helpBarItems())

	var bottomBar string
	switch {
	case a.searchBar.IsActive():
		bottomBar = a.searchBar.View()
	case a.modal.IsVisible() && a.modal.IsSearching():
		bottomBar = a.modal.SearchView(a.width)
	case a.showHelp && a.helpSearching:
		bottomBar = components.RenderFilterBarInput(&a.helpSearch)
	default:
		bottomBar = a.helpBar.View()
	}

	full := lipgloss.JoinVertical(lipgloss.Left, content, bottomBar)
	full = a.overlays.Render(full, a.width, a.height)
	if a.showHelp {
		full = a.renderHelpOverlay(full)
	}
	return full
}

func (a *App) renderHelpOverlay(base string) string {
	bindings := a.filteredHelpBindings()

	if a.helpCursor >= len(bindings) {
		a.helpCursor = len(bindings) - 1
	}
	if a.helpCursor < 0 {
		a.helpCursor = 0
	}

	maxKey := 0
	for _, b := range bindings {
		if len(b.Key) > maxKey {
			maxKey = len(b.Key)
		}
	}

	popupW := min(maxKey+40, a.width-4)

	keyNormal := lipgloss.NewStyle().Foreground(theme.ColorAccent).Bold(true)
	keySel := lipgloss.NewStyle().Foreground(theme.ColorAccent).Bold(true).Background(theme.ColorHighlight)
	descSel := lipgloss.NewStyle().Background(theme.ColorHighlight)

	lines := make([]string, 0, len(bindings)+2)
	lines = append(lines, "")
	descMaxW := popupW - maxKey - 6
	for i, b := range bindings {
		padded := b.Key
		for len(padded) < maxKey {
			padded += " "
		}
		desc := components.TruncateEnd(b.Description, descMaxW)
		for len(desc) < descMaxW {
			desc += " "
		}
		var line string
		if i == a.helpCursor {
			line = descSel.Render("  ") + keySel.Render(padded) + descSel.Render("  "+desc)
		} else {
			line = "  " + keyNormal.Render(padded) + "  " + desc
		}
		lines = append(lines, line)
	}
	lines = append(lines, "")

	popupH := max(min(len(lines), a.height-4), 1)
	footer := fmt.Sprintf("%d of %d", a.helpCursor+1, len(bindings))

	popupContent := strings.Join(lines, "\n")
	content := components.RenderPanelFull("Keybindings", footer, popupContent, popupW, popupH, true, nil)

	return components.Overlay(base, content, a.width, a.height)
}

func (a *App) handleGitBranchSwitch(name, note string) (tea.Model, tea.Cmd) {
	a.gitBranch = name
	status := "checked out " + name
	if note != "" {
		status += " · " + note
	}
	a.helpBar.SetStatusMsg(status)
	if a.cfg.Git.CloseOnCheckout {
		return a, tea.Quit
	}
	return a, nil
}

// tabConfigs is the configured tab list, with a single implicit Open tab when
// none are configured so the list is never tabless.
func (a *App) tabConfigs() []config.PullRequestTabConfig {
	if a.cfg != nil && len(a.cfg.PullRequestTabs) > 0 {
		return a.cfg.PullRequestTabs
	}
	return []config.PullRequestTabConfig{{Name: "Open", State: "", Filter: "", Query: "", MaxResults: nil}}
}

// fetchKeyOf is the request identity of a tab: tabs that agree on state and
// query share one fetch and re-filter its result client side.
func (a *App) fetchKeyOf(tab int) string {
	tabs := a.tabConfigs()
	if tab < 0 || tab >= len(tabs) {
		return "OPEN|"
	}
	return tabs[tab].ResolveState() + "|" + tabs[tab].Query
}

// fetchActiveTab requests the pull requests behind the tab in view.
func (a *App) fetchActiveTab() tea.Cmd {
	return a.fetchPullRequests(a.fetchKeyOf(a.prList.GetTabIndex()))
}

// needsRepoDiscovery reports whether repository discovery has to be re-run
// before a pull request fetch can do anything. Discovery either failed, or it
// came back empty while nothing was configured — either way repoSlugs is
// empty, so every pull request fetch is a silent no-op and the session is
// inert until discovery lands.
//
// It is deliberately consulted only from the user-driven refresh paths and not
// from handleReposLoaded, which would otherwise re-issue discovery on every
// empty response and spin.
func (a *App) needsRepoDiscovery() bool {
	if a.repoDiscoveryFailed {
		return true
	}
	return len(a.repoSlugs) == 0 && len(a.cfg.Bitbucket.Repositories) == 0
}

// refreshCmd is what the refresh paths issue. Repository discovery is retried
// first when it is what is missing, because without it repoSlugs is empty and
// every pull request fetch is a silent no-op.
func (a *App) refreshCmd() tea.Cmd {
	if a.needsRepoDiscovery() {
		return a.fetchRepos()
	}
	return a.fetchActiveTab()
}

// refreshAllCmd is what the refresh-all key re-reads: the list the view on
// screen is about. The pipelines list is not on the auto-refresh tick — it
// costs a round of requests per repository — so this key is the only thing
// that re-reads it.
func (a *App) refreshAllCmd() tea.Cmd {
	// Discovery comes first even here: without repoSlugs the pipelines fetch is
	// a silent no-op, which would leave the refresh key doing nothing at all in
	// the view the user is looking at.
	if a.view == viewPipelines && !a.needsRepoDiscovery() {
		return a.fetchPipelines()
	}
	return a.refreshCmd()
}

func (a *App) updateFocusState() {
	a.statusPanel.SetFocused(false)
	a.prList.SetFocused(false)
	a.buildsPanel.SetFocused(false)
	a.pipelineList.SetFocused(false)
	a.repoList.SetFocused(false)
	a.detailView.SetFocused(false)

	if a.side == sideLeft {
		switch a.leftFocus {
		case focusStatus:
			a.statusPanel.SetFocused(true)
		case focusPullRequests:
			a.prList.SetFocused(true)
		case focusBuilds:
			a.buildsPanel.SetFocused(true)
		case focusPipelines:
			a.pipelineList.SetFocused(true)
		case focusRepos:
			a.repoList.SetFocused(true)
		}
	} else {
		a.detailView.SetFocused(true)
	}

	a.helpBar.SetItems(a.helpBarItems())
	a.layoutPanels()
}

// focusDetail moves the focus onto the detail panel and, when the panel shows a
// pull request and gui.prefetchDetailTabs is on, warms its three review tabs at
// once. Every path that lands the focus on the right-hand side goes through it,
// so the prefetch has one place to live rather than one per key and click.
func (a *App) focusDetail() tea.Cmd {
	a.side = sideRight
	a.updateFocusState()
	return a.prefetchDetailTabs()
}

// showSplash points the detail panel at the startup screen with fresh counts.
func (a *App) showSplash() {
	a.splashInfo.Workspace = a.workspace
	if a.currentUser != nil {
		a.splashInfo.User = a.currentUser.Name()
	}
	a.splashInfo.RepoCount = len(a.repoSlugs)
	a.detailView.SetSplash(a.splashInfo)
}
