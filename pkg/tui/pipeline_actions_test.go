package tui

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// pipelineApp is one open pull request with a single build row under the Builds
// panel cursor: the context both pipeline keys act from.
func pipelineApp(t *testing.T, fake *bitbuckettest.FakeClient, build int) *App {
	t.Helper()
	app := sizedApp(t, fake)
	pr := withAuthor(newPR(testRepo, 101, testTitle), testUserUUID)
	app.merged[testFetchKey] = []bitbucket.PullRequest{pr}
	app.prList.SetPullRequests(app.merged[testFetchKey])
	app.prCache[pr.Key()] = &pr
	app.previewKey = pr.Key()
	app.detailView.SetPullRequest(&pr)
	app.buildsPanel.SetPullRequest(pr.Key())
	app.buildsPanel.SetStatuses(pr.Key(), []bitbucket.CommitStatus{
		newStatus(testRepo, build, bitbucket.StatusInProgress),
	})
	app.leftFocus = focusBuilds
	app.updateFocusState()
	return app
}

// runUUID is the braced uuid of a fixture run, which is what a stop is sent
// with.
func runUUID(build int) string { return "{run-" + strconv.Itoa(build) + "}" }

// newRun is a run as GetPipeline answers with one: a pull request target, so
// the dialog's second row has something to say.
func newRun(build int, state bitbucket.PipelineState) *bitbucket.Pipeline {
	return &bitbucket.Pipeline{
		UUID:        runUUID(build),
		BuildNumber: build,
		State:       state,
		Trigger:     bitbucket.PipelineTrigger{Type: "pipeline_trigger_push", Name: "PUSH"},
		Creator:     &bitbucket.Account{DisplayName: testRunCreator},
		Target: bitbucket.PipelineTarget{
			Type:        "pipeline_pullrequest_target",
			Source:      testBranch,
			Destination: branchMain,
			Commit:      &bitbucket.CommitRef{Hash: testSourceHash},
			PullRequest: &bitbucket.PipelinePullRequestRef{ID: 101},
		},
	}
}

// completedRun is a run there is nothing left to stop and everything to repeat.
func completedRun(build int) *bitbucket.Pipeline {
	return newRun(build, bitbucket.PipelineState{
		Type:   "pipeline_state_completed",
		Name:   "COMPLETED",
		Result: &bitbucket.PipelineStateResult{Type: "pipeline_state_completed_failed", Name: "FAILED"},
	})
}

// runningRun is the converse: it can be stopped and must not be repeated.
func runningRun(build int) *bitbucket.Pipeline {
	return newRun(build, bitbucket.PipelineState{
		Type:  "pipeline_state_in_progress",
		Name:  "IN_PROGRESS",
		Stage: &bitbucket.PipelineStateStage{Type: "pipeline_state_in_progress_running", Name: "RUNNING"},
	})
}

// answerRuns makes the fake answer GetPipeline from a build-number-keyed set and
// records every ref it was asked for, which is how the tests tell the read
// before the dialog from the refetch after the write.
func answerRuns(fake *bitbuckettest.FakeClient, runs map[string]*bitbucket.Pipeline) *[]string {
	asked := make([]string, 0, 2)
	fake.GetPipelineFunc = func(_ context.Context, _, _, ref string) (*bitbucket.Pipeline, error) {
		asked = append(asked, ref)
		return runs[ref], nil
	}
	return &asked
}

// stubPipelineReads answers the two requests a drill-down into a run makes and
// the status refetch every pipeline write ends with.
func stubPipelineReads(fake *bitbuckettest.FakeClient) {
	fake.ListPipelineStepsFunc = func(context.Context, string, string, string) ([]bitbucket.PipelineStep, error) {
		return []bitbucket.PipelineStep{{UUID: "{step-1}", Name: "Build"}}, nil
	}
	fake.ListPullRequestStatusesFunc = func(
		context.Context, string, string, int,
	) ([]bitbucket.CommitStatus, error) {
		return []bitbucket.CommitStatus{newStatus(testRepo, testNewBuild, bitbucket.StatusInProgress)}, nil
	}
}

// pressKey sends a key and drains everything it fans out into.
func pressKey(t *testing.T, app *App, key rune) {
	t.Helper()
	_, cmd := app.Update(runeKey(key))
	runAll(t, app, cmd)
}

// -----------------------------------------------------------------------------
// Where the keys work, and where they only say where to press them
// -----------------------------------------------------------------------------

func TestPipelineKeys_OutsideARunTheyOnlyPointAtTheBuildsPanel(t *testing.T) {
	t.Parallel()

	for _, key := range []rune{'P', 'X'} {
		t.Run(string(key), func(t *testing.T) {
			t.Parallel()
			fake := &bitbuckettest.FakeClient{T: t}
			app := writeApp(t, fake)

			_, cmd := app.Update(runeKey(key))

			if cmd != nil {
				t.Error("the pull request list has no run to act on, so nothing is fetched")
			}
			if app.form.IsVisible() {
				t.Error("no dialog belongs on a key pressed where it does not work")
			}
			if got := plain(app.helpBar.View()); !strings.Contains(got,
				"select a build in the Builds panel (3) first") {
				t.Errorf("help bar = %q, want the hint naming the Builds panel", got)
			}
			testkit.AssertEqual(t, "pipeline reads", fake.CallCount("GetPipeline"), 0)
		})
	}
}

func TestPipelineKeys_AThirdPartyBuildIsNotARun(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	app := pipelineApp(t, fake, testBuild)
	// A Jenkins or GitHub Actions reporter posts its own URL, and there is no
	// Bitbucket run behind it.
	status := newStatus(testRepo, testBuild, bitbucket.StatusInProgress)
	status.URL = "https://example.test/job/api-gateway/17"
	app.buildsPanel.SetStatuses(testPRKey, []bitbucket.CommitStatus{status})

	_, cmd := app.Update(runeKey('P'))

	if cmd != nil {
		t.Error("a build without a run must not be read")
	}
	if got := plain(app.helpBar.View()); !strings.Contains(got, notAPipelineBuild) {
		t.Errorf("help bar = %q, want the not-a-pipeline refusal", got)
	}
}

func TestPipelineKeys_WorkFromTheDetailPanelToo(t *testing.T) {
	t.Parallel()

	t.Run("rerun from the pipeline view", func(t *testing.T) {
		t.Parallel()
		fake := &bitbuckettest.FakeClient{T: t}
		_ = answerRuns(fake, map[string]*bitbucket.Pipeline{"412": completedRun(testBuild)})
		stubPipelineReads(fake)
		app := pipelineApp(t, fake, testBuild)

		_, cmd := app.handleActivate()
		runAll(t, app, cmd)
		testkit.AssertEqual(t, "drilled in", app.detailView.Mode(), views.ModePipeline)
		testkit.AssertEqual(t, "focus", app.side, sideRight)

		pressKey(t, app, 'P')

		if got := plain(app.form.View()); !strings.Contains(got, "Rerun pipeline #412?") {
			t.Errorf("dialog = %q, want the rerun question", got)
		}
	})

	t.Run("stop from the step log", func(t *testing.T) {
		t.Parallel()
		fake := &bitbuckettest.FakeClient{T: t}
		_ = answerRuns(fake, map[string]*bitbucket.Pipeline{"413": runningRun(testNewBuild)})
		stubPipelineReads(fake)
		fake.GetPipelineStepLogFunc = func(
			context.Context, string, string, string, string, bitbucket.StepLogOptions,
		) (string, error) {
			return testStepLog, nil
		}
		app := pipelineApp(t, fake, testNewBuild)

		_, cmd := app.handleActivate()
		runAll(t, app, cmd)
		// The detail view fills its step blocks during View, so render before
		// activating one.
		_ = app.detailView.View()
		_, cmd = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
		runAll(t, app, cmd)
		testkit.AssertEqual(t, "in the step log", app.detailView.Mode(), views.ModeStepLog)

		pressKey(t, app, 'X')

		if got := plain(app.form.View()); !strings.Contains(got, "Stop pipeline #413?") {
			t.Errorf("dialog = %q, want the stop question", got)
		}
	})
}

// -----------------------------------------------------------------------------
// Rerun
// -----------------------------------------------------------------------------

func TestRerunPipeline_ReadsTheRunAndAsksBeforeStartingIt(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	asked := answerRuns(fake, map[string]*bitbucket.Pipeline{"412": completedRun(testBuild)})
	app := pipelineApp(t, fake, testBuild)

	_, cmd := app.Update(runeKey('P'))
	if app.form.IsVisible() {
		t.Fatal("a dialog must not open before the run's state is known")
	}
	if got := plain(app.helpBar.View()); !strings.Contains(got, "checking pipeline #412…") {
		t.Errorf("help bar = %q, want the reading line", got)
	}
	runAll(t, app, cmd)

	testkit.AssertSliceEqual(t, "run read", *asked, []string{"412"})
	testkit.AssertEqual(t, "form id", app.form.ID(), formRerun)
	rendered := plain(app.form.View())
	for _, want := range []string{
		"Rerun pipeline #412?", "FAILED · PUSH · by " + testRunCreator,
		"PR #101 " + testBranch + " → " + branchMain,
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("dialog is missing %q:\n%s", want, rendered)
		}
	}

	_, cmd = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	runAll(t, app, cmd)

	if app.form.IsVisible() {
		t.Error("esc should close the dialog")
	}
	testkit.AssertEqual(t, "run calls", fake.CallCount("RunPipeline"), 0)
}

func TestRerunPipeline_ConfirmStartsTheRunAndOpensIt(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	original := completedRun(testBuild)
	asked := answerRuns(fake, map[string]*bitbucket.Pipeline{
		"412": original,
		"413": runningRun(testNewBuild),
	})
	stubPipelineReads(fake)
	fake.RunPipelineFunc = func(
		context.Context, string, string, bitbucket.PipelineTarget,
	) (*bitbucket.Pipeline, error) {
		return newRun(testNewBuild, bitbucket.PipelineState{Type: "pipeline_state_pending", Name: "PENDING"}), nil
	}
	app := pipelineApp(t, fake, testBuild)

	pressKey(t, app, 'P')
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "run calls", fake.CallCount("RunPipeline"), 1)
	call := fake.RunPipelineCalls[0]
	testkit.AssertEqual(t, "workspace", call.Workspace, testWorkspace)
	testkit.AssertEqual(t, "repository", call.RepoSlug, testRepo)
	// The rerun repeats the original run's target — its commit, its selector and
	// its pull request — rather than whatever the branch points at now.
	testkit.AssertEqual(t, "target", call.Target, original.Target)

	if got := plain(app.helpBar.View()); !strings.Contains(got, "pipeline #413 started (rerun of #412)") {
		t.Errorf("help bar = %q, want the started line", got)
	}
	// The new run is what the user pressed the key to watch, so the detail
	// panel drills into it and the build rows are refetched.
	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModePipeline)
	testkit.AssertEqual(t, "run in view", app.detailView.PipelineBuild(), testNewBuild)
	testkit.AssertEqual(t, "focus", app.side, sideRight)
	testkit.AssertSliceEqual(t, "runs read", *asked, []string{"412", "413"})
	testkit.AssertEqual(t, "statuses refetched", fake.CallCount("ListPullRequestStatuses"), 1)
	if _, busy := app.pipelineWrites[pipelineCacheKey(testPRKey, testBuild)]; busy {
		t.Error("the in-flight record must clear once the write has answered")
	}
}

func TestRerunPipeline_RefusesARunThatIsStillGoing(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	_ = answerRuns(fake, map[string]*bitbucket.Pipeline{"412": runningRun(testBuild)})
	app := pipelineApp(t, fake, testBuild)

	pressKey(t, app, 'P')

	if app.form.IsVisible() {
		t.Error("a running pipeline must not be offered a rerun")
	}
	if got := plain(app.helpBar.View()); !strings.Contains(got,
		"cannot rerun: pipeline #412 is still running") {
		t.Errorf("help bar = %q, want the still-running refusal", got)
	}
	testkit.AssertEqual(t, "run calls", fake.CallCount("RunPipeline"), 0)
}

// -----------------------------------------------------------------------------
// Stop
// -----------------------------------------------------------------------------

func TestStopPipeline_ConfirmSignalsTheRunAndDropsWhatItStaled(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	_ = answerRuns(fake, map[string]*bitbucket.Pipeline{"413": runningRun(testNewBuild)})
	stubPipelineReads(fake)
	fake.StopPipelineFunc = func(context.Context, string, string, string) error { return nil }
	app := pipelineApp(t, fake, testNewBuild)
	app.pipelineCache[pipelineCacheKey(testPRKey, testNewBuild)] = &pipelineEntry{
		pipeline: runningRun(testNewBuild), steps: nil,
	}

	pressKey(t, app, 'X')
	testkit.AssertEqual(t, "form id", app.form.ID(), formStop)
	if got := plain(app.form.View()); !strings.Contains(got, "Stop pipeline #413?") {
		t.Errorf("dialog = %q, want the stop question", got)
	}

	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "stop calls", fake.CallCount("StopPipeline"), 1)
	call := fake.StopPipelineCalls[0]
	testkit.AssertEqual(t, "workspace", call.Workspace, testWorkspace)
	testkit.AssertEqual(t, "repository", call.RepoSlug, testRepo)
	testkit.AssertEqual(t, "pipeline uuid", call.PipelineUUID, runUUID(testNewBuild))
	if got := plain(app.helpBar.View()); !strings.Contains(got, "stop requested for pipeline #413") {
		t.Errorf("help bar = %q, want the stop-requested line", got)
	}
	// The cached run said RUNNING and the build row said INPROGRESS; neither is
	// true any more, and only the server knows what replaced them.
	if _, cached := app.pipelineCache[pipelineCacheKey(testPRKey, testNewBuild)]; cached {
		t.Error("the stopped run must not stay in the pipeline cache")
	}
	testkit.AssertEqual(t, "statuses refetched", fake.CallCount("ListPullRequestStatuses"), 1)
}

// A stop confirmed inside the run's own view reads the run back, so the header
// flips to STOPPED — and does so without losing the user's place in the steps.
func TestStopPipeline_RefetchesTheRunTheUserIsWatching(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	ref := strconv.Itoa(testNewBuild)
	asked := answerRuns(fake, map[string]*bitbucket.Pipeline{ref: runningRun(testNewBuild)})
	stubPipelineReads(fake)
	fake.ListPipelineStepsFunc = func(context.Context, string, string, string) ([]bitbucket.PipelineStep, error) {
		return []bitbucket.PipelineStep{
			{UUID: "{step-1}", Name: "Build"}, {UUID: "{step-2}", Name: "Test"}, {UUID: "{step-3}", Name: "Publish"},
		}, nil
	}
	fake.StopPipelineFunc = func(context.Context, string, string, string) error { return nil }
	app := pipelineApp(t, fake, testNewBuild)

	_, cmd := app.handleActivate()
	runAll(t, app, cmd)
	// The detail view fills its step blocks during View, so render before
	// moving the cursor.
	_ = app.detailView.View()
	for range 2 {
		_, _ = app.Update(runeKey('j'))
	}
	testkit.AssertEqual(t, "cursor before the stop", app.detailView.SelectedStep().Name, "Publish")

	pressKey(t, app, 'X')
	_, cmd = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "stop calls", fake.CallCount("StopPipeline"), 1)
	// The drill-down, the read before the dialog, and the read-back after the
	// stop — the last one is what shows the run as the server left it.
	testkit.AssertSliceEqual(t, "runs read", *asked, []string{ref, ref, ref})
	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModePipeline)
	testkit.AssertEqual(t, "cursor after the stop", app.detailView.SelectedStep().Name, "Publish")
}

// From a step log the stop leaves the log alone: the run is not read back,
// because showing the refreshed run would replace the log the user is reading.
func TestStopPipeline_FromAStepLogLeavesTheLogOnScreen(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	ref := strconv.Itoa(testNewBuild)
	asked := answerRuns(fake, map[string]*bitbucket.Pipeline{ref: runningRun(testNewBuild)})
	stubPipelineReads(fake)
	fake.GetPipelineStepLogFunc = func(
		context.Context, string, string, string, string, bitbucket.StepLogOptions,
	) (string, error) {
		return testStepLog, nil
	}
	fake.StopPipelineFunc = func(context.Context, string, string, string) error { return nil }
	app := pipelineApp(t, fake, testNewBuild)

	_, cmd := app.handleActivate()
	runAll(t, app, cmd)
	_ = app.detailView.View()
	_, cmd = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	runAll(t, app, cmd)
	testkit.AssertEqual(t, "in the step log", app.detailView.Mode(), views.ModeStepLog)

	pressKey(t, app, 'X')
	_, cmd = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "stop calls", fake.CallCount("StopPipeline"), 1)
	// The drill-down and the read before the dialog, and nothing after the stop.
	testkit.AssertSliceEqual(t, "runs read", *asked, []string{ref, ref})
	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModeStepLog)
	if _, cached := app.pipelineCache[pipelineCacheKey(testPRKey, testNewBuild)]; cached {
		t.Error("the stopped run must not stay in the pipeline cache")
	}
	testkit.AssertEqual(t, "statuses refetched", fake.CallCount("ListPullRequestStatuses"), 1)
}

func TestStopPipeline_RefusesARunThatHasFinished(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	_ = answerRuns(fake, map[string]*bitbucket.Pipeline{"412": completedRun(testBuild)})
	app := pipelineApp(t, fake, testBuild)

	pressKey(t, app, 'X')

	if app.form.IsVisible() {
		t.Error("a completed pipeline has nothing to stop")
	}
	if got := plain(app.helpBar.View()); !strings.Contains(got,
		"cannot stop: pipeline #412 has already completed") {
		t.Errorf("help bar = %q, want the already-completed refusal", got)
	}
	testkit.AssertEqual(t, "stop calls", fake.CallCount("StopPipeline"), 0)
}

// The run can finish between the read and the confirmation, and then the server
// is the one that says so — in its own words.
func TestStopPipeline_ReportsTheServersRefusal(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	_ = answerRuns(fake, map[string]*bitbucket.Pipeline{"413": runningRun(testNewBuild)})
	fake.StopPipelineFunc = func(context.Context, string, string, string) error {
		return apiError(http.StatusBadRequest, "The specified pipeline has already completed.", nil)
	}
	app := pipelineApp(t, fake, testNewBuild)

	pressKey(t, app, 'X')
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	runAll(t, app, cmd)

	if got := plain(app.helpBar.View()); !strings.Contains(got,
		verbStopPipeline+" failed: The specified pipeline has already completed.") {
		t.Errorf("help bar = %q, want the server's own sentence", got)
	}
	if _, busy := app.pipelineWrites[pipelineCacheKey(testPRKey, testNewBuild)]; busy {
		t.Error("a failed stop must clear the in-flight record")
	}
}

// -----------------------------------------------------------------------------
// The guards both keys share
// -----------------------------------------------------------------------------

func TestPipelineKeys_ReadOnlyModeRefusesAndHidesThem(t *testing.T) {
	t.Parallel()

	t.Run("the keys refuse", func(t *testing.T) {
		t.Parallel()
		fake := &bitbuckettest.FakeClient{T: t}
		app := pipelineApp(t, fake, testBuild)
		app.readOnly = true

		for key, want := range map[rune]string{
			'P': "read-only mode: cannot " + verbRerunPipeline,
			'X': "read-only mode: cannot " + verbStopPipeline,
		} {
			_, cmd := app.Update(runeKey(key))
			if cmd != nil {
				t.Errorf("%q must not read the run in read-only mode", string(key))
			}
			if got := plain(app.helpBar.View()); !strings.Contains(got, want) {
				t.Errorf("help bar = %q, want %q", got, want)
			}
		}
		testkit.AssertEqual(t, "pipeline reads", fake.CallCount("GetPipeline"), 0)
	})

	t.Run("the help surfaces omit them", func(t *testing.T) {
		t.Parallel()
		app := pipelineApp(t, &bitbuckettest.FakeClient{T: t}, testBuild)

		bindings, items := app.ContextBindings(), app.helpBarItems()
		for _, key := range []string{"P", "X"} {
			if !hasBindingKey(bindings, key) {
				t.Errorf("the Builds help overlay should list %q", key)
			}
			if !hasHelpKey(items, key) {
				t.Errorf("the Builds help bar should list %q", key)
			}
		}

		app.readOnly = true
		bindings, items = app.ContextBindings(), app.helpBarItems()
		for _, key := range []string{"P", "X"} {
			if hasBindingKey(bindings, key) {
				t.Errorf("read-only mode must not advertise %q", key)
			}
			if hasHelpKey(items, key) {
				t.Errorf("read-only mode must not advertise %q on the help bar", key)
			}
		}
	})

	t.Run("the pipeline view lists them as well", func(t *testing.T) {
		t.Parallel()
		app := pipelineApp(t, &bitbuckettest.FakeClient{T: t}, testBuild)
		app.side = sideRight
		app.detailView.SetPipelineLoading(testPRKey, testBuild)

		if !hasBindingKey(app.ContextBindings(), "P") {
			t.Error("the pipeline view should list the rerun key")
		}
		if !hasHelpKey(app.helpBarItems(), "X") {
			t.Error("the pipeline strip should name the stop key")
		}
	})
}

func TestPipelineWrite_ASecondKeyIsRefusedWhileTheFirstIsRunning(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	_ = answerRuns(fake, map[string]*bitbucket.Pipeline{"412": completedRun(testBuild)})
	app := pipelineApp(t, fake, testBuild)

	pressKey(t, app, 'P')
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	for _, msg := range collectMsgs(cmd) {
		// The rerun command is left unrun, so the write is still in flight when
		// the next key arrives.
		_, _ = app.Update(msg)
	}

	testkit.AssertEqual(t, "in flight",
		app.pipelineWrites[pipelineCacheKey(testPRKey, testBuild)], verbRerunPipeline)

	_, _ = app.Update(runeKey('P'))

	if app.form.IsVisible() {
		t.Error("a rerun already running must block a second dialog")
	}
	if got := plain(app.helpBar.View()); !strings.Contains(got, verbRerunPipeline+" already in progress") {
		t.Errorf("help bar = %q, want the in-flight refusal", got)
	}
	testkit.AssertEqual(t, "run calls", fake.CallCount("RunPipeline"), 0)
}

// A pull request key and a pipeline key share the "ws/slug#N" shape, so the two
// in-flight maps have to be separate ones.
func TestPipelineWrite_APullRequestWriteDoesNotBlockTheSameNumberedRun(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	_ = answerRuns(fake, map[string]*bitbucket.Pipeline{"101": completedRun(101)})
	app := pipelineApp(t, fake, 101)
	app.writesInFlight[testPRKey] = verbMerge

	pressKey(t, app, 'P')

	if !app.form.IsVisible() {
		t.Fatal("a merge of pull request #101 says nothing about build #101")
	}
	if got := plain(app.form.View()); !strings.Contains(got, "Rerun pipeline #101?") {
		t.Errorf("dialog = %q, want the rerun question", got)
	}
}

func TestPipelineAction_APreDialogReadThatFailsOpensNothing(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.GetPipelineFunc = func(context.Context, string, string, string) (*bitbucket.Pipeline, error) {
		return nil, apiError(http.StatusNotFound, "Not found", nil)
	}
	app := pipelineApp(t, fake, testBuild)

	pressKey(t, app, 'P')

	if app.form.IsVisible() {
		t.Error("a dialog that cannot state the run's state must not open")
	}
	if got := plain(app.helpBar.View()); !strings.Contains(got, verbRerunPipeline+" failed: Not found") {
		t.Errorf("help bar = %q, want the failure", got)
	}
	testkit.AssertEqual(t, "run calls", fake.CallCount("RunPipeline"), 0)
}

func TestPipelineAction_AReadThatLandsAfterTheUserMovedOpensNothing(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	_ = answerRuns(fake, map[string]*bitbucket.Pipeline{"412": completedRun(testBuild)})
	app := pipelineApp(t, fake, testBuild)

	_, cmd := app.Update(runeKey('P'))
	// The user walks off to another pull request while the read is in flight.
	other := withAuthor(newPR(testRepo, 102, testTitle), testUserUUID)
	app.prCache[other.Key()] = &other
	app.previewKey = other.Key()
	runAll(t, app, cmd)

	if app.form.IsVisible() {
		t.Error("a dialog must not open over a pull request the user has left")
	}
	testkit.AssertEqual(t, "run calls", fake.CallCount("RunPipeline"), 0)
}

// -----------------------------------------------------------------------------
// The pipelines view, where there is no pull request at all
// -----------------------------------------------------------------------------

// pipelinesViewApp is the pipelines view with its list loaded and the cursor on
// api-gateway's run, and not one pull request anywhere in the app — which is
// the regression the pipelineRef refactor is for.
func pipelinesViewApp(t *testing.T, fake *bitbuckettest.FakeClient) *App {
	t.Helper()
	stubPipelineList(fake)
	app := pipelinesApp(t, fake)
	runAll(t, app, app.fetchPipelines())
	_, cmd := app.Update(runeKey('j'))
	runAll(t, app, cmd)
	testkit.AssertEqual(t, "the cursor is on the api-gateway run", app.runKey, testRunKey)
	return app
}

func TestPipelineKeys_RerunFromThePipelinesPanelWithoutAPullRequest(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	original := completedRun(testBuild)
	asked := answerRuns(fake, map[string]*bitbucket.Pipeline{"412": original})
	fake.RunPipelineFunc = func(
		context.Context, string, string, bitbucket.PipelineTarget,
	) (*bitbucket.Pipeline, error) {
		return newRun(testNewBuild, bitbucket.PipelineState{Type: "pipeline_state_pending", Name: "PENDING"}), nil
	}
	app := pipelinesViewApp(t, fake)

	pressKey(t, app, 'P')
	if got := plain(app.form.View()); !strings.Contains(got, "Rerun pipeline #412?") {
		t.Fatalf("dialog = %q, want the rerun question", got)
	}

	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "run calls", fake.CallCount("RunPipeline"), 1)
	call := fake.RunPipelineCalls[0]
	testkit.AssertEqual(t, "the repository comes from the run, not a pull request", call.RepoSlug, testRepo)
	testkit.AssertEqual(t, "target", call.Target, original.Target)
	testkit.AssertSliceEqual(t, "runs read", *asked, []string{"412"})
	// The list is what this view drills into runs from, so it is read again and
	// the view is pointed at the run the rerun created.
	testkit.AssertEqual(t, "the list was refetched", fake.CallCount("ListPipelines"), 4)
	testkit.AssertEqual(t, "the new run is what the view points at", app.runKey,
		pipelineRef{workspace: testWorkspace, slug: testRepo, build: testNewBuild}.key())
	if got := plain(app.helpBar.View()); !strings.Contains(got, "pipeline #413 started (rerun of #412)") {
		t.Errorf("help bar = %q, want the started line", got)
	}
	if _, busy := app.pipelineWrites[testRunKey]; busy {
		t.Error("the in-flight record must clear once the write has answered")
	}
}

func TestPipelineKeys_StopFromThePipelinesPanelRefetchesTheList(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	_ = answerRuns(fake, map[string]*bitbucket.Pipeline{"412": runningRun(testBuild)})
	fake.StopPipelineFunc = func(context.Context, string, string, string) error { return nil }
	app := pipelinesViewApp(t, fake)

	pressKey(t, app, 'X')
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "stop calls", fake.CallCount("StopPipeline"), 1)
	testkit.AssertEqual(t, "repository", fake.StopPipelineCalls[0].RepoSlug, testRepo)
	testkit.AssertEqual(t, "uuid", fake.StopPipelineCalls[0].PipelineUUID, runUUID(testBuild))
	testkit.AssertEqual(t, "the list was refetched", fake.CallCount("ListPipelines"), 4)
	if got := plain(app.helpBar.View()); !strings.Contains(got, "stop requested for pipeline #412") {
		t.Errorf("help bar = %q, want the stop line", got)
	}
}

func TestPipelineKeys_WorkFromTheStandaloneRunInTheDetailPanel(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	_ = answerRuns(fake, map[string]*bitbucket.Pipeline{"412": completedRun(testBuild)})
	app := pipelinesViewApp(t, fake)

	_, cmd := app.handleActivate()
	runAll(t, app, cmd)
	testkit.AssertEqual(t, "focus", app.side, sideRight)

	pressKey(t, app, 'P')

	if got := plain(app.form.View()); !strings.Contains(got, "Rerun pipeline #412?") {
		t.Errorf("dialog = %q, want the rerun question", got)
	}
}

func TestPipelineKeys_InThePipelinesViewTheHintNamesThePipelinesPanel(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	app := pipelinesViewApp(t, fake)
	app.leftFocus = focusRepos
	app.updateFocusState()

	_, cmd := app.Update(runeKey('P'))

	if cmd != nil {
		t.Error("with no run under the cursor nothing is read")
	}
	if got := plain(app.helpBar.View()); !strings.Contains(got, "select a run in the Pipelines panel (2) first") {
		t.Errorf("help bar = %q, want the hint naming the Pipelines panel", got)
	}
}
