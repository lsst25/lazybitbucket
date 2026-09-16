package views

import (
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
	"github.com/lsst25/lazybitbucket/pkg/tui/theme"
)

// ThreadSelectedMsg asks the app to jump to a review thread, which is what
// Enter on a commented diff line does.
type ThreadSelectedMsg struct {
	PullRequestKey string
	RootID         int64
}

// generalThreadLabel is the anchor shown for a thread that belongs to the pull
// request rather than to any file.
const generalThreadLabel = "General"

// replyIndent is how far a reply's body sits in from the block's left edge: it
// lines the body up under the author name, past the " ↳ " glyph.
const replyIndent = "   "

// renderThreadBlocks builds one block per review thread: the anchor header,
// the root comment and every reply. Blocks longer than maxBlockLines are
// collapsed by the block engine and Enter expands them, so a long argument in a
// review does not push every other thread off screen.
func (d *DetailView) renderThreadBlocks(width int) [][]string {
	if placeholder, ok := d.threadsPlaceholder(); ok {
		return [][]string{{placeholder}}
	}

	listed := d.listedThreads()
	blocks := make([][]string, 0, len(listed))
	for i := range listed {
		blocks = append(blocks, d.renderThread(listed[i], width, ""))
	}
	return blocks
}

// threadsPlaceholder is the single line the Comments tab shows instead of a
// list, and whether there is one. Like the Files tab, a refetch over a loaded
// list keeps the list rather than collapsing it to a placeholder.
func (d *DetailView) threadsPlaceholder() (string, bool) {
	if len(d.listedThreads()) > 0 {
		return "", false
	}
	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)
	switch {
	case d.threads.errMsg != "":
		return d.theme.ErrorText.Render(d.threads.errMsg), true
	case d.threads.loading || !d.threads.loaded:
		return gray.Render("Loading comments…"), true
	case d.hiddenThreadCount() > 0:
		// Not "No comments": there are some, and the reader hid them.
		return gray.Render("Every conversation is resolved"), true
	default:
		return gray.Render("No comments"), true
	}
}

// renderThread lays out one thread's lines. headerIndent is what the anchor
// line sits behind: nothing in the list, where the cursor bar owns that column,
// and one space in the thread section, where the bylines already carry one.
func (d *DetailView) renderThread(thread bitbucket.Thread, width int, headerIndent string) []string {
	lines := make([]string, 0, 4+2*len(thread.Replies))
	lines = append(lines, headerIndent+d.threadHeader(thread, width))
	lines = append(lines, d.renderComment(thread.Root, "", width)...)
	for _, reply := range thread.Replies {
		lines = append(lines, d.renderComment(reply, "↳ ", width)...)
	}
	return lines
}

// threadHeader is the anchor line: where the thread hangs, whether it has been
// resolved and how many replies it carries.
//
// A resolved conversation is marked twice over, because it is listed among open
// ones only when the reader asked for it and has to be tellable apart at a
// glance: a green ✔ leads the line, and the anchor itself is drawn in gray
// rather than in the cyan an open conversation gets.
func (d *DetailView) threadHeader(thread bitbucket.Thread, width int) string {
	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)
	anchorStyle := lipgloss.NewStyle().Foreground(theme.ColorCyan)

	prefix := ""
	if thread.Resolved() {
		prefix = lipgloss.NewStyle().Foreground(theme.ColorGreen).Render("✔ ")
		anchorStyle = gray
	}

	header := prefix + anchorStyle.Render(
		components.TruncateEnd(threadAnchor(thread), max(width-30, 8)))
	if thread.Resolved() {
		header += gray.Render("  " + resolvedBy(thread))
	}
	if n := len(thread.Replies); n > 0 {
		header += gray.Render(" · " + strconv.Itoa(n) + " " + plural(n, "reply", "replies"))
	}
	return header
}

// resolvedBy says who ended the conversation and when, and falls back to the
// bare word for a resolution whose members the server did not send — which is
// what a collection answers with unless the request asks for them by name.
func resolvedBy(thread bitbucket.Thread) string {
	resolution := thread.Root.Resolution
	if resolution == nil {
		return "resolved"
	}
	text := "resolved"
	if resolution.User != nil {
		if name := resolution.User.Name(); name != "" {
			text += " by " + name
		}
	}
	if age := timeAgo(resolution.CreatedOn); age != "" {
		text += " · " + age + " ago"
	}
	return text
}

// threadAnchor is "path:line" for an inline thread, the bare path when the
// thread hangs off the file rather than a line, and "General" when it hangs off
// the pull request.
func threadAnchor(thread bitbucket.Thread) string {
	if !thread.IsInline() {
		return generalThreadLabel
	}
	path := thread.Path()
	if line, _, ok := thread.Root.Inline.Line(); ok {
		return path + ":" + strconv.Itoa(line)
	}
	return path
}

// plural picks the noun for n.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// renderComment is one comment's author line plus its Markdown body. glyph
// prefixes the author line and pushes the body in under it, so a reply reads as
// subordinate to the root above it. The body is the readable form of the raw
// Markdown: a mention Bitbucket rendered reads as "@Grace Hopper", in the
// theme's mention colour, rather than as the account id the raw stores it
// under.
func (d *DetailView) renderComment(comment bitbucket.Comment, glyph string, width int) []string {
	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)
	yellow := lipgloss.NewStyle().Foreground(theme.ColorYellow)

	indent := " "
	if glyph != "" {
		indent = replyIndent
	}

	byline := " " + glyph + theme.AuthorRender(comment.User.Name())
	if age := timeAgo(comment.CreatedOn); age != "" {
		byline += gray.Render(" · " + age + " ago")
	}
	if comment.Deleted {
		byline += gray.Render("  (deleted)")
	}
	if comment.Pending {
		byline += yellow.Render("  (pending)")
	}

	body := d.renderBody(comment.Content.Raw, comment.Content.HTML, max(width-len(indent)-3, 8))
	lines := make([]string, 0, 1+len(body))
	lines = append(lines, byline)
	for _, line := range body {
		lines = append(lines, indent+line)
	}
	return lines
}

// activateThread opens the thread under the cursor in the panel itself, the way
// Enter on a changed file opens its diff and Enter on a pipeline step opens its
// log. The list collapses a long conversation behind an ellipsis; the section
// shows it whole and scrolls, and it is where replying to a comment will live.
func (d *DetailView) activateThread() tea.Cmd {
	thread := d.SelectedThread()
	if thread == nil {
		return nil
	}
	d.openThread(thread.Root.ID)
	return nil
}

// openThread puts one conversation on screen, rewound to the top.
func (d *DetailView) openThread(rootID int64) {
	d.mode = ModeThread
	d.threadRoot = rootID
	d.scrollY = 0
	d.listCursor = 0
	d.invalidateBlocks()
}

// OpenedThread is the conversation ModeThread is showing, nil outside it and
// when the thread it names is no longer in the list — a refetch can drop a
// comment the reader had open.
func (d *DetailView) OpenedThread() *bitbucket.Thread {
	if d.mode != ModeThread {
		return nil
	}
	i := d.threadIndex(d.threadRoot)
	if i < 0 {
		return nil
	}
	return &d.threads.items[i]
}

// threadIndex is the position of the thread rooted at rootID among every
// thread the pull request has, -1 when there is none. It deliberately ignores
// the resolved-threads filter: a conversation open in the section stays open
// when the list it came from stops listing it.
//
// listedThreadIndex is the other one — where a thread sits on the list, which
// is what the cursor indexes.
func (d *DetailView) threadIndex(rootID int64) int {
	for i := range d.threads.items {
		if d.threads.items[i].Root.ID == rootID {
			return i
		}
	}
	return -1
}

// openThreadAnchor is the open conversation's anchor, for the panel title. A
// thread that is no longer in the list still has a title to carry.
func (d *DetailView) openThreadAnchor() string {
	if thread := d.OpenedThread(); thread != nil {
		return threadAnchor(*thread)
	}
	return generalThreadLabel
}

// threadBodyLines is the ModeThread body: the open conversation whole — header,
// root and every reply, none of it collapsed. Rendering runs glamour once per
// thread and width; a text body is rebuilt on every View, so the result is
// memoised until something invalidates it.
func (d *DetailView) threadBodyLines(width int) []string {
	if d.threadBody.valid && d.threadBody.root == d.threadRoot && d.threadBody.width == width {
		return d.threadBody.lines
	}

	var lines []string
	if thread := d.OpenedThread(); thread != nil {
		lines = d.renderThread(*thread, width, " ")
	} else {
		lines = []string{" " + lipgloss.NewStyle().Foreground(theme.ColorGray).
			Render("This comment is no longer in the pull request.")}
	}

	d.threadBody = threadBodyCache{lines: lines, root: d.threadRoot, width: width, valid: true}
	return lines
}

// threadBodyCache memoises one conversation's rendered lines. An entry is only
// valid for the thread and width it was built at.
type threadBodyCache struct {
	lines []string
	root  int64
	width int
	valid bool
}

// SelectedOrOpenThread is the conversation a key pressed now would act on: the
// one the section is showing, or the one under the cursor on the Comments tab.
// It is nil anywhere else, and nil for a thread a refetch has taken away.
func (d *DetailView) SelectedOrOpenThread() *bitbucket.Thread {
	if d.mode == ModeThread {
		return d.OpenedThread()
	}
	return d.SelectedThread()
}

// HidesResolvedThreads reports whether the Comments tab is leaving resolved
// conversations out of the list.
func (d *DetailView) HidesResolvedThreads() bool { return !d.showResolved }

// SetShowResolvedThreads sets where the filter starts, which is what
// gui.showResolvedComments configures.
func (d *DetailView) SetShowResolvedThreads(show bool) {
	d.showResolved = show
	d.invalidateBlocksFor(listThreads)
}

// ToggleResolvedThreads hides or shows the resolved conversations and reports
// whether they are now hidden. The cursor lands on the first thread that is
// still listed, because the row it was on may not be there any more.
func (d *DetailView) ToggleResolvedThreads() bool {
	d.showResolved = !d.showResolved
	d.listCursor = 0
	d.scrollY = 0
	d.invalidateBlocksFor(listThreads)
	return !d.showResolved
}

// listedThreads is what the Comments tab shows: every thread, or only the
// unresolved ones. Everything that indexes the list — the cursor, Enter, the
// copy and browser keys — goes through it, so hiding a thread hides it from
// all of them at once rather than from the rendering alone.
func (d *DetailView) listedThreads() []bitbucket.Thread {
	if d.showResolved {
		return d.threads.items
	}
	kept := make([]bitbucket.Thread, 0, len(d.threads.items))
	for _, thread := range d.threads.items {
		if !thread.Resolved() {
			kept = append(kept, thread)
		}
	}
	return kept
}

// hiddenThreadCount is how many conversations the filter is keeping off the
// list, which is what the tab label says.
func (d *DetailView) hiddenThreadCount() int {
	if d.showResolved {
		return 0
	}
	return len(d.threads.items) - len(d.listedThreads())
}

// listedThreadIndex is the position of the thread rooted at rootID on the list
// as it is drawn, -1 when the filter is keeping it off.
func (d *DetailView) listedThreadIndex(rootID int64) int {
	listed := d.listedThreads()
	for i := range listed {
		if listed[i].Root.ID == rootID {
			return i
		}
	}
	return -1
}
