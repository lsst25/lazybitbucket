package tui

// isVerticalLayout returns true when terminal is too narrow for side-by-side.
func (a *App) isVerticalLayout() bool {
	return a.width < 80
}

// sideWidth calculates the left panel width, shrinking for narrow terminals.
func (a *App) sideWidth() int {
	if a.isVerticalLayout() {
		return a.width
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
