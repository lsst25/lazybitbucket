package views

import (
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// typeInto sends one key per rune, which is what a keyboard does and what the
// composer has to survive.
func typeInto(t *testing.T, view *DetailView, text string) {
	t.Helper()
	for _, r := range text {
		key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
		if r == ' ' {
			key = tea.KeyMsg{Type: tea.KeySpace}
		}
		view.Update(key)
	}
}

// submittedComment runs the view's command and reports the message it produced,
// failing unless it is a CommentSubmittedMsg.
func submittedComment(t *testing.T, cmd tea.Cmd) CommentSubmittedMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("the composer should have reported a comment to post")
	}
	msg, ok := cmd().(CommentSubmittedMsg)
	if !ok {
		t.Fatalf("command produced %T, want CommentSubmittedMsg", cmd())
	}
	return msg
}

func TestComposer_RepliesToTheThreadUnderTheCursor(t *testing.T) {
	t.Parallel()
	view := threadsDetail(t)

	if !view.BeginComment() {
		t.Fatal("the Comments tab should take a comment")
	}
	testkit.AssertEqual(t, "parent", view.ComposerParent(), int64(testInlineRootID))

	typeInto(t, view, "looks right to me")
	_, cmd := view.Update(tea.KeyMsg{Type: tea.KeyEnter})

	msg := submittedComment(t, cmd)
	testkit.AssertEqual(t, "key", msg.PullRequestKey, testPRKey)
	testkit.AssertEqual(t, "parent", msg.ParentID, int64(testInlineRootID))
	testkit.AssertEqual(t, "text", msg.Text, "looks right to me")
	if view.Composing() {
		t.Error("the composer should close once it has sent")
	}
}

func TestComposer_RepliesToTheConversationInTheSection(t *testing.T) {
	t.Parallel()
	view := threadsDetail(t)
	view.openThread(testGeneralRootID)
	view.View()

	if !view.BeginComment() {
		t.Fatal("the thread section should take a reply")
	}

	testkit.AssertEqual(t, "mode", view.Mode(), ModeThread)
	testkit.AssertEqual(t, "parent", view.ComposerParent(), int64(testGeneralRootID))
}

func TestComposer_WritesANewCommentFromTheOverviewTab(t *testing.T) {
	t.Parallel()
	view := threadsDetail(t)
	view.SetActiveTab(TabOverview)
	view.View()

	if !view.BeginComment() {
		t.Fatal("the Overview tab should take a comment on the pull request")
	}
	// No parent: this starts a conversation rather than joining one.
	testkit.AssertEqual(t, "parent", view.ComposerParent(), int64(0))

	typeInto(t, view, "ready from my side")
	_, cmd := view.Update(tea.KeyMsg{Type: tea.KeyEnter})

	msg := submittedComment(t, cmd)
	testkit.AssertEqual(t, "parent", msg.ParentID, int64(0))
	testkit.AssertEqual(t, "text", msg.Text, "ready from my side")
}

func TestComposer_IsRefusedWhereThereIsNothingToAnswer(t *testing.T) {
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

			if view.BeginComment() {
				t.Errorf("%s should not open the composer", tc.name)
			}
			if view.Composing() {
				t.Error("nothing should be open")
			}
		})
	}
}

func TestComposer_TakesTheFirstCommentOfAnEmptyPullRequest(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())
	view.SetThreads(testPRKey, nil)
	view.SetActiveTab(TabComments)
	view.View()

	if !view.BeginComment() {
		t.Fatal("an empty Comments tab should still take the first comment")
	}
	testkit.AssertEqual(t, "parent", view.ComposerParent(), int64(0))
}

func TestComposer_OwnsEveryKeyWhileItIsOpen(t *testing.T) {
	t.Parallel()
	view := threadsDetail(t)
	before := view.listCursor
	view.BeginComment()

	// j and k navigate the thread list; inside the composer they are letters.
	typeInto(t, view, "jk quit")

	testkit.AssertEqual(t, "cursor", view.listCursor, before)
	testkit.AssertEqual(t, "text", view.ComposerText(), "jk quit")
}

func TestComposer_EscapeDropsWhatWasTyped(t *testing.T) {
	t.Parallel()
	view := threadsDetail(t)
	view.BeginComment()
	typeInto(t, view, "never mind")

	_, cmd := view.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if cmd != nil {
		t.Error("cancelling must not report anything to post")
	}
	if view.Composing() {
		t.Error("escape should close the composer")
	}
	testkit.AssertEqual(t, "text", view.ComposerText(), "")
}

func TestComposer_EnterOnAnEmptyBoxJustCloses(t *testing.T) {
	t.Parallel()
	view := threadsDetail(t)
	view.BeginComment()
	typeInto(t, view, "   ")

	_, cmd := view.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd != nil {
		t.Errorf("whitespace is not a comment, got %T", cmd())
	}
	if view.Composing() {
		t.Error("the composer should close")
	}
}

func TestComposer_CtrlEHandsTheTextToTheEditor(t *testing.T) {
	t.Parallel()
	view := threadsDetail(t)
	view.BeginComment()
	typeInto(t, view, "a longer thought")

	_, cmd := view.Update(tea.KeyMsg{Type: tea.KeyCtrlE})

	if cmd == nil {
		t.Fatal("ctrl+e should ask the app for the editor")
	}
	msg, ok := cmd().(ComposeCommentInEditorMsg)
	if !ok {
		t.Fatalf("command produced %T, want ComposeCommentInEditorMsg", cmd())
	}
	testkit.AssertEqual(t, "key", msg.PullRequestKey, testPRKey)
	testkit.AssertEqual(t, "parent", msg.ParentID, int64(testInlineRootID))
	// The text travels with it: the editor opens on what was already written.
	testkit.AssertEqual(t, "text", msg.Text, "a longer thought")
	if view.Composing() {
		t.Error("the box closes while the editor has the terminal")
	}
}

func TestComposer_ResumeCarriesAFailedReplyBack(t *testing.T) {
	t.Parallel()
	view := threadsDetail(t)

	view.ResumeComment(testInlineRootID, nil, "the reply that did not send")

	if !view.Composing() {
		t.Fatal("resuming should reopen the composer")
	}
	testkit.AssertEqual(t, "parent", view.ComposerParent(), int64(testInlineRootID))
	testkit.AssertEqual(t, "text", view.ComposerText(), "the reply that did not send")
}

func TestComposer_IsDrawnUnderTheConversation(t *testing.T) {
	t.Parallel()
	view := threadsDetail(t)
	view.BeginComment()
	typeInto(t, view, "sending this")

	out := stripANSI(view.View())

	if !strings.Contains(out, "Reply") {
		t.Errorf("the box should be labelled for a reply:\n%s", out)
	}
	if !strings.Contains(out, "sending this") {
		t.Errorf("the box should show what was typed:\n%s", out)
	}
	if !strings.Contains(out, composerHint) {
		t.Errorf("the box should name the keys:\n%s", out)
	}
	// The thread list is still behind it rather than replaced by it.
	if !strings.Contains(out, "This needs a nil check.") {
		t.Errorf("the conversation should still be on screen:\n%s", out)
	}
}

func TestComposer_LabelsANewCommentAsAComment(t *testing.T) {
	t.Parallel()
	view := threadsDetail(t)
	view.SetActiveTab(TabOverview)
	view.View()
	view.BeginComment()

	out := stripANSI(view.View())

	if !strings.Contains(out, "Comment") {
		t.Errorf("a comment on the pull request should say so:\n%s", out)
	}
}

func TestComposer_ClosesWhenThePanelMovesOn(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		move func(view *DetailView)
	}{
		{name: "another tab", move: func(view *DetailView) { view.SetActiveTab(TabCommits) }},
		{name: "another pull request", move: func(view *DetailView) {
			other := detailPR()
			other.ID = 202
			view.SetPullRequest(other)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			view := threadsDetail(t)
			view.BeginComment()

			tc.move(view)

			if view.Composing() {
				t.Errorf("%s should close the composer", tc.name)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Inline anchors
// -----------------------------------------------------------------------------

// composerAnchorAt puts the diff cursor on row and opens the composer there.
func composerAnchorAt(t *testing.T, view *DetailView, row int) *bitbucket.CommentAnchor {
	t.Helper()
	view.listCursor = row
	if !view.BeginComment() {
		t.Fatalf("row %d should take an inline comment", row)
	}
	anchor := view.ComposerAnchor()
	if anchor == nil {
		t.Fatalf("row %d opened the composer with no anchor", row)
	}
	return anchor
}

func TestComposer_AnchorsToTheLineUnderTheDiffCursor(t *testing.T) {
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

			anchor := composerAnchorAt(t, view, tc.row)

			testkit.AssertEqual(t, "path", anchor.Path, testDiffPath)
			testkit.AssertEqual(t, "line", anchor.Line, tc.line)
			testkit.AssertEqual(t, "old side", anchor.OldSide, tc.oldSide)
			testkit.AssertEqual(t, "parent", view.ComposerParent(), int64(0))
		})
	}
}

func TestComposer_AnchorsToTheFileOnARowWithNoLine(t *testing.T) {
	t.Parallel()

	// Row 0 is the hunk header and row 6 the "\ No newline" marker: neither is
	// a line of the file, so the comment hangs off the file itself.
	for _, row := range []int{0, 6} {
		t.Run(strconv.Itoa(row), func(t *testing.T) {
			t.Parallel()
			view := fileDiffDetail(t, nil)

			anchor := composerAnchorAt(t, view, row)

			testkit.AssertEqual(t, "path", anchor.Path, testDiffPath)
			testkit.AssertEqual(t, "line", anchor.Line, 0)
		})
	}
}

func TestComposer_AnchorsToTheSelectedFileOnTheFilesTab(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())
	view.SetDiffStat(testPRKey, testDiffStat())
	view.SetActiveTab(TabFiles)
	view.View()

	if !view.BeginComment() {
		t.Fatal("the Files tab should take a comment on the selected file")
	}

	anchor := view.ComposerAnchor()
	if anchor == nil {
		t.Fatal("the Files tab should anchor the comment to a file")
	}
	testkit.AssertEqual(t, "path", anchor.Path, testDiffStat()[0].Path())
	testkit.AssertEqual(t, "line", anchor.Line, 0)
}

func TestComposer_AMarkedLineStartsASecondConversation(t *testing.T) {
	t.Parallel()
	// Line 100 already carries a thread, which is what the marker says.
	view := fileDiffDetail(t, map[int]int{100: 1})

	anchor := composerAnchorAt(t, view, 4)

	// C starts a conversation wherever it is pressed; joining the one that is
	// there is what enter and then C do.
	testkit.AssertEqual(t, "parent", view.ComposerParent(), int64(0))
	testkit.AssertEqual(t, "line", anchor.Line, 100)
}

func TestComposer_SendsTheAnchorWithTheText(t *testing.T) {
	t.Parallel()
	view := fileDiffDetail(t, nil)
	view.listCursor = 3
	view.BeginComment()
	typeInto(t, view, "this shadows the outer ctx")

	_, cmd := view.Update(tea.KeyMsg{Type: tea.KeyEnter})

	msg := submittedComment(t, cmd)
	if msg.Anchor == nil {
		t.Fatal("an inline comment should carry its anchor")
	}
	testkit.AssertEqual(t, "path", msg.Anchor.Path, testDiffPath)
	testkit.AssertEqual(t, "line", msg.Anchor.Line, 99)
	testkit.AssertEqual(t, "old side", msg.Anchor.OldSide, false)
	testkit.AssertEqual(t, "text", msg.Text, "this shadows the outer ctx")
}

func TestComposer_TheEditorHandOffKeepsTheAnchor(t *testing.T) {
	t.Parallel()
	view := fileDiffDetail(t, nil)
	view.listCursor = 2
	view.BeginComment()

	_, cmd := view.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	if cmd == nil {
		t.Fatal("ctrl+e should ask the app for the editor")
	}
	msg, ok := cmd().(ComposeCommentInEditorMsg)
	if !ok {
		t.Fatalf("command produced %T, want ComposeCommentInEditorMsg", cmd())
	}

	if msg.Anchor == nil {
		t.Fatal("the editor hand-off should carry the anchor")
	}
	testkit.AssertEqual(t, "line", msg.Anchor.Line, 99)
	testkit.AssertEqual(t, "old side", msg.Anchor.OldSide, true)
}

func TestComposer_LabelsAnInlineCommentWithTheLine(t *testing.T) {
	t.Parallel()
	view := fileDiffDetail(t, nil)
	view.listCursor = 3
	view.BeginComment()

	out := stripANSI(view.View())

	// The base name and the line: the panel title already carries the path.
	if !strings.Contains(out, "app.go:99") {
		t.Errorf("the box should name the anchored line:\n%s", out)
	}
	// And the diff is still behind it.
	if !strings.Contains(out, "defer stop()") {
		t.Errorf("the diff should still be on screen:\n%s", out)
	}
}
