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
)

// Keymap maps actions to key strings. Several keys may trigger one action.
type Keymap map[Action][]string

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
	}
}

// KeymapFromConfig starts from the defaults and applies every non-empty
// override. There is one set line per remappable action and only for fields
// config actually has, so no binding is silently unreachable and no config key
// is silently dead.
func KeymapFromConfig(kcfg config.KeybindingConfig) Keymap {
	km := DefaultKeymap()
	set := func(action Action, val string) {
		if val != "" {
			km[action] = []string{val}
		}
	}

	set(ActQuit, kcfg.Universal.Quit)
	set(ActHelp, kcfg.Universal.Help)
	set(ActSearch, kcfg.Universal.Search)
	set(ActSwitchPanel, kcfg.Universal.SwitchPanel)
	set(ActRefresh, kcfg.Universal.Refresh)
	set(ActRefreshAll, kcfg.Universal.RefreshAll)
	set(ActPrevTab, kcfg.Universal.PrevTab)
	set(ActNextTab, kcfg.Universal.NextTab)
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

	set(ActFocusLeft, kcfg.Detail.FocusLeft)
	set(ActBack, kcfg.Detail.Back)
	set(ActDetailScrollDown, kcfg.Detail.ScrollDown)
	set(ActDetailScrollUp, kcfg.Detail.ScrollUp)
	set(ActDetailHalfDown, kcfg.Detail.HalfPageDown)
	set(ActDetailHalfUp, kcfg.Detail.HalfPageUp)
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
