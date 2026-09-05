package views

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/lsst25/lazybitbucket/pkg/diff"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// The escape sequences the external-renderer fixtures are built from.
const (
	greenFG   = "\x1b[32m"
	redFG     = "\x1b[31m"
	tintBG    = "\x1b[48;5;236m"
	resetSGR  = "\x1b[0m"
	renderedC = "    ctx := RENDERED()"
)

// renderedDiffFile is one hunk whose lines carry the Source indices the parser
// fills in: this file sits at offset 4 of a document that opened with a file
// header and a hunk header, which is what the renderer's output is indexed by.
func renderedDiffFile() *diff.File {
	return &diff.File{
		OldPath: testDiffPath, NewPath: testDiffPath, Status: diff.Modified,
		Hunks: []diff.Hunk{{
			OldStart: 98, OldLines: 2, NewStart: 98, NewLines: 2, Header: "func main() {",
			Lines: []diff.Line{
				{Kind: diff.LineContext, Content: "\tctx := context.Background()", OldNo: 98, NewNo: 98, Source: 4},
				{Kind: diff.LineRemoved, Content: "\tdefer cancel()", OldNo: 99, Source: 5},
				{Kind: diff.LineAdded, Content: "\tdefer stop()", NewNo: 99, Source: 6},
			},
		}},
	}
}

// renderedCells is the renderer's output for that document: four lines of
// header before the three code cells, each already stripped of its marker and
// expanded, as the fetch command hands them over.
func renderedCells() []string {
	return []string{"", "", "", "", greenFG + renderedC, redFG + "    defer RENDERED()", greenFG + "    stop RENDERED()"}
}

// renderedDiffView is a focused detail view showing renderedDiffFile with the
// given renderer output and chroma style, after a first View().
func renderedDiffView(t *testing.T, codeStyle string, rendered []string) *DetailView {
	t.Helper()
	view := NewDetailView(PlainRenderer{}, codeStyle)
	view.ResolveNav = navResolverForDetail
	view.SetSize(80, 24)
	view.SetFocused(true)
	view.SetPullRequest(detailPR())
	view.SetFileDiffLoading(testPRKey, testDiffPath)
	view.SetFileDiff(testPRKey, testDiffPath, renderedDiffFile(), rendered, nil)
	view.View()
	return view
}

// diffRowContaining is the rendered row, escape sequences and all, whose plain
// text holds want.
func diffRowContaining(t *testing.T, view *DetailView, want string) string {
	t.Helper()
	for _, row := range strings.Split(view.View(), "\n") {
		if strings.Contains(stripANSI(row), want) {
			return row
		}
	}
	t.Fatalf("no row contains %q:\n%s", want, stripANSI(view.View()))
	return ""
}

func TestDetailView_FileDiff_DrawsTheExternalRenderersCells(t *testing.T) {
	t.Parallel()
	view := renderedDiffView(t, chromaStyleDark, renderedCells())
	out := view.View()
	plain := stripANSI(out)

	// The code column is the renderer's, not the parser's own content.
	if !strings.Contains(plain, "ctx := RENDERED()") {
		t.Errorf("the renderer's cell is not on screen:\n%s", plain)
	}
	if strings.Contains(plain, "ctx := context.Background()") {
		t.Errorf("the built-in rendering should have been replaced:\n%s", plain)
	}
	// Its escape sequences reach the terminal untouched.
	if !strings.Contains(out, greenFG+renderedC) {
		t.Errorf("the renderer's SGR did not survive:\n%q", out)
	}
	// The gutters and the marker column stay ours: a removed line still gets a
	// '-' in the column the review markers take over, whatever the renderer did.
	for _, want := range []string{
		" 98  98 │ " + renderedC,
		" 99     │-    defer RENDERED()",
		"     99 │+    stop RENDERED()",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("view does not contain %q:\n%s", want, plain)
		}
	}
	// And the hunk header is still drawn by this view.
	if !strings.Contains(plain, "@@ -98,2 +98,2 @@ func main() {") {
		t.Errorf("the hunk header should still be ours:\n%s", plain)
	}
}

func TestDetailView_FileDiff_RenderedCellsSkipTheHighlighter(t *testing.T) {
	t.Parallel()

	withRenderer := renderedDiffView(t, chromaStyleDark, renderedCells())
	if withRenderer.fileDiff.highlighter != nil {
		t.Error("an external renderer owns the code column; chroma must not be built as well")
	}

	withoutRenderer := renderedDiffView(t, chromaStyleDark, nil)
	if withoutRenderer.fileDiff.highlighter == nil {
		t.Error("with no renderer the chroma highlighter is what colours the code")
	}
}

func TestDetailView_FileDiff_PadsARenderedCellUnderItsOwnBackground(t *testing.T) {
	t.Parallel()
	cells := renderedCells()
	cells[4] = tintBG + renderedC
	view := renderedDiffView(t, "", cells)

	row := diffRowContaining(t, view, "ctx := RENDERED()")

	// delta tints the whole row and closes it with an erase-in-line sequence,
	// which cannot survive inside a column; the pane repaints the tint out to
	// the width of the code column instead.
	if got := strings.Count(row, tintBG); got != 2 {
		t.Errorf("the background should be re-opened for the padding, found it %d times in %q", got, row)
	}
	// What follows the second one is spaces, closed again straight away, so the
	// tint stops at the edge of the code column and nothing else is repainted.
	tail := row[strings.LastIndex(row, tintBG)+len(tintBG):]
	padding, _, closed := strings.Cut(tail, resetSGR)
	if !closed {
		t.Errorf("the padded row must close its background, got %q", row)
	}
	if padding == "" || strings.Trim(padding, " ") != "" {
		t.Errorf("the padding should be spaces alone, got %q", padding)
	}
	testkit.AssertEqual(t, "the row still fits the panel", ansi.StringWidth(row) <= 80, true)
}

func TestDetailView_FileDiff_FallsBackWhenTheCellIsMissing(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		rendered []string
		style    string
		wantANSI bool
	}{
		{"no renderer, chroma on", nil, chromaStyleDark, true},
		{"no renderer, chroma off", nil, "", false},
		// The renderer answered for a shorter document than the file was parsed
		// out of, so no line's Source addresses a cell.
		{"output too short", []string{"one", "two"}, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			view := renderedDiffView(t, tc.style, tc.rendered)

			row := diffRowContaining(t, view, "ctx := context.Background()")

			if strings.Contains(stripANSI(row), "RENDERED") {
				t.Errorf("no cell should have been used, got %q", row)
			}
			// The context row is the one only the highlighter ever colours.
			code := row[strings.Index(row, "│")+len("│"):]
			if got := strings.Contains(code, escape); got != tc.wantANSI {
				t.Errorf("code carries ANSI = %v, want %v: %q", got, tc.wantANSI, code)
			}
		})
	}
}

func TestLeadingSGR(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		line     string
		wantSeq  string
		wantHave bool
	}{
		{"plain text", "code", "", false},
		{"foreground", greenFG + "code", greenFG, true},
		{"background", tintBG + "code", tintBG, true},
		{"reset", resetSGR + "code", resetSGR, true},
		{"no parameters", "\x1b[mcode", "\x1b[m", true},
		{"not an SGR", "\x1b[0Kcode", "", false},
		{"an SGR that is not first", "code" + greenFG, "", false},
		{"an OSC", "\x1b]8;;https://x\x07code", "", false},
		{"unterminated", "\x1b[32", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			seq, ok := leadingSGR(tc.line)
			testkit.AssertEqual(t, "ok", ok, tc.wantHave)
			testkit.AssertEqual(t, "sequence", seq, tc.wantSeq)
		})
	}
}

func TestSGRSetsBackground(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		seq  string
		want bool
	}{
		{"foreground", greenFG, false},
		{"reset", resetSGR, false},
		{"no parameters", "\x1b[m", false},
		{"ansi background", "\x1b[42m", true},
		{"bright background", "\x1b[102m", true},
		{"palette background", tintBG, true},
		{"rgb background", "\x1b[48;2;40;40;40m", true},
		// The parameters of an extended foreground must not be read as codes of
		// their own: the 41 below is a shade of green, not a red background.
		{"palette foreground", "\x1b[38;5;41m", false},
		{"rgb foreground", "\x1b[38;2;100;102;40m", false},
		{"foreground then background", "\x1b[38;2;255;0;0;44m", true},
		{"bold, foreground, background", "\x1b[1;32;48;2;40;40;40m", true},
		{"truncated extended colour", "\x1b[38;5m", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			testkit.AssertEqual(t, "sgrSetsBackground", sgrSetsBackground(tc.seq), tc.want)
		})
	}
}

func TestPadUnderBackground(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		cell  string
		width int
		want  string
	}{
		{"background is repainted", tintBG + "tint", 10, tintBG + "tint" + tintBG + "      " + resetSGR},
		{"foreground is left alone", greenFG + "text", 10, greenFG + "text"},
		{"plain text is left alone", "text", 10, "text"},
		{"a full cell needs no padding", tintBG + "tint", 4, tintBG + "tint"},
		{"an over-wide cell needs none either", tintBG + "tinted", 4, tintBG + "tinted"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			testkit.AssertEqual(t, "padUnderBackground", padUnderBackground(tc.cell, tc.width), tc.want)
		})
	}
}
