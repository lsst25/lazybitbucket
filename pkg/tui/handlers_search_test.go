package tui

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
)

func searchApp(t *testing.T) *App {
	t.Helper()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	app.prList.SetPullRequests([]bitbucket.PullRequest{
		newPR(testRepo, 101, "alpha rate limiter"),
		newPR(testRepo, 102, "beta oauth"),
	})
	return app
}

func TestHandleSearchChanged_FiltersThePullRequestList(t *testing.T) {
	t.Parallel()
	app := searchApp(t)

	_, _ = app.handleSearchChanged(components.SearchChangedMsg{Query: "beta"})

	sel := app.prList.SelectedPullRequest()
	if sel == nil || sel.ID != 102 {
		t.Fatalf("filtered selection = %v, want #102", sel)
	}
}

func TestHandleSearchChanged_FiltersTheReposPanel(t *testing.T) {
	t.Parallel()
	app := searchApp(t)
	app.leftFocus = focusRepos
	app.repoList.SetRepos([]bitbucket.Repository{
		{Slug: testRepo, Name: testRepo},
		{Slug: testRepo2, Name: testRepo2},
	})

	_, _ = app.handleSearchChanged(components.SearchChangedMsg{Query: testRepo2})

	repo := app.repoList.SelectedRepo()
	if repo == nil || repo.Slug != testRepo2 {
		t.Fatalf("filtered repo = %v, want %s", repo, testRepo2)
	}
}

func TestHandleSearchConfirmed_KeepsTheCursorWhereSearchLanded(t *testing.T) {
	t.Parallel()
	app := searchApp(t)

	app.prList.SetFilter("beta")
	_, cmd := app.handleSearchConfirmed()

	sel := app.prList.SelectedPullRequest()
	if sel == nil || sel.ID != 102 {
		t.Fatalf("cursor landed on %v, want #102", sel)
	}
	testkit.AssertEqual(t, "previewKey", app.previewKey, testPRKey2)
	if cmd == nil {
		t.Error("confirming a search should re-preview the landed pull request")
	}
}

func TestHandleSearchConfirmed_ReposSelectsTheRepository(t *testing.T) {
	t.Parallel()
	app := searchApp(t)
	app.leftFocus = focusRepos
	app.repoList.SetRepos([]bitbucket.Repository{{Slug: testRepo, Name: testRepo}})
	app.repoList.Cursor = 1

	_, _ = app.handleSearchConfirmed()

	testkit.AssertEqual(t, "repo filter", app.repoFilter, testRepo)
	testkit.AssertEqual(t, "leftFocus", app.leftFocus, focusPullRequests)
}

func TestHandleSearchCancelled_ClearsEveryFilter(t *testing.T) {
	t.Parallel()
	app := searchApp(t)
	app.prList.SetFilter("alpha")
	app.repoList.SetFilter("alpha")

	_, cmd := app.handleSearchCancelled()

	if cmd != nil {
		t.Error("cancelling a search should produce no command")
	}
	if app.prList.ItemCount() != 2 {
		t.Errorf("item count = %d, want both rows back", app.prList.ItemCount())
	}
}

func TestHandleAutoFetch_DisabledWhenRefreshIsOff(t *testing.T) {
	t.Parallel()
	app := searchApp(t)

	_, cmd := app.handleAutoFetch()

	if cmd != nil {
		t.Error("auto-refresh is off: the tick must not re-arm")
	}
}

func TestHandleAutoFetch_RearmsWhenEnabled(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.ListPullRequestsFunc = func(_ context.Context, _, repo string,
		_ bitbucket.ListPullRequestsOptions,
	) ([]bitbucket.PullRequest, error) {
		return []bitbucket.PullRequest{newPR(repo, 101, testTitle)}, nil
	}
	app := sizedApp(t, fake)
	app.refreshEvery = time.Hour

	_, cmd := app.handleAutoFetch()

	if cmd == nil {
		t.Fatal("an enabled auto-refresh must always re-arm the tick")
	}
}

func TestRouteToPanel_ForwardsToTheFocusedPanel(t *testing.T) {
	t.Parallel()

	t.Run("pull request list moves its cursor", func(t *testing.T) {
		t.Parallel()
		app := searchApp(t)
		app.prList.SetFocused(true)

		_ = app.routeToPanel(runeKey('j'))

		sel := app.prList.SelectedPullRequest()
		if sel == nil || sel.ID != 102 {
			t.Errorf("cursor = %v, want #102 after j", sel)
		}
	})

	t.Run("detail panel keeps the right side focused", func(t *testing.T) {
		t.Parallel()
		app := searchApp(t)
		app.side = sideRight
		app.updateFocusState()

		_ = app.routeToPanel(tea.KeyMsg{Type: tea.KeyDown})

		testkit.AssertEqual(t, "side", app.side, sideRight)
	})
}
