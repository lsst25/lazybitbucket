package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// keysApp has one pull request selected and its statuses cached.
func keysApp(t *testing.T) *App {
	t.Helper()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	pr := withAuthor(newPR(testRepo, 101, testTitle), testUserUUID)
	pr.Description = "See https://example.test/spec for the design."
	app.merged[testFetchKey] = []bitbucket.PullRequest{pr}
	app.prList.SetPullRequests(app.merged[testFetchKey])
	app.previewKey = pr.Key()
	app.prCache[pr.Key()] = &pr
	app.statusCache[pr.Key()] = statusEntry{
		statuses:  []bitbucket.CommitStatus{newStatus(testRepo, 412, bitbucket.StatusSuccessful)},
		updatedOn: pr.UpdatedOn,
		fetchedAt: testNow,
	}
	app.updateFocusState()
	return app
}

// captureExternal swaps the app's external-command seam and returns a pointer
// to the last URL or clipboard payload it saw. The seam is per-App, so tests
// using it stay parallel-safe.
func captureExternal(t *testing.T, app *App) (url, stdin *string) {
	t.Helper()
	var gotURL, gotStdin string
	app.exec = func(input string, _ bool, _ string, args ...string) {
		gotStdin = input
		if len(args) > 0 {
			gotURL = args[len(args)-1]
		}
	}
	return &gotURL, &gotStdin
}

func TestFocusActions_WalkTheLeftPanelsAndBack(t *testing.T) {
	t.Parallel()
	app := keysApp(t)

	steps := []focusPanel{focusBuilds, focusRepos, focusStatus, focusPullRequests}
	for _, want := range steps {
		if _, _, ok := app.handleFocusAction(ActFocusRight); !ok {
			t.Fatal("focus right must be handled on the left side")
		}
		testkit.AssertEqual(t, "leftFocus", app.leftFocus, want)
	}

	back := []focusPanel{focusStatus, focusRepos, focusBuilds, focusPullRequests}
	for _, want := range back {
		if _, _, ok := app.handleFocusAction(ActFocusLeft); !ok {
			t.Fatal("focus left must be handled on the left side")
		}
		testkit.AssertEqual(t, "leftFocus", app.leftFocus, want)
	}
}

func TestFocusActions_NumberKeys(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		action Action
		want   focusPanel
	}{
		{"status", ActFocusStatus, focusStatus},
		{"pull requests", ActFocusPullRequests, focusPullRequests},
		{"builds", ActFocusBuilds, focusBuilds},
		{"repos", ActFocusRepos, focusRepos},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			app := keysApp(t)
			app.side = sideRight

			_, _, ok := app.handleFocusAction(testCase.action)

			if !ok {
				t.Fatal("a focus action must be handled")
			}
			testkit.AssertEqual(t, "side", app.side, sideLeft)
			testkit.AssertEqual(t, "leftFocus", app.leftFocus, testCase.want)
		})
	}
}

func TestFocusStatus_ShowsTheSplash(t *testing.T) {
	t.Parallel()
	app := keysApp(t)
	app.currentUser = &bitbucket.Account{UUID: testUserUUID, DisplayName: "Demo User"}

	_, _, _ = app.handleFocusAction(ActFocusStatus)

	testkit.AssertEqual(t, "detail mode", app.detailView.Mode(), views.ModeSplash)
	testkit.AssertEqual(t, "splash user", app.splashInfo.User, "Demo User")
}

func TestSwitchPanel_TogglesSides(t *testing.T) {
	t.Parallel()
	app := keysApp(t)

	_, _, _ = app.handleFocusAction(ActSwitchPanel)
	testkit.AssertEqual(t, "side", app.side, sideRight)

	_, _, _ = app.handleFocusAction(ActSwitchPanel)
	testkit.AssertEqual(t, "side", app.side, sideLeft)
}

func TestActivate_OnThePullRequestListOpensTheDetail(t *testing.T) {
	t.Parallel()
	app := keysApp(t)
	// This is about the preview, not the review tabs: the prefetch focusing the
	// panel sets off has its own tests in review_prefetch_test.go.
	app.cfg.GUI.PrefetchDetailTabs = new(bool)

	_, cmd := app.handleActivate()

	testkit.AssertEqual(t, "side", app.side, sideRight)
	if cmd != nil {
		t.Error("the detail is already previewed; opening it costs nothing")
	}
}

func TestActivate_OnAnEmptyListDoesNothing(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})

	_, cmd := app.handleActivate()

	testkit.AssertEqual(t, "side", app.side, sideLeft)
	if cmd != nil {
		t.Error("there is nothing to open")
	}
}

func TestActivate_OnTheDetailPanelIsDispatchedToTheView(t *testing.T) {
	t.Parallel()
	app := keysApp(t)
	app.side = sideRight

	model, cmd := app.handleActivate()

	// The app consumes the key and asks the view to activate its selection, so
	// a rebound open/select key works on the detail panel too. The overview of
	// a pull request has nothing to open, so no command comes back.
	if model == nil {
		t.Fatal("activation on the detail panel must be handled, not dropped")
	}
	if cmd != nil {
		t.Error("the overview tab has nothing to activate")
	}
}

func TestBrowserAndCopy_TargetThePullRequest(t *testing.T) {
	t.Parallel()
	app := keysApp(t)
	gotURL, gotStdin := captureExternal(t, app)

	_, _, ok := app.handlePullRequestAction(ActBrowser)
	if !ok {
		t.Fatal("the browser action must be handled")
	}
	if !strings.Contains(*gotURL, "pull-requests") {
		t.Errorf("opened %q, want the pull request URL", *gotURL)
	}

	_, _, ok = app.handlePullRequestAction(ActCopyURL)
	if !ok {
		t.Fatal("the copy action must be handled")
	}
	if !strings.Contains(*gotStdin, "pull-requests") {
		t.Errorf("copied %q, want the pull request URL", *gotStdin)
	}
}

func TestURLPicker_ListsTheReachableURLsAndOpensThePick(t *testing.T) {
	t.Parallel()
	app := keysApp(t)
	gotURL, _ := captureExternal(t, app)

	_, _ = app.handleActionURLPicker()

	if !app.modal.IsVisible() {
		t.Fatal("the URL picker should have opened the modal")
	}
	if app.onSelect == nil {
		t.Fatal("the picker must install a selection callback")
	}
	rendered := plain(app.modal.View())
	if !strings.Contains(rendered, "Pull request") {
		t.Errorf("picker = %q, want the pull request section", rendered)
	}
	if !strings.Contains(rendered, "Description") {
		t.Errorf("picker = %q, want the description section for the URLs found in the body", rendered)
	}

	_, _ = app.Update(components.ModalSelectedMsg{
		Item: components.ModalItem{ID: "https://example.test/spec"},
	})
	testkit.AssertEqual(t, "opened url", *gotURL, "https://example.test/spec")
}

func TestURLPicker_WithoutAPullRequestIsANoop(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})

	_, cmd := app.handleActionURLPicker()

	if cmd != nil || app.modal.IsVisible() {
		t.Error("there are no URLs to pick from")
	}
}

func TestRefresh_InvalidatesTheSelectedPullRequestAndRefetches(t *testing.T) {
	t.Parallel()
	app := keysApp(t)

	_, cmd, ok := app.handlePullRequestAction(ActRefresh)

	if !ok || cmd == nil {
		t.Fatal("r should refetch the selected pull request")
	}
	if _, hit := app.prCache[testPRKey]; hit {
		t.Error("the detail cache should have been invalidated")
	}
	if _, hit := app.statusCache[testPRKey]; hit {
		t.Error("the status cache should have been invalidated")
	}
}

func TestRefreshAll_RefetchesTheTabEvenWhenItIsCached(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.ListPullRequestsFunc = func(_ context.Context, _, repo string,
		_ bitbucket.ListPullRequestsOptions,
	) ([]bitbucket.PullRequest, error) {
		return []bitbucket.PullRequest{newPR(repo, 101, testTitle)}, nil
	}
	app := keysApp(t)
	app.client = fake

	_, cmd, ok := app.handlePullRequestAction(ActRefreshAll)

	if !ok || cmd == nil {
		t.Fatal("R should always refetch the active tab")
	}
	if _, isLoaded := cmd().(pullRequestsLoadedMsg); !isLoaded {
		t.Error("R should have produced a list fetch")
	}
}

func TestHandleKeyMsg_UnknownKeyFallsThrough(t *testing.T) {
	t.Parallel()
	app := keysApp(t)

	model, cmd := app.handleKeyMsg(runeKey('z'))

	if model != nil || cmd != nil {
		t.Error("an unbound key must be left to the focused panel")
	}
}

func TestHandleKeyMsg_DetailScrollWorksFromTheLeftPanels(t *testing.T) {
	t.Parallel()
	app := keysApp(t)

	model, _ := app.handleKeyMsg(runeKey('J'))

	if model == nil {
		t.Error("J should scroll the detail pane from the pull request list")
	}
}

func TestHandleKeyMsg_HelpOverlayTakesEveryKey(t *testing.T) {
	t.Parallel()
	app := keysApp(t)
	app.showHelp = true

	_, _ = app.handleKeyMsg(tea.KeyMsg{Type: tea.KeyEsc})

	if app.showHelp {
		t.Error("esc should close the help overlay")
	}
}
