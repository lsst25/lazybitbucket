package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/config"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

func appForKeybindings(t *testing.T) *App {
	t.Helper()
	return sizedApp(t, &bitbuckettest.FakeClient{T: t})
}

func TestContextBindings_ContainsQuit(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		setup func(*App)
	}{
		{"pull requests focus", func(app *App) { app.side, app.leftFocus = sideLeft, focusPullRequests }},
		{"builds focus", func(app *App) { app.side, app.leftFocus = sideLeft, focusBuilds }},
		{"repos focus", func(app *App) { app.side, app.leftFocus = sideLeft, focusRepos }},
		{"status focus", func(app *App) { app.side, app.leftFocus = sideLeft, focusStatus }},
		{"detail side", func(app *App) { app.side = sideRight }},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			app := appForKeybindings(t)
			testCase.setup(app)

			bindings := app.ContextBindings()

			if len(bindings) == 0 {
				t.Fatal("expected non-empty bindings")
			}
			if !hasDescription(bindings, string(ActQuit)) {
				t.Error("quit binding missing from the context bindings")
			}
		})
	}
}

func TestContextBindings_PipelineModeOffersBack(t *testing.T) {
	t.Parallel()
	app := appForKeybindings(t)
	app.side = sideRight
	pr := newPR(testRepo, 101, testTitle)
	app.detailView.SetPullRequest(&pr)
	app.detailView.SetPipelineLoading(pr.Key(), 412)

	bindings := app.ContextBindings()

	if !hasDescription(bindings, "back to the pull request") {
		t.Error("the pipeline view must offer a way back")
	}
}

func TestContextBindings_PullRequestModeOffersCheckout(t *testing.T) {
	t.Parallel()
	app := appForKeybindings(t)

	bindings := app.ContextBindings()

	if !hasDescription(bindings, "check out the source branch") {
		t.Error("the pull request list must offer checkout")
	}
}

func TestContextBindings_IncludesCustomCommands(t *testing.T) {
	t.Parallel()
	app := appForKeybindings(t)
	app.cfg.CustomCommands = []config.CustomCommandConfig{
		{Key: "F", Name: "open in fzf", Command: "echo {{.PR.ID}}", Contexts: []string{"builds"}},
	}
	app.initCustomCommands()
	app.leftFocus = focusBuilds

	if !hasDescription(app.ContextBindings(), "open in fzf") {
		t.Error("a builds-context custom command must appear in the builds bindings")
	}
}

func TestHelpBarItems_NotEmpty(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		setup func(*App)
	}{
		{"pull requests", func(app *App) { app.side, app.leftFocus = sideLeft, focusPullRequests }},
		{"builds", func(app *App) { app.side, app.leftFocus = sideLeft, focusBuilds }},
		{"repos", func(app *App) { app.side, app.leftFocus = sideLeft, focusRepos }},
		{"status", func(app *App) { app.side, app.leftFocus = sideLeft, focusStatus }},
		{"detail", func(app *App) { app.side = sideRight }},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			app := appForKeybindings(t)
			testCase.setup(app)

			if len(app.helpBarItems()) == 0 {
				t.Error("expected non-empty help bar items")
			}
		})
	}
}

func TestHelpBarItems_PipelineModeShowsStepLog(t *testing.T) {
	t.Parallel()
	app := appForKeybindings(t)
	app.side = sideRight
	pr := newPR(testRepo, 101, testTitle)
	app.detailView.SetPullRequest(&pr)
	app.detailView.SetPipeline(pr.Key(), &bitbucket.Pipeline{BuildNumber: 412}, nil)

	items := app.helpBarItems()

	found := false
	for _, item := range items {
		if item.Description == "step log" {
			found = true
		}
	}
	if !found {
		t.Errorf("pipeline help bar = %v, want a step log hint", items)
	}
}

func TestHelpBarItems_ModalTakesPriority(t *testing.T) {
	t.Parallel()
	app := appForKeybindings(t)
	app.modal.Show("URLs", nil)

	items := app.helpBarItems()

	if len(items) == 0 || items[0].Description != "search" {
		t.Errorf("modal help bar = %v, want the modal hints", items)
	}
}

func TestNavBindings_HasSixEntries(t *testing.T) {
	t.Parallel()
	app := appForKeybindings(t)

	if got := len(app.navBindings()); got != 6 {
		t.Errorf("navBindings len = %d, want 6", got)
	}
}

func TestDetailScrollBindings_HasFourEntries(t *testing.T) {
	t.Parallel()
	app := appForKeybindings(t)

	if got := len(app.detailScrollBindings()); got != 4 {
		t.Errorf("detailScrollBindings len = %d, want 4", got)
	}
}

func TestBind_CarriesKeyAndDescription(t *testing.T) {
	t.Parallel()
	app := appForKeybindings(t)

	b := app.bind(ActQuit, "quit the app")

	if b.Description != "quit the app" || b.Key == "" {
		t.Errorf("bind = %+v, want a key and the given description", b)
	}
}

func TestActiveContexts_FollowFocusAndDetailMode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		setup func(*App)
		want  []config.Context
	}{
		{"pull requests", func(app *App) { app.leftFocus = focusPullRequests }, []config.Context{config.CtxPullRequests}},
		{"builds", func(app *App) { app.leftFocus = focusBuilds }, []config.Context{config.CtxBuilds}},
		{"repos", func(app *App) { app.leftFocus = focusRepos }, []config.Context{config.CtxRepos}},
		{"status", func(app *App) { app.leftFocus = focusStatus }, nil},
		{
			name: "detail pull request",
			setup: func(app *App) {
				app.side = sideRight
				pr := newPR(testRepo, 101, testTitle)
				app.detailView.SetPullRequest(&pr)
			},
			want: []config.Context{config.CtxDetail},
		},
		{
			name: "detail files tab",
			setup: func(app *App) {
				app.side = sideRight
				pr := newPR(testRepo, 101, testTitle)
				app.detailView.SetPullRequest(&pr)
				app.detailView.SetActiveTab(views.TabFiles)
			},
			want: []config.Context{config.CtxDiff, config.CtxDetail},
		},
		{
			name: "detail file diff",
			setup: func(app *App) {
				app.side = sideRight
				pr := newPR(testRepo, 101, testTitle)
				app.detailView.SetPullRequest(&pr)
				app.detailView.SetFileDiffLoading(pr.Key(), testFileGo)
			},
			want: []config.Context{config.CtxDiff, config.CtxDetail},
		},
		{
			name: "detail repo",
			setup: func(app *App) {
				app.side = sideRight
				app.detailView.SetRepo(&views.RepoItem{Slug: testRepo})
			},
			want: []config.Context{config.CtxRepos},
		},
		{
			name: "detail splash",
			setup: func(app *App) {
				app.side = sideRight
				app.detailView.SetSplash(views.SplashInfo{Version: "test"})
			},
			want: nil,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			app := appForKeybindings(t)
			testCase.setup(app)

			got := app.activeContexts()

			if len(got) != len(testCase.want) {
				t.Fatalf("contexts = %v, want %v", got, testCase.want)
			}
			for i := range got {
				if got[i] != testCase.want[i] {
					t.Errorf("contexts[%d] = %q, want %q", i, got[i], testCase.want[i])
				}
			}
		})
	}
}

func hasDescription(bindings []Binding, want string) bool {
	for _, b := range bindings {
		if b.Description == want {
			return true
		}
	}
	return false
}

func TestContextBindings_FollowTheDetailTabAndMode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		setup func(app *App, pr *bitbucket.PullRequest)
		want  []string
	}{
		{
			name:  "files tab",
			setup: func(app *App, _ *bitbucket.PullRequest) { app.detailView.SetActiveTab(views.TabFiles) },
			want:  []string{"open this file's diff", "next changed file", "open this file in the editor"},
		},
		{
			name:  "commits tab",
			setup: func(app *App, _ *bitbucket.PullRequest) { app.detailView.SetActiveTab(views.TabCommits) },
			want:  []string{"expand the commit message", "copy the commit URL"},
		},
		{
			name:  "comments tab",
			setup: func(app *App, _ *bitbucket.PullRequest) { app.detailView.SetActiveTab(views.TabComments) },
			want:  []string{"open the thread", "open the thread in a browser"},
		},
		{
			name: "file diff",
			setup: func(app *App, pr *bitbucket.PullRequest) {
				app.detailView.SetFileDiffLoading(pr.Key(), testFileGo)
			},
			want: []string{"back to the changed files", "open this line in the editor", "next changed file"},
		},
		{
			name: "thread",
			setup: func(app *App, pr *bitbucket.PullRequest) {
				app.detailView.SetThreads(pr.Key(), testThreads())
				app.detailView.SetActiveTab(views.TabComments)
				// Enter reads the block list, which the first render fills in.
				app.detailView.SetSize(80, 40)
				app.detailView.View()
				app.detailView.Activate()
			},
			want: []string{"back to the comments", "open the comment in a browser", "copy the comment URL"},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			app := appForKeybindings(t)
			app.side = sideRight
			pr := newPR(testRepo, 101, testTitle)
			app.detailView.SetPullRequest(&pr)
			testCase.setup(app, &pr)

			bindings := app.ContextBindings()

			for _, want := range testCase.want {
				if !hasDescription(bindings, want) {
					t.Errorf("the help overlay is missing %q", want)
				}
			}
			// Every tab offers the four jumps.
			if !hasDescription(bindings, "commits tab") {
				t.Error("the help overlay should list the tab jumps")
			}
		})
	}
}

func TestHelpBarItems_FollowTheDetailTabAndMode(t *testing.T) {
	t.Parallel()
	app := appForKeybindings(t)
	app.side = sideRight
	pr := newPR(testRepo, 101, testTitle)
	app.detailView.SetPullRequest(&pr)

	app.detailView.SetActiveTab(views.TabFiles)
	if !hasHelpDescription(app.helpBarItems(), "editor") {
		t.Error("the Files tab strip should name the editor key")
	}

	app.detailView.SetFileDiffLoading(pr.Key(), testFileGo)
	items := app.helpBarItems()
	if !hasHelpDescription(items, "file") || !hasHelpDescription(items, "editor") {
		t.Errorf("the diff strip should name the file and editor keys: %+v", items)
	}
}

func hasHelpDescription(items []components.HelpItem, want string) bool {
	for _, item := range items {
		if item.Description == want {
			return true
		}
	}
	return false
}

// hasBindingKey reports whether any binding in the list is bound to key.
func hasBindingKey(bindings []Binding, key string) bool {
	for _, binding := range bindings {
		if binding.Key == key {
			return true
		}
	}
	return false
}

// hasHelpKey reports whether any help item in the strip is bound to key.
func hasHelpKey(items []components.HelpItem, key string) bool {
	for _, item := range items {
		if item.Key == key {
			return true
		}
	}
	return false
}

func TestWriteBindings_ListedWhenWritableAndOmittedWhenNot(t *testing.T) {
	t.Parallel()

	writeKeys := []string{"M", "S", "E", "V", "D", "A"}

	t.Run("the pull request list lists all six", func(t *testing.T) {
		t.Parallel()
		app := appForKeybindings(t)
		app.side, app.leftFocus = sideLeft, focusPullRequests

		bindings := app.ContextBindings()
		items := app.helpBarItems()

		for _, key := range writeKeys {
			if !hasBindingKey(bindings, key) {
				t.Errorf("the help overlay should list %q", key)
			}
			if !hasHelpKey(items, key) {
				t.Errorf("the help bar should list %q", key)
			}
		}
	})

	t.Run("the detail panel lists them too", func(t *testing.T) {
		t.Parallel()
		app := appForKeybindings(t)
		pr := newPR(testRepo, 101, testTitle)
		app.detailView.SetPullRequest(&pr)
		app.side = sideRight

		bindings := app.ContextBindings()

		for _, key := range writeKeys {
			if !hasBindingKey(bindings, key) {
				t.Errorf("the detail help overlay should list %q", key)
			}
		}
	})

	t.Run("read-only mode omits them everywhere", func(t *testing.T) {
		t.Parallel()
		app := appForKeybindings(t)
		app.readOnly = true
		app.side, app.leftFocus = sideLeft, focusPullRequests

		bindings := app.ContextBindings()
		items := app.helpBarItems()

		for _, key := range writeKeys {
			if hasBindingKey(bindings, key) {
				t.Errorf("read-only mode must not advertise %q", key)
			}
			if hasHelpKey(items, key) {
				t.Errorf("read-only mode must not advertise %q on the help bar", key)
			}
		}
	})
}

func TestHelpBarItems_FollowTheOpenOverlay(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		setup func(*App)
		want  []string
	}{
		{
			name: "a form names its own keys",
			setup: func(app *App) {
				app.form.Show(formSync, "Sync", verbSync, []components.FormField{
					{ID: fieldPush, Label: "Push", Kind: components.FormToggle},
				})
			},
			want: []string{"move", "change", "toggle", "confirm", "cancel"},
		},
		{
			name: "an inline text edit says keep and revert",
			setup: func(app *App) {
				app.form.Show(formMerge, "Merge", verbMerge, []components.FormField{
					{ID: fieldMessage, Label: "Message", Kind: components.FormText},
				})
				_, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
			},
			want: []string{"keep", "revert"},
		},
		{
			name:  "the input says confirm and cancel",
			setup: func(app *App) { app.input.Show("Title", testTitle) },
			want:  []string{"confirm", "cancel"},
		},
		{
			name: "the checklist saves rather than selects",
			setup: func(app *App) {
				app.modal.ShowChecklist("Reviewers", []components.ModalItem{{ID: "a", Label: "A"}}, nil)
			},
			want: []string{"toggle", "save", "filter", "cancel"},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			app := appForKeybindings(t)
			testCase.setup(app)

			items := app.helpBarItems()

			for _, want := range testCase.want {
				found := false
				for _, item := range items {
					if item.Description == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("help strip %v is missing %q", items, want)
				}
			}
		})
	}
}
