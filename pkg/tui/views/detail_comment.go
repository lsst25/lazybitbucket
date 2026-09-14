package views

import (
	"path"
	"strconv"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
)

// Writing a comment is the app's — it opens the same input popup the title
// editor does, and sends what is confirmed. What the panel owns is the one
// thing only it knows: where a comment written from here would go.
//
// That is the whole of this file. The panel is a different place at every
// moment — a conversation, a thread list, a file's diff, a changed-file list —
// and each of them anchors a comment somewhere else.

// CommentTarget is where a comment written from the panel's current position
// belongs, and how to name it to the person writing it.
type CommentTarget struct {
	// ParentID is the comment being replied to, 0 for a new conversation.
	ParentID int64
	// Anchor is the file, and the line of it, a new conversation hangs off.
	// nil is a comment on the pull request itself.
	Anchor *bitbucket.CommentAnchor
	// Title heads the popup: "Reply to Grace Hopper", "Comment on bucket.go:13",
	// "Comment on #101".
	Title string
}

// CommentTarget resolves where a comment written now would go, and reports
// false where there is nothing to write one on — so the caller can say why
// rather than open an empty box.
func (d *DetailView) CommentTarget() (CommentTarget, bool) {
	if d.pr == nil {
		return CommentTarget{}, false
	}
	switch d.mode {
	case ModeThread:
		// The conversation on screen — unless a refetch has taken it away, in
		// which case there is nothing left to reply to.
		thread := d.OpenedThread()
		if thread == nil {
			return CommentTarget{}, false
		}
		return replyTarget(*thread), true
	case ModeFileDiff:
		return d.diffCommentTarget()
	case ModePullRequest:
		return d.tabCommentTarget()
	case ModePipeline, ModeStepLog, ModeRepo, ModeSplash:
		return CommentTarget{}, false
	default:
		return CommentTarget{}, false
	}
}

// tabCommentTarget is CommentTarget for the four pull request tabs.
func (d *DetailView) tabCommentTarget() (CommentTarget, bool) {
	switch d.activeTab {
	case TabComments:
		// A thread under the cursor is answered; a second conversation on a
		// commented line is started from the diff, not from here.
		if thread := d.SelectedThread(); thread != nil {
			return replyTarget(*thread), true
		}
		// An empty Comments tab: the first comment of a pull request is a
		// comment on the pull request.
		return d.pullRequestCommentTarget(), true
	case TabOverview:
		return d.pullRequestCommentTarget(), true
	case TabFiles:
		// A file with no line: what Bitbucket shows above the first hunk.
		file := d.SelectedFile()
		if file == nil {
			return CommentTarget{}, false
		}
		return anchorTarget(bitbucket.CommentAnchor{
			Path: file.Path(), Line: 0, OldSide: false,
		}), true
	case TabCommits:
		return CommentTarget{}, false
	default:
		return CommentTarget{}, false
	}
}

// diffCommentTarget is where a comment written in the file diff hangs: the line
// under the cursor, on the side that line lives on. A row with no line of its
// own — a hunk header, the "\ No newline" marker — anchors to the file, which
// is the other shape Bitbucket takes and the one the reader is looking at
// anyway.
func (d *DetailView) diffCommentTarget() (CommentTarget, bool) {
	if d.fileDiff.path == "" {
		return CommentTarget{}, false
	}
	anchor := bitbucket.CommentAnchor{Path: d.fileDiff.path, Line: 0, OldSide: false}
	if row, ok := d.SelectedDiffLine(); ok {
		if line, newSide, anchored := diffRowAnchor(row); anchored {
			anchor.Line, anchor.OldSide = line, !newSide
		}
	}
	return anchorTarget(anchor), true
}

// pullRequestCommentTarget is a comment on the pull request itself.
func (d *DetailView) pullRequestCommentTarget() CommentTarget {
	return CommentTarget{
		ParentID: 0, Anchor: nil,
		Title: "Comment on #" + strconv.Itoa(d.pr.ID),
	}
}

// replyTarget is an answer to a conversation, named after who started it.
func replyTarget(thread bitbucket.Thread) CommentTarget {
	title := "Reply"
	if name := thread.Root.User.Name(); name != "" {
		title += " to " + name
	}
	return CommentTarget{ParentID: thread.Root.ID, Anchor: nil, Title: title}
}

// anchorTarget is a new conversation in the diff, named by the file and line it
// hangs off — by base name, because the popup's title is one line and the path
// of a deeply nested file would push the line number off the end of it.
func anchorTarget(anchor bitbucket.CommentAnchor) CommentTarget {
	title := "Comment on " + path.Base(anchor.Path)
	if anchor.Line > 0 {
		title += ":" + strconv.Itoa(anchor.Line)
	}
	return CommentTarget{ParentID: 0, Anchor: &anchor, Title: title}
}
