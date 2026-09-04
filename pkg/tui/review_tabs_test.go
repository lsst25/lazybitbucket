package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/diff"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// The review fixtures. testDiffText is a real two-file unified diff so the
// parser, the diffstat and the comment anchors below cannot drift apart: the
// thread on testFileGo is anchored to new-side line 13, which is the second of
// the two added lines.
const testDiffText = "diff --git a/internal/rate/bucket.go b/internal/rate/bucket.go\n" +
	"index 1111111..2222222 100644\n" +
	"--- a/internal/rate/bucket.go\n" +
	"+++ b/internal/rate/bucket.go\n" +
	"@@ -10,4 +10,5 @@ func (b *Bucket) Allow() bool {\n" +
	" \tb.mu.Lock()\n" +
	" \tdefer b.mu.Unlock()\n" +
	"-\treturn b.tokens > 0\n" +
	"+\tb.refill()\n" +
	"+\treturn b.tokens >= 1\n" +
	" }\n" +
	"diff --git a/docs/rate.md b/docs/rate.md\n" +
	"new file mode 100644\n" +
	"index 0000000..3333333\n" +
	"--- /dev/null\n" +
	"+++ b/docs/rate.md\n" +
	"@@ -0,0 +1,2 @@\n" +
	"+# Rate limiting\n" +
	"+A token bucket per client.\n"

// testDiffStat is the changed-file list matching testDiffText.
func testDiffStat() []bitbucket.DiffStatEntry {
	return []bitbucket.DiffStatEntry{
		{
			Type: "diffstat", Status: bitbucket.DiffStatModified,
			LinesAdded: 2, LinesRemoved: 1,
			Old: &bitbucket.CommitFile{Path: testFileGo, EscapedPath: testFileGo},
			New: &bitbucket.CommitFile{Path: testFileGo, EscapedPath: testFileGo},
		},
		{
			Type: "diffstat", Status: bitbucket.DiffStatAdded,
			LinesAdded: 2, LinesRemoved: 0,
			Old: nil,
			New: &bitbucket.CommitFile{Path: testFileMD, EscapedPath: testFileMD},
		},
	}
}

// testCommits is a one-commit history for the Commits tab.
func testCommits() []bitbucket.Commit {
	commit := bitbucket.Commit{
		Type:    "commit",
		Hash:    testCommitHash,
		Date:    testNow.Add(-3 * time.Hour),
		Message: "Refill the bucket before checking it\n\nThe old check never refilled.\n",
		Author:  bitbucket.CommitAuthor{Type: "author", Raw: "Grace Hopper <grace@example.test>", User: nil},
	}
	commit.Links.HTML = bitbucket.Link{Href: "https://bitbucket.org/acme/api-gateway/commits/" + testCommitHash}
	return []bitbucket.Commit{commit}
}

// testThreads is one inline thread anchored to the second added line of
// testFileGo, which is what puts a marker in the diff view.
func testThreads() []bitbucket.Thread {
	root := bitbucket.Comment{
		Type:      "pullrequest_comment",
		ID:        testThreadRoot,
		CreatedOn: testNow.Add(-time.Hour),
		UpdatedOn: testNow.Add(-time.Hour),
		Content:   bitbucket.RenderedContent{Raw: "Is one token enough here?"},
		User:      bitbucket.Account{UUID: testOtherUUID, DisplayName: "Ada Lovelace"},
		Inline:    &bitbucket.CommentInline{Path: testFileGo, From: nil, To: ptrInt(13)},
	}
	root.Links.HTML = bitbucket.Link{Href: "https://bitbucket.org/acme/api-gateway/pull-requests/101#comment-9001"}
	return []bitbucket.Thread{{Root: root, Replies: nil}}
}

func ptrInt(v int) *int { return &v }

// parseFixtureDiff is testDiffText through the real parser, which is what the
// fetch command hands the view.
func parseFixtureDiff(t *testing.T) ([]diff.File, error) {
	t.Helper()
	return diff.Parse(testDiffText)
}

// withSource stamps the source commit the review caches are keyed against.
func withSource(pr bitbucket.PullRequest, hash string) bitbucket.PullRequest {
	pr.Source.Commit = &bitbucket.CommitRef{Hash: hash}
	return pr
}

// reviewApp is the canonical app for the review tabs: one pull request
// selected, the detail panel focused and showing it, and a fake that fails
// loudly on any client call the test did not stub.
func reviewApp(t *testing.T, fake *bitbuckettest.FakeClient) *App {
	t.Helper()
	app := sizedApp(t, fake)
	pr := withSource(withAuthor(newPR(testRepo, 101, testTitle), testUserUUID), testSourceHash)
	app.merged[testFetchKey] = []bitbucket.PullRequest{pr}
	app.prList.SetPullRequests(app.merged[testFetchKey])
	app.prCache[pr.Key()] = &pr
	app.previewKey = pr.Key()
	// Cached statuses plus the review data withAuthor stamps mean a re-preview
	// costs no request, so a test can call previewSelected without stubbing the
	// selection flow.
	app.statusCache[pr.Key()] = statusEntry{statuses: nil, updatedOn: pr.UpdatedOn, fetchedAt: testNow}
	app.detailView.SetPullRequest(&pr)
	app.side = sideRight
	app.updateFocusState()
	return app
}

// stubDiffStat, stubCommits, stubComments and stubDiff are the four review
// endpoints, each answering the fixtures above.
func stubDiffStat(fake *bitbuckettest.FakeClient) {
	fake.ListPullRequestDiffStatFunc = func(context.Context, string, string, int) ([]bitbucket.DiffStatEntry, error) {
		return testDiffStat(), nil
	}
}

func stubCommits(fake *bitbuckettest.FakeClient) {
	fake.ListPullRequestCommitsFunc = func(context.Context, string, string, int,
		bitbucket.ListOptions,
	) ([]bitbucket.Commit, error) {
		return testCommits(), nil
	}
}

func stubComments(fake *bitbuckettest.FakeClient) {
	fake.ListPullRequestCommentsFunc = func(context.Context, string, string, int,
		bitbucket.ListOptions,
	) ([]bitbucket.Comment, error) {
		return []bitbucket.Comment{testThreads()[0].Root}, nil
	}
}

func stubDiff(fake *bitbuckettest.FakeClient) {
	fake.GetPullRequestDiffFunc = func(_ context.Context, _, _ string, _ int,
		_ bitbucket.DiffOptions,
	) (string, error) {
		return testDiffText, nil
	}
}

func TestDetailTabEntry_FetchesEachTabExactlyOnce(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		tab    views.DetailTab
		stub   func(*bitbuckettest.FakeClient)
		method string
		assert func(t *testing.T, app *App)
	}{
		{
			name: "files", tab: views.TabFiles, stub: stubDiffStat,
			method: "ListPullRequestDiffStat",
			assert: func(t *testing.T, app *App) {
				t.Helper()
				testkit.AssertEqual(t, "changed files", len(app.detailView.DiffStat()), 2)
			},
		},
		{
			name: "commits", tab: views.TabCommits, stub: stubCommits,
			method: "ListPullRequestCommits",
			assert: func(t *testing.T, app *App) {
				t.Helper()
				testkit.AssertEqual(t, "commits", len(app.detailView.Commits()), 1)
			},
		},
		{
			name: "comments", tab: views.TabComments, stub: stubComments,
			method: "ListPullRequestComments",
			assert: func(t *testing.T, app *App) {
				t.Helper()
				testkit.AssertEqual(t, "threads", len(app.detailView.Threads()), 1)
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			fake := &bitbuckettest.FakeClient{T: t}
			testCase.stub(fake)
			app := reviewApp(t, fake)

			runAll(t, app, app.jumpToTab(testCase.tab))

			testkit.AssertEqual(t, "requests", fake.CallCount(testCase.method), 1)
			testCase.assert(t, app)

			// Leaving and coming back is served from the cache.
			runAll(t, app, app.jumpToTab(views.TabOverview))
			runAll(t, app, app.jumpToTab(testCase.tab))
			testkit.AssertEqual(t, "requests after a revisit", fake.CallCount(testCase.method), 1)
			testCase.assert(t, app)
		})
	}
}

func TestDetailTabEntry_OverviewCostsNothing(t *testing.T) {
	t.Parallel()
	app := reviewApp(t, &bitbuckettest.FakeClient{T: t})

	if cmd := app.jumpToTab(views.TabOverview); cmd != nil {
		t.Error("the overview is rendered from the pull request and must cost no request")
	}
	testkit.AssertEqual(t, "tab", app.detailView.ActiveTab(), views.TabOverview)
}

func TestDetailTabEntry_SelectingAPullRequestFetchesNothing(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	app := reviewApp(t, fake)

	runAll(t, app, app.previewSelected())

	for _, method := range []string{
		"ListPullRequestDiffStat", "ListPullRequestCommits", "ListPullRequestComments", "GetPullRequestDiff",
	} {
		testkit.AssertEqual(t, method+" on selection", fake.CallCount(method), 0)
	}
}

func TestNextTabKey_WarmsTheTabItLandsOn(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubDiffStat(fake)
	app := reviewApp(t, fake)

	_, cmd, handled := app.handleTabAction(ActNextTab)
	if !handled {
		t.Fatal("the next-tab action must be handled on the detail panel")
	}
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "tab", app.detailView.ActiveTab(), views.TabFiles)
	testkit.AssertEqual(t, "diffstat requests", fake.CallCount("ListPullRequestDiffStat"), 1)
}

func TestClickingTheDetailTabBar_WarmsTheTab(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubDiffStat(fake)
	app := reviewApp(t, fake)
	// ClickTab hit-tests the zones the last render recorded.
	_ = app.detailView.View()

	x := app.panelSideW + strings.Index(plain(app.detailView.View()), "Files")
	_, cmd := app.mouseClick(panelDetail, 0, x)
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "tab", app.detailView.ActiveTab(), views.TabFiles)
	testkit.AssertEqual(t, "diffstat requests", fake.CallCount("ListPullRequestDiffStat"), 1)
}

func TestReviewCaches_DroppedWhenTheSourceCommitMoves(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubDiffStat(fake)
	app := reviewApp(t, fake)
	runAll(t, app, app.jumpToTab(views.TabFiles))
	testkit.AssertEqual(t, "diffstat requests", fake.CallCount("ListPullRequestDiffStat"), 1)

	// A push: the same pull request, a different head.
	pushed := withSource(withAuthor(newPR(testRepo, 101, testTitle), testUserUUID), testPushedHash)
	app.prCache[pushed.Key()] = &pushed
	runAll(t, app, app.previewSelected())

	if _, ok := app.diffStatCache[testPRKey]; ok {
		t.Error("a new head must drop the cached changed-file list")
	}
	runAll(t, app, app.jumpToTab(views.TabFiles))
	testkit.AssertEqual(t, "diffstat requests after the push", fake.CallCount("ListPullRequestDiffStat"), 2)
}

func TestReviewCaches_SurviveARefreshOfTheSameHead(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubDiffStat(fake)
	app := reviewApp(t, fake)
	runAll(t, app, app.jumpToTab(views.TabFiles))

	runAll(t, app, app.previewSelected())

	if _, ok := app.diffStatCache[testPRKey]; !ok {
		t.Error("a background refresh of the same head must keep the cache")
	}
	testkit.AssertEqual(t, "diffstat requests", fake.CallCount("ListPullRequestDiffStat"), 1)
}

func TestRefreshKey_RefetchesTheActiveTab(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubDiffStat(fake)
	fake.ListPullRequestStatusesFunc = func(context.Context, string, string, int) ([]bitbucket.CommitStatus, error) {
		return nil, nil
	}
	fake.GetPullRequestFunc = func(context.Context, string, string, int) (*bitbucket.PullRequest, error) {
		pr := withSource(withAuthor(newPR(testRepo, 101, testTitle), testUserUUID), testSourceHash)
		return &pr, nil
	}
	app := reviewApp(t, fake)
	runAll(t, app, app.jumpToTab(views.TabFiles))

	_, cmd, handled := app.handleActionRefresh()
	if !handled {
		t.Fatal("refresh must be handled")
	}
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "diffstat requests", fake.CallCount("ListPullRequestDiffStat"), 2)
}

func TestReviewSetters_DropAResponseForAnotherPullRequest(t *testing.T) {
	t.Parallel()
	app := reviewApp(t, &bitbuckettest.FakeClient{T: t})
	// The tab is switched without warming it: this test is about the response
	// that arrives, not about the request.
	app.detailView.SetActiveTab(views.TabFiles)

	_, _ = app.Update(diffStatLoadedMsg{
		key: testPRKey2, entries: testDiffStat(), hash: testSourceHash, err: nil,
	})

	testkit.AssertEqual(t, "changed files on screen", len(app.detailView.DiffStat()), 0)
	// The response is still cached: it is correct for the pull request it names.
	if _, ok := app.diffStatCache[testPRKey2]; !ok {
		t.Error("a response for another pull request is still worth caching")
	}
}

func TestDiffStatTooLarge_SaysToOpenTheBrowser(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.ListPullRequestDiffStatFunc = func(context.Context, string, string, int) ([]bitbucket.DiffStatEntry, error) {
		return nil, &bitbucket.APIError{Status: 555, Message: "Diff Too Large"}
	}
	app := reviewApp(t, fake)

	runAll(t, app, app.jumpToTab(views.TabFiles))

	rendered := plain(app.detailView.View())
	if !strings.Contains(rendered, "too large — open in browser (o)") {
		t.Errorf("the Files tab should name the way out:\n%s", rendered)
	}
	// A 555 is data, not a broken connection.
	testkit.AssertEqual(t, "status panel error", app.statusPanel.ErrorMessage(), "")

	// The refusal is cached, so re-entering the tab does not retry.
	runAll(t, app, app.jumpToTab(views.TabOverview))
	runAll(t, app, app.jumpToTab(views.TabFiles))
	testkit.AssertEqual(t, "diffstat requests", fake.CallCount("ListPullRequestDiffStat"), 1)
}

func TestCommitsError_ReachesTheTabAndTheStatusPanel(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.ListPullRequestCommitsFunc = func(context.Context, string, string, int,
		bitbucket.ListOptions,
	) ([]bitbucket.Commit, error) {
		return nil, errors.New(testBoom)
	}
	app := reviewApp(t, fake)

	runAll(t, app, app.jumpToTab(views.TabCommits))

	if !strings.Contains(plain(app.detailView.View()), testBoom) {
		t.Error("the Commits tab should show why it is empty")
	}
	testkit.AssertEqual(t, "status panel error", app.statusPanel.ErrorMessage(), testBoom)
}

func TestReviewTabURL_FollowsTheCursor(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubCommits(fake)
	stubComments(fake)
	app := reviewApp(t, fake)

	runAll(t, app, app.jumpToTab(views.TabCommits))
	_ = app.detailView.View()
	if got := app.browserURL(); !strings.Contains(got, testCommitHash) {
		t.Errorf("browser URL on the Commits tab = %q, want the commit", got)
	}

	runAll(t, app, app.jumpToTab(views.TabComments))
	_ = app.detailView.View()
	if got := app.browserURL(); !strings.Contains(got, "comment-9001") {
		t.Errorf("browser URL on the Comments tab = %q, want the thread", got)
	}

	runAll(t, app, app.jumpToTab(views.TabOverview))
	if got := app.browserURL(); !strings.Contains(got, "pull-requests") {
		t.Errorf("browser URL on the Overview tab = %q, want the pull request", got)
	}
}

func TestCopyURL_FollowsTheBrowserTarget(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubCommits(fake)
	app := reviewApp(t, fake)
	_, gotStdin := captureExternal(t, app)
	runAll(t, app, app.jumpToTab(views.TabCommits))
	_ = app.detailView.View()

	_, _, ok := app.handlePullRequestAction(ActCopyURL)

	if !ok {
		t.Fatal("the copy action must be handled")
	}
	if !strings.Contains(*gotStdin, testCommitHash) {
		t.Errorf("copied %q, want the commit URL the browser key would open", *gotStdin)
	}
}

func TestURLPicker_ListsTheCommitAndThreadGroups(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubCommits(fake)
	stubComments(fake)
	app := reviewApp(t, fake)
	runAll(t, app, app.jumpToTab(views.TabCommits))
	runAll(t, app, app.jumpToTab(views.TabComments))
	app.modal.SetSize(80, 30)

	_, _ = app.handleActionURLPicker()

	rendered := plain(app.modal.View())
	for _, want := range []string{"Commits", "Comments"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("the picker is missing the %q group:\n%s", want, rendered)
		}
	}
}

func TestReviewActions_AreIgnoredWithTheLeftPanelsFocused(t *testing.T) {
	t.Parallel()
	app := reviewApp(t, &bitbuckettest.FakeClient{T: t})
	app.side = sideLeft
	app.updateFocusState()

	for _, action := range []Action{
		ActDetailOverview, ActDetailFiles, ActDetailCommits, ActDetailComments,
		ActNextFile, ActPrevFile, ActOpenEditor,
	} {
		if _, _, handled := app.handleReviewAction(action); handled {
			t.Errorf("%s must fall through to the left panels", action)
		}
	}
}

func TestSameSource_TreatsTwoUnknownHashesAsOneCommit(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{name: "both unknown", a: "", b: "", want: true},
		{name: "one unknown", a: "", b: testSourceHash, want: false},
		{name: "same commit", a: testSourceHash, b: testSourceHash[:12], want: true},
		{name: "different commits", a: testSourceHash, b: testPushedHash, want: false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			testkit.AssertEqual(t, "sameSource", sameSource(testCase.a, testCase.b), testCase.want)
		})
	}
}

func TestThreadSelected_SaysSoWhileTheCommentsAreStillColdOnScreen(t *testing.T) {
	t.Parallel()
	app := reviewApp(t, &bitbuckettest.FakeClient{T: t})

	_, cmd := app.Update(views.ThreadSelectedMsg{PullRequestKey: testPRKey, RootID: testThreadRoot})

	if cmd != nil {
		t.Error("jumping to an unknown thread costs no command")
	}
	if !strings.Contains(plain(app.helpBar.View()), "comments still loading") {
		t.Errorf("the help bar should explain the no-op:\n%s", plain(app.helpBar.View()))
	}
}
