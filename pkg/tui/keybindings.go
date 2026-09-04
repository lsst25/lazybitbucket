package tui

import (
	"slices"

	"github.com/lsst25/lazybitbucket/pkg/config"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// Binding is one keybinding row in the help overlay.
type Binding struct {
	Key         string
	Description string
}

func (a *App) bind(action Action, desc string) Binding {
	return Binding{Key: a.keymap.Keys(action), Description: desc}
}

func (a *App) navBindings() []Binding {
	return []Binding{
		a.bind(ActNavDown, "navigate down"),
		a.bind(ActNavUp, "navigate up"),
		a.bind(ActNavTop, "go to top"),
		a.bind(ActNavBottom, "go to bottom"),
		a.bind(ActNavHalfDown, "half-page down"),
		a.bind(ActNavHalfUp, "half-page up"),
	}
}

func (a *App) detailScrollBindings() []Binding {
	return []Binding{
		a.bind(ActDetailScrollDown, "scroll detail down"),
		a.bind(ActDetailScrollUp, "scroll detail up"),
		a.bind(ActDetailHalfDown, "half-page detail down"),
		a.bind(ActDetailHalfUp, "half-page detail up"),
	}
}

func (a *App) globalBindings() []Binding {
	return []Binding{
		a.bind(ActQuit, "quit"),
		a.bind(ActSwitchPanel, "switch left/right panels"),
		a.bind(ActFocusStatus, "focus status panel"),
		a.bind(ActFocusPullRequests, "focus pull requests panel"),
		a.bind(ActFocusBuilds, "focus builds panel"),
		a.bind(ActFocusRepos, "focus repos panel"),
		a.bind(ActSearch, "search / filter current list"),
		a.bind(ActRefresh, "refresh the selected pull request"),
		a.bind(ActRefreshAll, "refresh the pull request list"),
		a.bind(ActHelp, "show all keybindings"),
	}
}

// ContextBindings returns the keybindings that apply to the current focus.
func (a *App) ContextBindings() []Binding {
	global := a.globalBindings()

	if a.side == sideRight {
		bindings := slices.Concat(global, a.navBindings())
		switch a.detailView.Mode() {
		case views.ModePipeline, views.ModeStepLog:
			bindings = append(bindings,
				a.bind(ActBack, "back to the pull request"),
				a.bind(ActOpen, "open the step log"),
				a.bind(ActBrowser, "open the pipeline in a browser"),
			)
		case views.ModePullRequest, views.ModeRepo, views.ModeSplash:
			bindings = append(bindings,
				a.bind(ActBrowser, "open in browser"),
				a.bind(ActURLPicker, "open URL picker"),
				a.bind(ActCopyURL, "copy the pull request URL"),
			)
		}
		bindings = append(bindings, a.bind(ActFocusLeft, "back to the left panels"))
		bindings = append(bindings, a.customCommandBindings(config.CtxDetail)...)
		return bindings
	}

	switch a.leftFocus {
	case focusPullRequests:
		bindings := slices.Concat(global, a.navBindings(), a.detailScrollBindings())
		bindings = append(bindings,
			a.bind(ActOpen, "open the pull request detail"),
			a.bind(ActFocusRight, "next panel"),
			a.bind(ActBrowser, "open the pull request in a browser"),
			a.bind(ActURLPicker, "open URL picker"),
			a.bind(ActCopyURL, "copy the pull request URL"),
			a.bind(ActCheckout, "check out the source branch"),
			Binding{Key: "[]", Description: "switch tab"},
		)
		bindings = append(bindings, a.customCommandBindings(config.CtxPullRequests)...)
		return bindings

	case focusBuilds:
		bindings := slices.Concat(global, a.navBindings(), a.detailScrollBindings())
		bindings = append(bindings,
			a.bind(ActOpen, "open the pipeline run"),
			a.bind(ActBrowser, "open the build in a browser"),
			a.bind(ActFocusRight, "next panel"),
			a.bind(ActFocusLeft, "previous panel"),
		)
		bindings = append(bindings, a.customCommandBindings(config.CtxBuilds)...)
		return bindings

	case focusRepos:
		bindings := slices.Concat(global, a.navBindings())
		bindings = append(bindings,
			a.bind(ActSelect, "filter the list by this repository"),
			a.bind(ActBrowser, "open the repository in a browser"),
			a.bind(ActFocusRight, "next panel"),
			a.bind(ActFocusLeft, "previous panel"),
		)
		bindings = append(bindings, a.customCommandBindings(config.CtxRepos)...)
		return bindings

	case focusStatus:
		return append(global, a.bind(ActFocusRight, "switch to the pull requests panel"))
	}

	return global
}

// helpBarItems is the one-line hint strip under the panels.
func (a *App) helpBarItems() []components.HelpItem {
	km := a.keymap

	switch {
	case a.showHelp:
		return []components.HelpItem{
			{Key: "/", Description: "filter"},
			{Key: "esc", Description: "close"},
		}
	case a.modal.IsVisible():
		return []components.HelpItem{
			{Key: "/", Description: "search"},
			{Key: "enter", Description: "select"},
			{Key: "esc", Description: "cancel"},
		}
	}

	if a.side == sideRight {
		head := make([]components.HelpItem, 0, 3)
		switch a.detailView.Mode() {
		case views.ModePipeline, views.ModeStepLog:
			head = append(head,
				components.HelpItem{Key: km.Keys(ActOpen), Description: "step log"},
				components.HelpItem{Key: km.Keys(ActBack), Description: "back"},
			)
		case views.ModePullRequest, views.ModeRepo, views.ModeSplash:
			head = append(head,
				components.HelpItem{Key: km.Keys(ActBrowser), Description: "browser"},
				components.HelpItem{Key: km.Keys(ActURLPicker), Description: "urls"},
			)
		}
		head = append(head, components.HelpItem{Key: km.Keys(ActFocusLeft), Description: "back"})
		return a.panelHelpItems(config.CtxDetail, head...)
	}

	switch a.leftFocus {
	case focusPullRequests:
		return a.panelHelpItems(config.CtxPullRequests,
			components.HelpItem{Key: km.Keys(ActOpen), Description: "detail"},
			components.HelpItem{Key: km.Keys(ActBrowser), Description: "browser"},
			components.HelpItem{Key: km.Keys(ActCopyURL), Description: "copy url"},
			components.HelpItem{Key: km.Keys(ActCheckout), Description: "checkout"},
			components.HelpItem{Key: "[]", Description: "tabs"},
		)
	case focusBuilds:
		return a.panelHelpItems(config.CtxBuilds,
			components.HelpItem{Key: km.Keys(ActOpen), Description: "pipeline"},
			components.HelpItem{Key: km.Keys(ActBrowser), Description: "browser"},
		)
	case focusRepos:
		return a.panelHelpItems(config.CtxRepos,
			components.HelpItem{Key: km.Keys(ActSelect), Description: "filter"},
			components.HelpItem{Key: km.Keys(ActBrowser), Description: "browser"},
		)
	case focusStatus:
		return []components.HelpItem{
			{Key: km.Keys(ActSwitchPanel) + "/" + km.Keys(ActFocusRight), Description: "detail"},
			{Key: km.Keys(ActHelp), Description: "help"},
		}
	}
	return nil
}

// panelHelpItems appends a panel's custom commands and the help key to its
// fixed hints, so every panel's strip is built the same way.
func (a *App) panelHelpItems(ctx config.Context, head ...components.HelpItem) []components.HelpItem {
	custom := a.customCommandHelpItems(ctx)
	items := make([]components.HelpItem, 0, len(head)+len(custom)+1)
	items = append(items, head...)
	items = append(items, custom...)
	return append(items, components.HelpItem{Key: a.keymap.Keys(ActHelp), Description: "help"})
}
