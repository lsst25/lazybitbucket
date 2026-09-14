package tui

import (
	"errors"
	"os"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// Writing a comment is the seventh write action and the only one without a
// dialog: C opens the composer docked in the detail panel (views/composer), and
// what the composer sends is the confirmation. The guards are the same as every
// other write's — nothing selected, read-only mode, another overlay, another
// write in flight — with one deliberate difference: a comment is not refused on
// a merged or declined pull request, because a conversation outlives the branch.
//
// What comes back is applied optimistically. Bitbucket answers the POST with
// the created comment, rendering included, so the thread it belongs to is
// rebuilt from the cache the Comments tab is already showing rather than
// refetched. The next natural refresh reconciles anything else that moved.

// verbComment is the comment write's name on the help bar. Like the other
// verbs it appears in the refusal, the in-flight guard and the failure line.
const verbComment = "comment"

// commentEditorHint is what the help bar says while the composer is open, so
// the three ways out are not only under the input.
const commentEditorHint = "enter to send · ctrl+e for the editor · esc to cancel"

// commentPostedMsg carries a written comment back. parentID and text are echoed
// so a failure can hand the reply back to the composer exactly as it was typed,
// and tmpPath names the file an editor-composed reply is still sitting in.
type commentPostedMsg struct {
	key      string
	parentID int64
	text     string
	tmpPath  string
	comment  *bitbucket.Comment
	err      error
}

// commentEditedMsg is the editor exiting on a reply composed outside the app.
type commentEditedMsg struct {
	key      string
	parentID int64
	path     string
	err      error
}

// handleActionComment opens the composer on whatever the panel is showing. It
// is the C key's whole job: nothing is written until the composer says so.
func (a *App) handleActionComment() tea.Cmd {
	pr, ok := a.commentTarget()
	if !ok {
		return nil
	}

	// The composer lives in the detail panel, so the panel has to be the thing
	// with the keyboard before there is anywhere to type.
	var cmd tea.Cmd
	if a.side != sideRight {
		cmd = a.focusDetail()
	}
	if !a.detailView.BeginComment() {
		a.helpBar.SetStatusMsg("open the Overview or Comments tab to comment on #" + strconv.Itoa(pr.ID))
		return cmd
	}
	a.helpBar.SetStatusMsg(commentEditorHint)
	return cmd
}

// commentTarget is writeTarget for the one write a closed pull request still
// takes.
func (a *App) commentTarget() (*bitbucket.PullRequest, bool) {
	return a.writeTargetInState(verbComment, false)
}

// handleCommentSubmitted posts what the composer sent.
func (a *App) handleCommentSubmitted(msg views.CommentSubmittedMsg) (tea.Model, tea.Cmd) {
	return a, a.postComment(msg.PullRequestKey, msg.ParentID, msg.Text, "")
}

// postComment writes one comment. tmpPath is the file an editor-composed reply
// was written in, kept until the post succeeds so a failure never costs the
// text; it is empty for a reply typed in the composer, which is handed back to
// the composer instead.
func (a *App) postComment(key string, parentID int64, text, tmpPath string) tea.Cmd {
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
	create := bitbucket.CommentCreate{Raw: text, ParentID: parentID}

	a.beginWrite(key, verbComment)
	a.helpBar.SetStatusMsg("posting a comment on #" + strconv.Itoa(id) + "…")

	return func() tea.Msg {
		comment, err := client.CreateComment(ctx, workspace, slug, id, create)
		return commentPostedMsg{
			key: key, parentID: parentID, text: text, tmpPath: tmpPath,
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
		a.helpBar.SetStatusMsg(commentDoneText(msg.parentID))
		return a, nil
	}

	threads := bitbucket.AppendComment(entry.value, *msg.comment)
	a.threadsCache[msg.key] = reviewEntry[[]bitbucket.Thread]{
		value: threads, sourceHash: entry.sourceHash, err: nil,
	}
	a.detailView.SetThreads(msg.key, threads)
	a.applyFileDiffMarkers(msg.key)

	// A new conversation is worth showing: the reader has just written the
	// first word of it and nothing on screen names it yet. A reply lands in the
	// thread that is already open, so the panel stays where it is.
	if msg.parentID == 0 && a.detailView.PullRequestKey() == msg.key {
		a.detailView.FocusThread(msg.comment.ID)
	}
	a.helpBar.SetStatusMsg(commentDoneText(msg.parentID))
	return a, nil
}

// commentDoneText is the line a written comment leaves behind.
func commentDoneText(parentID int64) string {
	if parentID == 0 {
		return "comment posted"
	}
	return "reply posted"
}

// noteCommentFailure says what went wrong and gives the text back. A reply
// typed in the composer goes back into the composer; one composed in the editor
// stays in its file, and the message says where, because a paragraph of
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
	a.helpBar.SetStatusMsg(reason)
	if a.detailView.PullRequestKey() == msg.key {
		a.detailView.ResumeComment(msg.parentID, msg.text)
	}
	return nil
}

// handleComposeCommentInEditor takes the composer's text out to the configured
// editor. It is the same hand-off the description editor makes: a temp file,
// tea.ExecProcess, and the file read back when the editor exits.
func (a *App) handleComposeCommentInEditor(msg views.ComposeCommentInEditorMsg) (tea.Model, tea.Cmd) {
	pr := a.pullRequestByKey(msg.PullRequestKey)
	if pr == nil {
		return a, nil
	}

	path, err := writeCommentTempFile(pr.RepoSlug(), pr.ID, msg.Text)
	if err != nil {
		a.statusPanel.SetError(verbComment + ": " + err.Error())
		a.detailView.ResumeComment(msg.ParentID, msg.Text)
		return a, nil
	}
	cmd, err := a.buildTempFileEditorCmd(path)
	if err != nil {
		_ = os.Remove(path)
		a.statusPanel.SetError(err.Error())
		a.detailView.ResumeComment(msg.ParentID, msg.Text)
		return a, nil
	}

	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return a, tea.ExecProcess(cmd, func(execErr error) tea.Msg {
		return commentEditedMsg{
			key: msg.PullRequestKey, parentID: msg.ParentID, path: path, err: execErr,
		}
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

	if post := a.postComment(msg.key, msg.parentID, string(body), msg.path); post != nil {
		cmds = append(cmds, post)
	}
	return a, tea.Batch(cmds...)
}

// writeCommentTempFile puts a reply somewhere an editor can open it. The name
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
