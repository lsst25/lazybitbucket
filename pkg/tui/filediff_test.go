package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// fileDiffApp is reviewApp with the Files tab already loaded, which is where
// every path into the diff view starts.
func fileDiffApp(t *testing.T, fake *bitbuckettest.FakeClient) *App {
	t.Helper()
	stubDiffStat(fake)
	app := reviewApp(t, fake)
	runAll(t, app, app.jumpToTab(views.TabFiles))
	// The block list is built during render, and Enter reads it.
	_ = app.detailView.View()
	return app
}

func TestOpenFileDiff_ParsesTheRequestedFileAndFetchesTheThreads(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	var gotOpts bitbucket.DiffOptions
	fake.GetPullRequestDiffFunc = func(_ context.Context, workspace, slug string, id int,
		opts bitbucket.DiffOptions,
	) (string, error) {
		testkit.AssertEqual(t, "workspace", workspace, testWorkspace)
		testkit.AssertEqual(t, "repo", slug, testRepo)
		testkit.AssertEqual(t, "id", id, 101)
		gotOpts = opts
		return testDiffText, nil
	}
	stubComments(fake)
	app := fileDiffApp(t, fake)

	_, cmd := app.Update(views.FileSelectedMsg{PullRequestKey: testPRKey, Path: testFileMD})
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "requested path", gotOpts.Path, testFileMD)
	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModeFileDiff)
	testkit.AssertEqual(t, "file on screen", app.detailView.FileDiffPath(), testFileMD)
	// The whole diff document came back; the view got the file that was asked
	// for, not the first one in it.
	rendered := plain(app.detailView.View())
	if !strings.Contains(rendered, "Rate limiting") {
		t.Errorf("the diff view is showing the wrong file:\n%s", rendered)
	}
	// Opening a diff warms the threads, because the markers come from them.
	testkit.AssertEqual(t, "comment requests", fake.CallCount("ListPullRequestComments"), 1)
}

func TestOpenFileDiff_MarkersArriveWhicheverOrderTheResponsesDo(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		threadsLast bool
	}{
		{name: "threads first", threadsLast: false},
		{name: "diff first", threadsLast: true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			app := fileDiffApp(t, &bitbuckettest.FakeClient{T: t})
			// The two responses are delivered by hand so the order is the test.
			app.detailView.SetFileDiffLoading(testPRKey, testFileGo)
			diffMsg := parsedFileDiffMsg(t, testFileGo)
			threadMsg := threadsLoadedMsg{
				key: testPRKey, threads: testThreads(), hash: testSourceHash, err: nil,
			}

			if testCase.threadsLast {
				_, _ = app.Update(diffMsg)
				_, _ = app.Update(threadMsg)
			} else {
				_, _ = app.Update(threadMsg)
				_, _ = app.Update(diffMsg)
			}

			rendered := plain(app.detailView.View())
			if !strings.Contains(rendered, "●") {
				t.Errorf("the commented line should carry a marker:\n%s", rendered)
			}
		})
	}
}

// parsedFileDiffMsg is the message fetchFileDiff would have produced for one
// file of the fixture diff.
func parsedFileDiffMsg(t *testing.T, path string) fileDiffLoadedMsg {
	t.Helper()
	files, err := parseFixtureDiff(t)
	if err != nil {
		t.Fatalf("parse the fixture diff: %v", err)
	}
	return fileDiffLoadedMsg{
		key: testPRKey, path: path, file: pickDiffFile(files, path), hash: testSourceHash, err: nil,
	}
}

func TestEnterOnACommentedLine_LandsOnTheThread(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubDiff(fake)
	stubComments(fake)
	app := fileDiffApp(t, fake)
	runAll(t, app, app.openFileDiff(testFileGo))
	_ = app.detailView.View()

	// The commented line is new-side 13, the fifth row of the file's only hunk
	// (header, two context lines, one removed line, then the two added ones).
	moveDiffCursorTo(t, app, 13)
	_ = app.detailView.View()
	_, cmd := app.detailView.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter on a commented line should emit a thread selection")
	}
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModePullRequest)
	testkit.AssertEqual(t, "tab", app.detailView.ActiveTab(), views.TabComments)
	thread := app.detailView.SelectedThread()
	if thread == nil || thread.Root.ID != testThreadRoot {
		t.Fatalf("cursor = %v, want the thread the line carries", thread)
	}
}

// A pull request the user steps away from and comes back to keeps its threads
// cached while the detail view's own thread list is reset, so opening a file
// diff has to re-feed the view from the cache — otherwise the markers the diff
// draws from the cache have nothing behind them and Enter does nothing.
func TestEnterOnACommentedLine_WorksAfterReSelectingThePullRequest(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubDiff(fake)
	stubComments(fake)
	app := fileDiffApp(t, fake)

	// The Comments tab warms the cache and the view together.
	runAll(t, app, app.jumpToTab(views.TabComments))
	testkit.AssertEqual(t, "comment requests", fake.CallCount("ListPullRequestComments"), 1)

	// Away to another pull request and back: the cache survives, the view's
	// thread list does not.
	other := withSource(newPR(testRepo, 102, "Retry the upstream call"), testSourceHash)
	app.detailView.SetPullRequest(&other)
	app.detailView.SetPullRequest(app.currentPullRequest())
	testkit.AssertEqual(t, "threads in the view", len(app.detailView.Threads()), 0)

	runAll(t, app, app.jumpToTab(views.TabFiles))
	_ = app.detailView.View()
	runAll(t, app, app.openFileDiff(testFileGo))
	_ = app.detailView.View()

	// The cached threads went back into the view, and cost no second request.
	testkit.AssertEqual(t, "comment requests", fake.CallCount("ListPullRequestComments"), 1)
	testkit.AssertEqual(t, "threads in the view", len(app.detailView.Threads()), 1)

	moveDiffCursorTo(t, app, 13)
	_ = app.detailView.View()
	_, cmd := app.detailView.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter on a commented line should emit a thread selection")
	}
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "tab", app.detailView.ActiveTab(), views.TabComments)
	thread := app.detailView.SelectedThread()
	if thread == nil || thread.Root.ID != testThreadRoot {
		t.Fatalf("cursor = %v, want the thread the line carries", thread)
	}
}

// moveDiffCursorTo walks the diff view's line cursor to the row whose new-side
// number is line. It drives ScrollBy, the same thing the navigation keys do.
func moveDiffCursorTo(t *testing.T, app *App, line int) {
	t.Helper()
	for range 40 {
		row, ok := app.detailView.SelectedDiffLine()
		if ok && row.NewNo == line {
			return
		}
		app.detailView.ScrollBy(1)
	}
	t.Fatalf("no diff row with new-side line %d", line)
}

func TestNextFile_OpensTheNeighbourFromInsideTheDiff(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubDiff(fake)
	stubComments(fake)
	app := fileDiffApp(t, fake)
	runAll(t, app, app.openFileDiff(testFileGo))

	_, cmd, handled := app.handleReviewAction(ActNextFile)
	if !handled {
		t.Fatal("the next-file action must be handled on the detail panel")
	}
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModeFileDiff)
	testkit.AssertEqual(t, "file on screen", app.detailView.FileDiffPath(), testFileMD)

	// Past the last file the cursor stays put and the help bar says why.
	_, cmd, _ = app.handleReviewAction(ActNextFile)
	runAll(t, app, cmd)
	testkit.AssertEqual(t, "file after the end", app.detailView.FileDiffPath(), testFileMD)
	if !strings.Contains(plain(app.helpBar.View()), "last file") {
		t.Errorf("the help bar should report the edge:\n%s", plain(app.helpBar.View()))
	}

	// And back again.
	_, cmd, _ = app.handleReviewAction(ActPrevFile)
	runAll(t, app, cmd)
	testkit.AssertEqual(t, "file after prev", app.detailView.FileDiffPath(), testFileGo)
}

func TestNextFile_MovesTheCursorOnTheFilesTab(t *testing.T) {
	t.Parallel()
	app := fileDiffApp(t, &bitbuckettest.FakeClient{T: t})

	_, cmd, _ := app.handleReviewAction(ActNextFile)

	if cmd != nil {
		t.Error("stepping the Files tab cursor costs no request")
	}
	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModePullRequest)
	selected := app.detailView.SelectedFile()
	if selected == nil || selected.Path() != testFileMD {
		t.Fatalf("cursor = %v, want the second changed file", selected)
	}

	// Back to the first file, then one step too far.
	_, _, _ = app.handleReviewAction(ActPrevFile)
	_, _, _ = app.handleReviewAction(ActPrevFile)

	selected = app.detailView.SelectedFile()
	if selected == nil || selected.Path() != testFileGo {
		t.Fatalf("cursor = %v, want the first changed file", selected)
	}
	if !strings.Contains(plain(app.helpBar.View()), "first file") {
		t.Errorf("the help bar should report the top edge:\n%s", plain(app.helpBar.View()))
	}
}

func TestNextFile_WarmsTheChangedFileListFirst(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubDiffStat(fake)
	app := reviewApp(t, fake)
	// Straight onto the Files tab without warming it, the way a remapped key
	// pressed at the wrong moment would.
	app.detailView.SetActiveTab(views.TabFiles)

	_, cmd, _ := app.handleReviewAction(ActNextFile)
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "diffstat requests", fake.CallCount("ListPullRequestDiffStat"), 1)
	testkit.AssertEqual(t, "changed files", len(app.detailView.DiffStat()), 2)
}

func TestFileDiff_CachedFileCostsNoSecondRequest(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubDiff(fake)
	stubComments(fake)
	app := fileDiffApp(t, fake)

	runAll(t, app, app.openFileDiff(testFileGo))
	if !app.detailView.Back() {
		t.Fatal("Back() should walk out of the diff view")
	}
	runAll(t, app, app.openFileDiff(testFileGo))

	testkit.AssertEqual(t, "diff requests", fake.CallCount("GetPullRequestDiff"), 1)
	testkit.AssertEqual(t, "file on screen", app.detailView.FileDiffPath(), testFileGo)
}

func TestFileDiff_BackLandsOnTheFilesTabWithTheCursorOnThatFile(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubDiff(fake)
	stubComments(fake)
	app := fileDiffApp(t, fake)
	runAll(t, app, app.openFileDiff(testFileMD))

	if !app.detailView.Back() {
		t.Fatal("Back() should walk out of the diff view")
	}

	testkit.AssertEqual(t, "tab", app.detailView.ActiveTab(), views.TabFiles)
	_ = app.detailView.View()
	selected := app.detailView.SelectedFile()
	if selected == nil || selected.Path() != testFileMD {
		t.Fatalf("cursor = %v, want the file just read", selected)
	}
}

func TestFileDiffTooLarge_SaysToOpenTheBrowser(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.GetPullRequestDiffFunc = func(_ context.Context, _, _ string, _ int,
		_ bitbucket.DiffOptions,
	) (string, error) {
		return "", &bitbucket.APIError{Status: 555, Message: "Diff Too Large"}
	}
	stubComments(fake)
	app := fileDiffApp(t, fake)

	runAll(t, app, app.openFileDiff(testFileGo))

	rendered := plain(app.detailView.View())
	if !strings.Contains(rendered, "diff too large — open in browser (o)") {
		t.Errorf("the diff view should name the way out:\n%s", rendered)
	}
	// And `o` really does have somewhere to go.
	if got := app.browserURL(); !strings.HasSuffix(got, "/diff") {
		t.Errorf("browser URL in the diff view = %q, want the pull request's diff", got)
	}
}

func TestFileDiff_ParseFailureIsShownInPlace(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.GetPullRequestDiffFunc = func(_ context.Context, _, _ string, _ int,
		_ bitbucket.DiffOptions,
	) (string, error) {
		// A hunk with no file header ahead of it is the one thing the parser
		// rejects outright.
		return "@@ -1,1 +1,1 @@\n-a\n+b\n", nil
	}
	stubComments(fake)
	app := fileDiffApp(t, fake)

	runAll(t, app, app.openFileDiff(testFileGo))

	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModeFileDiff)
	if plain(app.detailView.View()) == "" {
		t.Error("the diff view should render the parse failure rather than nothing")
	}
	if app.statusPanel.ErrorMessage() == "" {
		t.Error("a parse failure is a real failure and belongs on the status panel")
	}
}

func TestPickDiffFile(t *testing.T) {
	t.Parallel()
	files, err := parseFixtureDiff(t)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	testkit.AssertEqual(t, "match", pickDiffFile(files, testFileMD).Path(), testFileMD)
	testkit.AssertEqual(t, "fallback", pickDiffFile(files, "nowhere/at/all.go").Path(), testFileGo)
	if pickDiffFile(nil, testFileGo) != nil {
		t.Error("an empty diff yields no file")
	}
}

func TestDoubleClickingAChangedFile_OpensItsDiff(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubDiff(fake)
	stubComments(fake)
	app := fileDiffApp(t, fake)

	// Row 1 of the body is the first changed file; the second click within the
	// double-click window is what activates it.
	_, _ = app.mouseClick(panelDetail, 1, app.panelSideW+4)
	_, cmd := app.mouseClick(panelDetail, 1, app.panelSideW+4)
	if cmd == nil {
		t.Fatal("a double-clicked file should open its diff")
	}
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModeFileDiff)
	testkit.AssertEqual(t, "file on screen", app.detailView.FileDiffPath(), testFileGo)
}

// threeFileDiffStat returns three changed files so stepping can be exercised
// past the second file, where a probe-then-step bug used to pin the cursor.
func threeFileDiffStat() []bitbucket.DiffStatEntry {
	entry := func(path string) bitbucket.DiffStatEntry {
		return bitbucket.DiffStatEntry{
			Type:   "diffstat",
			Status: bitbucket.DiffStatModified,
			Old:    &bitbucket.CommitFile{Type: "commit_file", Path: path, EscapedPath: path},
			New:    &bitbucket.CommitFile{Type: "commit_file", Path: path, EscapedPath: path},
		}
	}
	return []bitbucket.DiffStatEntry{entry(testFileGo), entry(testFileMD), entry("cmd/third.go")}
}

func TestNextFile_WalksEveryFileOnTheFilesTab(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.ListPullRequestDiffStatFunc = func(_ context.Context, _, _ string, _ int) ([]bitbucket.DiffStatEntry, error) {
		return threeFileDiffStat(), nil
	}
	app := reviewApp(t, fake)
	runAll(t, app, app.jumpToTab(views.TabFiles))
	_ = app.detailView.View()

	want := []string{testFileMD, "cmd/third.go"}
	for i, path := range want {
		_, _, _ = app.handleReviewAction(ActNextFile)
		selected := app.detailView.SelectedFile()
		if selected == nil || selected.Path() != path {
			t.Fatalf("after n #%d cursor = %v, want %s", i+1, selected, path)
		}
	}

	// One more step at the end reports the edge and stays put.
	_, _, _ = app.handleReviewAction(ActNextFile)
	if selected := app.detailView.SelectedFile(); selected == nil || selected.Path() != "cmd/third.go" {
		t.Fatalf("cursor after the last file = %v, want it to stay on the last file", selected)
	}
	if !strings.Contains(plain(app.helpBar.View()), "last file") {
		t.Errorf("the help bar should report the bottom edge:\n%s", plain(app.helpBar.View()))
	}

	// And p walks all the way back.
	for range 2 {
		_, _, _ = app.handleReviewAction(ActPrevFile)
	}
	if selected := app.detailView.SelectedFile(); selected == nil || selected.Path() != testFileGo {
		t.Fatalf("cursor after p p = %v, want the first file", selected)
	}
}

func TestHandleActivate_OnTheDetailPanelDispatchesToTheView(t *testing.T) {
	t.Parallel()
	app := fileDiffApp(t, &bitbuckettest.FakeClient{T: t})
	app.side = sideRight

	model, cmd := app.handleActivate()

	if model == nil {
		t.Fatal("activation on the detail panel must be handled by the app, not dropped")
	}
	if cmd == nil {
		t.Fatal("Enter on a file row should ask to open its diff")
	}
	msg, ok := cmd().(views.FileSelectedMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want views.FileSelectedMsg", msg)
	}
	testkit.AssertEqual(t, "selected path", msg.Path, testFileGo)
}
