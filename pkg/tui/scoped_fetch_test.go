package tui

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
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
	app.repoFilter = testRepo
	return app, recorder
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

// A fetch that was in flight when the user widened the view dropped the
// widening's own request, so its answer has to ask for the rest.
func TestHandlePullRequestsLoaded_AsksForWhatTheViewNeededMeanwhile(t *testing.T) {
	t.Parallel()
	app, recorder := scopedApp(t)
	cmd := app.fetchPullRequests(testFetchKey)

	// The user picks the other repository before the first answer arrives.
	app.repoFilter = testRepo2
	testkit.AssertEqual(t, "dropped while in flight", app.fetchUnloadedPullRequests(testFetchKey, nil) == nil, true)

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
	testkit.AssertEqual(t, "failed repository stays unloaded", app.loadedRepos[testFetchKey][testRepo2], false)
}

func TestMergeFetched_KeepsTheRepositoriesTheFetchDidNotCover(t *testing.T) {
	t.Parallel()
	app, _ := scopedApp(t)
	app.merged[testFetchKey] = []bitbucket.PullRequest{
		newPR(testRepo, 1, "old"), newPR(testRepo2, 7, "other"),
	}

	got := app.mergeFetched(testFetchKey, []bitbucket.RepoPullRequests{
		{Repo: testRepo, PullRequests: []bitbucket.PullRequest{newPR(testRepo, 2, "new")}},
	})

	keys := make([]string, 0, len(got))
	for index := range got {
		keys = append(keys, got[index].Key())
	}
	slices.Sort(keys)
	testkit.AssertSliceEqual(t, "rows", keys, []string{
		testWorkspace + "/" + testRepo + "#2",
		testWorkspace + "/" + testRepo2 + "#7",
	})
}
