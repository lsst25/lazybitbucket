package diff

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// wantFile is the file-level summary the table tests assert on.
type wantFile struct {
	oldPath string
	newPath string
	path    string
	status  FileStatus
	binary  bool
	oldMode string
	newMode string
	hunks   int
	added   int
	removed int
}

func assertFiles(t *testing.T, got []File, want []wantFile) {
	t.Helper()

	testkit.AssertEqual(t, "file count", len(got), len(want))
	for index := range want {
		if index >= len(got) {
			return
		}
		file, expected := got[index], want[index]
		label := fmt.Sprintf("files[%d]", index)
		testkit.AssertEqual(t, label+".OldPath", file.OldPath, expected.oldPath)
		testkit.AssertEqual(t, label+".NewPath", file.NewPath, expected.newPath)
		testkit.AssertEqual(t, label+".Path()", file.Path(), expected.path)
		testkit.AssertEqual(t, label+".Status", int(file.Status), int(expected.status))
		testkit.AssertEqual(t, label+".IsBinary", file.IsBinary, expected.binary)
		testkit.AssertEqual(t, label+".OldMode", file.OldMode, expected.oldMode)
		testkit.AssertEqual(t, label+".NewMode", file.NewMode, expected.newMode)
		testkit.AssertEqual(t, label+" hunks", len(file.Hunks), expected.hunks)
		testkit.AssertEqual(t, label+".Added()", file.Added(), expected.added)
		testkit.AssertEqual(t, label+".Removed()", file.Removed(), expected.removed)
	}
}

// wantLine is the per-line detail the line-level tests assert on.
type wantLine struct {
	kind    LineKind
	content string
	oldNo   int
	newNo   int
}

func assertLines(t *testing.T, label string, got []Line, want []wantLine) {
	t.Helper()

	testkit.AssertEqual(t, label+" line count", len(got), len(want))
	for index := range want {
		if index >= len(got) {
			return
		}
		line, expected := got[index], want[index]
		at := fmt.Sprintf("%s[%d]", label, index)
		testkit.AssertEqual(t, at+".Kind", int(line.Kind), int(expected.kind))
		testkit.AssertEqual(t, at+".Content", line.Content, expected.content)
		testkit.AssertEqual(t, at+".OldNo", line.OldNo, expected.oldNo)
		testkit.AssertEqual(t, at+".NewNo", line.NewNo, expected.newNo)
	}
}

func mustParse(t *testing.T, text string) []File {
	t.Helper()

	files, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return files
}

func assertHunk(t *testing.T, label string, got Hunk, oldStart, oldLines, newStart, newLines int, header string) {
	t.Helper()

	testkit.AssertEqual(t, label+".OldStart", got.OldStart, oldStart)
	testkit.AssertEqual(t, label+".OldLines", got.OldLines, oldLines)
	testkit.AssertEqual(t, label+".NewStart", got.NewStart, newStart)
	testkit.AssertEqual(t, label+".NewLines", got.NewLines, newLines)
	testkit.AssertEqual(t, label+".Header", got.Header, header)
}

func TestParse_FileSummaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []wantFile
	}{
		{
			name:  "multi file",
			input: multiFileDiff,
			want: []wantFile{
				{
					oldPath: "pkg/tui/app.go", newPath: "pkg/tui/app.go", path: "pkg/tui/app.go",
					status: Modified, oldMode: modeFile, newMode: modeFile,
					hunks: 2, added: 4, removed: 1,
				},
				{
					newPath: "deploy/staging.yml", path: "deploy/staging.yml",
					status: Added, newMode: modeFile,
					hunks: 1, added: 3,
				},
				{
					oldPath: "docs/OLD.md", path: "docs/OLD.md",
					status: Deleted, oldMode: modeFile,
					hunks: 1, removed: 2,
				},
			},
		},
		{
			name:  "rename with an edit",
			input: renameDiff,
			want: []wantFile{{
				oldPath: "pkg/old/name.go", newPath: "pkg/new/name.go", path: "pkg/new/name.go",
				status: Renamed, oldMode: modeFile, newMode: modeFile,
				hunks: 1, added: 1, removed: 1,
			}},
		},
		{
			name:  "pure copy",
			input: copyDiff,
			want: []wantFile{{
				oldPath: "a.txt", newPath: "b.txt", path: "b.txt", status: Copied,
			}},
		},
		{
			name:  "mode change only",
			input: modeDiff,
			want: []wantFile{{
				oldPath: "scripts/run.sh", newPath: "scripts/run.sh", path: "scripts/run.sh",
				status: Modified, oldMode: modeFile, newMode: modeExec,
			}},
		},
		{
			name:  "binary sections",
			input: binaryDiff,
			want: []wantFile{
				{
					oldPath: "assets/logo.png", newPath: "assets/logo.png", path: "assets/logo.png",
					status: Modified, binary: true, oldMode: modeFile, newMode: modeFile,
				},
				{
					newPath: "assets/new.png", path: "assets/new.png",
					status: Added, binary: true, newMode: modeFile,
				},
				{
					newPath: "assets/icon.ico", path: "assets/icon.ico",
					status: Added, binary: true, newMode: modeFile,
				},
				{
					oldPath: "assets/gone.png", path: "assets/gone.png",
					status: Deleted, binary: true, oldMode: modeFile,
				},
			},
		},
		{
			name:  "quoted paths",
			input: quotedPathDiff,
			want: []wantFile{
				{
					oldPath: quotedPath, newPath: quotedPath, path: quotedPath,
					status: Modified, oldMode: modeFile, newMode: modeFile,
					hunks: 1, added: 1, removed: 1,
				},
				{
					oldPath: "docs/café.md", newPath: "docs/café.md", path: "docs/café.md",
					status: Modified, oldMode: modeFile, newMode: modeFile,
					hunks: 1, added: 1, removed: 1,
				},
			},
		},
		{
			name:  "unquoted path with a space",
			input: spacedPathDiff,
			want: []wantFile{{
				oldPath: quotedPath, newPath: quotedPath, path: quotedPath,
				status: Modified, oldMode: modeFile, newMode: modeFile,
				hunks: 1, added: 1, removed: 1,
			}},
		},
		{
			name:  "trailing garbage after the last hunk",
			input: trailingGarbageDiff,
			want: []wantFile{{
				oldPath: "x.txt", newPath: "x.txt", path: "x.txt",
				status: Modified, oldMode: modeFile, newMode: modeFile,
				hunks: 1, added: 1, removed: 1,
			}},
		},
		{
			name:  "truncated final hunk is kept",
			input: truncatedDiff,
			want: []wantFile{{
				oldPath: "big.txt", newPath: "big.txt", path: "big.txt",
				status: Modified, oldMode: modeFile, newMode: modeFile,
				hunks: 1, added: 1,
			}},
		},
		{
			name:  "diff of a patch file",
			input: patchOfPatchDiff,
			want: []wantFile{{
				oldPath: "patches/fix.patch", newPath: "patches/fix.patch", path: "patches/fix.patch",
				status: Modified, oldMode: modeFile, newMode: modeFile,
				hunks: 1, added: 1, removed: 1,
			}},
		},
		{
			name:  "no newline at end of file",
			input: noNewlineDiff,
			want: []wantFile{{
				oldPath: "config.env", newPath: "config.env", path: "config.env",
				status: Modified, oldMode: modeFile, newMode: modeFile,
				hunks: 1, added: 1, removed: 1,
			}},
		},
		{
			name:  "no file section at all",
			input: garbageOnly,
			want:  nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assertFiles(t, mustParse(t, test.input), test.want)
		})
	}
}

func TestParse_HunkRangesAndHeaders(t *testing.T) {
	t.Parallel()

	files := mustParse(t, multiFileDiff)
	if len(files) != 3 {
		t.Fatalf("files = %d, want 3", len(files))
	}

	assertHunk(t, "app.go hunk 0", files[0].Hunks[0], 10, 6, 10, 8, "func main() {")
	assertHunk(t, "app.go hunk 1", files[0].Hunks[1], 40, 3, 42, 4, "func run(ctx context.Context) error {")
	assertHunk(t, "staging.yml hunk 0", files[1].Hunks[0], 0, 0, 1, 3, "")
	assertHunk(t, "OLD.md hunk 0", files[2].Hunks[0], 1, 2, 0, 0, "")
}

func TestParse_LineKindsAndNumbers(t *testing.T) {
	t.Parallel()

	files := mustParse(t, lineNumbersDiff)
	if len(files) != 1 || len(files[0].Hunks) != 1 {
		t.Fatalf("got %d files, first with %d hunks; want 1 and 1", len(files), len(files[0].Hunks))
	}

	assertHunk(t, "hunk", files[0].Hunks[0], 10, 3, 10, 4, "func main() {")
	assertLines(t, "hunk", files[0].Hunks[0].Lines, []wantLine{
		{kind: LineContext, content: "alpha", oldNo: 10, newNo: 10},
		{kind: LineRemoved, content: "bravo", oldNo: 11},
		{kind: LineAdded, content: "charlie", newNo: 11},
		{kind: LineAdded, content: "delta", newNo: 12},
		{kind: LineContext, content: "echo", oldNo: 12, newNo: 13},
	})
}

func TestParse_AddedAndDeletedFileLineNumbers(t *testing.T) {
	t.Parallel()

	files := mustParse(t, multiFileDiff)

	assertLines(t, "staging.yml", files[1].Hunks[0].Lines, []wantLine{
		{kind: LineAdded, content: "replicas: 2", newNo: 1},
		{kind: LineAdded, content: "image: api-gateway:latest", newNo: 2},
		{kind: LineAdded, content: "region: eu-west-1", newNo: 3},
	})
	assertLines(t, "OLD.md", files[2].Hunks[0].Lines, []wantLine{
		{kind: LineRemoved, content: "# Old", oldNo: 1},
		{kind: LineRemoved, content: "Gone.", oldNo: 2},
	})
}

func TestParse_NoNewlineMarkers(t *testing.T) {
	t.Parallel()

	files := mustParse(t, noNewlineDiff)

	assertLines(t, "config.env", files[0].Hunks[0].Lines, []wantLine{
		{kind: LineContext, content: "KEY=value", oldNo: 1, newNo: 1},
		{kind: LineRemoved, content: "TOKEN=old", oldNo: 2},
		{kind: LineNoNewline, content: noNewlineText},
		{kind: LineAdded, content: "TOKEN=new", newNo: 2},
		{kind: LineNoNewline, content: noNewlineText},
	})
}

func TestParse_PatchOfPatchKeepsMarkersAsContent(t *testing.T) {
	t.Parallel()

	files := mustParse(t, patchOfPatchDiff)

	assertLines(t, "fix.patch", files[0].Hunks[0].Lines, []wantLine{
		{kind: LineContext, content: "diff --git a/README.md b/README.md", oldNo: 1, newNo: 1},
		{kind: LineRemoved, content: "-- old", oldNo: 2},
		{kind: LineAdded, content: "++ new", newNo: 2},
		{kind: LineContext, content: "tail", oldNo: 3, newNo: 3},
	})
}

func TestParse_BitbucketLiveExcerpt(t *testing.T) {
	t.Parallel()

	files := mustParse(t, bitbucketDiff)
	const path = "packages/editor/conversation/src/components/Editor.tsx"
	assertFiles(t, files, []wantFile{{
		oldPath: path, newPath: path, path: path,
		status: Modified, oldMode: modeFile, newMode: modeFile,
		hunks: 1, added: 2, removed: 2,
	}})

	hunk := files[0].Hunks[0]
	assertHunk(t, "Editor.tsx hunk", hunk, 119, 7, 119, 7,
		"export default class Editor extends React.Component<Props, State> {")
	assertLines(t, "Editor.tsx hunk", hunk.Lines, []wantLine{
		{kind: LineContext, content: "    const { isExpanded } = this.state;", oldNo: 119, newNo: 119},
		{kind: LineContext, content: "    return (", oldNo: 120, newNo: 120},
		{kind: LineContext, content: "      <div>", oldNo: 121, newNo: 121},
		{kind: LineRemoved, content: "        <Comment {...props} />", oldNo: 122},
		{kind: LineRemoved, content: "        <Footer />", oldNo: 123},
		{kind: LineAdded, content: "        <Comment {...props} key={id} />", newNo: 122},
		{kind: LineAdded, content: "        <Footer compact />", newNo: 123},
		{kind: LineContext, content: "      </div>", oldNo: 124, newNo: 124},
		{kind: LineContext, content: "    );", oldNo: 125, newNo: 125},
	})
}

func TestParse_CRLFLineEndings(t *testing.T) {
	t.Parallel()

	input := strings.ReplaceAll(lineNumbersDiff, "\n", "\r\n")
	files := mustParse(t, input)

	assertFiles(t, files, []wantFile{{
		oldPath: "main.go", newPath: "main.go", path: "main.go",
		status: Modified, oldMode: modeFile, newMode: modeFile,
		hunks: 1, added: 2, removed: 1,
	}})
	assertLines(t, "main.go", files[0].Hunks[0].Lines, []wantLine{
		{kind: LineContext, content: "alpha", oldNo: 10, newNo: 10},
		{kind: LineRemoved, content: "bravo", oldNo: 11},
		{kind: LineAdded, content: "charlie", newNo: 11},
		{kind: LineAdded, content: "delta", newNo: 12},
		{kind: LineContext, content: "echo", oldNo: 12, newNo: 13},
	})
}

func TestParse_PlainUnifiedDiffWithoutGitHeaders(t *testing.T) {
	t.Parallel()

	input := "--- old/a.txt\t2026-01-01 10:00:00.000000000 +0100\n" +
		"+++ new/a.txt\t2026-01-02 11:00:00.000000000 +0100\n" +
		"@@ -1,2 +1,2 @@\n" +
		"-one\n" +
		"+uno\n" +
		" two\n" +
		"--- old/b.txt\t2026-01-01 10:00:00.000000000 +0100\n" +
		"+++ new/b.txt\t2026-01-02 11:00:00.000000000 +0100\n" +
		"@@ -1 +1 @@\n" +
		"-x\n" +
		"+y\n"

	assertFiles(t, mustParse(t, input), []wantFile{
		{
			oldPath: "old/a.txt", newPath: "new/a.txt", path: "new/a.txt",
			status: Modified, hunks: 1, added: 1, removed: 1,
		},
		{
			oldPath: "old/b.txt", newPath: "new/b.txt", path: "new/b.txt",
			status: Modified, hunks: 1, added: 1, removed: 1,
		},
	})
}

func TestParse_TabsAndInvalidUTF8PassThrough(t *testing.T) {
	t.Parallel()

	input := "diff --git a/x.go b/x.go\n" +
		"--- a/x.go\n" +
		"+++ b/x.go\n" +
		"@@ -1,2 +1,2 @@\n" +
		"-\tif old {  \n" +
		"+\tif new {\n" +
		" \tlatin1: \xff\xfe\n"

	files := mustParse(t, input)
	assertLines(t, "x.go", files[0].Hunks[0].Lines, []wantLine{
		{kind: LineRemoved, content: "\tif old {  ", oldNo: 1},
		{kind: LineAdded, content: "\tif new {", newNo: 1},
		{kind: LineContext, content: "\tlatin1: \xff\xfe", oldNo: 2, newNo: 2},
	})
}

func TestParse_EmptyContextLineWithoutMarker(t *testing.T) {
	t.Parallel()

	// A transport that strips trailing whitespace leaves an empty context line
	// with no marker at all.
	input := "diff --git a/x.txt b/x.txt\n" +
		"--- a/x.txt\n" +
		"+++ b/x.txt\n" +
		"@@ -1,3 +1,3 @@\n" +
		" one\n" +
		"\n" +
		"-two\n" +
		"+three\n"

	files := mustParse(t, input)
	assertLines(t, "x.txt", files[0].Hunks[0].Lines, []wantLine{
		{kind: LineContext, content: "one", oldNo: 1, newNo: 1},
		{kind: LineContext, content: "", oldNo: 2, newNo: 2},
		{kind: LineRemoved, content: "two", oldNo: 3},
		{kind: LineAdded, content: "three", newNo: 3},
	})
}

func TestParse_LenientHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    []wantFile
		wantSum string
	}{
		{
			name:  "hunk header without a closing marker",
			input: "diff --git a/x.txt b/x.txt\n@@ -3,2 +3,2\n-a\n+b\n",
			want: []wantFile{{
				oldPath: "x.txt", newPath: "x.txt", path: "x.txt",
				status: Modified, hunks: 1, added: 1, removed: 1,
			}},
			wantSum: "3,2 3,2 ",
		},
		{
			name:  "hunk header with unparsable numbers",
			input: "diff --git a/x.txt b/x.txt\n@@ -x,y +z,w @@ ctx\n a\n",
			want: []wantFile{{
				oldPath: "x.txt", newPath: "x.txt", path: "x.txt",
				status: Modified, hunks: 1,
			}},
			wantSum: "0,0 0,0 ctx",
		},
		{
			name:  "single line hunk without counts",
			input: "diff --git a/x.txt b/x.txt\n@@ -7 +7 @@ ctx\n-a\n+b\n",
			want: []wantFile{{
				oldPath: "x.txt", newPath: "x.txt", path: "x.txt",
				status: Modified, hunks: 1, added: 1, removed: 1,
			}},
			wantSum: "7,1 7,1 ctx",
		},
		{
			name:  "git header without a second path",
			input: "diff --git a/only.txt\n@@ -1 +1 @@\n-a\n+b\n",
			want: []wantFile{{
				oldPath: "only.txt", path: "only.txt",
				status: Modified, hunks: 1, added: 1, removed: 1,
			}},
			wantSum: "1,1 1,1 ",
		},
		{
			name:  "git header without side prefixes",
			input: "diff --git plain.txt plain.txt\n@@ -1 +1 @@\n-a\n+b\n",
			want: []wantFile{{
				oldPath: "plain.txt", newPath: "plain.txt", path: "plain.txt",
				status: Modified, hunks: 1, added: 1, removed: 1,
			}},
			wantSum: "1,1 1,1 ",
		},
		{
			name:  "git header with an unclosed quote",
			input: "diff --git \"a/x b/x\n@@ -1 +1 @@\n-a\n+b\n",
			want: []wantFile{{
				oldPath: "\"a/x", newPath: "x", path: "x",
				status: Modified, hunks: 1, added: 1, removed: 1,
			}},
			wantSum: "1,1 1,1 ",
		},
		{
			name:  "old path header at the very end",
			input: "diff --git a/x.txt b/x.txt\n--- a/x.txt\n",
			want: []wantFile{{
				oldPath: "x.txt", newPath: "x.txt", path: "x.txt", status: Modified,
			}},
		},
		{
			name:  "index line without a mode",
			input: "diff --git a/x.txt b/x.txt\nindex 1111111..2222222\n",
			want: []wantFile{{
				oldPath: "x.txt", newPath: "x.txt", path: "x.txt", status: Modified,
			}},
		},
		{
			name:  "binary marker without paths to take",
			input: "Binary files a and b differ\n",
			want:  nil,
		},
		{
			name:  "binary marker naming nothing",
			input: "diff --git a/x.bin b/x.bin\nBinary files unknown differ\n",
			want: []wantFile{{
				oldPath: "x.bin", newPath: "x.bin", path: "x.bin",
				status: Modified, binary: true,
			}},
		},
		{
			name:  "new path header without an old one",
			input: "diff --git a/x.txt b/x.txt\n+++ b/renamed.txt\n",
			want: []wantFile{{
				oldPath: "x.txt", newPath: "renamed.txt", path: "renamed.txt", status: Modified,
			}},
		},
		{
			name:    "added file known only from /dev/null",
			input:   "--- /dev/null\n+++ b/new.txt\n@@ -0,0 +1 @@\n+hi\n",
			want:    []wantFile{{newPath: "new.txt", path: "new.txt", status: Added, hunks: 1, added: 1}},
			wantSum: "0,0 1,1 ",
		},
		{
			name:    "deleted file known only from /dev/null",
			input:   "--- a/gone.txt\n+++ /dev/null\n@@ -1 +0,0 @@\n-bye\n",
			want:    []wantFile{{oldPath: "gone.txt", path: "gone.txt", status: Deleted, hunks: 1, removed: 1}},
			wantSum: "1,1 0,0 ",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			files := mustParse(t, test.input)
			assertFiles(t, files, test.want)
			if test.wantSum == "" {
				return
			}
			hunk := files[0].Hunks[0]
			got := fmt.Sprintf("%d,%d %d,%d %s",
				hunk.OldStart, hunk.OldLines, hunk.NewStart, hunk.NewLines, hunk.Header)
			testkit.AssertEqual(t, "hunk summary", got, test.wantSum)
		})
	}
}

func TestParse_EmptyInput(t *testing.T) {
	t.Parallel()

	files, err := Parse("")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	testkit.AssertEqual(t, "file count", len(files), 0)
}

func TestParse_HunkBeforeAnyFileHeader(t *testing.T) {
	t.Parallel()

	files, err := Parse(hunkFirstDiff)
	if !errors.Is(err, errHunkBeforeFile) {
		t.Fatalf("Parse error = %v, want errHunkBeforeFile", err)
	}
	if files != nil {
		t.Errorf("files = %v, want nil", files)
	}
	if !strings.Contains(err.Error(), "diff line 1") {
		t.Errorf("error %q does not name the offending line", err.Error())
	}
}
