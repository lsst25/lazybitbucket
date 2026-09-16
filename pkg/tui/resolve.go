package tui

import (
	"strconv"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// Resolving is a property of a conversation, not of a comment: Bitbucket hangs
// it off the comment that starts the thread, so T acts on whichever thread is
// in hand — the one the section is showing, or the one under the cursor on the
// Comments tab — whatever row of it the cursor happens to be on.
//
// Like A, it is one key in two directions and the dialog says which: a resolved
// conversation is reopened, an open one resolved. Like every other write it is
// refused in read-only mode, and like commenting it is not refused on a merged
// pull request, because a conversation outlives the branch.

// The two directions, named for the help bar.
const (
	verbResolve = "resolve"
	verbReopen  = "reopen"
)

// formResolve is the confirmation dialog's id.
const formResolve = "resolve"

// noThreadHere is what T answers where no conversation is in hand.
const noThreadHere = "no conversation here: open the Comments tab"

// commentResolvedMsg is a resolve or a reopen, finished. The resolution is what
// the server recorded, nil for a reopen and for an answer that carried none.
type commentResolvedMsg struct {
	key        string
	rootID     int64
	anchor     string
	reopen     bool
	resolution *bitbucket.CommentResolution
	err        error
}

// handleActionResolve opens the confirmation for the conversation in hand.
func (a *App) handleActionResolve() tea.Cmd {
	pr, ok := a.writeTargetInState(verbResolve, false)
	if !ok {
		return nil
	}
	thread := a.detailView.SelectedOrOpenThread()
	if thread == nil {
		a.helpBar.SetStatusMsg(noThreadHere)
		return nil
	}

	reopen := thread.Resolved()
	anchor := threadAnchorText(*thread)
	title := "Resolve " + anchor + "?"
	if reopen {
		title = "Reopen " + anchor + "?"
	}

	key, rootID := pr.Key(), thread.Root.ID
	a.armForm(formResolve, func(components.FormConfirmedMsg) tea.Cmd {
		return a.setResolved(key, rootID, anchor, reopen)
	})
	a.form.SetSize(a.width, a.height)
	a.form.Show(formResolve, title, "confirm", []components.FormField{{
		Kind: components.FormNote, Note: threadSummary(*thread),
		ID: "", Label: "", Options: nil, Index: 0, On: false, Value: "",
	}})
	return nil
}

// threadAnchorText is where a conversation hangs, for a dialog title and a help
// bar line: "src/auth.go:13", or "the pull request" for a general thread.
func threadAnchorText(thread bitbucket.Thread) string {
	if !thread.IsInline() {
		return "the conversation on the pull request"
	}
	if line, _, ok := thread.Root.Inline.Line(); ok {
		return thread.Path() + ":" + strconv.Itoa(line)
	}
	return thread.Path()
}

// threadSummary is the line the dialog shows under its question: who started
// the conversation, and how many replies it has.
func threadSummary(thread bitbucket.Thread) string {
	summary := thread.Root.User.Name()
	if summary == "" {
		summary = "a comment"
	}
	if replies := len(thread.Replies); replies > 0 {
		summary += " · " + strconv.Itoa(replies) + " " + pluralReplies(replies)
	}
	return summary
}

// pluralReplies picks the noun for n.
func pluralReplies(n int) string {
	if n == 1 {
		return "reply"
	}
	return "replies"
}

// setResolved sends the confirmed resolve or reopen.
func (a *App) setResolved(key string, rootID int64, anchor string, reopen bool) tea.Cmd {
	pr := a.pullRequestByKey(key)
	if pr == nil {
		return nil
	}
	verb := verbResolve
	if reopen {
		verb = verbReopen
	}
	if running, busy := a.writesInFlight[key]; busy {
		a.helpBar.SetStatusMsg(running + " already in progress")
		return nil
	}

	ctx, client := a.context(), a.client
	workspace, slug, id := pr.Workspace(), pr.RepoSlug(), pr.ID

	a.beginWrite(key, verb)
	a.helpBar.SetStatusMsg(verb + "ing " + anchor + "…")

	return func() tea.Msg {
		done := commentResolvedMsg{
			key: key, rootID: rootID, anchor: anchor, reopen: reopen,
			resolution: nil, err: nil,
		}
		if reopen {
			done.err = client.ReopenComment(ctx, workspace, slug, id, rootID)
			return done
		}
		done.resolution, done.err = client.ResolveComment(ctx, workspace, slug, id, rootID)
		return done
	}
}

// handleCommentResolved applies what the write left behind. The thread is
// patched where it is held rather than refetched, the way a written comment is.
func (a *App) handleCommentResolved(msg commentResolvedMsg) (tea.Model, tea.Cmd) {
	a.finishWrite(msg.key)
	a.logFlag.Store(false)

	verb := verbResolve
	if msg.reopen {
		verb = verbReopen
	}
	if msg.err != nil {
		a.helpBar.SetStatusMsg(verb + " failed: " + a.writeErrorText(verb, msg.err))
		return a, nil
	}
	a.statusPanel.SetOnline(true)

	a.ensureReviewCaches()
	entry, cached := a.threadsCache[msg.key]
	if cached && entry.err == nil {
		threads := patchResolution(entry.value, msg.rootID, a.resolutionFor(msg))
		a.threadsCache[msg.key] = reviewEntry[[]bitbucket.Thread]{
			value: threads, sourceHash: entry.sourceHash, err: nil,
		}
		a.detailView.SetThreads(msg.key, threads)
	}
	a.helpBar.SetStatusMsg(resolvedText(msg.reopen) + " " + msg.anchor)
	return a, nil
}

// resolutionFor is what to record on the thread: what the server answered with,
// or — for an answer that carried none — who pressed the key and when, which is
// what the server recorded whether or not it said so. A reopen records nothing.
func (a *App) resolutionFor(msg commentResolvedMsg) *bitbucket.CommentResolution {
	switch {
	case msg.reopen:
		return nil
	case msg.resolution != nil:
		return msg.resolution
	default:
		resolution := &bitbucket.CommentResolution{
			Type: "comment_resolution", User: nil, CreatedOn: a.nowFn(),
		}
		if a.currentUser != nil {
			user := *a.currentUser
			resolution.User = &user
		}
		return resolution
	}
}

// patchResolution puts a resolution — or nothing, for a reopen — on the thread
// rooted at rootID, leaving every other thread as it was.
func patchResolution(
	threads []bitbucket.Thread, rootID int64, resolution *bitbucket.CommentResolution,
) []bitbucket.Thread {
	patched := make([]bitbucket.Thread, len(threads))
	copy(patched, threads)
	for i := range patched {
		if patched[i].Root.ID == rootID {
			patched[i].Root.Resolution = resolution
			break
		}
	}
	return patched
}

// resolvedText is what the help bar says a write left behind.
func resolvedText(reopen bool) string {
	if reopen {
		return "reopened"
	}
	return "resolved"
}

// handleActionHideResolved flips the Comments tab's resolved-thread filter and
// says what it did: the tab label carries the count, but a reader who pressed a
// key deserves an answer in the place they are looking.
func (a *App) handleActionHideResolved() tea.Cmd {
	if a.detailView.Mode() != views.ModeThread && !a.reviewTabOnScreen(
		a.detailView.PullRequestKey(), views.TabComments) {
		return nil
	}
	if a.detailView.ToggleResolvedThreads() {
		a.helpBar.SetStatusMsg("hiding resolved conversations")
		return nil
	}
	a.helpBar.SetStatusMsg("showing every conversation")
	return nil
}
