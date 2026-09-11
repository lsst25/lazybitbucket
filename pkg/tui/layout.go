package tui

import "github.com/lsst25/lazybitbucket/pkg/config"

// screenMode is how much of the window the panels share, lazygit's screen
// modes: normal is the configured split, half gives the side panels half the
// width, full hands the whole window to the focused panel. The narrow, stacked
// layout knows only normal and full — half has nothing to halve there.
type screenMode int

const (
	screenNormal screenMode = iota
	screenHalf
	screenFull
	screenModeCount
)

// screenModeFromConfig maps gui.screenMode to a screenMode; anything the
// config validator would have rejected, and the empty default, is normal.
func screenModeFromConfig(value string) screenMode {
	switch value {
	case config.ScreenModeHalf:
		return screenHalf
	case config.ScreenModeFull:
		return screenFull
	default:
		return screenNormal
	}
}

// cycleScreenMode moves step modes forward (or back, for a negative step),
// wrapping at both ends: normal → half → full → normal.
func (a *App) cycleScreenMode(step int) {
	n := int(screenModeCount)
	a.screen = screenMode(((int(a.screen)+step)%n + n) % n)
}

// fullScreenPanel is the one panel on screen in full mode: the detail panel
// when the right side is focused, else the focused left panel. ok is false
// outside full mode.
func (a *App) fullScreenPanel() (panelID, bool) {
	if a.screen != screenFull {
		return 0, false
	}
	if a.side == sideRight {
		return panelDetail, true
	}
	switch a.leftFocus {
	case focusStatus:
		return panelStatus, true
	case focusBuilds:
		return panelBuilds, true
	case focusPipelines:
		return panelPipelines, true
	case focusRepos:
		return panelRepos, true
	case focusPullRequests:
		return panelPullRequests, true
	default:
		return panelPullRequests, true
	}
}

// panelView renders one panel by id. The log panel is never shown alone: in
// full mode the detail panel takes its rows too.
func (a *App) panelView(panel panelID) string {
	switch panel {
	case panelStatus:
		return a.statusPanel.View()
	case panelPullRequests:
		return a.prList.View()
	case panelBuilds:
		return a.buildsPanel.View()
	case panelPipelines:
		return a.pipelineList.View()
	case panelRepos:
		return a.repoList.View()
	case panelDetail, panelLog:
		return a.detailView.View()
	default:
		return a.detailView.View()
	}
}

// isVerticalLayout returns true when terminal is too narrow for side-by-side.
func (a *App) isVerticalLayout() bool {
	return a.width < 80
}

// sideWidth calculates the left panel width, shrinking for narrow terminals.
// In half mode the side panels take exactly half the window, whatever
// sidePanelWidth says.
func (a *App) sideWidth() int {
	if a.isVerticalLayout() {
		return a.width
	}
	if a.screen == screenHalf {
		return a.width / 2
	}
	sideW := a.cfg.GUI.SidePanelWidth
	if sideW <= 0 {
		sideW = 40
	}
	if a.width < 120 && sideW > a.width*35/100 {
		sideW = a.width * 35 / 100
	}
	if sideW > a.width/2 {
		sideW = a.width / 2
	}
	if sideW < 25 {
		sideW = 25
	}
	return sideW
}

// panelExtent is one left-column panel and the height the last layout gave it,
// which is what hitTest walks.
type panelExtent struct {
	id     panelID
	height int
}

// leftColumn is the left-hand panels of the view on screen, top to bottom. It
// is the one place that knows what a view's left column is made of; layout,
// hit testing and rendering all read it.
func (a *App) leftColumn() []panelExtent {
	if a.view == viewPipelines {
		return []panelExtent{
			{panelStatus, a.panelStatusH},
			{panelPipelines, a.panelPipelinesH},
			{panelRepos, a.panelReposH},
		}
	}
	return []panelExtent{
		{panelStatus, a.panelStatusH},
		{panelPullRequests, a.panelPRH},
		{panelBuilds, a.panelBuildsH},
		{panelRepos, a.panelReposH},
	}
}

// leftColumnViews renders the left column of the view on screen, in order.
func (a *App) leftColumnViews() []string {
	column := a.leftColumn()
	out := make([]string, 0, len(column))
	for _, panel := range column {
		out = append(out, a.panelView(panel.id))
	}
	return out
}

func (a *App) layoutPanels() {
	totalH := a.height - 1

	if panel, ok := a.fullScreenPanel(); ok {
		a.layoutFullScreen(panel, totalH)
		return
	}
	if a.isVerticalLayout() {
		a.layoutVertical(totalH)
		return
	}
	a.layoutHorizontal(totalH)
}

// The narrow layout's constants: the detail panel's target height and the caps
// the two secondary panels may grow to when they are focused.
const (
	detailTargetH = 12 // enough for a pull request description
	reposCapH     = 8
	buildsCapH    = 8
)

// layoutVertical stacks every panel, giving the focused one what is left after
// the detail panel has had its target.
func (a *App) layoutVertical(totalH int) {
	if a.view == viewPipelines {
		a.layoutVerticalPipelines(totalH)
		return
	}
	a.layoutVerticalPullRequests(totalH)
}

func (a *App) layoutVerticalPullRequests(totalH int) {
	w := a.width
	logH := 5

	// Priority-based accordion: non-focused left panels are collapsed to one
	// line, the detail panel takes its target, and the focused left panel
	// takes the rest.
	statusH := 1
	prH := 1
	buildsH := 1
	reposH := 1
	var detailH int

	avail := totalH - statusH - logH

	switch {
	case a.side == sideRight:
		detailH = avail - prH - buildsH - reposH

	case a.leftFocus == focusPullRequests:
		remaining := avail - buildsH - reposH
		detailH = min(detailTargetH, max(remaining-1, 1))
		prH = remaining - detailH

	case a.leftFocus == focusBuilds:
		remaining := avail - prH - reposH
		detailH = min(detailTargetH, max(remaining-1, 1))
		buildsH = min(remaining-detailH, buildsCapH)
		detailH = remaining - buildsH

	case a.leftFocus == focusRepos:
		remaining := avail - prH - buildsH
		detailH = min(detailTargetH, max(remaining-1, 1))
		reposH = min(remaining-detailH, reposCapH)
		detailH = remaining - reposH

	case a.leftFocus == focusStatus:
		statusH = a.statusPanelHeight()
		avail -= statusH - 1
		remaining := avail - buildsH - reposH
		detailH = min(detailTargetH, max(remaining-1, 1))
		prH = remaining - detailH

	default:
		detailH = avail - prH - buildsH - reposH
	}

	prH = max(prH, 1)
	buildsH = max(buildsH, 1)
	reposH = max(reposH, 1)
	detailH = max(detailH, 3)

	a.statusPanel.SetSize(w, statusH)
	a.prList.SetSize(w, prH)
	a.buildsPanel.SetSize(w, buildsH)
	a.repoList.SetSize(w, reposH)
	a.detailView.SetSize(w, detailH)
	a.logPanel.SetSize(w, logH)
	a.panelSideW = w
	a.panelStatusH = statusH
	a.panelPRH = prH
	a.panelBuildsH = buildsH
	a.panelPipelinesH = 0
	a.panelReposH = reposH
	a.panelDetailH = detailH
	a.panelLogH = logH
}

// layoutVerticalPipelines is the same accordion over the pipelines view's
// three left panels, which has the pull request list and the builds panel
// replaced by the one pipelines list.
func (a *App) layoutVerticalPipelines(totalH int) {
	w := a.width
	logH := 5

	statusH := 1
	pipelinesH := 1
	reposH := 1
	var detailH int

	avail := totalH - statusH - logH

	switch {
	case a.side == sideRight:
		detailH = avail - pipelinesH - reposH

	case a.leftFocus == focusRepos:
		remaining := avail - pipelinesH
		detailH = min(detailTargetH, max(remaining-1, 1))
		reposH = min(remaining-detailH, reposCapH)
		detailH = remaining - reposH

	case a.leftFocus == focusStatus:
		statusH = a.statusPanelHeight()
		avail -= statusH - 1
		remaining := avail - reposH
		detailH = min(detailTargetH, max(remaining-1, 1))
		pipelinesH = remaining - detailH

	default: // the pipelines list is focused, and gets what is left
		remaining := avail - reposH
		detailH = min(detailTargetH, max(remaining-1, 1))
		pipelinesH = remaining - detailH
	}

	pipelinesH = max(pipelinesH, 1)
	reposH = max(reposH, 1)
	detailH = max(detailH, 3)

	a.statusPanel.SetSize(w, statusH)
	a.pipelineList.SetSize(w, pipelinesH)
	a.repoList.SetSize(w, reposH)
	a.detailView.SetSize(w, detailH)
	a.logPanel.SetSize(w, logH)
	a.panelSideW = w
	a.panelStatusH = statusH
	a.panelPRH = 0
	a.panelBuildsH = 0
	a.panelPipelinesH = pipelinesH
	a.panelReposH = reposH
	a.panelDetailH = detailH
	a.panelLogH = logH
}

// layoutHorizontal is the side-by-side layout: the view's left column beside
// the shared detail panel.
func (a *App) layoutHorizontal(totalH int) {
	sideW := a.sideWidth()
	mainW := a.width - sideW

	statusH := a.statusPanelHeight()
	a.statusPanel.SetSize(sideW, statusH)
	remaining := totalH - statusH

	if a.view == viewPipelines {
		a.layoutPipelinesColumn(sideW, remaining)
	} else {
		a.layoutPullRequestsColumn(sideW, remaining)
	}

	// Right column: log fits content or max 8, detail gets the rest.
	logH := 8
	detailH := max(totalH-logH, 5)

	a.detailView.SetSize(mainW, detailH)
	a.logPanel.SetSize(mainW, logH)

	// Cache sizes for mouse.
	a.panelSideW = sideW
	a.panelStatusH = statusH
	a.panelDetailH = detailH
	a.panelLogH = logH
}

// minPanelH is the smallest a side panel may be drawn at, borders included.
const minPanelH = 3

// collapsedH is the height a side panel is compacted to when another one has
// the focus.
func (a *App) collapsedH() int {
	return max(a.cfg.GUI.CollapsedPanelHeight, minPanelH)
}

// layoutPullRequestsColumn sizes Pull Requests, Builds and Repos, and zeroes
// the pipelines extent so a click cannot land on a panel this view does not
// draw.
func (a *App) layoutPullRequestsColumn(sideW, remaining int) {
	var prH, buildsH, reposH int
	collapsed := a.collapsedH()

	switch a.leftFocus {
	case focusRepos:
		// Repos focused: builds and pull requests compact.
		buildsH = collapsed
		prNat := max(min(a.prList.ContentHeight(), 12), minPanelH)
		prH = max(min(prNat, remaining/3), minPanelH)
		reposH = remaining - prH - buildsH
	case focusBuilds:
		// Builds focused: pull requests compact, repos compact.
		reposH = collapsed
		prNat := max(min(a.prList.ContentHeight(), 12), minPanelH)
		prH = max(min(prNat, remaining/3), minPanelH)
		buildsH = remaining - prH - reposH
	default:
		// Pull requests focused (or status): builds and repos compact, pull
		// requests get the space.
		buildsH = collapsed
		reposH = collapsed
		prH = remaining - buildsH - reposH
	}

	prH = max(prH, minPanelH)
	buildsH = max(buildsH, minPanelH)
	reposH = max(reposH, minPanelH)

	a.prList.SetSize(sideW, prH)
	a.buildsPanel.SetSize(sideW, buildsH)
	a.repoList.SetSize(sideW, reposH)

	a.panelPRH = prH
	a.panelBuildsH = buildsH
	a.panelPipelinesH = 0
	a.panelReposH = reposH
}

// layoutPipelinesColumn sizes Pipelines and Repos. The pipelines list gets the
// space the pull request list gets in the other view, and the two swap when
// Repos is focused, exactly as Builds and Repos do there.
func (a *App) layoutPipelinesColumn(sideW, remaining int) {
	var pipelinesH, reposH int

	if a.leftFocus == focusRepos {
		pipeNat := max(min(a.pipelineList.ContentHeight(), 12), minPanelH)
		pipelinesH = max(min(pipeNat, remaining/3), minPanelH)
		reposH = remaining - pipelinesH
	} else {
		reposH = a.collapsedH()
		pipelinesH = remaining - reposH
	}

	pipelinesH = max(pipelinesH, minPanelH)
	reposH = max(reposH, minPanelH)

	a.pipelineList.SetSize(sideW, pipelinesH)
	a.repoList.SetSize(sideW, reposH)

	a.panelPRH = 0
	a.panelBuildsH = 0
	a.panelPipelinesH = pipelinesH
	a.panelReposH = reposH
}

// layoutFullScreen sizes the one visible panel to the whole window and zeroes
// the cached extents of the rest, so hitTest lands every click on the panel
// that is actually on screen. Hidden panels keep their last size; they are not
// rendered, so it does not matter.
func (a *App) layoutFullScreen(panel panelID, totalH int) {
	w := a.width
	a.panelSideW, a.panelStatusH, a.panelPRH, a.panelBuildsH, a.panelReposH = 0, 0, 0, 0, 0
	a.panelPipelinesH, a.panelDetailH, a.panelLogH = 0, 0, 0

	switch panel {
	case panelStatus:
		a.statusPanel.SetSize(w, totalH)
		a.panelSideW, a.panelStatusH = w, totalH
	case panelPullRequests:
		a.prList.SetSize(w, totalH)
		a.panelSideW, a.panelPRH = w, totalH
	case panelBuilds:
		a.buildsPanel.SetSize(w, totalH)
		a.panelSideW, a.panelBuildsH = w, totalH
	case panelPipelines:
		a.pipelineList.SetSize(w, totalH)
		a.panelSideW, a.panelPipelinesH = w, totalH
	case panelRepos:
		a.repoList.SetSize(w, totalH)
		a.panelSideW, a.panelReposH = w, totalH
	case panelDetail, panelLog:
		a.detailView.SetSize(w, totalH)
		a.panelDetailH = totalH
	default:
		a.detailView.SetSize(w, totalH)
		a.panelDetailH = totalH
	}
}
