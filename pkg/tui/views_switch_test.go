package tui

import (
	"context"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/config"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// shiftTab is the view switch's default binding.
func shiftTab() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyShiftTab} }

func TestViewFromConfig(t *testing.T) {
	t.Parallel()
	cases := map[string]appView{
		"":                           viewPullRequests,
		config.StartViewPullRequests: viewPullRequests,
		config.StartViewPipelines:    viewPipelines,
		"builds":                     viewPullRequests,
	}
	for value, want := range cases {
		testkit.AssertEqual(t, "viewFromConfig("+value+")", viewFromConfig(value), want)
	}
}

func TestCycleView_WrapsBothWays(t *testing.T) {
	t.Parallel()
	app := &App{}
	forward := []appView{viewPipelines, viewPullRequests}
	for i, want := range forward {
		app.cycleView(1)
		testkit.AssertEqual(t, "forward step "+strconv.Itoa(i), app.view, want)
	}
	app.cycleView(-1)
	testkit.AssertEqual(t, "backward wraps", app.view, viewPipelines)
}

func TestViewSwitch_LandsOnThePipelinesPanelAndComesBack(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipelineList(fake)
	app := writeApp(t, fake)
	app.detailView.SetPullRequest(app.prList.SelectedPullRequest())

	_, cmd := app.Update(shiftTab())
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "view", app.view, viewPipelines)
	testkit.AssertEqual(t, "side", app.side, sideLeft)
	testkit.AssertEqual(t, "focus lands on [2]", app.leftFocus, focusPipelines)
	if got := plain(app.statusPanel.View()); !strings.Contains(got, "[1] Status · Pipelines") {
		t.Errorf("status panel = %q, want the view name in the title", got)
	}
	if got := plain(app.repoList.View()); !strings.Contains(got, "[3] Repos") {
		t.Errorf("repos panel = %q, want the pipelines view's number", got)
	}
	testkit.AssertEqual(t, "the detail panel shows the run", app.detailView.PipelineOwner(), testOtherRunKey)

	// Walk onto the second run, so there is view state to survive the trip.
	_, cmd = app.Update(runeKey('j'))
	runAll(t, app, cmd)
	testkit.AssertEqual(t, "cursor moved", app.runKey, testRunKey)

	_, cmd = app.Update(shiftTab())
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "back in the pull request view", app.view, viewPullRequests)
	testkit.AssertEqual(t, "focus lands on [2]", app.leftFocus, focusPullRequests)
	testkit.AssertEqual(t, "the detail panel is a pull request again",
		app.detailView.Mode(), views.ModePullRequest)
	if got := plain(app.statusPanel.View()); strings.Contains(got, "· Pipelines") {
		t.Errorf("status panel = %q, want no view name in the pull request view", got)
	}

	_, cmd = app.Update(shiftTab())
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "the run selection survived the round trip", app.runKey, testRunKey)
	testkit.AssertEqual(t, "the list was not fetched again", fake.CallCount("ListPipelines"), 2)
}

func TestViewSwitch_ShowsTheSplashWhenNoPullRequestIsSelected(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipelineList(fake)
	app := sizedApp(t, fake)

	_, cmd := app.Update(shiftTab())
	runAll(t, app, cmd)
	_, cmd = app.Update(shiftTab())
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "detail mode", app.detailView.Mode(), views.ModeSplash)
}

func TestStartView_OpensThePipelinesView(t *testing.T) {
	t.Parallel()
	fake, _ := constructionFake(t)
	cfg := testConfig()
	cfg.GUI.StartView = config.StartViewPipelines

	app := NewAppWithAuth(cfg, fake, AuthSaved)
	t.Cleanup(app.Shutdown)

	testkit.AssertEqual(t, "view", app.view, viewPipelines)
	testkit.AssertEqual(t, "focus", app.leftFocus, focusPipelines)
	if got := plain(app.statusPanel.View()); !strings.Contains(got, "· Pipelines") {
		t.Errorf("status panel = %q, want the view name", got)
	}
}

func TestStartView_FetchesTheListOnceTheRepositoriesAreKnown(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipelineList(fake)
	fake.ListPullRequestsFunc = func(
		context.Context, string, string, bitbucket.ListPullRequestsOptions,
	) ([]bitbucket.PullRequest, error) {
		return nil, nil
	}
	app := pipelinesApp(t, fake)
	app.pipelinesLoaded = false

	_, cmd := app.Update(reposLoadedMsg{repos: []bitbucket.Repository{
		{Slug: testRepo, Name: testRepo}, {Slug: testRepo2, Name: testRepo2},
	}, err: nil})
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "the pipelines list is fetched too", fake.CallCount("ListPipelines"), 2)
}

func TestFocusKeys_FollowTheView(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipelineList(fake)
	app := pipelinesApp(t, fake)

	// l walks Status → Pipelines → Repos → Status; the Builds panel is not in
	// this view at all.
	app.leftFocus = focusStatus
	for _, want := range []focusPanel{focusPipelines, focusRepos, focusStatus} {
		if _, _, ok := app.handleFocusAction(ActFocusRight); !ok {
			t.Fatal("focus right must be handled on the left side")
		}
		testkit.AssertEqual(t, "leftFocus", app.leftFocus, want)
	}
	for _, want := range []focusPanel{focusRepos, focusPipelines, focusStatus} {
		if _, _, ok := app.handleFocusAction(ActFocusLeft); !ok {
			t.Fatal("focus left must be handled on the left side")
		}
		testkit.AssertEqual(t, "leftFocus", app.leftFocus, want)
	}

	// The number keys: 2 is the list this view is about, and both 3 and 4 land
	// on Repos.
	cases := map[Action]focusPanel{
		ActFocusPullRequests: focusPipelines,
		ActFocusBuilds:       focusRepos,
		ActFocusRepos:        focusRepos,
	}
	for action, want := range cases {
		app.leftFocus = focusStatus
		_, _, ok := app.handleFocusAction(action)
		if !ok {
			t.Fatalf("%s must be handled", action)
		}
		testkit.AssertEqual(t, string(action), app.leftFocus, want)
	}
}

func TestLayout_ThePipelinesViewDrawsItsOwnLeftColumn(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipelineList(fake)
	app := pipelinesApp(t, fake)
	runAll(t, app, app.fetchPipelines())
	app.layoutPanels()

	testkit.AssertEqual(t, "the pull request list is not drawn", app.panelPRH, 0)
	testkit.AssertEqual(t, "the builds panel is not drawn", app.panelBuildsH, 0)
	if app.panelPipelinesH <= app.panelReposH {
		t.Errorf("the pipelines list must get the space: %d vs repos %d",
			app.panelPipelinesH, app.panelReposH)
	}

	out := plain(app.View())
	if !strings.Contains(out, "[2] Pipelines") || !strings.Contains(out, "[3] Repos") {
		t.Errorf("view = %q, want the pipelines view's panels", out)
	}
	if strings.Contains(out, "[3] Builds") {
		t.Error("the builds panel belongs to the other view")
	}
}

func TestMouse_ClickAndWheelLandOnThePipelinesPanel(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipelineList(fake)
	app := pipelinesApp(t, fake)
	runAll(t, app, app.fetchPipelines())
	app.side = sideRight
	app.layoutPanels()

	// A click inside the second row of the [2] panel, which starts under the
	// status panel.
	panel, relY := app.hitTest(2, app.panelStatusH+2)
	testkit.AssertEqual(t, "panel", panel, panelPipelines)

	_, cmd := app.mouseClick(panel, relY, 2)
	runAll(t, app, cmd)
	testkit.AssertEqual(t, "side", app.side, sideLeft)
	testkit.AssertEqual(t, "leftFocus", app.leftFocus, focusPipelines)
	testkit.AssertEqual(t, "the clicked run is shown", app.runKey, testRunKey)

	_, cmd = app.mouseScroll(panelPipelines, -3)
	runAll(t, app, cmd)
	testkit.AssertEqual(t, "the wheel moved the cursor", app.runKey, testOtherRunKey)
}

func TestCustomCommands_DoNotFireInThePipelinesView(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipelineList(fake)
	app := loadedPipelinesApp(t, fake)
	app.customCmds = []config.ResolvedCustomCommand{
		{Name: "open", Key: "w", Command: "echo", Contexts: []config.Context{config.CtxPullRequests}},
		{Name: "clone", Key: "z", Command: "echo", Contexts: []config.Context{config.CtxRepos}},
		{Name: "note", Key: "Z", Command: "echo", Contexts: []config.Context{config.CtxDetail}},
	}

	// The two panels the pipelines view shares with the other one, and its
	// detail panel, are the states a context could leak through.
	for _, focus := range []struct {
		name  string
		key   string
		setup func()
	}{
		{name: "the pipelines panel", key: "w", setup: func() { app.leftFocus = focusPipelines }},
		{name: "the repos panel", key: "z", setup: func() { app.leftFocus = focusRepos }},
		{name: "a standalone run", key: "Z", setup: func() { app.side = sideRight }},
	} {
		app.side, app.leftFocus = sideLeft, focusPipelines
		focus.setup()
		app.updateFocusState()

		if ctxs := app.activeContexts(); len(ctxs) != 0 {
			t.Errorf("activeContexts on %s = %v, want none in the pipelines view", focus.name, ctxs)
		}
		if _, _, ok := app.handleCustomCommand(focus.key); ok {
			t.Errorf("a command written against a pull request must not fire on %s", focus.name)
		}
		// What does not fire is not advertised either.
		if hasHelpKey(app.helpBarItems(), focus.key) {
			t.Errorf("the help bar on %s lists a command that cannot run", focus.name)
		}
		if hasBindingKey(app.ContextBindings(), focus.key) {
			t.Errorf("the help overlay on %s lists a command that cannot run", focus.name)
		}
	}
}

// The pull request keys keep working on the pull request the other view has
// selected unless they are stopped: nothing on this screen names it.
func TestPullRequestKeys_AreRefusedInThePipelinesView(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipelineList(fake)
	app := writeApp(t, fake)

	_, cmd := app.Update(shiftTab())
	runAll(t, app, cmd)

	for _, key := range []rune{'M', 'S', 'E', 'V', 'D', 'A', 'c', 'u'} {
		app.helpBar.SetStatusMsg("")
		_, cmd := app.Update(runeKey(key))
		runAll(t, app, cmd)

		if app.overlayVisible() {
			t.Errorf("%q opened something over the pipelines view", string(key))
		}
		if got := plain(app.helpBar.View()); !strings.Contains(got, noPullRequestHere) {
			t.Errorf("help bar after %q = %q, want %q", string(key), got, noPullRequestHere)
		}
	}
}

// A run drilled into from the Builds panel belongs to the other view: entering
// the pipelines view has to replace it, or the pipeline keys would read the
// pull request's own id as a build number.
func TestViewSwitch_APullRequestRunDoesNotSurviveTheSwitch(t *testing.T) {
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
	app := pipelineApp(t, fake, testBuild)

	// Into the build's run, then into a step's log: the deepest the other view
	// goes.
	_, cmd := app.handleActivate()
	runAll(t, app, cmd)
	_ = app.detailView.View()
	_, cmd = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	runAll(t, app, cmd)
	testkit.AssertEqual(t, "in the step log", app.detailView.Mode(), views.ModeStepLog)

	_, cmd = app.Update(shiftTab())
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "view", app.view, viewPipelines)
	testkit.AssertEqual(t, "the panel shows this view's run", app.detailView.PipelineOwner(), testOtherRunKey)
	testkit.AssertEqual(t, "which is a standalone one", app.detailView.PipelineOwnerIsPR(), false)
	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModePipeline)
}

func TestLayout_ThePipelinesViewStacksItsOwnPanelsWhenNarrow(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipelineList(fake)
	app := loadedPipelinesApp(t, fake)

	app.width = 60
	app.layoutPanels()
	if !app.isVerticalLayout() {
		t.Fatal("a 60-column window is the narrow layout")
	}

	testkit.AssertEqual(t, "no pull request list", app.panelPRH, 0)
	testkit.AssertEqual(t, "no builds panel", app.panelBuildsH, 0)
	if app.panelPipelinesH <= 1 {
		t.Errorf("pipelines height = %d, want the focused panel's share", app.panelPipelinesH)
	}

	out := plain(app.View())
	if !strings.Contains(out, "[2] Pipelines") || strings.Contains(out, "[3] Builds") {
		t.Errorf("narrow view = %q, want the pipelines column", out)
	}

	// A click has to land on the panel that is actually drawn there.
	panel, _ := app.hitTest(2, app.panelStatusH+2)
	testkit.AssertEqual(t, "panel under the click", panel, panelPipelines)
}

func TestHelp_NamesTheViewTheSwitchGoesTo(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipelineList(fake)

	app := writeApp(t, fake)
	if got := plain(app.helpBar.View()); !strings.Contains(got, viewNamePipelines) {
		t.Errorf("help bar = %q, want the pipelines view as the switch's destination", got)
	}
	if !hasBinding(app.ContextBindings(), "switch to the pipelines view") {
		t.Error("the help overlay must list the view switch")
	}

	_, cmd := app.Update(shiftTab())
	runAll(t, app, cmd)

	if got := plain(app.helpBar.View()); !strings.Contains(got, viewNamePullRequests) {
		t.Errorf("help bar = %q, want the pull requests view as the switch's destination", got)
	}
	bindings := app.ContextBindings()
	if !hasBinding(bindings, "switch to the pull requests view") {
		t.Error("the help overlay must list the view switch")
	}
	if !hasBinding(bindings, "go to the pull request this run ran for") {
		t.Error("the pipelines panel must list its own key")
	}
	if !hasBinding(bindings, "rerun the pipeline") {
		t.Error("the pipeline writes work on the pipelines panel and must be listed there")
	}
}

// hasBinding reports whether a binding with that description is in the list.
func hasBinding(bindings []Binding, description string) bool {
	for _, b := range bindings {
		if b.Description == description {
			return true
		}
	}
	return false
}
