package views

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
// the comment being replied to, 0 for a new comment on the pull request.
type CommentSubmittedMsg struct {
	PullRequestKey string
	ParentID       int64
	Text           string
}

// ComposeCommentInEditorMsg asks the app to reopen the composer's text in the
// configured editor, which is the way out of a one-line input for a reply that
// needs more room. The text travels with it so nothing typed is lost on the way.
type ComposeCommentInEditorMsg struct {
	PullRequestKey string
	ParentID       int64
	Text           string
}

// composerState is the docked reply box. parentID names what is being answered;
// zero is a new comment on the pull request itself.
type composerState struct {
	active   bool
	parentID int64
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
	parentID, ok := d.commentParent()
	if !ok {
		return false
	}
	d.openComposer(parentID, "")
	return true
}

// ResumeComment reopens the composer on text that was already written — what a
// failed post hands back, so a refusal costs the wording and not the reply.
func (d *DetailView) ResumeComment(parentID int64, text string) {
	d.openComposer(parentID, text)
}

// CancelComment closes the composer and forgets what was in it.
func (d *DetailView) CancelComment() {
	d.composer = composerState{active: false, parentID: 0, input: components.NewTextInput()}
}

// openComposer is the shared open: a fresh input carrying text, with the cursor
// at the end of it.
func (d *DetailView) openComposer(parentID int64, text string) {
	input := components.NewTextInput()
	input.SetValue(text)
	d.composer = composerState{active: true, parentID: parentID, input: input}
}

// commentParent resolves what a comment written here would answer, and whether
// this is a place to write one at all.
func (d *DetailView) commentParent() (int64, bool) {
	if d.pr == nil {
		return 0, false
	}
	switch d.mode {
	case ModeThread:
		// The conversation on screen — unless a refetch has taken it away, in
		// which case there is nothing left to reply to.
		thread := d.OpenedThread()
		if thread == nil {
			return 0, false
		}
		return thread.Root.ID, true
	case ModePullRequest:
		switch d.activeTab {
		case TabComments:
			if thread := d.SelectedThread(); thread != nil {
				return thread.Root.ID, true
			}
			// An empty Comments tab: the first comment of a pull request is a
			// comment on the pull request.
			return 0, true
		case TabOverview:
			return 0, true
		case TabFiles, TabCommits:
			return 0, false
		default:
			return 0, false
		}
	case ModePipeline, ModeStepLog, ModeRepo, ModeSplash, ModeFileDiff:
		return 0, false
	default:
		return 0, false
	}
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
	text, parentID, key := d.composer.input.Value(), d.composer.parentID, d.PullRequestKey()
	d.CancelComment()
	if strings.TrimSpace(text) == "" || key == "" {
		return nil
	}
	return func() tea.Msg {
		return CommentSubmittedMsg{PullRequestKey: key, ParentID: parentID, Text: text}
	}
}

// composeInEditor asks the app to continue in the editor. The box closes — the
// editor takes the whole terminal — and the app reopens it if the editor could
// not be started.
func (d *DetailView) composeInEditor() tea.Cmd {
	text, parentID, key := d.composer.input.Value(), d.composer.parentID, d.PullRequestKey()
	if key == "" {
		return nil
	}
	d.CancelComment()
	return func() tea.Msg {
		return ComposeCommentInEditorMsg{PullRequestKey: key, ParentID: parentID, Text: text}
	}
}

// composerLines is the docked box: the labelled input and the key hint under
// it. width is the panel's content width.
func (d *DetailView) composerLines(width int) []string {
	label := "Reply"
	if d.composer.parentID == 0 {
		label = "Comment"
	}

	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)
	// The label, its spaces and the cursor block the input always draws.
	d.composer.input.SetWidth(max(width-len(label)-3, 8))

	return []string{
		" " + theme.AuthorRender(label) + " " + d.composer.input.View(),
		" " + gray.Render(components.TruncateEnd(composerHint, max(width-1, 8))),
	}
}
