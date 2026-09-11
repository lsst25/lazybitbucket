package tui

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// loadedPipelinesApp is the pipelines view with its list fetched: the newest
// run (web-app's) under the cursor.
func loadedPipelinesApp(t *testing.T, fake *bitbuckettest.FakeClient) *App {
	t.Helper()
	app := pipelinesApp(t, fake)
	runAll(t, app, app.fetchPipelines())
	return app
}

func TestPipelinesList_MergesEveryRepositoryNewestFirst(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipelineList(fake)
	app := loadedPipelinesApp(t, fake)

	testkit.AssertEqual(t, "one run per repository", len(app.pipelineEntries), 2)
	testkit.AssertEqual(t, "newest first", app.pipelineEntries[0].Repo, testRepo2)
	testkit.AssertEqual(t, "requests", fake.CallCount("ListPipelines"), 2)
	testkit.AssertEqual(t, "runs per repository", fake.ListPipelinesCalls[0].Opts.MaxItems, pipelinesPerRepo)

	out := plain(app.pipelineList.View())
	if !strings.Contains(out, testRepo) || !strings.Contains(out, testRepo2) {
		t.Errorf("list = %q, want a row from each repository", out)
	}
}

func TestPipelinesList_APartialFailureStillListsWhatCameBack(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipelineList(fake)
	fake.ListPipelinesFunc = func(
		_ context.Context, _, repoSlug string, _ bitbucket.ListPipelinesOptions,
	) ([]bitbucket.Pipeline, error) {
		if repoSlug == testRepo2 {
			return nil, errors.New(testBoom)
		}
		return []bitbucket.Pipeline{newPipelineRun(testRepo, testBuild, 101, 0)}, nil
	}
	app := loadedPipelinesApp(t, fake)

	testkit.AssertEqual(t, "the repository that answered is listed", len(app.pipelineEntries), 1)
	if got := plain(app.helpBar.View()); !strings.Contains(got, "1 of 2 repositories could not be listed") {
		t.Errorf("help bar = %q, want the partial-failure note", got)
	}
}

func TestPipelinesList_EveryRepositoryFailingShowsTheFirstError(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.ListPipelinesFunc = func(
		context.Context, string, string, bitbucket.ListPipelinesOptions,
	) ([]bitbucket.Pipeline, error) {
		return nil, errors.New(testBoom)
	}
	app := loadedPipelinesApp(t, fake)

	if got := plain(app.pipelineList.View()); !strings.Contains(got, testBoom) {
		t.Errorf("list = %q, want the failure in place of the rows", got)
	}
}

func TestPipelinesList_TheRefreshKeyRefetchesAndTheTickDoesNot(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipelineList(fake)
	fake.ListPullRequestsFunc = func(
		context.Context, string, string, bitbucket.ListPullRequestsOptions,
	) ([]bitbucket.PullRequest, error) {
		return nil, nil
	}
	app := loadedPipelinesApp(t, fake)
	app.refreshEvery = 0

	_, cmd := app.Update(runeKey('R'))
	runAll(t, app, cmd)
	testkit.AssertEqual(t, "R refetches the list", fake.CallCount("ListPipelines"), 4)

	// The tick refreshes the pull request list behind the view, never the
	// pipelines list: it costs a round of requests per repository. It is the
	// command the tick batches that is run here — handleAutoFetch itself
	// returns before it with the interval switched off, which would make this
	// assertion true whatever refreshCmd did.
	runAll(t, app, app.refreshCmd())
	testkit.AssertEqual(t, "the tick left the list alone", fake.CallCount("ListPipelines"), 4)
	if fake.CallCount("ListPullRequests") == 0 {
		t.Error("the tick must still refresh the pull request list behind the view")
	}
}

// R has to reach repository discovery too: without repoSlugs the pipelines
// fetch is a silent no-op, which would leave the refresh key doing nothing at
// all in the view the user is looking at.
func TestPipelinesList_TheRefreshKeyRetriesRepositoryDiscovery(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipelineList(fake)
	fake.ListRepositoriesFunc = func(
		context.Context, string, bitbucket.ListRepositoriesOptions,
	) ([]bitbucket.Repository, error) {
		return nil, errors.New(testBoom)
	}
	app := pipelinesApp(t, fake)
	app.repoSlugs = nil
	app.repoDiscoveryFailed = true
	app.cfg.Bitbucket.Repositories = nil

	_, cmd := app.Update(runeKey('R'))
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "discovery was retried", fake.CallCount("ListRepositories"), 1)
}

func TestPipelinesSearch_FiltersConfirmsAndCancels(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipelineList(fake)
	app := loadedPipelinesApp(t, fake)

	_, _ = app.handleSearchChanged(components.SearchChangedMsg{Query: testRepo})

	if got := plain(app.pipelineList.View()); strings.Contains(got, testRepo2) {
		t.Errorf("list = %q, want the other repository filtered out", got)
	}

	_, cmd := app.handleSearchConfirmed()
	runAll(t, app, cmd)

	// The filter is dropped and the cursor stays on what the search found, so
	// the next key acts on the run the user was looking at.
	if got := plain(app.pipelineList.View()); !strings.Contains(got, testRepo2) {
		t.Errorf("list = %q, want every row back", got)
	}
	testkit.AssertEqual(t, "the searched run is still selected", app.runKey, testRunKey)

	_, _ = app.handleSearchChanged(components.SearchChangedMsg{Query: testRepo})
	_, _ = app.handleSearchCancelled()
	if got := plain(app.pipelineList.View()); !strings.Contains(got, testRepo2) {
		t.Errorf("list = %q, want the cancelled filter cleared", got)
	}
}

func TestPipelineSelection_ShowsTheRunAtOnceAndFetchesItsSteps(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipelineList(fake)
	app := loadedPipelinesApp(t, fake)

	testkit.AssertEqual(t, "the newest run is shown", app.runKey, testOtherRunKey)
	testkit.AssertEqual(t, "the detail panel owns it", app.detailView.PipelineOwner(), testOtherRunKey)
	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModePipeline)
	testkit.AssertEqual(t, "its steps were fetched", fake.CallCount("ListPipelineSteps"), 1)

	_, cmd := app.Update(runeKey('j'))
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "the second run is shown", app.runKey, testRunKey)
	testkit.AssertEqual(t, "its steps were fetched too", fake.CallCount("ListPipelineSteps"), 2)

	// Walking back onto a run costs nothing: its steps are cached.
	_, cmd = app.Update(runeKey('k'))
	runAll(t, app, cmd)
	testkit.AssertEqual(t, "the cached run needs no request", fake.CallCount("ListPipelineSteps"), 2)
}

func TestPipelineSteps_AStaleResponseIsDropped(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipelineList(fake)
	app := loadedPipelinesApp(t, fake)

	// A response for the run the cursor has since left, and one from a fetch a
	// refetch superseded.
	_, _ = app.Update(pipelineStepsLoadedMsg{
		runKey: testRunKey, steps: nil, err: errors.New(testBoom), epoch: app.pipelinesEpoch,
	})
	if got := plain(app.detailView.View()); strings.Contains(got, testBoom) {
		t.Error("a response for another run must not reach the panel")
	}
	_, _ = app.Update(pipelineStepsLoadedMsg{
		runKey: app.runKey, steps: nil, err: errors.New(testBoom), epoch: app.pipelinesEpoch - 1,
	})
	if got := plain(app.detailView.View()); strings.Contains(got, testBoom) {
		t.Error("a response from a superseded fetch must not reach the panel")
	}

	// The panel drops a mismatched owner on its own, so the cache is what shows
	// whether the app-level guard is doing its job.
	_, _ = app.Update(pipelineStepsLoadedMsg{
		runKey: testRunKey, steps: []bitbucket.PipelineStep{{Name: "Build"}}, epoch: app.pipelinesEpoch,
	})
	if _, cached := app.pipelineCache[testRunKey]; cached {
		t.Error("another run's steps must not be cached")
	}
}

// A run the list has not caught up with — the one a rerun has just created —
// still caches its steps, from the run the panel is showing. Without that the
// list's arrival would replace the steps with the loading line and fetch them a
// second time.
func TestPipelineSteps_ARunTheListDoesNotHoldYetIsStillCached(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipelineList(fake)
	app := loadedPipelinesApp(t, fake)

	created := newPipelineRun(testRepo, testNewBuild, 101, 0)
	key := pipelineRef{workspace: testWorkspace, slug: testRepo, build: testNewBuild}.key()
	app.runKey = key
	app.detailView.SetRun(key, &created, nil)

	_, _ = app.Update(pipelineStepsLoadedMsg{
		runKey: key, steps: []bitbucket.PipelineStep{{Name: "Build"}}, epoch: app.pipelinesEpoch,
	})

	entry, cached := app.pipelineCache[key]
	if !cached || entry == nil || len(entry.steps) != 1 {
		t.Fatalf("pipelineCache[%q] = %+v, want the steps of the run on screen", key, entry)
	}
	testkit.AssertEqual(t, "the cached run", entry.pipeline.BuildNumber, testNewBuild)
}

// The cursor and the run on screen move together even out of a step log: a
// reader who walks back to the list and onto another run gets that run, and the
// keys that act on it act on the one they can see.
func TestPipelineSelection_MovingOffAStepLogRepointsThePanel(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipelineList(fake)
	_ = answerRuns(fake, map[string]*bitbucket.Pipeline{
		strconv.Itoa(testBuild): completedRun(testBuild),
	})
	fake.GetPipelineStepLogFunc = func(
		context.Context, string, string, string, string, bitbucket.StepLogOptions,
	) (string, error) {
		return testStepLog, nil
	}
	app := loadedPipelinesApp(t, fake)

	// Into the newest run, then into one of its steps.
	_, cmd := app.handleActivate()
	runAll(t, app, cmd)
	_ = app.detailView.View()
	_, cmd = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	runAll(t, app, cmd)
	testkit.AssertEqual(t, "in the step log", app.detailView.Mode(), views.ModeStepLog)

	// A refetch landing under the reader leaves the log alone.
	runAll(t, app, app.fetchPipelines())
	testkit.AssertEqual(t, "the log survived the refetch", app.detailView.Mode(), views.ModeStepLog)

	// Back to the list and onto the other run: that is a move, not a refetch.
	_, cmd = app.Update(runeKey('2'))
	runAll(t, app, cmd)
	_, cmd = app.Update(runeKey('j'))
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "the cursor moved", app.runKey, testRunKey)
	testkit.AssertEqual(t, "the panel followed it", app.detailView.PipelineOwner(), testRunKey)
	testkit.AssertEqual(t, "and left the log", app.detailView.Mode(), views.ModePipeline)

	// The write keys act on the run under the cursor rather than being dropped
	// against a run the panel has left.
	pressKey(t, app, 'P')
	if got := plain(app.form.View()); !strings.Contains(got, "Rerun pipeline #"+strconv.Itoa(testBuild)+"?") {
		t.Errorf("dialog = %q, want the rerun question for the selected run", got)
	}
}

// A pull request list refresh runs behind the pipelines view: it must not take
// the run off the shared detail panel.
func TestPipelinesView_APullRequestRefreshBehindItLeavesTheRunOnScreen(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipelineList(fake)
	app := loadedPipelinesApp(t, fake)
	owner := app.detailView.PipelineOwner()

	pr := withAuthor(newPR(testRepo, 101, testTitle), testUserUUID)
	app.previewKey = pr.Key()
	_, _ = app.Update(pullRequestDetailLoadedMsg{pr: &pr, epoch: app.previewEpoch})

	testkit.AssertEqual(t, "the run is still the owner", app.detailView.PipelineOwner(), owner)
	testkit.AssertEqual(t, "and still on screen", app.detailView.Mode(), views.ModePipeline)
	if got := plain(app.detailView.View()); !strings.Contains(got, "Pipeline #") {
		t.Errorf("detail = %q, want the run rather than the pull request", got)
	}
}

// A rerun started in the pipelines view still refreshes that view's list when
// its answer arrives after the user has switched away, and moves nothing on the
// screen they are now looking at.
func TestPipelineRerun_AnAnswerAfterAViewSwitchStillRefetchesTheList(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipelineList(fake)
	app := loadedPipelinesApp(t, fake)
	before := fake.CallCount("ListPipelines")

	app.view = viewPullRequests
	app.applyView()
	created := newPipelineRun(testRepo, testNewBuild, 101, 0)
	_, cmd := app.Update(pipelineWrittenMsg{
		ref:      pipelineRef{workspace: testWorkspace, slug: testRepo, build: testBuild},
		prKey:    "",
		verb:     verbRerunPipeline,
		pipeline: &created,
	})
	runAll(t, app, cmd)

	if fake.CallCount("ListPipelines") <= before {
		t.Error("the pipelines list must be refetched for the view the rerun was started in")
	}
	testkit.AssertEqual(t, "the pull request view was not hijacked", app.side, sideLeft)
}

func TestPipelineSteps_TheRefreshKeyReadsThemAgain(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipelineList(fake)
	app := loadedPipelinesApp(t, fake)

	_, cmd := app.Update(runeKey('r'))
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "steps refetched", fake.CallCount("ListPipelineSteps"), 2)
	testkit.AssertEqual(t, "the list was left alone", fake.CallCount("ListPipelines"), 2)
}

func TestPipelineEnter_FocusesTheDetailAndOpensAStepLog(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipelineList(fake)
	fake.GetPipelineStepLogFunc = func(
		_ context.Context, workspace, slug, _, _ string, _ bitbucket.StepLogOptions,
	) (string, error) {
		testkit.AssertEqual(t, "workspace", workspace, testWorkspace)
		testkit.AssertEqual(t, "the log comes from the run's own repository", slug, testRepo2)
		return testStepLog, nil
	}
	app := loadedPipelinesApp(t, fake)

	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	runAll(t, app, cmd)
	testkit.AssertEqual(t, "side", app.side, sideRight)
	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModePipeline)

	// The detail view fills its step blocks during View, so render before
	// activating one.
	_ = app.detailView.View()
	_, cmd = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModeStepLog)
	if got := plain(app.detailView.View()); !strings.Contains(got, "go build") {
		t.Errorf("detail = %q, want the step log", got)
	}
}

func TestPipelineOpenPullRequest_ItsThreeOutcomes(t *testing.T) {
	t.Parallel()

	t.Run("the pull request is on the tab in view", func(t *testing.T) {
		t.Parallel()
		fake := &bitbuckettest.FakeClient{T: t}
		stubPipelineList(fake)
		fake.ListPullRequestStatusesFunc = func(
			context.Context, string, string, int,
		) ([]bitbucket.CommitStatus, error) {
			return nil, nil
		}
		app := loadedPipelinesApp(t, fake)
		pr := withAuthor(newPR(testRepo, 101, testTitle), testUserUUID)
		app.merged[testFetchKey] = []bitbucket.PullRequest{pr}
		app.prList.SetPullRequests(app.merged[testFetchKey])
		app.prCache[pr.Key()] = &pr
		// The cursor is on web-app's run; step onto api-gateway's, which ran
		// for pull request 101.
		_, cmd := app.Update(runeKey('j'))
		runAll(t, app, cmd)

		_, cmd = app.Update(runeKey('L'))
		runAll(t, app, cmd)

		testkit.AssertEqual(t, "view", app.view, viewPullRequests)
		testkit.AssertEqual(t, "focus", app.leftFocus, focusPullRequests)
		testkit.AssertEqual(t, "selection", app.prList.SelectedPullRequest().Key(), testPRKey)
	})

	t.Run("the pull request is not loaded", func(t *testing.T) {
		t.Parallel()
		fake := &bitbuckettest.FakeClient{T: t}
		stubPipelineList(fake)
		app := loadedPipelinesApp(t, fake)

		_, cmd := app.Update(runeKey('L'))
		runAll(t, app, cmd)

		testkit.AssertEqual(t, "the view did not move", app.view, viewPipelines)
		if got := plain(app.helpBar.View()); !strings.Contains(got, "pull request #201 is not in this tab") {
			t.Errorf("help bar = %q, want the not-in-this-tab refusal", got)
		}
	})

	t.Run("the run is not a pull request build", func(t *testing.T) {
		t.Parallel()
		fake := &bitbuckettest.FakeClient{T: t}
		stubPipelineList(fake)
		fake.ListPipelinesFunc = func(
			_ context.Context, _, repoSlug string, _ bitbucket.ListPipelinesOptions,
		) ([]bitbucket.Pipeline, error) {
			if repoSlug != testRepo {
				return nil, nil
			}
			run := newPipelineRun(testRepo, testBuild, 101, 0)
			run.Target = bitbucket.PipelineTarget{Type: "pipeline_ref_target", RefType: "branch", RefName: branchMain}
			return []bitbucket.Pipeline{run}, nil
		}
		app := loadedPipelinesApp(t, fake)

		_, cmd := app.Update(runeKey('L'))
		runAll(t, app, cmd)

		testkit.AssertEqual(t, "the view did not move", app.view, viewPipelines)
		if got := plain(app.helpBar.View()); !strings.Contains(got, notAPullRequestBuild) {
			t.Errorf("help bar = %q, want the not-a-pull-request refusal", got)
		}
	})
}

func TestPipelinesList_TheRepositoryFilterRefetchesTheList(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipelineList(fake)
	app := loadedPipelinesApp(t, fake)
	app.repoList.SetRepos([]bitbucket.Repository{{Slug: testRepo, Name: testRepo}})

	runAll(t, app, app.selectRepo(&views.RepoItem{Slug: testRepo, Name: testRepo}))

	testkit.AssertEqual(t, "the filter is set", app.repoFilter, testRepo)
	testkit.AssertEqual(t, "focus stays in this view", app.leftFocus, focusPipelines)
	testkit.AssertEqual(t, "only the filtered repository is listed", fake.CallCount("ListPipelines"), 3)
	testkit.AssertEqual(t, "and it is the one that was picked",
		fake.ListPipelinesCalls[2].RepoSlug, testRepo)
}

func TestPipelinesBrowserURL_IsTheSelectedRun(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipelineList(fake)
	app := loadedPipelinesApp(t, fake)

	want := "https://bitbucket.org/" + testWorkspace + "/" + testRepo2 + "/pipelines/results/" +
		"77"
	testkit.AssertEqual(t, "browserURL", app.browserURL(), want)
}

func TestPipelinesView_ABackgroundPullRequestRefreshLeavesTheRunOnScreen(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipelineList(fake)
	fake.ListPullRequestStatusesFunc = func(
		context.Context, string, string, int,
	) ([]bitbucket.CommitStatus, error) {
		return nil, nil
	}
	app := loadedPipelinesApp(t, fake)
	pr := withAuthor(newPR(testRepo, 101, testTitle), testUserUUID)
	app.merged[testFetchKey] = []bitbucket.PullRequest{pr}
	app.prList.SetPullRequests(app.merged[testFetchKey])
	app.prCache[pr.Key()] = &pr

	// What the auto-refresh tick does behind the pipelines view.
	runAll(t, app, app.previewSelected())

	testkit.AssertEqual(t, "the detail panel still shows the run",
		app.detailView.PipelineOwner(), testOtherRunKey)
	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModePipeline)
}
