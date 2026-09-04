package tui

import (
	"context"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// debounceApp holds two pull requests and nothing cached, so every selection
// schedules a debounce.
func debounceApp(t *testing.T, fake *bitbuckettest.FakeClient) *App {
	t.Helper()
	app := sizedApp(t, fake)
	app.merged[testFetchKey] = []bitbucket.PullRequest{
		newPR(testRepo, 101, testTitle),
		newPR(testRepo, 102, "second"),
	}
	app.prList.SetPullRequests(app.merged[testFetchKey])
	return app
}

func TestPreviewDebounce_SelectingBumpsTheEpochAndSchedulesATick(t *testing.T) {
	t.Parallel()
	app := debounceApp(t, &bitbuckettest.FakeClient{T: t})

	pr := app.merged[testFetchKey][0]
	_, cmd := app.Update(views.PullRequestSelectedMsg{PullRequest: &pr})

	testkit.AssertEqual(t, "epoch", app.previewEpoch, 1)
	testkit.AssertEqual(t, "previewKey", app.previewKey, testPRKey)
	if cmd == nil {
		t.Fatal("selecting an uncached pull request should schedule the debounce")
	}
	tick, ok := cmd().(previewDebounceMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want previewDebounceMsg", cmd())
	}
	testkit.AssertEqual(t, "tick key", tick.key, testPRKey)
	testkit.AssertEqual(t, "tick epoch", tick.epoch, 1)
}

func TestPreviewDebounce_AStaleTickFetchesNothing(t *testing.T) {
	t.Parallel()
	// The fake fails on any call, so a stale tick that fetched would fail here.
	app := debounceApp(t, &bitbuckettest.FakeClient{T: t})

	first := app.merged[testFetchKey][0]
	second := app.merged[testFetchKey][1]
	_, _ = app.Update(views.PullRequestSelectedMsg{PullRequest: &first})
	_, _ = app.Update(views.PullRequestSelectedMsg{PullRequest: &second})
	testkit.AssertEqual(t, "epoch after two selections", app.previewEpoch, 2)

	_, cmd := app.Update(previewDebounceMsg{key: testPRKey, epoch: 1})

	if cmd != nil {
		t.Error("the superseded selection must not cost a request")
	}
}

func TestPreviewDebounce_AFreshTickFetchesStatusesAndDetail(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.ListPullRequestStatusesFunc = func(context.Context, string, string, int,
	) ([]bitbucket.CommitStatus, error) {
		return []bitbucket.CommitStatus{newStatus(testRepo, 412, bitbucket.StatusSuccessful)}, nil
	}
	fake.GetPullRequestFunc = func(context.Context, string, string, int) (*bitbucket.PullRequest, error) {
		pr := withAuthor(newPR(testRepo, 101, testTitle), testUserUUID)
		return &pr, nil
	}
	app := debounceApp(t, fake)

	pr := app.merged[testFetchKey][0]
	_, cmd := app.Update(views.PullRequestSelectedMsg{PullRequest: &pr})
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "statuses fetched", fake.CallCount("ListPullRequestStatuses"), 1)
	testkit.AssertEqual(t, "detail fetched", fake.CallCount("GetPullRequest"), 1)
	if _, ok := app.statusCache[testPRKey]; !ok {
		t.Error("the debounced fetch should have filled the status cache")
	}
	if _, ok := app.prCache[testPRKey]; !ok {
		t.Error("a list row without review data should have been completed by a detail fetch")
	}
}

func TestPreviewDebounce_CachedStatusesAndReviewDataCostNothing(t *testing.T) {
	t.Parallel()
	app := debounceApp(t, &bitbuckettest.FakeClient{T: t})
	pr := withAuthor(newPR(testRepo, 101, testTitle), testUserUUID)
	app.statusCache[testPRKey] = statusEntry{
		statuses:  []bitbucket.CommitStatus{newStatus(testRepo, 412, bitbucket.StatusSuccessful)},
		updatedOn: pr.UpdatedOn,
		fetchedAt: testNow,
	}

	_, cmd := app.Update(views.PullRequestSelectedMsg{PullRequest: &pr})

	if cmd != nil {
		t.Error("a fully cached pull request must not schedule a debounce")
	}
	testkit.AssertEqual(t, "builds panel filled from cache", len(app.buildsPanel.Statuses()), 1)
}

func TestPreviewDebounce_ReviewDataAloneStillTriggersADetailFetch(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.GetPullRequestFunc = func(context.Context, string, string, int) (*bitbucket.PullRequest, error) {
		pr := withAuthor(newPR(testRepo, 101, testTitle), testUserUUID)
		return &pr, nil
	}
	app := debounceApp(t, fake)
	pr := newPR(testRepo, 101, testTitle) // no review data
	app.statusCache[testPRKey] = statusEntry{updatedOn: pr.UpdatedOn, fetchedAt: testNow}

	_, cmd := app.Update(views.PullRequestSelectedMsg{PullRequest: &pr})
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "no status request", fake.CallCount("ListPullRequestStatuses"), 0)
	testkit.AssertEqual(t, "detail requested", fake.CallCount("GetPullRequest"), 1)
}

func TestPreviewDebounce_ATickForAnotherPullRequestIsDropped(t *testing.T) {
	t.Parallel()
	app := debounceApp(t, &bitbuckettest.FakeClient{T: t})
	app.previewKey = testPRKey
	app.previewEpoch = 3

	_, cmd := app.Update(previewDebounceMsg{key: testPRKey2, epoch: 3})

	if cmd != nil {
		t.Error("a tick whose key is not the preview must be dropped")
	}
}

func TestPreviewSelected_IsNilWithoutASelection(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})

	if app.previewSelected() != nil {
		t.Error("previewSelected on an empty list must produce no command")
	}
}
