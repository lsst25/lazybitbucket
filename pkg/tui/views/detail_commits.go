package views

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
	"github.com/lsst25/lazybitbucket/pkg/tui/theme"
)

// renderCommitBlocks builds a two-line block per commit: the identity line
// (hash, author, age) and the subject under it. The body of the message is not
// drawn here — Enter expands it, which is the only way a fifty-line commit
// message does not swamp the list.
func (d *DetailView) renderCommitBlocks(width int) [][]string {
	if placeholder, ok := d.commitsPlaceholder(); ok {
		return [][]string{{placeholder}}
	}

	blocks := make([][]string, 0, len(d.commits.items))
	for i := range d.commits.items {
		commit := d.commits.items[i]
		blocks = append(blocks, []string{
			commitHeadline(commit, width),
			" " + components.TruncateEnd(commit.Subject(), max(width-1, 8)),
		})
	}
	return blocks
}

// commitsPlaceholder is the single line the Commits tab shows instead of a
// list, and whether there is one. Like the Files tab, a refetch over a loaded
// list keeps the list rather than collapsing it to a placeholder.
func (d *DetailView) commitsPlaceholder() (string, bool) {
	if len(d.commits.items) > 0 {
		return "", false
	}
	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)
	switch {
	case d.commits.errMsg != "":
		return d.theme.ErrorText.Render(d.commits.errMsg), true
	case d.commits.loading || !d.commits.loaded:
		return gray.Render("Loading commits…"), true
	default:
		return gray.Render("No commits"), true
	}
}

// commitHeadline is "abc1234  Grace Hopper  3d ago".
func commitHeadline(commit bitbucket.Commit, width int) string {
	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)
	yellow := lipgloss.NewStyle().Foreground(theme.ColorYellow)

	head := yellow.Render(commit.ShortHash()) + "  " +
		theme.AuthorRender(components.TruncateEnd(commit.AuthorName(), max(width/2, 8)))
	if age := timeAgo(commit.Date); age != "" {
		head += gray.Render("  " + age + " ago")
	}
	return head
}

// activateCommit hands the app the whole commit message to open in a modal:
// the full hash and author, then the message verbatim, which is the only place
// the body below the subject is readable.
func (d *DetailView) activateCommit() tea.Cmd {
	commit := d.SelectedCommit()
	if commit == nil {
		return nil
	}

	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)
	lines := []string{
		" " + commit.Hash,
		" " + theme.AuthorRender(commit.AuthorName()) + gray.Render("  "+commit.Date.Format(commitDateLayout)),
		"",
	}
	for _, line := range strings.Split(strings.TrimRight(commit.Message, "\n"), "\n") {
		lines = append(lines, " "+line)
	}

	title := "Commit " + commit.ShortHash()
	return func() tea.Msg {
		return ExpandBlockMsg{Title: title, Lines: lines}
	}
}

// commitDateLayout is the absolute timestamp the expanded commit shows, since
// the list only has room for a relative age.
const commitDateLayout = "2006-01-02 15:04"
