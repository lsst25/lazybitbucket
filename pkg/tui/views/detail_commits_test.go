package views

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// testCommitBody is the part of a commit message the list does not show, so a
// test can tell the block apart from the expansion.
const testCommitBody = "The gateway rejected anonymous requests with a 500."

// testCommits is two commits: one whose git ident matched a Bitbucket account
// and one that did not, which is the AuthorName fallback the list has to cope
// with.
func testCommits() []bitbucket.Commit {
	return []bitbucket.Commit{
		{
			Hash:    testCommitHash,
			Date:    time.Now().Add(-72 * time.Hour),
			Message: "Add gateway auth\n\n" + testCommitBody + "\n",
			Author: bitbucket.CommitAuthor{
				Raw:  testAuthor + " <grace@example.com>",
				User: &bitbucket.Account{UUID: "{author}", DisplayName: testAuthor},
			},
			Links: bitbucket.CommitLinks{
				HTML: bitbucket.Link{Href: "https://bitbucket.org/acme/api-gateway/commits/" + testCommitHash},
			},
		},
		{
			Hash:    "fedcba9876543210fedcba9876543210fedcba98",
			Date:    time.Now().Add(-2 * time.Hour),
			Message: "Fix the lockfile\n",
			Author:  bitbucket.CommitAuthor{Raw: "Alan Turing <alan@example.com>"},
		},
	}
}

// commitsDetail returns a focused detail view on the Commits tab with the
// commits loaded and View() called.
func commitsDetail(t *testing.T) *DetailView {
	t.Helper()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())
	view.SetCommits(testPRKey, testCommits())
	view.SetActiveTab(TabCommits)
	view.View()
	return view
}

func TestDetailView_Commits_RendersHashAuthorAgeAndSubject(t *testing.T) {
	t.Parallel()
	out := stripANSI(commitsDetail(t).View())

	for _, want := range []string{
		"abc1234", testAuthor, "3d ago", "Add gateway auth",
		"fedcba9", "Alan Turing", "2h ago", "Fix the lockfile",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("view does not contain %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, testCommitBody) {
		t.Errorf("the list must not show the message body:\n%s", out)
	}
	if !strings.Contains(out, "1 of 2") {
		t.Errorf("footer should count the commits:\n%s", out)
	}
}

func TestDetailView_Commits_PlaceholderStates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		setup func(view *DetailView)
		want  string
	}{
		{"before the fetch", func(*DetailView) {}, "Loading commits…"},
		{"loading", func(v *DetailView) { v.SetCommitsLoading(testPRKey) }, "Loading commits…"},
		{"error", func(v *DetailView) { v.SetCommitsError(testPRKey, "403 forbidden") }, "403 forbidden"},
		{"empty", func(v *DetailView) { v.SetCommits(testPRKey, nil) }, "No commits"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			view := makeFocusedDetail()
			view.SetPullRequest(detailPR())
			view.SetActiveTab(TabCommits)
			tc.setup(view)

			out := stripANSI(view.View())
			if !strings.Contains(out, tc.want) {
				t.Errorf("view does not contain %q:\n%s", tc.want, out)
			}
			if got := view.SelectedCommit(); got != nil {
				t.Errorf("SelectedCommit() = %v, want nil", got)
			}
		})
	}
}

func TestDetailView_Commits_EnterExpandsTheWholeMessage(t *testing.T) {
	t.Parallel()
	view := commitsDetail(t)

	_, cmd := view.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter on a commit emitted nothing")
	}
	msg, ok := cmd().(ExpandBlockMsg)
	if !ok {
		t.Fatalf("msg = %T, want ExpandBlockMsg", cmd())
	}
	testkit.AssertEqual(t, "title", msg.Title, "Commit abc1234")

	body := stripANSI(strings.Join(msg.Lines, "\n"))
	for _, want := range []string{testCommitHash, testAuthor, "Add gateway auth", testCommitBody} {
		if !strings.Contains(body, want) {
			t.Errorf("expansion does not contain %q:\n%s", want, body)
		}
	}
}

func TestDetailView_Commits_SelectedCommitTracksTheCursor(t *testing.T) {
	t.Parallel()
	view := commitsDetail(t)

	testkit.AssertEqual(t, "first commit", view.SelectedCommit().ShortHash(), "abc1234")
	view.ScrollBy(1)
	testkit.AssertEqual(t, "second commit", view.SelectedCommit().ShortHash(), "fedcba9")
}

func TestDetailView_Commits_RefetchKeepsTheListAndTheCursor(t *testing.T) {
	t.Parallel()
	view := commitsDetail(t)
	view.ScrollBy(1)
	second := view.SelectedCommit().Hash

	view.SetCommitsLoading(testPRKey)
	out := stripANSI(view.View())
	if strings.Contains(out, "Loading commits…") {
		t.Errorf("a refetch must not replace the list it is refreshing:\n%s", out)
	}
	testkit.AssertEqual(t, "block count", view.listTabItemCount(), len(testCommits()))
	testkit.AssertEqual(t, "cursor kept", view.SelectedCommit().Hash, second)

	view.SetCommitsError(testPRKey, "403 forbidden")
	out = stripANSI(view.View())
	if !strings.Contains(out, "403 forbidden") {
		t.Errorf("a failed refetch should be reported above the list:\n%s", out)
	}
	testkit.AssertEqual(t, "cursor kept after the error", view.SelectedCommit().Hash, second)
}
