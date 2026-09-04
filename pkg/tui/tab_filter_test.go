package tui

import (
	"context"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// tabFilterFixture is one pull request authored by the current user, one they
// review and one that is neither.
func tabFilterFixture() []bitbucket.PullRequest {
	mine := withAuthor(newPR(testRepo, 101, testTitle), testUserUUID)
	reviewing := withReviewer(withAuthor(newPR(testRepo, 102, "second"), testOtherUUID), testUserUUID)
	theirs := withAuthor(newPR(testRepo2, 40, "third"), testOtherUUID)
	return []bitbucket.PullRequest{mine, reviewing, theirs}
}

func TestTabFilters_EmptyUntilTheCurrentUserIsKnown(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})

	_, _ = app.Update(loadedMsg(bitbucket.RepoPullRequests{Repo: testRepo, PullRequests: tabFilterFixture()}))

	testkit.AssertEqual(t, "open tab shows everything", app.prList.ItemCount(), 3)

	app.prList.SetTabIndex(1) // Mine
	testkit.AssertEqual(t, "mine is empty without a user", app.prList.ItemCount(), 0)

	app.prList.SetTabIndex(2) // Reviewing
	testkit.AssertEqual(t, "reviewing is empty without a user", app.prList.ItemCount(), 0)

	if !containsPlain(app.View(), "current user unknown") {
		t.Errorf("status = %q, want the current-user warning", plain(app.View()))
	}
}

func TestTabFilters_RefilterWhenTheUserArrivesLate(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	_, _ = app.Update(loadedMsg(bitbucket.RepoPullRequests{Repo: testRepo, PullRequests: tabFilterFixture()}))

	_, _ = app.Update(currentUserLoadedMsg{user: &bitbucket.Account{UUID: testUserUUID, DisplayName: "Demo"}})

	app.prList.SetTabIndex(1)
	testkit.AssertEqual(t, "mine", app.prList.ItemCount(), 1)
	sel := app.prList.SelectedPullRequest()
	if sel == nil || sel.ID != 101 {
		t.Errorf("mine = %v, want the pull request authored by the user", sel)
	}

	app.prList.SetTabIndex(2)
	testkit.AssertEqual(t, "reviewing", app.prList.ItemCount(), 1)
	sel = app.prList.SelectedPullRequest()
	if sel == nil || sel.ID != 102 {
		t.Errorf("reviewing = %v, want the pull request the user reviews", sel)
	}
}

func TestTabFilters_UserKnownBeforeTheList(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})

	_, _ = app.Update(currentUserLoadedMsg{user: &bitbucket.Account{UUID: testUserUUID}})
	app.prList.SetTabIndex(1)
	_, _ = app.Update(loadedMsg(bitbucket.RepoPullRequests{Repo: testRepo, PullRequests: tabFilterFixture()}))

	testkit.AssertEqual(t, "mine", app.prList.ItemCount(), 1)
}

func TestApplyTabFilter_UnknownReviewDataExcludesTheRow(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	app.currentUser = &bitbucket.Account{UUID: testUserUUID}
	// No participants and no reviewers: review data was never loaded.
	app.merged[testFetchKey] = []bitbucket.PullRequest{newPR(testRepo, 101, testTitle)}

	testkit.AssertEqual(t, "reviewing excludes unknown review data", len(app.applyTabFilter(2)), 0)
}

func TestApplyTabFilter_UnknownTabIndexFallsBackToEverything(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	app.merged[testFetchKey] = tabFilterFixture()

	testkit.AssertEqual(t, "out of range", len(app.applyTabFilter(99)), 3)
}

func TestRefilterTabs_LeavesUnfetchedTabsUncached(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	app.cfg.PullRequestTabs[1].State = "MERGED"
	app.prList.SetTabs(app.cfg.PullRequestTabs)
	app.merged[testFetchKey] = tabFilterFixture()

	app.refilterTabs("")

	app.prList.SetTabIndex(1)
	if app.prList.HasCachedTab() {
		t.Error("a tab whose fetch has not landed must still count as uncached")
	}
}

// TestNextTab_FetchesWhenTheTabHasItsOwnFetchKey covers the other half of
// afterTabSwitch: a tab whose (state, query) key has never been fetched must
// issue the request rather than show an empty list forever.
func TestNextTab_FetchesWhenTheTabHasItsOwnFetchKey(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	// FetchPullRequests calls the fake from several goroutines, so the test must
	// not capture into a shared variable here; the fake records every call under
	// its own lock and the recorder is read only after cmd() has returned.
	fake.ListPullRequestsFunc = func(_ context.Context, _, _ string,
		_ bitbucket.ListPullRequestsOptions,
	) ([]bitbucket.PullRequest, error) {
		return nil, nil
	}
	app := sizedApp(t, fake)
	app.currentUser = &bitbucket.Account{UUID: testUserUUID}
	app.cfg.PullRequestTabs[1].State = "MERGED"
	app.prList.SetTabs(app.cfg.PullRequestTabs)
	// Only the OPEN key is cached, so tab 1 has nothing.
	app.merged[testFetchKey] = tabFilterFixture()
	app.refilterTabs("")

	_, cmd, handled := app.handleTabAction(ActNextTab)

	if !handled {
		t.Fatal("the next-tab action must be handled")
	}
	testkit.AssertEqual(t, "tab", app.prList.GetTabIndex(), 1)
	if cmd == nil {
		t.Fatal("switching to an unfetched tab must issue a request")
	}
	wantKey := app.fetchKeyOf(1)
	testkit.AssertEqual(t, "fetch key", wantKey, "MERGED|")
	testkit.AssertEqual(t, "in flight", app.inFlight[wantKey], true)

	msg, ok := cmd().(pullRequestsLoadedMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want pullRequestsLoadedMsg", msg)
	}
	testkit.AssertEqual(t, "loaded fetch key", msg.fetchKey, wantKey)
	if len(fake.ListPullRequestsCalls) == 0 {
		t.Fatal("no ListPullRequests call was recorded")
	}
	gotStates := fake.ListPullRequestsCalls[0].Opts.States
	testkit.AssertEqual(t, "requested states", len(gotStates), 1)
	if len(gotStates) == 1 {
		testkit.AssertEqual(t, "requested state", gotStates[0], bitbucket.PullRequestMerged)
	}
}

func TestNextTab_PreviewsWhenTheTabIsCached(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	app.currentUser = &bitbucket.Account{UUID: testUserUUID}
	app.merged[testFetchKey] = tabFilterFixture()
	app.refilterTabs("")

	_, cmd, handled := app.handleTabAction(ActNextTab)

	if !handled {
		t.Fatal("the next-tab action must be handled")
	}
	testkit.AssertEqual(t, "tab", app.prList.GetTabIndex(), 1)
	// Every default tab shares one fetch key, so the data is already there and
	// the command is the preview, not a request.
	if cmd == nil {
		t.Error("switching to a cached tab should still preview its selection")
	}
}

func TestPrevTab_OnTheDetailPanelWalksTheDetailTabs(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	app.side = sideRight

	_, cmd, handled := app.handleTabAction(ActPrevTab)

	if !handled || cmd != nil {
		t.Error("tab keys on the detail panel are handled without a command")
	}
}
