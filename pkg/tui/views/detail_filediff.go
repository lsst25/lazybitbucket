package views

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/diff"
	"github.com/lsst25/lazybitbucket/pkg/tui/theme"
)

// diffHunkHeader marks a rendered row that is a hunk header rather than a line
// of the file. It is deliberately outside diff.LineKind's own range, so a
// caller that switches on the kind cannot mistake a header for content.
const diffHunkHeader diff.LineKind = -1

const (
	// diffTabWidth is the tab stop the code column expands tabs to. Diff rows
	// are hard-truncated rather than wrapped, so a literal tab would make the
	// column widths depend on the terminal.
	diffTabWidth = 4
	// diffGutterMin is the narrowest a line-number column gets, so short files
	// and long files line their code up at the same place.
	diffGutterMin = 3
	// diffCodeMin keeps a sliver of code visible in a panel too narrow to
	// deserve one.
	diffCodeMin = 8
	// fileLevelMarker is the marker map key of a thread anchored to the file
	// rather than to any of its lines. It matches bitbucket.CommentedLines.
	fileLevelMarker = 0
	// analyseBudget bounds the text handed to the lexer sniffer when the file
	// extension says nothing.
	analyseBudget = 8 << 10
)

// SetFileDiffLoading opens the file diff view on path with a placeholder body.
// A call for another pull request is dropped.
func (d *DetailView) SetFileDiffLoading(prKey, path string) {
	if !d.keyMatches(prKey) {
		return
	}
	d.clearFileDiff()
	d.fileDiff.path = path
	d.fileDiff.loading = true
	d.mode = ModeFileDiff
	d.scrollY = 0
	d.listCursor = 0
	d.invalidateBlocks()
}

// SetFileDiff shows one file's parsed diff. comments maps line numbers to the
// number of review threads anchored there, in bitbucket.CommentedLines' key
// space: new-side lines positive, old-side lines negated, file-level threads at
// zero. The call is dropped unless the view is still waiting on this file of
// this pull request, and it does not move the cursor — the app calls it a
// second time once the threads arrive, to light up the markers under a reader
// who has already scrolled.
func (d *DetailView) SetFileDiff(prKey, path string, file *diff.File, comments map[int]int) {
	if !d.fileDiffMatches(prKey, path) {
		return
	}
	if d.fileDiff.file != file {
		d.fileDiff.highlighter = newHighlighter(path, analyseSample(file), d.codeStyle)
	}
	d.fileDiff.file = file
	d.fileDiff.markers = comments
	d.fileDiff.loading = false
	d.fileDiff.errMsg = ""
	d.fileDiff.lines = buildDiffRows(file)
	d.listCursor = clampIndex(d.listCursor, len(d.fileDiff.lines))
	d.invalidateBlocks()
}

// SetFileDiffError shows why a file's diff could not be fetched — a parse
// failure, or Bitbucket's "too large" refusal.
func (d *DetailView) SetFileDiffError(prKey, path, msg string) {
	if !d.fileDiffMatches(prKey, path) {
		return
	}
	d.fileDiff.errMsg = msg
	d.fileDiff.loading = false
	d.fileDiff.file = nil
	d.fileDiff.lines = nil
	d.listCursor = 0
	d.invalidateBlocks()
}

// fileDiffMatches reports whether a diff response is still the one the view is
// waiting on. Like the step log, the check is identity-based: Back() and
// FocusThread() clear the path, so a late response cannot re-enter the mode.
func (d *DetailView) fileDiffMatches(prKey, path string) bool {
	return d.mode == ModeFileDiff && d.fileDiff.path != "" &&
		d.fileDiff.path == path && d.keyMatches(prKey)
}

// FileDiffPath is the file whose diff is on screen, "" in every other mode.
func (d *DetailView) FileDiffPath() string {
	if d.mode != ModeFileDiff {
		return ""
	}
	return d.fileDiff.path
}

// SelectedDiffLine is the diff line under the cursor. A hunk header reports
// diffHunkHeader as its kind with the hunk's start lines filled in, which is
// what "open the editor here" needs when the cursor is not on a real line.
func (d *DetailView) SelectedDiffLine() (diff.Line, bool) {
	if d.mode != ModeFileDiff || d.listCursor < 0 || d.listCursor >= len(d.fileDiff.lines) {
		return diff.Line{}, false
	}
	return d.fileDiff.lines[d.listCursor], true
}

// buildDiffRows flattens a file's hunks into the rows the view draws: a header
// row per hunk followed by that hunk's lines.
func buildDiffRows(file *diff.File) []diff.Line {
	if file == nil {
		return nil
	}
	rows := make([]diff.Line, 0, 8)
	for _, hunk := range file.Hunks {
		rows = append(rows, diff.Line{
			Kind:    diffHunkHeader,
			Content: hunkHeaderText(hunk),
			OldNo:   hunk.OldStart,
			NewNo:   hunk.NewStart,
		})
		rows = append(rows, hunk.Lines...)
	}
	return rows
}

// hunkHeaderText rebuilds the "@@ -a,b +c,d @@ context" line.
func hunkHeaderText(hunk diff.Hunk) string {
	text := fmt.Sprintf("@@ -%d,%d +%d,%d @@", hunk.OldStart, hunk.OldLines, hunk.NewStart, hunk.NewLines)
	if hunk.Header != "" {
		text += " " + hunk.Header
	}
	return text
}

// analyseSample is the text the lexer sniffer sees when the file extension is
// unknown: the diff's own content lines, capped, since the whole file is not
// available here.
func analyseSample(file *diff.File) string {
	if file == nil {
		return ""
	}
	var sample strings.Builder
	for _, hunk := range file.Hunks {
		for _, line := range hunk.Lines {
			if sample.Len()+len(line.Content) > analyseBudget {
				return sample.String()
			}
			sample.WriteString(line.Content)
			sample.WriteByte('\n')
		}
	}
	return sample.String()
}

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------

// fileDiffHeader is the two-line preamble above the diff: the file's own status
// line, and a rule. Neither is selectable, so the cursor only ever lands on a
// diff row.
func (d *DetailView) fileDiffHeader(width int) []string {
	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)

	note := ""
	if n := d.fileDiff.markers[fileLevelMarker]; n > 0 {
		note = " · " + strconv.Itoa(n) + " " + plural(n, "comment", "comments")
	}

	entry := d.diffStatEntryFor(d.fileDiff.path)
	counts := fileCounts(entry)
	headline := fileLine(entry, counts, lipgloss.Width(counts), max(width-2-len(note), 8))

	return []string{
		" " + headline + gray.Render(note),
		" " + gray.Render(strings.Repeat("─", max(width-2, 1))),
	}
}

// diffStatEntryFor is the diffstat row describing path. A file diff can be
// opened before the diffstat has arrived (or for a path the diffstat does not
// list), so it falls back to what the parsed diff itself knows.
func (d *DetailView) diffStatEntryFor(path string) bitbucket.DiffStatEntry {
	if i := d.fileIndex(path); i >= 0 {
		return d.files.items[i]
	}

	entry := bitbucket.DiffStatEntry{
		Type:         "diffstat",
		Status:       bitbucket.DiffStatModified,
		LinesAdded:   0,
		LinesRemoved: 0,
		Old:          nil,
		New:          &bitbucket.CommitFile{Path: path, EscapedPath: path},
	}
	file := d.fileDiff.file
	if file == nil {
		return entry
	}
	entry.Status = diffStatusName(file.Status)
	entry.LinesAdded = file.Added()
	entry.LinesRemoved = file.Removed()
	if file.OldPath != "" {
		entry.Old = &bitbucket.CommitFile{Path: file.OldPath, EscapedPath: file.OldPath}
	}
	if file.IsBinary {
		entry.New.Attributes = []string{"binary"}
	}
	return entry
}

// diffStatusName maps the parser's file status onto the diffstat vocabulary the
// file list is rendered from, so the header of a diff and its row in the Files
// tab cannot disagree.
func diffStatusName(status diff.FileStatus) string {
	switch status {
	case diff.Added:
		return bitbucket.DiffStatAdded
	case diff.Deleted:
		return bitbucket.DiffStatRemoved
	case diff.Renamed, diff.Copied:
		return bitbucket.DiffStatRenamed
	case diff.Modified:
		return bitbucket.DiffStatModified
	default:
		return bitbucket.DiffStatModified
	}
}

// newDiffLineBlocks builds one single-line block per diff row, so the block
// engine's cursor, auto-scroll, footer and hit-test all work on diff lines
// without a second cursor implementation. blockGap is zero for this list, which
// is what makes N rows render as N lines.
//
// The rows come back empty, with renderRow to fill in the window on screen:
// every row costs a tab expansion, a truncation and a chroma pass, and a
// six-thousand-line file is a second of that per render — once per keystroke,
// and again for every column a terminal drag passes through. The block heights
// are known without the text (one line each), so nothing the engine does before
// drawing needs it.
func (d *DetailView) newDiffLineBlocks(width int) (blocks [][]string, rendered []bool, renderRow func(int) string) {
	if placeholder, ok := d.fileDiffPlaceholder(); ok {
		return [][]string{{placeholder}}, nil, nil
	}

	oldWidth, newWidth := diffGutterWidths(d.fileDiff.lines)
	codeWidth := max(width-oldWidth-newWidth-4, diffCodeMin)

	rows := d.fileDiff.lines
	blocks = make([][]string, len(rows))
	for i := range blocks {
		blocks[i] = []string{""}
	}
	return blocks, make([]bool, len(rows)), func(i int) string {
		return d.renderDiffRow(rows[i], oldWidth, newWidth, codeWidth)
	}
}

// fileDiffPlaceholder is the single line the diff shows instead of rows, and
// whether there is one.
func (d *DetailView) fileDiffPlaceholder() (string, bool) {
	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)
	switch {
	case d.fileDiff.errMsg != "":
		return d.theme.ErrorText.Render(d.fileDiff.errMsg), true
	case d.fileDiff.loading || d.fileDiff.file == nil:
		return gray.Render("Loading diff…"), true
	case d.fileDiff.file.IsBinary:
		return gray.Render("Binary file"), true
	case len(d.fileDiff.lines) == 0:
		return gray.Render("(no textual changes)"), true
	default:
		return "", false
	}
}

// diffGutterWidths is how wide each line-number column has to be for the
// widest number in the file, never narrower than diffGutterMin.
func diffGutterWidths(rows []diff.Line) (oldWidth, newWidth int) {
	oldMax, newMax := 0, 0
	for _, row := range rows {
		oldMax = max(oldMax, row.OldNo)
		newMax = max(newMax, row.NewNo)
	}
	return max(len(strconv.Itoa(oldMax)), diffGutterMin), max(len(strconv.Itoa(newMax)), diffGutterMin)
}

// renderDiffRow draws one row: the two line-number columns, a separator, the
// marker column and the code. The row is hard-truncated rather than wrapped, so
// every row is exactly one line and the cursor bar can replace its leading
// space without disturbing the columns.
func (d *DetailView) renderDiffRow(row diff.Line, oldWidth, newWidth, codeWidth int) string {
	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)
	if row.Kind == diffHunkHeader {
		text := ansi.Truncate(row.Content, oldWidth+newWidth+4+codeWidth, "")
		return lipgloss.NewStyle().Foreground(theme.ColorCyan).Render(text)
	}

	gutter := padLeft(diffLineNo(row.OldNo), oldWidth) + " " +
		padLeft(diffLineNo(row.NewNo), newWidth) + " │"
	marker, code := d.diffRowBody(row, codeWidth)
	return gray.Render(gutter) + marker + code
}

// diffLineNo is a gutter cell: the line number, or blank on the side the line
// does not exist on.
func diffLineNo(number int) string {
	if number == 0 {
		return ""
	}
	return strconv.Itoa(number)
}

// diffRowBody is the marker column and the code column of one row. A review
// thread anchored to the line takes the marker column over the +/- sign,
// because a reader scanning for conversations needs to find them and already
// knows the colour of an added line.
func (d *DetailView) diffRowBody(row diff.Line, codeWidth int) (marker, code string) {
	green := lipgloss.NewStyle().Foreground(theme.ColorGreen)
	red := lipgloss.NewStyle().Foreground(theme.ColorRed)
	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)

	text := ansi.Truncate(expandTabs(row.Content, diffTabWidth), codeWidth, "")
	switch row.Kind {
	case diff.LineNoNewline:
		return gray.Render("\\"), gray.Render(text)
	case diff.LineAdded:
		return d.diffMarker(row, green.Render("+")), d.diffCode(text, green)
	case diff.LineRemoved:
		return d.diffMarker(row, red.Render("-")), d.diffCode(text, red)
	case diff.LineContext:
		return d.diffMarker(row, " "), d.diffCode(text, lipgloss.NewStyle())
	default:
		return " ", text
	}
}

// diffCode highlights a code cell, falling back to colouring the whole cell in
// the line's own colour when highlighting is off.
func (d *DetailView) diffCode(text string, plain lipgloss.Style) string {
	if d.fileDiff.highlighter == nil {
		return plain.Render(text)
	}
	return d.fileDiff.highlighter.Line(text)
}

// diffMarker is the marker cell: the thread glyph when the line carries review
// threads (the count itself once there is more than one), else fallback.
func (d *DetailView) diffMarker(row diff.Line, fallback string) string {
	count := d.diffRowThreadCount(row)
	switch {
	case count == 1:
		return lipgloss.NewStyle().Foreground(theme.ColorCyan).Render("●")
	case count > 1:
		return lipgloss.NewStyle().Foreground(theme.ColorCyan).Render(strconv.Itoa(min(count, 9)))
	default:
		return fallback
	}
}

// diffRowThreadCount is how many review threads hang off a row. The marker map
// keys new-side lines positively and old-side lines negatively, and zero means
// "the whole file", so a row with no number on its side is never looked up.
func (d *DetailView) diffRowThreadCount(row diff.Line) int {
	if len(d.fileDiff.markers) == 0 {
		return 0
	}
	switch row.Kind {
	case diff.LineAdded, diff.LineContext:
		if row.NewNo == 0 {
			return 0
		}
		return d.fileDiff.markers[row.NewNo]
	case diff.LineRemoved:
		if row.OldNo == 0 {
			return 0
		}
		return d.fileDiff.markers[-row.OldNo]
	case diff.LineNoNewline:
		return 0
	default:
		return 0
	}
}

// activateDiffLine asks the app to jump to the review thread on the cursor
// line. It emits nothing when the line has none, and nothing at all before the
// threads have loaded — the markers are not on screen then either.
func (d *DetailView) activateDiffLine() tea.Cmd {
	row, ok := d.SelectedDiffLine()
	if !ok || d.pr == nil || len(d.threads.items) == 0 {
		return nil
	}
	line, newSide, anchored := diffRowAnchor(row)
	if !anchored {
		return nil
	}
	thread, found := bitbucket.ThreadAt(d.threads.items, d.fileDiff.path, line, newSide)
	if !found {
		return nil
	}

	key, rootID := d.pr.Key(), thread.Root.ID
	return func() tea.Msg {
		return ThreadSelectedMsg{PullRequestKey: key, RootID: rootID}
	}
}

// diffRowAnchor is the (line, side) pair an inline comment on this row would
// be anchored to.
func diffRowAnchor(row diff.Line) (line int, newSide, ok bool) {
	switch row.Kind {
	case diff.LineAdded, diff.LineContext:
		return row.NewNo, true, row.NewNo != 0
	case diff.LineRemoved:
		return row.OldNo, false, row.OldNo != 0
	case diff.LineNoNewline:
		return 0, false, false
	default:
		return 0, false, false
	}
}
