package bitbucket

import (
	"strconv"
	"testing"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// threadBase is the timestamp comment fixtures count minutes from, so ordering
// is readable in the test bodies.
var threadBase = time.Date(2026, time.August, 4, 12, 0, 0, 0, time.UTC)

// commentOpts builds one comment for the thread tests. Only the fields the
// grouping and ordering rules read are set.
type commentOpts struct {
	id       int64
	parent   int64
	minutes  int
	updated  int
	path     string
	line     int
	oldLine  int
	deleted  bool
	resolved bool
	body     string
	fileOnly bool
}

func makeComment(opts commentOpts) Comment {
	comment := Comment{
		Type:       "pullrequest_comment",
		ID:         opts.id,
		CreatedOn:  threadBase.Add(time.Duration(opts.minutes) * time.Minute),
		UpdatedOn:  threadBase.Add(time.Duration(max(opts.updated, opts.minutes)) * time.Minute),
		Content:    RenderedContent{Type: "rendered", Raw: opts.body, Markup: "markdown", HTML: ""},
		User:       Account{Type: "user", UUID: testAuthorUUID, DisplayName: "Yurii Tatar"},
		Deleted:    opts.deleted,
		Pending:    false,
		Parent:     nil,
		Inline:     nil,
		Resolution: nil,
		Links:      CommentLinks{},
	}

	if opts.parent != 0 {
		comment.Parent = &CommentRef{ID: opts.parent}
	}
	if opts.path != "" {
		inline := &CommentInline{Path: opts.path, From: nil, To: nil, StartFrom: nil, StartTo: nil}
		switch {
		case opts.fileOnly:
		case opts.oldLine != 0:
			inline.From = &opts.oldLine
		default:
			inline.To = &opts.line
		}
		comment.Inline = inline
	}
	if opts.resolved {
		comment.Resolution = &CommentResolution{
			Type:      "pullrequest_comment_resolution",
			User:      &Account{UUID: testReviewerUUID},
			CreatedOn: threadBase.Add(time.Hour),
		}
	}
	return comment
}

// threadIDs is the root id of every thread, in order — the shape the ordering
// assertions compare.
func threadIDs(threads []Thread) []int64 {
	ids := make([]int64, 0, len(threads))
	for _, thread := range threads {
		ids = append(ids, thread.Root.ID)
	}
	return ids
}

// replyIDs is the ids of one thread's replies, in order.
func replyIDs(thread Thread) []int64 {
	ids := make([]int64, 0, len(thread.Replies))
	for _, reply := range thread.Replies {
		ids = append(ids, reply.ID)
	}
	return ids
}

func TestBuildThreads_Empty(t *testing.T) {
	t.Parallel()

	if threads := BuildThreads(nil); threads != nil {
		t.Errorf("BuildThreads(nil) = %v, want nil", threads)
	}
	if lines := CommentedLines(nil); lines != nil {
		t.Errorf("CommentedLines(nil) = %v, want nil", lines)
	}
}

func TestBuildThreads_GroupsRepliesUnderTheirRoot(t *testing.T) {
	t.Parallel()

	// Deliberately out of chronological order: the input order must not decide
	// the output order.
	comments := []Comment{
		makeComment(commentOpts{id: 30, parent: 10, minutes: 30, body: "later reply"}),
		makeComment(commentOpts{id: 10, minutes: 0, body: "root"}),
		makeComment(commentOpts{id: 20, parent: 10, minutes: 10, body: "first reply"}),
	}

	threads := BuildThreads(comments)

	testkit.AssertEqual(t, "threads", len(threads), 1)
	testkit.AssertEqual(t, "root", threads[0].Root.ID, int64(10))
	testkit.AssertSliceEqual(t, "replies", replyIDs(threads[0]), []int64{20, 30})
	testkit.AssertEqual(t, "IsInline", threads[0].IsInline(), false)
	testkit.AssertEqual(t, "Path", threads[0].Path(), "")
	testkit.AssertEqual(t, "Resolved", threads[0].Resolved(), false)
	testkit.AssertEqual(t, "LastActivity", threads[0].LastActivity(), threadBase.Add(30*time.Minute))
}

func TestBuildThreads_FlattensNestedRepliesInChronologicalOrder(t *testing.T) {
	t.Parallel()

	// A reply to a reply belongs to the same thread, flattened by time.
	comments := []Comment{
		makeComment(commentOpts{id: 1, minutes: 0, body: "root"}),
		makeComment(commentOpts{id: 2, parent: 1, minutes: 5, body: "reply"}),
		makeComment(commentOpts{id: 3, parent: 2, minutes: 10, body: "reply to the reply"}),
		makeComment(commentOpts{id: 4, parent: 3, minutes: 15, body: "and deeper"}),
	}

	threads := BuildThreads(comments)

	testkit.AssertEqual(t, "threads", len(threads), 1)
	testkit.AssertSliceEqual(t, "replies", replyIDs(threads[0]), []int64{2, 3, 4})
}

func TestBuildThreads_RepliesWithTheSameTimestampOrderByID(t *testing.T) {
	t.Parallel()

	comments := []Comment{
		makeComment(commentOpts{id: 1, minutes: 0, body: "root"}),
		makeComment(commentOpts{id: 9, parent: 1, minutes: 5, body: "b"}),
		makeComment(commentOpts{id: 7, parent: 1, minutes: 5, body: "a"}),
	}

	threads := BuildThreads(comments)

	testkit.AssertSliceEqual(t, "replies", replyIDs(threads[0]), []int64{7, 9})
}

func TestBuildThreads_OrphanReplyBecomesItsOwnRoot(t *testing.T) {
	t.Parallel()

	// The parent is not in the list — a partial fetch, or a parent the caller
	// cannot see. The reply must still be shown.
	comments := []Comment{
		makeComment(commentOpts{id: 5, parent: 999, minutes: 10, body: "orphan"}),
		makeComment(commentOpts{id: 6, parent: 5, minutes: 20, body: "reply to the orphan"}),
	}

	threads := BuildThreads(comments)

	testkit.AssertEqual(t, "threads", len(threads), 1)
	testkit.AssertEqual(t, "root", threads[0].Root.ID, int64(5))
	testkit.AssertSliceEqual(t, "replies", replyIDs(threads[0]), []int64{6})
}

func TestBuildThreads_ParentCycleDoesNotHang(t *testing.T) {
	t.Parallel()

	// Nothing in the API should produce this, but the chain comes off the wire.
	comments := []Comment{
		makeComment(commentOpts{id: 1, parent: 2, minutes: 0, body: "a"}),
		makeComment(commentOpts{id: 2, parent: 1, minutes: 5, body: "b"}),
	}

	threads := BuildThreads(comments)

	if len(threads) == 0 {
		t.Fatal("a cyclic chain lost every comment")
	}
}

func TestBuildThreads_DeletedComments(t *testing.T) {
	t.Parallel()

	comments := []Comment{
		makeComment(commentOpts{id: 1, minutes: 0, deleted: true, body: "deleted root with a reply"}),
		makeComment(commentOpts{id: 2, parent: 1, minutes: 5, body: "reply that survives"}),
		makeComment(commentOpts{id: 3, minutes: 10, deleted: true, body: "deleted root, no replies"}),
		makeComment(commentOpts{id: 4, minutes: 15, body: "live root"}),
		makeComment(commentOpts{id: 5, parent: 4, minutes: 20, deleted: true, body: "deleted childless reply"}),
	}

	threads := BuildThreads(comments)

	testkit.AssertSliceEqual(t, "threads", threadIDs(threads), []int64{1, 4})

	testkit.AssertEqual(t, "deleted root kept", threads[0].Root.Deleted, true)
	testkit.AssertEqual(t, "deleted root body cleared", threads[0].Root.Content.Raw, "")
	testkit.AssertSliceEqual(t, "replies of the deleted root", replyIDs(threads[0]), []int64{2})
	testkit.AssertEqual(t, "childless deleted reply dropped", len(threads[1].Replies), 0)
}

func TestBuildThreads_DeletedReplyWithChildrenIsKeptAsAPlaceholder(t *testing.T) {
	t.Parallel()

	comments := []Comment{
		makeComment(commentOpts{id: 1, minutes: 0, body: "root"}),
		makeComment(commentOpts{id: 2, parent: 1, minutes: 5, deleted: true, body: "deleted middle"}),
		makeComment(commentOpts{id: 3, parent: 2, minutes: 10, body: "child of the deleted reply"}),
	}

	threads := BuildThreads(comments)

	testkit.AssertEqual(t, "threads", len(threads), 1)
	testkit.AssertSliceEqual(t, "replies", replyIDs(threads[0]), []int64{2, 3})
	testkit.AssertEqual(t, "placeholder body", threads[0].Replies[0].Content.Raw, "")
	testkit.AssertEqual(t, "placeholder deleted", threads[0].Replies[0].Deleted, true)
	testkit.AssertEqual(t, "surviving child body", threads[0].Replies[1].Content.Raw,
		"child of the deleted reply")
}

func TestBuildThreads_DeletedRootWithOnlyDeletedRepliesDisappears(t *testing.T) {
	t.Parallel()

	// Keeping a deleted comment has to be transitive. A root whose only reply
	// was deleted too has nothing left to caption, and a child count taken
	// before the replies are dropped would leave an empty "(deleted)" thread
	// on the pane.
	comments := []Comment{
		makeComment(commentOpts{id: 1, minutes: 0, deleted: true, body: "deleted root"}),
		makeComment(commentOpts{id: 2, parent: 1, minutes: 5, deleted: true, body: "deleted reply"}),
	}

	testkit.AssertEqual(t, "threads", len(BuildThreads(comments)), 0)
}

func TestBuildThreads_DeletedRootWithALiveReplySurvives(t *testing.T) {
	t.Parallel()

	comments := []Comment{
		makeComment(commentOpts{id: 1, minutes: 0, deleted: true, body: "deleted root"}),
		makeComment(commentOpts{id: 2, parent: 1, minutes: 5, body: "live reply"}),
	}

	threads := BuildThreads(comments)

	testkit.AssertEqual(t, "threads", len(threads), 1)
	testkit.AssertEqual(t, "root deleted", threads[0].Root.Deleted, true)
	testkit.AssertEqual(t, "root body cleared", threads[0].Root.Content.Raw, "")
	testkit.AssertSliceEqual(t, "replies", replyIDs(threads[0]), []int64{2})
}

func TestBuildThreads_DeletedChainIsKeptOnlyDownToALiveDescendant(t *testing.T) {
	t.Parallel()

	// Two deleted comments deep, then something readable: every ancestor of a
	// live comment is worth a placeholder, and the deleted sibling chain that
	// leads nowhere is not.
	comments := []Comment{
		makeComment(commentOpts{id: 1, minutes: 0, deleted: true, body: "deleted root"}),
		makeComment(commentOpts{id: 2, parent: 1, minutes: 5, deleted: true, body: "deleted middle"}),
		makeComment(commentOpts{id: 3, parent: 2, minutes: 10, body: "live leaf"}),
		makeComment(commentOpts{id: 4, minutes: 15, deleted: true, body: "deleted root, no replies"}),
		makeComment(commentOpts{id: 5, parent: 4, minutes: 20, deleted: true, body: "deleted reply"}),
	}

	threads := BuildThreads(comments)

	testkit.AssertSliceEqual(t, "threads", threadIDs(threads), []int64{1})
	testkit.AssertSliceEqual(t, "replies", replyIDs(threads[0]), []int64{2, 3})
}

func TestBuildThreads_OrdersInlineThreadsBeforeGeneralOnes(t *testing.T) {
	t.Parallel()

	comments := []Comment{
		makeComment(commentOpts{id: 1, minutes: 0, body: "oldest general"}),
		makeComment(commentOpts{id: 2, minutes: 50, body: "newest general"}),
		makeComment(commentOpts{id: 3, minutes: 10, path: "z.go", line: 5, body: "z:5"}),
		makeComment(commentOpts{id: 4, minutes: 20, path: testDiffFilePath, line: 40, body: "app:40"}),
		makeComment(commentOpts{id: 5, minutes: 30, path: testDiffFilePath, line: 12, body: "app:12"}),
		makeComment(commentOpts{id: 6, minutes: 40, path: testDiffFilePath, fileOnly: true, body: "app file"}),
	}

	threads := BuildThreads(comments)

	// Inline threads first, grouped by path, walking down the file with the
	// file-level thread at the top; general threads last, oldest first.
	testkit.AssertSliceEqual(t, "threads", threadIDs(threads), []int64{6, 5, 4, 3, 1, 2})
}

func TestBuildThreads_TiesOnPathAndLineOrderByCreation(t *testing.T) {
	t.Parallel()

	comments := []Comment{
		makeComment(commentOpts{id: 20, minutes: 30, path: testDiffFilePath, line: 12, body: "second"}),
		makeComment(commentOpts{id: 10, minutes: 10, path: testDiffFilePath, line: 12, body: "first"}),
	}

	threads := BuildThreads(comments)

	testkit.AssertSliceEqual(t, "threads", threadIDs(threads), []int64{10, 20})
}

func TestThread_ResolvedAndPath(t *testing.T) {
	t.Parallel()

	comments := []Comment{
		makeComment(commentOpts{
			id: 1, minutes: 0, path: testDiffFilePath, line: 383, resolved: true, body: "resolved",
		}),
	}

	threads := BuildThreads(comments)

	testkit.AssertEqual(t, "threads", len(threads), 1)
	testkit.AssertEqual(t, "Path", threads[0].Path(), testDiffFilePath)
	testkit.AssertEqual(t, "IsInline", threads[0].IsInline(), true)
	testkit.AssertEqual(t, "Resolved", threads[0].Resolved(), true)
}

func TestThread_LastActivityUsesTheNewestUpdate(t *testing.T) {
	t.Parallel()

	comments := []Comment{
		makeComment(commentOpts{id: 1, minutes: 0, updated: 90, body: "edited root"}),
		makeComment(commentOpts{id: 2, parent: 1, minutes: 10, body: "reply"}),
	}

	threads := BuildThreads(comments)

	testkit.AssertEqual(t, "LastActivity", threads[0].LastActivity(), threadBase.Add(90*time.Minute))
}

func TestCommentedLines(t *testing.T) {
	t.Parallel()

	comments := []Comment{
		makeComment(commentOpts{id: 1, minutes: 0, path: testDiffFilePath, line: 383, body: "new side"}),
		makeComment(commentOpts{id: 2, minutes: 5, path: testDiffFilePath, line: 383, body: "same line again"}),
		makeComment(commentOpts{id: 3, parent: 1, minutes: 6, path: testDiffFilePath, line: 383, body: "a reply"}),
		makeComment(commentOpts{id: 4, minutes: 10, path: testDiffFilePath, oldLine: 12, body: "old side"}),
		makeComment(commentOpts{id: 5, minutes: 15, path: "README.md", fileOnly: true, body: "file level"}),
		makeComment(commentOpts{id: 6, minutes: 20, body: "general, no file"}),
	}

	lines := CommentedLines(BuildThreads(comments))

	testkit.AssertEqual(t, "files", len(lines), 2)
	testkit.AssertEqual(t, "app.go lines", len(lines[testDiffFilePath]), 2)
	// Two threads on the new-side line 383 — the reply does not count again.
	testkit.AssertEqual(t, "new-side count", lines[testDiffFilePath][383], 2)
	// The old-side anchor is reported under the negated line number.
	testkit.AssertEqual(t, "old-side count", lines[testDiffFilePath][-12], 1)
	testkit.AssertEqual(t, "file-level count", lines["README.md"][0], 1)
	testkit.AssertEqual(t, "general threads are not files", len(lines[""]), 0)
}

func TestCommentedLines_MarksEveryLineOfAMultiLineRange(t *testing.T) {
	t.Parallel()

	// ThreadAt answers for any line inside Inline.Range(), so the marks have to
	// cover the same lines: marking only the anchor left the other lines of a
	// range unmarked while Enter on them still jumped to the thread.
	newSide := makeComment(commentOpts{id: 1, minutes: 0, path: testDiffFilePath, line: 15, body: "new range"})
	startTo := 10
	newSide.Inline.StartTo = &startTo

	oldSide := makeComment(commentOpts{id: 2, minutes: 5, path: testDiffFilePath, oldLine: 8, body: "old range"})
	startFrom := 6
	oldSide.Inline.StartFrom = &startFrom

	lines := CommentedLines(BuildThreads([]Comment{newSide, oldSide}))[testDiffFilePath]

	testkit.AssertEqual(t, "marked lines", len(lines), 9)
	for line := 10; line <= 15; line++ {
		testkit.AssertEqual(t, "new-side line "+strconv.Itoa(line), lines[line], 1)
	}
	// The old side is reported under the negated line number, range included.
	for line := 6; line <= 8; line++ {
		testkit.AssertEqual(t, "old-side line -"+strconv.Itoa(line), lines[-line], 1)
	}
	testkit.AssertEqual(t, "outside the new range", lines[9], 0)
	testkit.AssertEqual(t, "outside the new range", lines[16], 0)
	testkit.AssertEqual(t, "outside the old range", lines[-5], 0)
	testkit.AssertEqual(t, "old range is not marked on the new side", lines[6], 0)
}

func TestCommentedLines_NoInlineThreads(t *testing.T) {
	t.Parallel()

	threads := BuildThreads([]Comment{makeComment(commentOpts{id: 1, minutes: 0, body: "general"})})

	if lines := CommentedLines(threads); lines != nil {
		t.Errorf("CommentedLines = %v, want nil", lines)
	}
}

func TestCommentTotal(t *testing.T) {
	t.Parallel()

	pending := makeComment(commentOpts{id: 5, minutes: 25, body: "own draft"})
	pending.Pending = true

	tests := []struct {
		name     string
		comments []Comment
		want     int
	}{
		{"no threads", nil, 0},
		{
			"a root and its replies",
			[]Comment{
				makeComment(commentOpts{id: 1, minutes: 0, body: "root"}),
				makeComment(commentOpts{id: 2, parent: 1, minutes: 5, body: "reply"}),
				makeComment(commentOpts{id: 3, parent: 1, minutes: 10, body: "another reply"}),
			},
			3,
		},
		{
			// The placeholder is a caption for the reply below it, not a
			// comment anybody wrote any more.
			"a deleted placeholder root does not count",
			[]Comment{
				makeComment(commentOpts{id: 1, minutes: 0, deleted: true, body: "deleted root"}),
				makeComment(commentOpts{id: 2, parent: 1, minutes: 5, body: "live reply"}),
			},
			1,
		},
		{
			"a deleted placeholder reply does not count either",
			[]Comment{
				makeComment(commentOpts{id: 1, minutes: 0, body: "root"}),
				makeComment(commentOpts{id: 2, parent: 1, minutes: 5, deleted: true, body: "deleted middle"}),
				makeComment(commentOpts{id: 3, parent: 2, minutes: 10, body: "child"}),
			},
			2,
		},
		{
			// A pending comment is the reader's own unpublished draft. It is
			// on the pane, so it is in the count.
			"a pending comment counts",
			[]Comment{
				makeComment(commentOpts{id: 4, minutes: 20, body: "published"}),
				pending,
			},
			2,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			testkit.AssertEqual(t, "total", CommentTotal(BuildThreads(tc.comments)), tc.want)
		})
	}
}

func TestThreadAt(t *testing.T) {
	t.Parallel()

	spanning := makeComment(commentOpts{id: 7, minutes: 30, path: testDiffFilePath, line: 400, body: "range"})
	start := 396
	spanning.Inline.StartTo = &start

	threads := BuildThreads([]Comment{
		makeComment(commentOpts{id: 1, minutes: 0, path: testDiffFilePath, line: 383, body: "new side"}),
		makeComment(commentOpts{id: 2, minutes: 5, path: testDiffFilePath, line: 383, body: "same line again"}),
		makeComment(commentOpts{id: 4, minutes: 10, path: testDiffFilePath, oldLine: 12, body: "old side"}),
		makeComment(commentOpts{id: 5, minutes: 15, path: "README.md", fileOnly: true, body: "file level"}),
		makeComment(commentOpts{id: 6, minutes: 20, body: "general, no file"}),
		spanning,
	})

	tests := []struct {
		name    string
		path    string
		line    int
		newSide bool
		want    int64
		found   bool
	}{
		{"new side anchor", testDiffFilePath, 383, true, 1, true},
		{"old side anchor", testDiffFilePath, 12, false, 4, true},
		{"old side line is not a new side line", testDiffFilePath, 12, true, 0, false},
		{"file level", "README.md", 0, true, 5, true},
		{"multi-line range covers the line", testDiffFilePath, 398, true, 7, true},
		{"range end", testDiffFilePath, 400, true, 7, true},
		{"outside the range", testDiffFilePath, 395, true, 0, false},
		{"unknown file", "nope.go", 383, true, 0, false},
		{"no path", "", 383, true, 0, false},
		{"general threads are unreachable", testDiffFilePath, 0, true, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			thread, ok := ThreadAt(threads, tc.path, tc.line, tc.newSide)
			testkit.AssertEqual(t, "found", ok, tc.found)
			testkit.AssertEqual(t, "root id", thread.Root.ID, tc.want)
		})
	}
}

func TestThreadAt_EmptyThreadList(t *testing.T) {
	t.Parallel()

	if _, ok := ThreadAt(nil, testDiffFilePath, 1, true); ok {
		t.Error("ThreadAt found a thread in an empty list")
	}
}

func TestAppendComment_AttachesAReplyToItsThread(t *testing.T) {
	t.Parallel()

	threads := BuildThreads([]Comment{
		makeComment(commentOpts{id: 1, minutes: 0, body: "is one token enough?", path: "a.go", line: 3}),
		makeComment(commentOpts{id: 2, parent: 1, minutes: 5, body: "we rotate weekly"}),
		makeComment(commentOpts{id: 7, minutes: 2, body: "general"}),
	})

	updated := AppendComment(threads, makeComment(commentOpts{
		id: 9, parent: 1, minutes: 9, body: "a window is still a window",
	}))

	if len(updated) != 2 {
		t.Fatalf("threads = %d, want the same 2", len(updated))
	}
	testkit.AssertEqual(t, "root", updated[0].Root.ID, int64(1))
	if len(updated[0].Replies) != 2 {
		t.Fatalf("replies = %d, want 2", len(updated[0].Replies))
	}
	// The reply lands in written order, which is BuildThreads' rule rather than
	// a second one here.
	testkit.AssertEqual(t, "last reply", updated[0].Replies[1].ID, int64(9))
	testkit.AssertEqual(t, "the other thread is untouched", updated[1].Root.ID, int64(7))
}

func TestAppendComment_StartsAThreadForANewRoot(t *testing.T) {
	t.Parallel()

	threads := BuildThreads([]Comment{
		makeComment(commentOpts{id: 1, minutes: 0, body: "first", path: "a.go", line: 3}),
	})

	updated := AppendComment(threads, makeComment(commentOpts{id: 5, minutes: 4, body: "a general remark"}))

	if len(updated) != 2 {
		t.Fatalf("threads = %d, want 2", len(updated))
	}
	// An inline thread still sorts ahead of a general one, whenever it arrived.
	testkit.AssertEqual(t, "inline first", updated[0].Root.ID, int64(1))
	testkit.AssertEqual(t, "the new general thread", updated[1].Root.ID, int64(5))
}

func TestAppendComment_ReplacesACommentItAlreadyHolds(t *testing.T) {
	t.Parallel()

	threads := BuildThreads([]Comment{
		makeComment(commentOpts{id: 1, minutes: 0, body: "first"}),
		makeComment(commentOpts{id: 2, parent: 1, minutes: 1, body: "draft text"}),
	})

	edited := makeComment(commentOpts{id: 2, parent: 1, minutes: 1, body: "final text"})
	updated := AppendComment(AppendComment(threads, edited), edited)

	if len(updated) != 1 {
		t.Fatalf("threads = %d, want 1", len(updated))
	}
	if len(updated[0].Replies) != 1 {
		t.Fatalf("replies = %d, want 1 — applying the same answer twice must not duplicate it",
			len(updated[0].Replies))
	}
	testkit.AssertEqual(t, "body", updated[0].Replies[0].Content.Raw, "final text")
}

func TestAppendComment_BuildsAThreadOutOfNothing(t *testing.T) {
	t.Parallel()

	updated := AppendComment(nil, makeComment(commentOpts{id: 3, minutes: 0, body: "the first word"}))

	if len(updated) != 1 {
		t.Fatalf("threads = %d, want 1", len(updated))
	}
	testkit.AssertEqual(t, "root", updated[0].Root.ID, int64(3))
}
