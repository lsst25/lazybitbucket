package tui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/config"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

func TestApp_SideWidth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		width      int
		configured int
		want       int
	}{
		{"vertical layout returns full width", 60, 40, 60},
		{"default when unset", 200, 0, 40},
		{"explicit width honored when wide", 200, 50, 50},
		{"clamped to half the terminal", 200, 150, 100},
		{"shrinks to 35 percent on narrow terminal", 100, 40, 35},
		{"floors at minimum width", 100, 10, 25},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			app := &App{width: testCase.width, cfg: &config.Config{}}
			app.cfg.GUI.SidePanelWidth = testCase.configured

			testkit.AssertEqual(t, "sideWidth", app.sideWidth(), testCase.want)
		})
	}
}

func TestApp_SideWidth_HalfModeTakesHalfTheWindow(t *testing.T) {
	t.Parallel()
	app := &App{width: 200, cfg: &config.Config{}, screen: screenHalf}
	app.cfg.GUI.SidePanelWidth = 40

	testkit.AssertEqual(t, "half of 200", app.sideWidth(), 100)

	app.width = 60 // the stacked layout has nothing to halve
	testkit.AssertEqual(t, "stacked layout keeps the full width", app.sideWidth(), 60)
}

func TestScreenModeFromConfig(t *testing.T) {
	t.Parallel()
	cases := map[string]screenMode{
		"":                      screenNormal,
		config.ScreenModeNormal: screenNormal,
		config.ScreenModeHalf:   screenHalf,
		config.ScreenModeFull:   screenFull,
		"wide":                  screenNormal,
	}
	for value, want := range cases {
		testkit.AssertEqual(t, "screenModeFromConfig("+value+")", screenModeFromConfig(value), want)
	}
}

func TestCycleScreenMode_WrapsBothWays(t *testing.T) {
	t.Parallel()
	app := &App{}
	forward := []screenMode{screenHalf, screenFull, screenNormal, screenHalf}
	for i, want := range forward {
		app.cycleScreenMode(1)
		testkit.AssertEqual(t, "forward step "+strconv.Itoa(i), app.screen, want)
	}
	backward := []screenMode{screenNormal, screenFull, screenHalf}
	for i, want := range backward {
		app.cycleScreenMode(-1)
		testkit.AssertEqual(t, "backward step "+strconv.Itoa(i), app.screen, want)
	}
}

func TestLayoutPanels_FullModeGivesTheFocusedPanelTheWindow(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	app.helpBar.SetWidth(app.width)
	app.screen = screenFull
	totalH := app.height - 1

	app.side = sideRight
	app.layoutPanels()
	testkit.AssertEqual(t, "detail: no side column", app.panelSideW, 0)
	testkit.AssertEqual(t, "detail: full height", app.panelDetailH, totalH)
	testkit.AssertEqual(t, "detail: log hidden", app.panelLogH, 0)
	for _, x := range []int{0, 3, app.width - 1} {
		panel, _ := app.hitTest(x, totalH-1)
		testkit.AssertEqual(t, "click at x="+strconv.Itoa(x), panel, panelDetail)
	}

	app.side, app.leftFocus = sideLeft, focusBuilds
	app.layoutPanels()
	testkit.AssertEqual(t, "builds: side column is the window", app.panelSideW, app.width)
	testkit.AssertEqual(t, "builds: full height", app.panelBuildsH, totalH)
	testkit.AssertEqual(t, "builds: pull requests hidden", app.panelPRH, 0)
	testkit.AssertEqual(t, "builds: detail hidden", app.panelDetailH, 0)
	panel, _ := app.hitTest(app.width-1, 0)
	testkit.AssertEqual(t, "click lands on builds", panel, panelBuilds)

	for _, line := range strings.Split(plain(app.View()), "\n") {
		if lipgloss.Width(line) > app.width {
			t.Fatalf("a full-screen line overflows the window: %d > %d", lipgloss.Width(line), app.width)
		}
	}
}

func TestLayoutPanels_FullModeFollowsTheFocus(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	app.screen = screenFull
	app.side = sideLeft
	cases := map[focusPanel]panelID{
		focusStatus:       panelStatus,
		focusPullRequests: panelPullRequests,
		focusBuilds:       panelBuilds,
		focusRepos:        panelRepos,
	}
	for focus, want := range cases {
		app.leftFocus = focus
		got, ok := app.fullScreenPanel()
		if !ok || got != want {
			t.Errorf("leftFocus %d: fullScreenPanel = (%d, %v), want (%d, true)", focus, got, ok, want)
		}
	}
	app.screen = screenNormal
	if _, ok := app.fullScreenPanel(); ok {
		t.Error("normal mode has no full-screen panel")
	}
}

func TestScreenModeKeys_CycleTheModes(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})

	_, _ = app.Update(runeKey('+'))
	testkit.AssertEqual(t, "after +", app.screen, screenHalf)
	_, _ = app.Update(runeKey('+'))
	testkit.AssertEqual(t, "after + +", app.screen, screenFull)
	_, _ = app.Update(runeKey('_'))
	testkit.AssertEqual(t, "after _", app.screen, screenHalf)

	// Half mode is visible in the layout at once.
	app.layoutPanels()
	testkit.AssertEqual(t, "half mode side width", app.panelSideW, app.width/2)
}
