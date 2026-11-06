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

// globalBindings are the keys every panel answers to. The panel keys and the
// two refresh keys are worded per view, because the numbers focus different
// panels there and the refreshes read a different list.
func (a *App) globalBindings() []Binding {
	bindings := []Binding{
		a.bind(ActQuit, "quit"),
		a.bind(ActSwitchPanel, "switch left/right panels"),
		a.bind(ActFocusStatus, "focus status panel"),
	}
	if a.view == viewPipelines {
		bindings = append(bindings,
			a.bind(ActFocusPullRequests, "focus pipelines panel"),
			a.bind(ActFocusRepos, "focus repos panel"),
			a.bind(ActSearch, "search / filter current list"),
			a.bind(ActRefresh, "refresh the selected run"),
			a.bind(ActRefreshAll, "refresh the pipeline list"),
		)
	} else {
		bindings = append(bindings,
			a.bind(ActFocusPullRequests, "focus pull requests panel"),
			a.bind(ActFocusBuilds, "focus builds panel"),
			a.bind(ActFocusRepos, "focus repos panel"),
			a.bind(ActSearch, "search / filter current list"),
			a.bind(ActRefresh, "refresh the selected pull request"),
			a.bind(ActRefreshAll, "refresh the pull request list"),
		)
	}
	return append(bindings,
		a.bind(ActNextScreenMode, "next screen mode: normal → half → full"),
		a.bind(ActPrevScreenMode, "previous screen mode"),
		a.viewSwitchBinding(),
		a.bind(ActHelp, "show all keybindings"),
	)
}

// pipelineBackBinding is where backspace goes from the run or step log on
// screen, and whether it goes anywhere at all: a step log always has its run
// behind it, but a standalone run in the pipelines view was not drilled into
// from anything.
func (a *App) pipelineBackBinding() (Binding, bool) {
	if a.detailView.Mode() == views.ModeStepLog {
		return a.bind(ActBack, "back to the pipeline run"), true
	}
	if a.view == viewPullRequests {
		return a.bind(ActBack, "back to the pull request"), true
	}
	return Binding{Key: "", Description: ""}, false
}

// reviewTabBindings are the keys the active pull request tab answers to. Each
// tab is a different list with a different Enter, so the help has to follow the
// tab and not just the mode.
func (a *App) reviewTabBindings() []Binding {
	switch a.detailView.ActiveTab() {
	case views.TabFiles:
		return append([]Binding{
			a.bind(ActOpen, "open this file's diff"),
			a.bind(ActToggleViewed, "mark this file viewed and go to the next (again: clear the mark)"),
			a.bind(ActNextFile, "next changed file"),
			a.bind(ActPrevFile, "previous changed file"),
			a.bind(ActOpenEditor, "open this file in the editor"),
			a.bind(ActBrowser, "open in browser"),
		}, a.commentBindings("comment on this file")...)
	case views.TabCommits:
		return []Binding{
			a.bind(ActOpen, "expand the commit message"),
			a.bind(ActBrowser, "open the commit in a browser"),
			a.bind(ActCopyURL, "copy the commit URL"),
		}
	case views.TabComments:
		bindings := append([]Binding{
			a.bind(ActOpen, "open the thread"),
			a.bind(ActBrowser, "open the thread in a browser"),
			a.bind(ActCopyURL, "copy the thread URL"),
		}, a.commentBindings("reply to this thread")...)
		bindings = append(bindings, a.resolveBindings()...)
		return append(bindings, a.bind(ActHideResolved, "hide or show resolved conversations"))
	case views.TabOverview:
		return append([]Binding{
			a.bind(ActBrowser, "open in browser"),
			a.bind(ActURLPicker, "open URL picker"),
			a.bind(ActCopyURL, "copy the pull request URL"),
		}, a.commentBindings("comment on the pull request")...)
	}
	return nil
}

// resolveBindings is the resolve key, which is a write and so is absent in
// read-only mode. It reads both ways round because one key does both.
func (a *App) resolveBindings() []Binding {
	if a.readOnly {
		return nil
	}
	return []Binding{a.bind(ActResolveThread, "resolve this conversation (again: reopen it)")}
}

// commentBindings is the composer key, described for where it is pressed. Like
// the other writes it is omitted in read-only mode, and it is listed on the two
// tabs and in the thread section — the places that have something to answer.
func (a *App) commentBindings(description string) []Binding {
	if a.readOnly {
		return nil
	}
	return []Binding{a.bind(ActComment, description)}
}

// writeBindings are the keys that change the pull request on Bitbucket, plus
// the one that opens a new one from the checked-out branch. They are omitted
// entirely in read-only mode: a key that answers "read-only mode" is not a
// keybinding worth listing.
func (a *App) writeBindings() []Binding {
	if a.readOnly {
		return nil
	}
	return []Binding{
		a.bind(ActCreatePullRequest, "open a pull request for the checked-out branch"),
		a.bind(ActMerge, "merge the pull request"),
		a.bind(ActSync, "sync the source branch with the target"),
		a.bind(ActEdit, "edit the title or description"),
		a.bind(ActReviewers, "set the reviewers"),
		a.bind(ActToggleDraft, "toggle draft state"),
		a.bind(ActApprove, "approve the pull request (again: remove your approval)"),
	}
}

// writeHelpItems is the write half of the pull request help strip.
func (a *App) writeHelpItems() []components.HelpItem {
	if a.readOnly {
		return nil
	}
	km := a.keymap
	return []components.HelpItem{
		{Key: km.Keys(ActCreatePullRequest), Description: "new"},
		{Key: km.Keys(ActMerge), Description: "merge"},
		{Key: km.Keys(ActSync), Description: "sync"},
		{Key: km.Keys(ActEdit), Description: "edit"},
		{Key: km.Keys(ActReviewers), Description: "reviewers"},
		{Key: km.Keys(ActToggleDraft), Description: "draft"},
		{Key: km.Keys(ActApprove), Description: "approve"},
	}
}

// pipelineWriteBindings are the two keys that act on the pipeline behind a
// build. They are listed only where they work — the Builds panel and the two
// pipeline views — and, like the pull request writes, not at all in read-only
// mode.
func (a *App) pipelineWriteBindings() []Binding {
	if a.readOnly {
		return nil
	}
	return []Binding{
		a.bind(ActRerunPipeline, "rerun the pipeline"),
		a.bind(ActStopPipeline, "stop the running pipeline"),
	}
}

// pipelineWriteHelpItems is the write half of the builds and pipeline help
// strips.
func (a *App) pipelineWriteHelpItems() []components.HelpItem {
	if a.readOnly {
		return nil
	}
	km := a.keymap
	return []components.HelpItem{
		{Key: km.Keys(ActRerunPipeline), Description: "rerun"},
		{Key: km.Keys(ActStopPipeline), Description: "stop"},
	}
}

// tabJumpBindings are the four keys that go straight to a pull request tab.
func (a *App) tabJumpBindings() []Binding {
	return []Binding{
		a.bind(ActDetailOverview, "overview tab"),
		a.bind(ActDetailFiles, "changed files tab"),
		a.bind(ActDetailCommits, "commits tab"),
		a.bind(ActDetailComments, "comments tab"),
	}
}

// ContextBindings returns the keybindings that apply to the current focus.
func (a *App) ContextBindings() []Binding {
	global := a.globalBindings()

	if a.side == sideRight {
		bindings := slices.Concat(global, a.navBindings())
		switch a.detailView.Mode() {
		case views.ModePipeline, views.ModeStepLog:
			if back, ok := a.pipelineBackBinding(); ok {
				bindings = append(bindings, back)
			}
			bindings = append(bindings,
				a.bind(ActOpen, "open the step log"),
				a.bind(ActBrowser, "open the pipeline in a browser"),
			)
			bindings = append(bindings, a.pipelineWriteBindings()...)
		case views.ModeFileDiff:
			bindings = append(bindings,
				a.bind(ActBack, "back to the changed files"),
				a.bind(ActOpen, "jump to the thread on this line"),
				a.bind(ActToggleViewed, "mark this file viewed and open the next (again: clear the mark)"),
				a.bind(ActNextFile, "next changed file"),
				a.bind(ActPrevFile, "previous changed file"),
				a.bind(ActOpenEditor, "open this line in the editor"),
				a.bind(ActBrowser, "open the diff in a browser"),
				a.bind(ActCopyURL, "copy the diff URL"),
			)
			bindings = append(bindings, a.commentBindings("comment on this line")...)
			bindings = append(bindings, a.tabJumpBindings()...)
		case views.ModeThread:
			bindings = append(bindings,
				a.bind(ActBack, "back to the comments"),
				a.bind(ActBrowser, "open the comment in a browser"),
				a.bind(ActCopyURL, "copy the comment URL"),
			)
			bindings = append(bindings, a.commentBindings("reply to this thread")...)
			bindings = append(bindings, a.resolveBindings()...)
			bindings = append(bindings, a.tabJumpBindings()...)
		case views.ModePullRequest:
			bindings = append(bindings, a.reviewTabBindings()...)
			bindings = append(bindings, a.tabJumpBindings()...)
			bindings = append(bindings, a.writeBindings()...)
		case views.ModeRepo, views.ModeSplash:
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
		bindings = append(bindings, a.writeBindings()...)
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
		bindings = append(bindings, a.pipelineWriteBindings()...)
		bindings = append(bindings, a.customCommandBindings(config.CtxBuilds)...)
		return bindings

	case focusPipelines:
		bindings := slices.Concat(global, a.navBindings(), a.detailScrollBindings())
		bindings = append(bindings,
			a.bind(ActOpen, "open the run"),
			a.bind(ActOpenPullRequest, "go to the pull request this run ran for"),
			a.bind(ActBrowser, "open the run in a browser"),
			a.bind(ActCopyURL, "copy the run URL"),
			a.bind(ActFocusRight, "next panel"),
			a.bind(ActFocusLeft, "previous panel"),
		)
		// Custom commands are deliberately absent: they are written against a
		// pull request, and this view has none. See activeContexts.
		return append(bindings, a.pipelineWriteBindings()...)

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
	case a.form.IsVisible() && a.form.IsEditing():
		return []components.HelpItem{
			{Key: "enter", Description: "keep"},
			{Key: "esc", Description: "revert"},
		}
	case a.form.IsVisible():
		return []components.HelpItem{
			{Key: "j/k", Description: "move"},
			{Key: "h/l", Description: "change"},
			{Key: "space", Description: "toggle"},
			{Key: "enter", Description: "confirm"},
			{Key: "esc", Description: "cancel"},
		}
	case a.input.IsVisible():
		// The comment popup answers one key the title editor does not.
		if a.pendingComment != nil {
			return commentHelpBarItems()
		}
		return []components.HelpItem{
			{Key: "enter", Description: "confirm"},
			{Key: "esc", Description: "cancel"},
		}
	// The checklist answers to a different set than the picker: space toggles
	// and enter saves the whole selection. `h` and `q` also cancel — Modal
	// hard-codes both — but the strip names the one key people look for.
	case a.modal.IsVisible() && a.modal.IsChecklist():
		return []components.HelpItem{
			{Key: "space", Description: "toggle"},
			{Key: "enter", Description: "save"},
			{Key: "/", Description: "filter"},
			{Key: "esc", Description: "cancel"},
		}
	case a.modal.IsVisible():
		return []components.HelpItem{
			{Key: "/", Description: "search"},
			{Key: "enter", Description: "select"},
			{Key: "esc", Description: "cancel"},
		}
	}

	if a.side == sideRight {
		head := make([]components.HelpItem, 0, 6)
		switch a.detailView.Mode() {
		case views.ModePipeline, views.ModeStepLog:
			head = append(head, components.HelpItem{Key: km.Keys(ActOpen), Description: "step log"})
			if _, ok := a.pipelineBackBinding(); ok {
				head = append(head, components.HelpItem{Key: km.Keys(ActBack), Description: "back"})
			}
			head = append(head, a.pipelineWriteHelpItems()...)
		case views.ModeFileDiff:
			head = append(head,
				components.HelpItem{Key: km.Keys(ActToggleViewed), Description: "viewed"},
				components.HelpItem{Key: km.Keys(ActNextFile) + "/" + km.Keys(ActPrevFile), Description: "file"},
				components.HelpItem{Key: km.Keys(ActOpenEditor), Description: "editor"},
			)
			head = append(head, a.commentHelpItems("comment")...)
			head = append(head, components.HelpItem{Key: km.Keys(ActBack), Description: "back"})
		case views.ModeThread:
			head = append(head, a.commentHelpItems("reply")...)
			head = append(head, a.resolveHelpItems()...)
			head = append(head,
				components.HelpItem{Key: km.Keys(ActBrowser), Description: "browser"},
				components.HelpItem{Key: km.Keys(ActBack), Description: "back"},
			)
		case views.ModePullRequest:
			head = append(head, a.reviewTabHelpItems()...)
		case views.ModeRepo, views.ModeSplash:
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
		writes := a.writeHelpItems()
		head := make([]components.HelpItem, 0, 5+len(writes))
		head = append(head,
			components.HelpItem{Key: km.Keys(ActOpen), Description: "detail"},
			components.HelpItem{Key: km.Keys(ActBrowser), Description: "browser"},
			components.HelpItem{Key: km.Keys(ActCopyURL), Description: "copy url"},
			components.HelpItem{Key: km.Keys(ActCheckout), Description: "checkout"},
			components.HelpItem{Key: "[]", Description: "tabs"},
		)
		head = append(head, writes...)
		return a.panelHelpItems(config.CtxPullRequests, head...)
	case focusBuilds:
		writes := a.pipelineWriteHelpItems()
		head := make([]components.HelpItem, 0, 2+len(writes))
		head = append(head,
			components.HelpItem{Key: km.Keys(ActOpen), Description: "pipeline"},
			components.HelpItem{Key: km.Keys(ActBrowser), Description: "browser"},
		)
		head = append(head, writes...)
		return a.panelHelpItems(config.CtxBuilds, head...)
	case focusPipelines:
		writes := a.pipelineWriteHelpItems()
		head := make([]components.HelpItem, 0, 4+len(writes))
		head = append(head,
			components.HelpItem{Key: km.Keys(ActOpen), Description: "run"},
			components.HelpItem{Key: km.Keys(ActOpenPullRequest), Description: "pull request"},
			components.HelpItem{Key: km.Keys(ActBrowser), Description: "browser"},
		)
		head = append(head, writes...)
		// No custom command items: none fire in this view.
		return a.closeHelpItems(head)
	case focusRepos:
		return a.panelHelpItems(config.CtxRepos,
			components.HelpItem{Key: km.Keys(ActSelect), Description: "filter"},
			components.HelpItem{Key: km.Keys(ActBrowser), Description: "browser"},
		)
	case focusStatus:
		return a.closeHelpItems([]components.HelpItem{
			{Key: km.Keys(ActSwitchPanel) + "/" + km.Keys(ActFocusRight), Description: "detail"},
		})
	}
	return nil
}

// reviewTabHelpItems is the hint strip of the active pull request tab.
func (a *App) reviewTabHelpItems() []components.HelpItem {
	km := a.keymap
	switch a.detailView.ActiveTab() {
	case views.TabFiles:
		return append([]components.HelpItem{
			{Key: km.Keys(ActOpen), Description: "diff"},
			{Key: km.Keys(ActToggleViewed), Description: "viewed"},
			{Key: km.Keys(ActNextFile) + "/" + km.Keys(ActPrevFile), Description: "file"},
			{Key: km.Keys(ActOpenEditor), Description: "editor"},
		}, a.commentHelpItems("comment")...)
	case views.TabCommits:
		return []components.HelpItem{
			{Key: km.Keys(ActOpen), Description: "expand"},
			{Key: km.Keys(ActBrowser), Description: "browser"},
		}
	case views.TabComments:
		items := append([]components.HelpItem{
			{Key: km.Keys(ActOpen), Description: "open"},
			{Key: km.Keys(ActBrowser), Description: "browser"},
		}, a.commentHelpItems("reply")...)
		items = append(items, a.resolveHelpItems()...)
		return append(items, components.HelpItem{
			Key: km.Keys(ActHideResolved), Description: "resolved",
		})
	case views.TabOverview:
		return append([]components.HelpItem{
			{Key: km.Keys(ActBrowser), Description: "browser"},
			{Key: km.Keys(ActURLPicker), Description: "urls"},
		}, a.commentHelpItems("comment")...)
	}
	return nil
}

// resolveHelpItems is the resolve key on a help strip, absent in read-only
// mode like every other write.
func (a *App) resolveHelpItems() []components.HelpItem {
	if a.readOnly {
		return nil
	}
	return []components.HelpItem{{Key: a.keymap.Keys(ActResolveThread), Description: "resolve"}}
}

// commentHelpItems is the composer key on a help strip, absent in read-only
// mode like every other write.
func (a *App) commentHelpItems(description string) []components.HelpItem {
	if a.readOnly {
		return nil
	}
	return []components.HelpItem{{Key: a.keymap.Keys(ActComment), Description: description}}
}

// panelHelpItems appends a panel's custom commands and the help key to its
// fixed hints, so every panel's strip is built the same way.
func (a *App) panelHelpItems(ctx config.Context, head ...components.HelpItem) []components.HelpItem {
	custom := a.customCommandHelpItems(ctx)
	items := make([]components.HelpItem, 0, len(head)+len(custom)+2)
	items = append(items, head...)
	items = append(items, custom...)
	return a.closeHelpItems(items)
}

// closeHelpItems ends every strip the same way: the view switch, named after
// the view it goes to, and then the help key.
func (a *App) closeHelpItems(items []components.HelpItem) []components.HelpItem {
	return append(items,
		a.viewSwitchHelpItem(),
		components.HelpItem{Key: a.keymap.Keys(ActHelp), Description: "help"},
	)
}
