package tui

import (
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// repoFilterApp holds two repositories worth of pull requests and a populated
// Repos panel.
func repoFilterApp(t *testing.T) *App {
	t.Helper()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	prs := []bitbucket.PullRequest{
		newPR(testRepo, 101, testTitle),
		newPR(testRepo2, 40, "dark mode"),
	}
	app.merged[testFetchKey] = prs
	app.prList.SetPullRequests(prs)
	app.repoList.SetRepos([]bitbucket.Repository{
		{Slug: testRepo, Name: testRepo},
		{Slug: testRepo2, Name: testRepo2},
	})
	return app
}

func TestSelectRepo_NarrowsTheListAndMovesFocus(t *testing.T) {
	t.Parallel()
	app := repoFilterApp(t)
	app.leftFocus = focusRepos

	cmd := app.selectRepo(&views.RepoItem{Slug: testRepo, Name: testRepo})

	testkit.AssertEqual(t, "repo filter", app.repoFilter, testRepo)
	testkit.AssertEqual(t, "list filter", app.prList.RepoFilter(), testRepo)
	testkit.AssertEqual(t, "active slug", app.repoList.ActiveSlug(), testRepo)
	testkit.AssertEqual(t, "rows", app.prList.ItemCount(), 1)
	testkit.AssertEqual(t, "focus", app.leftFocus, focusPullRequests)
	if !containsPlain(app.statusPanel.View(), testRepo) {
		t.Errorf("status scope = %q, want the repository slug", plain(app.statusPanel.View()))
	}
	if cmd == nil {
		t.Error("selecting a repository should preview the new selection")
	}
}

func TestSelectRepo_TheAllRowClearsTheFilter(t *testing.T) {
	t.Parallel()
	app := repoFilterApp(t)
	_ = app.selectRepo(&views.RepoItem{Slug: testRepo})

	_ = app.selectRepo(&views.RepoItem{All: true})

	testkit.AssertEqual(t, "repo filter cleared", app.repoFilter, "")
	testkit.AssertEqual(t, "rows", app.prList.ItemCount(), 2)
	if !containsPlain(app.statusPanel.View(), "all repos") {
		t.Errorf("status scope = %q, want all repos", plain(app.statusPanel.View()))
	}
}

func TestSelectRepo_NilIsANoop(t *testing.T) {
	t.Parallel()
	app := repoFilterApp(t)

	if cmd := app.selectRepo(nil); cmd != nil {
		t.Error("selecting nothing must produce no command")
	}
	testkit.AssertEqual(t, "rows untouched", app.prList.ItemCount(), 2)
}

func TestSelectRepo_ThroughTheActivationKey(t *testing.T) {
	t.Parallel()
	app := repoFilterApp(t)
	app.leftFocus = focusRepos
	app.repoList.Cursor = 1 // the first real repository, after the All row

	_, _ = app.handleActivate()

	testkit.AssertEqual(t, "repo filter", app.repoFilter, testRepo)
	testkit.AssertEqual(t, "focus", app.leftFocus, focusPullRequests)
}

func TestFocusRepos_PreviewsTheRepositoryUnderTheCursor(t *testing.T) {
	t.Parallel()
	app := repoFilterApp(t)
	app.repoList.Cursor = 1

	_, _, handled := app.handleFocusAction(ActFocusRepos)

	if !handled {
		t.Fatal("focusing the repos panel must be handled")
	}
	testkit.AssertEqual(t, "detail mode", app.detailView.Mode(), views.ModeRepo)
}

func TestBrowserURL_FollowsTheFocusedPanel(t *testing.T) {
	t.Parallel()
	app := repoFilterApp(t)
	app.repoList.SetRepos([]bitbucket.Repository{{
		Slug: testRepo, Name: testRepo, FullName: testWorkspace + "/" + testRepo,
	}})
	app.repoList.Cursor = 1
	app.buildsPanel.SetPullRequest(testPRKey)
	app.buildsPanel.SetStatuses(testPRKey, []bitbucket.CommitStatus{
		newStatus(testRepo, 412, bitbucket.StatusSuccessful),
	})

	app.leftFocus = focusPullRequests
	if got := app.browserURL(); got == "" || !containsPlain(got, "pull-requests") {
		t.Errorf("pull request URL = %q", got)
	}

	app.leftFocus = focusBuilds
	if got := app.browserURL(); !containsPlain(got, "pipelines/results/412") {
		t.Errorf("build URL = %q", got)
	}

	app.leftFocus = focusRepos
	if got := app.browserURL(); !containsPlain(got, testRepo) {
		t.Errorf("repository URL = %q", got)
	}

	// The detail panel showing a repository opens that repository, not
	// whichever pull request happens to still be selected.
	app.detailView.SetRepo(app.repoList.SelectedRepo())
	app.side = sideRight
	testkit.AssertEqual(t, "detail mode", app.detailView.Mode(), views.ModeRepo)
	got := app.browserURL()
	if !containsPlain(got, testRepo) {
		t.Errorf("repository URL = %q, want the repository", got)
	}
	if containsPlain(got, "pull-requests") {
		t.Errorf("repository URL = %q, want no pull request", got)
	}
}
