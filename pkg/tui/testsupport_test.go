package tui

import (
	"context"
	"regexp"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/config"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// testNow is the fixed clock hand-built apps run on, so staleness rules are
// decided by the test rather than by wall time.
var testNow = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

func plain(s string) string { return ansiRe.ReplaceAllString(s, "") }

func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// testConfig is the configuration a hand-built app runs on: the demo
// workspace, two repositories and auto-refresh off, so Init emits no timer.
func testConfig() *config.Config {
	return &config.Config{
		Bitbucket: config.BitbucketConfig{
			Workspace:    testWorkspace,
			Repositories: []string{testRepo, testRepo2},
			Email:        "demo@example.com",
		},
		PullRequestTabs: config.DefaultPullRequestTabs(),
		RefreshInterval: "off",
		GUI: config.GUIConfig{
			SidePanelWidth:        40,
			CollapsedPanelHeight:  5,
			PullRequestListFields: config.DefaultPullRequestListFields(),
		},
	}
}

// newTestApp is the bare struct literal: panes and configuration, no client.
func newTestApp() *App {
	return &App{
		cfg:        testConfig(),
		prList:     views.NewPullRequestList(),
		repoList:   views.NewRepoList(),
		detailView: views.NewDetailView(views.PlainRenderer{}),
		logFlag:    new(atomic.Bool),
		side:       sideLeft,
		leftFocus:  focusPullRequests,
	}
}

// newAppWithFake is the canonical app for unit tests: every pane, every cache,
// a fixed clock, a cancellable context and no timers.
func newAppWithFake(t *testing.T, fake *bitbuckettest.FakeClient) *App {
	t.Helper()
	app := newTestApp()
	app.client = fake
	app.workspace = testWorkspace
	app.repoSlugs = []string{testRepo, testRepo2}
	app.prList.SetTabs(app.cfg.PullRequestTabs)
	app.prList.SetFields(app.cfg.GUI.PullRequestListFields)
	app.buildsPanel = views.NewBuildsPanel()
	app.statusPanel = views.NewStatusPanel("all repos", "demo", testWorkspace)
	app.logPanel = views.NewLogPanel()
	app.merged = map[string][]bitbucket.PullRequest{}
	app.inFlight = map[string]bool{}
	app.prCache = map[string]*bitbucket.PullRequest{}
	app.statusCache = map[string]statusEntry{}
	app.pipelineCache = map[string]*pipelineEntry{}
	app.now = func() time.Time { return testNow }
	app.debounce = time.Nanosecond
	app.refreshEvery = 0

	ctx, cancel := context.WithCancel(t.Context())
	app.ctx, app.cancel = ctx, cancel
	t.Cleanup(cancel)

	return app
}

// sizedApp is newAppWithFake plus a keymap and a laid-out 120x40 terminal.
func sizedApp(t *testing.T, fake *bitbuckettest.FakeClient) *App {
	t.Helper()
	app := newAppWithFake(t, fake)
	app.keymap = DefaultKeymap()
	app.prList.ResolveNav = app.keymap.MatchNav
	app.repoList.ResolveNav = app.keymap.MatchNav
	app.buildsPanel.ResolveNav = app.keymap.MatchNav
	app.detailView.ResolveNav = app.keymap.MatchNav
	app.width = 120
	app.height = 40
	app.layoutPanels()
	return app
}

// runAll executes a command and everything it fans out into, feeding each
// message back through Update until the chain is quiet. It is bounded so a
// self-feeding handler fails the test rather than hanging it.
func runAll(t *testing.T, app *App, cmd tea.Cmd) {
	t.Helper()
	budget := 500
	var run func(tea.Cmd)
	run = func(c tea.Cmd) {
		if c == nil || budget <= 0 {
			return
		}
		budget--
		switch msg := c().(type) {
		case nil:
		case tea.BatchMsg:
			for _, sub := range msg {
				run(sub)
			}
		case tea.QuitMsg:
		default:
			_, next := app.Update(msg)
			run(next)
		}
	}
	run(cmd)
	if budget <= 0 {
		t.Fatal("runAll exhausted its budget: the message chain does not settle")
	}
}

// collectMsgs flattens a command into the messages it produces, without
// feeding them back into an app.
func collectMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, sub := range batch {
			out = append(out, collectMsgs(sub)...)
		}
		return out
	}
	if msg == nil {
		return nil
	}
	return []tea.Msg{msg}
}

// newPR builds a list-shaped pull request: enough of the wire structure that
// Key, RepoSlug and the branch helpers work.
func newPR(repo string, id int, title string) bitbucket.PullRequest {
	pr := bitbucket.PullRequest{
		ID:        id,
		Title:     title,
		State:     bitbucket.PullRequestOpen,
		UpdatedOn: testNow,
		CreatedOn: testNow.Add(-24 * time.Hour),
	}
	ref := bitbucket.RepositoryRef{Name: repo, FullName: testWorkspace + "/" + repo}
	pr.Destination.Repository = ref
	pr.Destination.Branch = bitbucket.Branch{Name: "main"}
	pr.Source.Repository = ref
	pr.Source.Branch = bitbucket.Branch{Name: testBranch}
	pr.Links.HTML = bitbucket.Link{Href: "https://bitbucket.org/" + ref.FullName + "/pull-requests/1"}
	return pr
}

// withAuthor stamps an author and marks the pull request as carrying review
// data (an empty, non-nil participants slice).
func withAuthor(pr bitbucket.PullRequest, uuid string) bitbucket.PullRequest {
	pr.Author = bitbucket.Account{UUID: uuid, DisplayName: uuid}
	pr.Participants = []bitbucket.Participant{}
	return pr
}

// withReviewer adds a reviewer, which also marks review data as loaded.
func withReviewer(pr bitbucket.PullRequest, uuid string) bitbucket.PullRequest {
	pr.Reviewers = append(pr.Reviewers, bitbucket.Account{UUID: uuid, DisplayName: uuid})
	if pr.Participants == nil {
		pr.Participants = []bitbucket.Participant{}
	}
	return pr
}

// newStatus builds a build status pointing at a pipeline run.
func newStatus(repo string, build int, state bitbucket.CommitStatusState) bitbucket.CommitStatus {
	return bitbucket.CommitStatus{
		Type:      "build",
		Key:       "prs:**:" + testBranch,
		Name:      "Pipeline - pullrequests: **",
		RefName:   testBranch,
		URL:       "https://bitbucket.org/" + testWorkspace + "/" + repo + "/pipelines/results/" + strconv.Itoa(build),
		State:     state,
		UpdatedOn: testNow,
	}
}
