package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// drilldownApp has one pull request selected and one build status under the
// Builds panel cursor.
func drilldownApp(t *testing.T, fake *bitbuckettest.FakeClient) *App {
	t.Helper()
	app := sizedApp(t, fake)
	pr := withAuthor(newPR(testRepo, 101, testTitle), testUserUUID)
	app.merged[testFetchKey] = []bitbucket.PullRequest{pr}
	app.prList.SetPullRequests(app.merged[testFetchKey])
	app.previewKey = pr.Key()
	app.detailView.SetPullRequest(&pr)
	app.buildsPanel.SetPullRequest(pr.Key())
	app.buildsPanel.SetStatuses(pr.Key(), []bitbucket.CommitStatus{
		newStatus(testRepo, 412, bitbucket.StatusSuccessful),
	})
	app.leftFocus = focusBuilds
	app.updateFocusState()
	return app
}

func stubPipeline(fake *bitbuckettest.FakeClient) {
	fake.GetPipelineFunc = func(context.Context, string, string, string) (*bitbucket.Pipeline, error) {
		return &bitbucket.Pipeline{
			UUID:              "{run}",
			BuildNumber:       412,
			DurationInSeconds: 586,
			State: bitbucket.PipelineState{
				Name:   "COMPLETED",
				Result: &bitbucket.PipelineStateResult{Name: "SUCCESSFUL"},
			},
		}, nil
	}
	fake.ListPipelineStepsFunc = func(context.Context, string, string, string) ([]bitbucket.PipelineStep, error) {
		return []bitbucket.PipelineStep{
			{UUID: "{step-1}", Name: "Build", State: bitbucket.PipelineStepState{
				Name: "COMPLETED", Result: &bitbucket.PipelineStepResult{Name: "SUCCESSFUL"},
			}},
			{UUID: "{step-2}", Name: "Test", State: bitbucket.PipelineStepState{
				Name: "COMPLETED", Result: &bitbucket.PipelineStepResult{Name: "FAILED"},
			}},
		}, nil
	}
}

func TestOpenBuild_LoadsThePipelineAndShowsItsSteps(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipeline(fake)
	app := drilldownApp(t, fake)

	_, cmd := app.handleActivate()

	testkit.AssertEqual(t, "side", app.side, sideRight)
	testkit.AssertEqual(t, "mode while loading", app.detailView.Mode(), views.ModePipeline)
	if !containsPlain(app.detailView.View(), "Loading pipeline #412") {
		t.Errorf("detail = %q, want the loading placeholder", plain(app.detailView.View()))
	}

	runAll(t, app, cmd)

	rendered := plain(app.detailView.View())
	if !strings.Contains(rendered, "Pipeline #412") {
		t.Errorf("detail = %q, want the pipeline header", rendered)
	}
	if !strings.Contains(rendered, "Build") || !strings.Contains(rendered, "Test") {
		t.Errorf("detail = %q, want both steps", rendered)
	}
	if _, ok := app.pipelineCache["acme/api-gateway#412"]; !ok {
		t.Error("the run should have been cached by repository and build number")
	}
}

func TestOpenBuild_ACachedRunCostsNoRequest(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	app := drilldownApp(t, fake)
	app.pipelineCache["acme/api-gateway#412"] = &pipelineEntry{
		pipeline: &bitbucket.Pipeline{UUID: "{run}", BuildNumber: 412},
		steps:    []bitbucket.PipelineStep{{UUID: "{step-1}", Name: "Build"}},
	}

	_, cmd := app.handleActivate()

	if cmd != nil {
		t.Error("a cached pipeline run must not cost a request")
	}
	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModePipeline)
}

func TestOpenBuild_WithoutABuildNumberFallsBackToTheBrowser(t *testing.T) {
	t.Parallel()
	app := drilldownApp(t, &bitbuckettest.FakeClient{T: t})
	opened, _ := captureExternal(t, app)
	status := newStatus(testRepo, 412, bitbucket.StatusSuccessful)
	status.URL = "https://example.test/some/other/page"

	_, cmd := app.openBuild(&status)

	if cmd != nil {
		t.Error("a status without a build number must not fetch a pipeline")
	}
	testkit.AssertEqual(t, "opened url", *opened, status.URL)
}

func TestOpenBuild_NilStatusIsANoop(t *testing.T) {
	t.Parallel()
	app := drilldownApp(t, &bitbuckettest.FakeClient{T: t})

	_, cmd := app.openBuild(nil)

	if cmd != nil {
		t.Error("there is nothing to open")
	}
}

func TestPipelineError_IsShownInPlace(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.GetPipelineFunc = func(context.Context, string, string, string) (*bitbucket.Pipeline, error) {
		return nil, errors.New(testBoom)
	}
	app := drilldownApp(t, fake)

	_, cmd := app.handleActivate()
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModePipeline)
	testkit.AssertEqual(t, "status error", app.statusPanel.ErrorMessage(), testBoom)
	if !containsPlain(app.detailView.View(), testBoom) {
		t.Errorf("detail = %q, want the failure explained in place", plain(app.detailView.View()))
	}
}

func TestStepDrilldown_EnterOnAStepLoadsItsLogThenBackWalksOut(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipeline(fake)
	fake.GetPipelineStepLogFunc = func(_ context.Context, _, _, _, stepUUID string,
		_ bitbucket.StepLogOptions,
	) (string, error) {
		testkit.AssertEqual(t, "step uuid", stepUUID, "{step-1}")
		return "+ go build ./...\nok", nil
	}
	app := drilldownApp(t, fake)

	_, cmd := app.handleActivate()
	runAll(t, app, cmd)

	// The detail view fills its blocks during View, so render before pressing
	// Enter on a step.
	_ = app.detailView.View()
	app.side = sideRight
	app.updateFocusState()

	_, cmd = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModeStepLog)
	if !containsPlain(app.detailView.View(), "go build") {
		t.Errorf("detail = %q, want the step log", plain(app.detailView.View()))
	}

	_, _ = app.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	testkit.AssertEqual(t, "back to the run", app.detailView.Mode(), views.ModePipeline)
	testkit.AssertEqual(t, "still on the right", app.side, sideRight)

	_, _ = app.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	testkit.AssertEqual(t, "back to the pull request", app.detailView.Mode(), views.ModePullRequest)
	testkit.AssertEqual(t, "still on the right", app.side, sideRight)
}

// TestBackgroundRefresh_KeepsTheDrilldown pins the rule that a list refresh
// re-previewing the same pull request must not throw the user out of the
// pipeline or step log they are reading — the auto-refresh tick lands roughly
// every two minutes by default and used to reset the panel to the overview.
func TestBackgroundRefresh_KeepsTheDrilldown(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipeline(fake)
	fake.GetPipelineStepLogFunc = func(_ context.Context, _, _, _, _ string,
		_ bitbucket.StepLogOptions,
	) (string, error) {
		return "+ go build ./...\nok", nil
	}
	app := drilldownApp(t, fake)
	// Cached statuses and review data, so a re-preview costs no request.
	app.statusCache[testPRKey] = statusEntry{
		statuses:  []bitbucket.CommitStatus{newStatus(testRepo, 412, bitbucket.StatusSuccessful)},
		updatedOn: testNow,
		fetchedAt: testNow,
	}

	_, cmd := app.handleActivate()
	runAll(t, app, cmd)
	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModePipeline)

	refresh := func() {
		t.Helper()
		_, cmd := app.Update(loadedMsg(bitbucket.RepoPullRequests{
			Repo: testRepo, PullRequests: app.merged[testFetchKey],
		}))
		runAll(t, app, cmd)
	}

	refresh()
	testkit.AssertEqual(t, "pipeline survives a refresh", app.detailView.Mode(), views.ModePipeline)

	// Drill one level deeper and refresh again.
	_ = app.detailView.View()
	app.side = sideRight
	app.updateFocusState()
	_, cmd = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	runAll(t, app, cmd)
	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModeStepLog)

	refresh()
	testkit.AssertEqual(t, "step log survives a refresh", app.detailView.Mode(), views.ModeStepLog)
}

// TestPreviewSelected_FromTheRepoScreenReturnsToThePullRequest is the converse:
// the panel parked on a repository must still be re-pointed at the pull
// request, even though the preview key never changed.
func TestPreviewSelected_FromTheRepoScreenReturnsToThePullRequest(t *testing.T) {
	t.Parallel()
	app := drilldownApp(t, &bitbuckettest.FakeClient{T: t})
	// Cached statuses and review data, so a re-preview costs no request.
	app.statusCache[testPRKey] = statusEntry{
		statuses:  []bitbucket.CommitStatus{newStatus(testRepo, 412, bitbucket.StatusSuccessful)},
		updatedOn: testNow,
		fetchedAt: testNow,
	}
	app.repoList.SetRepos([]bitbucket.Repository{{
		Slug: testRepo, Name: testRepo, FullName: testWorkspace + "/" + testRepo,
	}})
	app.repoList.Cursor = 1
	app.detailView.SetRepo(app.repoList.SelectedRepo())
	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModeRepo)

	runAll(t, app, app.previewSelected())

	testkit.AssertEqual(t, "back on the pull request", app.detailView.Mode(), views.ModePullRequest)
}

func TestFocusLeft_MovesOnlyOnceTheDrilldownIsExhausted(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipeline(fake)
	app := drilldownApp(t, fake)
	_, cmd := app.handleActivate()
	runAll(t, app, cmd)

	_, _ = app.Update(runeKey('h'))
	testkit.AssertEqual(t, "back to the pull request", app.detailView.Mode(), views.ModePullRequest)
	testkit.AssertEqual(t, "focus kept on the right", app.side, sideRight)

	_, _ = app.Update(runeKey('h'))
	testkit.AssertEqual(t, "now focus moves left", app.side, sideLeft)
}

func TestBrowserURL_InThePipelineViewOpensTheRun(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.GetPipelineFunc = func(context.Context, string, string, string) (*bitbucket.Pipeline, error) {
		return &bitbucket.Pipeline{
			UUID:        "{run}",
			BuildNumber: 412,
			Repository:  &bitbucket.RepositoryRef{FullName: testWorkspace + "/" + testRepo},
		}, nil
	}
	fake.ListPipelineStepsFunc = func(context.Context, string, string, string) ([]bitbucket.PipelineStep, error) {
		return nil, nil
	}
	app := drilldownApp(t, fake)
	_, cmd := app.handleActivate()
	runAll(t, app, cmd)

	got := app.browserURL()

	if !strings.Contains(got, "pipelines/results/412") {
		t.Errorf("browser URL = %q, want the pipeline run", got)
	}
}
