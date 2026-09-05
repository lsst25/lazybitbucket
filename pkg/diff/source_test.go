package diff

import (
	"strings"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// sourceIndexDiff is one document holding two files, the first with two hunks
// and the second with a removed and an added line that each end without a
// newline. Line.Source indexes this whole document, not each file, which is
// what a per-line rendering of the same text has to be looked up by.
const sourceIndexDiff = "diff --git a/a.txt b/a.txt\n" + // 0
	"index 1111111..2222222 100644\n" + // 1
	"--- a/a.txt\n" + // 2
	"+++ b/a.txt\n" + // 3
	"@@ -1,3 +1,3 @@\n" + // 4
	" one\n" + // 5
	"-two\n" + // 6
	"+TWO\n" + // 7
	" three\n" + // 8
	"@@ -10,2 +10,3 @@ func tail()\n" + // 9
	" ten\n" + // 10
	"+eleven\n" + // 11
	" twelve\n" + // 12
	"diff --git a/b.txt b/b.txt\n" + // 13
	"index 3333333..4444444 100644\n" + // 14
	"--- a/b.txt\n" + // 15
	"+++ b/b.txt\n" + // 16
	"@@ -1,2 +1,2 @@\n" + // 17
	" keep\n" + // 18
	"-gone\n" + // 19
	"\\ " + noNewlineText + "\n" + // 20
	"+added\n" + // 21
	"\\ " + noNewlineText + "\n" // 22

func TestParse_LineSourceIndexesTheWholeDocument(t *testing.T) {
	t.Parallel()

	files := mustParse(t, sourceIndexDiff)
	testkit.AssertEqual(t, "file count", len(files), 2)

	tests := []struct {
		label string
		lines []Line
		want  []int
	}{
		{"a.txt hunk 1", files[0].Hunks[0].Lines, []int{5, 6, 7, 8}},
		{"a.txt hunk 2", files[0].Hunks[1].Lines, []int{10, 11, 12}},
		{"b.txt hunk 1", files[1].Hunks[0].Lines, []int{18, 19, 20, 21, 22}},
	}
	for _, tc := range tests {
		testkit.AssertEqual(t, tc.label+" line count", len(tc.lines), len(tc.want))
		for i, line := range tc.lines {
			testkit.AssertEqual(t, tc.label+" line "+line.Content+" Source", line.Source, tc.want[i])
		}
	}
}

// TestParse_LineSourcePointsAtTheLineItWasReadFrom is the property the index is
// for: rendering the same document with a per-line pass and looking a parsed
// line's cell up by its Source has to give back that line's own text.
func TestParse_LineSourcePointsAtTheLineItWasReadFrom(t *testing.T) {
	t.Parallel()

	raw := strings.Split(strings.TrimSuffix(sourceIndexDiff, "\n"), "\n")
	files := mustParse(t, sourceIndexDiff)

	for _, file := range files {
		for _, hunk := range file.Hunks {
			for _, line := range hunk.Lines {
				if line.Source < 0 || line.Source >= len(raw) {
					t.Fatalf("%q: Source %d is outside the %d-line document", line.Content, line.Source, len(raw))
				}
				source := raw[line.Source]
				if line.Kind == LineNoNewline {
					testkit.AssertEqual(t, "no-newline marker", source, "\\ "+line.Content)
					continue
				}
				testkit.AssertEqual(t, "line "+line.Content, source[1:], line.Content)
			}
		}
	}
}

func TestParse_LineSourceForAnEmptyContextLine(t *testing.T) {
	t.Parallel()

	// A transport that strips trailing whitespace leaves the context line with
	// no marker at all; it still has to name the line it came from.
	files := mustParse(t, "--- a/x.txt\n+++ b/x.txt\n@@ -1,2 +1,2 @@\n\n+added\n")

	assertLines(t, "x.txt", files[0].Hunks[0].Lines, []wantLine{
		{kind: LineContext, content: "", oldNo: 1, newNo: 1},
		{kind: LineAdded, content: "added", newNo: 2},
	})
	testkit.AssertEqual(t, "empty context Source", files[0].Hunks[0].Lines[0].Source, 3)
	testkit.AssertEqual(t, "added Source", files[0].Hunks[0].Lines[1].Source, 4)
}
