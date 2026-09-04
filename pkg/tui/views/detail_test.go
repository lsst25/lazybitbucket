package views

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
)

// navResolverForDetail maps the keys the detail tests press. J/K are the
// detail-pane scroll keys, matching the default keybindings.
func navResolverForDetail(key string) components.NavAction {
	switch key {
	case "J", "j":
		return components.NavDown
	case "K", "k":
		return components.NavUp
	case "ctrl+f":
		return components.NavHalfDown
	case "ctrl+b":
		return components.NavHalfUp
	case "g":
		return components.NavTop
	case "G":
		return components.NavBottom
	default:
		return components.NavNone
	}
}

func makeFocusedDetail() *DetailView {
	view := NewDetailView(PlainRenderer{}, "")
	view.ResolveNav = navResolverForDetail
	view.SetSize(80, 24)
	view.SetFocused(true)
	return view
}

func detailPR() *bitbucket.PullRequest {
	pr := withApproval(newPR(101, testRepo, testPRTitle))
	pr.Description = "## Why\n\nBecause the gateway needs auth.\n\n- one\n- two\n"
	pr.CommentCount = 7
	return &pr
}

func testPipeline() *bitbucket.Pipeline {
	return &bitbucket.Pipeline{
		UUID:              testPipelineUUID,
		BuildNumber:       42,
		DurationInSeconds: 586,
		CreatedOn:         time.Now().Add(-3 * time.Hour),
		State: bitbucket.PipelineState{
			Name:   "COMPLETED",
			Result: &bitbucket.PipelineStateResult{Name: "FAILED"},
		},
		Trigger: bitbucket.PipelineTrigger{Name: "PUSH"},
		Creator: &bitbucket.Account{DisplayName: testAuthor},
		Target: bitbucket.PipelineTarget{
			Type:        "pipeline_pullrequest_target",
			Source:      "feature/x",
			Destination: "develop",
			PullRequest: &bitbucket.PipelinePullRequestRef{ID: 101},
		},
	}
}

func testSteps() []bitbucket.PipelineStep {
	return []bitbucket.PipelineStep{
		{
			UUID:              testStepUUID,
			Name:              "Preparation",
			DurationInSeconds: 62,
			State: bitbucket.PipelineStepState{
				Name: "COMPLETED",
				Result: &bitbucket.PipelineStepResult{
					Name:  "FAILED",
					Error: &bitbucket.PipelineError{Key: "step-failed", Message: "exit code 1"},
				},
			},
		},
		{
			UUID: testStepUUID2,
			Name: "Ensure V3 Lockfile",
			State: bitbucket.PipelineStepState{
				Name:   "COMPLETED",
				Result: &bitbucket.PipelineStepResult{Name: "NOT_RUN"},
			},
		},
	}
}

// pipelineDetail returns a detail view already drilled into a pipeline run,
// with View() called so the block cache is populated.
func pipelineDetail(t *testing.T) *DetailView {
	t.Helper()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())
	view.SetPipeline(testPRKey, testPipeline(), testSteps())
	view.View()
	return view
}

func TestDetailView_View_PlaceholderWithoutAPullRequest(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()

	out := stripANSI(view.View())
	if !strings.Contains(out, "Select a pull request") {
		t.Errorf("View() = %q, want the placeholder", out)
	}
	testkit.AssertEqual(t, "mode", view.Mode(), ModePullRequest)
	testkit.AssertEqual(t, "key", view.PullRequestKey(), "")
}

func TestDetailView_View_OverviewShowsHeaderAndDescription(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())

	out := stripANSI(view.View())
	for _, want := range []string{
		"[0] #101 " + testRepo,
		testPRTitle,
		"#101 · " + testWorkspace + "/" + testRepo + " · OPEN",
		"feature/x → main",
		"Author: " + testAuthor,
		"Created 2d ago",
		"Updated 3h ago",
		"7 comments",
		"Reviewers: ✓ " + testReviewer,
		"Builds: loading…",
		"Because the gateway needs auth.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("View() = %q, want it to contain %q", out, want)
		}
	}
	testkit.AssertEqual(t, "key", view.PullRequestKey(), testPRKey)
}

func TestDetailView_View_OverviewWithoutReviewDataOrDescription(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	pr := newPR(101, testRepo, testPRTitle)
	view.SetPullRequest(&pr)

	out := stripANSI(view.View())
	for _, want := range []string{"Reviewers: loading…", "(no description)"} {
		if !strings.Contains(out, want) {
			t.Errorf("View() = %q, want it to contain %q", out, want)
		}
	}
}

func TestDetailView_View_OverviewEmptyReviewerList(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	pr := newPR(101, testRepo, testPRTitle)
	pr.Participants = []bitbucket.Participant{}
	view.SetPullRequest(&pr)

	if !strings.Contains(stripANSI(view.View()), "Reviewers: (none)") {
		t.Error("a pull request with no reviewers should say (none)")
	}
}

func TestDetailView_SetStatuses_FeedsTheBuildsLine(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())
	view.SetStatuses(testPRKey, testStatuses())

	out := stripANSI(view.View())
	for _, want := range []string{"Builds: ✘ FAILED", "Lint", "Builds: ✔ SUCCESSFUL"} {
		if !strings.Contains(out, want) {
			t.Errorf("View() = %q, want it to contain %q", out, want)
		}
	}

	view.SetStatuses(testPRKey, nil)
	if !strings.Contains(stripANSI(view.View()), "Builds: none") {
		t.Error("an empty status list should render Builds: none")
	}
}

func TestDetailView_SetStatuses_StaleKeyDropped(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())
	view.SetStatuses(testPRKey2, testStatuses())

	if !strings.Contains(stripANSI(view.View()), "Builds: loading…") {
		t.Error("statuses for another pull request must be dropped")
	}
}

func TestDetailView_SetPullRequest_KeepsScrollOnSameKey(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())
	view.ScrollBy(3)
	testkit.AssertEqual(t, "scroll", view.scrollY, 3)

	same := detailPR()
	same.Title = "Refreshed in the background"
	view.SetPullRequest(same)
	testkit.AssertEqual(t, "scroll preserved", view.scrollY, 3)

	other := newPR(102, testRepo, "Another")
	view.SetPullRequest(&other)
	testkit.AssertEqual(t, "scroll reset", view.scrollY, 0)
}

func TestDetailView_UpdatePullRequestData_KeepsMode(t *testing.T) {
	t.Parallel()
	view := pipelineDetail(t)
	view.UpdatePullRequestData(detailPR())

	testkit.AssertEqual(t, "mode", view.Mode(), ModePipeline)
}

func TestDetailView_View_PipelineHeaderAndStepBlocks(t *testing.T) {
	t.Parallel()
	view := pipelineDetail(t)

	out := stripANSI(view.View())
	for _, want := range []string{
		"[0] Pipeline #42",
		"Pipeline #42 · FAILED · 9m46s · PUSH",
		"by " + testAuthor,
		"3h ago",
		"PR #101 feature/x → develop",
		"✘ Preparation",
		"1m02s",
		"step-failed: exit code 1",
		"- Ensure V3 Lockfile",
		notRunLabel,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("View() = %q, want it to contain %q", out, want)
		}
	}
	testkit.AssertEqual(t, "is a list mode", view.IsListTab(), true)
	testkit.AssertEqual(t, "pipeline", view.Pipeline().BuildNumber, 42)
}

func TestDetailView_View_PipelineErrorAndLoading(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())

	view.SetPipelineLoading(testPRKey, 42)
	testkit.AssertEqual(t, "mode", view.Mode(), ModePipeline)
	if !strings.Contains(stripANSI(view.View()), "Loading pipeline #42…") {
		t.Error("View() should show the pipeline loading placeholder")
	}

	view.SetPipelineError(testPRKey, "get pipeline acme/api-gateway 42: status 404")
	if !strings.Contains(stripANSI(view.View()), "status 404") {
		t.Error("View() should show the pipeline error")
	}
}

func TestDetailView_View_PipelineRunErrorLines(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())
	p := testPipeline()
	p.State.Result = &bitbucket.PipelineStateResult{
		Name:  "ERROR",
		Error: &bitbucket.PipelineError{Key: "config-error", Message: "invalid yaml"},
	}
	view.SetPipeline(testPRKey, p, testSteps())

	out := stripANSI(view.View())
	if !strings.Contains(out, "config-error: invalid yaml") {
		t.Errorf("View() = %q, want the run error line", out)
	}
}

func TestDetailView_PipelineStaleKeysAreDropped(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())

	view.SetPipeline(testPRKey2, testPipeline(), testSteps())
	testkit.AssertEqual(t, "mode unchanged", view.Mode(), ModePullRequest)
	if view.Pipeline() != nil {
		t.Error("a pipeline for another pull request must be dropped")
	}

	view.SetPipelineLoading(testPRKey2, 9)
	testkit.AssertEqual(t, "mode still overview", view.Mode(), ModePullRequest)

	view.SetPipelineError(testPRKey2, "boom")
	testkit.AssertEqual(t, "mode still overview after error", view.Mode(), ModePullRequest)
}

func TestDetailView_SelectedStep_TracksTheBlockCursor(t *testing.T) {
	t.Parallel()
	view := pipelineDetail(t)

	step := view.SelectedStep()
	if step == nil {
		t.Fatal("SelectedStep() = nil")
	}
	testkit.AssertEqual(t, "first step", step.UUID, testStepUUID)

	view.ScrollBy(1)
	testkit.AssertEqual(t, "second step", view.SelectedStep().UUID, testStepUUID2)

	view.Back()
	if view.SelectedStep() != nil {
		t.Error("SelectedStep() should be nil outside the pipeline view")
	}
}

func TestDetailView_Update_EnterOnStepEmitsStepSelected(t *testing.T) {
	t.Parallel()
	view := pipelineDetail(t)

	_, cmd := view.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Update() returned no command on Enter")
	}
	msg, ok := cmd().(StepSelectedMsg)
	if !ok {
		t.Fatalf("msg = %T, want StepSelectedMsg", cmd())
	}
	testkit.AssertEqual(t, "pipeline uuid", msg.PipelineUUID, testPipelineUUID)
	testkit.AssertEqual(t, "step uuid", msg.StepUUID, testStepUUID)
}

func TestDetailView_Update_EnterOnTruncatedBlockEmitsExpand(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())

	// A step with a long error message and no UUID: nothing to open a log for,
	// so Enter offers the block instead.
	long := strings.Repeat("a failing assertion line. ", 40)
	view.SetPipeline(testPRKey, testPipeline(), []bitbucket.PipelineStep{{
		Name: "Test",
		State: bitbucket.PipelineStepState{
			Name: "COMPLETED",
			Result: &bitbucket.PipelineStepResult{
				Name:  "FAILED",
				Error: &bitbucket.PipelineError{Message: long},
			},
		},
	}})
	view.View()

	_, cmd := view.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Update() returned no command on Enter")
	}
	msg, ok := cmd().(ExpandBlockMsg)
	if !ok {
		t.Fatalf("msg = %T, want ExpandBlockMsg", cmd())
	}
	testkit.AssertEqual(t, "title", msg.Title, "Details")
	if len(msg.Lines) <= maxBlockLines {
		t.Errorf("len(Lines) = %d, want more than %d", len(msg.Lines), maxBlockLines)
	}
	if !strings.Contains(stripANSI(view.View()), "...") {
		t.Error("a truncated block should render the ellipsis line")
	}
}

func TestDetailView_Update_EnterWithoutBlocksIsANoOp(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())
	view.View()

	if _, cmd := view.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Error("Enter in the overview should emit nothing")
	}
}

func TestDetailView_Update_UnfocusedIsInert(t *testing.T) {
	t.Parallel()
	view := pipelineDetail(t)
	view.SetFocused(false)

	if _, cmd := view.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Error("an unfocused detail view should emit nothing")
	}
}

func TestDetailView_StepLog_LoadingTextAndErrors(t *testing.T) {
	t.Parallel()
	view := pipelineDetail(t)

	view.SetStepLogLoading(testPipelineUUID, testStepUUID)
	testkit.AssertEqual(t, "mode", view.Mode(), ModeStepLog)
	out := stripANSI(view.View())
	if !strings.Contains(out, "Loading log…") {
		t.Errorf("View() = %q, want the log loading placeholder", out)
	}
	if !strings.Contains(out, "[0] Pipeline #42 · Preparation") {
		t.Errorf("View() = %q, want the step log title", out)
	}

	view.SetStepLog(testPipelineUUID, testStepUUID, "+ go test ./...\nok\n")
	out = stripANSI(view.View())
	for _, want := range []string{"+ go test ./...", "ok"} {
		if !strings.Contains(out, want) {
			t.Errorf("View() = %q, want it to contain %q", out, want)
		}
	}

	view.SetStepLogError(testPipelineUUID, testStepUUID, "get step log: status 404")
	if !strings.Contains(stripANSI(view.View()), "status 404") {
		t.Error("View() should show the step log error")
	}
}

func TestDetailView_StepLog_EmptyLog(t *testing.T) {
	t.Parallel()
	view := pipelineDetail(t)
	view.SetStepLogLoading(testPipelineUUID, testStepUUID)
	view.SetStepLog(testPipelineUUID, testStepUUID, "   \n")

	if !strings.Contains(stripANSI(view.View()), "(empty log)") {
		t.Error("an empty log should render the placeholder")
	}
}

func TestDetailView_StepLog_HardWrapsLongLines(t *testing.T) {
	t.Parallel()
	view := pipelineDetail(t)
	view.SetStepLogLoading(testPipelineUUID, testStepUUID)
	view.SetStepLog(testPipelineUUID, testStepUUID, strings.Repeat("x", 300))

	for _, line := range strings.Split(stripANSI(view.View()), "\n") {
		if len([]rune(line)) > 80 {
			t.Fatalf("line is %d columns wide, want at most 80", len([]rune(line)))
		}
	}
}

func TestDetailView_StepLog_StaleKeysAreDropped(t *testing.T) {
	t.Parallel()
	view := pipelineDetail(t)

	view.SetStepLogLoading("{other-pipeline}", testStepUUID)
	testkit.AssertEqual(t, "mode unchanged", view.Mode(), ModePipeline)

	view.SetStepLogLoading(testPipelineUUID, testStepUUID)
	view.SetStepLog(testPipelineUUID, testStepUUID2, "wrong step")
	if strings.Contains(stripANSI(view.View()), "wrong step") {
		t.Error("a log for another step must be dropped")
	}

	view.SetStepLogError(testPipelineUUID, testStepUUID2, "wrong error")
	if strings.Contains(stripANSI(view.View()), "wrong error") {
		t.Error("an error for another step must be dropped")
	}
}

func TestDetailView_Back_WalksTheDrillDownChain(t *testing.T) {
	t.Parallel()
	view := pipelineDetail(t)
	view.SetStepLogLoading(testPipelineUUID, testStepUUID)
	view.SetStepLog(testPipelineUUID, testStepUUID, "log text")

	testkit.AssertEqual(t, "mode", view.Mode(), ModeStepLog)
	testkit.AssertEqual(t, "back from the log", view.Back(), true)
	testkit.AssertEqual(t, "mode", view.Mode(), ModePipeline)
	testkit.AssertEqual(t, "back from the pipeline", view.Back(), true)
	testkit.AssertEqual(t, "mode", view.Mode(), ModePullRequest)
	testkit.AssertEqual(t, "back from the overview", view.Back(), false)
	testkit.AssertEqual(t, "key still set", view.PullRequestKey(), testPRKey)
}

func TestDetailView_SetRepo_RendersRepositoryMetadata(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetRepo(&RepoItem{
		Slug:       testRepo,
		Name:       "API Gateway",
		FullName:   testWorkspace + "/" + testRepo,
		MainBranch: "main",
		URL:        "https://bitbucket.org/acme/api-gateway",
		OpenCount:  3,
	})

	testkit.AssertEqual(t, "mode", view.Mode(), ModeRepo)
	testkit.AssertEqual(t, "no pull request key", view.PullRequestKey(), "")
	out := stripANSI(view.View())
	for _, want := range []string{"[0] Repo: " + testRepo, "API Gateway", "main", "3", "bitbucket.org"} {
		if !strings.Contains(out, want) {
			t.Errorf("View() = %q, want it to contain %q", out, want)
		}
	}
}

func TestDetailView_SetSplash_RendersBannerAndFields(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetSize(80, 30)
	view.SetSplash(SplashInfo{
		Version:    "v0.1.0",
		AuthMethod: "bearer",
		Workspace:  testWorkspace,
		User:       "Demo User",
		RepoCount:  3,
	})

	testkit.AssertEqual(t, "mode", view.Mode(), ModeSplash)
	out := stripANSI(view.View())
	for _, want := range []string{"[0] lazybitbucket", "lazybitbucket v0.1.0", "Auth:", "bearer",
		"Workspace:", testWorkspace, "User:", "Demo User", "Repos:", "3"} {
		if !strings.Contains(out, want) {
			t.Errorf("View() = %q, want it to contain %q", out, want)
		}
	}
}

func TestDetailView_SetSplash_RepoNoteReplacesTheCount(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetSize(80, 30)
	view.SetSplash(SplashInfo{RepoNote: "none visible to this token"})

	if !strings.Contains(stripANSI(view.View()), "none visible to this token") {
		t.Error("RepoNote should replace the repository count")
	}
}

func TestSplashArt_FitsSixtyColumns(t *testing.T) {
	t.Parallel()
	for _, line := range strings.Split(splashArt, "\n") {
		if got := len([]rune(line)); got > 60 {
			t.Errorf("splash art line is %d columns wide, want at most 60", got)
		}
	}
}

func TestDetailView_Tabs_NoBarOutsideThePullRequestView(t *testing.T) {
	t.Parallel()
	view := pipelineDetail(t)

	title := stripANSI(view.buildTitle(70))
	testkit.AssertEqual(t, "title is the mode title", title, "[0] Pipeline #42")

	// With no tabs there are no zones, so a click cannot move the tab.
	testkit.AssertEqual(t, "click is a no-op", view.ClickTab(30), false)
	testkit.AssertEqual(t, "tab unchanged", view.ActiveTab(), TabOverview)

	view.NextTab()
	testkit.AssertEqual(t, "next tab stays put", view.ActiveTab(), TabOverview)
	view.PrevTab()
	testkit.AssertEqual(t, "prev tab stays put", view.ActiveTab(), TabOverview)
}

func TestDetailView_ScrollBy_LineOffsetInTextModes(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	pr := detailPR()
	pr.Description = strings.Repeat("a line of description\n", 60)
	view.SetPullRequest(pr)

	view.ScrollBy(5)
	testkit.AssertEqual(t, "scroll", view.scrollY, 5)
	view.ScrollBy(-99)
	testkit.AssertEqual(t, "scroll floored at zero", view.scrollY, 0)
}

func TestDetailView_Update_NavigationKeys(t *testing.T) {
	t.Parallel()
	view := pipelineDetail(t)

	for _, key := range []string{"J", "ctrl+f", "G"} {
		view.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	}
	testkit.AssertEqual(t, "cursor at the last step", view.SelectedStep().UUID, testStepUUID2)

	view.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	testkit.AssertEqual(t, "cursor back at the first step", view.SelectedStep().UUID, testStepUUID)

	view.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("K")})
	testkit.AssertEqual(t, "up wraps to the last step", view.SelectedStep().UUID, testStepUUID2)
}

func TestDetailView_Update_TabKeyDoesNotCycleTabs(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())

	// Tab cycling belongs to the app, which pairs it with the review fetches
	// the new tab needs. A "tab" reaching the view (universal.switchPanel
	// rebound off it) must be inert rather than switch to a tab nobody has
	// warmed.
	for range 4 {
		got, cmd := view.Update(tea.KeyMsg{Type: tea.KeyTab})
		if cmd != nil {
			t.Error("an unbound tab key should emit nothing")
		}
		testkit.AssertEqual(t, "tab", got.ActiveTab(), TabOverview)
	}
}

func TestDetailView_ClickItem_SelectsTheClickedBlock(t *testing.T) {
	t.Parallel()
	view := pipelineDetail(t)

	// relY counts from the title border, so the body starts at relY 1. The
	// header comes first, then block 0 (two lines) and its separator.
	header := len(view.listModeHeader(78))
	if cmd := view.ClickItem(1 + header + 3); cmd != nil {
		t.Error("a single click should not expand a block")
	}
	testkit.AssertEqual(t, "cursor moved to the second block", view.SelectedStep().UUID, testStepUUID2)

	if cmd := view.ClickItem(0); cmd != nil {
		t.Error("a click on the title bar should do nothing")
	}
	if cmd := view.ClickItem(999); cmd != nil {
		t.Error("a click past the last block should do nothing")
	}
}

func TestDetailView_ClickItem_DoubleClickExpandsATruncatedBlock(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())
	view.SetPipeline(testPRKey, testPipeline(), []bitbucket.PipelineStep{{
		Name: "Test",
		State: bitbucket.PipelineStepState{
			Name: "COMPLETED",
			Result: &bitbucket.PipelineStepResult{
				Name:  "FAILED",
				Error: &bitbucket.PipelineError{Message: strings.Repeat("a failing assertion. ", 40)},
			},
		},
	}})
	view.View()

	relY := 1 + len(view.listModeHeader(78))
	if cmd := view.ClickItem(relY); cmd != nil {
		t.Fatal("the first click should only select")
	}
	cmd := view.ClickItem(relY)
	if cmd == nil {
		t.Fatal("the second click should expand the truncated block")
	}
	if _, ok := cmd().(ExpandBlockMsg); !ok {
		t.Fatalf("msg = %T, want ExpandBlockMsg", cmd())
	}
}

func TestDetailView_ClickItem_IgnoredOutsideListModes(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())

	if cmd := view.ClickItem(3); cmd != nil {
		t.Error("ClickItem should be inert in the overview")
	}
}

func TestDetailView_SetFocused_KeepsTheBlockCursorAcrossABlur(t *testing.T) {
	t.Parallel()
	view := pipelineDetail(t)
	view.ScrollBy(1)
	testkit.AssertEqual(t, "cursor", view.SelectedStep().UUID, testStepUUID2)

	// The app recomputes focus on every panel switch, unfocusing the detail
	// panel even when it keeps focus, so a reset here would lose the reader's
	// place in a diff or a thread on each visit to the left column.
	view.SetFocused(false)
	view.SetFocused(true)
	testkit.AssertEqual(t, "cursor kept", view.SelectedStep().UUID, testStepUUID2)
}

func TestDetailView_View_TinyHeightStillRenders(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())
	view.SetSize(80, 1)

	if stripANSI(view.View()) == "" {
		t.Error("View() should still render at height 1")
	}
}

func TestFormatDuration(t *testing.T) {
	t.Parallel()
	cases := map[time.Duration]string{
		0:                       "0s",
		35 * time.Second:        "35s",
		62 * time.Second:        "1m02s",
		586 * time.Second:       "9m46s",
		3900 * time.Second:      "1h05m",
		-5 * time.Second:        "0s",
		2*time.Hour + time.Hour: "3h00m",
	}
	for in, want := range cases {
		if got := formatDuration(in); got != want {
			t.Errorf("formatDuration(%s) = %q, want %q", in, got, want)
		}
	}
}

func TestDetailView_View_ClampsEveryLineToThePanelWidth(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		width int
		setup func(view *DetailView)
	}{
		{"splash narrower than the banner", 40, func(view *DetailView) {
			view.SetSplash(SplashInfo{Version: "v0.1.0", Workspace: testWorkspace})
		}},
		{"placeholder without a pull request", 30, func(*DetailView) {}},
		{"title with a long repository slug", 50, func(view *DetailView) {
			pr := newPR(12345, "a-really-quite-long-repository-slug-here", testPRTitle)
			view.SetPullRequest(&pr)
		}},
		{"description with an unbreakable token", 50, func(view *DetailView) {
			pr := detailPR()
			pr.Description = "See https://example.com/" + strings.Repeat("seg", 40)
			view.SetPullRequest(pr)
		}},
		{"repository metadata", 30, func(view *DetailView) {
			view.SetRepo(&RepoItem{
				Slug:      "a-really-quite-long-repository-slug-here",
				Name:      "API Gateway",
				URL:       "https://bitbucket.org/acme/a-really-quite-long-repository-slug-here",
				OpenCount: 3,
			})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			view := makeFocusedDetail()
			view.SetSize(tc.width, 24)
			tc.setup(view)

			for _, line := range strings.Split(view.View(), "\n") {
				if got := lipgloss.Width(line); got > tc.width {
					t.Errorf("line %q is %d columns wide, want at most %d", stripANSI(line), got, tc.width)
				}
			}
		})
	}
}

func TestDetailView_SetSplash_NarrowPanelDropsTheBanner(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetSize(40, 24)
	view.SetSplash(SplashInfo{Version: "v0.1.0"})

	out := stripANSI(view.View())
	if strings.Contains(out, "███") {
		t.Errorf("View() = %q, want the block banner dropped on a narrow panel", out)
	}
	if !strings.Contains(out, "lazybitbucket v0.1.0") {
		t.Errorf("View() = %q, want the plain heading instead", out)
	}
}

func TestDetailView_StepLog_LateResponseAfterBackIsDropped(t *testing.T) {
	t.Parallel()
	view := pipelineDetail(t)
	view.SetStepLogLoading(testPipelineUUID, testStepUUID)
	testkit.AssertEqual(t, "mode", view.Mode(), ModeStepLog)

	testkit.AssertEqual(t, "back out of the log", view.Back(), true)
	testkit.AssertEqual(t, "mode", view.Mode(), ModePipeline)

	// Everything below is a response to the fetch the user walked away from.
	view.SetStepLog(testPipelineUUID, testStepUUID, "late log")
	testkit.AssertEqual(t, "mode after the late log", view.Mode(), ModePipeline)

	view.SetStepLog(testPipelineUUID, testStepUUID2, "another step's log")
	testkit.AssertEqual(t, "mode after another step's log", view.Mode(), ModePipeline)

	view.SetStepLogError(testPipelineUUID, testStepUUID, "late failure")
	testkit.AssertEqual(t, "mode after the late error", view.Mode(), ModePipeline)

	out := stripANSI(view.View())
	for _, unwanted := range []string{"late log", "another step's log", "late failure"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("View() = %q, want %q dropped", out, unwanted)
		}
	}
}
