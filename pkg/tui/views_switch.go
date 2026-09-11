package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/config"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
)

// appView is which screen the app is on. The detail panel is shared between
// them; the left column is what a view swaps, which is why the panel numbers
// are per view rather than per panel.
type appView int

const (
	viewPullRequests appView = iota
	viewPipelines
	appViewCount
)

// The name of the view the switch key goes to, which is what the binding and
// the help strip are worded from. They are the views' own names rather than
// their panels', because that is what a reader who has just left one is
// looking for.
const (
	viewNamePullRequests = "pull requests"
	viewNamePipelines    = "pipelines"
)

// statusViewName is what the status panel appends to its title in the
// pipelines view. The pull request view appends nothing: its panels are the
// ones the numbering is named after.
const statusViewName = "Pipelines"

// reposPanelNumber is the number the Repos panel announces in each view. The
// pipelines view has one panel fewer above it, so Repos moves up to [3].
const (
	reposPanelPullRequests = 4
	reposPanelPipelines    = 3
)

// viewFromConfig maps gui.startView to a view; anything the config validator
// would have rejected, and the empty default, is the pull request view.
func viewFromConfig(value string) appView {
	if value == config.StartViewPipelines {
		return viewPipelines
	}
	return viewPullRequests
}

// cycleView moves step views forward (or back, for a negative step), wrapping
// at both ends exactly as cycleScreenMode does.
func (a *App) cycleView(step int) {
	n := int(appViewCount)
	a.view = appView(((int(a.view)+step)%n + n) % n)
}

// mainPanel is the view's [2] panel: the one a view switch lands the focus on,
// because it is the list the view exists for.
func (a *App) mainPanel() focusPanel {
	if a.view == viewPipelines {
		return focusPipelines
	}
	return focusPullRequests
}

// leftFocusOrder is the left column top to bottom, which is the order the
// focus keys walk it in.
func (a *App) leftFocusOrder() []focusPanel {
	if a.view == viewPipelines {
		return []focusPanel{focusStatus, focusPipelines, focusRepos}
	}
	return []focusPanel{focusStatus, focusPullRequests, focusBuilds, focusRepos}
}

// stepLeftFocus moves the left-column focus by step panels, wrapping, so h and
// l walk whichever panels the view on screen actually draws.
func (a *App) stepLeftFocus(step int) {
	order := a.leftFocusOrder()
	index := 0
	for i, panel := range order {
		if panel == a.leftFocus {
			index = i
			break
		}
	}
	n := len(order)
	a.leftFocus = order[((index+step)%n+n)%n]
}

// viewName is the name the status panel shows beside its own.
func (a *App) viewName() string {
	if a.view == viewPipelines {
		return statusViewName
	}
	return ""
}

// otherViewName is the view the switch key goes to, which is how the key is
// described: a reader wants to know where it takes them, not what they are
// leaving.
func (a *App) otherViewName() string {
	if a.view == viewPipelines {
		return viewNamePullRequests
	}
	return viewNamePipelines
}

// reposPanelNumber is the number the Repos panel wears in the current view.
func (a *App) reposPanelNumber() int {
	if a.view == viewPipelines {
		return reposPanelPipelines
	}
	return reposPanelPullRequests
}

// applyView tells the panels which view they are in: the Repos panel's number
// and the status panel's suffix both move with it, and they are the only two
// panels both views share on the left.
func (a *App) applyView() {
	a.repoList.SetPanelNumber(a.reposPanelNumber())
	a.statusPanel.SetViewName(a.viewName())
}

// switchView is what the view switch key does. Focus goes to the left column's
// [2] panel — each view keeps its own selection, caches and scroll, so the only
// thing that has to move is the detail panel, which is re-pointed at whatever
// the view being entered has selected.
func (a *App) switchView() tea.Cmd {
	a.cycleView(1)
	a.side = sideLeft
	a.leftFocus = a.mainPanel()
	a.applyView()

	var cmds []tea.Cmd
	if a.view == viewPipelines {
		// The list costs a round of requests per repository, so it is fetched
		// on the first entry into the view and then only on demand.
		if !a.pipelinesLoaded {
			cmds = append(cmds, a.fetchPipelines())
		}
		cmds = append(cmds, a.showSelectedRun())
	} else {
		a.showPreviewedPullRequest()
	}

	a.updateFocusState()
	return tea.Batch(cmds...)
}

// showPreviewedPullRequest re-points the detail panel at the pull request the
// list is on, or at the splash screen when nothing is selected yet — which is
// what the panel showed before the user left this view.
func (a *App) showPreviewedPullRequest() {
	if pr := a.currentPullRequest(); pr != nil {
		a.detailView.SetPullRequest(pr)
		return
	}
	a.showSplash()
}

// viewSwitchBinding is the help overlay's row for the switch key, described as
// the view it goes to.
func (a *App) viewSwitchBinding() Binding {
	return a.bind(ActNextView, "switch to the "+a.otherViewName()+" view")
}

// viewSwitchHelpItem is the same key on the one-line strip, where only the
// destination fits.
func (a *App) viewSwitchHelpItem() components.HelpItem {
	return components.HelpItem{Key: viewSwitchLabel(a.keymap.Keys(ActNextView)), Description: a.otherViewName()}
}

// viewSwitchLabel shortens the default binding to the glyph the strip has room
// for; a rebound key is shown exactly as it was configured.
func viewSwitchLabel(key string) string {
	if key == defaultNextViewKey {
		return "⇧tab"
	}
	return key
}
