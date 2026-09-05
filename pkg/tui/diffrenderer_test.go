package tui

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/config"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// colourEachLine is a stand-in for delta --color-only: it opens every line it
// reads with a green SGR and closes it with the erase-in-line sequence delta
// paints a row's tint to the edge of the terminal with, and prints exactly one
// line per line it read.
const colourEachLine = `while IFS= read -r line; do printf '\033[32m%s\033[0K\n' "$line"; done
`

// The escape sequences the stand-in emits, as the assertions read them, plus
// the two zero-width shapes a shell renderer written with `tput` adds to them:
// the reset an xterm terminfo opens with (ESC ( B, "designate ASCII as G0")
// and the shift-in a tmux or screen terminfo closes with.
const (
	greenSGR   = "\x1b[32m"
	eraseEOL   = "\x1b[0K"
	terminfoG0 = "\x1b(B"
	shiftIn    = "\x0f"
)

// rendererInput is the diff document handed to the stand-in: a file header, a
// hunk header and two content lines, one context and one added, both indented
// with a tab so the expansion is visible.
const rendererInput = "diff --git a/a.txt b/a.txt\n" +
	"--- a/a.txt\n" +
	"+++ b/a.txt\n" +
	"@@ -1,2 +1,2 @@\n" +
	" \tkeep\n" +
	"+\tadded\n"

// missingRenderer is a program name that is not on $PATH, so exec resolves it
// to exec.ErrNotFound rather than trying to run anything.
const missingRenderer = "lazybitbucket-no-such-diff-renderer"

// backgroundChild opens a stand-in with a child that inherits its standard
// output and outlives it by far. It is the shape a wrapper script has — a
// shell that runs delta without exec — and the one the run has to survive:
// the deadline reaches the program, never the child holding the pipe, so the
// wait ends on the WaitDelay or not at all.
const backgroundChild = "sleep 20 &\n"

// rendererPatience is the outside time a run that has to end early may take.
// It is far under the stand-in child's 20 seconds and far over the second the
// WaitDelay costs, so a run that waits for the child fails the assertion on
// any machine.
const rendererPatience = 10 * time.Second

// fakeRenderer writes a stand-in diff renderer — a shell script with the given
// body — into a temp directory and returns its path. It is fakeTmux for the
// diffRenderer key: no test may run a real delta.
func fakeRenderer(t *testing.T, body string) string {
	t.Helper()
	script := filepath.Join(t.TempDir(), "renderer")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

func TestRenderDiffLines_ColoursEveryLineAndKeepsTheAlignment(t *testing.T) {
	t.Parallel()
	script := fakeRenderer(t, colourEachLine)

	lines, err := renderDiffLines(t.Context(), script, nil, rendererInput)

	if err != nil {
		t.Fatalf("renderDiffLines: %v", err)
	}
	testkit.AssertEqual(t, "one cell per input line", len(lines), 6)
	// The marker column is the view's own, so the leading ' ' and '+' are gone
	// from the cell while the colour in front of them stays, and the tab is
	// expanded to the same stop the built-in rendering uses.
	testkit.AssertEqual(t, "context cell", lines[4], greenSGR+"    keep")
	testkit.AssertEqual(t, "added cell", lines[5], greenSGR+"    added")
	// A line that does not open with a diff marker keeps its first character.
	testkit.AssertEqual(t, "header cell", lines[0], greenSGR+"diff --git a/a.txt b/a.txt")
	testkit.AssertEqual(t, "old path cell", lines[1], greenSGR+"-- a/a.txt")

	for i, line := range lines {
		if strings.Contains(line, eraseEOL) {
			t.Errorf("cell %d still carries the erase-in-line sequence: %q", i, line)
		}
	}
}

func TestDiffRendererCell(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain context line", " code", "code"},
		{"coloured added line", greenSGR + "+\tcode" + eraseEOL, greenSGR + "    code"},
		{"a header keeps its first character", "@@ -1,2 +1,2 @@", "@@ -1,2 +1,2 @@"},
		{"a carriage return cannot reach the terminal", greenSGR + " code\r", greenSGR + "code"},
		{
			"a line a tput renderer reset first",
			terminfoG0 + "\x1b[m" + greenSGR + "+\tcode" + eraseEOL,
			terminfoG0 + "\x1b[m" + greenSGR + "    code",
		},
		{"empty", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			testkit.AssertEqual(t, "diffRendererCell", diffRendererCell(tc.in), tc.want)
		})
	}
}

func TestRenderDiffLines_RejectsOutputThatDoesNotLineUp(t *testing.T) {
	t.Parallel()
	script := fakeRenderer(t, colourEachLine+"printf 'one line too many\\n'\n")

	_, err := renderDiffLines(t.Context(), script, nil, rendererInput)

	if !errors.Is(err, errDiffRendererMisaligned) {
		t.Fatalf("err = %v, want a misalignment", err)
	}
	for _, want := range []string{"7 lines", "6 lines", "--color-only"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not mention %q: %v", want, err)
		}
	}
}

func TestRenderDiffLines_CarriesTheProgramsOwnComplaint(t *testing.T) {
	t.Parallel()
	script := fakeRenderer(t, "echo 'unknown option --colour-only' >&2\necho 'usage: delta' >&2\nexit 2\n")

	_, err := renderDiffLines(t.Context(), script, []string{"--colour-only"}, rendererInput)

	if err == nil {
		t.Fatal("a non-zero exit must be an error")
	}
	if !strings.Contains(err.Error(), "unknown option --colour-only") {
		t.Errorf("the error should carry the first line of stderr, got %v", err)
	}
	if strings.Contains(err.Error(), "usage: delta") {
		t.Errorf("only the first line of stderr belongs on the status panel, got %v", err)
	}
	if errors.Is(err, errDiffRendererMisaligned) {
		t.Error("a non-zero exit is not a misalignment; it must not disable the renderer")
	}
}

func TestRenderDiffLines_ReportsAProgramThatIsNotInstalled(t *testing.T) {
	t.Parallel()

	_, err := renderDiffLines(t.Context(), missingRenderer, nil, rendererInput)

	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("err = %v, want exec.ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), missingRenderer) {
		t.Errorf("the error should name the program, got %v", err)
	}
}

func TestRenderDiffLines_EmptyDocumentRunsNothing(t *testing.T) {
	t.Parallel()

	lines, err := renderDiffLines(t.Context(), missingRenderer, nil, "")

	if err != nil {
		t.Fatalf("renderDiffLines on an empty document: %v", err)
	}
	if lines != nil {
		t.Errorf("lines = %q, want nil", lines)
	}
}

func TestRenderDiffLines_ReportsAPathThatNamesNoProgram(t *testing.T) {
	t.Parallel()
	// exec only searches $PATH for a bare name, so `~/bin/delta` — nothing
	// expands the tilde — or a path carried over from another machine fails at
	// fork/exec instead of the lookup. Same misconfiguration, different error,
	// and both have to condemn the renderer for the session.
	missing := filepath.Join(t.TempDir(), missingRenderer)

	_, err := renderDiffLines(t.Context(), missing, nil, rendererInput)

	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("err = %v, want fs.ErrNotExist", err)
	}
	if !rendererIsHopeless(err) {
		t.Error("a program that is not there must turn the renderer off for the session")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("the error should name the program, got %v", err)
	}
}

func TestRenderDiffLines_AProgramThatLeftAChildBehindStillRenders(t *testing.T) {
	t.Parallel()
	script := fakeRenderer(t, backgroundChild+colourEachLine)

	start := time.Now()
	lines, err := renderDiffLines(t.Context(), script, nil, rendererInput)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("renderDiffLines: %v", err)
	}
	testkit.AssertEqual(t, "one cell per input line", len(lines), 6)
	testkit.AssertEqual(t, "context cell", lines[4], greenSGR+"    keep")
	if elapsed > rendererPatience {
		t.Errorf("the run took %s: it waited on the child rather than on the program", elapsed)
	}
}

func TestRenderDiffLines_TheDeadlineHoldsWhenAChildHoldsTheOutput(t *testing.T) {
	t.Parallel()
	// The program leaves a child holding its standard output and then hands its
	// own process over to something that never returns. The deadline kills the
	// program, but the pipe stays open behind it, which is what would leave the
	// pane on its loading placeholder for as long as the child lives.
	script := fakeRenderer(t, backgroundChild+"exec sleep 20\n")
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := renderDiffLines(ctx, script, nil, rendererInput)
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the deadline", err)
	}
	if rendererIsHopeless(err) {
		t.Error("a timeout costs one file, not the session")
	}
	if elapsed > rendererPatience {
		t.Errorf("the run took %s: the deadline did not bound it", elapsed)
	}
}

// -----------------------------------------------------------------------------
// The pure post-processing helpers
// -----------------------------------------------------------------------------

func TestStripEraseInLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"nothing to do", "plain text", "plain text"},
		{"trailing erase", "code" + eraseEOL, "code"},
		{"bare erase", "code\x1b[K", "code"},
		{"every parameter", "a\x1b[0Kb\x1b[1Kc\x1b[2Kd", "abcd"},
		{"in the middle", greenSGR + eraseEOL + "code", greenSGR + "code"},
		{"colours survive", greenSGR + "code\x1b[0m" + eraseEOL, greenSGR + "code\x1b[0m"},
		{"an unterminated sequence is left alone", "code\x1b[", "code\x1b["},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			testkit.AssertEqual(t, "stripEraseInLine", stripEraseInLine(tc.in), tc.want)
		})
	}
}

func TestDropDiffMarker(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"added", "+code", "code"},
		{"removed", "-code", "code"},
		{"context", " code", "code"},
		{"no newline marker", `\ No newline at end of file`, ` No newline at end of file`},
		{"behind a colour", greenSGR + "+code", greenSGR + "code"},
		{"behind two sequences", greenSGR + "\x1b[1m code", greenSGR + "\x1b[1mcode"},
		{"behind an OSC", "\x1b]8;;https://x\x07+code", "\x1b]8;;https://x\x07code"},
		// `tput sgr0` wraps its reset in bytes that cost no column, and none of
		// them is the marker.
		{
			"behind a terminfo reset",
			terminfoG0 + "\x1b[m" + greenSGR + "+code",
			terminfoG0 + "\x1b[m" + greenSGR + "code",
		},
		{"behind a shift-in", "\x1b[m" + shiftIn + "-code", "\x1b[m" + shiftIn + "code"},
		{"not a marker", "diff --git a/x b/x", "diff --git a/x b/x"},
		{"empty", "", ""},
		{"colour only", greenSGR, greenSGR},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			testkit.AssertEqual(t, "dropDiffMarker", dropDiffMarker(tc.in), tc.want)
		})
	}
}

func TestExpandTabsANSI(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"nothing to do", "code", "code"},
		{"leading tab", "\tcode", "    code"},
		{"partial stop", "ab\tcd", "ab  cd"},
		{"two tabs", "\t\tx", "        x"},
		// The colour costs no columns, so the stop is the same as without it.
		{"a colour does not move the stop", greenSGR + "ab\tcd", greenSGR + "ab  cd"},
		{"a colour between the columns", "a" + greenSGR + "b\tcd", "a" + greenSGR + "b  cd"},
		{"a terminfo reset does not move the stop", terminfoG0 + "ab\tcd", terminfoG0 + "ab  cd"},
		{"a shift-in does not move the stop", shiftIn + "ab\tcd", shiftIn + "ab  cd"},
		{"wide runes count as two columns", "日\tx", "日  x"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			testkit.AssertEqual(t, "expandTabsANSI", expandTabsANSI(tc.in, 4), tc.want)
		})
	}
}

func TestAnsiSequenceLen(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"not a sequence", "code", 0},
		{"sgr", greenSGR, len(greenSGR)},
		{"erase", eraseEOL, len(eraseEOL)},
		{"csi with no parameters", "\x1b[m", 3},
		{"osc closed with bel", "\x1b]8;;https://x\x07", len("\x1b]8;;https://x\x07")},
		{"osc closed with st", "\x1b]8;;x\x1b\\", len("\x1b]8;;x\x1b\\")},
		// ESC ( B is three bytes, not two: `tput sgr0` on an xterm terminfo
		// opens its reset with it, and reading the final 'B' as text would
		// make it the line's first visible rune.
		{"nf escape", terminfoG0, 3},
		{"nf escape with another intermediate", "\x1b#8", 3},
		{"unterminated nf escape", "\x1b(", 2},
		{"nf intermediates broken off by a control", "\x1b(\x0f", 2},
		{"esc plus a final byte", "\x1bM", 2},
		{"lone escape", "\x1b", 1},
		{"unterminated csi swallows the rest", "\x1b[32", 4},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			testkit.AssertEqual(t, "ansiSequenceLen", ansiSequenceLen(tc.in, 0), tc.want)
		})
	}
}

// -----------------------------------------------------------------------------
// The app wiring
// -----------------------------------------------------------------------------

func TestDiffRendererCommand_WhenItRuns(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		value    string
		style    string
		disabled bool
		wantOK   bool
	}{
		{"unset", "", "", false, false},
		{"configured", "delta --color-only", "", false, true},
		{"notty cannot show colour", "delta --color-only", config.RendererStyleNoTTY, false, false},
		{"disabled for the session", "delta --color-only", "", true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			app := newAppWithFake(t, &bitbuckettest.FakeClient{T: t})
			app.cfg.DiffRenderer = tc.value
			app.cfg.RendererStyle = tc.style
			app.diffRendererDisabled = tc.disabled

			name, args, ok := app.diffRendererCommand()

			testkit.AssertEqual(t, "ok", ok, tc.wantOK)
			if !tc.wantOK {
				testkit.AssertEqual(t, "name", name, "")
				return
			}
			testkit.AssertEqual(t, "name", name, "delta")
			testkit.AssertSliceEqual(t, "args", args, []string{"--color-only"})
		})
	}
}

func TestFetchFileDiff_RunsTheRendererAndTheViewDrawsItsCells(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubDiff(fake)
	stubComments(fake)
	app := fileDiffApp(t, fake)
	app.cfg.DiffRenderer = fakeRenderer(t, colourEachLine)

	runAll(t, app, app.openFileDiff(testFileGo))
	out := app.detailView.View()

	// The cells are cached with the diff, so re-opening the file costs neither
	// another request nor another run.
	entry, ok := app.fileDiffCache[fileDiffKey(testPRKey, testFileGo)]
	if !ok {
		t.Fatal("the file diff was not cached")
	}
	testkit.AssertEqual(t, "a cell per line of the document", len(entry.value.rendered),
		len(strings.Split(strings.TrimSuffix(testDiffText, "\n"), "\n")))
	// Line 5 of the document is the first context line of testFileGo's hunk.
	testkit.AssertEqual(t, "the context cell", entry.value.rendered[5], greenSGR+"    b.mu.Lock()")

	if !strings.Contains(out, greenSGR) {
		t.Errorf("the renderer's colours are not on screen:\n%q", out)
	}
	if !strings.Contains(plain(out), "b.mu.Lock()") {
		t.Errorf("the diff is not on screen:\n%s", plain(out))
	}
	testkit.AssertEqual(t, "status error", app.statusPanel.ErrorMessage(), "")
	testkit.AssertEqual(t, "renderer still on", app.diffRendererDisabled, false)
}

func TestFetchFileDiff_LeavesTheRendererAloneWhenItIsNotConfigured(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubDiff(fake)
	stubComments(fake)
	app := fileDiffApp(t, fake)

	runAll(t, app, app.openFileDiff(testFileGo))

	entry, ok := app.fileDiffCache[fileDiffKey(testPRKey, testFileGo)]
	if !ok {
		t.Fatal("the file diff was not cached")
	}
	if entry.value.rendered != nil {
		t.Errorf("rendered = %q, want nil with no diffRenderer configured", entry.value.rendered)
	}
	if !strings.Contains(plain(app.detailView.View()), "b.mu.Lock()") {
		t.Error("the diff should still be on screen")
	}
}

func TestFetchFileDiff_AMisalignedRendererStillShowsTheDiff(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubDiff(fake)
	stubComments(fake)
	app := fileDiffApp(t, fake)
	app.cfg.DiffRenderer = fakeRenderer(t, colourEachLine+"printf 'one line too many\\n'\n")

	runAll(t, app, app.openFileDiff(testFileGo))

	if !strings.Contains(plain(app.detailView.View()), "b.mu.Lock()") {
		t.Error("a renderer failure must not cost the diff")
	}
	if !strings.Contains(app.statusPanel.ErrorMessage(), "diffRenderer: ") {
		t.Errorf("the failure should be on the status panel, got %q", app.statusPanel.ErrorMessage())
	}
	testkit.AssertEqual(t, "renderer off for the session", app.diffRendererDisabled, true)
}

func TestFetchFileDiff_APathThatNamesNoProgramTurnsTheRendererOff(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubDiff(fake)
	stubComments(fake)
	app := fileDiffApp(t, fake)
	app.cfg.DiffRenderer = filepath.Join(t.TempDir(), missingRenderer) + " --color-only"

	runAll(t, app, app.openFileDiff(testFileGo))

	if !strings.Contains(plain(app.detailView.View()), "b.mu.Lock()") {
		t.Error("a renderer failure must not cost the diff")
	}
	if !strings.Contains(app.statusPanel.ErrorMessage(), "diffRenderer: ") {
		t.Errorf("the failure should be on the status panel, got %q", app.statusPanel.ErrorMessage())
	}
	// Nothing about the next file makes a program that is not there run, so the
	// renderer is off rather than retried once per file opened.
	testkit.AssertEqual(t, "renderer off for the session", app.diffRendererDisabled, true)
	_, _, ok := app.diffRendererCommand()
	testkit.AssertEqual(t, "the next file skips the renderer", ok, false)
}

func TestHandleFileDiffLoaded_RendererFailuresAreReportedOncePerMessage(t *testing.T) {
	t.Parallel()
	app := fileDiffApp(t, &bitbuckettest.FakeClient{T: t})
	app.detailView.SetFileDiffLoading(testPRKey, testFileGo)
	msg := parsedFileDiffMsg(t, testFileGo)
	msg.rendererErr = errors.New("delta: exit status 2")

	_, _ = app.Update(msg)

	testkit.AssertEqual(t, "reported", app.statusPanel.ErrorMessage(), "diffRenderer: delta: exit status 2")
	if !strings.Contains(plain(app.detailView.View()), "b.mu.Lock()") {
		t.Error("the diff still has to be drawn with the built-in colouring")
	}

	// The same complaint on the next file is silent: a broken renderer must not
	// repaint the status panel once per file opened.
	app.statusPanel.SetError("")
	_, _ = app.Update(msg)
	testkit.AssertEqual(t, "repeat stayed quiet", app.statusPanel.ErrorMessage(), "")

	// A different complaint is worth saying.
	msg.rendererErr = errors.New("delta: signal: killed")
	_, _ = app.Update(msg)
	testkit.AssertEqual(t, "new failure", app.statusPanel.ErrorMessage(), "diffRenderer: delta: signal: killed")
}

func TestHandleFileDiffLoaded_OnlyHopelessFailuresDisableTheRenderer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		err          error
		wantDisabled bool
	}{
		{"not installed", &exec.Error{Name: missingRenderer, Err: exec.ErrNotFound}, true},
		{
			"a path that names nothing",
			&fs.PathError{Op: "fork/exec", Path: "/no/such/delta", Err: fs.ErrNotExist},
			true,
		},
		{
			"misaligned output",
			errors.New("delta printed 7 lines for 6 lines of diff: " + errDiffRendererMisaligned.Error()),
			false,
		},
		{"misaligned output, wrapped", errors.Join(errors.New("delta"), errDiffRendererMisaligned), true},
		{"non-zero exit", errors.New("delta: unknown option"), false},
		{"timed out", context.DeadlineExceeded, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			app := fileDiffApp(t, &bitbuckettest.FakeClient{T: t})
			app.detailView.SetFileDiffLoading(testPRKey, testFileGo)
			msg := parsedFileDiffMsg(t, testFileGo)
			msg.rendererErr = tc.err

			_, _ = app.Update(msg)

			testkit.AssertEqual(t, "disabled", app.diffRendererDisabled, tc.wantDisabled)
			if app.statusPanel.ErrorMessage() == "" {
				t.Error("every renderer failure is worth one line on the status panel")
			}
		})
	}
}
