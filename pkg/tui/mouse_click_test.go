package tui

import (
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

func mouseApp(t *testing.T) *App {
	t.Helper()
	return appWithPanelDims(t, 120)
}

func TestMouseClick_StatusFocusesStatusAndShowsSplash(t *testing.T) {
	t.Parallel()
	app := mouseApp(t)
	app.side = sideRight

	_, _ = app.mouseClick(panelStatus, 0, 5)

	testkit.AssertEqual(t, "side", app.side, sideLeft)
	testkit.AssertEqual(t, "leftFocus", app.leftFocus, focusStatus)
	testkit.AssertEqual(t, "detail mode", app.detailView.Mode(), views.ModeSplash)
}

func TestMouseClick_PullRequestRowPreviews(t *testing.T) {
	t.Parallel()
	app := mouseApp(t)
	app.side = sideRight
	app.prList.SetPullRequests([]bitbucket.PullRequest{newPR(testRepo, 101, testTitle)})

	_, cmd := app.mouseClick(panelPullRequests, 1, 5)

	testkit.AssertEqual(t, "side", app.side, sideLeft)
	testkit.AssertEqual(t, "leftFocus", app.leftFocus, focusPullRequests)
	testkit.AssertEqual(t, "previewKey", app.previewKey, testPRKey)
	if cmd == nil {
		t.Error("clicking a row should schedule the preview debounce")
	}
}

func TestMouseClick_PullRequestTitleBarSwitchesTab(t *testing.T) {
	t.Parallel()
	app := mouseApp(t)

	_, _ = app.mouseClick(panelPullRequests, 0, 15)

	testkit.AssertEqual(t, "leftFocus", app.leftFocus, focusPullRequests)
	if app.prList.GetTabIndex() == 0 {
		t.Error("clicking the second tab zone should change the tab")
	}
}

func TestMouseClick_BuildsFocusesBuilds(t *testing.T) {
	t.Parallel()
	app := mouseApp(t)
	app.side = sideRight

	_, _ = app.mouseClick(panelBuilds, 1, 5)

	testkit.AssertEqual(t, "side", app.side, sideLeft)
	testkit.AssertEqual(t, "leftFocus", app.leftFocus, focusBuilds)
}

func TestMouseClick_ReposHoverPreviewsRepository(t *testing.T) {
	t.Parallel()
	app := mouseApp(t)
	app.repoList.SetRepos([]bitbucket.Repository{{Slug: testRepo, Name: testRepo}})

	_, _ = app.mouseClick(panelRepos, 1, 5)

	testkit.AssertEqual(t, "leftFocus", app.leftFocus, focusRepos)
	testkit.AssertEqual(t, "detail mode", app.detailView.Mode(), views.ModeRepo)
}

func TestMouseClick_ReposDoubleClickFiltersTheList(t *testing.T) {
	t.Parallel()
	app := mouseApp(t)
	app.repoList.SetRepos([]bitbucket.Repository{{Slug: testRepo, Name: testRepo}})

	// Row 1 is the first real repository; the second click on the same row is
	// the double-click that activates it.
	_, _ = app.mouseClick(panelRepos, 2, 5)
	_, _ = app.mouseClick(panelRepos, 2, 5)

	testkit.AssertEqual(t, "repo filter", app.repoFilter, testRepo)
	testkit.AssertEqual(t, "leftFocus", app.leftFocus, focusPullRequests)
}

func TestMouseClick_DetailFocusesDetail(t *testing.T) {
	t.Parallel()
	app := mouseApp(t)
	app.side = sideLeft

	_, _ = app.mouseClick(panelDetail, 1, 40)

	testkit.AssertEqual(t, "side", app.side, sideRight)
}
