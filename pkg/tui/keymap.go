package tui

import (
	"slices"

	"github.com/lsst25/lazybitbucket/pkg/config"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
)

// Action represents a user-triggerable action.
type Action string

// Actions, each remappable to different keys via config.
const (
	ActQuit        Action = "quit"
	ActHelp        Action = "help"
	ActSearch      Action = "search"
	ActSwitchPanel Action = "switchPanel"
	ActRefresh     Action = "refresh"
	ActRefreshAll  Action = "refreshAll"
	ActPrevTab     Action = "prevTab"
	ActNextTab     Action = "nextTab"
	// The screen modes: normal → half → full and back, as in lazygit.
	ActNextScreenMode Action = "nextScreenMode"
	ActPrevScreenMode Action = "prevScreenMode"
	// ActNextView switches between the pull request view and the pipelines
	// view, which share the detail panel and nothing else.
	ActNextView Action = "nextView"

	ActFocusDetail       Action = "focusDetail"
	ActFocusStatus       Action = "focusStatus"
	ActFocusPullRequests Action = "focusPullRequests"
	ActFocusBuilds       Action = "focusBuilds"
	ActFocusRepos        Action = "focusRepos"
	ActFocusLeft         Action = "focusLeft"
	ActFocusRight        Action = "focusRight"

	// ActSelect marks and opens; ActOpen opens without marking. The Builds and
	// Repos panels reuse both — the handler decides by focus.
	ActSelect Action = "select"
	ActOpen   Action = "open"

	ActBrowser   Action = "browser"
	ActURLPicker Action = "urlPicker"
	ActCopyURL   Action = "copyURL"
	ActCheckout  Action = "checkout"
	ActBack      Action = "back"

	ActNavDown     Action = "navDown"
	ActNavUp       Action = "navUp"
	ActNavTop      Action = "navTop"
	ActNavBottom   Action = "navBottom"
	ActNavHalfDown Action = "navHalfPageDown"
	ActNavHalfUp   Action = "navHalfPageUp"

	ActDetailScrollDown Action = "detailScrollDown"
	ActDetailScrollUp   Action = "detailScrollUp"
	ActDetailHalfDown   Action = "detailHalfPageDown"
	ActDetailHalfUp     Action = "detailHalfPageUp"

	// The review actions: the four tab jumps, stepping through the changed
	// files and opening the file under the cursor in an editor. They only act
	// while the detail panel is focused.
	ActDetailOverview Action = "detailOverview"
	ActDetailFiles    Action = "detailFiles"
	ActDetailCommits  Action = "detailCommits"
	ActDetailComments Action = "detailComments"
	ActNextFile       Action = "nextFile"
	ActPrevFile       Action = "prevFile"
	ActOpenEditor     Action = "openEditor"

	// The six write actions. Each opens a dialog first — nothing here reaches
	// Bitbucket without a confirmation — and all six are refused, and hidden
	// from the help surfaces, in read-only mode.
	ActMerge       Action = "merge"
	ActSync        Action = "sync"
	ActEdit        Action = "edit"
	ActReviewers   Action = "reviewers"
	ActToggleDraft Action = "toggleDraft"
	ActApprove     Action = "approve"

	// ActCreatePullRequest opens a pull request for the checked-out branch. It
	// is the one write that does not start from a pull request, so it is live
	// wherever the pull request view is.
	ActCreatePullRequest Action = "createPullRequest"

	// ActResolveThread resolves the conversation in hand, or reopens it when it
	// is already resolved — one key in two directions, like ActApprove.
	ActResolveThread Action = "resolveThread"

	// ActHideResolved is not a write: it leaves the resolved conversations off
	// the Comments tab, and puts them back.
	ActHideResolved Action = "hideResolved"

	// ActComment is the seventh write and the only one whose dialog is the
	// title editor's input popup rather than a form. It is read-only-refused
	// and hidden like the rest.
	ActComment Action = "comment"

	// The two pipeline writes. They act on the run behind a build rather than
	// on the pull request, so they are live in the Builds panel and in the
	// detail panel's pipeline and step-log views and nowhere else — but they
	// open a dialog and are hidden in read-only mode like the six above.
	ActRerunPipeline Action = "rerunPipeline"
	ActStopPipeline  Action = "stopPipeline"

	// ActOpenPullRequest is the pipelines list's own key: it leaves for the
	// pull request the selected run ran for. No other panel has it, because no
	// other panel shows a run that belongs to a pull request elsewhere.
	ActOpenPullRequest Action = "openPullRequest"
)

// defaultNextViewKey is the view switch's default binding, which the help strip
// draws as a glyph; see viewSwitchLabel.
const defaultNextViewKey = "shift+tab"

// Keymap maps actions to key strings. Several keys may trigger one action.
type Keymap map[Action][]string

// release takes key away from every action but keeper, dropping an action that
// is left with no key at all. It is what makes a configured binding win over
// the default that held the same key.
func (k Keymap) release(key string, keeper Action) {
	for action, keys := range k {
		if action == keeper {
			continue
		}
		kept := slices.DeleteFunc(slices.Clone(keys), func(k string) bool { return k == key })
		if len(kept) == len(keys) {
			continue
		}
		if len(kept) == 0 {
			delete(k, action)
			continue
		}
		k[action] = kept
	}
}

// DefaultKeymap returns the default key bindings.
func DefaultKeymap() Keymap {
	return Keymap{
		ActQuit:        {"q", "ctrl+c"},
		ActHelp:        {"?"},
		ActSearch:      {"/"},
		ActSwitchPanel: {"tab"},
		ActRefresh:     {"r"},
		ActRefreshAll:  {"R"},
		ActPrevTab:     {"["},
		ActNextTab:     {"]"},

		ActNextScreenMode: {"+"},
		ActPrevScreenMode: {"_"},
		ActNextView:       {defaultNextViewKey},

		ActFocusDetail:       {"0"},
		ActFocusStatus:       {"1"},
		ActFocusPullRequests: {"2"},
		ActFocusBuilds:       {"3"},
		ActFocusRepos:        {"4"},
		ActFocusLeft:         {"h", "left", "esc"},
		ActFocusRight:        {"l", "right"},

		ActSelect: {" "},
		ActOpen:   {"enter"},

		ActBrowser:   {"o"},
		ActURLPicker: {"u"},
		ActCopyURL:   {"y"},
		ActCheckout:  {"c"},
		ActBack:      {"backspace"},

		ActNavDown:     {"j", "down", "ctrl+j"},
		ActNavUp:       {"k", "up", "ctrl+k"},
		ActNavTop:      {"g", "home"},
		ActNavBottom:   {"G", "end"},
		ActNavHalfDown: {"ctrl+d"},
		ActNavHalfUp:   {"ctrl+u"},

		ActDetailScrollDown: {"J"},
		ActDetailScrollUp:   {"K"},
		ActDetailHalfDown:   {"ctrl+f"},
		ActDetailHalfUp:     {"ctrl+b"},

		ActDetailOverview: {"i"},
		ActDetailFiles:    {"d"},
		ActDetailCommits:  {"m"},
		ActDetailComments: {"t"},
		ActNextFile:       {"n"},
		ActPrevFile:       {"p"},
		ActOpenEditor:     {"e"},

		// Uppercase on purpose: the lowercase review keys d, m and e stay where
		// they are, and a write is worth a shifted key.
		ActMerge:             {"M"},
		ActSync:              {"S"},
		ActEdit:              {"E"},
		ActReviewers:         {"V"},
		ActToggleDraft:       {"D"},
		ActApprove:           {"A"},
		ActComment:           {"C"},
		ActCreatePullRequest: {"N"},
		ActResolveThread:     {"T"},
		ActHideResolved:      {"f"},

		ActRerunPipeline: {"P"},
		ActStopPipeline:  {"X"},

		ActOpenPullRequest: {"L"},
	}
}

// KeymapFromConfig starts from the defaults and applies every non-empty
// override. There is one set line per remappable action and only for fields
// config actually has, so no binding is silently unreachable and no config key
// is silently dead.
func KeymapFromConfig(kcfg config.KeybindingConfig) Keymap {
	km := DefaultKeymap()
	set := func(action Action, val string) {
		if val == "" {
			return
		}
		// A key belongs to one action. Binding it here takes it from whatever
		// default held it, or Match — which walks a map — would answer with
		// either of them depending on the iteration order, and a remap onto a
		// key that is already spoken for is exactly what a user does when they
		// want the other action out of the way.
		km.release(val, action)
		km[action] = []string{val}
	}

	set(ActQuit, kcfg.Universal.Quit)
	set(ActHelp, kcfg.Universal.Help)
	set(ActSearch, kcfg.Universal.Search)
	set(ActSwitchPanel, kcfg.Universal.SwitchPanel)
	set(ActRefresh, kcfg.Universal.Refresh)
	set(ActRefreshAll, kcfg.Universal.RefreshAll)
	set(ActPrevTab, kcfg.Universal.PrevTab)
	set(ActNextTab, kcfg.Universal.NextTab)
	set(ActNextScreenMode, kcfg.Universal.NextScreenMode)
	set(ActPrevScreenMode, kcfg.Universal.PrevScreenMode)
	set(ActNextView, kcfg.Universal.NextView)
	set(ActFocusDetail, kcfg.Universal.FocusDetail)
	set(ActFocusStatus, kcfg.Universal.FocusStatus)
	set(ActFocusPullRequests, kcfg.Universal.FocusPullRequests)
	set(ActFocusBuilds, kcfg.Universal.FocusBuilds)
	set(ActFocusRepos, kcfg.Universal.FocusRepos)

	// The Repos and Builds panels reuse these three.
	set(ActSelect, kcfg.PullRequests.Select)
	set(ActOpen, kcfg.PullRequests.Open)
	set(ActFocusRight, kcfg.PullRequests.FocusRight)
	set(ActBrowser, kcfg.PullRequests.Browser)
	set(ActURLPicker, kcfg.PullRequests.URLPicker)
	set(ActCopyURL, kcfg.PullRequests.CopyURL)
	set(ActCheckout, kcfg.PullRequests.Checkout)
	set(ActMerge, kcfg.PullRequests.Merge)
	set(ActSync, kcfg.PullRequests.Sync)
	set(ActEdit, kcfg.PullRequests.Edit)
	set(ActReviewers, kcfg.PullRequests.Reviewers)
	set(ActToggleDraft, kcfg.PullRequests.ToggleDraft)
	set(ActApprove, kcfg.PullRequests.Approve)
	set(ActComment, kcfg.PullRequests.Comment)
	set(ActCreatePullRequest, kcfg.PullRequests.Create)
	set(ActResolveThread, kcfg.PullRequests.Resolve)

	// The Builds group carries the two pipeline writes and nothing else: every
	// other key that works on a build row is the pull request group's.
	set(ActRerunPipeline, kcfg.Builds.Rerun)
	set(ActStopPipeline, kcfg.Builds.Stop)

	// The Pipelines group carries the one key the pipelines list has and no
	// other panel does; rerun and stop stay above, in the Builds group.
	set(ActOpenPullRequest, kcfg.Pipelines.OpenPullRequest)

	set(ActFocusLeft, kcfg.Detail.FocusLeft)
	set(ActBack, kcfg.Detail.Back)
	set(ActDetailScrollDown, kcfg.Detail.ScrollDown)
	set(ActDetailScrollUp, kcfg.Detail.ScrollUp)
	set(ActDetailHalfDown, kcfg.Detail.HalfPageDown)
	set(ActDetailHalfUp, kcfg.Detail.HalfPageUp)
	set(ActHideResolved, kcfg.Detail.HideResolved)
	set(ActDetailOverview, kcfg.Detail.OverviewTab)
	set(ActDetailFiles, kcfg.Detail.FilesTab)
	set(ActDetailCommits, kcfg.Detail.CommitsTab)
	set(ActDetailComments, kcfg.Detail.CommentsTab)
	set(ActNextFile, kcfg.Detail.NextFile)
	set(ActPrevFile, kcfg.Detail.PrevFile)
	set(ActOpenEditor, kcfg.Detail.OpenEditor)

	set(ActNavDown, kcfg.Navigation.Down)
	set(ActNavUp, kcfg.Navigation.Up)
	set(ActNavTop, kcfg.Navigation.Top)
	set(ActNavBottom, kcfg.Navigation.Bottom)
	set(ActNavHalfDown, kcfg.Navigation.HalfDown)
	set(ActNavHalfUp, kcfg.Navigation.HalfUp)

	return km
}

// Match returns the action bound to a key, or "" when it is unbound.
func (km Keymap) Match(key string) Action {
	for action, keys := range km {
		if slices.Contains(keys, key) {
			return action
		}
	}
	return ""
}

var navActionMap = map[Action]components.NavAction{
	ActNavDown:     components.NavDown,
	ActNavUp:       components.NavUp,
	ActNavTop:      components.NavTop,
	ActNavBottom:   components.NavBottom,
	ActNavHalfDown: components.NavHalfDown,
	ActNavHalfUp:   components.NavHalfUp,
}

// MatchNav resolves a key to a list navigation action.
func (km Keymap) MatchNav(key string) components.NavAction {
	for action, nav := range navActionMap {
		if slices.Contains(km[action], key) {
			return nav
		}
	}
	return components.NavNone
}

// Keys returns the first key bound to an action, for display in help.
func (km Keymap) Keys(action Action) string {
	if keys, ok := km[action]; ok && len(keys) > 0 {
		k := keys[0]
		if k == " " {
			return "space"
		}
		return k
	}
	return ""
}
