package views

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// Root ids of the thread fixtures, so a test can name the thread it means.
const (
	testInlineRootID   = 1001
	testResolvedRootID = 1002
	testGeneralRootID  = 1003
	testDeletedRootID  = 1004
)

// threadComment builds one comment for the thread fixtures.
func threadComment(id int64, author, body string, ago time.Duration) bitbucket.Comment {
	return bitbucket.Comment{
		ID:        id,
		CreatedOn: time.Now().Add(-ago),
		UpdatedOn: time.Now().Add(-ago),
		Content:   bitbucket.RenderedContent{Raw: body, Markup: "markdown"},
		User:      bitbucket.Account{UUID: "{u}", DisplayName: author},
		Links: bitbucket.CommentLinks{
			HTML: bitbucket.Link{
				Href: fmt.Sprintf("https://bitbucket.org/acme/api-gateway/pull-requests/101#comment-%d", id),
			},
		},
	}
}

// inlineAnchor anchors a comment to a new-side line of testDiffPath.
func inlineAnchor(comment bitbucket.Comment, line int) bitbucket.Comment {
	comment.Inline = &bitbucket.CommentInline{Path: testDiffPath, To: &line}
	return comment
}

// testThreads is the four review threads the Comments tab tests render: an
// inline thread with a reply, a resolved one, a general one, and a deleted root
// that survives because something replied to it.
func testThreads() []bitbucket.Thread {
	inline := inlineAnchor(threadComment(testInlineRootID, testReviewer, "This needs a nil check.", 3*time.Hour), 12)
	reply := inlineAnchor(threadComment(2001, testAuthor, "Fixed in the next commit.", time.Hour), 12)
	reply.Parent = &bitbucket.CommentRef{ID: testInlineRootID}

	resolved := inlineAnchor(threadComment(testResolvedRootID, testAuthor, "Naming nit.", 5*time.Hour), 40)
	resolved.Resolution = &bitbucket.CommentResolution{
		Type:      "pullrequest_comment_resolution",
		User:      &bitbucket.Account{UUID: "{r}", DisplayName: testReviewer},
		CreatedOn: time.Now().Add(-2 * time.Hour),
	}

	pending := threadComment(testGeneralRootID, testReviewer, "Overall this looks good.", 4*time.Hour)
	pending.Pending = true

	deleted := threadComment(testDeletedRootID, testAuthor, "", 6*time.Hour)
	deleted.Deleted = true
	deletedReply := threadComment(2002, testReviewer, "Still relevant though.", 5*time.Hour)
	deletedReply.Parent = &bitbucket.CommentRef{ID: testDeletedRootID}

	return bitbucket.BuildThreads([]bitbucket.Comment{
		inline, reply, resolved, pending, deleted, deletedReply,
	})
}

// threadsDetail returns a focused detail view on the Comments tab with the
// threads loaded and View() called.
func threadsDetail(t *testing.T) *DetailView {
	t.Helper()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())
	view.SetThreads(testPRKey, testThreads())
	view.SetActiveTab(TabComments)
	view.SetSize(80, 60)
	view.View()
	return view
}

func TestDetailView_Threads_RendersAnchorsBodiesAndReplies(t *testing.T) {
	t.Parallel()
	out := stripANSI(threadsDetail(t).View())

	for _, want := range []string{
		testDiffPath + ":12",
		"1 reply",
		testReviewer,
		"This needs a nil check.",
		"↳ " + testAuthor,
		"Fixed in the next commit.",
		testDiffPath + ":40",
		"✔ resolved",
		generalThreadLabel,
		"(pending)",
		"(deleted)",
		"Still relevant though.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("view does not contain %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "1 of 4") {
		t.Errorf("footer should count the threads:\n%s", out)
	}
}

func TestDetailView_Threads_PlaceholderStates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		setup func(view *DetailView)
		want  string
	}{
		{"before the fetch", func(*DetailView) {}, "Loading comments…"},
		{"loading", func(v *DetailView) { v.SetThreadsLoading(testPRKey) }, "Loading comments…"},
		{"error", func(v *DetailView) { v.SetThreadsError(testPRKey, "rate limited") }, "rate limited"},
		{"empty", func(v *DetailView) { v.SetThreads(testPRKey, nil) }, "No comments"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			view := makeFocusedDetail()
			view.SetPullRequest(detailPR())
			view.SetActiveTab(TabComments)
			tc.setup(view)

			out := stripANSI(view.View())
			if !strings.Contains(out, tc.want) {
				t.Errorf("view does not contain %q:\n%s", tc.want, out)
			}
			if got := view.SelectedThread(); got != nil {
				t.Errorf("SelectedThread() = %v, want nil", got)
			}
		})
	}
}

func TestDetailView_Threads_EnterOpensTheThreadInThePanel(t *testing.T) {
	t.Parallel()
	view := threadsDetail(t)

	// Enter opens the conversation in the panel itself, the way Enter on a
	// changed file opens its diff — no modal, no message for the app to route.
	_, cmd := view.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("opening a thread should need no command, got %T", cmd())
	}
	testkit.AssertEqual(t, "mode", view.Mode(), ModeThread)
	testkit.AssertEqual(t, "open thread", view.OpenedThread().Root.ID, int64(testInlineRootID))

	out := stripANSI(view.View())
	if !strings.Contains(out, "[0] Thread "+testDiffPath+":12") {
		t.Errorf("the title should name the conversation:\n%s", out)
	}
	for _, want := range []string{"This needs a nil check.", "Fixed in the next commit."} {
		if !strings.Contains(out, want) {
			t.Errorf("the section does not contain %q:\n%s", want, out)
		}
	}
}

func TestDetailView_Threads_BackReturnsToTheCommentsTab(t *testing.T) {
	t.Parallel()
	view := threadsDetail(t)
	view.ScrollBy(1)
	second := view.SelectedThread().Root.ID

	view.Update(tea.KeyMsg{Type: tea.KeyEnter})
	testkit.AssertEqual(t, "mode", view.Mode(), ModeThread)

	testkit.AssertEqual(t, "Back()", view.Back(), true)
	testkit.AssertEqual(t, "mode", view.Mode(), ModePullRequest)
	testkit.AssertEqual(t, "tab", view.ActiveTab(), TabComments)
	// The cursor comes back to the conversation just read, not to the top of
	// the list.
	testkit.AssertEqual(t, "cursor", view.SelectedThread().Root.ID, second)
	if view.OpenedThread() != nil {
		t.Error("walking out of a thread should forget it")
	}
}

func TestDetailView_Threads_SectionScrollsInsteadOfCollapsing(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())
	root := inlineAnchor(threadComment(9001, testReviewer,
		strings.Repeat("a wordy review remark. ", 40), time.Hour), 5)
	view.SetThreads(testPRKey, bitbucket.BuildThreads([]bitbucket.Comment{root}))
	view.SetActiveTab(TabComments)
	view.SetSize(80, 12)
	view.View()

	view.Update(tea.KeyMsg{Type: tea.KeyEnter})
	testkit.AssertEqual(t, "mode", view.Mode(), ModeThread)

	// The list collapses this thread behind an ellipsis; the section does not —
	// it is a scrolling body, like a step log.
	out := stripANSI(view.View())
	if strings.Contains(out, "    ...") {
		t.Errorf("the section should not collapse the conversation:\n%s", out)
	}
	view.ScrollBy(3)
	if scrolled := stripANSI(view.View()); scrolled == out {
		t.Error("the section should scroll")
	}
}

func TestDetailView_Threads_SectionSaysWhenTheThreadIsGone(t *testing.T) {
	t.Parallel()
	view := threadsDetail(t)
	view.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// A refetch that no longer carries the open conversation — it was deleted,
	// or the pull request was pushed to — leaves the reader with a sentence
	// rather than with a stale copy of it.
	view.SetThreads(testPRKey, nil)
	out := stripANSI(view.View())
	if !strings.Contains(out, "no longer in the pull request") {
		t.Errorf("the section should say the thread is gone:\n%s", out)
	}
	if view.OpenedThread() != nil {
		t.Error("OpenedThread() should be nil once the thread is gone")
	}
}

func TestDetailView_Threads_TabKeyLeavesTheSection(t *testing.T) {
	t.Parallel()
	view := threadsDetail(t)
	view.Update(tea.KeyMsg{Type: tea.KeyEnter})

	view.SetActiveTab(TabFiles)
	testkit.AssertEqual(t, "mode", view.Mode(), ModePullRequest)
	testkit.AssertEqual(t, "tab", view.ActiveTab(), TabFiles)
	if view.OpenedThread() != nil {
		t.Error("a tab key should walk out of the conversation")
	}
}

func TestDetailView_Threads_LongThreadCollapsesInTheListAndOpensWhole(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())

	root := inlineAnchor(threadComment(9001, testReviewer, strings.Repeat("a wordy review remark. ", 40), time.Hour), 5)
	view.SetThreads(testPRKey, bitbucket.BuildThreads([]bitbucket.Comment{root}))
	view.SetActiveTab(TabComments)

	out := stripANSI(view.View())
	if !strings.Contains(out, "...") {
		t.Errorf("a long thread should collapse behind the ellipsis line:\n%s", out)
	}

	// A collapsed block used to expand into a modal, and the fallback that did
	// it must not fire behind the section: opening the thread is the whole of
	// Enter here, command and all.
	_, cmd := view.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("opening a collapsed thread should emit nothing, got %T", cmd())
	}
	testkit.AssertEqual(t, "mode", view.Mode(), ModeThread)
	if lines := view.threadBodyLines(78); len(lines) <= maxBlockLines {
		t.Errorf("len(lines) = %d, want the unclamped conversation", len(lines))
	}
}

func TestDetailView_FocusThread_PutsTheCursorOnTheThread(t *testing.T) {
	t.Parallel()
	view := threadsDetail(t)
	view.SetActiveTab(TabOverview)

	testkit.AssertEqual(t, "unknown thread", view.FocusThread(424242), false)
	testkit.AssertEqual(t, "known thread", view.FocusThread(testGeneralRootID), true)
	testkit.AssertEqual(t, "mode", view.Mode(), ModePullRequest)
	testkit.AssertEqual(t, "tab", view.ActiveTab(), TabComments)
	testkit.AssertEqual(t, "cursor", view.SelectedThread().Root.ID, int64(testGeneralRootID))
}

func TestExtractURLs_AddsCommitsAndComments(t *testing.T) {
	t.Parallel()
	pr := newPR(101, testRepo, testPRTitle)
	pr.Links.HTML.Href = "https://bitbucket.org/acme/api-gateway/pull-requests/101"

	groups := ExtractURLs(&pr, nil, testCommits(), testThreads())

	sections := make([]string, 0, len(groups))
	for _, group := range groups {
		sections = append(sections, group.Section)
	}
	testkit.AssertSliceEqual(t, "sections", sections, []string{"Pull request", "Commits", "Comments"})

	testkit.AssertSliceEqual(t, "commit URLs", groups[1].URLs,
		[]string{"https://bitbucket.org/acme/api-gateway/commits/" + testCommitHash})
	if len(groups[2].URLs) == 0 {
		t.Error("the Comments section should carry the thread roots' URLs")
	}
}

func TestDetailView_Threads_RefetchKeepsTheListAndTheCursor(t *testing.T) {
	t.Parallel()
	view := threadsDetail(t)
	view.ScrollBy(1)
	second := view.SelectedThread().Root.ID

	view.SetThreadsLoading(testPRKey)
	out := stripANSI(view.View())
	if strings.Contains(out, "Loading comments…") {
		t.Errorf("a refetch must not replace the list it is refreshing:\n%s", out)
	}
	testkit.AssertEqual(t, "block count", view.listTabItemCount(), len(testThreads()))
	testkit.AssertEqual(t, "cursor kept", view.SelectedThread().Root.ID, second)

	view.SetThreadsError(testPRKey, "403 forbidden")
	out = stripANSI(view.View())
	if !strings.Contains(out, "403 forbidden") {
		t.Errorf("a failed refetch should be reported above the list:\n%s", out)
	}
	testkit.AssertEqual(t, "cursor kept after the error", view.SelectedThread().Root.ID, second)
}
