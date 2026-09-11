package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/config"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// commandApp is a laid-out app holding one selected pull request, one build and
// one repository, so every scope has a source.
func commandApp(t *testing.T, commands ...config.CustomCommandConfig) *App {
	t.Helper()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	app.cfg.CustomCommands = commands
	app.initCustomCommands()

	pr := withAuthor(newPR(testRepo, 101, testTitle), testUserUUID)
	pr.CommentCount = 4
	pr.TaskCount = 1
	app.prList.SetPullRequests([]bitbucket.PullRequest{pr})
	app.repos = []bitbucket.Repository{{
		Slug: testRepo, Name: "API Gateway", FullName: testWorkspace + "/" + testRepo,
		MainBranch: &bitbucket.Branch{Name: "main"},
	}}
	app.repoList.SetRepos(app.repos)
	app.buildsPanel.SetPullRequest(pr.Key())
	app.buildsPanel.SetStatuses(pr.Key(), []bitbucket.CommitStatus{
		newStatus(testRepo, 412, bitbucket.StatusSuccessful),
	})
	app.gitRepoPath = "/tmp/repo"
	app.gitBranch = testBranch
	return app
}

func TestBuildCommandData_FillsOnlyTheDeclaredScopes(t *testing.T) {
	t.Parallel()

	// The scope mask comes from the command's contexts, not its template:
	// pullrequests implies pull request + repository, builds adds the build.
	cases := []struct {
		name     string
		contexts []string
		wantPR   bool
		wantRepo bool
		wantBld  bool
	}{
		{"pull requests context", []string{"pullrequests"}, true, true, false},
		{"detail context", []string{"detail"}, true, true, false},
		{"repos context", []string{"repos"}, false, true, false},
		{"builds context", []string{"builds"}, true, true, true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			app := commandApp(t, config.CustomCommandConfig{
				Key: "x", Name: testCase.name, Command: "echo hi", Contexts: testCase.contexts,
			})
			if len(app.customCmds) != 1 {
				t.Fatalf("custom commands = %d, want 1", len(app.customCmds))
			}

			data, _, ok := app.buildCommandData(app.customCmds[0], config.Context(testCase.contexts[0]))

			if !ok {
				t.Fatal("every scope had a source; buildCommandData should have succeeded")
			}
			testkit.AssertEqual(t, "PR filled", data.PR != nil, testCase.wantPR)
			testkit.AssertEqual(t, "Repo filled", data.Repo != nil, testCase.wantRepo)
			testkit.AssertEqual(t, "Build filled", data.Build != nil, testCase.wantBld)
			testkit.AssertEqual(t, "workspace", data.Workspace, testWorkspace)
			testkit.AssertEqual(t, "git branch", data.GitBranch, testBranch)
		})
	}
}

func TestBuildCommandData_PullRequestFields(t *testing.T) {
	t.Parallel()
	app := commandApp(t, config.CustomCommandConfig{
		Key: "x", Name: "pr", Command: "echo {{.PR.ID}}", Contexts: []string{"pullrequests"},
	})

	data, _, ok := app.buildCommandData(app.customCmds[0], config.CtxPullRequests)

	if !ok || data.PR == nil {
		t.Fatal("the pull request scope should have been filled")
	}
	testkit.AssertEqual(t, "id", data.PR.ID, 101)
	testkit.AssertEqual(t, "repo", data.PR.Repo, testRepo)
	testkit.AssertEqual(t, "workspace", data.PR.Workspace, testWorkspace)
	testkit.AssertEqual(t, "state", data.PR.State, testOpenState)
	testkit.AssertEqual(t, "source branch", data.PR.SourceBranch, testBranch)
	testkit.AssertEqual(t, "destination branch", data.PR.DestinationBranch, "main")
	testkit.AssertEqual(t, "comment count", data.PR.CommentCount, 4)
	testkit.AssertEqual(t, "author", data.PR.Author, testUserUUID)
}

func TestBuildCommandData_RepoScopeFallsBackToThePullRequestOnTheAllRow(t *testing.T) {
	t.Parallel()
	app := commandApp(t, config.CustomCommandConfig{
		Key: "x", Name: "repo", Command: "echo {{.Repo.Slug}}", Contexts: []string{"repos"},
	})
	// The cursor is on the synthetic "All repositories" row, which names no
	// repository, so the previewed pull request is the only concrete source.
	app.repoList.Cursor = 0

	data, _, ok := app.buildCommandData(app.customCmds[0], config.CtxRepos)

	if !ok || data.Repo == nil {
		t.Fatal("the repository scope should have been filled")
	}
	testkit.AssertEqual(t, "slug", data.Repo.Slug, testRepo)
	testkit.AssertEqual(t, "name from the repository list", data.Repo.Name, "API Gateway")
	testkit.AssertEqual(t, "main branch", data.Repo.MainBranch, "main")
}

// TestBuildCommandData_RepoScopeFollowsTheMatchedContext pins the rule that the
// repository scope is read from whatever the matched context is showing: the
// highlighted Repos row in the repos context, and the previewed pull request's
// repository everywhere else.
func TestBuildCommandData_RepoScopeFollowsTheMatchedContext(t *testing.T) {
	t.Parallel()

	setup := func(t *testing.T) *App {
		t.Helper()
		app := commandApp(t,
			config.CustomCommandConfig{
				Key: "x", Name: "repos", Command: "echo {{.Repo.Slug}}", Contexts: []string{"repos"},
				Suspend: boolPtr(false),
			},
			config.CustomCommandConfig{
				Key: "y", Name: "prs", Command: "echo {{.Repo.Slug}}", Contexts: []string{"pullrequests"},
			},
		)
		// A second repository, highlighted, while the previewed pull request
		// still lives in the first one.
		app.repos = append(app.repos, bitbucket.Repository{
			Slug: testRepo2, Name: "Web App", FullName: testWorkspace + "/" + testRepo2,
		})
		app.repoList.SetRepos(app.repos)
		app.repoList.Cursor = 2
		return app
	}

	t.Run("repos context uses the highlighted row", func(t *testing.T) {
		t.Parallel()
		app := setup(t)
		app.leftFocus = focusRepos

		data, _, ok := app.buildCommandData(app.customCmds[0], config.CtxRepos)

		if !ok || data.Repo == nil {
			t.Fatal("the repository scope should have been filled")
		}
		testkit.AssertEqual(t, "slug", data.Repo.Slug, testRepo2)
		testkit.AssertEqual(t, "name", data.Repo.Name, "Web App")
	})

	t.Run("pull request context still uses the pull request", func(t *testing.T) {
		t.Parallel()
		app := setup(t)

		data, _, ok := app.buildCommandData(app.customCmds[1], config.CtxPullRequests)

		if !ok || data.Repo == nil {
			t.Fatal("the repository scope should have been filled")
		}
		testkit.AssertEqual(t, "slug", data.Repo.Slug, testRepo)
	})

	t.Run("end to end through handleCustomCommand", func(t *testing.T) {
		t.Parallel()
		app := setup(t)
		app.leftFocus = focusRepos

		_, cmd, ok := app.handleCustomCommand("x")
		if !ok || cmd == nil {
			t.Fatal("the repos command should have been consumed")
		}
		msg, isFinished := cmd().(customCommandFinishedMsg)
		if !isFinished || msg.err != nil {
			t.Fatalf("msg = %+v, want a successful run", msg)
		}
		testkit.AssertEqual(t, "expanded repository", strings.TrimSpace(msg.output), testRepo2)
	})
}

func TestBuildCommandData_ReportsTheFirstMissingScope(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		command string
		context string
		strip   func(app *App)
		want    config.ScopeMask
	}{
		{
			name: "no pull request", command: "echo {{.PR.ID}}", context: "pullrequests",
			strip: func(app *App) { app.prList.SetPullRequests(nil) },
			want:  config.ScopePullRequest,
		},
		{
			name: "no build", command: "echo {{.Build.State}}", context: "builds",
			strip: func(app *App) { app.buildsPanel.SetStatuses(testPRKey, nil) },
			want:  config.ScopeBuild,
		},
		{
			name: "no repository", command: "echo {{.Repo.Slug}}", context: "repos",
			strip: func(app *App) {
				app.prList.SetPullRequests(nil)
				app.repoList.SetRepos(nil)
			},
			want: config.ScopeRepo,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			app := commandApp(t, config.CustomCommandConfig{
				Key: "x", Name: testCase.name, Command: testCase.command, Contexts: []string{testCase.context},
			})
			testCase.strip(app)

			_, missing, ok := app.buildCommandData(app.customCmds[0], config.Context(testCase.context))

			if ok {
				t.Fatal("a missing scope should not build command data")
			}
			testkit.AssertEqual(t, "missing scope", missing, testCase.want)
		})
	}
}

func TestScopeNoun(t *testing.T) {
	t.Parallel()

	testkit.AssertEqual(t, "pull request", scopeNoun(config.ScopePullRequest), "pull request")
	testkit.AssertEqual(t, "repository", scopeNoun(config.ScopeRepo), "repository")
	testkit.AssertEqual(t, "build", scopeNoun(config.ScopeBuild), "build")
	testkit.AssertEqual(t, "changed files", scopeNoun(config.ScopeChangedFiles), "changed file list")
	testkit.AssertEqual(t, "unknown", scopeNoun(0), "selection")
}

func TestHandleCustomCommand_MatchesOnKeyAndContext(t *testing.T) {
	t.Parallel()
	app := commandApp(t, config.CustomCommandConfig{
		Key: "F", Name: "echo pr", Command: "echo {{.PR.ID}}", Contexts: []string{"pullrequests"},
	})

	if _, _, ok := app.handleCustomCommand("Z"); ok {
		t.Error("an unbound key must not be consumed")
	}

	app.leftFocus = focusRepos
	if _, _, ok := app.handleCustomCommand("F"); ok {
		t.Error("a command bound to pullrequests must not fire on the repos panel")
	}

	app.leftFocus = focusPullRequests
	_, cmd, ok := app.handleCustomCommand("F")
	if !ok || cmd == nil {
		t.Fatal("the command should have been consumed and produced a command")
	}
}

func TestHandleCustomCommand_ReportsAMissingSelection(t *testing.T) {
	t.Parallel()
	app := commandApp(t, config.CustomCommandConfig{
		Key: "F", Name: "echo build", Command: "echo {{.Build.State}}", Contexts: []string{"builds"},
	})
	app.leftFocus = focusBuilds
	app.buildsPanel.SetStatuses(testPRKey, nil)

	_, cmd, ok := app.handleCustomCommand("F")

	if !ok {
		t.Fatal("the key should still be consumed")
	}
	if cmd != nil {
		t.Error("a missing selection must not run anything")
	}
	if !strings.Contains(plain(app.helpBar.View()), "no build selected") {
		t.Errorf("help bar = %q, want the missing-build message", plain(app.helpBar.View()))
	}
}

func TestExecuteCustomCommand_TemplateErrorIsReported(t *testing.T) {
	t.Parallel()
	app := commandApp(t, config.CustomCommandConfig{
		Key: "F", Name: "bad", Command: "echo {{.PR.Nope}}", Contexts: []string{"pullrequests"},
		Suspend: boolPtr(false),
	})

	data, _, ok := app.buildCommandData(app.customCmds[0], config.CtxPullRequests)
	if !ok {
		t.Fatal("command data should have been built")
	}
	msg, isFinished := app.executeCustomCommand(app.customCmds[0], data)().(customCommandFinishedMsg)

	if !isFinished || msg.err == nil {
		t.Fatalf("msg = %+v, want a template error", msg)
	}
	if !strings.Contains(msg.err.Error(), "template error") {
		t.Errorf("err = %v, want a template error", msg.err)
	}
}

func TestExecuteCustomCommand_BackgroundCommandCapturesOutput(t *testing.T) {
	t.Parallel()
	app := commandApp(t, config.CustomCommandConfig{
		Key: "F", Name: "echo", Command: "echo {{.PR.ID}}", Contexts: []string{"pullrequests"},
		Suspend: boolPtr(false),
	})

	data, _, ok := app.buildCommandData(app.customCmds[0], config.CtxPullRequests)
	if !ok {
		t.Fatal("command data should have been built")
	}
	msg, isFinished := app.executeCustomCommand(app.customCmds[0], data)().(customCommandFinishedMsg)

	if !isFinished {
		t.Fatalf("msg = %T, want customCommandFinishedMsg", msg)
	}
	if msg.err != nil {
		t.Fatalf("command failed: %v (%s)", msg.err, msg.output)
	}
	testkit.AssertEqual(t, "output", strings.TrimSpace(msg.output), "101")
}

func TestHandleCustomCommandFinished(t *testing.T) {
	t.Parallel()

	t.Run("failure reaches the status panel", func(t *testing.T) {
		t.Parallel()
		app := commandApp(t)

		_, cmd := app.handleCustomCommandFinished(customCommandFinishedMsg{
			err: errors.New("exit status 1"), output: "nope\n",
		})

		if cmd == nil {
			t.Error("mouse motion should always be re-enabled")
		}
		if !strings.Contains(app.statusPanel.ErrorMessage(), "nope") {
			t.Errorf("status error = %q, want the command output", app.statusPanel.ErrorMessage())
		}
	})

	t.Run("refresh re-fetches the selected pull request", func(t *testing.T) {
		t.Parallel()
		app := commandApp(t)
		app.prCache[testPRKey] = ptr(newPR(testRepo, 101, testTitle))
		app.statusCache[testPRKey] = statusEntry{}

		_, _ = app.handleCustomCommandFinished(customCommandFinishedMsg{refresh: true})

		if _, ok := app.prCache[testPRKey]; ok {
			t.Error("refresh should have invalidated the detail cache")
		}
		if _, ok := app.statusCache[testPRKey]; ok {
			t.Error("refresh should have invalidated the status cache")
		}
	})

	t.Run("without refresh nothing is invalidated", func(t *testing.T) {
		t.Parallel()
		app := commandApp(t)
		app.prCache[testPRKey] = ptr(newPR(testRepo, 101, testTitle))

		_, _ = app.handleCustomCommandFinished(customCommandFinishedMsg{output: "ok"})

		if _, ok := app.prCache[testPRKey]; !ok {
			t.Error("a command without refresh must not cost a request")
		}
	})
}

func TestQuitReachableWarning(t *testing.T) {
	t.Parallel()

	every := []string{"pullrequests", "builds", "repos", "detail", "diff"}
	cases := []struct {
		name     string
		commands []config.CustomCommandConfig
		want     bool
	}{
		{"nothing shadows quit", nil, false},
		{
			name: "one key shadowed everywhere is not enough",
			commands: []config.CustomCommandConfig{
				{Key: "q", Name: "shadow", Command: "true", Contexts: every},
			},
			want: false,
		},
		{
			name: "every key shadowed everywhere",
			commands: []config.CustomCommandConfig{
				{Key: "q", Name: "shadow q", Command: "true", Contexts: every},
				{Key: "ctrl+c", Name: "shadow ctrl+c", Command: "true", Contexts: every},
			},
			want: true,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			cfg := &config.Config{CustomCommands: testCase.commands}
			resolved, err := cfg.ResolveCustomCommands()
			if err != nil {
				t.Fatalf("resolve custom commands: %v", err)
			}

			got := quitReachableWarning(DefaultKeymap(), resolved) != ""

			testkit.AssertEqual(t, "warns", got, testCase.want)
		})
	}
}

func TestQuitReachableWarning_NoQuitBinding(t *testing.T) {
	t.Parallel()

	testkit.AssertEqual(t, "warning", quitReachableWarning(Keymap{}, nil), "")
}

func TestLastNonEmptyLine(t *testing.T) {
	t.Parallel()

	testkit.AssertEqual(t, "trailing blanks", lastNonEmptyLine("a\nb\n\n  \n"), "b")
	testkit.AssertEqual(t, "empty", lastNonEmptyLine("   "), "")
}

func TestInitCustomCommands_SurfacesResolutionErrors(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	app.cfg.CustomCommands = []config.CustomCommandConfig{{Key: "", Name: "broken", Command: "true"}}

	app.initCustomCommands()

	if app.customCmds != nil {
		t.Error("a resolution failure must leave the command list empty")
	}
	if !strings.Contains(app.statusPanel.ErrorMessage(), "custom commands") {
		t.Errorf("status error = %q, want the resolution failure", app.statusPanel.ErrorMessage())
	}
}

func boolPtr(v bool) *bool { return &v }

func ptr(pr bitbucket.PullRequest) *bitbucket.PullRequest { return &pr }

func TestBuildCommandData_FileScopeFollowsTheDiffView(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubDiff(fake)
	stubComments(fake)
	app := fileDiffApp(t, fake)
	app.cfg.CustomCommands = []config.CustomCommandConfig{
		{Key: "x", Name: "blame", Command: "git blame -L{{.File.Line}} {{.File.Path}}", Contexts: []string{"diff"}},
	}
	app.initCustomCommands()
	runAll(t, app, app.openFileDiff(testFileGo))
	_ = app.detailView.View()
	app.detailView.ScrollBy(4) // the first added line, new-side 12

	data, _, ok := app.buildCommandData(app.customCmds[0], config.CtxDiff)

	if !ok {
		t.Fatal("the diff view is a file source; buildCommandData should have succeeded")
	}
	if data.File == nil {
		t.Fatal("a diff-context command must be given the file scope")
	}
	testkit.AssertEqual(t, "path", data.File.Path, testFileGo)
	testkit.AssertEqual(t, "line", data.File.Line, 12)
	testkit.AssertEqual(t, "status", data.File.Status, bitbucket.DiffStatModified)
	testkit.AssertEqual(t, "old path", data.File.OldPath, testFileGo)
}

func TestBuildCommandData_FileScopeReportsMissingWithNoFileOnScreen(t *testing.T) {
	t.Parallel()
	app := reviewApp(t, &bitbuckettest.FakeClient{T: t})
	app.cfg.CustomCommands = []config.CustomCommandConfig{
		{Key: "x", Name: "blame", Command: "echo {{.File.Path}}", Contexts: []string{"diff"}},
	}
	app.initCustomCommands()

	_, missing, ok := app.buildCommandData(app.customCmds[0], config.CtxDiff)

	if ok {
		t.Fatal("the Overview tab names no file")
	}
	testkit.AssertEqual(t, "missing scope", missing, config.ScopeFile)
	testkit.AssertEqual(t, "noun", scopeNoun(config.ScopeFile), "file")
}

func TestHandleCustomCommand_DiffContextWinsOverDetail(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubDiff(fake)
	stubComments(fake)
	app := fileDiffApp(t, fake)
	app.cfg.CustomCommands = []config.CustomCommandConfig{
		{Key: "x", Name: "diff one", Command: "true", Contexts: []string{"diff"}},
		{Key: "x", Name: "detail one", Command: "true", Contexts: []string{"detail"}},
	}
	app.initCustomCommands()
	runAll(t, app, app.openFileDiff(testFileGo))

	contexts := app.activeContexts()

	if len(contexts) != 2 || contexts[0] != config.CtxDiff {
		t.Fatalf("contexts = %v, want the diff context first", contexts)
	}
	if _, _, ok := app.handleCustomCommand("x"); !ok {
		t.Error("the key must be consumed by the diff-context command")
	}
}

// changedFilesCmd is the command shape the changed-file list exists for:
// open every file the pull request touches, skipping the deleted ones.
func changedFilesCmd(contexts ...string) config.CustomCommandConfig {
	return config.CustomCommandConfig{
		Key: "E", Name: "Open changed files", ChangedFiles: true, Contexts: contexts,
		Command: `nvim -p {{range .Files}}{{if ne .Status "removed"}}{{shellescape .Path}} {{end}}{{end}}`,
	}
}

func TestBuildCommandData_ChangedFilesComeFromTheDiffStatCache(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubDiffStat(fake)
	app := reviewApp(t, fake)
	app.cfg.CustomCommands = []config.CustomCommandConfig{changedFilesCmd("detail")}
	app.initCustomCommands()
	runAll(t, app, app.jumpToTab(views.TabFiles))

	data, _, ok := app.buildCommandData(app.customCmds[0], config.CtxDetail)

	if !ok {
		t.Fatal("the diffstat is cached; buildCommandData should have succeeded")
	}
	if len(data.Files) != 2 {
		t.Fatalf("files = %v, want the two diffstat entries", data.Files)
	}
	testkit.AssertEqual(t, "first path", data.Files[0].Path, testFileGo)
	testkit.AssertEqual(t, "first status", data.Files[0].Status, bitbucket.DiffStatModified)
	testkit.AssertEqual(t, "first line", data.Files[0].Line, 1)
	testkit.AssertEqual(t, "second path", data.Files[1].Path, testFileMD)
	testkit.AssertEqual(t, "second old path", data.Files[1].OldPath, "")
}

func TestBuildCommandData_ChangedFilesAreOnlyFilledWhenAskedFor(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubDiffStat(fake)
	app := reviewApp(t, fake)
	app.cfg.CustomCommands = []config.CustomCommandConfig{
		{Key: "x", Name: "plain", Command: "echo {{.PR.ID}}", Contexts: []string{"detail"}},
	}
	app.initCustomCommands()
	runAll(t, app, app.jumpToTab(views.TabFiles))

	data, _, ok := app.buildCommandData(app.customCmds[0], config.CtxDetail)

	if !ok {
		t.Fatal("buildCommandData should have succeeded")
	}
	if data.Files != nil {
		t.Errorf("files = %v, want nil for a command that did not ask for them", data.Files)
	}
}

func TestHandleCustomCommand_FetchesTheChangedFilesThenRuns(t *testing.T) {
	t.Parallel()
	calls := 0
	fake := &bitbuckettest.FakeClient{T: t}
	fake.ListPullRequestDiffStatFunc = func(context.Context, string, string, int) ([]bitbucket.DiffStatEntry, error) {
		calls++
		return testDiffStat(), nil
	}
	app := reviewApp(t, fake)
	app.cfg.CustomCommands = []config.CustomCommandConfig{changedFilesCmd("detail")}
	app.initCustomCommands()

	// Nothing is cached yet: the first press fetches instead of running.
	_, cmd, ok := app.handleCustomCommand("E")
	if !ok || cmd == nil {
		t.Fatal("the key should be consumed and start the diffstat fetch")
	}
	if app.pendingCmd == nil || app.pendingCmd.key != testPRKey {
		t.Fatalf("pending command = %v, want one waiting on %s", app.pendingCmd, testPRKey)
	}

	// The answer resumes it. runAll drains the whole chain, and the command
	// the resume produced is the shell exec, which runAll would run — assert on
	// the data instead by resuming by hand.
	msg, msgOK := cmd().(diffStatLoadedMsg)
	if !msgOK {
		t.Fatal("expected a diffStatLoadedMsg")
	}
	_, next := app.handleDiffStatLoaded(msg)
	if next == nil {
		t.Fatal("the cached diffstat should have resumed the custom command")
	}
	testkit.AssertEqual(t, "pending cleared", app.pendingCmd == nil, true)

	// A second press runs straight off the cache.
	_, cmd, ok = app.handleCustomCommand("E")
	if !ok || cmd == nil {
		t.Fatal("the second press should have run the command")
	}
	testkit.AssertEqual(t, "diffstat requests", calls, 1)
	testkit.AssertEqual(t, "nothing left pending", app.pendingCmd == nil, true)
}

func TestHandleCustomCommand_ChangedFilesFetchFailureIsReported(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.ListPullRequestDiffStatFunc = func(context.Context, string, string, int) ([]bitbucket.DiffStatEntry, error) {
		return nil, errors.New("boom")
	}
	app := reviewApp(t, fake)
	app.cfg.CustomCommands = []config.CustomCommandConfig{changedFilesCmd("detail")}
	app.initCustomCommands()

	_, cmd, _ := app.handleCustomCommand("E")
	msg, ok := cmd().(diffStatLoadedMsg)
	if !ok {
		t.Fatal("expected a diffStatLoadedMsg")
	}
	_, next := app.handleDiffStatLoaded(msg)

	if next != nil {
		t.Error("a failed diffstat must not run the command")
	}
	testkit.AssertEqual(t, "pending cleared", app.pendingCmd == nil, true)
	if !strings.Contains(app.statusPanel.ErrorMessage(), "boom") {
		t.Errorf("status error = %q, want the fetch failure", app.statusPanel.ErrorMessage())
	}

	// The failure is cached, so a second press reports it without re-fetching.
	_, cmd, consumed := app.handleCustomCommand("E")
	testkit.AssertEqual(t, "key consumed", consumed, true)
	if cmd != nil {
		t.Error("a cached failure must not run the command either")
	}
}

func TestResumeCustomCommand_DropsAResumeForAnotherPullRequest(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubDiffStat(fake)
	app := reviewApp(t, fake)
	app.cfg.CustomCommands = []config.CustomCommandConfig{changedFilesCmd("detail")}
	app.initCustomCommands()
	app.pendingCmd = &pendingCustomCommand{cmd: app.customCmds[0], ctx: config.CtxDetail, key: "other/repo#7"}

	if cmd := app.resumeCustomCommand(testPRKey); cmd != nil {
		t.Error("a diffstat for a different pull request must not resume the command")
	}
	testkit.AssertEqual(t, "pending kept", app.pendingCmd != nil, true)
}
