package tui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

func appForView(t *testing.T) *App {
	t.Helper()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	app.updateFocusState()
	return app
}

func TestView_LoadingBeforeWidthSet(t *testing.T) {
	t.Parallel()
	app := newAppWithFake(t, &bitbuckettest.FakeClient{T: t})

	output := app.View()

	if output != "Loading..." {
		t.Errorf("View() = %q, want Loading...", output)
	}
}

func TestView_HorizontalLayoutShowsPullRequest(t *testing.T) {
	t.Parallel()
	app := appForView(t)
	app.prList.SetPullRequests([]bitbucket.PullRequest{newPR(testRepo, 101, testTitle)})

	stripped := plain(app.View())

	if !strings.Contains(stripped, "#101") {
		t.Errorf("view should list #101:\n%s", stripped)
	}
	if !strings.Contains(stripped, "api-gate") {
		t.Errorf("view should name the repository:\n%s", stripped)
	}
}

func TestView_VerticalLayout(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	app.width = 60
	app.layoutPanels()
	app.updateFocusState()
	app.prList.SetPullRequests([]bitbucket.PullRequest{newPR(testRepo, 101, testTitle)})

	if !app.isVerticalLayout() {
		t.Fatal("60 columns should be a vertical layout")
	}
	if plain(app.View()) == "" {
		t.Error("vertical View() returned nothing")
	}
}

func TestView_RateLimitWarningFromTheAtomic(t *testing.T) {
	t.Parallel()
	app := appForView(t)
	app.rateLimit.Store(150 + 1)

	stripped := plain(app.View())

	if !strings.Contains(stripped, "rate limit") {
		t.Errorf("a low rate limit should reach the status panel:\n%s", stripped)
	}
}

func TestView_NoRateLimitLineWhenUnknown(t *testing.T) {
	t.Parallel()
	app := appForView(t)

	stripped := plain(app.View())

	if strings.Contains(stripped, "rate limit") {
		t.Errorf("an unknown rate limit must render nothing:\n%s", stripped)
	}
}

func TestView_ShowHelpOverlay(t *testing.T) {
	t.Parallel()
	app := appForView(t)
	app.showHelp = true

	stripped := plain(app.View())

	if !strings.Contains(stripped, "Keybindings") {
		t.Errorf("help overlay missing:\n%s", stripped)
	}
}

func TestView_ShowHelpOverlayWithFilter(t *testing.T) {
	t.Parallel()
	app := appForView(t)
	app.showHelp = true
	app.helpFilter = string(ActQuit)

	if plain(app.View()) == "" {
		t.Error("filtered help overlay rendered nothing")
	}
}

// TestView_ShowHelpOverlayInATinyTerminal pins the clamps around the help
// popup: a terminal shorter than the popup's own chrome used to compute a
// negative inner height and panic the renderer out of View().
func TestView_ShowHelpOverlayInATinyTerminal(t *testing.T) {
	t.Parallel()

	sizes := []struct {
		width, height int
	}{
		{120, 1}, {120, 2}, {120, 3}, {120, 4}, {120, 5},
		{10, 3}, {6, 1}, {1, 1},
	}

	for _, size := range sizes {
		t.Run(strconv.Itoa(size.width)+"x"+strconv.Itoa(size.height), func(t *testing.T) {
			t.Parallel()
			app := appForView(t)
			app.showHelp = true
			app.width, app.height = size.width, size.height
			app.layoutPanels()

			// The assertion is that View does not panic; the output of a
			// terminal this small is not worth pinning.
			_ = app.View()
		})
	}
}

func TestView_NoPanicInAnyFocusState(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		setup func(*App)
	}{
		{"pull requests", func(app *App) { app.side, app.leftFocus = sideLeft, focusPullRequests }},
		{"builds", func(app *App) { app.side, app.leftFocus = sideLeft, focusBuilds }},
		{"repos", func(app *App) { app.side, app.leftFocus = sideLeft, focusRepos }},
		{"status", func(app *App) { app.side, app.leftFocus = sideLeft, focusStatus }},
		{"detail", func(app *App) { app.side = sideRight }},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			app := appForView(t)
			testCase.setup(app)
			app.updateFocusState()

			if app.View() == "" {
				t.Error("View() returned an empty string")
			}
		})
	}
}

func TestView_FullScreenModeShowsOnlyTheFocusedPanel(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	app.helpBar.SetWidth(app.width) // as the window-size handler does; JoinVertical pads to the widest line
	app.screen = screenFull

	// The pull request panel's title is its tab bar, "[2] Open - Mine - …".
	app.side = sideRight
	out := plain(app.View())
	if strings.Contains(out, "[2] Open") {
		t.Error("with the detail panel full screen the pull request panel must not be drawn")
	}

	app.side, app.leftFocus = sideLeft, focusPullRequests
	out = plain(app.View())
	if !strings.Contains(out, "[2] Open") {
		t.Error("the focused pull request panel must be drawn")
	}
	first, _, _ := strings.Cut(out, "\n")
	testkit.AssertEqual(t, "the one panel spans the window", lipgloss.Width(first), app.width)
	if strings.Contains(out, "[0]") {
		t.Error("the detail panel must not be drawn next to a full-screen left panel")
	}
}
