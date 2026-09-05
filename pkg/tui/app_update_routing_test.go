package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/diff"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

type routingCase struct {
	name    string
	setup   func(app *App, fake *bitbuckettest.FakeClient)
	msg     tea.Msg
	wantCmd bool
	assert  func(t *testing.T, app *App)
}

func runRoutingCases(t *testing.T, cases []routingCase) {
	t.Helper()
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			fake := &bitbuckettest.FakeClient{T: t}
			app := sizedApp(t, fake)
			if testCase.setup != nil {
				testCase.setup(app, fake)
			}

			_, cmd := app.Update(testCase.msg)

			if testCase.wantCmd && cmd == nil {
				t.Error("expected a non-nil command")
			}
			if !testCase.wantCmd && cmd != nil {
				t.Error("expected no command")
			}
			if testCase.assert != nil {
				testCase.assert(t, app)
			}
		})
	}
}

func TestUpdate_RoutesEveryBranch(t *testing.T) {
	t.Parallel()

	runRoutingCases(t, []routingCase{
		{
			name:    "window size",
			msg:     tea.WindowSizeMsg{Width: 100, Height: 30},
			wantCmd: false,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				testkit.AssertEqual(t, "width", app.width, 100)
			},
		},
		{
			name:    "mouse press focuses a panel",
			msg:     tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 2, Y: 0},
			wantCmd: false,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				testkit.AssertEqual(t, "leftFocus", app.leftFocus, focusStatus)
			},
		},
		{
			name:    "quit key",
			msg:     runeKey('q'),
			wantCmd: true,
		},
		{
			name:    "help key",
			msg:     runeKey('?'),
			wantCmd: false,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				if !app.showHelp {
					t.Error("? should open the help overlay")
				}
			},
		},
		{
			name:    "search key",
			msg:     runeKey('/'),
			wantCmd: false,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				if !app.searchBar.IsActive() {
					t.Error("/ should activate the search bar")
				}
			},
		},
		{
			name: "search changed filters the list",
			setup: func(app *App, _ *bitbuckettest.FakeClient) {
				app.prList.SetPullRequests([]bitbucket.PullRequest{
					newPR(testRepo, 101, "alpha"),
					newPR(testRepo, 102, "beta"),
				})
			},
			msg:     components.SearchChangedMsg{Query: "beta"},
			wantCmd: false,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				testkit.AssertEqual(t, "rows", app.prList.ItemCount(), 1)
			},
		},
		{
			name:    "search cancelled",
			msg:     components.SearchCancelledMsg{},
			wantCmd: false,
		},
		{
			name:    "search confirmed with an empty list",
			msg:     components.SearchConfirmedMsg{},
			wantCmd: false,
		},
		{
			name:    "auto fetch tick is inert while refresh is off",
			msg:     autoFetchTickMsg{},
			wantCmd: false,
		},
		{
			name:    "current user loaded",
			msg:     currentUserLoadedMsg{user: &bitbucket.Account{UUID: testUserUUID}},
			wantCmd: false,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				if app.currentUser == nil {
					t.Fatal("the current user was not stored")
				}
				testkit.AssertEqual(t, "uuid", app.currentUser.UUID, testUserUUID)
			},
		},
		{
			name: "repos loaded triggers the first fetch",
			setup: func(_ *App, fake *bitbuckettest.FakeClient) {
				fake.ListPullRequestsFunc = func(_ context.Context, _, repo string,
					_ bitbucket.ListPullRequestsOptions,
				) ([]bitbucket.PullRequest, error) {
					return []bitbucket.PullRequest{newPR(repo, 101, testTitle)}, nil
				}
			},
			msg:     reposLoadedMsg{repos: []bitbucket.Repository{{Slug: testRepo}}},
			wantCmd: true,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				testkit.AssertSliceEqual(t, "repo slugs", app.repoSlugs, []string{testRepo})
			},
		},
		{
			name: "pull requests loaded fills the list",
			msg: pullRequestsLoadedMsg{
				fetchKey: testFetchKey,
				results: []bitbucket.RepoPullRequests{
					{Repo: testRepo, PullRequests: []bitbucket.PullRequest{
						withAuthor(newPR(testRepo, 101, testTitle), testUserUUID),
					}},
				},
			},
			wantCmd: true,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				testkit.AssertEqual(t, "rows", app.prList.ItemCount(), 1)
			},
		},
		{
			name: "pull request detail loaded patches the caches",
			setup: func(app *App, _ *bitbuckettest.FakeClient) {
				app.previewEpoch = 1
			},
			msg: func() tea.Msg {
				pr := withAuthor(newPR(testRepo, 101, testTitle), testUserUUID)
				return pullRequestDetailLoadedMsg{pr: &pr, epoch: 1}
			}(),
			wantCmd: false,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				if _, ok := app.prCache[testPRKey]; !ok {
					t.Error("the detail should have been cached")
				}
			},
		},
		{
			name: "statuses loaded fill the builds panel",
			setup: func(app *App, _ *bitbuckettest.FakeClient) {
				app.previewKey = testPRKey
				app.buildsPanel.SetPullRequest(testPRKey)
			},
			msg: statusesLoadedMsg{
				key:      testPRKey,
				statuses: []bitbucket.CommitStatus{newStatus(testRepo, 412, bitbucket.StatusSuccessful)},
			},
			wantCmd: false,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				testkit.AssertEqual(t, "statuses", len(app.buildsPanel.Statuses()), 1)
			},
		},
		{
			name:    "a stale debounce tick is dropped",
			msg:     previewDebounceMsg{key: testPRKey, epoch: 99},
			wantCmd: false,
		},
		{
			name:    "a stale prefetch result is dropped",
			msg:     prefetchedMsg{key: testPRKey, epoch: 99},
			wantCmd: false,
		},
		{
			name: "pipeline loaded switches the detail to the run",
			setup: func(app *App, _ *bitbuckettest.FakeClient) {
				pr := newPR(testRepo, 101, testTitle)
				app.detailView.SetPullRequest(&pr)
				app.detailView.SetPipelineLoading(testPRKey, 412)
			},
			msg: pipelineLoadedMsg{
				key:      testPRKey,
				build:    412,
				pipeline: &bitbucket.Pipeline{BuildNumber: 412, UUID: "{run}"},
			},
			wantCmd: false,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModePipeline)
				if _, ok := app.pipelineCache["acme/api-gateway#412"]; !ok {
					t.Error("the run should have been cached")
				}
			},
		},
		{
			name: "step selected fetches its log",
			setup: func(app *App, fake *bitbuckettest.FakeClient) {
				pr := newPR(testRepo, 101, testTitle)
				app.prList.SetPullRequests([]bitbucket.PullRequest{pr})
				app.detailView.SetPullRequest(&pr)
				app.detailView.SetPipeline(testPRKey, &bitbucket.Pipeline{UUID: "{run}", BuildNumber: 412},
					[]bitbucket.PipelineStep{{UUID: "{step}", Name: "Build"}})
				fake.GetPipelineStepLogFunc = func(context.Context, string, string, string, string,
					bitbucket.StepLogOptions,
				) (string, error) {
					return "log", nil
				}
			},
			msg:     views.StepSelectedMsg{PipelineUUID: "{run}", StepUUID: "{step}"},
			wantCmd: true,
		},
		{
			name: "step log loaded switches to the log view",
			setup: func(app *App, _ *bitbuckettest.FakeClient) {
				pr := newPR(testRepo, 101, testTitle)
				app.detailView.SetPullRequest(&pr)
				app.detailView.SetPipeline(testPRKey, &bitbucket.Pipeline{UUID: "{run}", BuildNumber: 412},
					[]bitbucket.PipelineStep{{UUID: "{step}", Name: "Build"}})
				app.detailView.SetStepLogLoading("{run}", "{step}")
			},
			msg:     stepLogLoadedMsg{pipelineUUID: "{run}", stepUUID: "{step}", text: "hello"},
			wantCmd: false,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModeStepLog)
			},
		},
		{
			name: "pull request selected previews it",
			msg: func() tea.Msg {
				pr := newPR(testRepo, 101, testTitle)
				return views.PullRequestSelectedMsg{PullRequest: &pr}
			}(),
			wantCmd: true,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				testkit.AssertEqual(t, "previewKey", app.previewKey, testPRKey)
				testkit.AssertEqual(t, "builds panel key", app.buildsPanel.Key(), testPRKey)
			},
		},
		{
			name:    "a nil pull request selection is ignored",
			msg:     views.PullRequestSelectedMsg{},
			wantCmd: false,
		},
		{
			name:    "repo hovered while the repos panel is focused",
			setup:   func(app *App, _ *bitbuckettest.FakeClient) { app.leftFocus = focusRepos },
			msg:     views.RepoHoveredMsg{Repo: &views.RepoItem{Slug: testRepo}},
			wantCmd: false,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModeRepo)
			},
		},
		{
			name:    "repo hovered elsewhere is ignored",
			msg:     views.RepoHoveredMsg{Repo: &views.RepoItem{Slug: testRepo}},
			wantCmd: false,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				testkit.AssertEqual(t, "mode unchanged", app.detailView.Mode(), views.ModePullRequest)
			},
		},
		{
			name:    "expand block opens the read-only modal",
			msg:     views.ExpandBlockMsg{Title: "Details", Lines: []string{"a", "b"}},
			wantCmd: false,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				if !app.modal.IsVisible() {
					t.Error("expanding a block should open the modal")
				}
			},
		},
		{
			name:    "git error reaches the status panel",
			msg:     gitErrorMsg{err: errors.New(testBoom)},
			wantCmd: false,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				testkit.AssertEqual(t, "status error", app.statusPanel.ErrorMessage(), testBoom)
			},
		},
		{
			name:    "checkout done reports the branch",
			msg:     gitCheckoutDoneMsg{name: testBranch},
			wantCmd: false,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				testkit.AssertEqual(t, "git branch", app.gitBranch, testBranch)
			},
		},
		{
			name:    "an error opens the modal and marks the app offline",
			msg:     errorMsg{err: errors.New(testBoom)},
			wantCmd: false,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				testkit.AssertEqual(t, "status error", app.statusPanel.ErrorMessage(), testBoom)
				if !app.modal.IsVisible() {
					t.Error("an errorMsg should open the error modal")
				}
			},
		},
		{
			name:    "a custom command result reaches the help bar",
			msg:     customCommandFinishedMsg{output: "done"},
			wantCmd: true,
		},
		{
			name: "a changed-file list reaches the Files tab",
			setup: func(app *App, _ *bitbuckettest.FakeClient) {
				routingSelect(app)
				app.detailView.SetActiveTab(views.TabFiles)
			},
			msg: diffStatLoadedMsg{
				key: testPRKey, entries: testDiffStat(), hash: testSourceHash, err: nil,
			},
			wantCmd: false,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				testkit.AssertEqual(t, "changed files", len(app.detailView.DiffStat()), 2)
			},
		},
		{
			name: "a commit list reaches the Commits tab",
			setup: func(app *App, _ *bitbuckettest.FakeClient) {
				routingSelect(app)
				app.detailView.SetActiveTab(views.TabCommits)
			},
			msg: commitsLoadedMsg{
				key: testPRKey, commits: testCommits(), hash: testSourceHash, err: nil,
			},
			wantCmd: false,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				testkit.AssertEqual(t, "commits", len(app.detailView.Commits()), 1)
			},
		},
		{
			name: "review threads reach the Comments tab",
			setup: func(app *App, _ *bitbuckettest.FakeClient) {
				routingSelect(app)
				app.detailView.SetActiveTab(views.TabComments)
			},
			msg: threadsLoadedMsg{
				key: testPRKey, threads: testThreads(), hash: testSourceHash, err: nil,
			},
			wantCmd: false,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				testkit.AssertEqual(t, "threads", len(app.detailView.Threads()), 1)
			},
		},
		{
			name: "a parsed file diff reaches the diff view",
			setup: func(app *App, _ *bitbuckettest.FakeClient) {
				routingSelect(app)
				app.detailView.SetFileDiffLoading(testPRKey, testFileGo)
			},
			msg:     routingFileDiffMsg(),
			wantCmd: false,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				testkit.AssertEqual(t, "file on screen", app.detailView.FileDiffPath(), testFileGo)
			},
		},
		{
			name: "a file selection opens its diff",
			setup: func(app *App, fake *bitbuckettest.FakeClient) {
				stubDiff(fake)
				stubComments(fake)
				routingSelect(app)
				app.detailView.SetDiffStat(testPRKey, testDiffStat())
			},
			msg:     views.FileSelectedMsg{PullRequestKey: testPRKey, Path: testFileGo},
			wantCmd: true,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModeFileDiff)
			},
		},
		{
			name: "a thread selection jumps to the Comments tab",
			setup: func(app *App, _ *bitbuckettest.FakeClient) {
				routingSelect(app)
				app.detailView.SetThreads(testPRKey, testThreads())
			},
			msg:     views.ThreadSelectedMsg{PullRequestKey: testPRKey, RootID: testThreadRoot},
			wantCmd: false,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				testkit.AssertEqual(t, "tab", app.detailView.ActiveTab(), views.TabComments)
			},
		},
		{
			name:    "a finished editor re-arms the mouse",
			msg:     editorFinishedMsg{err: nil},
			wantCmd: true,
		},
		{
			name: "a cancelled form drops every callback",
			setup: func(app *App, _ *bitbuckettest.FakeClient) {
				app.armForm(formMerge, func(components.FormConfirmedMsg) tea.Cmd { return nil })
				app.onSelect = func(components.ModalItem) tea.Cmd { return nil }
			},
			msg:     components.FormCancelledMsg{ID: formMerge},
			wantCmd: false,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				if app.onForm != nil || app.onSelect != nil {
					t.Error("cancelling an overlay must clear every callback slot")
				}
			},
		},
		{
			name: "a confirmed form runs its callback once",
			setup: func(app *App, _ *bitbuckettest.FakeClient) {
				app.armForm(formDraft, func(components.FormConfirmedMsg) tea.Cmd { return tea.Quit })
			},
			msg:     components.FormConfirmedMsg{ID: formDraft, Fields: nil},
			wantCmd: true,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				if app.onForm != nil {
					t.Error("a confirmed form must drop its callback")
				}
			},
		},
		{
			name: "a cancelled input drops every callback",
			setup: func(app *App, _ *bitbuckettest.FakeClient) {
				app.onInput = func(string) tea.Cmd { return nil }
			},
			msg:     components.InputCancelledMsg{},
			wantCmd: false,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				if app.onInput != nil {
					t.Error("cancelling the input must clear its callback")
				}
			},
		},
		{
			name:    "a confirmed input without a callback is ignored",
			msg:     components.InputConfirmedMsg{Text: testTitle},
			wantCmd: false,
		},
		{
			name:    "a confirmed checklist without a callback is ignored",
			msg:     components.ChecklistConfirmedMsg{Selected: nil},
			wantCmd: false,
		},
		{
			name:    "a failed description edit reports the editor and re-arms the mouse",
			msg:     descriptionEditedMsg{key: testPRKey, path: "", original: "", err: errors.New(testBoom)},
			wantCmd: true,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				if !strings.Contains(app.statusPanel.ErrorMessage(), testBoom) {
					t.Errorf("status error = %q, want the editor failure", app.statusPanel.ErrorMessage())
				}
			},
		},
		{
			name:    "a queued merge schedules the next poll",
			msg:     mergeQueuedMsg{key: testPRKey, taskURL: testTaskURL, attempt: 0},
			wantCmd: true,
		},
		{
			name:    "a write failure reaches the help bar",
			msg:     pullRequestWrittenMsg{key: testPRKey, verb: verbEdit, pr: nil, err: errors.New(testBoom)},
			wantCmd: false,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				if got := plain(app.helpBar.View()); !strings.Contains(got, "edit failed: "+testBoom) {
					t.Errorf("help bar = %q, want the failure line", got)
				}
			},
		},
		{
			name:    "a sync result reaches the help bar",
			msg:     syncDoneMsg{key: testPRKey, summary: "synced", branch: "", pushed: false, err: nil},
			wantCmd: false,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				if got := plain(app.helpBar.View()); !strings.Contains(got, "synced") {
					t.Errorf("help bar = %q, want the summary", got)
				}
			},
		},
		{
			name:    "reviewer candidates for another pull request are dropped",
			msg:     reviewerCandidatesMsg{key: testPRKey2, candidates: nil, note: "", err: nil},
			wantCmd: false,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				if app.modal.IsVisible() {
					t.Error("candidates for a pull request nobody is looking at must open nothing")
				}
			},
		},
		{
			name: "an unclaimed key falls through to the focused panel",
			setup: func(app *App, _ *bitbuckettest.FakeClient) {
				app.prList.SetPullRequests([]bitbucket.PullRequest{
					newPR(testRepo, 101, "alpha"),
					newPR(testRepo, 102, "beta"),
				})
				app.updateFocusState()
			},
			msg:     runeKey('j'),
			wantCmd: true,
			assert: func(t *testing.T, app *App) {
				t.Helper()
				sel := app.prList.SelectedPullRequest()
				if sel == nil || sel.ID != 102 {
					t.Errorf("cursor = %v, want #102", sel)
				}
			},
		},
	})
}

// routingSelect points the detail panel at one pull request, which is what the
// review messages are addressed to.
func routingSelect(app *App) {
	pr := withSource(withAuthor(newPR(testRepo, 101, testTitle), testUserUUID), testSourceHash)
	app.merged[testFetchKey] = []bitbucket.PullRequest{pr}
	app.prList.SetPullRequests(app.merged[testFetchKey])
	app.prCache[pr.Key()] = &pr
	app.previewKey = pr.Key()
	app.detailView.SetPullRequest(&pr)
	app.side = sideRight
	app.updateFocusState()
}

// routingFileDiffMsg is one file of the fixture diff, parsed.
func routingFileDiffMsg() fileDiffLoadedMsg {
	files, _ := diff.Parse(testDiffText)
	return fileDiffLoadedMsg{
		key: testPRKey, path: testFileGo, file: pickDiffFile(files, testFileGo),
		hash: testSourceHash, err: nil,
	}
}

func TestUpdate_SearchBarSwallowsTypingButNotArrows(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	app.prList.SetPullRequests([]bitbucket.PullRequest{
		newPR(testRepo, 101, "alpha"),
		newPR(testRepo, 102, "beta"),
	})
	app.updateFocusState()
	app.searchBar.Activate()

	_, _ = app.Update(runeKey('q'))
	if app.searchBar.Query() != "q" {
		t.Errorf("query = %q, want the typed rune", app.searchBar.Query())
	}

	_, _ = app.Update(tea.KeyMsg{Type: tea.KeyDown})
	sel := app.prList.SelectedPullRequest()
	if sel == nil || sel.ID != 102 {
		t.Errorf("arrow keys should still reach the list, cursor = %v", sel)
	}
}

func TestUpdate_ModalInterceptsBeforeAnyHandler(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	app.modal.Show("URLs", []components.ModalItem{{ID: "https://example.test", Label: "example"}})

	_, cmd := app.Update(runeKey('q'))

	// q is quit everywhere else. Inside the modal it is the modal's own cancel
	// key, and the quit handler never sees it.
	if app.modal.IsVisible() {
		t.Error("q inside the modal should close it")
	}
	msgs := collectMsgs(cmd)
	if len(msgs) != 1 {
		t.Fatalf("messages = %v, want exactly the modal's cancellation", msgs)
	}
	if _, ok := msgs[0].(components.ModalCancelledMsg); !ok {
		t.Errorf("message = %T, want ModalCancelledMsg rather than a quit", msgs[0])
	}
}

func TestUpdate_FormAndInputInterceptKeysToo(t *testing.T) {
	t.Parallel()

	t.Run("the form swallows navigation", func(t *testing.T) {
		t.Parallel()
		app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
		app.form.Show("t", "Title", "confirm", []components.FormField{
			{ID: "a", Label: "A", Kind: components.FormToggle},
			{ID: "b", Label: "B", Kind: components.FormToggle},
		})

		_, _ = app.Update(runeKey('j'))
		_, _ = app.Update(runeKey(' '))

		if !app.form.IsVisible() {
			t.Fatal("the form should still be open")
		}
		if !strings.Contains(plain(app.form.View()), "[x]") {
			t.Error("space should have toggled the row the cursor moved to")
		}
	})

	t.Run("the input swallows typing", func(t *testing.T) {
		t.Parallel()
		app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
		app.input.Show("Title", "")

		_, _ = app.Update(runeKey('q'))

		if !app.input.IsVisible() {
			t.Fatal("q must be typed into the input, not quit the app")
		}
		if !strings.Contains(plain(app.input.View()), "q") {
			t.Error("the typed rune should be in the input")
		}
	})
}

func TestUpdate_ModalSelectionRunsTheCallbackOnce(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	calls := 0
	app.onSelect = func(components.ModalItem) tea.Cmd {
		calls++
		return nil
	}

	_, _ = app.Update(components.ModalSelectedMsg{Item: components.ModalItem{ID: "x"}})
	_, _ = app.Update(components.ModalSelectedMsg{Item: components.ModalItem{ID: "x"}})

	testkit.AssertEqual(t, "callback calls", calls, 1)
}

func TestUpdate_ModalCancelDropsTheCallback(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	app.onSelect = func(components.ModalItem) tea.Cmd { return nil }

	_, _ = app.Update(components.ModalCancelledMsg{})

	if app.onSelect != nil {
		t.Error("cancelling the modal must drop the pending callback")
	}
}
