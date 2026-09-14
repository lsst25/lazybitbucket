package views

import (
	"strconv"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// commentTarget resolves where a comment written from the view's current
// position would go, failing the test when the view says there is nowhere.
func commentTarget(t *testing.T, view *DetailView) CommentTarget {
	t.Helper()
	target, ok := view.CommentTarget()
	if !ok {
		t.Fatal("this is a place that should take a comment")
	}
	return target
}

func TestCommentTarget_RepliesToTheThreadUnderTheCursor(t *testing.T) {
	t.Parallel()
	view := threadsDetail(t)

	target := commentTarget(t, view)

	testkit.AssertEqual(t, "parent", target.ParentID, int64(testInlineRootID))
	if target.Anchor != nil {
		t.Errorf("a reply hangs where its parent hangs, got %+v", target.Anchor)
	}
	// The popup names who is being answered.
	testkit.AssertEqual(t, "title", target.Title, "Reply to "+testReviewer)
}

func TestCommentTarget_RepliesToTheConversationInTheSection(t *testing.T) {
	t.Parallel()
	view := threadsDetail(t)
	view.openThread(testGeneralRootID)
	view.View()

	target := commentTarget(t, view)

	testkit.AssertEqual(t, "mode", view.Mode(), ModeThread)
	testkit.AssertEqual(t, "parent", target.ParentID, int64(testGeneralRootID))
}

func TestCommentTarget_TheThreadSectionForgetsAThreadThatIsGone(t *testing.T) {
	t.Parallel()
	view := threadsDetail(t)
	view.openThread(testGeneralRootID)
	view.View()

	// A refetch that no longer carries the conversation: there is nothing left
	// to reply to, and the section says so rather than answering nobody.
	view.SetThreads(testPRKey, nil)

	if _, ok := view.CommentTarget(); ok {
		t.Error("a thread that is gone should take no reply")
	}
}

func TestCommentTarget_TheOverviewTabCommentsOnThePullRequest(t *testing.T) {
	t.Parallel()
	view := threadsDetail(t)
	view.SetActiveTab(TabOverview)
	view.View()

	target := commentTarget(t, view)

	// No parent and no anchor: this starts a conversation on the pull request.
	testkit.AssertEqual(t, "parent", target.ParentID, int64(0))
	if target.Anchor != nil {
		t.Errorf("a comment on the pull request hangs off nothing, got %+v", target.Anchor)
	}
	testkit.AssertEqual(t, "title", target.Title, "Comment on #101")
}

func TestCommentTarget_TakesTheFirstCommentOfAnEmptyPullRequest(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())
	view.SetThreads(testPRKey, nil)
	view.SetActiveTab(TabComments)
	view.View()

	target := commentTarget(t, view)

	testkit.AssertEqual(t, "parent", target.ParentID, int64(0))
	testkit.AssertEqual(t, "title", target.Title, "Comment on #101")
}

func TestCommentTarget_IsRefusedWhereThereIsNothingToCommentOn(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		setup func(view *DetailView)
	}{
		{name: "commits tab", setup: func(view *DetailView) { view.SetActiveTab(TabCommits) }},
		{name: "a step log", setup: func(view *DetailView) { view.mode = ModeStepLog }},
		{name: "a pipeline run", setup: func(view *DetailView) { view.mode = ModePipeline }},
		{name: "the splash screen", setup: func(view *DetailView) { view.mode = ModeSplash }},
		{name: "a file diff with no file", setup: func(view *DetailView) { view.mode = ModeFileDiff }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			view := threadsDetail(t)
			tc.setup(view)

			if _, ok := view.CommentTarget(); ok {
				t.Errorf("%s should take no comment", tc.name)
			}
		})
	}
}

func TestCommentTarget_WithoutAPullRequestThereIsNowhereToWrite(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()

	if _, ok := view.CommentTarget(); ok {
		t.Error("a panel with no pull request should take no comment")
	}
}

// -----------------------------------------------------------------------------
// Inline anchors
// -----------------------------------------------------------------------------

func TestCommentTarget_AnchorsToTheLineUnderTheDiffCursor(t *testing.T) {
	t.Parallel()

	// The rows of testDiffFile, in the order buildDiffRows lays them out.
	for _, tc := range []struct {
		name    string
		row     int
		line    int
		oldSide bool
	}{
		{name: "a context line is the new side", row: 1, line: 98, oldSide: false},
		{name: "a removed line is the old side", row: 2, line: 99, oldSide: true},
		{name: "an added line is the new side", row: 3, line: 99, oldSide: false},
		{name: "the second added line", row: 4, line: 100, oldSide: false},
		{name: "the closing context line", row: 5, line: 101, oldSide: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			view := fileDiffDetail(t, nil)
			view.listCursor = tc.row

			target := commentTarget(t, view)

			if target.Anchor == nil {
				t.Fatal("a line of a diff should anchor the comment")
			}
			testkit.AssertEqual(t, "path", target.Anchor.Path, testDiffPath)
			testkit.AssertEqual(t, "line", target.Anchor.Line, tc.line)
			testkit.AssertEqual(t, "old side", target.Anchor.OldSide, tc.oldSide)
			testkit.AssertEqual(t, "parent", target.ParentID, int64(0))
			// The popup names the line, by base name: the panel title already
			// carries the path.
			testkit.AssertEqual(t, "title", target.Title,
				"Comment on app.go:"+strconv.Itoa(tc.line))
		})
	}
}

func TestCommentTarget_AnchorsToTheFileOnARowWithNoLine(t *testing.T) {
	t.Parallel()

	// Row 0 is the hunk header and row 6 the "\ No newline" marker: neither is
	// a line of the file, so the comment hangs off the file itself.
	for _, row := range []int{0, 6} {
		t.Run(strconv.Itoa(row), func(t *testing.T) {
			t.Parallel()
			view := fileDiffDetail(t, nil)
			view.listCursor = row

			target := commentTarget(t, view)

			if target.Anchor == nil {
				t.Fatal("the row should still anchor the comment to the file")
			}
			testkit.AssertEqual(t, "path", target.Anchor.Path, testDiffPath)
			testkit.AssertEqual(t, "line", target.Anchor.Line, 0)
			testkit.AssertEqual(t, "title", target.Title, "Comment on app.go")
		})
	}
}

func TestCommentTarget_AnchorsToTheSelectedFileOnTheFilesTab(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())
	view.SetDiffStat(testPRKey, testDiffStat())
	view.SetActiveTab(TabFiles)
	view.View()

	target := commentTarget(t, view)

	if target.Anchor == nil {
		t.Fatal("the Files tab should anchor the comment to a file")
	}
	testkit.AssertEqual(t, "path", target.Anchor.Path, testDiffStat()[0].Path())
	testkit.AssertEqual(t, "line", target.Anchor.Line, 0)
}

func TestCommentTarget_AMarkedLineStartsASecondConversation(t *testing.T) {
	t.Parallel()
	// Line 100 already carries a thread, which is what the marker says.
	view := fileDiffDetail(t, map[int]int{100: 1})
	view.listCursor = 4

	target := commentTarget(t, view)

	// C starts a conversation wherever it is pressed; joining the one that is
	// there is what enter and then C do.
	testkit.AssertEqual(t, "parent", target.ParentID, int64(0))
	testkit.AssertEqual(t, "line", target.Anchor.Line, 100)
}
