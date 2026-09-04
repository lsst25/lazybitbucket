package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/config"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// constructionFake is a fake that only expects the SetOnRequest wiring the
// constructor does; any other call fails the test.
func constructionFake(t *testing.T) (*bitbuckettest.FakeClient, *func(bitbucket.RequestLog)) {
	t.Helper()
	fake := &bitbuckettest.FakeClient{T: t}
	var captured func(bitbucket.RequestLog)
	fake.SetOnRequestFunc = func(fn func(bitbucket.RequestLog)) { captured = fn }
	return fake, &captured
}

func TestNewAppWithAuth_WiresPanesFromConfig(t *testing.T) {
	t.Parallel()
	fake, captured := constructionFake(t)
	cfg := testConfig()
	cfg.RendererStyle = config.RendererStyleNoTTY

	app := NewAppWithAuth(cfg, fake, AuthSaved)
	t.Cleanup(app.Shutdown)

	testkit.AssertEqual(t, "workspace", app.workspace, testWorkspace)
	testkit.AssertSliceEqual(t, "repo slugs", app.repoSlugs, []string{testRepo, testRepo2})
	testkit.AssertEqual(t, "side", app.side, sideLeft)
	testkit.AssertEqual(t, "leftFocus", app.leftFocus, focusPullRequests)
	testkit.AssertEqual(t, "auth method on the splash", app.splashInfo.AuthMethod, string(AuthSaved))
	testkit.AssertEqual(t, "detail starts on the splash", app.detailView.Mode(), views.ModeSplash)
	testkit.AssertEqual(t, "SetOnRequest calls", fake.SetOnRequestCalls, 1)
	if *captured == nil {
		t.Fatal("the request callback was not registered")
	}
	if _, ok := app.renderer.(views.GlamourRenderer); !ok {
		t.Errorf("renderer = %T, want views.GlamourRenderer", app.renderer)
	}
	testkit.AssertEqual(t, "auto-refresh disabled by config", app.refreshEvery, time.Duration(0))
}

func TestNewAppWithAuth_RequestCallbackFeedsRateLimitAndLog(t *testing.T) {
	t.Parallel()
	fake, captured := constructionFake(t)

	app := NewAppWithAuth(testConfig(), fake, AuthEnv)
	t.Cleanup(app.Shutdown)
	onRequest := *captured

	onRequest(bitbucket.RequestLog{Method: "GET", Path: "/x", Status: 200, RateLimitRemaining: -1})
	testkit.AssertEqual(t, "unknown rate limit stays unknown", app.rateLimit.Load(), int64(0))

	app.logFlag.Store(true)
	onRequest(bitbucket.RequestLog{
		Method: "GET", Path: "/y", Status: 200,
		Elapsed: time.Millisecond, RateLimitRemaining: 42,
	})
	testkit.AssertEqual(t, "rate limit stored as remaining+1", app.rateLimit.Load(), int64(43))
}

func TestNewApp_DefaultsToTheEnvironmentLabel(t *testing.T) {
	t.Parallel()
	fake, _ := constructionFake(t)

	app := NewApp(testConfig(), fake)
	t.Cleanup(app.Shutdown)

	testkit.AssertEqual(t, "auth method", app.splashInfo.AuthMethod, string(AuthEnv))
}

func TestNewAppWithAuth_DemoModeIsRecorded(t *testing.T) {
	t.Parallel()
	fake, _ := constructionFake(t)

	app := NewAppWithAuth(testConfig(), fake, AuthDemo)
	t.Cleanup(app.Shutdown)

	if !app.demoMode {
		t.Error("AuthDemo should put the app in demo mode")
	}
}

func TestNewAppWithAuth_WarnsWhenQuitIsShadowedEverywhere(t *testing.T) {
	t.Parallel()
	fake, _ := constructionFake(t)
	cfg := testConfig()
	contexts := []string{"pullrequests", "builds", "repos", "detail"}
	cfg.CustomCommands = []config.CustomCommandConfig{
		{Key: "q", Name: "shadow q", Command: "echo hi", Contexts: contexts},
		{Key: "ctrl+c", Name: "shadow ctrl c", Command: "echo hi", Contexts: contexts},
	}

	app := NewAppWithAuth(cfg, fake, AuthEnv)
	t.Cleanup(app.Shutdown)

	if !strings.Contains(app.statusPanel.ErrorMessage(), "unreachable") {
		t.Errorf("status error = %q, want the quit-unreachable warning", app.statusPanel.ErrorMessage())
	}
}

func TestNewAppWithAuth_WarnsWhenRefreshIsTooFrequent(t *testing.T) {
	t.Parallel()
	fake, _ := constructionFake(t)
	cfg := testConfig()
	cfg.RefreshInterval = "5s"
	cfg.Bitbucket.Repositories = []string{"a", "b", "c", "d"}

	app := NewAppWithAuth(cfg, fake, AuthEnv)
	t.Cleanup(app.Shutdown)

	if plain(app.statusPanel.View()) == "" {
		t.Fatal("status panel rendered nothing")
	}
	testkit.AssertEqual(t, "refresh interval parsed", app.refreshEvery, 5*time.Second)
}

func TestRefreshRateWarning(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		repos int
		every time.Duration
		want  bool
	}{
		{"refresh disabled", 10, 0, false},
		{"no repositories", 0, time.Second, false},
		{"comfortable", 3, 2 * time.Minute, false},
		{"too frequent", 4, 5 * time.Second, true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got := refreshRateWarning(testCase.repos, testCase.every) != ""
			testkit.AssertEqual(t, "warns", got, testCase.want)
		})
	}
}

func TestScopeLabel(t *testing.T) {
	t.Parallel()

	testkit.AssertEqual(t, "empty slug", scopeLabel(""), "all repos")
	testkit.AssertEqual(t, "a slug", scopeLabel(testRepo), testRepo)
}
