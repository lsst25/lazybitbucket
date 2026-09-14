package tui

import (
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// The three client methods one review prefetch asks for.
const (
	methodDiffStat = "ListPullRequestDiffStat"
	methodCommits  = "ListPullRequestCommits"
	methodComments = "ListPullRequestComments"
)

// namedTab pairs a tab with something a failure message can print: DetailTab is
// an integer at heart, so %v on it says nothing useful.
type namedTab struct {
	name string
	tab  views.DetailTab
}

func reviewTabs() []namedTab {
	return []namedTab{
		{"Files", views.TabFiles},
		{"Commits", views.TabCommits},
		{"Comments", views.TabComments},
	}
}

// stubReviewTabs answers all three review list endpoints, which is what one
// prefetch asks for.
func stubReviewTabs(fake *bitbuckettest.FakeClient) {
	stubDiffStat(fake)
	stubCommits(fake)
	stubComments(fake)
}

// detailFocusApp is reviewApp with the focus back on the pull request list,
// which is where every path that lands it on the detail panel starts.
func detailFocusApp(t *testing.T, fake *bitbuckettest.FakeClient) *App {
	t.Helper()
	app := reviewApp(t, fake)
	app.side = sideLeft
	app.leftFocus = focusPullRequests
	app.updateFocusState()
	return app
}

// assertReviewRequests checks the three review endpoints in one go.
func assertReviewRequests(t *testing.T, fake *bitbuckettest.FakeClient, what string, want int) {
	t.Helper()
	testkit.AssertEqual(t, what+": diffstat requests", fake.CallCount(methodDiffStat), want)
	testkit.AssertEqual(t, what+": commit requests", fake.CallCount(methodCommits), want)
	testkit.AssertEqual(t, what+": comment requests", fake.CallCount(methodComments), want)
}

func TestFocusDetail_WarmsTheThreeReviewTabsExactlyOnce(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubReviewTabs(fake)
	app := detailFocusApp(t, fake)

	_, cmd, handled := app.handleFocusAction(ActFocusDetail)
	if !handled {
		t.Fatal("the focus-detail action must be handled")
	}
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "side", app.side, sideRight)
	assertReviewRequests(t, fake, "on focus", 1)
	testkit.AssertEqual(t, "changed files", len(app.detailView.DiffStat()), 2)
	testkit.AssertEqual(t, "commits", len(app.detailView.Commits()), 1)
	testkit.AssertEqual(t, "threads", len(app.detailView.Threads()), 1)

	// Walking the tabs afterwards is served from the cache.
	for _, entry := range reviewTabs() {
		runAll(t, app, app.jumpToTab(entry.tab))
	}
	assertReviewRequests(t, fake, "after walking the tabs", 1)
	testkit.AssertEqual(t, "nothing left in flight", len(app.reviewInFlight), 0)
}

func TestFocusDetail_ATabEnteredMidPrefetchWaitsForTheAnswer(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubReviewTabs(fake)
	app := detailFocusApp(t, fake)

	// The answers are collected rather than fed back, so all three fetches are
	// still in flight when the reader presses a tab key.
	_, cmd, _ := app.handleFocusAction(ActFocusDetail)
	msgs := collectMsgs(cmd)
	testkit.AssertEqual(t, "prefetch answers", len(msgs), 3)
	assertReviewRequests(t, fake, "on focus", 1)
	testkit.AssertEqual(t, "marks in flight", len(app.reviewInFlight), 3)

	for _, entry := range reviewTabs() {
		if tabCmd := app.jumpToTab(entry.tab); tabCmd != nil {
			t.Errorf("the %s fetch was already in flight and must not be issued again", entry.name)
		}
	}
	assertReviewRequests(t, fake, "after the tab keys", 1)

	// The answers still land, and they are still the fresh ones.
	for _, msg := range msgs {
		_, _ = app.Update(msg)
	}
	testkit.AssertEqual(t, "nothing left in flight", len(app.reviewInFlight), 0)
	testkit.AssertEqual(t, "changed files", len(app.detailView.DiffStat()), 2)
	testkit.AssertEqual(t, "commits", len(app.detailView.Commits()), 1)
	testkit.AssertEqual(t, "threads", len(app.detailView.Threads()), 1)
}

func TestFocusDetail_EveryPathOntoThePanelPrefetches(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		focus func(app *App) tea.Cmd
	}{
		{
			name:  "the focus-detail key",
			focus: func(app *App) tea.Cmd { _, cmd, _ := app.handleFocusAction(ActFocusDetail); return cmd },
		},
		{
			name:  "the switch-panel key",
			focus: func(app *App) tea.Cmd { _, cmd, _ := app.handleFocusAction(ActSwitchPanel); return cmd },
		},
		{
			name:  "enter on the pull request list",
			focus: func(app *App) tea.Cmd { _, cmd := app.handleActivate(); return cmd },
		},
		{
			name: "a click on the detail panel",
			focus: func(app *App) tea.Cmd {
				_, cmd := app.mouseClick(panelDetail, 3, app.panelSideW+2)
				return cmd
			},
		},
		{
			name: "the wheel over the detail panel",
			focus: func(app *App) tea.Cmd {
				_, cmd := app.mouseScroll(panelDetail, 3)
				return cmd
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			fake := &bitbuckettest.FakeClient{T: t}
			stubReviewTabs(fake)
			app := detailFocusApp(t, fake)

			runAll(t, app, testCase.focus(app))

			testkit.AssertEqual(t, "side", app.side, sideRight)
			assertReviewRequests(t, fake, "on focus", 1)
		})
	}
}

func TestFocusDetail_PrefetchOffLeavesEachTabToWarmItself(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	// Only the Files endpoint is stubbed: the strict fake fails the test if the
	// prefetch asks for the other two anyway.
	stubDiffStat(fake)
	app := detailFocusApp(t, fake)
	app.cfg.GUI.PrefetchDetailTabs = new(bool)

	_, cmd, _ := app.handleFocusAction(ActFocusDetail)
	runAll(t, app, cmd)
	assertReviewRequests(t, fake, "with the prefetch off", 0)

	runAll(t, app, app.jumpToTab(views.TabFiles))

	testkit.AssertEqual(t, "diffstat requests", fake.CallCount(methodDiffStat), 1)
	testkit.AssertEqual(t, "commit requests", fake.CallCount(methodCommits), 0)
	testkit.AssertEqual(t, "comment requests", fake.CallCount(methodComments), 0)
	testkit.AssertEqual(t, "changed files", len(app.detailView.DiffStat()), 2)
}

func TestFocusDetail_WithNoPullRequestFetchesNothing(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	app := sizedApp(t, fake)

	_, cmd, handled := app.handleFocusAction(ActFocusDetail)

	if !handled {
		t.Fatal("the focus-detail action must be handled")
	}
	if cmd != nil {
		t.Error("there is no pull request to warm, so focusing the panel costs nothing")
	}
	assertReviewRequests(t, fake, "with nothing selected", 0)
}

func TestFocusDetail_OnAPipelineFetchesNoReviewData(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubPipeline(fake)
	app := detailFocusApp(t, fake)
	status := newStatus(testRepo, testBuild, bitbucket.StatusSuccessful)

	_, cmd := app.openBuild(&status)
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "side", app.side, sideRight)
	testkit.AssertEqual(t, "detail mode", app.detailView.Mode(), views.ModePipeline)
	assertReviewRequests(t, fake, "drilling into a build", 0)

	// The focus is already on the panel, and the panel is still showing the
	// run, so another focus action does not smuggle the prefetch in either.
	_, focusCmd, _ := app.handleFocusAction(ActFocusDetail)
	runAll(t, app, focusCmd)
	assertReviewRequests(t, fake, "refocusing the run", 0)
}

func TestReviewInFlight_ClearedWhenTheAnswerLands(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubComments(fake)
	app := reviewApp(t, fake)
	pr := app.currentPullRequest()

	runAll(t, app, app.ensureThreads(pr))

	testkit.AssertEqual(t, "comment requests", fake.CallCount(methodComments), 1)
	testkit.AssertEqual(t, "nothing left in flight", len(app.reviewInFlight), 0)

	// A dropped cache fetches again rather than believing a stale mark.
	app.dropReviewCaches(testPRKey)
	runAll(t, app, app.ensureThreads(pr))

	testkit.AssertEqual(t, "comment requests after the drop", fake.CallCount(methodComments), 2)
	testkit.AssertEqual(t, "nothing left in flight", len(app.reviewInFlight), 0)
}

// countingDemoApp is demoApp with the client's request callback replaced by a
// counter, so a test can say how many times each endpoint was actually asked.
// The app's own callback only feeds the rate-limit gauge and the request log,
// neither of which these tests look at.
func countingDemoApp(t *testing.T) (*App, func(substr string) int) {
	t.Helper()
	app := demoApp(t)

	var mu sync.Mutex
	var paths []string
	app.client.SetOnRequest(func(entry bitbucket.RequestLog) {
		mu.Lock()
		defer mu.Unlock()
		paths = append(paths, entry.Path)
	})

	return app, func(substr string) int {
		mu.Lock()
		defer mu.Unlock()
		count := 0
		for _, path := range paths {
			if strings.Contains(path, substr) {
				count++
			}
		}
		return count
	}
}

func TestDemo_FocusingTheDetailPanelWarmsTheThreeTabsOnce(t *testing.T) {
	t.Parallel()
	app, requests := countingDemoApp(t)
	if !app.prList.SelectByKey(demoPRKey103) {
		t.Fatalf("%s should be in the Open tab", demoPRKey103)
	}
	runAll(t, app, app.previewSelected())

	_, cmd, _ := app.handleFocusAction(ActFocusDetail)
	runAll(t, app, cmd)

	prefix := "/repositories/acme/api-gateway/pullrequests/103/"
	for _, endpoint := range []string{"diffstat", "commits", "comments"} {
		testkit.AssertEqual(t, endpoint+" requests", requests(prefix+endpoint), 1)
	}

	// Walking the three tabs afterwards asks for nothing more.
	for _, entry := range reviewTabs() {
		runAll(t, app, app.jumpToTab(entry.tab))
	}
	for _, endpoint := range []string{"diffstat", "commits", "comments"} {
		testkit.AssertEqual(t, endpoint+" requests after the walk", requests(prefix+endpoint), 1)
	}
}
