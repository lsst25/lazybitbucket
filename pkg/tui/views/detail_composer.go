package views

import (
	"path"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
	"github.com/lsst25/lazybitbucket/pkg/tui/theme"
)

// The composer is the one write surface that is not a dialog: it docks at the
// bottom of the detail panel, under the conversation it is answering, and takes
// every key while it is open. Enter sends, esc drops what was typed, and ctrl+e
// hands the text to the configured editor for a reply that wants paragraphs —
// the input itself is one line, which is what most replies are.
//
// The view composes; the app writes. What the composer knows is the text and
// the comment it answers; whether that may be sent at all — read-only mode, a
// write already in flight — is the app's to decide, as it is for every other
// write key.

// CommentSubmittedMsg asks the app to post what the composer holds. ParentID is
// the comment being replied to and Anchor the place in the diff a new
// conversation hangs off; a comment on the pull request itself carries neither.
type CommentSubmittedMsg struct {
	PullRequestKey string
	ParentID       int64
	Anchor         *bitbucket.CommentAnchor
	Text           string
}

// ComposeCommentInEditorMsg asks the app to reopen the composer's text in the
// configured editor, which is the way out of a one-line input for a reply that
// needs more room. The text and the target travel with it so nothing typed is
// lost on the way, and the comment still lands where it was started.
type ComposeCommentInEditorMsg struct {
	PullRequestKey string
	ParentID       int64
	Anchor         *bitbucket.CommentAnchor
	Text           string
}

// composerState is the docked reply box. parentID names what is being answered
// and anchor where a new conversation hangs; both empty is a comment on the
// pull request itself.
type composerState struct {
	active   bool
	parentID int64
	anchor   *bitbucket.CommentAnchor
	input    components.TextInput
}

// composerHint is the key line under the input. It names the three ways out in
// the order they are reached for.
const composerHint = "enter send · ctrl+e editor · esc cancel"

// composerRows is how many lines the composer takes off the body: the input and
// the hint under it.
const composerRows = 2

// Composing reports whether the reply box is open, which is what makes the app
// hand it every key instead of matching bindings.
func (d *DetailView) Composing() bool { return d.composer.active }

// ComposerParent is the comment the open composer answers, 0 for a new comment
// on the pull request and outside the composer alike.
func (d *DetailView) ComposerParent() int64 {
	if !d.composer.active {
		return 0
	}
	return d.composer.parentID
}

// ComposerAnchor is where the open composer's comment will hang, nil for a
// reply and for a comment on the pull request itself.
func (d *DetailView) ComposerAnchor() *bitbucket.CommentAnchor {
	if !d.composer.active {
		return nil
	}
	return d.composer.anchor
}

// ComposerText is what has been typed so far.
func (d *DetailView) ComposerText() string {
	if !d.composer.active {
		return ""
	}
	return d.composer.input.Value()
}

// BeginComment opens the composer against whatever the panel is showing: the
// conversation in view, the conversation under the cursor on the Comments tab,
// or the pull request itself from the Overview tab and from a Comments tab with
// nothing in it yet. It reports false where there is nothing to comment on, so
// the caller can say why rather than open an empty box.
func (d *DetailView) BeginComment() bool {
	parentID, anchor, ok := d.commentTargetHere()
	if !ok {
		return false
	}
	d.openComposer(parentID, anchor, "")
	return true
}

// ResumeComment reopens the composer on text that was already written — what a
// failed post hands back, so a refusal costs the wording and not the reply.
func (d *DetailView) ResumeComment(parentID int64, anchor *bitbucket.CommentAnchor, text string) {
	d.openComposer(parentID, anchor, text)
}

// CancelComment closes the composer and forgets what was in it.
func (d *DetailView) CancelComment() {
	d.composer = composerState{
		active: false, parentID: 0, anchor: nil, input: components.NewTextInput(),
	}
}

// openComposer is the shared open: a fresh input carrying text, with the cursor
// at the end of it.
func (d *DetailView) openComposer(parentID int64, anchor *bitbucket.CommentAnchor, text string) {
	input := components.NewTextInput()
	input.SetValue(text)
	d.composer = composerState{active: true, parentID: parentID, anchor: anchor, input: input}
}

// commentTargetHere resolves what a comment written from where the panel
// currently is would hang off — the comment it answers, or the place in the
// diff it anchors to — and whether this is a place to write one at all.
func (d *DetailView) commentTargetHere() (int64, *bitbucket.CommentAnchor, bool) {
	if d.pr == nil {
		return 0, nil, false
	}
	switch d.mode {
	case ModeThread:
		// The conversation on screen — unless a refetch has taken it away, in
		// which case there is nothing left to reply to.
		thread := d.OpenedThread()
		if thread == nil {
			return 0, nil, false
		}
		return thread.Root.ID, nil, true
	case ModeFileDiff:
		return d.diffCommentAnchor()
	case ModePullRequest:
		return d.tabCommentTarget()
	case ModePipeline, ModeStepLog, ModeRepo, ModeSplash:
		return 0, nil, false
	default:
		return 0, nil, false
	}
}

// tabCommentTarget is commentTargetHere for the four pull request tabs.
func (d *DetailView) tabCommentTarget() (int64, *bitbucket.CommentAnchor, bool) {
	switch d.activeTab {
	case TabComments:
		// A thread under the cursor is answered; a marked line's second
		// conversation is started from the diff, not from here.
		if thread := d.SelectedThread(); thread != nil {
			return thread.Root.ID, nil, true
		}
		// An empty Comments tab: the first comment of a pull request is a
		// comment on the pull request.
		return 0, nil, true
	case TabOverview:
		return 0, nil, true
	case TabFiles:
		// A file with no line: what Bitbucket shows above the first hunk.
		file := d.SelectedFile()
		if file == nil {
			return 0, nil, false
		}
		return 0, &bitbucket.CommentAnchor{Path: file.Path(), Line: 0, OldSide: false}, true
	case TabCommits:
		return 0, nil, false
	default:
		return 0, nil, false
	}
}

// diffCommentAnchor is where a comment written in the file diff hangs: the line
// under the cursor, on the side that line lives on. A row with no line of its
// own — a hunk header, the "\ No newline" marker — anchors to the file, which
// is the other shape Bitbucket takes and the one the reader is looking at
// anyway.
func (d *DetailView) diffCommentAnchor() (int64, *bitbucket.CommentAnchor, bool) {
	if d.fileDiff.path == "" {
		return 0, nil, false
	}
	anchor := &bitbucket.CommentAnchor{Path: d.fileDiff.path, Line: 0, OldSide: false}
	if row, ok := d.SelectedDiffLine(); ok {
		if line, newSide, anchored := diffRowAnchor(row); anchored {
			anchor.Line, anchor.OldSide = line, !newSide
		}
	}
	return 0, anchor, true
}

// composerKey is the composer's whole keyboard. It owns every key while it is
// open — a reply saying "quit" must not quit — so anything the three ways out
// do not claim goes into the text.
func (d *DetailView) composerKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEsc:
		d.CancelComment()
		return nil
	case tea.KeyEnter:
		return d.submitComment()
	case tea.KeyCtrlE:
		return d.composeInEditor()
	default:
		updated, _ := d.composer.input.Update(msg)
		d.composer.input = updated
		return nil
	}
}

// submitComment hands the text to the app and closes the box. An empty one is
// not a comment, so Enter on it simply closes: the reader changed their mind,
// and saying so with an error would be noise.
func (d *DetailView) submitComment() tea.Cmd {
	text, parentID, anchor := d.composer.input.Value(), d.composer.parentID, d.composer.anchor
	key := d.PullRequestKey()
	d.CancelComment()
	if strings.TrimSpace(text) == "" || key == "" {
		return nil
	}
	return func() tea.Msg {
		return CommentSubmittedMsg{
			PullRequestKey: key, ParentID: parentID, Anchor: anchor, Text: text,
		}
	}
}

// composeInEditor asks the app to continue in the editor. The box closes — the
// editor takes the whole terminal — and the app reopens it if the editor could
// not be started.
func (d *DetailView) composeInEditor() tea.Cmd {
	text, parentID, anchor := d.composer.input.Value(), d.composer.parentID, d.composer.anchor
	key := d.PullRequestKey()
	if key == "" {
		return nil
	}
	d.CancelComment()
	return func() tea.Msg {
		return ComposeCommentInEditorMsg{
			PullRequestKey: key, ParentID: parentID, Anchor: anchor, Text: text,
		}
	}
}

// composerLines is the docked box: the labelled input and the key hint under
// it. width is the panel's content width.
func (d *DetailView) composerLines(width int) []string {
	label := d.composerLabel()

	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)
	// The label, its spaces and the cursor block the input always draws.
	d.composer.input.SetWidth(max(width-len(label)-3, 8))

	return []string{
		" " + theme.AuthorRender(label) + " " + d.composer.input.View(),
		" " + gray.Render(components.TruncateEnd(composerHint, max(width-1, 8))),
	}
}

// composerLabel names what is being written, in the room a prompt has: "Reply"
// for an answer, the file and line for an inline comment — by base name, since
// the panel title already carries the path and a prompt taking half the width
// would leave nowhere to type — and "Comment" for one on the pull request.
func (d *DetailView) composerLabel() string {
	switch {
	case d.composer.parentID != 0:
		return "Reply"
	case d.composer.anchor == nil:
		return "Comment"
	case d.composer.anchor.Line > 0:
		return path.Base(d.composer.anchor.Path) + ":" + strconv.Itoa(d.composer.anchor.Line)
	default:
		return path.Base(d.composer.anchor.Path)
	}
}
