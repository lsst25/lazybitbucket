package tui

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/config"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// listRecorder answers every list request with one pull request and records
// which repositories were asked for.
type listRecorder struct {
	mu    sync.Mutex
	repos []string
}

func (r *listRecorder) install(fake *bitbuckettest.FakeClient) {
	fake.ListPullRequestsFunc = func(
		_ context.Context, _, repo string, _ bitbucket.ListPullRequestsOptions,
	) ([]bitbucket.PullRequest, error) {
		r.mu.Lock()
		r.repos = append(r.repos, repo)
		r.mu.Unlock()
		return []bitbucket.PullRequest{newPR(repo, 1, testTitle)}, nil
	}
}

func (r *listRecorder) asked() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := slices.Clone(r.repos)
	slices.Sort(out)
	return out
}

func (r *listRecorder) reset() {
	r.mu.Lock()
	r.repos = nil
	r.mu.Unlock()
}

func scopedApp(t *testing.T) (*App, *listRecorder) {
	t.Helper()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPreview(fake)
	recorder := &listRecorder{}
	recorder.install(fake)
	app := sizedApp(t, fake)
	// Nothing is read yet, as at startup.
	app.fetched = prStore{}
	app.repoFilter = testRepo
	return app, recorder
}

// mergedTabKey is the fetch key of the extra tab twoKeyApp adds.
const mergedTabKey = "MERGED|"

// twoKeyApp is scopedApp with a second tab that has a fetch key of its own, so
// the two tabs do not share rows.
func twoKeyApp(t *testing.T) (*App, *listRecorder) {
	t.Helper()
	app, recorder := scopedApp(t)
	app.cfg.PullRequestTabs = []config.PullRequestTabConfig{
		{Name: "Open"},
		{Name: "Merged", State: "MERGED"},
	}
	app.prList.SetTabs(app.cfg.PullRequestTabs)
	testkit.AssertEqual(t, "second tab key", app.fetchKeyOf(1), mergedTabKey)
	return app, recorder
}

// rowCount is the rows on screen, after the repository filter.
func rowCount(app *App) int {
	return app.prList.ItemCount()
}

func TestFetchPullRequests_AsksOnlyForTheFilteredRepository(t *testing.T) {
	t.Parallel()
	app, recorder := scopedApp(t)

	runAll(t, app, app.fetchPullRequests(testFetchKey))

	testkit.AssertSliceEqual(t, "repositories asked", recorder.asked(), []string{testRepo})
	testkit.AssertEqual(t, "rows", len(app.merged[testFetchKey]), 1)
}

func TestFetchPullRequests_WithoutAFilterAsksForEveryRepository(t *testing.T) {
	t.Parallel()
	app, recorder := scopedApp(t)
	app.repoFilter = ""

	runAll(t, app, app.fetchPullRequests(testFetchKey))

	testkit.AssertSliceEqual(t, "repositories asked", recorder.asked(), []string{testRepo, testRepo2})
}

func TestSelectRepo_FetchesAnUnloadedRepositoryOnce(t *testing.T) {
	t.Parallel()
	app, recorder := scopedApp(t)
	runAll(t, app, app.fetchPullRequests(testFetchKey))
	recorder.reset()

	runAll(t, app, app.selectRepo(&views.RepoItem{Slug: testRepo2}))
	testkit.AssertSliceEqual(t, "second repository asked", recorder.asked(), []string{testRepo2})
	testkit.AssertEqual(t, "both repositories held", len(app.merged[testFetchKey]), 2)

	recorder.reset()
	runAll(t, app, app.selectRepo(&views.RepoItem{Slug: testRepo}))
	runAll(t, app, app.selectRepo(&views.RepoItem{Slug: testRepo2}))
	testkit.AssertEqual(t, "a loaded repository costs no request", len(recorder.asked()), 0)
}

func TestSelectRepo_AllFetchesOnlyWhatIsMissing(t *testing.T) {
	t.Parallel()
	app, recorder := scopedApp(t)
	runAll(t, app, app.fetchPullRequests(testFetchKey))
	recorder.reset()

	runAll(t, app, app.selectRepo(&views.RepoItem{All: true}))

	testkit.AssertSliceEqual(t, "only the missing repository", recorder.asked(), []string{testRepo2})
	testkit.AssertEqual(t, "rows of both repositories", len(app.merged[testFetchKey]), 2)
}

// The tick refreshes only what is in scope, so a repository that was out of
// scope for longer than the interval is read again when the user returns to it.
func TestSelectRepo_ReadsAStaleRepositoryAgain(t *testing.T) {
	t.Parallel()
	app, recorder := scopedApp(t)
	app.refreshEvery = time.Minute
	now := testNow
	app.now = func() time.Time { return now }
	runAll(t, app, app.fetchPullRequests(testFetchKey))
	runAll(t, app, app.selectRepo(&views.RepoItem{Slug: testRepo2}))
	recorder.reset()

	now = now.Add(30 * time.Second)
	runAll(t, app, app.selectRepo(&views.RepoItem{Slug: testRepo}))
	testkit.AssertEqual(t, "fresh enough", len(recorder.asked()), 0)

	now = now.Add(time.Minute)
	runAll(t, app, app.selectRepo(&views.RepoItem{Slug: testRepo2}))
	testkit.AssertSliceEqual(t, "stale repository asked", recorder.asked(), []string{testRepo2})
}

// A repository picked in the pipelines view narrows the pull request list
// behind it too, so that list has to read the repository: the switch back must
// not land on an empty list.
func TestSelectRepo_InThePipelinesViewLoadsThePullRequestsToo(t *testing.T) {
	t.Parallel()
	app, recorder := scopedApp(t)
	app.client.(*bitbuckettest.FakeClient).ListPipelinesFunc = func(
		context.Context, string, string, bitbucket.ListPipelinesOptions,
	) ([]bitbucket.Pipeline, error) {
		return nil, nil
	}
	runAll(t, app, app.fetchPullRequests(testFetchKey))
	runAll(t, app, app.switchView())
	testkit.AssertEqual(t, "in the pipelines view", app.view, viewPipelines)
	recorder.reset()

	runAll(t, app, app.selectRepo(&views.RepoItem{Slug: testRepo2}))
	testkit.AssertSliceEqual(t, "pull requests of the picked repository", recorder.asked(), []string{testRepo2})

	runAll(t, app, app.switchView())
	testkit.AssertEqual(t, "back in the pull request view", app.view, viewPullRequests)
	testkit.AssertEqual(t, "rows on screen", rowCount(app), 1)
}

// The way back into the pull request view checks the scope by itself, so a
// filter that changed while the list was not looked at cannot leave it empty.
func TestSwitchView_BackToPullRequestsLoadsTheScope(t *testing.T) {
	t.Parallel()
	app, recorder := scopedApp(t)
	runAll(t, app, app.fetchPullRequests(testFetchKey))
	app.view = viewPipelines
	app.pipelinesLoaded = true
	app.repoFilter = testRepo2
	app.prList.SetRepoFilter(testRepo2)
	recorder.reset()

	runAll(t, app, app.switchView())

	testkit.AssertSliceEqual(t, "repository in scope asked", recorder.asked(), []string{testRepo2})
	testkit.AssertEqual(t, "rows on screen", rowCount(app), 1)
}

// Tabs with different fetch keys do not share rows. Widening on one of them
// must not leave the other on the rows of the old scope.
func TestTabSwitch_LoadsTheScopeForATabWithItsOwnKey(t *testing.T) {
	t.Parallel()
	app, recorder := twoKeyApp(t)
	runAll(t, app, app.fetchPullRequests(testFetchKey))
	app.prList.NextTab()
	runAll(t, app, app.afterTabSwitch())
	testkit.AssertEqual(t, "merged tab rows", rowCount(app), 1)

	// Widen on the Merged tab, then return to the Open tab.
	runAll(t, app, app.selectRepo(&views.RepoItem{All: true}))
	testkit.AssertEqual(t, "merged tab rows after widening", rowCount(app), 2)
	recorder.reset()
	app.prList.PrevTab()
	runAll(t, app, app.afterTabSwitch())

	testkit.AssertSliceEqual(t, "missing repository asked", recorder.asked(), []string{testRepo2})
	testkit.AssertEqual(t, "open tab rows", rowCount(app), 2)
}

// A fetch that was in flight when the user widened the view dropped the
// widening's own request, so its answer has to ask for the rest.
func TestHandlePullRequestsLoaded_AsksForWhatTheViewNeededMeanwhile(t *testing.T) {
	t.Parallel()
	app, recorder := scopedApp(t)
	cmd := app.fetchPullRequests(testFetchKey)

	// The user picks the other repository before the first answer arrives.
	app.repoFilter = testRepo2
	testkit.AssertEqual(t, "dropped while in flight", app.ensureScopeLoaded(nil) == nil, true)

	runAll(t, app, cmd)

	testkit.AssertSliceEqual(t, "both repositories asked", recorder.asked(), []string{testRepo, testRepo2})
	testkit.AssertEqual(t, "rows", len(app.merged[testFetchKey]), 2)
}

func TestHandlePullRequestsLoaded_AFailingRepositoryIsNotAskedForAgain(t *testing.T) {
	t.Parallel()
	app, _ := scopedApp(t)
	app.repoFilter = ""

	app.inFlight[testFetchKey] = true
	_, _ = app.Update(pullRequestsLoadedMsg{
		fetchKey: testFetchKey,
		results: []bitbucket.RepoPullRequests{
			{Repo: testRepo, PullRequests: []bitbucket.PullRequest{newPR(testRepo, 1, testTitle)}},
			{Repo: testRepo2, Err: errors.New(testBoom)},
		},
	})

	testkit.AssertEqual(t, "no follow-up request", app.inFlight[testFetchKey], false)
	_, loaded := app.fetched.get(testFetchKey, testRepo2)
	testkit.AssertEqual(t, "failed repository stays unloaded", loaded, false)
}

func TestStoreFetched_KeepsTheRepositoriesTheFetchDidNotCover(t *testing.T) {
	t.Parallel()
	app, _ := scopedApp(t)
	app.storeFetched(testFetchKey, []bitbucket.RepoPullRequests{
		{Repo: testRepo, PullRequests: []bitbucket.PullRequest{newPR(testRepo, 1, "old")}},
		{Repo: testRepo2, PullRequests: []bitbucket.PullRequest{newPR(testRepo2, 7, "other")}},
	})

	app.storeFetched(testFetchKey, []bitbucket.RepoPullRequests{
		{Repo: testRepo, PullRequests: []bitbucket.PullRequest{newPR(testRepo, 2, "new")}},
	})

	keys := make([]string, 0, len(app.merged[testFetchKey]))
	for index := range app.merged[testFetchKey] {
		keys = append(keys, app.merged[testFetchKey][index].Key())
	}
	slices.Sort(keys)
	testkit.AssertSliceEqual(t, "rows", keys, []string{
		testWorkspace + "/" + testRepo + "#2",
		testWorkspace + "/" + testRepo2 + "#7",
	})
}

// Rows are held under the slug that was asked for. A repository whose pull
// requests spell the slug differently must still be replaced by a refresh, not
// added to.
func TestStoreFetched_ARefreshReplacesRowsWhateverTheirSlugSays(t *testing.T) {
	t.Parallel()
	app, _ := scopedApp(t)
	asked := strings.ToUpper(testRepo)

	for range 3 {
		app.storeFetched(testFetchKey, []bitbucket.RepoPullRequests{
			{Repo: asked, PullRequests: []bitbucket.PullRequest{newPR(testRepo, 1, testTitle)}},
		})
	}

	testkit.AssertEqual(t, "rows after three refreshes", len(app.merged[testFetchKey]), 1)
}

// A detail answer patches the row in the store as well, so the next rebuild
// does not bring the row without its review data back.
func TestPatchMerged_SurvivesARebuild(t *testing.T) {
	t.Parallel()
	app, _ := scopedApp(t)
	app.storeFetched(testFetchKey, []bitbucket.RepoPullRequests{
		{Repo: testRepo, PullRequests: []bitbucket.PullRequest{newPR(testRepo, 1, "list row")}},
	})

	detail := newPR(testRepo, 1, "detail row")
	app.patchMerged(&detail)
	testkit.AssertEqual(t, "flat list patched", app.merged[testFetchKey][0].Title, "detail row")

	app.storeFetched(testFetchKey, []bitbucket.RepoPullRequests{{Repo: testRepo2}})
	testkit.AssertEqual(t, "patch kept by the rebuild", app.merged[testFetchKey][0].Title, "detail row")
}

func TestWarnRefreshRate_CountsTheRepositoriesInScope(t *testing.T) {
	t.Parallel()
	app, _ := scopedApp(t)
	app.repoFilter = ""
	// Two repositories every 5 seconds is 1440 requests an hour; one is 720,
	// which is still over the budget, so use an interval between the two.
	app.refreshEvery = 8 * time.Second

	app.warnRefreshRate()
	testkit.AssertEqual(t, "warned for every repository", app.statusWarning != "", true)

	app.repoFilter = testRepo
	app.warnRefreshRate()
	testkit.AssertEqual(t, "taken back for one repository", app.statusWarning, "")
}
