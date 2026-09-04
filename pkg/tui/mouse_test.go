package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

func appWithPanelDims(t *testing.T, width int) *App {
	t.Helper()
	app := newAppWithFake(t, &bitbuckettest.FakeClient{T: t})
	app.keymap = DefaultKeymap()
	app.width = width
	app.height = 40
	app.panelSideW = 30
	app.panelStatusH = 2
	app.panelPRH = 5
	app.panelBuildsH = 4
	app.panelReposH = 6
	app.panelDetailH = 20
	app.panelLogH = 5
	return app
}

func TestHitTest_Horizontal(t *testing.T) {
	t.Parallel()
	app := appWithPanelDims(t, 120)

	tests := []struct {
		name     string
		x, y     int
		want     panelID
		wantRelY int
	}{
		{"status", 5, 0, panelStatus, 0},
		{"pull requests", 5, 3, panelPullRequests, 1},
		{"builds", 5, 8, panelBuilds, 1},
		{"repos", 5, 15, panelRepos, 4},
		{"detail", 40, 5, panelDetail, 5},
		{"log", 40, 25, panelLog, 5},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			panel, relY := app.hitTest(testCase.x, testCase.y)
			testkit.AssertEqual(t, "panel", panel, testCase.want)
			testkit.AssertEqual(t, "relY", relY, testCase.wantRelY)
		})
	}
}

func TestHitTest_Vertical(t *testing.T) {
	t.Parallel()
	app := appWithPanelDims(t, 60)

	tests := []struct {
		name     string
		y        int
		want     panelID
		wantRelY int
	}{
		{"status", 0, panelStatus, 0},
		{"pull requests", 3, panelPullRequests, 1},
		{"builds", 8, panelBuilds, 1},
		{"repos", 13, panelRepos, 2},
		{"detail", 20, panelDetail, 3},
		{"log", 38, panelLog, 1},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			panel, relY := app.hitTest(2, testCase.y)
			testkit.AssertEqual(t, "panel", panel, testCase.want)
			testkit.AssertEqual(t, "relY", relY, testCase.wantRelY)
		})
	}
}

func TestHandleMouse_WheelMovesTheListCursor(t *testing.T) {
	t.Parallel()

	t.Run("wheel down advances and takes focus", func(t *testing.T) {
		t.Parallel()
		app := appWithPanelDims(t, 120)
		app.side = sideRight
		app.prList.SetPullRequests([]bitbucket.PullRequest{
			newPR(testRepo, 101, testTitle),
			newPR(testRepo, 102, "second"),
		})

		_, _ = app.handleMouse(tea.MouseMsg{
			Button: tea.MouseButtonWheelDown,
			Action: tea.MouseActionPress,
			X:      5,
			Y:      3,
		})

		testkit.AssertEqual(t, "side", app.side, sideLeft)
		testkit.AssertEqual(t, "leftFocus", app.leftFocus, focusPullRequests)
		testkit.AssertEqual(t, "cursor", app.prList.Cursor, 1)
	})

	t.Run("wheel up rewinds", func(t *testing.T) {
		t.Parallel()
		app := appWithPanelDims(t, 120)
		app.prList.SetPullRequests([]bitbucket.PullRequest{
			newPR(testRepo, 101, testTitle),
			newPR(testRepo, 102, "second"),
		})
		app.prList.Cursor = 1

		_, _ = app.handleMouse(tea.MouseMsg{
			Button: tea.MouseButtonWheelUp,
			Action: tea.MouseActionPress,
			X:      5,
			Y:      3,
		})

		testkit.AssertEqual(t, "cursor", app.prList.Cursor, 0)
	})
}

func TestHandleMouse_DetailScrollSwitchesSide(t *testing.T) {
	t.Parallel()
	app := appWithPanelDims(t, 120)

	_, _ = app.mouseScroll(panelDetail, 3)

	testkit.AssertEqual(t, "side", app.side, sideRight)
}

func TestHandleMouse_MotionIsNoop(t *testing.T) {
	t.Parallel()
	app := appWithPanelDims(t, 120)

	_, cmd := app.handleMouse(tea.MouseMsg{
		Button: tea.MouseButtonNone,
		Action: tea.MouseActionMotion,
		X:      5,
		Y:      3,
	})

	if cmd != nil {
		t.Error("mouse motion should produce no command")
	}
}
