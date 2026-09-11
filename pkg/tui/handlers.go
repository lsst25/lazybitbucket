package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/tui/components"
)

// handleResize re-lays out the panels for a new terminal size.
func (a *App) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	a.width = msg.Width
	a.height = msg.Height
	a.overlays.SetSize(msg.Width, msg.Height)
	a.layoutPanels()
	a.helpBar.SetWidth(msg.Width)
	a.searchBar.SetWidth(msg.Width)
	return a, nil
}

// handleSearchChanged filters the focused list as the user types.
func (a *App) handleSearchChanged(msg components.SearchChangedMsg) (tea.Model, tea.Cmd) {
	if a.side != sideLeft {
		return a, nil
	}
	switch a.leftFocus {
	case focusPullRequests:
		a.prList.SetFilter(msg.Query)
	case focusBuilds:
		a.buildsPanel.SetFilter(msg.Query)
	case focusPipelines:
		a.pipelineList.SetFilter(msg.Query)
	case focusRepos:
		a.repoList.SetFilter(msg.Query)
	case focusStatus:
	}
	return a, nil
}

// handleSearchConfirmed drops the filter and keeps the cursor where the search
// landed, so the next action targets what the user searched for.
func (a *App) handleSearchConfirmed() (tea.Model, tea.Cmd) {
	if a.side != sideLeft {
		return a, nil
	}
	switch a.leftFocus {
	case focusPullRequests:
		selected := a.prList.SelectedPullRequest()
		a.prList.ClearFilter()
		if selected != nil {
			return a, a.previewSelected()
		}
	case focusBuilds:
		// Keep the highlighted build across the filter reset, like the pull
		// request pane does, so Enter afterwards opens the build the user saw.
		selected := a.buildsPanel.SelectedStatus()
		a.buildsPanel.ClearFilter()
		if selected != nil {
			a.buildsPanel.SelectByKey(selected.Key)
		}
	case focusPipelines:
		// Keep the highlighted run across the filter reset, so Enter afterwards
		// opens the run the user searched for.
		selected := a.pipelineList.SelectedEntry()
		a.pipelineList.ClearFilter()
		if selected != nil {
			a.pipelineList.SelectByRun(selected.Repo, selected.Pipeline.BuildNumber)
			return a, a.showSelectedRun()
		}
	case focusRepos:
		repo := a.repoList.SelectedRepo()
		a.repoList.SetFilter("")
		if repo != nil {
			return a, a.selectRepo(repo)
		}
	case focusStatus:
	}
	return a, nil
}

// handleSearchCancelled clears every list filter.
func (a *App) handleSearchCancelled() (tea.Model, tea.Cmd) {
	a.prList.ClearFilter()
	a.buildsPanel.ClearFilter()
	a.pipelineList.ClearFilter()
	a.repoList.SetFilter("")
	return a, nil
}

// handleAutoFetch refreshes the active tab and re-arms the timer.
func (a *App) handleAutoFetch() (tea.Model, tea.Cmd) {
	if a.refreshEvery <= 0 {
		return a, nil
	}
	var cmds []tea.Cmd
	if cmd := a.refreshCmd(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	cmds = append(cmds, autoFetchTick(a.refreshEvery))
	return a, tea.Batch(cmds...)
}

// routeToPanel forwards an unclaimed message to the focused panel.
func (a *App) routeToPanel(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	if a.side == sideLeft {
		switch a.leftFocus {
		case focusPullRequests:
			updated, cmd := a.prList.Update(msg)
			a.prList = updated
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		case focusBuilds:
			updated, cmd := a.buildsPanel.Update(msg)
			a.buildsPanel = updated
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		case focusPipelines:
			updated, cmd := a.pipelineList.Update(msg)
			a.pipelineList = updated
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		case focusRepos:
			updated, cmd := a.repoList.Update(msg)
			a.repoList = updated
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		case focusStatus:
			updated, cmd := a.statusPanel.Update(msg)
			a.statusPanel = updated
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	} else {
		updated, cmd := a.detailView.Update(msg)
		a.detailView = updated
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}
