package views

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/diff"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// testDiffFile is one modified Go file with a single hunk: a context line
// carrying a tab, a removed line, two added lines, a closing context line and a
// no-newline marker. It exercises every row shape the diff view draws.
func testDiffFile() *diff.File {
	return &diff.File{
		OldPath: testDiffPath,
		NewPath: testDiffPath,
		Status:  diff.Modified,
		Hunks: []diff.Hunk{{
			OldStart: 98, OldLines: 4, NewStart: 98, NewLines: 5, Header: "func main() {",
			Lines: []diff.Line{
				{Kind: diff.LineContext, Content: "\tctx := context.Background()", OldNo: 98, NewNo: 98},
				{Kind: diff.LineRemoved, Content: "\tdefer cancel()", OldNo: 99},
				{Kind: diff.LineAdded, Content: "\tdefer stop()", NewNo: 99},
				{Kind: diff.LineAdded, Content: "\trun(ctx)", NewNo: 100},
				{Kind: diff.LineContext, Content: "}", OldNo: 100, NewNo: 101},
				{Kind: diff.LineNoNewline, Content: "No newline at end of file"},
			},
		}},
	}
}

// fileDiffDetail returns a focused detail view showing testDiffFile's diff,
// with markers if any, and View() called.
func fileDiffDetail(t *testing.T, markers map[int]int) *DetailView {
	t.Helper()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())
	view.SetDiffStat(testPRKey, testDiffStat())
	view.SetActiveTab(TabFiles)
	view.SetFileDiffLoading(testPRKey, testDiffPath)
	view.SetFileDiff(testPRKey, testDiffPath, testDiffFile(), nil, markers)
	view.View()
	return view
}

func TestDetailView_FileDiff_RendersGuttersMarkersAndHeader(t *testing.T) {
	t.Parallel()
	view := fileDiffDetail(t, nil)
	out := stripANSI(view.View())

	testkit.AssertEqual(t, "mode", view.Mode(), ModeFileDiff)
	testkit.AssertEqual(t, "title", stripANSI(view.buildTitle(60)), "[0] "+testDiffPath)
	testkit.AssertEqual(t, "path", view.FileDiffPath(), testDiffPath)

	for _, want := range []string{
		// The file's own status line, taken from the diffstat, and the rule.
		"M " + testDiffPath, "+12 −3",
		"@@ -98,4 +98,5 @@ func main() {",
		// Three-column gutters per side; one side blank on a changed line.
		" 98  98 │     ctx := context.Background()",
		" 99     │-    defer cancel()",
		"     99 │+    defer stop()",
		"100 101 │ }",
		`        │\No newline at end of file`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("view does not contain %q:\n%s", want, out)
		}
	}

	// The hunk header plus its six lines, with nothing drawn between them.
	testkit.AssertEqual(t, "row count", view.listTabItemCount(), 7)
	if !strings.Contains(out, "│     ctx := context.Background()") {
		t.Fatalf("missing the context row:\n%s", out)
	}
	rows := strings.Split(out, "\n")
	for i, row := range rows {
		if !strings.Contains(row, "ctx := context.Background()") {
			continue
		}
		if !strings.Contains(rows[i+1], "defer cancel()") {
			t.Errorf("diff rows must sit directly under one another, got:\n%s\n%s", row, rows[i+1])
		}
	}
}

func TestDetailView_FileDiff_CommentMarkers(t *testing.T) {
	t.Parallel()
	// One thread on the new-side line 100, three on the removed old-side line
	// 99, and two on the file itself.
	out := stripANSI(fileDiffDetail(t, map[int]int{100: 1, -99: 3, 0: 2}).View())

	for _, want := range []string{
		"    100 │●    run(ctx)",
		" 99     │3    defer cancel()",
		"· 2 comments",
		// Line 98 carries no thread, so its marker column stays blank.
		" 98  98 │     ctx",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("view does not contain %q:\n%s", want, out)
		}
	}
}

func TestDetailView_FileDiff_EnterOnAMarkedLineJumpsToTheThread(t *testing.T) {
	t.Parallel()
	root := inlineAnchor(threadComment(testInlineRootID, testReviewer, "Why stop here?", time.Hour), 101)

	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())
	view.SetThreads(testPRKey, bitbucket.BuildThreads([]bitbucket.Comment{root}))
	view.SetFileDiffLoading(testPRKey, testDiffPath)
	view.SetFileDiff(testPRKey, testDiffPath, testDiffFile(), nil,
		bitbucket.CommentedLines(view.Threads())[testDiffPath])
	view.View()

	// Row 0 is the hunk header; row 5 is the context line at new-side 101,
	// where the thread hangs.
	view.ScrollBy(5)
	line, ok := view.SelectedDiffLine()
	testkit.AssertEqual(t, "cursor on a line", ok, true)
	testkit.AssertEqual(t, "new-side line", line.NewNo, 101)

	_, cmd := view.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter on a commented line emitted nothing")
	}
	msg, ok := cmd().(ThreadSelectedMsg)
	if !ok {
		t.Fatalf("msg = %T, want ThreadSelectedMsg", cmd())
	}
	testkit.AssertEqual(t, "key", msg.PullRequestKey, testPRKey)
	testkit.AssertEqual(t, "root", msg.RootID, int64(testInlineRootID))
}

func TestDetailView_FileDiff_EnterWithoutAThreadIsANoOp(t *testing.T) {
	t.Parallel()
	view := fileDiffDetail(t, nil)

	// The cursor starts on the hunk header, which never carries a thread.
	if _, cmd := view.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Error("Enter on a hunk header should emit nothing")
	}
	view.ScrollBy(1)
	if _, cmd := view.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Error("Enter with no threads loaded should emit nothing")
	}
}

func TestDetailView_SelectedDiffLine_ReportsTheHunkHeader(t *testing.T) {
	t.Parallel()
	view := fileDiffDetail(t, nil)

	line, ok := view.SelectedDiffLine()
	testkit.AssertEqual(t, "ok", ok, true)
	testkit.AssertEqual(t, "kind", line.Kind, diffHunkHeader)
	testkit.AssertEqual(t, "hunk start", line.NewNo, 98)

	view.Back()
	if _, ok := view.SelectedDiffLine(); ok {
		t.Error("SelectedDiffLine() outside the diff should report false")
	}
}

func TestDetailView_FileDiff_TruncatesLongLines(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())
	view.SetFileDiffLoading(testPRKey, testDiffPath)
	view.SetFileDiff(testPRKey, testDiffPath, &diff.File{
		NewPath: testDiffPath,
		Hunks: []diff.Hunk{{
			OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1,
			Lines: []diff.Line{{Kind: diff.LineAdded, Content: strings.Repeat("x", 400), NewNo: 1}},
		}},
	}, nil, nil)

	for _, line := range strings.Split(stripANSI(view.View()), "\n") {
		if got := len([]rune(line)); got > 80 {
			t.Fatalf("line is %d columns wide, panel is 80: %q", got, line)
		}
	}
}

func TestDetailView_FileDiff_PlaceholderStates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		setup func(view *DetailView)
		want  string
	}{
		{"loading", func(*DetailView) {}, "Loading diff…"},
		{
			"error",
			func(v *DetailView) { v.SetFileDiffError(testPRKey, testDiffPath, "diff too large") },
			"diff too large",
		},
		{
			"binary",
			func(v *DetailView) {
				v.SetFileDiff(testPRKey, testDiffPath, &diff.File{NewPath: testDiffPath, IsBinary: true}, nil, nil)
			},
			"Binary file",
		},
		{
			"no textual changes",
			func(v *DetailView) {
				v.SetFileDiff(testPRKey, testDiffPath, &diff.File{NewPath: testDiffPath}, nil, nil)
			},
			"(no textual changes)",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			view := makeFocusedDetail()
			view.SetPullRequest(detailPR())
			view.SetFileDiffLoading(testPRKey, testDiffPath)
			tc.setup(view)

			out := stripANSI(view.View())
			if !strings.Contains(out, tc.want) {
				t.Errorf("view does not contain %q:\n%s", tc.want, out)
			}
			if _, ok := view.SelectedDiffLine(); ok {
				t.Error("a placeholder body has no diff line to select")
			}
		})
	}
}

func TestDetailView_FileDiff_StaleResponsesAreDropped(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())
	view.SetFileDiffLoading(testPRKey, testDiffPath)

	view.SetFileDiff(testPRKey2, testDiffPath, testDiffFile(), nil, nil)
	view.SetFileDiff(testPRKey, "other/file.go", testDiffFile(), nil, nil)
	view.SetFileDiffError(testPRKey, "other/file.go", "boom")
	if _, ok := view.SelectedDiffLine(); ok {
		t.Error("a response for another pull request or file must be dropped")
	}
	if !strings.Contains(stripANSI(view.View()), "Loading diff…") {
		t.Error("the view should still be waiting on its own file")
	}

	// And a response that lands after the user walked out cannot pull them back.
	view.Back()
	view.SetFileDiff(testPRKey, testDiffPath, testDiffFile(), nil, nil)
	testkit.AssertEqual(t, "mode", view.Mode(), ModePullRequest)
}

func TestDetailView_FileDiff_SetFileDiffAgainKeepsTheCursor(t *testing.T) {
	t.Parallel()
	view := fileDiffDetail(t, nil)
	view.ScrollBy(4)

	// This is what the app does once the review threads arrive.
	view.SetFileDiff(testPRKey, testDiffPath, testDiffFile(), nil, map[int]int{100: 1})

	line, ok := view.SelectedDiffLine()
	testkit.AssertEqual(t, "ok", ok, true)
	testkit.AssertEqual(t, "cursor stayed put", line.NewNo, 100)
	if !strings.Contains(stripANSI(view.View()), "●") {
		t.Error("the second call should light up the markers")
	}
}

func TestDetailView_FileDiff_BackReturnsToTheFilesTabOnThatFile(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())
	view.SetDiffStat(testPRKey, testDiffStat())
	view.SetActiveTab(TabFiles)
	view.SetFileDiffLoading(testPRKey, "b.go")
	view.SetFileDiff(testPRKey, "b.go", testDiffFile(), nil, nil)

	testkit.AssertEqual(t, "key is still the pull request's", view.PullRequestKey(), testPRKey)
	testkit.AssertEqual(t, "back", view.Back(), true)
	testkit.AssertEqual(t, "mode", view.Mode(), ModePullRequest)
	testkit.AssertEqual(t, "tab", view.ActiveTab(), TabFiles)
	testkit.AssertEqual(t, "cursor on the file just read", view.SelectedFile().Path(), "b.go")
	testkit.AssertEqual(t, "path cleared", view.FileDiffPath(), "")
}

func TestDetailView_FileDiff_NextAndPrevFileNameTheNeighbour(t *testing.T) {
	t.Parallel()
	view := fileDiffDetail(t, nil)

	next, ok := view.NextFile()
	testkit.AssertEqual(t, "next ok", ok, true)
	testkit.AssertEqual(t, "next path", next, "pkg/new/thing.go")
	// Naming the neighbour must not leave the diff; the app opens it.
	testkit.AssertEqual(t, "still in the diff", view.Mode(), ModeFileDiff)
	testkit.AssertEqual(t, "still the same file", view.FileDiffPath(), testDiffPath)

	if _, ok := view.PrevFile(); ok {
		t.Error("PrevFile() on the first file should report false")
	}

	view.SetFileDiffLoading(testPRKey, "docs/logo.png")
	if _, ok := view.NextFile(); ok {
		t.Error("NextFile() on the last file should report false")
	}
	prev, ok := view.PrevFile()
	testkit.AssertEqual(t, "prev ok", ok, true)
	testkit.AssertEqual(t, "prev path", prev, "b.go")
}

func TestDetailView_FileDiff_TabKeyLeavesTheDiff(t *testing.T) {
	t.Parallel()
	view := fileDiffDetail(t, nil)

	view.SetActiveTab(TabCommits)
	testkit.AssertEqual(t, "mode", view.Mode(), ModePullRequest)
	testkit.AssertEqual(t, "tab", view.ActiveTab(), TabCommits)
	testkit.AssertEqual(t, "path cleared", view.FileDiffPath(), "")
}

func TestDetailView_FileDiff_ClickSelectsARow(t *testing.T) {
	t.Parallel()
	view := fileDiffDetail(t, nil)

	// relY 1 is the first body row: two header lines, then the rows.
	if cmd := view.ClickItem(1 + 2 + 4); cmd != nil {
		t.Error("a single click should not activate a row")
	}
	line, ok := view.SelectedDiffLine()
	testkit.AssertEqual(t, "ok", ok, true)
	testkit.AssertEqual(t, "clicked the fifth row", line.NewNo, 100)
}

func TestExpandTabs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no tabs", "plain text", "plain text"},
		{"leading tab", "\tx", "    x"},
		{"tab stop", "ab\tc", "ab  c"},
		{"two tabs", "\t\tx", "        x"},
		{"tab after three", "abc\td", "abc d"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			testkit.AssertEqual(t, "expandTabs", expandTabs(tc.in, 4), tc.want)
		})
	}
}

// wideDiffFile is one hunk of n context lines, for the lazy row rendering: a
// file long enough that highlighting all of it would be visible in the clock.
func wideDiffFile(n int) *diff.File {
	lines := make([]diff.Line, 0, n)
	for i := 1; i <= n; i++ {
		lines = append(lines, diff.Line{
			Kind: diff.LineContext, Content: fmt.Sprintf("\tcall(line%d)", i), OldNo: i, NewNo: i,
		})
	}
	return &diff.File{
		OldPath: testDiffPath, NewPath: testDiffPath, Status: diff.Modified,
		Hunks: []diff.Hunk{{OldStart: 1, OldLines: n, NewStart: 1, NewLines: n, Lines: lines}},
	}
}

func TestDetailView_FileDiff_RendersOnlyTheRowsOnScreen(t *testing.T) {
	t.Parallel()
	const rows = 500
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())
	view.SetFileDiffLoading(testPRKey, testDiffPath)
	view.SetFileDiff(testPRKey, testDiffPath, wideDiffFile(rows), nil, nil)
	out := stripANSI(view.View())

	// The hunk header plus every line is still a block, so the cursor, the
	// footer and the hit-test count the whole file.
	testkit.AssertEqual(t, "row count", view.listTabItemCount(), rows+1)
	if !strings.Contains(out, "call(line1)") {
		t.Errorf("the first rows should be on screen:\n%s", out)
	}
	if strings.Contains(out, "call(line400)") {
		t.Errorf("a row far below the window should not be drawn:\n%s", out)
	}

	if view.blockCache.rendered == nil {
		t.Fatal("the diff list should be built lazily, one window at a time")
	}
	rendered := 0
	for _, done := range view.blockCache.rendered {
		if done {
			rendered++
		}
	}
	if rendered == 0 {
		t.Error("the window on screen should have been rendered")
	}
	if budget := view.VisibleRows() + 2*blockOverscan + 1; rendered > budget {
		t.Errorf("rendered %d of %d rows for a %d-row window, want at most %d",
			rendered, rows+1, view.VisibleRows(), budget)
	}
}

func TestDetailView_FileDiff_FillsTheWindowTheCursorScrollsTo(t *testing.T) {
	t.Parallel()
	const rows = 500
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())
	view.SetFileDiffLoading(testPRKey, testDiffPath)
	view.SetFileDiff(testPRKey, testDiffPath, wideDiffFile(rows), nil, nil)
	view.View()

	for _, target := range []int{200, rows, 1} {
		view.listCursor = target
		out := stripANSI(view.View())
		row, ok := view.SelectedDiffLine()
		if !ok {
			t.Fatalf("cursor %d has no diff line", target)
		}
		want := strings.TrimSpace(row.Content)
		if !strings.Contains(out, want) {
			t.Errorf("the cursor row %q is blank on screen:\n%s", want, out)
		}
	}

	// A width change re-renders the window rather than reusing rows laid out
	// for the old one.
	view.SetSize(120, 24)
	out := stripANSI(view.View())
	if !strings.Contains(out, "call(line1)") {
		t.Errorf("rows should be redrawn at the new width:\n%s", out)
	}
}
