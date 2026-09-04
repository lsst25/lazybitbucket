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

func TestDetailView_Threads_EnterExpandsTheWholeThread(t *testing.T) {
	t.Parallel()
	view := threadsDetail(t)

	_, cmd := view.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter on a thread emitted nothing")
	}
	msg, ok := cmd().(ExpandBlockMsg)
	if !ok {
		t.Fatalf("msg = %T, want ExpandBlockMsg", cmd())
	}
	testkit.AssertEqual(t, "title", msg.Title, "Thread "+testDiffPath+":12")

	body := stripANSI(strings.Join(msg.Lines, "\n"))
	for _, want := range []string{"This needs a nil check.", "Fixed in the next commit."} {
		if !strings.Contains(body, want) {
			t.Errorf("expansion does not contain %q:\n%s", want, body)
		}
	}
}

func TestDetailView_Threads_LongThreadCollapsesAndExpands(t *testing.T) {
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

	_, cmd := view.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter on a collapsed thread emitted nothing")
	}
	msg, ok := cmd().(ExpandBlockMsg)
	if !ok {
		t.Fatalf("msg = %T, want ExpandBlockMsg", cmd())
	}
	if len(msg.Lines) <= maxBlockLines {
		t.Errorf("len(Lines) = %d, want the unclamped thread", len(msg.Lines))
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
