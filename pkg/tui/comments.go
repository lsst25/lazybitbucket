package tui

import (
	"errors"
	"os"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// Writing a comment is the seventh write action, and it asks the way the title
// editor asks: C opens the same InputModal, titled with what the comment will
// answer or hang off, and what is confirmed there is sent. The guards are every
// other write's — nothing selected, read-only mode, another overlay, another
// write in flight — with one deliberate difference: a comment is not refused on
// a merged or declined pull request, because a conversation outlives the branch.
//
// Where the comment lands is the detail panel's to decide (views.CommentTarget)
// and is held here while the popup is open: a reply carries the comment it
// answers, an inline comment the file and line it hangs off, a comment on the
// pull request neither.
//
// What comes back is applied optimistically. Bitbucket answers the POST with
// the created comment, rendering included, so the thread it belongs to is
// rebuilt from the cache the Comments tab is already showing rather than
// refetched. The next natural refresh reconciles anything else that moved.

// verbComment is the comment write's name on the help bar. Like the other
// verbs it appears in the refusal, the in-flight guard and the failure line.
const verbComment = "comment"

// commentEditorHint is what the help bar says while the popup is open, so the
// way out to a real editor is not a secret.
const commentEditorHint = "enter to send · ctrl+e for the editor · esc to cancel"

// pendingComment is the comment the open popup is writing: which pull request
// it is for and where in it the comment goes, so a failure — or the hand-off to
// an editor — can ask exactly the same question again.
type pendingComment struct {
	key    string
	target views.CommentTarget
}

// commentPostedMsg carries a written comment back. The target and the text are
// echoed so a failure can ask again with what was typed, and tmpPath names the
// file an editor-composed comment is still sitting in.
type commentPostedMsg struct {
	key     string
	target  views.CommentTarget
	text    string
	tmpPath string
	comment *bitbucket.Comment
	err     error
}

// commentEditedMsg is the editor exiting on a comment composed outside the app.
type commentEditedMsg struct {
	key    string
	target views.CommentTarget
	path   string
	err    error
}

// handleActionComment opens the popup on whatever the panel is showing. It is
// the C key's whole job: nothing is written until the popup is confirmed.
func (a *App) handleActionComment() tea.Cmd {
	pr, ok := a.commentTarget()
	if !ok {
		return nil
	}
	target, ok := a.detailView.CommentTarget()
	if !ok {
		a.helpBar.SetStatusMsg("nothing to comment on here (#" + strconv.Itoa(pr.ID) + ")")
		return nil
	}
	a.askForComment(pr.Key(), target, "")
	return nil
}

// askForComment opens the input popup on a target, prefilled with text — empty
// for a new comment, and what was typed when a failed post asks again.
func (a *App) askForComment(key string, target views.CommentTarget, text string) {
	a.pendingComment = &pendingComment{key: key, target: target}
	a.onInput = func(confirmed string) tea.Cmd {
		a.pendingComment = nil
		return a.postComment(commentWrite{key: key, target: target, text: confirmed, tmpPath: ""})
	}
	a.input.SetSize(a.width, a.height)
	a.input.Show(target.Title, text)
	a.helpBar.SetStatusMsg(commentEditorHint)
}

// commentTarget is writeTarget for the one write a closed pull request still
// takes.
func (a *App) commentTarget() (*bitbucket.PullRequest, bool) {
	return a.writeTargetInState(verbComment, false)
}

// commentInputKey answers ctrl+e inside the comment popup: the way out to a
// real editor for a comment that wants paragraphs. It runs before the overlay
// stack sees the key, because the input modal reads ctrl+e as "end of line" —
// which is what it has to keep meaning for the title editor.
func (a *App) commentInputKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if msg.Type != tea.KeyCtrlE || a.pendingComment == nil || !a.input.IsVisible() {
		return nil, false
	}
	pending := a.pendingComment
	text := a.input.Value()

	a.input.Hide()
	a.clearOverlayCallbacks()

	_, cmd := a.composeCommentInEditor(pending.key, pending.target, text)
	return cmd, true
}

// commentWrite is one comment on its way to Bitbucket: what to say, where it
// belongs, and the editor file it is still recoverable from.
type commentWrite struct {
	key    string
	target views.CommentTarget
	text   string
	// tmpPath is the file an editor-composed comment was written in, kept until
	// the post succeeds so a failure never costs the text. It is empty for one
	// typed in the popup, which is asked again instead.
	tmpPath string
}

// postComment writes one comment.
func (a *App) postComment(write commentWrite) tea.Cmd {
	key, target, text, tmpPath := write.key, write.target, write.text, write.tmpPath
	pr := a.pullRequestByKey(key)
	if pr == nil || strings.TrimSpace(text) == "" {
		a.discardCommentTempFile(tmpPath)
		return nil
	}
	if a.readOnly {
		a.helpBar.SetStatusMsg("read-only mode: cannot " + verbComment)
		a.discardCommentTempFile(tmpPath)
		return nil
	}
	if running, busy := a.writesInFlight[key]; busy {
		a.helpBar.SetStatusMsg(running + " already in progress")
		return nil
	}

	ctx, client := a.context(), a.client
	workspace, slug, id := pr.Workspace(), pr.RepoSlug(), pr.ID
	create := bitbucket.CommentCreate{
		Raw: text, ParentID: target.ParentID, Anchor: target.Anchor,
	}

	a.beginWrite(key, verbComment)
	a.helpBar.SetStatusMsg("posting a comment on #" + strconv.Itoa(id) + "…")

	return func() tea.Msg {
		comment, err := client.CreateComment(ctx, workspace, slug, id, create)
		return commentPostedMsg{
			key: key, target: target, text: text, tmpPath: tmpPath,
			comment: comment, err: err,
		}
	}
}

// handleCommentPosted folds the written comment into the conversation it
// belongs to. The comment the server answered with is the authoritative one —
// its id, its timestamps and the html the mentions are read out of are all the
// server's — so the thread is rebuilt from it rather than from what was typed.
func (a *App) handleCommentPosted(msg commentPostedMsg) (tea.Model, tea.Cmd) {
	a.finishWrite(msg.key)
	a.logFlag.Store(false)

	if msg.err != nil || msg.comment == nil {
		return a, a.noteCommentFailure(msg)
	}
	a.discardCommentTempFile(msg.tmpPath)
	a.statusPanel.SetOnline(true)

	a.ensureReviewCaches()
	entry, cached := a.threadsCache[msg.key]
	if !cached || entry.err != nil {
		// Nothing to fold it into: the Comments tab has not been read on this
		// pull request, so the comment is on the server and the next entry to
		// the tab fetches it with everything else.
		a.helpBar.SetStatusMsg(commentDoneText(msg.target))
		return a, nil
	}

	threads := bitbucket.AppendComment(entry.value, *msg.comment)
	a.threadsCache[msg.key] = reviewEntry[[]bitbucket.Thread]{
		value: threads, sourceHash: entry.sourceHash, err: nil,
	}
	a.detailView.SetThreads(msg.key, threads)
	a.applyFileDiffMarkers(msg.key)

	// A new conversation on the pull request is worth showing: the reader has
	// just written the first word of it and nothing on screen names it yet. A
	// reply lands in the thread that is already open, and an inline comment
	// lights up the marker on the line the cursor is still sitting on — being
	// thrown out of the diff halfway through reading it would be the opposite
	// of helpful.
	if msg.target.ParentID == 0 && msg.target.Anchor == nil &&
		a.detailView.PullRequestKey() == msg.key {
		a.detailView.FocusThread(msg.comment.ID)
	}
	a.helpBar.SetStatusMsg(commentDoneText(msg.target))
	return a, nil
}

// commentDoneText is the line a written comment leaves behind. An inline one
// names where it landed: the reader stays in the diff, where a new marker on
// one line is easy to miss.
func commentDoneText(target views.CommentTarget) string {
	if target.ParentID != 0 {
		return "reply posted"
	}
	if where := commentAnchorText(target.Anchor); where != "" {
		return "comment posted on " + where
	}
	return "comment posted"
}

// commentAnchorText names an anchor for a message, "" when there is none.
func commentAnchorText(anchor *bitbucket.CommentAnchor) string {
	switch {
	case anchor == nil:
		return ""
	case anchor.Line > 0:
		return anchor.Path + ":" + strconv.Itoa(anchor.Line)
	default:
		return anchor.Path
	}
}

// noteCommentFailure says what went wrong and gives the text back. A comment
// typed in the popup is asked again, prefilled with it; one composed in the
// editor stays in its file, and the message says where, because a paragraph of
// Markdown does not belong in a one-line input.
func (a *App) noteCommentFailure(msg commentPostedMsg) tea.Cmd {
	reason := verbComment + " failed: " + a.writeErrorText(verbComment, msg.err)
	if msg.err == nil {
		reason = verbComment + " failed: the server returned no comment"
	}

	if msg.tmpPath != "" {
		a.helpBar.SetStatusMsg(reason + " — your text is in " + msg.tmpPath)
		return nil
	}
	// Asking again is only possible if nothing else has taken the screen in the
	// meantime; a dialog the user opened while the post was in flight is not
	// something to shove aside.
	if !a.overlayVisible() {
		a.askForComment(msg.key, msg.target, msg.text)
	}
	a.helpBar.SetStatusMsg(reason)
	return nil
}

// composeCommentInEditor takes the text out to the configured editor. It is the
// same hand-off the description editor makes: a temp file, tea.ExecProcess, and
// the file read back when the editor exits.
func (a *App) composeCommentInEditor(
	key string, target views.CommentTarget, text string,
) (tea.Model, tea.Cmd) {
	pr := a.pullRequestByKey(key)
	if pr == nil {
		return a, nil
	}

	path, err := writeCommentTempFile(pr.RepoSlug(), pr.ID, text)
	if err != nil {
		a.statusPanel.SetError(verbComment + ": " + err.Error())
		a.askForComment(key, target, text)
		return a, nil
	}
	cmd, err := a.buildTempFileEditorCmd(path)
	if err != nil {
		_ = os.Remove(path)
		a.statusPanel.SetError(err.Error())
		a.askForComment(key, target, text)
		return a, nil
	}

	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return a, tea.ExecProcess(cmd, func(execErr error) tea.Msg {
		return commentEditedMsg{key: key, target: target, path: path, err: execErr}
	})
}

// handleCommentEdited posts what the editor left behind. An empty file is the
// way to change one's mind, exactly as it is for a commit message.
func (a *App) handleCommentEdited(msg commentEditedMsg) (tea.Model, tea.Cmd) {
	// Mouse reporting is off until this returns: the child terminal turned it
	// off and only the app can turn it back on.
	cmds := []tea.Cmd{tea.EnableMouseCellMotion}

	if msg.err != nil {
		a.statusPanel.SetError("editor: " + msg.err.Error())
		a.helpBar.SetStatusMsg("your text is in " + msg.path)
		return a, tea.Batch(cmds...)
	}

	body, err := os.ReadFile(msg.path) //nolint:gosec // the app wrote this path itself
	if err != nil {
		a.statusPanel.SetError(verbComment + ": " + err.Error())
		a.helpBar.SetStatusMsg("your text is in " + msg.path)
		return a, tea.Batch(cmds...)
	}
	if strings.TrimSpace(string(body)) == "" {
		a.discardCommentTempFile(msg.path)
		a.helpBar.SetStatusMsg(verbComment + " discarded")
		return a, tea.Batch(cmds...)
	}

	post := a.postComment(commentWrite{
		key: msg.key, target: msg.target, text: string(body), tmpPath: msg.path,
	})
	if post != nil {
		cmds = append(cmds, post)
	}
	return a, tea.Batch(cmds...)
}

// writeCommentTempFile puts a comment somewhere an editor can open it. The name
// carries the repository and the id, so a file left behind by a failed post
// says what it was.
func writeCommentTempFile(slug string, id int, body string) (string, error) {
	file, err := os.CreateTemp("", "lazybitbucket-"+slug+"-"+strconv.Itoa(id)+"-comment-*.md")
	if err != nil {
		return "", err
	}
	path := file.Name()
	_, writeErr := file.WriteString(body)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(path)
		return "", errors.Join(writeErr, closeErr)
	}
	return path, nil
}

// discardCommentTempFile removes a temp file once its text is safely on
// Bitbucket. It is deliberately not called on a failure.
func (a *App) discardCommentTempFile(path string) {
	if path == "" {
		return
	}
	_ = os.Remove(path)
}

// commentHelpBarItems is the popup's own help strip: the title editor's two
// keys, plus the editor hand-off this popup adds.
func commentHelpBarItems() []components.HelpItem {
	return []components.HelpItem{
		{Key: "enter", Description: "send"},
		{Key: "ctrl+e", Description: "editor"},
		{Key: "esc", Description: "cancel"},
	}
}
