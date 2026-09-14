package bitbucket

import (
	"cmp"
	"slices"
	"strings"
	"time"
)

// maxParentDepth bounds the walk up a reply's Parent chain. Bitbucket's own
// threads are one level deep, but the chain comes off the wire and a cycle
// there must not hang the TUI.
const maxParentDepth = 64

// fileLevelLine is the CommentedLines key for a thread anchored to a file but
// to no line, and the sort position of such a thread within its file.
const fileLevelLine = 0

// Thread is a root comment with its replies, built client-side from the flat
// comment list the API returns.
type Thread struct {
	Root Comment
	// Replies is sorted by (CreatedOn, ID). Bitbucket's threads are one level
	// deep, but a deeper chain is flattened into this same order rather than
	// nested, so the view never has to recurse.
	Replies []Comment
}

// Path is the file the thread is anchored to, "" for a general thread.
func (t Thread) Path() string {
	if t.Root.Inline == nil {
		return ""
	}
	return t.Root.Inline.Path
}

// IsInline reports whether the thread is anchored to a file.
func (t Thread) IsInline() bool { return t.Root.IsInline() }

// Resolved reports whether the thread has been marked resolved.
func (t Thread) Resolved() bool { return t.Root.Resolution.Resolved() }

// LastActivity is the newest UpdatedOn across the root and its replies.
func (t Thread) LastActivity() time.Time {
	latest := t.Root.UpdatedOn
	for _, reply := range t.Replies {
		if reply.UpdatedOn.After(latest) {
			latest = reply.UpdatedOn
		}
	}
	return latest
}

// BuildThreads groups a flat comment list into threads.
//
// A comment with no Parent is a thread root. A reply attaches to the thread
// holding its parent, however deep the chain runs; a reply whose parent is not
// in the list is an orphan and becomes a root of its own, so nothing is
// silently dropped when the caller only fetched part of the collection.
//
// A deleted comment is dropped unless something still readable hangs below it,
// in which case it is kept as a placeholder — Deleted true and Content.Raw
// emptied — so the view can show "(deleted)" above the replies that still make
// sense. That test is transitive: a deleted comment whose only replies were
// themselves deleted has nothing left to caption and disappears with them.
//
// The result is fully deterministic: inline threads first, ordered by path,
// then anchor line, then the root's CreatedOn and id; general threads after
// them, ordered by CreatedOn and id.
func BuildThreads(comments []Comment) []Thread {
	if len(comments) == 0 {
		return nil
	}

	byID := make(map[int64]Comment, len(comments))
	for _, comment := range comments {
		byID[comment.ID] = comment
	}

	kept := keptIDs(comments, byID)

	order := make([]int64, 0, len(comments))
	roots := make(map[int64]Comment, len(comments))
	replies := make(map[int64][]Comment, len(comments))

	for _, comment := range comments {
		survivor, ok := keepComment(comment, kept[comment.ID])
		if !ok {
			continue
		}

		rootID := threadRootID(comment, byID)
		if rootID == comment.ID {
			if _, seen := roots[comment.ID]; !seen {
				order = append(order, comment.ID)
			}
			roots[comment.ID] = survivor
			continue
		}
		replies[rootID] = append(replies[rootID], survivor)
	}

	threads := make([]Thread, 0, len(roots))
	for _, rootID := range order {
		root, ok := roots[rootID]
		if !ok {
			continue
		}
		thread := Thread{Root: root, Replies: replies[rootID]}
		if root.Deleted && len(thread.Replies) == 0 {
			// A placeholder is a caption for the replies under it. With none
			// left there is nothing for it to caption.
			continue
		}
		slices.SortStableFunc(thread.Replies, compareComments)
		threads = append(threads, thread)
	}

	slices.SortStableFunc(threads, compareThreads)
	return threads
}

// keepComment returns the form a comment survives in: itself when it is not
// deleted, a placeholder when it is deleted but kept for the sake of something
// below it, and nothing at all otherwise.
func keepComment(comment Comment, kept bool) (Comment, bool) {
	if !comment.Deleted {
		return comment, true
	}
	if !kept {
		return Comment{}, false
	}
	comment.Content.Raw = ""
	return comment, true
}

// keptIDs is the set of comments that survive into a thread: every comment that
// is not deleted, plus every ancestor of one. Keeping has to be transitive, or
// a deleted comment whose only reply was also deleted would survive as an empty
// "(deleted)" thread — the child count says it had a reply, and the reply is
// gone by the time the count is used.
func keptIDs(comments []Comment, byID map[int64]Comment) map[int64]bool {
	kept := make(map[int64]bool, len(comments))
	for _, comment := range comments {
		if comment.Deleted {
			continue
		}
		kept[comment.ID] = true
		keepAncestors(comment, byID, kept)
	}
	return kept
}

// keepAncestors walks up a comment's Parent chain and keeps every ancestor it
// passes. It stops at the top of the chain, where the chain leaves the list,
// and at maxParentDepth so a cycle cannot hang the walk.
func keepAncestors(comment Comment, byID map[int64]Comment, kept map[int64]bool) {
	current := comment
	for range maxParentDepth {
		if current.Parent == nil {
			return
		}
		parent, ok := byID[current.Parent.ID]
		if !ok || parent.ID == current.ID {
			return
		}
		kept[parent.ID] = true
		current = parent
	}
}

// threadRootID walks up the Parent chain to the comment that owns the thread.
// It returns the comment's own id when it is a root, and the id of the highest
// ancestor still present in the list when the chain leaves the list (an orphan
// reply then roots its own thread).
func threadRootID(comment Comment, byID map[int64]Comment) int64 {
	current := comment
	for range maxParentDepth {
		if current.Parent == nil {
			return current.ID
		}
		parent, ok := byID[current.Parent.ID]
		if !ok || parent.ID == current.ID {
			return current.ID
		}
		current = parent
	}
	return current.ID
}

// compareComments orders replies the way they were written: oldest first, id as
// the tie-break for two comments sharing a timestamp.
func compareComments(a, b Comment) int {
	if byTime := a.CreatedOn.Compare(b.CreatedOn); byTime != 0 {
		return byTime
	}
	return cmp.Compare(a.ID, b.ID)
}

// compareThreads orders inline threads before general ones, groups the inline
// ones by file and walks them down the file.
func compareThreads(a, b Thread) int {
	aInline, bInline := a.IsInline(), b.IsInline()
	if aInline != bInline {
		if aInline {
			return -1
		}
		return 1
	}
	if aInline {
		if byPath := strings.Compare(a.Path(), b.Path()); byPath != 0 {
			return byPath
		}
		if byLine := cmp.Compare(threadSortLine(a), threadSortLine(b)); byLine != 0 {
			return byLine
		}
	}
	return compareComments(a.Root, b.Root)
}

// threadSortLine is the line an inline thread sorts on within its file:
// the anchor line, or 0 for a file-level thread so it leads the file.
func threadSortLine(thread Thread) int {
	if thread.Root.Inline == nil {
		return fileLevelLine
	}
	line, _, ok := thread.Root.Inline.Line()
	if !ok {
		return fileLevelLine
	}
	return line
}

// CommentTotal is how many comments the threads hold — roots and replies
// alike, the deleted placeholders BuildThreads keeps for the sake of their
// replies not counted. It is the local answer to the pull request's
// comment_count.
func CommentTotal(threads []Thread) int {
	total := 0
	for _, thread := range threads {
		if !thread.Root.Deleted {
			total++
		}
		for _, reply := range thread.Replies {
			if !reply.Deleted {
				total++
			}
		}
	}
	return total
}

// CommentedLines maps each commented file to the lines that carry at least one
// inline thread, and how many threads each line carries. It is what lets the
// diff view mark commented lines.
//
// Keys are new-side line numbers. A thread anchored only on the old side is
// reported under the negation of its line number, so a removed line can be
// marked too, and a file-level thread (no line at all) is reported under 0.
//
// A multi-line thread counts on every line of its range, not just its anchor:
// ThreadAt answers for any line the range covers, so marking only the anchor
// would draw a marker the diff view could not explain and leave lines that jump
// to a thread unmarked.
func CommentedLines(threads []Thread) map[string]map[int]int {
	if len(threads) == 0 {
		return nil
	}

	lines := make(map[string]map[int]int)
	for _, thread := range threads {
		path := thread.Path()
		if path == "" {
			continue
		}
		if lines[path] == nil {
			lines[path] = make(map[int]int)
		}
		countCommentedLines(lines[path], thread.Root.Inline)
	}
	if len(lines) == 0 {
		return nil
	}
	return lines
}

// countCommentedLines counts one anchor into a file's line counts: every line
// of a multi-line range, the single line of an ordinary anchor, or 0 for a
// file-level anchor. New-side lines are counted under their own number and
// old-side lines under the negation, the convention CommentedLines documents.
func countCommentedLines(counts map[int]int, inline *CommentInline) {
	start, end, newSide, ok := inlineRange(inline)
	if !ok {
		counts[fileLevelLine]++
		return
	}

	for line := start; line <= end; line++ {
		if newSide {
			counts[line]++
		} else {
			counts[-line]++
		}
	}
}

// inlineRange is CommentInline.Range with the nil anchor folded in.
func inlineRange(inline *CommentInline) (int, int, bool, bool) {
	if inline == nil {
		return 0, 0, false, false
	}
	return inline.Range()
}

// ThreadAt finds the inline thread the diff view should jump to for one line of
// one file. newSide says which of the two diff sides line counts on, matching
// the flag CommentInline.Line reports; line 0 asks for the file-level thread,
// the one anchored to the path and to no line at all.
//
// Threads are searched in BuildThreads order, so the answer is the topmost
// thread on that line when several share it. A thread anchored to a single line
// wins over one whose multi-line range merely covers it.
func ThreadAt(threads []Thread, path string, line int, newSide bool) (Thread, bool) {
	if path == "" {
		return Thread{}, false
	}

	var covering Thread
	found := false
	for _, thread := range threads {
		if thread.Path() != path {
			continue
		}
		anchor, anchorSide, ok := thread.Root.Inline.Line()
		if line == fileLevelLine {
			if !ok {
				return thread, true
			}
			continue
		}
		if !ok || anchorSide != newSide {
			continue
		}
		if anchor == line {
			return thread, true
		}
		if start, end, _, spans := thread.Root.Inline.Range(); spans && line >= start && line <= end && !found {
			covering, found = thread, true
		}
	}
	return covering, found
}
