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

func (a *App) layoutPanels() {
	totalH := a.height - 1

	if panel, ok := a.fullScreenPanel(); ok {
		a.layoutFullScreen(panel, totalH)
		return
	}

	if a.isVerticalLayout() {
		w := a.width
		logH := 5

		// Priority-based accordion for vertical layout.
		// Non-focused left panels are collapsed (1 line).
		// Detail always gets its target first, then remaining
		// space goes to the focused left panel.
		const (
			detailTarget = 12 // enough for a pull request description
			reposCap     = 8  // hard cap for repos
			buildsCap    = 8  // hard cap for builds
		)

		statusH := 1 // always collapsed in vertical
		prH := 1
		buildsH := 1
		reposH := 1
		var detailH int

		avail := totalH - statusH - logH // space for detail + pull requests + builds + repos

		switch {
		case a.side == sideRight:
			// Detail focused: all left panels collapsed.
			detailH = avail - prH - buildsH - reposH

		case a.leftFocus == focusPullRequests:
			// Pull requests focused: others collapsed.
			remaining := avail - buildsH - reposH
			detailH = min(detailTarget, max(remaining-1, 1))
			prH = remaining - detailH

		case a.leftFocus == focusBuilds:
			// Builds focused: others collapsed.
			remaining := avail - prH - reposH
			detailH = min(detailTarget, max(remaining-1, 1))
			buildsH = min(remaining-detailH, buildsCap)
			detailH = remaining - buildsH

		case a.leftFocus == focusRepos:
			// Repos focused: others collapsed.
			remaining := avail - prH - buildsH
			detailH = min(detailTarget, max(remaining-1, 1))
			reposH = min(remaining-detailH, reposCap)
			detailH = remaining - reposH

		case a.leftFocus == focusStatus:
			statusH = a.statusPanelHeight()
			avail -= statusH - 1
			remaining := avail - buildsH - reposH
			detailH = min(detailTarget, max(remaining-1, 1))
			prH = remaining - detailH

		default:
			detailH = avail - prH - buildsH - reposH
		}

		if prH < 1 {
			prH = 1
		}
		if buildsH < 1 {
			buildsH = 1
		}
		if reposH < 1 {
			reposH = 1
		}
		if detailH < 3 {
			detailH = 3
		}

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
		a.panelReposH = reposH
		a.panelDetailH = detailH
		a.panelLogH = logH
		return
	}

	sideW := a.sideWidth()
	mainW := a.width - sideW

	statusH := a.statusPanelHeight()
	remaining := totalH - statusH

	var prH, buildsH, reposH int
	minH := 3
	collapsedH := max(a.cfg.GUI.CollapsedPanelHeight, minH)

	switch a.leftFocus {
	case focusRepos:
		// Repos focused: builds and pull requests compact.
		buildsH = collapsedH
		prNat := max(min(a.prList.ContentHeight(), 12), minH)
		prH = max(min(prNat, remaining/3), minH)
		reposH = remaining - prH - buildsH
	case focusBuilds:
		// Builds focused: pull requests compact, repos compact.
		reposH = collapsedH
		prNat := max(min(a.prList.ContentHeight(), 12), minH)
		prH = max(min(prNat, remaining/3), minH)
		buildsH = remaining - prH - reposH
	default:
		// Pull requests focused (or status): builds and repos compact, pull requests get the space.
		buildsH = collapsedH
		reposH = collapsedH
		prH = remaining - buildsH - reposH
	}

	if prH < minH {
		prH = minH
	}
	if buildsH < minH {
		buildsH = minH
	}
	if reposH < minH {
		reposH = minH
	}

	a.statusPanel.SetSize(sideW, statusH)
	a.prList.SetSize(sideW, prH)
	a.buildsPanel.SetSize(sideW, buildsH)
	a.repoList.SetSize(sideW, reposH)

	// Right column: log fits content or max 8, detail gets the rest.
	logH := 8
	detailH := max(totalH-logH, 5)

	a.detailView.SetSize(mainW, detailH)
	a.logPanel.SetSize(mainW, logH)

	// Cache sizes for mouse.
	a.panelSideW = sideW
	a.panelStatusH = statusH
	a.panelPRH = prH
	a.panelBuildsH = buildsH
	a.panelReposH = reposH
	a.panelDetailH = detailH
	a.panelLogH = logH
}

// layoutFullScreen sizes the one visible panel to the whole window and zeroes
// the cached extents of the rest, so hitTest lands every click on the panel
// that is actually on screen. Hidden panels keep their last size; they are not
// rendered, so it does not matter.
func (a *App) layoutFullScreen(panel panelID, totalH int) {
	w := a.width
	a.panelSideW, a.panelStatusH, a.panelPRH, a.panelBuildsH, a.panelReposH = 0, 0, 0, 0, 0
	a.panelDetailH, a.panelLogH = 0, 0

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
