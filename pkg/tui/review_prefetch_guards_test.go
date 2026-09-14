package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/config"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// The guards around the prefetch: what happens when something else wants the
// same answer, when the answer is for a head the branch has left, when the
// panel is not really showing a pull request, and when a prefetch fails behind
// the overview.

func TestChangedFilesCommand_JoinsTheDiffStatAlreadyInFlight(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubReviewTabs(fake)
	app := detailFocusApp(t, fake)
	app.cfg.CustomCommands = []config.CustomCommandConfig{changedFilesCmd("detail")}
	app.initCustomCommands()

	_, focus, _ := app.handleFocusAction(ActFocusDetail)
	answers := collectMsgs(focus) // the fetches run; their answers are held back
	testkit.AssertEqual(t, "diffstat requests after focus", fake.CallCount(methodDiffStat), 1)

	_, cmd, ok := app.handleCustomCommand("E")
	if !ok {
		t.Fatal("the key should be consumed")
	}
	testkit.AssertEqual(t, "no fetch of its own", cmd == nil, true)
	testkit.AssertEqual(t, "diffstat requests after the key", fake.CallCount(methodDiffStat), 1)
	if app.pendingCmd == nil || app.pendingCmd.key != testPRKey {
		t.Fatalf("pending command = %v, want one waiting on %s", app.pendingCmd, testPRKey)
	}

	// The answer already on its way resumes the command.
	for _, msg := range answers {
		loaded, isDiffStat := msg.(diffStatLoadedMsg)
		if !isDiffStat {
			continue
		}
		_, next := app.handleDiffStatLoaded(loaded)
		testkit.AssertEqual(t, "resumed", next != nil, true)
	}
	testkit.AssertEqual(t, "pending cleared", app.pendingCmd == nil, true)
}

func TestPipelinesView_RunKeyCollidingWithThePullRequestKeyStillShowsTheRun(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	// One run whose build number equals the previewed pull request's id, so the
	// run key and the pull request key are the same string.
	fake.ListPipelinesFunc = func(
		_ context.Context, _, repoSlug string, _ bitbucket.ListPipelinesOptions,
	) ([]bitbucket.Pipeline, error) {
		if repoSlug == testRepo {
			return []bitbucket.Pipeline{newPipelineRun(testRepo, 101, 101, time.Hour)}, nil
		}
		return nil, nil
	}
	// The steps fail, so nothing is cached for the run and the settled
	// shortcut is what decides whether the panel is re-pointed.
	fake.ListPipelineStepsFunc = func(context.Context, string, string, string) ([]bitbucket.PipelineStep, error) {
		return nil, errors.New(testBoom)
	}
	stubReviewTabs(fake)
	app := detailFocusApp(t, fake)

	runAll(t, app, app.switchView()) // pipelines
	runAll(t, app, app.switchView()) // pull requests: the panel is the pull request again
	runAll(t, app, app.switchView()) // pipelines: the run must come back
	testkit.AssertEqual(t, "view", app.view, viewPipelines)
	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModePipeline)

	_, cmd := app.handleActivate()
	runAll(t, app, cmd)
	testkit.AssertEqual(t, "side", app.side, sideRight)
	assertReviewRequests(t, fake, "enter in the pipelines view", 0)
}

func TestRefreshKey_ReissuesAFetchStillInFlight(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubReviewTabs(fake)
	fake.ListPullRequestStatusesFunc = func(context.Context, string, string, int) ([]bitbucket.CommitStatus, error) {
		return nil, nil
	}
	fake.GetPullRequestFunc = func(context.Context, string, string, int) (*bitbucket.PullRequest, error) {
		pr := withSource(withAuthor(newPR(testRepo, 101, testTitle), testUserUUID), testSourceHash)
		return &pr, nil
	}
	app := detailFocusApp(t, fake)

	_, focus, _ := app.handleFocusAction(ActFocusDetail)
	collectMsgs(focus) // fetched, answers held back
	assertReviewRequests(t, fake, "after focus", 1)
	runAll(t, app, app.jumpToTab(views.TabFiles)) // waits for the prefetch, no request

	_, cmd, handled := app.handleActionRefresh()
	if !handled {
		t.Fatal("refresh must be handled")
	}
	collectMsgs(cmd)
	testkit.AssertEqual(t, "diffstat requests after r", fake.CallCount(methodDiffStat), 2)
}

func TestReviewAnswer_ForAnOldHeadDoesNotOverwriteTheCurrentOne(t *testing.T) {
	t.Parallel()
	oldHead := strings.Repeat("b", 40)
	current := []bitbucket.Commit{{Hash: testSourceHash}}
	stale := []bitbucket.Commit{{Hash: oldHead}, {Hash: strings.Repeat("c", 40)}}

	t.Run("the old answer landing last is dropped", func(t *testing.T) {
		t.Parallel()
		app := reviewApp(t, &bitbuckettest.FakeClient{T: t}) // the pull request is at testSourceHash
		app.handleCommitsLoaded(commitsLoadedMsg{key: testPRKey, commits: current, hash: testSourceHash, err: nil})
		app.handleCommitsLoaded(commitsLoadedMsg{key: testPRKey, commits: stale, hash: oldHead, err: nil})

		testkit.AssertEqual(t, "cached head", app.commitsCache[testPRKey].sourceHash, testSourceHash)
		testkit.AssertEqual(t, "commits on screen", len(app.detailView.Commits()), 1)
	})

	t.Run("the current answer landing last still replaces the old one", func(t *testing.T) {
		t.Parallel()
		app := reviewApp(t, &bitbuckettest.FakeClient{T: t})
		app.handleCommitsLoaded(commitsLoadedMsg{key: testPRKey, commits: stale, hash: oldHead, err: nil})
		app.handleCommitsLoaded(commitsLoadedMsg{key: testPRKey, commits: current, hash: testSourceHash, err: nil})

		testkit.AssertEqual(t, "cached head", app.commitsCache[testPRKey].sourceHash, testSourceHash)
		testkit.AssertEqual(t, "commits on screen", len(app.detailView.Commits()), 1)
	})
}

func TestPrefetchFailure_StaysQuietUntilTheTabIsOpened(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.ListPullRequestDiffStatFunc = func(context.Context, string, string, int) ([]bitbucket.DiffStatEntry, error) {
		return nil, &bitbucket.APIError{Status: 555, Message: "Diff Too Large"}
	}
	fake.ListPullRequestCommitsFunc = func(context.Context, string, string, int,
		bitbucket.ListOptions,
	) ([]bitbucket.Commit, error) {
		return nil, errors.New(testBoom)
	}
	stubComments(fake)
	app := detailFocusApp(t, fake)

	_, cmd, _ := app.handleFocusAction(ActFocusDetail)
	runAll(t, app, cmd)

	// The reader is on the overview: neither failure is announced there.
	testkit.AssertEqual(t, "tab", app.detailView.ActiveTab(), views.TabOverview)
	if bar := plain(app.helpBar.View()); strings.Contains(bar, "too large") {
		t.Errorf("the help bar announces a tab the reader is not on:\n%s", bar)
	}
	testkit.AssertEqual(t, "status panel error", app.statusPanel.ErrorMessage(), "")

	// Each tab shows its own failure once opened, still without a fetch.
	runAll(t, app, app.jumpToTab(views.TabFiles))
	if got := plain(app.detailView.View()); !strings.Contains(got, "too large — open in browser (o)") {
		t.Errorf("the Files tab should name the way out:\n%s", got)
	}
	runAll(t, app, app.jumpToTab(views.TabCommits))
	if got := plain(app.detailView.View()); !strings.Contains(got, testBoom) {
		t.Errorf("the Commits tab should show why it is empty:\n%s", got)
	}
	testkit.AssertEqual(t, "diffstat requests", fake.CallCount(methodDiffStat), 1)
	testkit.AssertEqual(t, "commit requests", fake.CallCount(methodCommits), 1)
}

func TestPrefetchDetailTabs_NeverRunsInThePipelinesView(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	app := reviewApp(t, fake) // the panel shows a pull request

	// The pipelines view has no pull request by definition, whatever the
	// panel happens to be showing; the strict fake fails on any fetch.
	app.view = viewPipelines
	testkit.AssertEqual(t, "prefetch", app.prefetchDetailTabs() == nil, true)
	assertReviewRequests(t, fake, "in the pipelines view", 0)
}
