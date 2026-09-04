package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// loadedMsg builds a pullRequestsLoadedMsg out of per-repository results.
func loadedMsg(results ...bitbucket.RepoPullRequests) pullRequestsLoadedMsg {
	return pullRequestsLoadedMsg{fetchKey: testFetchKey, results: results}
}

func TestHandlePullRequestsLoaded_MergesEveryRepository(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	app.inFlight[testFetchKey] = true

	_, _ = app.Update(loadedMsg(
		bitbucket.RepoPullRequests{Repo: testRepo, PullRequests: []bitbucket.PullRequest{
			withAuthor(newPR(testRepo, 101, testTitle), testUserUUID),
		}},
		bitbucket.RepoPullRequests{Repo: testRepo2, PullRequests: []bitbucket.PullRequest{
			withAuthor(newPR(testRepo2, 40, "dark mode"), testOtherUUID),
		}},
	))

	testkit.AssertEqual(t, "in flight cleared", app.inFlight[testFetchKey], false)
	testkit.AssertEqual(t, "merged rows", len(app.merged[testFetchKey]), 2)
	testkit.AssertEqual(t, "list rows", app.prList.ItemCount(), 2)
	testkit.AssertEqual(t, "status cleared", app.statusPanel.ErrorMessage(), "")
}

func TestHandlePullRequestsLoaded_OneRepositoryFailingKeepsTheRest(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})

	_, _ = app.Update(loadedMsg(
		bitbucket.RepoPullRequests{Repo: testRepo, PullRequests: []bitbucket.PullRequest{
			newPR(testRepo, 101, testTitle),
		}},
		bitbucket.RepoPullRequests{Repo: testRepo2, Err: errors.New(testBoom)},
	))

	testkit.AssertEqual(t, "surviving rows", app.prList.ItemCount(), 1)
	if app.statusPanel.ErrorMessage() == "" {
		t.Error("a per-repository failure must be reported")
	}
	if !stillOnline(app) {
		t.Error("one failing repository must not mark the app offline")
	}
}

func TestHandlePullRequestsLoaded_EveryRepositoryFailingGoesOffline(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})

	_, _ = app.Update(loadedMsg(
		bitbucket.RepoPullRequests{Repo: testRepo, Err: errors.New(testBoom)},
		bitbucket.RepoPullRequests{Repo: testRepo2, Err: errors.New(testBoom)},
	))

	if stillOnline(app) {
		t.Error("every repository failing must mark the app offline")
	}
}

func TestHandlePullRequestsLoaded_FillsTheRepositoryCounts(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	app.repoList.SetRepos([]bitbucket.Repository{{Slug: testRepo}, {Slug: testRepo2}})

	_, _ = app.Update(loadedMsg(
		bitbucket.RepoPullRequests{Repo: testRepo, PullRequests: []bitbucket.PullRequest{
			newPR(testRepo, 101, testTitle), newPR(testRepo, 102, "second"),
		}},
	))

	if !containsPlain(app.repoList.View(), "2 open") {
		t.Errorf("repo panel should show the count:\n%s", plain(app.repoList.View()))
	}
}

func TestHandlePullRequestsLoaded_GitBranchAutoSelectRunsOnce(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	app.gitBranch = testBranch

	other := newPR(testRepo2, 40, "dark mode")
	other.Source.Branch = bitbucket.Branch{Name: "feature/dark-mode"}

	_, _ = app.Update(loadedMsg(bitbucket.RepoPullRequests{
		Repo: testRepo,
		PullRequests: []bitbucket.PullRequest{
			other,
			newPR(testRepo, 101, testTitle),
		},
	}))

	testkit.AssertEqual(t, "cursor on the checked-out branch", app.previewKey, testPRKey)
	if !app.gitAutoSelect {
		t.Error("the auto-select must be marked done so it never fights the user again")
	}
}

func TestHandleReposLoaded(t *testing.T) {
	t.Parallel()

	t.Run("an error stops the load", func(t *testing.T) {
		t.Parallel()
		app := sizedApp(t, &bitbuckettest.FakeClient{T: t})

		_, cmd := app.Update(reposLoadedMsg{err: errors.New(testBoom)})

		if cmd != nil {
			t.Error("a failed repository load must not fetch pull requests")
		}
		testkit.AssertEqual(t, "status error", app.statusPanel.ErrorMessage(), testBoom)
	})

	t.Run("no visible repositories is explained", func(t *testing.T) {
		t.Parallel()
		app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
		app.cfg.Bitbucket.Repositories = nil

		_, _ = app.Update(reposLoadedMsg{})

		if !containsPlain(app.View(), "no repositories visible") {
			t.Errorf("status = %q, want the discovery hint", plain(app.View()))
		}
		testkit.AssertEqual(t, "splash note", app.splashInfo.RepoNote != "", true)
	})

	t.Run("a full discovery page warns about the cap", func(t *testing.T) {
		t.Parallel()
		app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
		app.cfg.Bitbucket.Repositories = nil
		repos := make([]bitbucket.Repository, repoDiscoveryLimit)
		for i := range repos {
			repos[i] = bitbucket.Repository{Slug: testRepo}
		}

		_, _ = app.Update(reposLoadedMsg{repos: repos})

		if !containsPlain(app.View(), "first 25 repositories") {
			t.Errorf("status = %q, want the cap warning", plain(app.View()))
		}
	})
}

// TestRepoDiscovery_IsRetriedByTheRefreshPaths pins the recovery path: without
// repository discovery repoSlugs is empty and every pull request fetch is a
// silent no-op, so a failed (or empty) discovery used to leave the session
// permanently inert.
func TestRepoDiscovery_IsRetriedByTheRefreshPaths(t *testing.T) {
	t.Parallel()

	discovering := func(t *testing.T) *App {
		t.Helper()
		fake := &bitbuckettest.FakeClient{T: t}
		fake.ListRepositoriesFunc = func(_ context.Context, _ string,
			_ bitbucket.ListRepositoriesOptions,
		) ([]bitbucket.Repository, error) {
			return []bitbucket.Repository{{Slug: testRepo, FullName: testWorkspace + "/" + testRepo}}, nil
		}
		app := sizedApp(t, fake)
		// Discovery mode: nothing configured, so repoSlugs comes from the API.
		app.cfg.Bitbucket.Repositories = nil
		app.repoSlugs = nil
		// Non-zero so the auto-refresh path is live, but short enough that
		// draining its re-armed tick costs nothing.
		app.refreshEvery = time.Nanosecond
		return app
	}

	// issuesDiscovery reports whether cmd asks the API for repositories.
	issuesDiscovery := func(t *testing.T, cmd tea.Cmd) bool {
		t.Helper()
		for _, msg := range collectMsgs(cmd) {
			if loaded, ok := msg.(reposLoadedMsg); ok {
				return loaded.err == nil && len(loaded.repos) == 1
			}
		}
		return false
	}

	cases := []struct {
		name  string
		spoil func(app *App)
	}{
		{"discovery failed", func(app *App) {
			_, _ = app.Update(reposLoadedMsg{err: errors.New(testBoom)})
		}},
		{"discovery came back empty", func(app *App) {
			_, _ = app.Update(reposLoadedMsg{})
		}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name+"/refresh all", func(t *testing.T) {
			t.Parallel()
			app := discovering(t)
			testCase.spoil(app)

			_, cmd, ok := app.handlePullRequestAction(ActRefreshAll)

			if !ok {
				t.Fatal("refresh all must be handled")
			}
			if !issuesDiscovery(t, cmd) {
				t.Error("R must re-run repository discovery, not a no-op pull request fetch")
			}
		})

		t.Run(testCase.name+"/auto refresh", func(t *testing.T) {
			t.Parallel()
			app := discovering(t)
			testCase.spoil(app)

			_, cmd := app.handleAutoFetch()

			if !issuesDiscovery(t, cmd) {
				t.Error("the auto-refresh tick must re-run repository discovery")
			}
		})

		t.Run(testCase.name+"/tab switch", func(t *testing.T) {
			t.Parallel()
			app := discovering(t)
			testCase.spoil(app)

			_, cmd, ok := app.handleTabAction(ActNextTab)

			if !ok {
				t.Fatal("the next-tab action must be handled")
			}
			if !issuesDiscovery(t, cmd) {
				t.Error("switching tabs must re-run repository discovery")
			}
		})
	}

	t.Run("a successful discovery clears the flag", func(t *testing.T) {
		t.Parallel()
		app := discovering(t)
		_, _ = app.Update(reposLoadedMsg{err: errors.New(testBoom)})
		testkit.AssertEqual(t, "flag set", app.repoDiscoveryFailed, true)

		_, _ = app.Update(reposLoadedMsg{repos: []bitbucket.Repository{{Slug: testRepo}}})

		testkit.AssertEqual(t, "flag cleared", app.repoDiscoveryFailed, false)
		testkit.AssertEqual(t, "no more discovery", app.needsRepoDiscovery(), false)
	})
}

func TestHandleCurrentUserLoaded_AuthFailureIsAWarningNotAnError(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})

	_, _ = app.Update(currentUserLoadedMsg{err: &bitbucket.APIError{Status: 403, Message: "Forbidden"}})

	testkit.AssertEqual(t, "no hard error", app.statusPanel.ErrorMessage(), "")
	if !containsPlain(app.View(), "current user unknown") {
		t.Errorf("status = %q, want the access-token hint", plain(app.View()))
	}
}

func TestHandleCurrentUserLoaded_OtherFailuresAreErrors(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})

	_, _ = app.Update(currentUserLoadedMsg{err: errors.New(testBoom)})

	testkit.AssertEqual(t, "status error", app.statusPanel.ErrorMessage(), testBoom)
}

func TestHandleStatusesLoaded_FreshResultFillsEveryConsumer(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	pr := newPR(testRepo, 101, testTitle)
	app.prList.SetPullRequests([]bitbucket.PullRequest{pr})
	app.previewKey = testPRKey
	app.detailView.SetPullRequest(&pr)
	app.buildsPanel.SetPullRequest(testPRKey)

	_, _ = app.Update(statusesLoadedMsg{
		key:       testPRKey,
		statuses:  []bitbucket.CommitStatus{newStatus(testRepo, 412, bitbucket.StatusSuccessful)},
		updatedOn: pr.UpdatedOn,
	})

	entry, ok := app.statusCache[testPRKey]
	if !ok {
		t.Fatal("statuses should have been cached")
	}
	testkit.AssertEqual(t, "cached fetch time", entry.fetchedAt.Equal(testNow), true)
	testkit.AssertEqual(t, "builds panel", len(app.buildsPanel.Statuses()), 1)
	if !containsPlain(app.prList.View(), "✔") {
		t.Errorf("the build glyph should reach the list:\n%s", plain(app.prList.View()))
	}
}

func TestHandleStatusesLoaded_StaleResultForAnotherPullRequestIsDropped(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	app.previewKey = testPRKey
	app.previewEpoch = 5

	_, _ = app.Update(statusesLoadedMsg{
		key:      testPRKey2,
		epoch:    1,
		statuses: []bitbucket.CommitStatus{newStatus(testRepo, 412, bitbucket.StatusSuccessful)},
	})

	if _, ok := app.statusCache[testPRKey2]; ok {
		t.Error("a stale status response for another pull request must be dropped")
	}
}

// TestHandleStatusesLoaded_StaleEpochForThePreviewedPullRequestIsDropped covers
// the case an explicit refresh creates: two fetches for the same key are in
// flight, and the older one must not overwrite the newer result.
func TestHandleStatusesLoaded_StaleEpochForThePreviewedPullRequestIsDropped(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	pr := withAuthor(newPR(testRepo, 101, testTitle), testUserUUID)
	app.merged[testFetchKey] = []bitbucket.PullRequest{pr}
	app.prList.SetPullRequests(app.merged[testFetchKey])
	app.previewKey = testPRKey
	app.previewEpoch = 2
	app.buildsPanel.SetPullRequest(testPRKey)
	fresh := []bitbucket.CommitStatus{newStatus(testRepo, 412, bitbucket.StatusSuccessful)}
	app.cacheStatuses(testPRKey, fresh, pr.UpdatedOn)
	app.buildsPanel.SetStatuses(testPRKey, fresh)

	_, _ = app.Update(statusesLoadedMsg{
		key:      testPRKey,
		epoch:    1,
		statuses: []bitbucket.CommitStatus{newStatus(testRepo, 413, bitbucket.StatusInProgress)},
	})

	entry := app.statusCache[testPRKey]
	if len(entry.statuses) != 1 || entry.statuses[0].State != bitbucket.StatusSuccessful {
		t.Errorf("cached statuses = %+v, want the fresh SUCCESSFUL entry", entry.statuses)
	}
	if !containsPlain(app.prList.View(), "\u2714") {
		t.Errorf("the list glyph must not revert:\n%s", plain(app.prList.View()))
	}
}

func TestHandleStatusesLoaded_ErrorReachesTheBuildsPanel(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	app.previewKey = testPRKey
	app.buildsPanel.SetPullRequest(testPRKey)

	_, _ = app.Update(statusesLoadedMsg{key: testPRKey, err: errors.New(testBoom)})

	testkit.AssertEqual(t, "status error", app.statusPanel.ErrorMessage(), testBoom)
	if !containsPlain(app.buildsPanel.View(), testBoom) {
		t.Errorf("builds panel = %q, want the error", plain(app.buildsPanel.View()))
	}
}

func TestHandlePullRequestDetailLoaded_StaleEpochIsDropped(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	app.previewEpoch = 4
	pr := newPR(testRepo, 101, testTitle)

	_, _ = app.Update(pullRequestDetailLoadedMsg{pr: &pr, epoch: 1})

	if _, ok := app.prCache[testPRKey]; ok {
		t.Error("a superseded detail response must not reach the cache")
	}
}

func TestHandleStepLogLoaded_ErrorIsShownInPlace(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	pr := newPR(testRepo, 101, testTitle)
	app.detailView.SetPullRequest(&pr)
	app.detailView.SetPipeline(testPRKey, &bitbucket.Pipeline{UUID: "{run}", BuildNumber: 412},
		[]bitbucket.PipelineStep{{UUID: "{step}", Name: "Build"}})
	app.detailView.SetStepLogLoading("{run}", "{step}")

	_, _ = app.Update(stepLogLoadedMsg{pipelineUUID: "{run}", stepUUID: "{step}", err: errors.New(testBoom)})

	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModeStepLog)
	testkit.AssertEqual(t, "status error", app.statusPanel.ErrorMessage(), testBoom)
}

func TestPullRequestByKey_ResolvesThroughEveryStore(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})

	if app.pullRequestByKey(testPRKey) != nil {
		t.Error("an unknown key must resolve to nil")
	}

	app.merged[testFetchKey] = []bitbucket.PullRequest{newPR(testRepo, 101, testTitle)}
	if app.pullRequestByKey(testPRKey) == nil {
		t.Error("the merged result should resolve the key")
	}

	cached := newPR(testRepo, 101, "cached title")
	app.prCache[testPRKey] = &cached
	got := app.pullRequestByKey(testPRKey)
	if got == nil || got.Title != "cached title" {
		t.Errorf("pullRequestByKey = %v, want the cached detail to win", got)
	}
}

func TestCurrentPullRequest_PrefersThePreview(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	app.prList.SetPullRequests([]bitbucket.PullRequest{
		newPR(testRepo, 101, testTitle),
		newPR(testRepo, 102, "second"),
	})
	app.merged[testFetchKey] = app.prList.CurrentPullRequests()
	app.previewKey = testPRKey2

	got := app.currentPullRequest()

	if got == nil || got.ID != 102 {
		t.Errorf("currentPullRequest = %v, want the previewed #102", got)
	}
}

func TestCountsBySlug(t *testing.T) {
	t.Parallel()

	counts := countsBySlug([]bitbucket.PullRequest{
		newPR(testRepo, 101, testTitle),
		newPR(testRepo, 102, "second"),
		newPR(testRepo2, 40, "third"),
	})

	testkit.AssertEqual(t, testRepo, counts[testRepo], 2)
	testkit.AssertEqual(t, testRepo2, counts[testRepo2], 1)
}

func stillOnline(app *App) bool {
	return !containsPlain(app.View(), "✗")
}

func containsPlain(view, want string) bool {
	return strings.Contains(plain(view), want)
}
