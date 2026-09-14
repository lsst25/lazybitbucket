package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
)

// createKey is the N key as the keymap resolves it.
const createKey = 'N'

// testNewBranch is the branch a create is opened from, and testDefaultBranch
// the repository's own default.
const (
	testNewBranch     = "feature/rate-limiter"
	testDefaultBranch = "main"
)

// createdPullRequest is what a stubbed CreatePullRequest answers with.
func createdPullRequest(create bitbucket.NewPullRequest) *bitbucket.PullRequest {
	pr := newPR(testRepo, 104, create.Title)
	pr.Draft = create.Draft
	pr.Source.Branch.Name = create.SourceBranch
	pr.Destination.Branch.Name = create.DestinationBranch
	pr.Description = create.Description
	for _, uuid := range create.Reviewers {
		pr.Reviewers = append(pr.Reviewers, bitbucket.Account{UUID: uuid, DisplayName: uuid})
	}
	return &pr
}

// createApp is an app that looks like it was started inside a checkout of the
// configured repository, on a feature branch.
func createApp(t *testing.T, fake *bitbuckettest.FakeClient) *App {
	t.Helper()
	app := sizedApp(t, fake)
	app.repos = []bitbucket.Repository{
		{Slug: testRepo, FullName: testWorkspace + "/" + testRepo},
	}
	app.gitRepoRoot = t.TempDir()
	app.gitRepoPath = app.gitRepoRoot
	app.gitBranch = testNewBranch
	app.gitRemoteRepos = []string{testWorkspace + "/" + testRepo}
	// A loaded, empty Open tab: a create puts what it opened into the lists
	// that are already in hand, and there is nothing to put it into otherwise.
	app.merged[testFetchKey] = []bitbucket.PullRequest{}
	app.refilterTabs("")
	return app
}

// stubCreatePrefill answers the two reads behind the form.
func stubCreatePrefill(fake *bitbuckettest.FakeClient, defaults []bitbucket.Account) {
	fake.GetRepositoryFunc = func(_ context.Context, workspace, slug string) (*bitbucket.Repository, error) {
		return &bitbucket.Repository{
			Slug: slug, FullName: workspace + "/" + slug,
			MainBranch: &bitbucket.Branch{Name: testDefaultBranch},
		}, nil
	}
	fake.ListEffectiveDefaultReviewersFunc = func(context.Context, string, string) ([]bitbucket.Account, error) {
		return defaults, nil
	}
}

// stubCreate answers the create itself.
func stubCreate(fake *bitbuckettest.FakeClient) {
	fake.CreatePullRequestFunc = func(_ context.Context, _, _ string,
		create bitbucket.NewPullRequest,
	) (*bitbucket.PullRequest, error) {
		return createdPullRequest(create), nil
	}
}

// openCreateForm presses N and settles the reads behind the form.
func openCreateForm(t *testing.T, app *App) {
	t.Helper()
	_, cmd := app.Update(runeKey(createKey))
	runAll(t, app, cmd)
	if !app.form.IsVisible() {
		t.Fatalf("N should open the create form, help bar = %q", plain(app.helpBar.View()))
	}
}

// confirmCreateForm answers the open form, overriding the fields the test cares
// about: every other one keeps what the prefill put there.
func confirmCreateForm(t *testing.T, app *App, override func(field *components.FormField)) {
	t.Helper()
	fields := app.form.Fields()
	for i := range fields {
		override(&fields[i])
	}
	app.form.Hide()
	_, cmd := app.Update(components.FormConfirmedMsg{ID: formCreate, Fields: fields})
	runAll(t, app, cmd)
}

func TestCreate_FormIsPrefilledFromTheBranchAndTheRepository(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubCreatePrefill(fake, []bitbucket.Account{{UUID: testOtherUUID, DisplayName: "Ada Lovelace"}})
	app := createApp(t, fake)

	openCreateForm(t, app)

	rendered := plain(app.form.View())
	// The repository it will be opened in, and the branch it is opened from.
	if !strings.Contains(rendered, testRepo) {
		t.Errorf("the form should name the repository:\n%s", rendered)
	}
	if !strings.Contains(rendered, testNewBranch) {
		t.Errorf("the form should name the source branch:\n%s", rendered)
	}
	// The repository's own default branch is the target, not a guess.
	if !strings.Contains(rendered, testDefaultBranch) {
		t.Errorf("the form should propose the default branch:\n%s", rendered)
	}
	// The repository has one default reviewer, and the option says so.
	if !strings.Contains(rendered, "default reviewers (1)") {
		t.Errorf("the form should offer the default reviewers:\n%s", rendered)
	}
}

func TestCreate_ConfirmingOpensThePullRequestAndShowsIt(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubCreatePrefill(fake, nil)
	stubCreate(fake)
	stubPreview(fake)
	app := createApp(t, fake)
	openCreateForm(t, app)

	confirmCreateForm(t, app, func(field *components.FormField) {
		if field.ID == fieldTitleCreate {
			field.Value = "Add a token bucket rate limiter"
		}
		if field.ID == fieldDraftCreate {
			field.On = true
		}
	})

	if len(fake.CreatePullRequestCalls) != 1 {
		t.Fatalf("CreatePullRequest calls = %d, want 1", len(fake.CreatePullRequestCalls))
	}
	call := fake.CreatePullRequestCalls[0]
	testkit.AssertEqual(t, "workspace", call.Workspace, testWorkspace)
	testkit.AssertEqual(t, "repo", call.RepoSlug, testRepo)
	testkit.AssertEqual(t, "title", call.Create.Title, "Add a token bucket rate limiter")
	testkit.AssertEqual(t, "source", call.Create.SourceBranch, testNewBranch)
	testkit.AssertEqual(t, "target", call.Create.DestinationBranch, testDefaultBranch)
	testkit.AssertEqual(t, "draft", call.Create.Draft, true)

	// What was created is on screen: in the cache, in the list, and previewed.
	created := app.prCache[testWorkspace+"/"+testRepo+"#104"]
	if created == nil {
		t.Fatal("the created pull request should be cached")
	}
	selected := app.prList.SelectedPullRequest()
	if selected == nil {
		t.Fatal("the new pull request should be under the cursor")
	}
	testkit.AssertEqual(t, "selected", selected.Key(), created.Key())
	testkit.AssertEqual(t, "previewed", app.detailView.PullRequestKey(), created.Key())
	if got := plain(app.helpBar.View()); !strings.Contains(got, "opened #104") {
		t.Errorf("help bar = %q, want it to name the new pull request", got)
	}
	if app.pendingCreate != nil {
		t.Error("the create should be over")
	}
}

func TestCreate_TheDefaultReviewersChoiceSendsThem(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubCreatePrefill(fake, []bitbucket.Account{
		{UUID: testOtherUUID, DisplayName: "Ada Lovelace"},
		{UUID: testThirdUUID, DisplayName: "Ken Thompson"},
	})
	stubCreate(fake)
	stubPreview(fake)
	app := createApp(t, fake)
	openCreateForm(t, app)

	confirmCreateForm(t, app, func(field *components.FormField) {
		if field.ID == fieldReviewers {
			field.Index = 0 // "default reviewers (2)" leads when there are any
		}
	})

	create := fake.CreatePullRequestCalls[0].Create
	testkit.AssertEqual(t, "reviewers", len(create.Reviewers), 2)
	testkit.AssertEqual(t, "first reviewer", create.Reviewers[0], testOtherUUID)
}

func TestCreate_ThePickerChoiceOpensTheChecklistFirst(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubCreatePrefill(fake, []bitbucket.Account{{UUID: testOtherUUID, DisplayName: "Ada Lovelace"}})
	stubCreate(fake)
	stubPreview(fake)
	fake.ListWorkspaceMembersFunc = func(context.Context, string, bitbucket.ListOptions) ([]bitbucket.Account, error) {
		return []bitbucket.Account{
			{UUID: testOtherUUID, DisplayName: "Ada Lovelace"},
			{UUID: testThirdUUID, DisplayName: "Ken Thompson"},
		}, nil
	}
	app := createApp(t, fake)
	openCreateForm(t, app)

	confirmCreateForm(t, app, func(field *components.FormField) {
		if field.ID == fieldReviewers {
			field.Index = len(field.Options) - 1 // "pick…" is last
		}
	})

	// Nothing is written yet: the picker is what the form asked for.
	testkit.AssertEqual(t, "calls before the picker", len(fake.CreatePullRequestCalls), 0)
	if !app.modal.IsVisible() {
		t.Fatal("the reviewer checklist should be open")
	}
	if app.pendingCreate == nil {
		t.Fatal("the pull request should still be pending")
	}

	// Confirming the checklist is what sends it.
	_, cmd := app.Update(components.ChecklistConfirmedMsg{
		Selected: []components.ModalItem{{ID: testThirdUUID, Label: "Ken Thompson"}},
	})
	runAll(t, app, cmd)

	if len(fake.CreatePullRequestCalls) != 1 {
		t.Fatalf("CreatePullRequest calls = %d, want 1", len(fake.CreatePullRequestCalls))
	}
	create := fake.CreatePullRequestCalls[0].Create
	testkit.AssertEqual(t, "reviewers", len(create.Reviewers), 1)
	testkit.AssertEqual(t, "reviewer", create.Reviewers[0], testThirdUUID)
}

func TestCreate_TheDescriptionChoiceOpensTheEditorFirst(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubCreatePrefill(fake, nil)
	stubCreate(fake)
	stubPreview(fake)
	app := createApp(t, fake)
	app.cfg.Editor = testEditor
	openCreateForm(t, app)

	confirmCreateForm(t, app, func(field *components.FormField) {
		if field.ID == fieldDescCreate {
			field.Index = 1 // "write in $EDITOR"
		}
	})

	// The editor has the terminal; nothing has been written yet.
	testkit.AssertEqual(t, "calls before the editor exits", len(fake.CreatePullRequestCalls), 0)
	if app.pendingCreate == nil {
		t.Fatal("the pull request should still be pending")
	}

	// The editor exits, leaving a description behind.
	path, err := writeDescriptionTempFile(testRepo, 0, "## Why\n\nBecause the gateway needs it.\n")
	if err != nil {
		t.Fatal(err)
	}
	_, cmd := app.Update(createDescriptionEditedMsg{path: path, err: nil})
	runAll(t, app, cmd)

	if len(fake.CreatePullRequestCalls) != 1 {
		t.Fatalf("CreatePullRequest calls = %d, want 1", len(fake.CreatePullRequestCalls))
	}
	testkit.AssertEqual(t, "description", fake.CreatePullRequestCalls[0].Create.Description,
		"## Why\n\nBecause the gateway needs it.")
}

func TestCreate_RefusalsBeforeAnythingIsRead(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		setup func(app *App)
		want  string
	}{
		{
			name:  "outside a checkout",
			setup: func(app *App) { app.gitRepoRoot = "" },
			want:  noCheckout,
		},
		{
			name:  "a checkout of another repository",
			setup: func(app *App) { app.gitRemoteRepos = []string{"someone/else"} },
			want:  noRepoMatch,
		},
		{
			name:  "a detached HEAD",
			setup: func(app *App) { app.gitBranch = "" },
			want:  noBranchHere,
		},
		{
			name:  "read-only mode",
			setup: func(app *App) { app.readOnly = true },
			want:  "read-only",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := &bitbuckettest.FakeClient{T: t}
			app := createApp(t, fake)
			tc.setup(app)

			_, cmd := app.Update(runeKey(createKey))
			runAll(t, app, cmd)

			if app.form.IsVisible() {
				t.Error("no form should open")
			}
			if got := plain(app.helpBar.View()); !strings.Contains(got, tc.want) {
				t.Errorf("help bar = %q, want %q", got, tc.want)
			}
			// Nothing was read either: these are answered before any request.
			testkit.AssertEqual(t, "repository reads", fake.CallCount("GetRepository"), 0)
		})
	}
}

func TestCreate_AnEmptyTitleIsRefusedWithoutARequest(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubCreatePrefill(fake, nil)
	app := createApp(t, fake)
	openCreateForm(t, app)

	confirmCreateForm(t, app, func(field *components.FormField) {
		if field.ID == fieldTitleCreate {
			field.Value = "   "
		}
	})

	testkit.AssertEqual(t, "calls", len(fake.CreatePullRequestCalls), 0)
	if got := plain(app.helpBar.View()); !strings.Contains(got, "title cannot be empty") {
		t.Errorf("help bar = %q, want the refusal", got)
	}
	if app.pendingCreate != nil {
		t.Error("the create should have been dropped")
	}
}

func TestCreate_ABranchAgainstItselfIsRefused(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubCreatePrefill(fake, nil)
	app := createApp(t, fake)
	openCreateForm(t, app)

	confirmCreateForm(t, app, func(field *components.FormField) {
		if field.ID == fieldTargetCreate {
			field.Value = testNewBranch
		}
	})

	testkit.AssertEqual(t, "calls", len(fake.CreatePullRequestCalls), 0)
	if got := plain(app.helpBar.View()); !strings.Contains(got, "against itself") {
		t.Errorf("help bar = %q, want the refusal", got)
	}
}

func TestCreate_AFailedCreateSaysWhyAndKeepsNothingPending(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubCreatePrefill(fake, nil)
	fake.CreatePullRequestFunc = func(context.Context, string, string,
		bitbucket.NewPullRequest,
	) (*bitbucket.PullRequest, error) {
		return nil, errors.New("Source branch does not exist in the repository.")
	}
	app := createApp(t, fake)
	openCreateForm(t, app)

	confirmCreateForm(t, app, func(*components.FormField) {})

	if got := plain(app.helpBar.View()); !strings.Contains(got, "Source branch does not exist") {
		t.Errorf("help bar = %q, want the server's own words", got)
	}
	if app.pendingCreate != nil {
		t.Error("a failed create should leave nothing pending")
	}
}

func TestCreate_APushThatFailsStopsBeforeTheCreate(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubCreatePrefill(fake, nil)
	app := createApp(t, fake)
	app.pendingCreate = &pendingCreate{
		workspace: testWorkspace, slug: testRepo, source: testNewBranch,
		target: testDefaultBranch, title: "Add a token bucket rate limiter",
		push: false, remote: remoteOrigin,
	}

	_, cmd := app.Update(branchPushedMsg{err: errors.New("failed to push some refs")})
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "calls", len(fake.CreatePullRequestCalls), 0)
	if got := plain(app.helpBar.View()); !strings.Contains(got, "push failed") {
		t.Errorf("help bar = %q, want the push failure", got)
	}
	if app.pendingCreate != nil {
		t.Error("a failed push should leave nothing pending")
	}
}

func TestCreate_TheFormOffersToPushAnUnpublishedBranch(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		onRemote bool
		offered  bool
	}{
		{name: "a branch the remote does not have", onRemote: false, offered: true},
		{name: "a branch that is already pushed", onRemote: true, offered: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fields := createFormFields(createPrefillMsg{
				workspace: testWorkspace, slug: testRepo, source: testNewBranch,
				target: testDefaultBranch, title: "x", remote: remoteOrigin,
				onRemote: tc.onRemote, defaults: nil, note: "", err: nil,
			})

			found := false
			for _, field := range fields {
				if field.ID == fieldPushCreate {
					found = true
					testkit.AssertEqual(t, "on by default", field.On, true)
				}
			}
			testkit.AssertEqual(t, "push row offered", found, tc.offered)
		})
	}
}

func TestCreate_KeyIsIgnoredInThePipelinesView(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	app := createApp(t, fake)
	app.view = viewPipelines

	_, cmd := app.Update(runeKey(createKey))
	runAll(t, app, cmd)

	if app.form.IsVisible() {
		t.Error("the pipelines view has no branch to open a pull request from")
	}
	testkit.AssertEqual(t, "repository reads", fake.CallCount("GetRepository"), 0)
}

func TestCreate_TheNewPullRequestLandsInTheOpenTabs(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	app := createApp(t, fake)
	existing := newPR(testRepo, 101, testTitle)
	app.merged["OPEN|"] = []bitbucket.PullRequest{existing}
	app.merged["MERGED|"] = []bitbucket.PullRequest{existing}

	created := newPR(testRepo, 104, "Add a token bucket rate limiter")
	app.addToOpenTabs(&created)

	testkit.AssertEqual(t, "open tab", len(app.merged["OPEN|"]), 2)
	testkit.AssertEqual(t, "newest first", app.merged["OPEN|"][0].ID, 104)
	// A merged-state tab is not a list a new pull request belongs to.
	testkit.AssertEqual(t, "merged tab", len(app.merged["MERGED|"]), 1)

	// Applying the same answer twice must not double it.
	app.addToOpenTabs(&created)
	testkit.AssertEqual(t, "open tab after a second apply", len(app.merged["OPEN|"]), 2)
}

func TestDemo_CreateOpensAPullRequestThroughTheRealRoutes(t *testing.T) {
	t.Parallel()
	app := demoApp(t)
	app.gitRepoRoot = t.TempDir()
	app.gitBranch = "feature/keyboard-nav"
	app.gitRemoteRepos = []string{"acme/web-app"}

	_, cmd := app.Update(runeKey(createKey))
	runAll(t, app, cmd)
	if !app.form.IsVisible() {
		t.Fatalf("N should open the create form, help bar = %q", plain(app.helpBar.View()))
	}
	// web-app's default branch is develop, which is what the repository says.
	if rendered := plain(app.form.View()); !strings.Contains(rendered, "develop") {
		t.Errorf("the form should propose the repository's default branch:\n%s", rendered)
	}

	fields := app.form.Fields()
	for i := range fields {
		if fields[i].ID == fieldTitleCreate {
			fields[i].Value = "Keyboard navigation, second pass"
		}
	}
	app.form.Hide()
	_, cmd = app.Update(components.FormConfirmedMsg{ID: formCreate, Fields: fields})
	runAll(t, app, cmd)

	// Through the real client and the real routes: the pull request exists, is
	// authored by the demo user and is what the panel is showing.
	selected := app.prList.SelectedPullRequest()
	if selected == nil {
		t.Fatalf("the new pull request should be under the cursor, help bar = %q", plain(app.helpBar.View()))
	}
	key := selected.Key()
	created := app.prCache[key]
	if created == nil {
		t.Fatalf("the created pull request should be cached, help bar = %q", plain(app.helpBar.View()))
	}
	testkit.AssertEqual(t, "title", created.Title, "Keyboard navigation, second pass")
	testkit.AssertEqual(t, "source", created.SourceBranch(), "feature/keyboard-nav")
	testkit.AssertEqual(t, "target", created.Destination.Branch.Name, "develop")
	testkit.AssertEqual(t, "previewed", app.detailView.PullRequestKey(), key)
}
