package tui

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

func TestInit_LoadsUserAndReposWithoutATick(t *testing.T) {
	t.Parallel()
	fake, _ := constructionFake(t)
	app := NewAppWithAuth(testConfig(), fake, AuthEnv)
	t.Cleanup(app.Shutdown)
	fake.GetCurrentUserFunc = func(context.Context) (*bitbucket.Account, error) {
		return &bitbucket.Account{UUID: testUserUUID, DisplayName: "Demo User"}, nil
	}

	msgs := collectMsgs(app.Init())

	var sawUser, sawRepos bool
	for _, msg := range msgs {
		switch msg.(type) {
		case currentUserLoadedMsg:
			sawUser = true
		case reposLoadedMsg:
			sawRepos = true
		case autoFetchTickMsg:
			t.Error("refreshInterval is off: Init must not arm a tick")
		}
	}
	if !sawUser || !sawRepos {
		t.Errorf("Init produced %d messages, want the user and the repositories", len(msgs))
	}
}

func TestInit_ArmsTheTickWhenRefreshIsEnabled(t *testing.T) {
	t.Parallel()
	fake, _ := constructionFake(t)
	cfg := testConfig()
	cfg.RefreshInterval = "1ms"
	app := NewAppWithAuth(cfg, fake, AuthEnv)
	t.Cleanup(app.Shutdown)
	fake.GetCurrentUserFunc = func(context.Context) (*bitbucket.Account, error) {
		return &bitbucket.Account{UUID: testUserUUID}, nil
	}

	testkit.AssertEqual(t, "refresh interval", app.refreshEvery, time.Millisecond)

	var sawTick bool
	for _, msg := range collectMsgs(app.Init()) {
		if _, ok := msg.(autoFetchTickMsg); ok {
			sawTick = true
		}
	}
	if !sawTick {
		t.Error("an enabled refreshInterval must arm the auto-fetch tick")
	}
}

func TestShutdown_IsIdempotent(t *testing.T) {
	t.Parallel()
	fake, _ := constructionFake(t)
	app := NewAppWithAuth(testConfig(), fake, AuthEnv)

	app.Shutdown()
	app.Shutdown()
}

func TestShutdown_ToleratesAHandBuiltApp(t *testing.T) {
	t.Parallel()
	app := newTestApp()

	app.Shutdown()
}

func TestContextAndNow_FallBackForHandBuiltApps(t *testing.T) {
	t.Parallel()
	app := newTestApp()

	if app.context() == nil {
		t.Error("context() must never return nil")
	}
	if app.nowFn().IsZero() {
		t.Error("nowFn() must return a real time when no clock is injected")
	}
}

func TestUpdate_ResizeLaysOutThePanels(t *testing.T) {
	t.Parallel()
	app := newAppWithFake(t, &bitbuckettest.FakeClient{T: t})

	_, cmd := app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	if cmd != nil {
		t.Error("a resize should produce no command")
	}
	testkit.AssertEqual(t, "width", app.width, 120)
	if app.panelSideW == 0 {
		t.Error("resize should have laid out the panels")
	}
}
