package tui

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// prefetchApp holds three uncached pull requests on the active tab.
func prefetchApp(t *testing.T, fake *bitbuckettest.FakeClient) *App {
	t.Helper()
	app := sizedApp(t, fake)
	prs := []bitbucket.PullRequest{
		withAuthor(newPR(testRepo, 101, testTitle), testUserUUID),
		withAuthor(newPR(testRepo, 102, "second"), testUserUUID),
		withAuthor(newPR(testRepo2, 40, "third"), testUserUUID),
	}
	app.merged[testFetchKey] = prs
	app.prList.SetPullRequests(prs)
	return app
}

func TestStartPrefetch_AsksForEveryStaleRowAtOnce(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.ListPullRequestStatusesFunc = func(_ context.Context, _, repo string, _ int,
	) ([]bitbucket.CommitStatus, error) {
		return []bitbucket.CommitStatus{newStatus(repo, 412, bitbucket.StatusSuccessful)}, nil
	}
	app := prefetchApp(t, fake)

	cmd := app.startPrefetch()

	testkit.AssertEqual(t, "every row is in flight", len(app.prefetching), 3)
	testkit.AssertEqual(t, "nothing waits in the queue", len(app.prefetchQueue), 0)
	if cmd == nil {
		t.Fatal("startPrefetch should have started the requests")
	}

	runAll(t, app, cmd)

	testkit.AssertEqual(t, "one request per row", fake.CallCount("ListPullRequestStatuses"), 3)
	testkit.AssertEqual(t, "nothing left in flight", len(app.prefetching), 0)
	testkit.AssertEqual(t, "cached rows", len(app.statusCache), 3)
}

// The rows that are warmed are the ones on screen: with a repository filter
// set, a row the filter hides is not worth a request.
func TestStartPrefetch_WarmsOnlyTheRowsOnScreen(t *testing.T) {
	t.Parallel()
	app := prefetchApp(t, &bitbuckettest.FakeClient{T: t})
	app.prList.SetRepoFilter(testRepo2)

	_ = app.startPrefetch()

	testkit.AssertEqual(t, "rows in flight", len(app.prefetching), 1)
	testkit.AssertEqual(t, "the visible row", app.prefetching[testWorkspace+"/"+testRepo2+"#40"], true)
}

func TestStartPrefetch_SkipsRowsWhoseStatusesAreFresh(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.ListPullRequestStatusesFunc = func(_ context.Context, _, repo string, _ int,
	) ([]bitbucket.CommitStatus, error) {
		return []bitbucket.CommitStatus{newStatus(repo, 412, bitbucket.StatusSuccessful)}, nil
	}
	app := prefetchApp(t, fake)
	for _, pr := range app.merged[testFetchKey][:2] {
		app.statusCache[pr.Key()] = statusEntry{
			statuses:  []bitbucket.CommitStatus{newStatus(testRepo, 412, bitbucket.StatusSuccessful)},
			updatedOn: pr.UpdatedOn,
			fetchedAt: testNow,
		}
	}

	runAll(t, app, app.startPrefetch())

	testkit.AssertEqual(t, "only the stale row was fetched", fake.CallCount("ListPullRequestStatuses"), 1)
}

func TestStartPrefetch_StopsAtTheLimit(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	prs := make([]bitbucket.PullRequest, prefetchLimit+5)
	for i := range prs {
		prs[i] = withAuthor(newPR(testRepo, 100+i, testTitle), testUserUUID)
	}
	app.prList.SetPullRequests(prs)

	_ = app.startPrefetch()

	testkit.AssertEqual(t, "requests capped", len(app.prefetching), prefetchLimit)
	testkit.AssertEqual(t, "nothing queued past the limit", len(app.prefetchQueue), 0)

	// A refill while those are in flight asks for nothing twice, and the rows
	// in flight still count against the limit.
	cmd := app.startPrefetch()

	if cmd != nil {
		t.Error("a refill must not ask again for rows that are in flight")
	}
	testkit.AssertEqual(t, "still capped", len(app.prefetching), prefetchLimit)
	testkit.AssertEqual(t, "still nothing queued", len(app.prefetchQueue), 0)
}

// A refill does not cancel what is in flight, so the pool is what keeps a
// second batch from being stacked on the first: it waits in the queue, and
// each answer lets one more through.
func TestPrefetch_APoolThatIsFullMakesTheNextBatchWait(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	batch := func(first int) []bitbucket.PullRequest {
		prs := make([]bitbucket.PullRequest, prefetchLimit)
		for i := range prs {
			prs[i] = withAuthor(newPR(testRepo, first+i, testTitle), testUserUUID)
		}
		return prs
	}
	firstBatch := batch(100)
	app.prList.SetPullRequests(firstBatch)
	_ = app.startPrefetch()

	app.prList.SetPullRequests(batch(500))
	cmd := app.startPrefetch()

	if cmd != nil {
		t.Error("a full pool must not start more requests")
	}
	testkit.AssertEqual(t, "second batch waits", len(app.prefetchQueue), prefetchLimit)

	_, cmd = app.Update(prefetchedMsg{key: firstBatch[0].Key()})

	if cmd == nil {
		t.Error("an answer must let the next request through")
	}
	testkit.AssertEqual(t, "pool full again", len(app.prefetching), prefetchConcurrency)
	testkit.AssertEqual(t, "one less waits", len(app.prefetchQueue), prefetchLimit-1)
}

// Every cache a prefetch fills is keyed by the pull request, so an answer is
// worth keeping whatever the list shows when it arrives.
func TestPrefetch_AnAnswerForARowThatLeftTheScreenIsKept(t *testing.T) {
	t.Parallel()
	app := prefetchApp(t, &bitbuckettest.FakeClient{T: t})
	_ = app.startPrefetch()
	app.prList.SetRepoFilter(testRepo2)
	_ = app.startPrefetch()

	_, _ = app.Update(prefetchedMsg{
		key:      testPRKey,
		statuses: []bitbucket.CommitStatus{newStatus(testRepo, 412, bitbucket.StatusSuccessful)},
	})

	if _, ok := app.statusCache[testPRKey]; !ok {
		t.Error("the answer of a request that was already sent must reach the cache")
	}
	testkit.AssertEqual(t, "no longer in flight", app.prefetching[testPRKey], false)
}

func TestPrefetch_ErrorsAreSilent(t *testing.T) {
	t.Parallel()
	app := prefetchApp(t, &bitbuckettest.FakeClient{T: t})

	_, _ = app.Update(prefetchedMsg{key: testPRKey, err: errors.New(testBoom)})

	testkit.AssertEqual(t, "status untouched", app.statusPanel.ErrorMessage(), "")
	if _, ok := app.statusCache[testPRKey]; ok {
		t.Error("a failed prefetch must not cache anything")
	}
}

func TestPrefetch_ArrivingReviewDataRefiltersTheTabs(t *testing.T) {
	t.Parallel()
	app := prefetchApp(t, &bitbuckettest.FakeClient{T: t})
	app.currentUser = &bitbucket.Account{UUID: testUserUUID}
	app.prList.SetTabIndex(2) // Reviewing

	detail := withReviewer(newPR(testRepo, 101, testTitle), testUserUUID)
	_, _ = app.Update(prefetchedMsg{key: testPRKey, pr: &detail})

	if _, ok := app.prCache[testPRKey]; !ok {
		t.Fatal("the prefetched detail should have been cached")
	}
	testkit.AssertEqual(t, "reviewing tab now has the row", app.prList.ItemCount(), 1)
}

func TestStatusesStale_TheFourRules(t *testing.T) {
	t.Parallel()
	pr := newPR(testRepo, 101, testTitle)

	cases := []struct {
		name  string
		setup func(app *App)
		want  bool
	}{
		{"missing entry", func(*App) {}, true},
		{
			name: "the pull request has changed since",
			setup: func(app *App) {
				app.statusCache[testPRKey] = statusEntry{
					updatedOn: pr.UpdatedOn.Add(-time.Hour),
					fetchedAt: testNow,
					statuses:  []bitbucket.CommitStatus{newStatus(testRepo, 412, bitbucket.StatusSuccessful)},
				}
			},
			want: true,
		},
		{
			name: "a build was still running",
			setup: func(app *App) {
				app.statusCache[testPRKey] = statusEntry{
					updatedOn: pr.UpdatedOn,
					fetchedAt: testNow,
					statuses:  []bitbucket.CommitStatus{newStatus(testRepo, 413, bitbucket.StatusInProgress)},
				}
			},
			want: true,
		},
		{
			name: "no build reported and a refresh period has passed",
			setup: func(app *App) {
				app.refreshEvery = time.Minute
				app.statusCache[testPRKey] = statusEntry{
					updatedOn: pr.UpdatedOn,
					fetchedAt: testNow.Add(-2 * time.Minute),
				}
			},
			want: true,
		},
		{
			name: "no build reported but the period has not passed",
			setup: func(app *App) {
				app.refreshEvery = time.Hour
				app.statusCache[testPRKey] = statusEntry{
					updatedOn: pr.UpdatedOn,
					fetchedAt: testNow.Add(-time.Minute),
				}
			},
			want: false,
		},
		{
			name: "no build reported and auto-refresh is off",
			setup: func(app *App) {
				app.statusCache[testPRKey] = statusEntry{updatedOn: pr.UpdatedOn, fetchedAt: testNow}
			},
			want: false,
		},
		{
			name: "fresh and finished",
			setup: func(app *App) {
				app.statusCache[testPRKey] = statusEntry{
					updatedOn: pr.UpdatedOn,
					fetchedAt: testNow,
					statuses:  []bitbucket.CommitStatus{newStatus(testRepo, 412, bitbucket.StatusSuccessful)},
				}
			},
			want: false,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
			testCase.setup(app)

			testkit.AssertEqual(t, "stale", app.statusesStale(&pr), testCase.want)
		})
	}
}

func TestPrefetchNext_SkipsKeysThatAreNoLongerKnown(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	app.prefetchQueue = []string{"acme/gone#1", "acme/gone#2"}

	if cmd := app.prefetchNext(); cmd != nil {
		t.Error("keys that no longer resolve must not cost a request")
	}
	testkit.AssertEqual(t, "queue drained", len(app.prefetchQueue), 0)
}
