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

// CommandData is what a custom command template is executed against. Only the
// scopes the command declared are populated; the rest stay nil.
type CommandData struct {
	PR          *PullRequestScopeData
	Repo        *RepoScopeData
	Build       *BuildScopeData
	Workspace   string
	GitRepoPath string
	GitBranch   string
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
func (a *App) activeContexts() []config.Context {
	var out []config.Context
	if a.side == sideRight {
		switch a.detailView.Mode() {
		case views.ModePullRequest, views.ModePipeline, views.ModeStepLog:
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
	case focusStatus:
		// No commands bind to the status panel.
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

	return data, 0, true
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

// customCommandHelpItems returns the help bar entries for a context.
func (a *App) customCommandHelpItems(ctx config.Context) []components.HelpItem {
	items := make([]components.HelpItem, 0)
	for _, rc := range a.customCmds {
		if rc.HasContext(ctx) {
			items = append(items, components.HelpItem{Key: rc.Key, Description: rc.Name})
		}
	}
	return items
}

// customCommandBindings returns the help overlay entries for a context.
func (a *App) customCommandBindings(ctx config.Context) []Binding {
	bindings := make([]Binding, 0)
	for _, rc := range a.customCmds {
		if rc.HasContext(ctx) {
			bindings = append(bindings, Binding{Key: rc.Key, Description: rc.Name})
		}
	}
	return bindings
}
