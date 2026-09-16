package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/demo"
	"github.com/lsst25/lazybitbucket/pkg/config"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// demoApp stands a real bitbucket.Client in front of the demo server and runs
// the app's whole startup chain synchronously: no timers, no goroutine waits.
func demoApp(t *testing.T) *App {
	t.Helper()

	server, err := demo.NewServer()
	if err != nil {
		t.Fatalf("start demo server: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := server.Close(); closeErr != nil {
			t.Errorf("close demo server: %v", closeErr)
		}
	})

	cfg := config.DefaultConfig()
	cfg.Bitbucket.Workspace = demo.Workspace
	cfg.Bitbucket.Repositories = []string{"api-gateway", "web-app", "infra"}
	cfg.RefreshInterval = "off"
	cfg.RendererStyle = config.RendererStyleNoTTY

	client := bitbucket.NewClient(bitbucket.ClientOpts{BaseURL: server.URL, Token: "demo-token"})
	app := NewAppWithAuth(cfg, client, AuthDemo)
	t.Cleanup(app.Shutdown)

	// A plain Markdown renderer for deterministic bodies and an instant
	// debounce so runAll drains the chain without sleeping. The code style
	// stays on: the diff view's highlighting is part of what these tests check.
	app.detailView = views.NewDetailView(views.PlainRenderer{}, "monokai")
	app.detailView.ResolveNav = app.keymap.MatchNav
	app.debounce = time.Nanosecond

	_, _ = app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	runAll(t, app, app.Init())
	return app
}

func TestDemo_StartupShowsTheOpenPullRequests(t *testing.T) {
	t.Parallel()
	app := demoApp(t)

	rendered := plain(app.View())

	for _, want := range []string{"#101", "api-gate", "Open", "Mine", "Reviewing"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("view is missing %q:\n%s", want, rendered)
		}
	}
	testkit.AssertEqual(t, "workspace", app.workspace, demo.Workspace)
	testkit.AssertEqual(t, "current user", app.currentUser.DisplayName, "Demo User")
}

func TestDemo_ApprovalsColumnCountsTheReviewers(t *testing.T) {
	t.Parallel()
	app := demoApp(t)

	if !app.prList.SelectByKey("acme/api-gateway#101") {
		t.Fatal("#101 should be in the Open tab")
	}
	rendered := plain(app.prList.View())

	if !strings.Contains(rendered, "✓1") {
		t.Errorf("the approvals column should show one approval:\n%s", rendered)
	}
}

func TestDemo_DetailShowsTheMarkdownDescription(t *testing.T) {
	t.Parallel()
	app := demoApp(t)

	if !app.prList.SelectByKey("acme/api-gateway#101") {
		t.Fatal("#101 should be in the Open tab")
	}
	runAll(t, app, app.previewSelected())

	rendered := plain(app.detailView.View())
	if !strings.Contains(rendered, "Summary") {
		t.Errorf("detail is missing the description heading:\n%s", rendered)
	}
	if !strings.Contains(rendered, "#101") {
		t.Errorf("detail is missing the pull request id:\n%s", rendered)
	}
}

func TestDemo_BuildsPanelShowsTheStatus(t *testing.T) {
	t.Parallel()
	app := demoApp(t)

	if !app.prList.SelectByKey("acme/api-gateway#101") {
		t.Fatal("#101 should be in the Open tab")
	}
	runAll(t, app, app.previewSelected())

	rendered := plain(app.buildsPanel.View())
	if !strings.Contains(rendered, "feature/rate-limiter") {
		t.Errorf("builds panel is missing the build row:\n%s", rendered)
	}
	testkit.AssertEqual(t, "rolled up state",
		bitbucket.RollupStatuses(app.buildsPanel.Statuses()), bitbucket.StatusSuccessful)
}

func TestDemo_DrillingIntoABuildRendersThePipeline(t *testing.T) {
	t.Parallel()
	app := demoApp(t)

	if !app.prList.SelectByKey("acme/api-gateway#101") {
		t.Fatal("#101 should be in the Open tab")
	}
	runAll(t, app, app.previewSelected())

	app.leftFocus = focusBuilds
	app.updateFocusState()
	_, cmd := app.handleActivate()
	runAll(t, app, cmd)

	rendered := plain(app.detailView.View())
	if !strings.Contains(rendered, "Pipeline #") {
		t.Errorf("detail is missing the pipeline header:\n%s", rendered)
	}
	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModePipeline)

	if !app.detailView.Back() {
		t.Error("Back() should walk out of the pipeline view")
	}
	testkit.AssertEqual(t, "mode after back", app.detailView.Mode(), views.ModePullRequest)
}

func TestDemo_MineAndReviewingFilterByTheDemoUser(t *testing.T) {
	t.Parallel()
	app := demoApp(t)

	openCount := app.prList.ItemCount()
	if openCount < 3 {
		t.Fatalf("the Open tab has %d rows, want the demo's open pull requests", openCount)
	}

	app.prList.SetTabIndex(1) // Mine
	mine := app.prList.CurrentPullRequests()
	if len(mine) == 0 || len(mine) >= openCount {
		t.Fatalf("Mine has %d rows, want a strict subset of %d", len(mine), openCount)
	}
	for _, pr := range mine {
		if !pr.IsAuthor(app.currentUser.UUID) {
			t.Errorf("Mine holds #%d authored by %s", pr.ID, pr.Author.Name())
		}
	}

	app.prList.SetTabIndex(2) // Reviewing
	reviewing := app.prList.CurrentPullRequests()
	if len(reviewing) == 0 {
		t.Fatal("Reviewing is empty; the demo user reviews #102 and #103")
	}
	for _, pr := range reviewing {
		if !pr.IsReviewer(app.currentUser.UUID) {
			t.Errorf("Reviewing holds #%d, which the demo user does not review", pr.ID)
		}
	}
}

func TestDemo_RepoFilterNarrowsTheList(t *testing.T) {
	t.Parallel()
	app := demoApp(t)
	all := app.prList.ItemCount()

	runAll(t, app, app.selectRepo(&views.RepoItem{Slug: "web-app", Name: "web-app"}))

	filtered := app.prList.ItemCount()
	if filtered == 0 || filtered >= all {
		t.Fatalf("filtered rows = %d, want a strict subset of %d", filtered, all)
	}
	if !containsPlain(app.View(), "web-app") {
		t.Error("the status panel should be scoped to the repository")
	}
}

func TestDemo_OneRequestPerRepositoryForTheThreeDefaultTabs(t *testing.T) {
	t.Parallel()
	app := demoApp(t)

	// All three default tabs share the OPEN fetch key, so the startup cost is
	// one list request per repository.
	testkit.AssertEqual(t, "merged fetch keys", len(app.merged), 1)
	testkit.AssertEqual(t, "nothing left in flight", len(app.inFlight), 0)

	app.prList.SetTabIndex(1)
	if !app.prList.HasCachedTab() {
		t.Error("the Mine tab must be served from the shared fetch, not a new request")
	}
}

// demoReviewApp is demoApp with one pull request previewed and the detail panel
// focused, which is where every review-tab walk starts.
func demoReviewApp(t *testing.T, key string) *App {
	t.Helper()
	app := demoApp(t)
	if !app.prList.SelectByKey(key) {
		t.Fatalf("%s should be in the Open tab", key)
	}
	runAll(t, app, app.previewSelected())
	app.side = sideRight
	app.updateFocusState()
	return app
}

func TestDemo_FilesTabShowsTheDiffstat(t *testing.T) {
	t.Parallel()
	app := demoReviewApp(t, "acme/api-gateway#103")

	runAll(t, app, app.jumpToTab(views.TabFiles))

	rendered := plain(app.detailView.View())
	for _, want := range []string{"internal/upstream/retry.go", "docs/rfc-014.md", "Files (3)"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("the Files tab is missing %q:\n%s", want, rendered)
		}
	}
	// The counts come from the demo's own diff, so they cannot drift.
	if !strings.Contains(rendered, "+") || !strings.Contains(rendered, "−") {
		t.Errorf("the changed-file rows should carry +/− counts:\n%s", rendered)
	}
}

func TestDemo_EnterOnAFileRendersAHighlightedDiffWithAMarker(t *testing.T) {
	t.Parallel()
	app := demoReviewApp(t, "acme/api-gateway#103")
	runAll(t, app, app.jumpToTab(views.TabFiles))
	_ = app.detailView.View()

	_, cmd := app.detailView.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter on a changed file should open its diff")
	}
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModeFileDiff)
	testkit.AssertEqual(t, "file on screen", app.detailView.FileDiffPath(), "internal/upstream/retry.go")

	raw := app.detailView.View()
	if !strings.Contains(raw, "\x1b[") {
		t.Error("the diff's code column should be syntax highlighted")
	}
	rendered := plain(raw)
	if !strings.Contains(rendered, "●") {
		t.Errorf("the commented line should carry a marker:\n%s", rendered)
	}
	if !strings.Contains(rendered, "@@") {
		t.Errorf("the diff should show its hunk header:\n%s", rendered)
	}
}

func TestDemo_NextFileMovesToTheFollowingDiff(t *testing.T) {
	t.Parallel()
	app := demoReviewApp(t, "acme/api-gateway#103")
	runAll(t, app, app.jumpToTab(views.TabFiles))
	runAll(t, app, app.openFileDiff("internal/upstream/retry.go"))

	_, cmd, handled := app.handleReviewAction(ActNextFile)
	if !handled {
		t.Fatal("the next-file action must be handled")
	}
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModeFileDiff)
	testkit.AssertEqual(t, "file on screen",
		app.detailView.FileDiffPath(), "internal/upstream/retry_test.go")
}

func TestDemo_EnterOnACommentedLineLandsOnTheThread(t *testing.T) {
	t.Parallel()
	app := demoReviewApp(t, "acme/api-gateway#103")
	runAll(t, app, app.jumpToTab(views.TabFiles))
	runAll(t, app, app.openFileDiff("internal/upstream/retry.go"))
	_ = app.detailView.View()

	// The demo's first inline thread is anchored to new-side line 16.
	found := false
	for range 40 {
		row, ok := app.detailView.SelectedDiffLine()
		if ok && row.NewNo == 16 {
			found = true
			break
		}
		app.detailView.ScrollBy(1)
	}
	if !found {
		t.Fatal("no diff row with new-side line 16")
	}

	_ = app.detailView.View()
	_, cmd := app.detailView.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter on the commented line should jump to its thread")
	}
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModePullRequest)
	testkit.AssertEqual(t, "tab", app.detailView.ActiveTab(), views.TabComments)
	thread := app.detailView.SelectedThread()
	if thread == nil {
		t.Fatal("the cursor should be on a thread")
	}
	testkit.AssertEqual(t, "thread path", thread.Path(), "internal/upstream/retry.go")
}

func TestDemo_CommitsTabListsTheCommits(t *testing.T) {
	t.Parallel()
	app := demoReviewApp(t, "acme/api-gateway#103")

	runAll(t, app, app.jumpToTab(views.TabCommits))

	commits := app.detailView.Commits()
	if len(commits) == 0 {
		t.Fatal("the demo pull requests carry commits")
	}
	rendered := plain(app.detailView.View())
	if !strings.Contains(rendered, commits[0].ShortHash()) {
		t.Errorf("the Commits tab is missing the short hash:\n%s", rendered)
	}
	if !strings.Contains(rendered, commits[0].Subject()) {
		t.Errorf("the Commits tab is missing the subject:\n%s", rendered)
	}
}

func TestDemo_CommentsTabHidesTheResolvedThreadUntilAsked(t *testing.T) {
	t.Parallel()
	app := demoReviewApp(t, "acme/api-gateway#103")

	runAll(t, app, app.jumpToTab(views.TabComments))

	// A resolved conversation is out of the way from the start, and the tab
	// says so rather than leaving a reader to wonder.
	rendered := plain(app.detailView.View())
	if !strings.Contains(rendered, "1 hidden") {
		t.Errorf("the tab should say how many are hidden:\n%s", rendered)
	}
	if strings.Contains(rendered, "resolved by") {
		t.Errorf("no resolved conversation should be listed:\n%s", rendered)
	}
	if !strings.Contains(rendered, "internal/upstream/retry.go") {
		t.Errorf("an inline thread should name its file:\n%s", rendered)
	}

	// f asks for them, and they come back marked.
	runAll(t, app, app.handleActionHideResolved())
	rendered = plain(app.detailView.View())
	if !strings.Contains(rendered, "✔ ") || !strings.Contains(rendered, "resolved by") {
		t.Errorf("a shown resolved conversation should be marked:\n%s", rendered)
	}
}

func TestDemo_TooLargeDiffSaysToOpenTheBrowser(t *testing.T) {
	t.Parallel()
	app := demoApp(t)
	// #98 is merged, so it is only in a tab that asks for merged pull requests;
	// the review fetches work off the previewed pull request, not the list.
	pr, err := app.client.GetPullRequest(t.Context(), demo.Workspace, "api-gateway", 98)
	if err != nil {
		t.Fatalf("get #98: %v", err)
	}
	app.prCache[pr.Key()] = pr
	app.previewKey = pr.Key()
	app.detailView.SetPullRequest(pr)
	app.side = sideRight
	app.updateFocusState()

	runAll(t, app, app.jumpToTab(views.TabFiles))

	rendered := plain(app.detailView.View())
	if !strings.Contains(rendered, "too large — open in browser (o)") {
		t.Errorf("the Files tab should name the way out of a 555:\n%s", rendered)
	}
}

// demoPRKey101 and friends are the demo fixtures the write flows act on.
const (
	demoPRKey101 = "acme/api-gateway#101"
	demoPRKey103 = "acme/api-gateway#103"
	demoPRKey41  = "acme/web-app#41"
)

// confirmDialog presses enter on the open dialog and drains everything the
// confirmation fans out into.
func confirmDialog(t *testing.T, app *App) {
	t.Helper()
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	runAll(t, app, cmd)
}

func TestDemo_MergingLeavesTheOpenTabAndShowsMerged(t *testing.T) {
	t.Parallel()
	app := demoReviewApp(t, demoPRKey101)

	_, _ = app.Update(runeKey('M'))
	if !app.form.IsVisible() {
		t.Fatal("M should open the merge dialog")
	}
	if !strings.Contains(plain(app.form.View()), "Merge #101 into main") {
		t.Errorf("dialog = %q, want the merge title", plain(app.form.View()))
	}
	// The write and the list refetch it triggers are driven in two steps: the
	// refetch drops #101 from the Open tab and moves the cursor, so what the
	// detail panel says about #101 has to be read before that lands.
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	confirmed := collectMsgs(cmd)
	if len(confirmed) != 1 {
		t.Fatalf("messages = %v, want the form confirmation", confirmed)
	}
	_, cmd = app.Update(confirmed[0])
	written := collectMsgs(cmd)
	if len(written) != 1 {
		t.Fatalf("messages = %v, want the merge result", written)
	}
	_, cmd = app.Update(written[0])

	if !strings.Contains(plain(app.detailView.View()), "MERGED") {
		t.Errorf("the detail panel should show the merged state:\n%s", plain(app.detailView.View()))
	}
	if !strings.Contains(plain(app.helpBar.View()), "merged #101") {
		t.Errorf("help bar = %q, want the success line", plain(app.helpBar.View()))
	}

	runAll(t, app, cmd)
	// The refetch is the truth about which pull requests are open: nothing
	// filters by state client side.
	for _, pr := range app.prList.CurrentPullRequests() {
		if pr.Key() == demoPRKey101 {
			t.Error("a merged pull request must not survive the Open tab's refetch")
		}
	}
}

func TestDemo_MergingAPullRequestWhoseRefsMovedSaysSo(t *testing.T) {
	t.Parallel()
	app := demoApp(t)
	if !app.prList.SelectByKey(demoPRKey41) {
		t.Fatalf("%s should be in the Open tab", demoPRKey41)
	}
	runAll(t, app, app.previewSelected())

	_, _ = app.Update(runeKey('M'))
	confirmDialog(t, app)

	if got := plain(app.helpBar.View()); !strings.Contains(got, "merge failed: refs changed — refresh and retry") {
		t.Errorf("help bar = %q, want the 409 explanation", got)
	}
	if _, busy := app.writesInFlight[demoPRKey41]; busy {
		t.Error("a failed merge must clear its in-flight record")
	}
}

func TestDemo_TogglingDraftMarksThePullRequest(t *testing.T) {
	t.Parallel()
	app := demoReviewApp(t, demoPRKey103)

	_, _ = app.Update(runeKey('D'))
	if !strings.Contains(plain(app.form.View()), "Mark #103 as draft?") {
		t.Fatalf("dialog = %q, want the draft question", plain(app.form.View()))
	}
	confirmDialog(t, app)

	if !strings.Contains(plain(app.detailView.View()), "· draft") {
		t.Errorf("the detail panel should mark the pull request a draft:\n%s", plain(app.detailView.View()))
	}
	if !app.prCache[demoPRKey103].Draft {
		t.Error("the cached pull request should carry the server's new draft flag")
	}
}

func TestDemo_EditingTheTitleRoundTrips(t *testing.T) {
	t.Parallel()
	app := demoReviewApp(t, demoPRKey101)
	const newTitle = "Rate limit the public routes"

	_, _ = app.Update(runeKey('E'))
	if !app.modal.IsVisible() {
		t.Fatal("E should open the edit chooser")
	}
	// Enter picks the first row, which is Title.
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	runAll(t, app, cmd)
	if !app.input.IsVisible() {
		t.Fatal("the title editor should be open")
	}

	_, _ = app.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	_, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(newTitle)})
	_, cmd = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "cached title", app.prCache[demoPRKey101].Title, newTitle)
	if !strings.Contains(plain(app.detailView.View()), newTitle) {
		t.Errorf("the detail panel should show the new title:\n%s", plain(app.detailView.View()))
	}
	// The server is the truth: read it back rather than trusting the cache.
	fetched, err := app.client.GetPullRequest(t.Context(), demo.Workspace, "api-gateway", 101)
	if err != nil {
		t.Fatalf("get #101: %v", err)
	}
	testkit.AssertEqual(t, "server title", fetched.Title, newTitle)
	testkit.AssertEqual(t, "reviewers kept", len(fetched.Reviewers), 2)
}
