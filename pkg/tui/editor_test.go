package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

func TestEditorCommand_SpeaksEachDialect(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		editor   string
		line     int
		wantName string
		wantArgs []string
	}{
		{
			name: "nvim takes a plus line", editor: "nvim", line: 12,
			wantName: "nvim", wantArgs: []string{"+12", "/tmp/x.go"},
		},
		{
			name: "vi is the same family", editor: "vi", line: 3,
			wantName: "vi", wantArgs: []string{"+3", "/tmp/x.go"},
		},
		{
			name: "emacs too", editor: "emacs", line: 9,
			wantName: "emacs", wantArgs: []string{"+9", "/tmp/x.go"},
		},
		{
			name: "vs code goes to path:line", editor: "code", line: 40,
			wantName: "code", wantArgs: []string{"--goto", "/tmp/x.go:40"},
		},
		{
			name: "flags in the configured command are kept", editor: "code --wait", line: 40,
			wantName: "code", wantArgs: []string{"--wait", "--goto", "/tmp/x.go:40"},
		},
		{
			name: "sublime appends path:line", editor: "subl", line: 7,
			wantName: "subl", wantArgs: []string{"/tmp/x.go:7"},
		},
		{
			name: "helix too", editor: "hx", line: 7,
			wantName: "hx", wantArgs: []string{"/tmp/x.go:7"},
		},
		{
			name: "an absolute path is matched on its basename", editor: "/usr/local/bin/nvim", line: 5,
			wantName: "/usr/local/bin/nvim", wantArgs: []string{"+5", "/tmp/x.go"},
		},
		{
			name: "an unknown editor just gets the file", editor: "ed", line: 5,
			wantName: "ed", wantArgs: []string{"/tmp/x.go"},
		},
		{
			name: "a line of zero opens the file without a position", editor: "nvim", line: 0,
			wantName: "nvim", wantArgs: []string{"/tmp/x.go"},
		},
		{
			name: "a line of zero drops the goto too", editor: "code", line: 0,
			wantName: "code", wantArgs: []string{"/tmp/x.go"},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			name, args, ok := editorCommand(testCase.editor, "/tmp/x.go", testCase.line)

			if !ok {
				t.Fatal("a configured editor must resolve")
			}
			testkit.AssertEqual(t, "program", name, testCase.wantName)
			testkit.AssertSliceEqual(t, "args", args, testCase.wantArgs)
		})
	}
}

func TestEditorCommand_NoEditorConfigured(t *testing.T) {
	t.Parallel()

	if _, _, ok := editorCommand("   ", "/tmp/x.go", 1); ok {
		t.Error("a blank editor command must not resolve")
	}
}

// editorApp is a diff view over a real working tree, so buildEditorCmd can
// stat the file it is about to open.
func editorApp(t *testing.T, fake *bitbuckettest.FakeClient) (*App, string) {
	t.Helper()
	stubDiff(fake)
	stubComments(fake)
	app := fileDiffApp(t, fake)

	root := t.TempDir()
	for _, path := range []string{testFileGo, testFileMD} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	app.gitRepoRoot = root
	app.cfg.Editor = "nvim"
	return app, root
}

func TestBuildEditorCmd_ResolvesAgainstTheWorkingTreeRoot(t *testing.T) {
	t.Parallel()
	app, root := editorApp(t, &bitbuckettest.FakeClient{T: t})

	cmd, err := app.buildEditorCmd(testFileGo, 12)

	if err != nil {
		t.Fatalf("buildEditorCmd: %v", err)
	}
	testkit.AssertEqual(t, "program", filepath.Base(cmd.Path), "nvim")
	testkit.AssertEqual(t, "working directory", cmd.Dir, root)
	testkit.AssertSliceEqual(t, "args", cmd.Args[1:],
		[]string{"+12", filepath.Join(root, testFileGo)})
}

func TestBuildEditorCmd_ErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("outside a git repository", func(t *testing.T) {
		t.Parallel()
		app, _ := editorApp(t, &bitbuckettest.FakeClient{T: t})
		app.gitRepoRoot = ""

		_, err := app.buildEditorCmd(testFileGo, 1)

		if err == nil || !strings.Contains(err.Error(), "not inside a git repository") {
			t.Fatalf("err = %v, want the not-a-repository error", err)
		}
	})

	t.Run("the file is not checked out", func(t *testing.T) {
		t.Parallel()
		app, _ := editorApp(t, &bitbuckettest.FakeClient{T: t})

		_, err := app.buildEditorCmd("does/not/exist.go", 1)

		if err == nil || !strings.Contains(err.Error(), "not found in the working tree") {
			t.Fatalf("err = %v, want the missing-file error", err)
		}
		if !strings.Contains(err.Error(), "(c)") {
			t.Errorf("err = %v, want the checkout key named", err)
		}
	})
}

func TestOpenInEditor_ReportsTheErrorAndRunsNothing(t *testing.T) {
	t.Parallel()
	app, _ := editorApp(t, &bitbuckettest.FakeClient{T: t})
	app.gitRepoRoot = ""

	if cmd := app.openInEditor(testFileGo, 1); cmd != nil {
		t.Error("a failure to resolve the file must not launch anything")
	}
	if !strings.Contains(app.statusPanel.ErrorMessage(), "not inside a git repository") {
		t.Errorf("status error = %q, want the resolution failure", app.statusPanel.ErrorMessage())
	}
}

func TestOpenInEditor_NotesABranchMismatch(t *testing.T) {
	t.Parallel()
	app, _ := editorApp(t, &bitbuckettest.FakeClient{T: t})
	app.gitBranch = "main"

	if cmd := app.openInEditor(testFileGo, 1); cmd == nil {
		t.Fatal("the editor should still open on a different branch")
	}

	note := plain(app.helpBar.View())
	if !strings.Contains(note, "editing main") || !strings.Contains(note, testBranch) {
		t.Errorf("help bar = %q, want both branch names", note)
	}
}

func TestOpenInEditor_SaysNothingOnTheSourceBranch(t *testing.T) {
	t.Parallel()
	app, _ := editorApp(t, &bitbuckettest.FakeClient{T: t})
	app.gitBranch = testBranch

	if cmd := app.openInEditor(testFileGo, 1); cmd == nil {
		t.Fatal("the editor should open")
	}

	if strings.Contains(plain(app.helpBar.View()), "editing") {
		t.Error("no note is due when the checkout is on the pull request's source branch")
	}
}

func TestEditorLine_FollowsTheCursor(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		// cursorSteps is how many rows down from the file's first row the
		// cursor sits: 0 is the hunk header, then the rows in order.
		cursorSteps int
		wantLine    int
	}{
		{name: "hunk header opens the hunk start", cursorSteps: 0, wantLine: 10},
		{name: "a context line opens itself", cursorSteps: 1, wantLine: 10},
		{name: "a removed line opens the next new-side line", cursorSteps: 3, wantLine: 12},
		{name: "an added line opens itself", cursorSteps: 4, wantLine: 12},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			app, _ := editorApp(t, &bitbuckettest.FakeClient{T: t})
			runAll(t, app, app.openFileDiff(testFileGo))
			_ = app.detailView.View()
			app.detailView.ScrollBy(testCase.cursorSteps)

			testkit.AssertEqual(t, "editor line", app.editorLine(testFileGo), testCase.wantLine)
		})
	}
}

func TestEditorTarget_IsTheFilesTabCursorOutsideADiff(t *testing.T) {
	t.Parallel()
	app, _ := editorApp(t, &bitbuckettest.FakeClient{T: t})

	path, line, ok := app.editorTarget()

	if !ok {
		t.Fatal("the Files tab cursor is an editor target")
	}
	testkit.AssertEqual(t, "path", path, testFileGo)
	testkit.AssertEqual(t, "line", line, 1)
}

func TestEditorTarget_NothingToOpenElsewhere(t *testing.T) {
	t.Parallel()
	app, _ := editorApp(t, &bitbuckettest.FakeClient{T: t})
	app.detailView.SetActiveTab(views.TabCommits)

	if _, _, ok := app.editorTarget(); ok {
		t.Error("the Commits tab has no file to open")
	}
	if cmd := app.openEditorAtCursor(); cmd != nil {
		t.Error("the editor key is a no-op where no file is on screen")
	}
}

func TestOpenEditorAtCursor_RefusesAFileThePullRequestDeleted(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.ListPullRequestDiffStatFunc = func(_ context.Context, _, _ string, _ int) ([]bitbucket.DiffStatEntry, error) {
		return []bitbucket.DiffStatEntry{{
			Type: "diffstat", Status: bitbucket.DiffStatRemoved,
			LinesAdded: 0, LinesRemoved: 4,
			Old: &bitbucket.CommitFile{Path: testFileGo, EscapedPath: testFileGo},
			New: nil,
		}}, nil
	}
	app := reviewApp(t, fake)
	runAll(t, app, app.jumpToTab(views.TabFiles))
	_ = app.detailView.View()

	if cmd := app.openEditorAtCursor(); cmd != nil {
		t.Error("there is nothing in the working tree to open")
	}
	testkit.AssertEqual(t, "status error", app.statusPanel.ErrorMessage(),
		"file was removed in this pull request")
}

func TestOpenEditorAction_LaunchesFromTheDiffView(t *testing.T) {
	t.Parallel()
	app, root := editorApp(t, &bitbuckettest.FakeClient{T: t})
	runAll(t, app, app.openFileDiff(testFileGo))
	_ = app.detailView.View()
	app.detailView.ScrollBy(4) // the first added line, new-side 12

	_, cmd, handled := app.handleReviewAction(ActOpenEditor)

	if !handled {
		t.Fatal("the editor action must be handled on the detail panel")
	}
	if cmd == nil {
		t.Fatal("the editor action should have produced a suspend command")
	}
	// The command itself is tea.ExecProcess, which only a running program can
	// execute; what it would run is asserted through buildEditorCmd.
	built, err := app.buildEditorCmd(testFileGo, app.editorLine(testFileGo))
	if err != nil {
		t.Fatalf("buildEditorCmd: %v", err)
	}
	testkit.AssertSliceEqual(t, "args", built.Args[1:],
		[]string{"+12", filepath.Join(root, testFileGo)})
}

func TestHandleEditorFinished_ReArmsTheMouse(t *testing.T) {
	t.Parallel()
	app := reviewApp(t, &bitbuckettest.FakeClient{T: t})

	_, cmd := app.Update(editorFinishedMsg{err: nil})

	if cmd == nil {
		t.Fatal("the editor must hand the mouse back")
	}
	// The message type is unexported, so the command's own output is the
	// comparison: a suspended editor leaves mouse reporting off.
	msgs := collectMsgs(cmd)
	if len(msgs) != 1 || msgs[0] != tea.EnableMouseCellMotion() {
		t.Errorf("cmd produced %v, want the mouse re-arm", msgs)
	}
}

func TestHandleEditorFinished_ReportsAFailedLaunch(t *testing.T) {
	t.Parallel()
	app := reviewApp(t, &bitbuckettest.FakeClient{T: t})

	_, _ = app.Update(editorFinishedMsg{err: errors.New(testBoom)})

	if !strings.Contains(app.statusPanel.ErrorMessage(), testBoom) {
		t.Errorf("status error = %q, want the editor failure", app.statusPanel.ErrorMessage())
	}
}
