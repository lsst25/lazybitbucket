package tui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// editorFinishedMsg is delivered when the editor the diff view suspended into
// exits.
type editorFinishedMsg struct{ err error }

// editorDialect is how one family of editors takes a file position on its
// command line. There is no common syntax, so the editor's own name is the
// only way to know.
type editorDialect int

const (
	// editorPathOnly is the zero value on purpose: an editor nobody recognises
	// is opened on the file and left to find its own way.
	editorPathOnly  editorDialect = iota
	editorPlusLine                // vim & friends: "+12" then the path
	editorGoto                    // VS Code: "--goto" then "path:12"
	editorColonLine               // Sublime, Zed, Helix: "path:12"
)

// editorDialects maps an editor's program name to its line-number syntax.
var editorDialects = map[string]editorDialect{
	"vim":           editorPlusLine,
	"nvim":          editorPlusLine,
	"vi":            editorPlusLine,
	"gvim":          editorPlusLine,
	"nano":          editorPlusLine,
	"micro":         editorPlusLine,
	"emacs":         editorPlusLine,
	"kak":           editorPlusLine,
	"code":          editorGoto,
	"codium":        editorGoto,
	"code-insiders": editorGoto,
	"subl":          editorColonLine,
	"sublime_text":  editorColonLine,
	"zed":           editorColonLine,
	"hx":            editorColonLine,
}

// editorCommand splits the configured editor command and appends the file
// position in the dialect that editor understands. The configured value may
// carry flags of its own ("code --wait"), so only its first field names the
// program. ok is false when no editor is configured at all.
func editorCommand(editor, path string, line int) (name string, args []string, ok bool) {
	fields := strings.Fields(editor)
	if len(fields) == 0 {
		return "", nil, false
	}
	name, args = fields[0], slices.Clone(fields[1:])
	line = max(line, 1)

	switch editorDialects[filepath.Base(name)] {
	case editorPlusLine:
		args = append(args, "+"+strconv.Itoa(line), path)
	case editorGoto:
		args = append(args, "--goto", path+":"+strconv.Itoa(line))
	case editorColonLine:
		args = append(args, path+":"+strconv.Itoa(line))
	case editorPathOnly:
		args = append(args, path)
	}
	return name, args, true
}

// buildEditorCmd is the command the editor key would run, resolved against the
// git checkout. It is factored out of openInEditor so tests can assert the
// program, its arguments and its working directory without launching an editor.
//
// Paths in a diff are repository-relative, so they are joined onto the working
// tree's root rather than onto the process's cwd — lazybitbucket is often run
// from a subdirectory. The root is whatever repository the app was started in
// and is never checked against the pull request's repository, because nothing
// maps a Bitbucket repository to a local checkout.
func (a *App) buildEditorCmd(path string, line int) (*exec.Cmd, error) {
	if a.gitRepoRoot == "" {
		return nil, errors.New("not inside a git repository")
	}
	full := filepath.Join(a.gitRepoRoot, path)
	if _, err := os.Stat(full); err != nil {
		return nil, fmt.Errorf("%s not found in the working tree — check out the source branch (%s)",
			path, a.keymap.Keys(ActCheckout))
	}
	name, args, ok := editorCommand(a.cfg.ResolveEditor(), full, line)
	if !ok {
		return nil, errors.New("no editor configured — set editor in config.yml, or $VISUAL/$EDITOR")
	}
	cmd := exec.CommandContext(a.context(), name, args...)
	cmd.Dir = a.gitRepoRoot
	return cmd, nil
}

// openInEditor suspends the TUI and opens one file at one line in the
// configured editor. Errors are reported on the status panel rather than
// raised: failing to find a file must not tear the session down.
func (a *App) openInEditor(path string, line int) tea.Cmd {
	cmd, err := a.buildEditorCmd(path, line)
	if err != nil {
		a.statusPanel.SetError(err.Error())
		return nil
	}
	a.noteBranchMismatch()

	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return tea.ExecProcess(cmd, func(execErr error) tea.Msg {
		return editorFinishedMsg{err: execErr}
	})
}

// noteBranchMismatch warns when the checkout is not on the pull request's
// source branch. The file still opens — it is the reader's checkout and their
// decision — but what they see is whatever revision is on disk, which is worth
// saying out loud.
func (a *App) noteBranchMismatch() {
	pr := a.currentPullRequest()
	if pr == nil || a.gitBranch == "" {
		return
	}
	source := pr.SourceBranch()
	if source == "" || source == a.gitBranch {
		return
	}
	a.helpBar.SetStatusMsg("editing " + a.gitBranch + ", PR source is " + source)
}

// handleEditorFinished re-arms mouse reporting, which the suspended child
// terminal turned off, and reports a launch failure.
func (a *App) handleEditorFinished(msg editorFinishedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		a.statusPanel.SetError("editor: " + msg.err.Error())
	}
	return a, tea.EnableMouseCellMotion
}

// openEditorAtCursor is the editor key: it opens the file the detail panel is
// showing at the line the cursor is on.
func (a *App) openEditorAtCursor() tea.Cmd {
	path, line, ok := a.editorTarget()
	if !ok {
		return nil
	}
	if a.fileRemoved(path) {
		a.statusPanel.SetError("file was removed in this pull request")
		return nil
	}
	return a.openInEditor(path, line)
}

// editorTarget is the file and line the editor should open: the cursor's line
// inside a file diff, and the first line of the selected file on the Files tab.
// It is also what a file-scoped custom command is filled from.
func (a *App) editorTarget() (path string, line int, ok bool) {
	switch a.detailView.Mode() {
	case views.ModeFileDiff:
		path = a.detailView.FileDiffPath()
		return path, a.editorLine(path), path != ""
	case views.ModePullRequest:
		if a.detailView.ActiveTab() != views.TabFiles {
			return "", 0, false
		}
		if entry := a.detailView.SelectedFile(); entry != nil {
			return entry.Path(), 1, true
		}
	case views.ModePipeline, views.ModeStepLog, views.ModeRepo, views.ModeSplash:
		// No file is on screen in these modes.
	}
	return "", 0, false
}

// fileRemoved reports whether this pull request deleted the file, in which case
// there is nothing in the working tree to open.
func (a *App) fileRemoved(path string) bool {
	for _, entry := range a.detailView.DiffStat() {
		if entry.Path() == path {
			return entry.Status == bitbucket.DiffStatRemoved
		}
	}
	return false
}

// editorLine is the new-side line under the cursor, which is the line the
// editor should land on: the working tree holds the new side of the diff. A
// removed line has no new-side number, so the nearest following line that does
// is used, and a hunk header opens the hunk's first line.
func (a *App) editorLine(path string) int {
	row, ok := a.detailView.SelectedDiffLine()
	if !ok {
		return 1
	}
	if row.NewNo > 0 {
		return row.NewNo
	}
	if row.OldNo > 0 {
		if line := a.newSideAfter(path, row.OldNo); line > 0 {
			return line
		}
	}
	return 1
}

// newSideAfter walks the hunk holding oldNo for the first line that exists on
// the new side, falling back to the hunk's own start when the whole rest of the
// hunk was deleted.
func (a *App) newSideAfter(path string, oldNo int) int {
	file := a.cachedFileDiff(path)
	if file == nil {
		return 0
	}
	for _, hunk := range file.Hunks {
		if oldNo < hunk.OldStart || oldNo >= hunk.OldStart+hunk.OldLines {
			continue
		}
		reached := false
		for _, line := range hunk.Lines {
			if line.OldNo == oldNo {
				reached = true
			}
			if reached && line.NewNo > 0 {
				return line.NewNo
			}
		}
		return hunk.NewStart
	}
	return 0
}
