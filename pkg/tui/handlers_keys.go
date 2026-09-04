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
			return a, nil, true
		}
		if a.leftFocus == focusPullRequests {
			a.prList.PrevTab()
			return a, a.afterTabSwitch(), true
		}
		return a, nil, true

	case ActNextTab:
		if a.side == sideRight {
			a.detailView.NextTab()
			return a, nil, true
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

func (a *App) handlePullRequestAction(action Action) (tea.Model, tea.Cmd, bool) {
	switch action { //nolint:exhaustive // the rest of the actions are handled elsewhere
	case ActCopyURL:
		if pr := a.currentPullRequest(); pr != nil {
			a.copyToClipboard(pr.HTMLURL())
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
	}
	return nil, nil, false
}

// browserURL is what `o` opens, which depends on what the user is looking at.
func (a *App) browserURL() string {
	if a.side == sideRight {
		mode := a.detailView.Mode()
		if mode == views.ModePipeline || mode == views.ModeStepLog {
			if pipeline := a.detailView.Pipeline(); pipeline != nil {
				if url := pipeline.HTMLURL(); url != "" {
					return url
				}
			}
			if status := a.buildsPanel.SelectedStatus(); status != nil {
				return status.URL
			}
		}
		if mode == views.ModeRepo {
			// The panel is showing a repository, so `o` opens that repository
			// rather than whichever pull request happens to be selected.
			if repo := a.repoList.SelectedRepo(); repo != nil && repo.URL != "" {
				return repo.URL
			}
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
	return a, a.gitCheckoutBranch(branch), true
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
	a.previewEpoch++
	a.logFlag.Store(true)
	return a, tea.Batch(
		a.fetchStatuses(pr, a.previewEpoch),
		a.fetchPullRequestDetail(pr, a.previewEpoch),
	), true
}

// handleActivate is Enter and space: open the pull request detail, drill into a
// build, or apply a repository filter.
func (a *App) handleActivate() (tea.Model, tea.Cmd) {
	if a.side != sideLeft {
		// The detail panel handles activation itself (a pipeline step's log).
		return nil, nil
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
	groups := views.ExtractURLs(pr, a.statusCache[pr.Key()].statuses)
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
