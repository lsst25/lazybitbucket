package tui

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/config"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// customCommandFinishedMsg is sent when a custom command exits. output is only
// populated for background commands; suspended ones write to the terminal.
type customCommandFinishedMsg struct {
	err     error
	output  string
	refresh bool
}

// PullRequestScopeData is the template data of a pull-request-scoped command.
type PullRequestScopeData struct {
	ID                int
	Title             string
	Description       string
	State             string
	Draft             bool
	Author            string
	AuthorNickname    string
	SourceBranch      string
	DestinationBranch string
	SourceCommit      string
	DestinationCommit string
	URL               string
	Repo              string
	RepoFullName      string
	Workspace         string
	CommentCount      int
	TaskCount         int
	Reviewers         int
	Approvals         int
	ChangesRequested  int
}

// RepoScopeData is the template data of a repository-scoped command.
type RepoScopeData struct {
	Slug       string
	Name       string
	FullName   string
	Workspace  string
	URL        string
	MainBranch string
}

// BuildScopeData is the template data of a build-scoped command.
type BuildScopeData struct {
	Name        string
	State       string
	Key         string
	RefName     string
	URL         string
	BuildNumber int
}

// FileScopeData is the template data of a file-scoped command: the changed
// file the detail panel is showing and the line the cursor is on. Path is
// repository-relative, as it is in the diff.
type FileScopeData struct {
	Path    string
	OldPath string
	Status  string
	Line    int
}

// CommandData is what a custom command template is executed against. Only the
// scopes the command declared are populated; the rest stay nil.
type CommandData struct {
	PR    *PullRequestScopeData
	Repo  *RepoScopeData
	Build *BuildScopeData
	File  *FileScopeData
	// Files is every file the pull request changes, in diffstat order. It is
	// only filled for a command that declared `changedFiles: true`, and is nil
	// otherwise — the list costs a request the other scopes do not.
	Files       []FileScopeData
	Workspace   string
	GitRepoPath string
	GitBranch   string
}

// pendingCustomCommand is a command that is waiting for the diffstat its
// {{.Files}} needs. It is resumed by handleDiffStatLoaded, and dropped
// whenever the fetch answers for a different pull request or fails.
type pendingCustomCommand struct {
	cmd config.ResolvedCustomCommand
	ctx config.Context
	key string
}

// initCustomCommands resolves the configured commands at startup. A resolution
// error is surfaced on the status panel and leaves the list empty.
func (a *App) initCustomCommands() {
	resolved, err := a.cfg.ResolveCustomCommands()
	if err != nil {
		a.statusPanel.SetError(fmt.Sprintf("custom commands: %v", err))
		a.customCmds = nil
		return
	}
	a.customCmds = resolved
}

// activeContexts returns the contexts that currently apply, most specific
// first. The first match wins in handleCustomCommand.
//
// The pipelines view has none at all. Every context is written against a pull
// request — {{.PR}}, its branch, its repository — and that view has no pull
// request under the cursor, so a command fired there would either expand to
// nothing or, worse, to whichever pull request the other view happens to have
// selected.
func (a *App) activeContexts() []config.Context {
	if a.view == viewPipelines {
		return nil
	}
	var out []config.Context
	if a.side == sideRight {
		switch a.detailView.Mode() {
		case views.ModeFileDiff:
			// The diff context is the more specific one; detail commands keep
			// working inside a file diff because it is still a pull request.
			out = append(out, config.CtxDiff, config.CtxDetail)
		case views.ModePullRequest:
			if a.detailView.ActiveTab() == views.TabFiles {
				out = append(out, config.CtxDiff)
			}
			out = append(out, config.CtxDetail)
		case views.ModePipeline, views.ModeStepLog:
			out = append(out, config.CtxDetail)
		case views.ModeRepo:
			out = append(out, config.CtxRepos)
		case views.ModeSplash:
			// The splash screen carries no selection.
		}
		return out
	}

	switch a.leftFocus {
	case focusPullRequests:
		out = append(out, config.CtxPullRequests)
	case focusBuilds:
		out = append(out, config.CtxBuilds)
	case focusRepos:
		out = append(out, config.CtxRepos)
	case focusStatus, focusPipelines:
		// No commands bind to the status panel, and none fire in the pipelines
		// view at all.
	}
	return out
}

// handleCustomCommand runs the command bound to keyStr in an active context.
// The third result reports whether the key was consumed.
func (a *App) handleCustomCommand(keyStr string) (tea.Model, tea.Cmd, bool) {
	for _, ctx := range a.activeContexts() {
		for _, rc := range a.customCmds {
			if rc.Key != keyStr || !rc.HasContext(ctx) {
				continue
			}
			if cmd, held := a.awaitChangedFiles(rc, ctx); held {
				return a, cmd, true
			}
			data, missing, ok := a.buildCommandData(rc, ctx)
			if !ok {
				a.helpBar.SetStatusMsg(fmt.Sprintf("%s: no %s selected", rc.Name, scopeNoun(missing)))
				return a, nil, true
			}
			return a, a.executeCustomCommand(rc, data), true
		}
	}
	return nil, nil, false
}

// scopeNoun names the selection a scope needs, for the "nothing selected"
// message.
func scopeNoun(scope config.ScopeMask) string {
	switch scope {
	case config.ScopePullRequest:
		return "pull request"
	case config.ScopeRepo:
		return "repository"
	case config.ScopeBuild:
		return "build"
	case config.ScopeFile:
		return "file"
	case config.ScopeChangedFiles:
		return "changed file list"
	default:
		return "selection"
	}
}

// buildCommandData fills one CommandData per scope bit the command declared.
// ctx is the context the command matched in, which decides where the
// repository scope is read from. The second result is the first scope that had
// no source, which is only meaningful when the third is false.
func (a *App) buildCommandData(rc config.ResolvedCustomCommand, ctx config.Context) (CommandData, config.ScopeMask, bool) {
	data := CommandData{
		PR:          nil,
		Repo:        nil,
		Build:       nil,
		File:        nil,
		Files:       nil,
		Workspace:   a.workspace,
		GitRepoPath: a.gitRepoPath,
		GitBranch:   a.gitBranch,
	}

	pr := a.currentPullRequest()

	if rc.Scopes&config.ScopePullRequest != 0 {
		if pr == nil {
			return data, config.ScopePullRequest, false
		}
		data.PR = a.buildPullRequestScopeData(pr)
	}

	if rc.Scopes&config.ScopeRepo != 0 {
		repo := a.buildRepoScopeData(pr, ctx)
		if repo == nil {
			return data, config.ScopeRepo, false
		}
		data.Repo = repo
	}

	if rc.Scopes&config.ScopeBuild != 0 {
		status := a.buildsPanel.SelectedStatus()
		if status == nil {
			return data, config.ScopeBuild, false
		}
		data.Build = buildScopeData(status)
	}

	if rc.NeedsChangedFiles() {
		entry, ok := reviewFresh(a.diffStatCache, pr.Key(), pr)
		if !ok || entry.err != nil {
			return data, config.ScopeChangedFiles, false
		}
		data.Files = changedFileScopeData(entry.value)
	}

	if rc.Scopes&config.ScopeFile != 0 {
		file := a.buildFileScopeData()
		if file == nil {
			return data, config.ScopeFile, false
		}
		data.File = file
	}

	return data, 0, true
}

// buildFileScopeData describes the changed file the detail panel is on: the
// file diff on screen, or the Files tab's cursor. The line is the same one the
// editor key would open, so {{.File.Line}} and `e` always agree.
func (a *App) buildFileScopeData() *FileScopeData {
	path, line, ok := a.editorTarget()
	if !ok {
		return nil
	}
	data := &FileScopeData{Path: path, OldPath: "", Status: "", Line: line}
	for _, entry := range a.detailView.DiffStat() {
		if entry.Path() != path {
			continue
		}
		data.OldPath = entry.OldPath()
		data.Status = entry.Status
	}
	return data
}

// changedFileScopeData turns a diffstat into the {{.Files}} list. Line is 1 on
// every entry: the list names files, not positions, and 1 is what an editor
// opens a file at.
func changedFileScopeData(entries []bitbucket.DiffStatEntry) []FileScopeData {
	files := make([]FileScopeData, 0, len(entries))
	for _, entry := range entries {
		files = append(files, FileScopeData{
			Path:    entry.Path(),
			OldPath: entry.OldPath(),
			Status:  entry.Status,
			Line:    1,
		})
	}
	return files
}

// awaitChangedFiles holds a {{.Files}} command back until the pull request's
// diffstat is cached, and reports whether it did. The list is not fetched with
// the pull request — no other scope costs a request — so the first press of
// such a key fetches, and the answer runs the command (resumeCustomCommand).
func (a *App) awaitChangedFiles(rc config.ResolvedCustomCommand, ctx config.Context) (tea.Cmd, bool) {
	if !rc.NeedsChangedFiles() {
		return nil, false
	}
	pr := a.currentPullRequest()
	if pr == nil {
		return nil, false // buildCommandData reports the missing pull request
	}
	entry, ok := reviewFresh(a.diffStatCache, pr.Key(), pr)
	switch {
	case ok && entry.err != nil:
		a.statusPanel.SetError(rc.Name + ": " + a.reviewErrorText(entry.err, "diff stat"))
		return nil, true
	case ok:
		return nil, false
	}
	a.helpBar.SetStatusMsg(rc.Name + ": loading changed files…")
	a.pendingCmd = &pendingCustomCommand{cmd: rc, ctx: ctx, key: pr.Key()}
	// ensureDiffStat rather than fetchDiffStat: a prefetch may already have the
	// diffstat on its way, and that answer resumes the command just as well.
	return a.ensureDiffStat(pr), true
}

// resumeCustomCommand runs the command that was waiting for key's diffstat.
// The caller has already cached the answer, so the command either runs now or
// is dropped — it is never queued twice.
func (a *App) resumeCustomCommand(key string) tea.Cmd {
	pending := a.pendingCmd
	if pending == nil || pending.key != key {
		return nil
	}
	a.pendingCmd = nil

	pr := a.currentPullRequest()
	if pr == nil || pr.Key() != key {
		return nil // the selection moved on while the request was in flight
	}
	data, missing, ok := a.buildCommandData(pending.cmd, pending.ctx)
	if !ok {
		a.helpBar.SetStatusMsg(fmt.Sprintf("%s: no %s selected", pending.cmd.Name, scopeNoun(missing)))
		return nil
	}
	return a.executeCustomCommand(pending.cmd, data)
}

func (a *App) buildPullRequestScopeData(pr *bitbucket.PullRequest) *PullRequestScopeData {
	summary := pr.ReviewSummary()
	data := &PullRequestScopeData{
		ID:                pr.ID,
		Title:             pr.Title,
		Description:       pr.Description,
		State:             string(pr.State),
		Draft:             pr.Draft,
		Author:            pr.Author.Name(),
		AuthorNickname:    pr.Author.Nickname,
		SourceBranch:      pr.SourceBranch(),
		DestinationBranch: pr.DestinationBranch(),
		SourceCommit:      "",
		DestinationCommit: "",
		URL:               pr.HTMLURL(),
		Repo:              pr.RepoSlug(),
		RepoFullName:      pr.RepoFullName(),
		Workspace:         pr.Workspace(),
		CommentCount:      pr.CommentCount,
		TaskCount:         pr.TaskCount,
		Reviewers:         summary.Reviewers,
		Approvals:         summary.Approved,
		ChangesRequested:  summary.ChangesRequested,
	}
	if pr.Source.Commit != nil {
		data.SourceCommit = pr.Source.Commit.Hash
	}
	if pr.Destination.Commit != nil {
		data.DestinationCommit = pr.Destination.Commit.Hash
	}
	return data
}

// buildRepoScopeData derives the repository the command should act on. In the
// Repos context — the focused Repos panel, or the detail panel showing a
// repository — that is the highlighted row, because that is the repository on
// screen. Everywhere else the pull request in view wins, enriched from the
// repository list, with the Repos panel selection as the fallback.
func (a *App) buildRepoScopeData(pr *bitbucket.PullRequest, ctx config.Context) *RepoScopeData {
	if ctx == config.CtxRepos {
		if data := a.repoScopeFromSelection(); data != nil {
			return data
		}
	}
	if pr != nil {
		return a.repoScopeFromPullRequest(pr)
	}
	return a.repoScopeFromSelection()
}

// repoScopeFromPullRequest builds the repository scope out of the pull request,
// enriched from the repository list when that repository is known.
func (a *App) repoScopeFromPullRequest(pr *bitbucket.PullRequest) *RepoScopeData {
	slug := pr.RepoSlug()
	data := &RepoScopeData{
		Slug:       slug,
		Name:       slug,
		FullName:   pr.RepoFullName(),
		Workspace:  pr.Workspace(),
		URL:        "",
		MainBranch: "",
	}
	for _, repo := range a.repos {
		if repo.Slug != slug {
			continue
		}
		if repo.Name != "" {
			data.Name = repo.Name
		}
		data.URL = repo.HTMLURL()
		data.MainBranch = repo.MainBranchName()
	}
	return data
}

// repoScopeFromSelection builds the repository scope out of the Repos panel
// cursor. The synthetic "All repositories" row names no repository, so it
// yields nothing and the caller falls back.
func (a *App) repoScopeFromSelection() *RepoScopeData {
	item := a.repoList.SelectedRepo()
	if item == nil || item.All {
		return nil
	}
	return &RepoScopeData{
		Slug:       item.Slug,
		Name:       item.Name,
		FullName:   item.FullName,
		Workspace:  a.workspace,
		URL:        item.URL,
		MainBranch: item.MainBranch,
	}
}

func buildScopeData(status *bitbucket.CommitStatus) *BuildScopeData {
	data := &BuildScopeData{
		Name:        status.Name,
		State:       string(status.State),
		Key:         status.Key,
		RefName:     status.RefName,
		URL:         status.URL,
		BuildNumber: 0,
	}
	if number, ok := status.BuildNumber(); ok {
		data.BuildNumber = number
	}
	return data
}

func (a *App) executeCustomCommand(rc config.ResolvedCustomCommand, data CommandData) tea.Cmd {
	var buf bytes.Buffer
	if err := rc.Template.Execute(&buf, data); err != nil {
		return func() tea.Msg {
			return customCommandFinishedMsg{err: fmt.Errorf("template error: %w", err), output: "", refresh: false}
		}
	}
	cmdStr := buf.String()

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "sh"
	}
	refresh := rc.Refresh

	if rc.ShouldSuspend() {
		c := exec.CommandContext(a.context(), shell, "-c", cmdStr) //nolint:gosec // user-configured custom commands are intentionally arbitrary shell commands
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return tea.ExecProcess(c, func(err error) tea.Msg {
			return customCommandFinishedMsg{err: err, output: "", refresh: refresh}
		})
	}

	// Background execution: capture the output and report it through the
	// message loop. The WaitGroup keeps Shutdown from orphaning the child.
	ctx := a.context()
	a.cmdWg.Add(1)
	return func() tea.Msg {
		defer a.cmdWg.Done()
		c := exec.CommandContext(ctx, shell, "-c", cmdStr) //nolint:gosec // user-configured custom commands are intentionally arbitrary shell commands
		c.Cancel = func() error {
			return c.Process.Signal(syscall.SIGTERM)
		}
		c.WaitDelay = 3 * time.Second
		out, err := c.CombinedOutput()
		return customCommandFinishedMsg{err: err, output: string(out), refresh: refresh}
	}
}

func (a *App) handleCustomCommandFinished(msg customCommandFinishedMsg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{tea.EnableMouseCellMotion}
	if msg.err != nil {
		errText := msg.err.Error()
		if tail := lastNonEmptyLine(msg.output); tail != "" {
			errText = tail + ": " + errText
		}
		a.statusPanel.SetError(errText)
		return a, tea.Batch(cmds...)
	}
	if tail := lastNonEmptyLine(msg.output); tail != "" {
		a.helpBar.SetStatusMsg(tail)
	}
	// Only a command that declared refresh: true costs a request.
	if msg.refresh {
		if _, cmd, _ := a.handleActionRefresh(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return a, tea.Batch(cmds...)
}

// lastNonEmptyLine is the last non-blank line of s, trimmed: one concise
// message out of arbitrary command output.
func lastNonEmptyLine(s string) string {
	lines := strings.Split(s, "\n")
	for _, v := range slices.Backward(lines) {
		if line := strings.TrimSpace(v); line != "" {
			return line
		}
	}
	return ""
}

// allUIContexts is the universe of UI contexts, used for shadowing analysis.
var allUIContexts = []config.Context{
	config.CtxPullRequests,
	config.CtxBuilds,
	config.CtxRepos,
	config.CtxDetail,
	config.CtxDiff,
}

// quitReachableWarning returns a warning when every key bound to ActQuit is
// shadowed by a custom command in every UI context, leaving no way out.
func quitReachableWarning(km Keymap, cmds []config.ResolvedCustomCommand) string {
	keys := km[ActQuit]
	if len(keys) == 0 {
		return ""
	}
	for _, key := range keys {
		for _, ctx := range allUIContexts {
			if !keyShadowsContext(key, ctx, cmds) {
				return ""
			}
		}
	}
	return fmt.Sprintf(
		"essential action %q is unreachable: keys %v are shadowed by custom commands in every UI context",
		ActQuit, keys,
	)
}

func keyShadowsContext(key string, ctx config.Context, cmds []config.ResolvedCustomCommand) bool {
	for _, rc := range cmds {
		if rc.Key == key && rc.HasContext(ctx) {
			return true
		}
	}
	return false
}

// customCommandHelpItems returns the help bar entries for a context. The
// pipelines view lists none, because activeContexts fires none there: a key in
// the help bar that does nothing is worse than no entry at all.
func (a *App) customCommandHelpItems(ctx config.Context) []components.HelpItem {
	if a.view == viewPipelines {
		return nil
	}
	items := make([]components.HelpItem, 0)
	for _, rc := range a.customCmds {
		if rc.HasContext(ctx) {
			items = append(items, components.HelpItem{Key: rc.Key, Description: rc.Name})
		}
	}
	return items
}

// customCommandBindings returns the help overlay entries for a context, and
// none in the pipelines view for the reason customCommandHelpItems gives.
func (a *App) customCommandBindings(ctx config.Context) []Binding {
	if a.view == viewPipelines {
		return nil
	}
	bindings := make([]Binding, 0)
	for _, rc := range a.customCmds {
		if rc.HasContext(ctx) {
			bindings = append(bindings, Binding{Key: rc.Key, Description: rc.Name})
		}
	}
	return bindings
}
