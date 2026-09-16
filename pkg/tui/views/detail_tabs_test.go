package views

import (
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// tabLabelFor is the label the tab bar would draw for one tab, count and all.
// It reads the labels rather than the rendered title, so no assertion depends
// on the panel being wide enough to hold every count.
func tabLabelFor(t *testing.T, view *DetailView, tab DetailTab) string {
	t.Helper()
	for _, label := range view.tabLabels() {
		if label.tab == tab {
			return label.label
		}
	}
	t.Fatalf("no label for tab %v", tab)
	return ""
}

func TestDetailView_Tabs_CommentsCountIsKnownBeforeTheThreadsLoad(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())

	// detailPR() carries comment_count 7, which the tab shows before anything
	// is fetched and while the fetch runs — no ellipsis, the number is real.
	testkit.AssertEqual(t, "before the fetch", tabLabelFor(t, view, TabComments), "Comments (7)")

	view.SetThreadsLoading(testPRKey)
	testkit.AssertEqual(t, "while loading", tabLabelFor(t, view, TabComments), "Comments (7)")

	view.SetShowResolvedThreads(true)
	view.SetThreads(testPRKey, testThreads())
	// testThreads() holds five comments: the inline root and its reply, the
	// resolved root, the pending general root and the live reply under the
	// deleted placeholder, which is not a comment any more.
	testkit.AssertEqual(t, "fixture total", bitbucket.CommentTotal(testThreads()), 5)
	testkit.AssertEqual(t, "once loaded", tabLabelFor(t, view, TabComments), "Comments (5)")

	// A refetch falls back to the pull request's number, which r refreshes
	// alongside the threads, rather than to an ellipsis.
	view.SetThreadsLoading(testPRKey)
	testkit.AssertEqual(t, "while refetching", tabLabelFor(t, view, TabComments), "Comments (7)")
}

func TestDetailView_Tabs_CommentsCountsCommentsNotThreads(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())
	// The label's business here is the count, not the "· n hidden" the filter
	// appends to it.
	view.SetShowResolvedThreads(true)
	view.SetThreads(testPRKey, testThreads())

	threads := view.Threads()
	testkit.AssertEqual(t, "threads", len(threads), 4)
	// Four threads, five comments: the list below counts threads, the tab
	// counts comments, like the overview's "· N comments".
	testkit.AssertEqual(t, "label", tabLabelFor(t, view, TabComments), "Comments (5)")
}

func TestDetailView_Tabs_CommentsCountZeroIsAnAnswer(t *testing.T) {
	t.Parallel()
	pr := detailPR()
	pr.CommentCount = 0
	view := makeFocusedDetail()
	view.SetPullRequest(pr)

	testkit.AssertEqual(t, "before the fetch", tabLabelFor(t, view, TabComments), "Comments (0)")

	view.SetThreads(testPRKey, nil)
	testkit.AssertEqual(t, "once loaded", tabLabelFor(t, view, TabComments), "Comments (0)")
}

func TestDetailView_Tabs_CommentsKeepTheKnownCountWhenTheFetchFails(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())

	view.SetThreadsLoading(testPRKey)
	view.SetThreadsError(testPRKey, "403 forbidden")

	// The failure belongs on the pane; the tab still knows what the pull
	// request said.
	testkit.AssertEqual(t, "label", tabLabelFor(t, view, TabComments), "Comments (7)")
}

func TestDetailView_Tabs_FilesAndCommitsCountOnlyWhatLoaded(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())

	// The pull request payload carries no file or commit count, so neither tab
	// claims one before its data lands.
	testkit.AssertEqual(t, "files unknown", tabLabelFor(t, view, TabFiles), "Files")
	testkit.AssertEqual(t, "commits unknown", tabLabelFor(t, view, TabCommits), "Commits")

	view.SetDiffStatLoading(testPRKey)
	view.SetCommitsLoading(testPRKey)
	testkit.AssertEqual(t, "files loading", tabLabelFor(t, view, TabFiles), "Files (…)")
	testkit.AssertEqual(t, "commits loading", tabLabelFor(t, view, TabCommits), "Commits (…)")

	view.SetDiffStat(testPRKey, testDiffStat()[:3])
	view.SetCommits(testPRKey, testCommits())
	testkit.AssertEqual(t, "files loaded", tabLabelFor(t, view, TabFiles), "Files (3)")
	testkit.AssertEqual(t, "commits loaded", tabLabelFor(t, view, TabCommits), "Commits (2)")
}
