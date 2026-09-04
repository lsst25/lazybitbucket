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

	// Deterministic body rendering and an instant debounce, so runAll drains
	// the chain without sleeping.
	app.detailView = views.NewDetailView(views.PlainRenderer{})
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
