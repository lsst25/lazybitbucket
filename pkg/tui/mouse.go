package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

type panelID int

const (
	panelStatus panelID = iota
	panelPullRequests
	panelBuilds
	panelRepos
	panelDetail
	panelLog
	// panelPipelines is the pipelines view's [2] panel; see focusPipelines.
	panelPipelines
)

func (a *App) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	x, y := msg.X, msg.Y
	panel, relY := a.hitTest(x, y)

	switch {
	case msg.Button == tea.MouseButtonWheelUp:
		return a.mouseScroll(panel, -3)
	case msg.Button == tea.MouseButtonWheelDown:
		return a.mouseScroll(panel, 3)
	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
		return a.mouseClick(panel, relY, x)
	}
	return a, nil
}

// hitTest determines which panel the coordinates fall in and the relative Y.
// It walks the view's own left column, whose panels the last layout sized —
// the panels a view does not draw were zeroed there, so a click cannot land on
// one.
func (a *App) hitTest(x, y int) (panelID, int) {
	column := a.leftColumn()

	if a.isVerticalLayout() {
		// Vertical: all stacked.
		top := 0
		for _, panel := range column {
			if y < top+panel.height {
				return panel.id, y - top
			}
			top += panel.height
		}
		if y < top+a.panelDetailH {
			return panelDetail, y - top
		}
		return panelLog, y - top - a.panelDetailH
	}

	// Horizontal layout. The bottom panel of the column catches everything
	// below it, so a click in the gap under a short list still selects a row.
	if x < a.panelSideW {
		top := 0
		for _, panel := range column[:len(column)-1] {
			if y < top+panel.height {
				return panel.id, y - top
			}
			top += panel.height
		}
		return column[len(column)-1].id, y - top
	}

	// Right side.
	if y < a.panelDetailH {
		return panelDetail, y
	}
	return panelLog, y - a.panelDetailH
}

func (a *App) mouseScroll(panel panelID, delta int) (tea.Model, tea.Cmd) {
	step := 1
	if delta < 0 {
		step = -1
	}

	switch panel { //nolint:exhaustive // the log panel and the status panel do not scroll
	case panelPullRequests:
		a.focusLeftPanel(focusPullRequests)
		a.userTookTheCursor()
		a.prList.ScrollBy(step)
		return a, a.previewSelected()
	case panelBuilds:
		a.focusLeftPanel(focusBuilds)
		a.buildsPanel.ScrollBy(step)
	case panelPipelines:
		a.focusLeftPanel(focusPipelines)
		a.pipelineList.ScrollBy(step)
		return a, a.showSelectedRun()
	case panelRepos:
		a.focusLeftPanel(focusRepos)
		a.repoList.ScrollBy(step)
		if repo := a.repoList.SelectedRepo(); repo != nil {
			a.detailView.SetRepo(repo)
		}
	case panelDetail:
		if a.side != sideRight {
			a.side = sideRight
			a.updateFocusState()
		}
		a.detailView.ScrollBy(step)
	}
	return a, nil
}

// focusLeftPanel moves focus to a left panel if it is not already there.
func (a *App) focusLeftPanel(panel focusPanel) {
	if a.side == sideLeft && a.leftFocus == panel {
		return
	}
	a.side = sideLeft
	a.leftFocus = panel
	a.updateFocusState()
}

func (a *App) mouseClick(panel panelID, relY, x int) (tea.Model, tea.Cmd) {
	switch panel { //nolint:exhaustive // clicking the log panel does nothing
	case panelStatus:
		a.focusLeftPanel(focusStatus)
		a.showSplash()

	case panelPullRequests:
		a.focusLeftPanel(focusPullRequests)
		a.userTookTheCursor()
		if relY == 0 {
			// Title bar — tab click.
			if a.prList.ClickTabAt(x) && !a.prList.HasCachedTab() {
				return a, a.refreshCmd()
			}
			return a, a.previewSelected()
		}
		a.prList.ClickAt(relY)
		return a, a.previewSelected()

	case panelBuilds:
		a.focusLeftPanel(focusBuilds)
		if relY > 0 && a.buildsPanel.ClickAt(relY) {
			return a.openBuild(a.buildsPanel.SelectedStatus())
		}

	case panelPipelines:
		// One click is enough: selecting a run is what shows it, so there is
		// nothing a double click would add.
		a.focusLeftPanel(focusPipelines)
		if relY > 0 {
			a.pipelineList.ClickAt(relY)
			return a, a.showSelectedRun()
		}

	case panelRepos:
		a.focusLeftPanel(focusRepos)
		if dbl := a.repoList.ClickAt(relY); dbl {
			return a, a.selectRepo(a.repoList.SelectedRepo())
		}
		if repo := a.repoList.SelectedRepo(); repo != nil {
			a.detailView.SetRepo(repo)
		}

	case panelDetail:
		if a.side != sideRight {
			a.side = sideRight
			a.updateFocusState()
		}
		if relY == 0 {
			relX := x
			if !a.isVerticalLayout() {
				relX = x - a.panelSideW
			}
			// A click that actually changes the tab warms it, exactly as the
			// tab keys do.
			if a.detailView.ClickTab(relX) {
				return a, a.onDetailTabChanged()
			}
			return a, nil
		}
		if cmd := a.detailView.ClickItem(relY); cmd != nil {
			return a, cmd
		}
	}
	return a, nil
}
