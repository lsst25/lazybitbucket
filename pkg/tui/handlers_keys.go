package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/tui/components"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// handleKeyMsg dispatches keyboard actions. It returns (nil, nil) when the key
// belongs to the focused panel instead.
func (a *App) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	a.helpBar.SetStatusMsg("")

	if a.showHelp {
		return a.handleHelpKeys(msg)
	}

	// Custom commands take precedence over built-in bindings so a user can
	// override any action.
	if m, cmd, ok := a.handleCustomCommand(msg.String()); ok {
		return m, cmd
	}

	action := a.keymap.Match(msg.String())

	// In the pipeline and step-log views, back and focus-left walk out of the
	// drill-down before they move focus.
	if action == ActBack || action == ActFocusLeft {
		if a.side == sideRight && a.detailView.Back() {
			return a, nil
		}
	}

	switch action { //nolint:exhaustive // the remaining actions are handled by the helpers below
	case ActQuit:
		return a, tea.Quit
	case ActHelp:
		a.showHelp = true
		a.helpCursor = 0
		a.helpFilter = ""
		a.helpSearching = false
		return a, nil
	case ActSearch:
		a.searchBar.Activate()
		return a, nil
	case ActSelect, ActOpen:
		return a.handleActivate()
	case ActURLPicker:
		return a.handleActionURLPicker()
	}

	if m, cmd, ok := a.handleFocusAction(action); ok {
		return m, cmd
	}
	if m, cmd, ok := a.handleTabAction(action); ok {
		return m, cmd
	}
	if m, cmd, ok := a.handleReviewAction(action); ok {
		return m, cmd
	}
	if m, cmd, ok := a.handlePullRequestAction(action); ok {
		return m, cmd
	}

	if a.side == sideLeft && a.leftFocus != focusStatus {
		if m, cmd, ok := a.handleDetailScroll(msg); ok {
			return m, cmd
		}
	}

	return nil, nil
}

// handleDetailScroll lets the left-hand panels scroll the detail pane.
func (a *App) handleDetailScroll(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch a.keymap.Match(msg.String()) { //nolint:exhaustive // only the four scroll actions apply
	case ActDetailScrollDown:
		a.detailView.ScrollBy(1)
		return a, nil, true
	case ActDetailScrollUp:
		a.detailView.ScrollBy(-1)
		return a, nil, true
	case ActDetailHalfDown:
		a.detailView.ScrollBy(a.detailView.VisibleRows() / 2)
		return a, nil, true
	case ActDetailHalfUp:
		a.detailView.ScrollBy(-a.detailView.VisibleRows() / 2)
		return a, nil, true
	}
	return nil, nil, false
}

func (a *App) handleHelpKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.helpSearching {
		return a.handleHelpSearchKey(msg)
	}

	bindings := a.filteredHelpBindings()
	key := msg.String()

	if key == "/" {
		a.helpSearching = true
		a.helpSearch.SetValue("")
		a.helpSearch.SetWidth(a.width - 5)
		return a, nil
	}
	if key == "esc" || key == "q" || key == "?" {
		a.showHelp = false
		a.helpFilter = ""
		a.helpSearching = false
		return a, nil
	}

	switch a.keymap.MatchNav(key) {
	case components.NavNone:
	case components.NavDown:
		if a.helpCursor < len(bindings)-1 {
			a.helpCursor++
		}
	case components.NavUp:
		if a.helpCursor > 0 {
			a.helpCursor--
		}
	case components.NavTop:
		a.helpCursor = 0
	case components.NavBottom:
		if len(bindings) > 0 {
			a.helpCursor = len(bindings) - 1
		}
	case components.NavHalfDown:
		halfPage := max(1, min(len(bindings), a.height-8)/2)
		a.helpCursor += halfPage
		if a.helpCursor >= len(bindings) {
			a.helpCursor = len(bindings) - 1
		}
	case components.NavHalfUp:
		halfPage := max(1, min(len(bindings), a.height-8)/2)
		a.helpCursor -= halfPage
		if a.helpCursor < 0 {
			a.helpCursor = 0
		}
	}
	return a, nil
}

func (a *App) handleHelpSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	filtered := a.filteredHelpBindings()
	switch msg.String() {
	case "enter":
		a.helpConfirmSearch()
	case "esc":
		a.helpSearching = false
		a.helpFilter = ""
		a.helpCursor = 0
	case "down", components.KeyCtrlJ:
		if a.helpCursor < len(filtered)-1 {
			a.helpCursor++
		}
	case "up", components.KeyCtrlK:
		if a.helpCursor > 0 {
			a.helpCursor--
		}
	default:
		updated, changed := a.helpSearch.Update(msg)
		a.helpSearch = updated
		if changed {
			a.helpFilter = a.helpSearch.Value()
			a.helpCursor = 0
		}
	}
	return a, nil
}

func (a *App) helpConfirmSearch() {
	var matchedBinding *Binding
	filtered := a.filteredHelpBindings()
	if a.helpCursor >= 0 && a.helpCursor < len(filtered) {
		b := filtered[a.helpCursor]
		matchedBinding = &b
	}
	a.helpSearching = false
	a.helpFilter = ""
	a.helpSearch.SetValue("")
	all := a.ContextBindings()
	a.helpCursor = 0
	if matchedBinding != nil {
		for i, b := range all {
			if b.Key == matchedBinding.Key && b.Description == matchedBinding.Description {
				a.helpCursor = i
				break
			}
		}
	}
}

func (a *App) filteredHelpBindings() []Binding {
	bindings := a.ContextBindings()
	if a.helpFilter == "" {
		return bindings
	}
	query := strings.ToLower(a.helpFilter)
	var result []Binding
	for _, b := range bindings {
		if strings.Contains(strings.ToLower(b.Key), query) ||
			strings.Contains(strings.ToLower(b.Description), query) {
			result = append(result, b)
		}
	}
	return result
}

func (a *App) handleFocusAction(action Action) (tea.Model, tea.Cmd, bool) {
	switch action { //nolint:exhaustive // only the focus actions are handled here
	case ActSwitchPanel:
		if a.side == sideLeft {
			a.side = sideRight
		} else {
			a.side = sideLeft
		}
		a.updateFocusState()
		return a, nil, true

	case ActFocusRight:
		if a.side == sideLeft {
			switch a.leftFocus {
			case focusStatus:
				a.leftFocus = focusPullRequests
			case focusPullRequests:
				a.leftFocus = focusBuilds
			case focusBuilds:
				a.leftFocus = focusRepos
			case focusRepos:
				a.leftFocus = focusStatus
			}
			a.updateFocusState()
			return a, nil, true
		}

	case ActFocusLeft:
		if a.side == sideLeft {
			switch a.leftFocus {
			case focusStatus:
				a.leftFocus = focusRepos
			case focusPullRequests:
				a.leftFocus = focusStatus
			case focusBuilds:
				a.leftFocus = focusPullRequests
			case focusRepos:
				a.leftFocus = focusBuilds
			}
			a.updateFocusState()
			return a, nil, true
		}
		a.side = sideLeft
		a.updateFocusState()
		return a, nil, true

	case ActFocusDetail:
		a.side = sideRight
		a.updateFocusState()
		return a, nil, true

	case ActFocusStatus:
		a.side = sideLeft
		a.leftFocus = focusStatus
		a.showSplash()
		a.updateFocusState()
		return a, nil, true

	case ActFocusPullRequests:
		a.side = sideLeft
		a.leftFocus = focusPullRequests
		a.updateFocusState()
		return a, a.previewSelected(), true

	case ActFocusBuilds:
		a.side = sideLeft
		a.leftFocus = focusBuilds
		a.updateFocusState()
		return a, nil, true

	case ActFocusRepos:
		a.side = sideLeft
		a.leftFocus = focusRepos
		a.updateFocusState()
		if repo := a.repoList.SelectedRepo(); repo != nil {
			a.detailView.SetRepo(repo)
		}
		return a, nil, true
	}
	return nil, nil, false
}

func (a *App) handleTabAction(action Action) (tea.Model, tea.Cmd, bool) {
	switch action { //nolint:exhaustive // only the two tab actions are handled here
	case ActPrevTab:
		if a.side == sideRight {
			a.detailView.PrevTab()
			return a, a.onDetailTabChanged(), true
		}
		if a.leftFocus == focusPullRequests {
			a.prList.PrevTab()
			return a, a.afterTabSwitch(), true
		}
		return a, nil, true

	case ActNextTab:
		if a.side == sideRight {
			a.detailView.NextTab()
			return a, a.onDetailTabChanged(), true
		}
		if a.leftFocus == focusPullRequests {
			a.prList.NextTab()
			return a, a.afterTabSwitch(), true
		}
		return a, nil, true
	}
	return nil, nil, false
}

// afterTabSwitch fetches the new tab when nothing has been loaded for it, and
// otherwise just previews whatever the cursor landed on. It goes through
// refreshCmd so a tab switch also re-runs repository discovery when that is
// what left the list empty.
func (a *App) afterTabSwitch() tea.Cmd {
	if !a.prList.HasCachedTab() {
		return a.refreshCmd()
	}
	return tea.Batch(a.previewSelected(), a.startPrefetch())
}

// handleReviewAction is the detail panel's own key set: the four tab jumps, the
// changed-file stepping and the editor. It acts only while the detail panel is
// focused; with a left panel focused these keys fall through to routeToPanel
// and the panel decides what, if anything, they mean there.
func (a *App) handleReviewAction(action Action) (tea.Model, tea.Cmd, bool) {
	if a.side != sideRight {
		return nil, nil, false
	}
	switch action { //nolint:exhaustive // every other action is handled elsewhere
	case ActDetailOverview:
		return a, a.jumpToTab(views.TabOverview), true
	case ActDetailFiles:
		return a, a.jumpToTab(views.TabFiles), true
	case ActDetailCommits:
		return a, a.jumpToTab(views.TabCommits), true
	case ActDetailComments:
		return a, a.jumpToTab(views.TabComments), true
	case ActNextFile:
		return a, a.stepChangedFile(true), true
	case ActPrevFile:
		return a, a.stepChangedFile(false), true
	case ActOpenEditor:
		return a, a.openEditorAtCursor(), true
	}
	return nil, nil, false
}

// jumpToTab puts one detail tab on screen and warms it. SetActiveTab leaves
// ModeFileDiff on the way, so a tab key pressed inside a diff lands on the tab
// rather than on a diff wearing another tab's title.
func (a *App) jumpToTab(tab views.DetailTab) tea.Cmd {
	mode := a.detailView.Mode()
	if mode != views.ModePullRequest && mode != views.ModeFileDiff {
		return nil
	}
	a.detailView.SetActiveTab(tab)
	return a.onDetailTabChanged()
}

// stepChangedFile moves to the neighbouring changed file: the cursor on the
// Files tab, the whole diff view in ModeFileDiff. It stops at both ends rather
// than wrapping, because wrapping while reading a diff reads as a glitch.
func (a *App) stepChangedFile(forward bool) tea.Cmd {
	mode := a.detailView.Mode()
	onFilesTab := mode == views.ModePullRequest && a.detailView.ActiveTab() == views.TabFiles
	if mode != views.ModeFileDiff && !onFilesTab {
		return nil
	}
	pr := a.currentPullRequest()
	if pr == nil {
		return nil
	}
	// The keys work before the Files tab has ever been opened, so the list may
	// still have to be fetched before there is anything to step through.
	if len(a.detailView.DiffStat()) == 0 {
		return a.ensureDiffStat(pr)
	}

	// Exactly one of the two may run: on the Files tab each call moves the list
	// cursor, so probing PrevFile before NextFile would step back and forth and
	// pin the cursor on the second file.
	step := a.detailView.PrevFile
	if forward {
		step = a.detailView.NextFile
	}
	path, ok := step()
	if !ok {
		a.helpBar.SetStatusMsg(fileEdgeNote(forward))
		return nil
	}
	if mode != views.ModeFileDiff {
		// On the Files tab the cursor has already moved; the diff opens on Enter.
		return nil
	}
	return a.openFileDiff(path)
}

// fileEdgeNote says which end of the changed-file list the cursor is stuck at.
func fileEdgeNote(forward bool) string {
	if forward {
		return "last file"
	}
	return "first file"
}

func (a *App) handlePullRequestAction(action Action) (tea.Model, tea.Cmd, bool) {
	switch action { //nolint:exhaustive // the rest of the actions are handled elsewhere
	case ActCopyURL:
		// Copy and browser share one target, so `y` always yields the address
		// `o` would have opened — the selected commit, the selected thread, the
		// diff, or the pull request.
		if url := a.browserURL(); url != "" {
			a.copyToClipboard(url)
		}
		return a, nil, true

	case ActBrowser:
		if url := a.browserURL(); url != "" {
			a.openBrowser(url)
		}
		return a, nil, true

	case ActCheckout:
		return a.handleActionCheckout()

	case ActRefresh:
		return a.handleActionRefresh()

	case ActRefreshAll:
		return a, a.refreshCmd(), true

	// The five write actions. Each one opens its own dialog and returns
	// without touching Bitbucket; see actions.go.
	case ActMerge:
		return a, a.handleActionMerge(), true

	case ActSync:
		return a, a.handleActionSync(), true

	case ActEdit:
		return a, a.handleActionEdit(), true

	case ActReviewers:
		return a, a.handleActionReviewers(), true

	case ActToggleDraft:
		return a, a.handleActionToggleDraft(), true
	}
	return nil, nil, false
}

// browserURL is what `o` opens, which depends on what the user is looking at.
func (a *App) browserURL() string {
	if a.side == sideRight {
		if url := a.detailBrowserURL(); url != "" {
			return url
		}
		if pr := a.currentPullRequest(); pr != nil {
			return pr.HTMLURL()
		}
		return ""
	}

	switch a.leftFocus {
	case focusBuilds:
		if status := a.buildsPanel.SelectedStatus(); status != nil {
			return status.URL
		}
	case focusRepos:
		if repo := a.repoList.SelectedRepo(); repo != nil {
			return repo.URL
		}
	case focusPullRequests, focusStatus:
		if pr := a.currentPullRequest(); pr != nil {
			return pr.HTMLURL()
		}
	}
	return ""
}

// detailBrowserURL is the deep link the detail panel's own contents deserve,
// "" when the pull request itself is the right target.
func (a *App) detailBrowserURL() string {
	switch a.detailView.Mode() {
	case views.ModePipeline, views.ModeStepLog:
		if pipeline := a.detailView.Pipeline(); pipeline != nil {
			if url := pipeline.HTMLURL(); url != "" {
				return url
			}
		}
		if status := a.buildsPanel.SelectedStatus(); status != nil {
			return status.URL
		}
	case views.ModeRepo:
		// The panel is showing a repository, so `o` opens that repository
		// rather than whichever pull request happens to be selected.
		if repo := a.repoList.SelectedRepo(); repo != nil && repo.URL != "" {
			return repo.URL
		}
	case views.ModeFileDiff:
		// Bitbucket has no per-file anchor worth relying on, so the diff view
		// opens the pull request's whole diff.
		if pr := a.currentPullRequest(); pr != nil {
			if url := pr.HTMLURL(); url != "" {
				return url + "/diff"
			}
		}
	case views.ModePullRequest:
		return a.reviewTabURL()
	case views.ModeSplash:
		// The splash screen carries no selection.
	}
	return ""
}

// reviewTabURL is the deep link of the active review tab's cursor: the commit
// or the thread the reader is on. "" falls back to the pull request.
func (a *App) reviewTabURL() string {
	switch a.detailView.ActiveTab() {
	case views.TabCommits:
		if commit := a.detailView.SelectedCommit(); commit != nil {
			return commit.HTMLURL()
		}
	case views.TabComments:
		if thread := a.detailView.SelectedThread(); thread != nil {
			return thread.Root.HTMLURL()
		}
	case views.TabOverview, views.TabFiles:
		// Both are faces of the pull request itself.
	}
	return ""
}

// handleActionCheckout checks out the selected pull request's source branch.
func (a *App) handleActionCheckout() (tea.Model, tea.Cmd, bool) {
	pr := a.currentPullRequest()
	if pr == nil {
		return a, nil, true
	}
	if a.gitRepoPath == "" {
		a.statusPanel.SetError("not inside a git repository")
		return a, nil, true
	}
	branch := pr.SourceBranch()
	if branch == "" {
		return a, nil, true
	}
	return a, a.gitCheckoutBranch(pr.Workspace(), pr.RepoSlug(), branch), true
}

// handleActionRefresh drops the selected pull request's caches and refetches
// its detail and statuses.
func (a *App) handleActionRefresh() (tea.Model, tea.Cmd, bool) {
	pr := a.currentPullRequest()
	if pr == nil {
		return a, nil, true
	}
	key := pr.Key()
	delete(a.prCache, key)
	delete(a.statusCache, key)
	a.dropReviewCaches(key)
	a.previewEpoch++
	a.logFlag.Store(true)
	cmds := []tea.Cmd{
		a.fetchStatuses(pr, a.previewEpoch),
		a.fetchPullRequestDetail(pr, a.previewEpoch),
	}
	// Whatever review data is on screen is refetched as well; the tabs the user
	// has not opened stay cold.
	if cmd := a.refreshReviewData(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	return a, tea.Batch(cmds...), true
}

// handleActivate is Enter and space: open the pull request detail, drill into a
// build, or apply a repository filter.
func (a *App) handleActivate() (tea.Model, tea.Cmd) {
	if a.side != sideLeft {
		// The detail panel owns activation (open a file's diff, a step's log,
		// expand a commit or thread). Dispatch through the resolved action so a
		// rebound `pullRequests.open`/`select` key keeps working there too.
		return a, a.detailView.Activate()
	}
	switch a.leftFocus {
	case focusPullRequests:
		if a.prList.SelectedPullRequest() == nil {
			return a, nil
		}
		a.side = sideRight
		a.updateFocusState()
		return a, nil
	case focusBuilds:
		return a.openBuild(a.buildsPanel.SelectedStatus())
	case focusRepos:
		return a, a.selectRepo(a.repoList.SelectedRepo())
	case focusStatus:
		return a, nil
	}
	return a, nil
}

// handleActionURLPicker opens every URL reachable from the pull request.
func (a *App) handleActionURLPicker() (tea.Model, tea.Cmd) {
	pr := a.currentPullRequest()
	if pr == nil {
		return a, nil
	}
	key := pr.Key()
	groups := views.ExtractURLs(pr, a.statusCache[key].statuses,
		a.commitsCache[key].value, a.threadsCache[key].value)
	if len(groups) == 0 {
		return a, nil
	}

	var items []components.ModalItem
	for i, group := range groups {
		if i > 0 || len(groups) > 1 {
			items = append(items, components.ModalItem{Label: group.Section, Separator: true})
		}
		for _, url := range group.URLs {
			display := strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "http://")
			items = append(items, components.ModalItem{ID: url, Label: display})
		}
	}
	a.onSelect = func(item components.ModalItem) tea.Cmd {
		if strings.HasPrefix(item.ID, "http") {
			a.openBrowser(item.ID)
		}
		return nil
	}
	a.modal.Show("URLs", items)
	return a, nil
}
