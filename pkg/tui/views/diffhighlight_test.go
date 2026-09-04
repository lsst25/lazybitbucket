package views

import (
	"strings"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/diff"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// escape is the byte every ANSI sequence starts with; the highlighter tests ask
// only whether colour was emitted, not which colour.
const escape = "\x1b"

func TestChromaStyleFor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		glamour string
		want    string
	}{
		{glamourStyleDark, chromaStyleDark},
		{glamourStyleLight, chromaStyleLight},
		{glamourStyleNoTTY, ""},
		{"", ""},
		{"ascii", ""},
	}
	for _, tc := range tests {
		t.Run(tc.glamour, func(t *testing.T) {
			t.Parallel()
			testkit.AssertEqual(t, "ChromaStyleFor", ChromaStyleFor(tc.glamour), tc.want)
		})
	}
}

func TestNewHighlighter_OffWithoutAStyle(t *testing.T) {
	t.Parallel()
	if got := newHighlighter(testDiffPath, "package main", ""); got != nil {
		t.Errorf("newHighlighter with no style = %v, want nil", got)
	}
	// A nil highlighter is the identity function rather than a panic.
	var off *highlighter
	testkit.AssertEqual(t, "nil.Line", off.Line("func main() {"), "func main() {")
}

func TestHighlighter_ColoursKeywords(t *testing.T) {
	t.Parallel()
	got := newHighlighter(testDiffPath, "", chromaStyleDark).Line("func main() {")

	if !strings.Contains(got, escape) {
		t.Errorf("Line() = %q, want ANSI colour", got)
	}
	if stripANSI(got) != "func main() {" {
		t.Errorf("stripANSI(Line()) = %q, want the line unchanged", stripANSI(got))
	}
}

func TestHighlighter_FallsBackWhenTheExtensionIsUnknown(t *testing.T) {
	t.Parallel()
	// No lexer matches ".unknownext", so the content is sniffed instead.
	got := newHighlighter("script.unknownext", "#!/bin/bash\nfor f in *; do echo \"$f\"; done\n",
		chromaStyleLight).Line("for f in *; do")

	if !strings.Contains(got, escape) {
		t.Errorf("Line() = %q, want the sniffed lexer to colour it", got)
	}
}

func TestHighlighter_SkipsVeryLongLines(t *testing.T) {
	t.Parallel()
	highlight := newHighlighter(testDiffPath, "", chromaStyleDark)

	long := "var x = \"" + strings.Repeat("y", maxHighlightBytes) + "\""
	testkit.AssertEqual(t, "long line is passed through", highlight.Line(long), long)

	short := "var x = 1"
	if !strings.Contains(highlight.Line(short), escape) {
		t.Error("a short line should still be highlighted")
	}
	testkit.AssertEqual(t, "empty line", highlight.Line(""), "")
}

func TestHighlighter_NeverEmitsANewline(t *testing.T) {
	t.Parallel()
	// A newline inside a rendered row would break the panel's line accounting.
	got := newHighlighter(testDiffPath, "", chromaStyleDark).Line("\tif err != nil {")
	if strings.Contains(got, "\n") {
		t.Errorf("Line() = %q, want no newline", got)
	}
}

func TestDetailView_FileDiff_HighlightingFollowsTheCodeStyle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		codeStyle string
		wantANSI  bool
	}{
		{"monokai highlights", chromaStyleDark, true},
		{"empty style leaves the code plain", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			view := NewDetailView(PlainRenderer{}, tc.codeStyle)
			view.SetSize(80, 24)
			view.SetFocused(true)
			view.SetPullRequest(detailPR())
			view.SetFileDiffLoading(testPRKey, testDiffPath)
			view.SetFileDiff(testPRKey, testDiffPath, &diff.File{
				NewPath: testDiffPath,
				Hunks: []diff.Hunk{{
					OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 2,
					Lines: []diff.Line{
						{Kind: diff.LineContext, Content: "func main() {", OldNo: 1, NewNo: 1},
						{Kind: diff.LineAdded, Content: "\tdefer stop()", NewNo: 2},
					},
				}},
			}, nil)

			out := view.View()
			// The context row is the one that is only ever coloured by the
			// highlighter: added and removed rows are red or green regardless.
			row := ""
			for _, line := range strings.Split(out, "\n") {
				if strings.Contains(stripANSI(line), "func main() {") {
					row = line
				}
			}
			if row == "" {
				t.Fatalf("no context row in:\n%s", stripANSI(out))
			}
			// Strip the gray gutter, which is styled either way.
			code := row[strings.Index(row, "│")+len("│"):]
			if got := strings.Contains(code, escape); got != tc.wantANSI {
				t.Errorf("code carries ANSI = %v, want %v: %q", got, tc.wantANSI, code)
			}
		})
	}
}
