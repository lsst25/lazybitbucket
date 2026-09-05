package tui

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
)

// writeApp is one open pull request, selected and previewed, with a strict fake
// behind it: any client call the test did not stub fails the test.
func writeApp(t *testing.T, fake *bitbuckettest.FakeClient) *App {
	t.Helper()
	app := sizedApp(t, fake)
	pr := withReviewer(withAuthor(newPR(testRepo, 101, testTitle), testUserUUID), testOtherUUID)
	pr.Description = testDescription
	pr.CloseSourceBranch = true
	app.merged[testFetchKey] = []bitbucket.PullRequest{pr}
	app.prList.SetPullRequests(app.merged[testFetchKey])
	app.prCache[pr.Key()] = &pr
	app.previewKey = pr.Key()
	app.updateFocusState()
	return app
}

// bareApp is writeApp over a list payload that carried no reviewers and no
// participants: the reviewer set is unknown rather than empty, which is what
// the fields projection failing leaves behind.
func bareApp(t *testing.T, fake *bitbuckettest.FakeClient) *App {
	t.Helper()
	app := sizedApp(t, fake)
	pr := newPR(testRepo, 101, testTitle)
	pr.Author = bitbucket.Account{UUID: testUserUUID, DisplayName: testUserUUID}
	app.merged[testFetchKey] = []bitbucket.PullRequest{pr}
	app.prList.SetPullRequests(app.merged[testFetchKey])
	app.prCache[pr.Key()] = &pr
	app.previewKey = pr.Key()
	app.updateFocusState()
	return app
}

// selectedPR is the pull request writeApp put on screen.
func selectedPR(app *App) *bitbucket.PullRequest { return app.pullRequestByKey(testPRKey) }

// gitIn runs git inside dir and fails the test if it does not succeed.
func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	if err := runGit(t, append([]string{"-C", dir}, args...)); err != nil {
		t.Fatalf("git %v in %s: %v", args, dir, err)
	}
}

// gitOutput runs git inside dir and returns its trimmed output.
func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v in %s: %v", args, dir, err)
	}
	return strings.TrimSpace(string(out))
}

// identify gives a repository the identity a commit needs, independently of the
// machine's global git configuration.
func identify(t *testing.T, dir string) {
	t.Helper()
	gitIn(t, dir, "config", "user.email", "test@example.com")
	gitIn(t, dir, "config", "user.name", "test")
	gitIn(t, dir, "config", "commit.gpgsign", "false")
}

// commitIn writes a file and commits it.
func commitIn(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-q", "-m", name)
}

// syncRepo builds the shape a sync needs and no network: a bare "remote"
// holding a main branch with a commit the checkout has never seen, and a
// working clone sitting on the pull request's source branch.
func syncRepo(t *testing.T) (remote, work string) {
	t.Helper()

	// The bare remote's path ends in <workspace>/<slug>.git, which is what
	// RemoteFor matches a remote URL against.
	remote = filepath.Join(t.TempDir(), testWorkspace, testRepo+".git")
	if err := os.MkdirAll(remote, 0o750); err != nil {
		t.Fatal(err)
	}
	gitIn(t, remote, "init", "--bare", "-q", "-b", branchMain)

	seed := t.TempDir()
	gitIn(t, seed, "init", "-q", "-b", branchMain)
	identify(t, seed)
	commitIn(t, seed, "README", "one\n")
	gitIn(t, seed, "remote", "add", remoteOrigin, remote)
	gitIn(t, seed, "push", "-q", remoteOrigin, branchMain)

	work = t.TempDir()
	gitIn(t, work, "clone", "-q", remote, work)
	identify(t, work)
	gitIn(t, work, "checkout", "-q", "-b", testBranch)
	commitIn(t, work, "feature.txt", "f\n")

	// A commit on main that only the remote has, so the sync has something to
	// bring in.
	commitIn(t, seed, "main.txt", "two\n")
	gitIn(t, seed, "push", "-q", remoteOrigin, branchMain)

	return remote, work
}

// apiError is a Bitbucket failure shaped the way the client hands one over.
func apiError(status int, message string, fields map[string][]string) error {
	return &bitbucket.APIError{Method: http.MethodPost, Path: "/x", Status: status,
		Message: message, Fields: fields}
}

// -----------------------------------------------------------------------------
// The guard every write key goes through
// -----------------------------------------------------------------------------

func TestWriteTarget_Refusals(t *testing.T) {
	t.Parallel()

	t.Run("read-only mode opens nothing", func(t *testing.T) {
		t.Parallel()
		app := writeApp(t, &bitbuckettest.FakeClient{T: t})
		app.readOnly = true

		_, _ = app.Update(runeKey('M'))

		if app.form.IsVisible() {
			t.Error("read-only mode must not open the merge dialog")
		}
		if got := plain(app.helpBar.View()); !strings.Contains(got, "read-only mode: cannot merge") {
			t.Errorf("help bar = %q, want the read-only refusal", got)
		}
	})

	t.Run("a merged pull request names its state", func(t *testing.T) {
		t.Parallel()
		app := writeApp(t, &bitbuckettest.FakeClient{T: t})
		pr := selectedPR(app)
		pr.State = bitbucket.PullRequestMerged

		_, _ = app.Update(runeKey('D'))

		if app.form.IsVisible() {
			t.Error("a merged pull request cannot be edited")
		}
		if got := plain(app.helpBar.View()); !strings.Contains(got, "pull request is MERGED") {
			t.Errorf("help bar = %q, want the state refusal", got)
		}
	})

	t.Run("a second write on the same pull request is refused", func(t *testing.T) {
		t.Parallel()
		app := writeApp(t, &bitbuckettest.FakeClient{T: t})
		app.writesInFlight[testPRKey] = verbMerge

		_, _ = app.Update(runeKey('D'))

		if app.form.IsVisible() {
			t.Error("a write already running must block the next dialog")
		}
		if got := plain(app.helpBar.View()); !strings.Contains(got, "merge already in progress") {
			t.Errorf("help bar = %q, want the in-flight refusal", got)
		}
	})

	t.Run("nothing opens behind an overlay", func(t *testing.T) {
		t.Parallel()
		app := writeApp(t, &bitbuckettest.FakeClient{T: t})
		app.input.Show("Title", testTitle)

		if _, ok := app.writeTarget(verbMerge); ok {
			t.Error("a write key must not open a dialog behind another overlay")
		}
	})
}

// A dialog is off screen from the moment it is confirmed, but its answer is
// still travelling: bubbletea runs the confirmation as a command. A write key
// pressed in that window used to arm a second callback, which the first
// dialog's result then fired.
func TestWriteTarget_RefusesWhileAConfirmationIsStillOnItsWay(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	merged := withAuthor(newPR(testRepo, 101, testTitle), testUserUUID)
	merged.State = bitbucket.PullRequestMerged
	fake.MergePullRequestFunc = func(
		_ context.Context, _, _ string, _ int, _ bitbucket.MergeOptions,
	) (bitbucket.MergeResult, error) {
		return bitbucket.MergeResult{PullRequest: &merged, TaskURL: ""}, nil
	}
	app := writeApp(t, fake)

	_, _ = app.Update(runeKey('M'))
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	confirmed := collectMsgs(cmd)
	if app.form.IsVisible() {
		t.Fatal("the dialog closes the moment it is confirmed")
	}

	// The draft key lands before the merge confirmation does.
	_, _ = app.Update(runeKey('D'))
	if app.form.IsVisible() {
		t.Error("no second dialog may be armed while a confirmation is in flight")
	}

	for _, msg := range confirmed {
		// The merge command runs; the refetches its result asks for do not.
		_, next := app.Update(msg)
		for _, done := range collectMsgs(next) {
			_, _ = app.Update(done)
		}
	}

	testkit.AssertEqual(t, "merge calls", fake.CallCount("MergePullRequest"), 1)
	testkit.AssertEqual(t, "update calls", fake.CallCount("UpdatePullRequest"), 0)
}

func TestFormResult_FromAnotherDialogIsIgnored(t *testing.T) {
	t.Parallel()
	app := writeApp(t, &bitbuckettest.FakeClient{T: t})

	_, _ = app.Update(runeKey('D'))
	if !app.form.IsVisible() {
		t.Fatal("D should open the draft dialog")
	}

	_, cmd := app.Update(components.FormConfirmedMsg{ID: formMerge, Fields: nil})
	if cmd != nil {
		t.Error("a confirmation from another dialog must run nothing")
	}
	_, _ = app.Update(components.FormCancelledMsg{ID: formSync})
	if app.onForm == nil {
		t.Fatal("another dialog's result must not disarm the one on screen")
	}

	_, cmd = app.Update(components.FormConfirmedMsg{ID: formDraft, Fields: nil})
	if cmd == nil {
		t.Error("the armed dialog still answers its own result")
	}
	testkit.AssertEqual(t, "in flight", app.writesInFlight[testPRKey], verbToggleDraft)
}

// -----------------------------------------------------------------------------
// Merge
// -----------------------------------------------------------------------------

func TestMerge_DialogShowsTheStrategiesAndThePullRequestsFlags(t *testing.T) {
	t.Parallel()
	app := writeApp(t, &bitbuckettest.FakeClient{T: t})

	_, _ = app.Update(runeKey('M'))

	if !app.form.IsVisible() {
		t.Fatal("M should open the merge dialog")
	}
	testkit.AssertEqual(t, "form id", app.form.ID(), formMerge)
	rendered := plain(app.form.View())
	for _, want := range []string{"Merge #101 into main", "merge commit", "Close source branch", "[x]", "Message"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("dialog is missing %q:\n%s", want, rendered)
		}
	}
}

func TestMerge_CancellingSendsNothing(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	app := writeApp(t, fake)

	_, _ = app.Update(runeKey('M'))
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	for _, msg := range collectMsgs(cmd) {
		_, _ = app.Update(msg)
	}

	if app.form.IsVisible() {
		t.Error("esc should close the dialog")
	}
	testkit.AssertEqual(t, "merge calls", fake.CallCount("MergePullRequest"), 0)
	if app.onForm != nil {
		t.Error("cancelling must drop the pending callback")
	}
}

func TestMerge_ConfirmSendsTheChosenOptions(t *testing.T) {
	t.Parallel()
	merged := withAuthor(newPR(testRepo, 101, testTitle), testUserUUID)
	merged.State = bitbucket.PullRequestMerged
	fake := &bitbuckettest.FakeClient{T: t}
	fake.MergePullRequestFunc = func(
		_ context.Context, _, _ string, _ int, _ bitbucket.MergeOptions,
	) (bitbucket.MergeResult, error) {
		return bitbucket.MergeResult{PullRequest: &merged, TaskURL: ""}, nil
	}
	app := writeApp(t, fake)

	_, _ = app.Update(runeKey('M'))
	// squash, then off with the close-source-branch flag, then back to the
	// strategy row so enter confirms instead of editing the message.
	_, _ = app.Update(runeKey('l'))
	_, _ = app.Update(runeKey('j'))
	_, _ = app.Update(runeKey(' '))
	_, _ = app.Update(runeKey('k'))
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyEnter})

	for _, msg := range collectMsgs(cmd) {
		_, next := app.Update(msg)
		collectMsgs(next)
	}

	if len(fake.MergePullRequestCalls) != 1 {
		t.Fatalf("merge calls = %d, want one", len(fake.MergePullRequestCalls))
	}
	call := fake.MergePullRequestCalls[0]
	testkit.AssertEqual(t, "workspace", call.Workspace, testWorkspace)
	testkit.AssertEqual(t, "repository", call.RepoSlug, testRepo)
	testkit.AssertEqual(t, "id", call.ID, 101)
	testkit.AssertEqual(t, "strategy", call.Opts.Strategy, bitbucket.MergeSquash)
	if call.Opts.CloseSourceBranch == nil || *call.Opts.CloseSourceBranch {
		t.Errorf("close source branch = %v, want an explicit false", call.Opts.CloseSourceBranch)
	}
	testkit.AssertEqual(t, "message", call.Opts.Message, "")
}

func TestMerge_TheWriteIsMarkedInFlightUntilItAnswers(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	app := writeApp(t, fake)

	_, _ = app.Update(runeKey('M'))
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	for _, msg := range collectMsgs(cmd) {
		// The confirmation is delivered, but the merge command it hands back is
		// deliberately not run: this is the pull request mid-merge.
		_, _ = app.Update(msg)
	}

	testkit.AssertEqual(t, "in flight", app.writesInFlight[testPRKey], verbMerge)

	_, _ = app.Update(runeKey('M'))

	if app.form.IsVisible() {
		t.Error("a merge already running must block a second dialog")
	}
	if got := plain(app.helpBar.View()); !strings.Contains(got, "merge already in progress") {
		t.Errorf("help bar = %q, want the in-flight refusal", got)
	}
	testkit.AssertEqual(t, "merge calls", fake.CallCount("MergePullRequest"), 0)
}

func TestMerge_SuccessPatchesTheListAndRefetchesIt(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	app := writeApp(t, fake)
	merged := withAuthor(newPR(testRepo, 101, testTitle), testUserUUID)
	merged.State = bitbucket.PullRequestMerged
	app.writesInFlight[testPRKey] = verbMerge

	_, cmd := app.Update(pullRequestWrittenMsg{key: testPRKey, verb: verbMerge, pr: &merged, err: nil})

	testkit.AssertEqual(t, "cached state", app.prCache[testPRKey].State, bitbucket.PullRequestMerged)
	testkit.AssertEqual(t, "row state", app.prList.SelectedPullRequest().State, bitbucket.PullRequestMerged)
	if _, busy := app.writesInFlight[testPRKey]; busy {
		t.Error("the write should no longer be in flight")
	}
	if got := plain(app.helpBar.View()); !strings.Contains(got, "merged #101") {
		t.Errorf("help bar = %q, want the success line", got)
	}
	if cmd == nil {
		t.Fatal("a pull request that left OPEN must trigger a list refetch")
	}
}

func TestPullRequestWritten_AnOffScreenPullRequestIsPatchedButNotRefetched(t *testing.T) {
	t.Parallel()
	app := writeApp(t, &bitbuckettest.FakeClient{T: t})
	app.detailView.SetPullRequest(selectedPR(app))
	before := plain(app.detailView.View())
	// The cursor moved on while the edit was in flight.
	app.previewKey = testPRKey2
	app.writesInFlight[testPRKey] = verbEdit
	edited := withReviewer(withAuthor(newPR(testRepo, 101, "renamed"), testUserUUID), testOtherUUID)

	_, cmd := app.Update(pullRequestWrittenMsg{key: testPRKey, verb: verbEdit, pr: &edited, err: nil})

	if cmd != nil {
		t.Error("a write on a pull request that is no longer previewed fetches nothing")
	}
	testkit.AssertEqual(t, "cached title", app.prCache[testPRKey].Title, "renamed")
	testkit.AssertEqual(t, "row title", app.prList.SelectedPullRequest().Title, "renamed")
	testkit.AssertEqual(t, "detail pane", plain(app.detailView.View()), before)
}

func TestMerge_QueuedPollsUntilTheTaskSucceeds(t *testing.T) {
	t.Parallel()
	merged := withAuthor(newPR(testRepo, 101, testTitle), testUserUUID)
	merged.State = bitbucket.PullRequestMerged
	polls := 0
	fake := &bitbuckettest.FakeClient{T: t}
	fake.GetMergeTaskStatusFunc = func(_ context.Context, _ string) (bitbucket.MergeTaskStatus, error) {
		polls++
		if polls == 1 {
			return bitbucket.MergeTaskStatus{Status: bitbucket.MergeTaskPending, MergeResult: nil}, nil
		}
		return bitbucket.MergeTaskStatus{Status: bitbucket.MergeTaskSuccess, MergeResult: &merged}, nil
	}
	fake.ListPullRequestStatusesFunc = func(_ context.Context, _, _ string, _ int) ([]bitbucket.CommitStatus, error) {
		return nil, nil
	}
	fake.ListPullRequestsFunc = func(
		_ context.Context, _, _ string, _ bitbucket.ListPullRequestsOptions,
	) ([]bitbucket.PullRequest, error) {
		return nil, nil
	}
	app := writeApp(t, fake)
	app.writesInFlight[testPRKey] = verbMerge

	_, cmd := app.Update(mergeQueuedMsg{key: testPRKey, taskURL: testTaskURL, attempt: 0})
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "polls", polls, 2)
	testkit.AssertEqual(t, "cached state", app.prCache[testPRKey].State, bitbucket.PullRequestMerged)
}

func TestMerge_PollingGivesUpAfterTheLastAttempt(t *testing.T) {
	t.Parallel()
	app := writeApp(t, &bitbuckettest.FakeClient{T: t})
	app.writesInFlight[testPRKey] = verbMerge

	_, cmd := app.Update(mergeQueuedMsg{key: testPRKey, taskURL: testTaskURL, attempt: mergePollMaxAttempts})

	if cmd != nil {
		t.Error("the last attempt must not schedule another poll")
	}
	if got := plain(app.helpBar.View()); !strings.Contains(got, "merge still running") {
		t.Errorf("help bar = %q, want the give-up line", got)
	}
	if _, busy := app.writesInFlight[testPRKey]; busy {
		t.Error("giving up must clear the in-flight record")
	}
}

func TestWriteErrors_AreShownInTheServersOwnWords(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		verb string
		err  error
		want string
	}{
		{
			name: "a merge conflict says the refs moved", verb: verbMerge,
			err: apiError(http.StatusConflict, "Unable to merge", nil), want: "refs changed — refresh and retry",
		},
		{
			name: "a merge 555 says to refresh", verb: verbMerge,
			err: apiError(bitbucket.StatusDiffTooLarge, "", nil), want: "merge timed out on Bitbucket",
		},
		{
			name: "a field error is quoted with its field", verb: verbEdit,
			err: apiError(http.StatusBadRequest, "Bad request",
				map[string][]string{"title": {"This field is required."}}),
			want: "Bad request: title: This field is required.",
		},
		{
			name: "read-only is a local refusal", verb: verbEdit,
			err: bitbucket.ErrReadOnly, want: "read-only mode",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			app := writeApp(t, &bitbuckettest.FakeClient{T: t})
			app.writesInFlight[testPRKey] = testCase.verb

			_, _ = app.Update(pullRequestWrittenMsg{
				key: testPRKey, verb: testCase.verb, pr: nil, err: testCase.err,
			})

			got := plain(app.helpBar.View())
			if !strings.Contains(got, testCase.verb+" failed: ") || !strings.Contains(got, testCase.want) {
				t.Errorf("help bar = %q, want %q", got, testCase.want)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Sync
// -----------------------------------------------------------------------------

func TestSync_PreconditionsCheckedBeforeTheDialog(t *testing.T) {
	t.Parallel()

	t.Run("outside a repository", func(t *testing.T) {
		t.Parallel()
		app := writeApp(t, &bitbuckettest.FakeClient{T: t})

		_, _ = app.Update(runeKey('S'))

		if app.form.IsVisible() {
			t.Error("no dialog without a checkout")
		}
		if got := plain(app.helpBar.View()); !strings.Contains(got, "not inside a git repository") {
			t.Errorf("help bar = %q", got)
		}
	})

	t.Run("on another branch", func(t *testing.T) {
		t.Parallel()
		app := writeApp(t, &bitbuckettest.FakeClient{T: t})
		app.gitRepoRoot = t.TempDir()
		app.gitBranch = branchMain

		_, _ = app.Update(runeKey('S'))

		if app.form.IsVisible() {
			t.Error("no dialog while the checkout is on another branch")
		}
		if got := plain(app.helpBar.View()); !strings.Contains(got, "check out the source branch first (c)") {
			t.Errorf("help bar = %q", got)
		}
	})

	t.Run("on the source branch the dialog opens", func(t *testing.T) {
		t.Parallel()
		app := writeApp(t, &bitbuckettest.FakeClient{T: t})
		app.gitRepoRoot = t.TempDir()
		app.gitBranch = testBranch

		_, _ = app.Update(runeKey('S'))

		if !app.form.IsVisible() {
			t.Fatal("S should open the sync dialog")
		}
		rendered := plain(app.form.View())
		for _, want := range []string{"Sync " + testBranch + " with main", "Method", syncMethodMerge, "Push"} {
			if !strings.Contains(rendered, want) {
				t.Errorf("dialog is missing %q:\n%s", want, rendered)
			}
		}
	})
}

func TestSync_TheRunIsMarkedInFlightUntilItAnswers(t *testing.T) {
	t.Parallel()
	app := writeApp(t, &bitbuckettest.FakeClient{T: t})
	app.gitRepoRoot = t.TempDir()
	app.gitBranch = testBranch

	_, _ = app.Update(runeKey('S'))
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	for _, msg := range collectMsgs(cmd) {
		// The git run itself is never started: the command is dropped, which is
		// what a slow fetch looks like from the event loop.
		_, _ = app.Update(msg)
	}

	testkit.AssertEqual(t, "in flight", app.writesInFlight[testPRKey], verbSync)

	_, _ = app.Update(runeKey('S'))

	if app.form.IsVisible() {
		t.Error("a sync already running must block a second dialog")
	}
	if got := plain(app.helpBar.View()); !strings.Contains(got, "sync already in progress") {
		t.Errorf("help bar = %q, want the in-flight refusal", got)
	}
}

func TestRunSync_MergesTheTargetAndPushes(t *testing.T) {
	t.Parallel()
	remote, work := syncRepo(t)

	result := runSync(syncParams{
		key: testPRKey, root: work, workspace: testWorkspace, slug: testRepo,
		source: testBranch, destination: branchMain, rebase: false, push: true, checkoutKey: "c",
	})

	if result.err != nil {
		t.Fatalf("sync: %v", result.err)
	}
	testkit.AssertEqual(t, "summary", result.summary,
		"synced "+testBranch+" with "+branchMain+" (merge), pushed")
	testkit.AssertEqual(t, "branch", result.branch, testBranch)
	if !result.pushed {
		t.Error("the run pushed, so the message should say so")
	}
	// The pushed branch is on the remote now.
	if out := gitOutput(t, remote, "branch", "--list", testBranch); !strings.Contains(out, testBranch) {
		t.Errorf("remote branches = %q, want the source branch", out)
	}
}

func TestRunSync_RefusesADirtyTree(t *testing.T) {
	t.Parallel()
	_, work := syncRepo(t)
	if err := os.WriteFile(filepath.Join(work, "dirty.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	result := runSync(syncParams{
		key: testPRKey, root: work, workspace: testWorkspace, slug: testRepo,
		source: testBranch, destination: branchMain, rebase: false, push: false, checkoutKey: "c",
	})

	if result.err == nil || !strings.Contains(result.err.Error(), "uncommitted changes") {
		t.Errorf("err = %v, want the dirty-tree refusal", result.err)
	}
}

func TestRunSync_RefusesTheWrongBranch(t *testing.T) {
	t.Parallel()
	_, work := syncRepo(t)
	if err := runGit(t, []string{"-C", work, "checkout", "-q", branchMain}); err != nil {
		t.Fatalf("checkout main: %v", err)
	}

	result := runSync(syncParams{
		key: testPRKey, root: work, workspace: testWorkspace, slug: testRepo,
		source: testBranch, destination: branchMain, rebase: false, push: false, checkoutKey: "c",
	})

	if result.err == nil || !strings.Contains(result.err.Error(), "check out the source branch first (c)") {
		t.Errorf("err = %v, want the branch refusal", result.err)
	}
}

func TestSyncDone_ReportsAndRefreshesAfterAPush(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	app := writeApp(t, fake)
	app.writesInFlight[testPRKey] = verbSync

	_, cmd := app.Update(syncDoneMsg{
		key: testPRKey, summary: "synced", branch: testBranch, pushed: true, err: nil,
	})

	testkit.AssertEqual(t, "branch", app.gitBranch, testBranch)
	if got := plain(app.helpBar.View()); !strings.Contains(got, "synced") {
		t.Errorf("help bar = %q, want the summary", got)
	}
	if cmd == nil {
		t.Error("a push moves the source commit, so the pull request is refetched")
	}
	if _, cached := app.prCache[testPRKey]; cached {
		t.Error("the stale pull request should have been dropped from the cache")
	}
}

func TestSyncDone_APushIsInvalidatedEvenOffScreen(t *testing.T) {
	t.Parallel()
	app := writeApp(t, &bitbuckettest.FakeClient{T: t})
	app.statusCache[testPRKey] = statusEntry{statuses: nil, updatedOn: testNow, fetchedAt: testNow}
	app.commitsCache[testPRKey] = reviewEntry[[]bitbucket.Commit]{
		value: nil, sourceHash: testSourceHash, err: nil,
	}
	// The cursor moved on while git was working.
	app.previewKey = testPRKey2
	epoch := app.previewEpoch
	app.writesInFlight[testPRKey] = verbSync

	_, cmd := app.Update(syncDoneMsg{
		key: testPRKey, summary: "synced", branch: testBranch, pushed: true, err: nil,
	})

	if cmd != nil {
		t.Error("a push for a pull request that is not previewed refetches nothing")
	}
	testkit.AssertEqual(t, "preview epoch", app.previewEpoch, epoch)
	testkit.AssertEqual(t, "branch", app.gitBranch, testBranch)
	// The head moved for the pushed pull request wherever the cursor is, so its
	// caches — stamped against the commit that just changed — go either way.
	if _, cached := app.prCache[testPRKey]; cached {
		t.Error("the pushed pull request should have been dropped from the cache")
	}
	if _, cached := app.statusCache[testPRKey]; cached {
		t.Error("the pushed pull request's statuses should have been dropped")
	}
	if _, cached := app.commitsCache[testPRKey]; cached {
		t.Error("the pushed pull request's commit list should have been dropped")
	}
}

func TestSyncDone_ReportsTheFailure(t *testing.T) {
	t.Parallel()
	app := writeApp(t, &bitbuckettest.FakeClient{T: t})
	app.writesInFlight[testPRKey] = verbSync

	_, cmd := app.Update(syncDoneMsg{
		key: testPRKey, summary: "", branch: "", pushed: false,
		err: errors.New("conflicts with main — resolve manually (aborted)"),
	})

	if cmd != nil {
		t.Error("a failed sync refreshes nothing")
	}
	if got := plain(app.helpBar.View()); !strings.Contains(got, "sync failed: conflicts with main") {
		t.Errorf("help bar = %q, want the conflict line", got)
	}
}

// -----------------------------------------------------------------------------
// Edit
// -----------------------------------------------------------------------------

func TestEdit_ChooserOffersTitleAndDescription(t *testing.T) {
	t.Parallel()
	app := writeApp(t, &bitbuckettest.FakeClient{T: t})

	_, _ = app.Update(runeKey('E'))

	if !app.modal.IsVisible() {
		t.Fatal("E should open the chooser")
	}
	rendered := plain(app.modal.View())
	for _, want := range []string{"Edit #101", "Title", "Description"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("chooser is missing %q:\n%s", want, rendered)
		}
	}
}

func TestEditTitle_SendsOnlyAChangedTitle(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		typed     string
		wantPut   bool
		wantNote  string
		wantTitle string
	}{
		{name: "a changed title is sent", typed: "New title", wantPut: true, wantTitle: "New title"},
		{name: "an unchanged title is not", typed: testTitle, wantPut: false, wantNote: "title unchanged"},
		{name: "an empty title is refused", typed: "   ", wantPut: false, wantNote: "title cannot be empty"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			updated := withAuthor(newPR(testRepo, 101, testCase.typed), testUserUUID)
			fake := &bitbuckettest.FakeClient{T: t}
			fake.UpdatePullRequestFunc = func(
				_ context.Context, _, _ string, _ int, _ bitbucket.PullRequestUpdate,
			) (*bitbucket.PullRequest, error) {
				return &updated, nil
			}
			app := writeApp(t, fake)

			_, _ = app.Update(runeKey('E'))
			_, cmd := app.Update(components.ModalSelectedMsg{Item: components.ModalItem{ID: editItemTitle}})
			for _, msg := range collectMsgs(cmd) {
				_, _ = app.Update(msg)
			}
			if !app.input.IsVisible() {
				t.Fatal("the title editor should be open")
			}
			if !strings.Contains(plain(app.input.View()), testTitle) {
				t.Error("the input should be prefilled with the current title")
			}

			_, cmd = app.Update(components.InputConfirmedMsg{Text: testCase.typed})
			collectMsgs(cmd)

			if !testCase.wantPut {
				testkit.AssertEqual(t, "update calls", fake.CallCount("UpdatePullRequest"), 0)
				if got := plain(app.helpBar.View()); !strings.Contains(got, testCase.wantNote) {
					t.Errorf("help bar = %q, want %q", got, testCase.wantNote)
				}
				return
			}
			if len(fake.UpdatePullRequestCalls) != 1 {
				t.Fatalf("update calls = %d, want one", len(fake.UpdatePullRequestCalls))
			}
			call := fake.UpdatePullRequestCalls[0]
			if call.Update.Title == nil || *call.Update.Title != testCase.wantTitle {
				t.Errorf("title = %v, want %q", call.Update.Title, testCase.wantTitle)
			}
			testkit.AssertSliceEqual(t, "reviewers", call.Update.Reviewers, []string{testOtherUUID})
		})
	}
}

func TestEditDescription_RoundTripsThroughATempFile(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	updated := withAuthor(newPR(testRepo, 101, testTitle), testUserUUID)
	fake.UpdatePullRequestFunc = func(
		_ context.Context, _, _ string, _ int, _ bitbucket.PullRequestUpdate,
	) (*bitbucket.PullRequest, error) {
		return &updated, nil
	}
	app := writeApp(t, fake)
	app.cfg.Editor = "true"

	if cmd := app.editDescription(testPRKey); cmd == nil {
		t.Fatal("editing a description should suspend into the editor")
	}
	path := app.descriptionEditPath
	body, err := app.readEditedDescription()
	if err != nil {
		t.Fatalf("read the temp file: %v", err)
	}
	testkit.AssertEqual(t, "temp file body", body, testDescription)

	if writeErr := os.WriteFile(path, []byte(testDescription+"\n\nMore.\n"), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	_, cmd := app.Update(descriptionEditedMsg{
		key: testPRKey, path: path, original: testDescription, err: nil,
	})
	collectMsgs(cmd)

	if len(fake.UpdatePullRequestCalls) != 1 {
		t.Fatalf("update calls = %d, want one", len(fake.UpdatePullRequestCalls))
	}
	got := fake.UpdatePullRequestCalls[0].Update
	if got.Description == nil || !strings.HasSuffix(*got.Description, "More.") {
		t.Errorf("description = %v, want the edited text", got.Description)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Error("the temp file should have been removed")
	}
}

func TestEditDescription_UnchangedTextSendsNothing(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	app := writeApp(t, fake)
	app.cfg.Editor = "true"

	if cmd := app.editDescription(testPRKey); cmd == nil {
		t.Fatal("editing a description should suspend into the editor")
	}
	path := app.descriptionEditPath
	// The editor added a trailing newline on save, which is not a change.
	if err := os.WriteFile(path, []byte(testDescription+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, cmd := app.Update(descriptionEditedMsg{
		key: testPRKey, path: path, original: testDescription, err: nil,
	})
	collectMsgs(cmd)

	testkit.AssertEqual(t, "update calls", fake.CallCount("UpdatePullRequest"), 0)
	if got := plain(app.helpBar.View()); !strings.Contains(got, "description unchanged") {
		t.Errorf("help bar = %q, want the unchanged note", got)
	}
}

// -----------------------------------------------------------------------------
// Reviewers
// -----------------------------------------------------------------------------

func TestReviewers_CandidatesAreOrderedDedupedAndExcludeTheAuthor(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.ListEffectiveDefaultReviewersFunc = func(_ context.Context, _, _ string) ([]bitbucket.Account, error) {
		return []bitbucket.Account{
			{UUID: testOtherUUID, DisplayName: "Already a reviewer"},
			{UUID: testThirdUUID, DisplayName: "Ada"},
		}, nil
	}
	fake.ListWorkspaceMembersFunc = func(
		_ context.Context, _ string, _ bitbucket.ListOptions,
	) ([]bitbucket.Account, error) {
		return []bitbucket.Account{
			{UUID: testUserUUID, DisplayName: "The author"},
			{UUID: testThirdUUID, DisplayName: "Ada"},
			{UUID: testFourthUUID, DisplayName: "Grace"},
		}, nil
	}
	app := writeApp(t, fake)

	_, cmd := app.Update(runeKey('V'))
	msgs := collectMsgs(cmd)
	if len(msgs) != 1 {
		t.Fatalf("messages = %v, want the candidate list", msgs)
	}
	candidates, ok := msgs[0].(reviewerCandidatesMsg)
	if !ok {
		t.Fatalf("message = %T, want reviewerCandidatesMsg", msgs[0])
	}

	uuids := make([]string, 0, len(candidates.candidates))
	groups := make([]string, 0, len(candidates.candidates))
	for _, candidate := range candidates.candidates {
		uuids = append(uuids, candidate.uuid)
		groups = append(groups, candidate.group)
	}
	testkit.AssertSliceEqual(t, "candidate order", uuids,
		[]string{testOtherUUID, testThirdUUID, testFourthUUID})
	testkit.AssertSliceEqual(t, "candidate groups", groups,
		[]string{groupReviewer, groupDefaultReviewer, groupMember})

	_, _ = app.Update(candidates)
	if !app.modal.IsVisible() || !app.modal.IsChecklist() {
		t.Fatal("the picker should be a checklist")
	}
	rendered := plain(app.modal.View())
	for _, want := range []string{"Reviewers for #101", "· " + groupDefaultReviewer, "· " + groupMember} {
		if !strings.Contains(rendered, want) {
			t.Errorf("picker is missing %q:\n%s", want, rendered)
		}
	}
}

func TestReviewers_AForbiddenListingIsANoteNotAFailure(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.ListEffectiveDefaultReviewersFunc = func(_ context.Context, _, _ string) ([]bitbucket.Account, error) {
		return nil, apiError(http.StatusForbidden, "Forbidden", nil)
	}
	fake.ListWorkspaceMembersFunc = func(
		_ context.Context, _ string, _ bitbucket.ListOptions,
	) ([]bitbucket.Account, error) {
		return nil, apiError(http.StatusForbidden, "Forbidden", nil)
	}
	app := writeApp(t, fake)

	_, cmd := app.Update(runeKey('V'))
	for _, msg := range collectMsgs(cmd) {
		_, _ = app.Update(msg)
	}

	if !app.modal.IsVisible() {
		t.Fatal("the picker still opens over the pull request's own reviewers")
	}
	got := plain(app.helpBar.View())
	if !strings.Contains(got, "default reviewers unavailable (403)") ||
		!strings.Contains(got, "workspace members unavailable (403)") {
		t.Errorf("help bar = %q, want both degradation notes", got)
	}
}

func TestReviewers_SaveSendsTheCheckedSet(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	updated := withAuthor(newPR(testRepo, 101, testTitle), testUserUUID)
	fake.UpdatePullRequestFunc = func(
		_ context.Context, _, _ string, _ int, _ bitbucket.PullRequestUpdate,
	) (*bitbucket.PullRequest, error) {
		return &updated, nil
	}
	app := writeApp(t, fake)
	_, _ = app.Update(reviewerCandidatesMsg{
		key: testPRKey, pr: nil,
		candidates: []reviewerCandidate{
			{uuid: testOtherUUID, label: "Reviewer", group: groupReviewer, selected: true},
			{uuid: testThirdUUID, label: "Ada", group: groupMember, selected: false},
		},
		note: "", err: nil,
	})

	_, cmd := app.Update(components.ChecklistConfirmedMsg{
		Selected: []components.ModalItem{{ID: testThirdUUID}},
	})
	collectMsgs(cmd)

	if len(fake.UpdatePullRequestCalls) != 1 {
		t.Fatalf("update calls = %d, want one", len(fake.UpdatePullRequestCalls))
	}
	testkit.AssertSliceEqual(t, "reviewers",
		fake.UpdatePullRequestCalls[0].Update.Reviewers, []string{testThirdUUID})
}

func TestReviewers_AnUnchangedSetSendsNothing(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	app := writeApp(t, fake)
	_, _ = app.Update(reviewerCandidatesMsg{
		key: testPRKey, pr: nil,
		candidates: []reviewerCandidate{
			{uuid: testOtherUUID, label: "Reviewer", group: groupReviewer, selected: true},
			{uuid: testThirdUUID, label: "Ada", group: groupMember, selected: false},
		},
		note: "", err: nil,
	})

	_, cmd := app.Update(components.ChecklistConfirmedMsg{
		Selected: []components.ModalItem{{ID: testOtherUUID}},
	})
	collectMsgs(cmd)

	testkit.AssertEqual(t, "update calls", fake.CallCount("UpdatePullRequest"), 0)
	if got := plain(app.helpBar.View()); !strings.Contains(got, "reviewers unchanged") {
		t.Errorf("help bar = %q, want the unchanged note", got)
	}
}

func TestReviewers_NoCandidatesOpensNothing(t *testing.T) {
	t.Parallel()
	app := writeApp(t, &bitbuckettest.FakeClient{T: t})

	_, _ = app.Update(reviewerCandidatesMsg{key: testPRKey, pr: nil, candidates: nil, note: "", err: nil})

	if app.modal.IsVisible() {
		t.Error("an empty candidate list must not open a picker")
	}
	if got := plain(app.helpBar.View()); !strings.Contains(got, "no reviewer candidates available") {
		t.Errorf("help bar = %q", got)
	}
}

func TestReviewers_AnUnknownSetIsReadBeforeThePickerOpens(t *testing.T) {
	t.Parallel()
	detail := withReviewer(withAuthor(newPR(testRepo, 101, testTitle), testUserUUID), testOtherUUID)
	fake := &bitbuckettest.FakeClient{T: t}
	fake.GetPullRequestFunc = func(_ context.Context, _, _ string, _ int) (*bitbucket.PullRequest, error) {
		return &detail, nil
	}
	fake.ListEffectiveDefaultReviewersFunc = func(_ context.Context, _, _ string) ([]bitbucket.Account, error) {
		return nil, nil
	}
	fake.ListWorkspaceMembersFunc = func(
		_ context.Context, _ string, _ bitbucket.ListOptions,
	) ([]bitbucket.Account, error) {
		return []bitbucket.Account{{UUID: testThirdUUID, DisplayName: "Ada"}}, nil
	}
	fake.UpdatePullRequestFunc = func(
		_ context.Context, _, _ string, _ int, _ bitbucket.PullRequestUpdate,
	) (*bitbucket.PullRequest, error) {
		return &detail, nil
	}
	app := bareApp(t, fake)

	_, cmd := app.Update(runeKey('V'))
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "detail fetches", fake.CallCount("GetPullRequest"), 1)
	if !app.modal.IsVisible() || !app.modal.IsChecklist() {
		t.Fatal("the picker should be open once the reviewer set is known")
	}
	if rendered := plain(app.modal.View()); !strings.Contains(rendered, "✓ "+testOtherUUID) {
		t.Errorf("the pull request's own reviewer should start checked:\n%s", rendered)
	}
	// The detail is worth keeping: the PUT that follows must not read it again.
	testkit.AssertSliceEqual(t, "cached reviewers",
		app.prCache[testPRKey].ReviewerUUIDs(), []string{testOtherUUID})

	_, cmd = app.Update(components.ChecklistConfirmedMsg{
		Selected: []components.ModalItem{{ID: testOtherUUID}, {ID: testThirdUUID}},
	})
	collectMsgs(cmd)

	if len(fake.UpdatePullRequestCalls) != 1 {
		t.Fatalf("update calls = %d, want one", len(fake.UpdatePullRequestCalls))
	}
	// The union, not just the row the user ticked: a set built from a picker
	// that never saw the current reviewers would unassign them.
	testkit.AssertSliceEqual(t, "reviewers",
		fake.UpdatePullRequestCalls[0].Update.Reviewers, []string{testOtherUUID, testThirdUUID})
	testkit.AssertEqual(t, "detail fetches", fake.CallCount("GetPullRequest"), 1)
}

func TestReviewers_APickerOpensOverNoSetItCouldNotRead(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	// Neither candidate listing is stubbed: a set that cannot be read stops the
	// flow before there is anything to list.
	fake.GetPullRequestFunc = func(_ context.Context, _, _ string, _ int) (*bitbucket.PullRequest, error) {
		return nil, apiError(http.StatusNotFound, "Not found", nil)
	}
	app := bareApp(t, fake)

	_, cmd := app.Update(runeKey('V'))
	runAll(t, app, cmd)

	if app.modal.IsVisible() {
		t.Error("a picker that cannot state the current reviewers must not open")
	}
	if got := plain(app.helpBar.View()); !strings.Contains(got, verbReviewers+" failed: Not found") {
		t.Errorf("help bar = %q, want the failure", got)
	}
	testkit.AssertEqual(t, "update calls", fake.CallCount("UpdatePullRequest"), 0)
}

// -----------------------------------------------------------------------------
// Draft
// -----------------------------------------------------------------------------

func TestToggleDraft_ConfirmFlipsTheFlag(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	updated := withAuthor(newPR(testRepo, 101, testTitle), testUserUUID)
	updated.Draft = true
	fake.UpdatePullRequestFunc = func(
		_ context.Context, _, _ string, _ int, _ bitbucket.PullRequestUpdate,
	) (*bitbucket.PullRequest, error) {
		return &updated, nil
	}
	app := writeApp(t, fake)

	_, _ = app.Update(runeKey('D'))
	if !app.form.IsVisible() {
		t.Fatal("D should ask before flipping the flag")
	}
	if !strings.Contains(plain(app.form.View()), "Mark #101 as draft?") {
		t.Errorf("dialog = %q, want the draft question", plain(app.form.View()))
	}

	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	for _, msg := range collectMsgs(cmd) {
		_, next := app.Update(msg)
		collectMsgs(next)
	}

	if len(fake.UpdatePullRequestCalls) != 1 {
		t.Fatalf("update calls = %d, want one", len(fake.UpdatePullRequestCalls))
	}
	got := fake.UpdatePullRequestCalls[0].Update
	if got.Draft == nil || !*got.Draft {
		t.Errorf("draft = %v, want an explicit true", got.Draft)
	}
	testkit.AssertSliceEqual(t, "reviewers", got.Reviewers, []string{testOtherUUID})
}

func TestToggleDraft_TheWriteIsMarkedInFlightUntilItAnswers(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	app := writeApp(t, fake)

	_, _ = app.Update(runeKey('D'))
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	for _, msg := range collectMsgs(cmd) {
		// As in the merge case: the PUT command is left unrun, so the write is
		// still in flight when the next key arrives.
		_, _ = app.Update(msg)
	}

	testkit.AssertEqual(t, "in flight", app.writesInFlight[testPRKey], verbToggleDraft)

	_, _ = app.Update(runeKey('D'))

	if app.form.IsVisible() {
		t.Error("an update already running must block a second dialog")
	}
	if got := plain(app.helpBar.View()); !strings.Contains(got, verbToggleDraft+" already in progress") {
		t.Errorf("help bar = %q, want the in-flight refusal", got)
	}
	testkit.AssertEqual(t, "update calls", fake.CallCount("UpdatePullRequest"), 0)
}

func TestUpdate_FetchesTheReviewerSetWhenTheListRowLacksIt(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	detail := withReviewer(withAuthor(newPR(testRepo, 101, testTitle), testUserUUID), testThirdUUID)
	fake.GetPullRequestFunc = func(_ context.Context, _, _ string, _ int) (*bitbucket.PullRequest, error) {
		return &detail, nil
	}
	fake.UpdatePullRequestFunc = func(
		_ context.Context, _, _ string, _ int, _ bitbucket.PullRequestUpdate,
	) (*bitbucket.PullRequest, error) {
		return &detail, nil
	}
	app := writeApp(t, fake)
	// A list payload without the reviewers projection: the set is unknown, not
	// empty, so it has to be read before a PUT can carry it.
	bare := newPR(testRepo, 101, testTitle)
	app.prCache[testPRKey] = &bare

	draft := true
	cmd := app.updatePullRequest(testPRKey, verbToggleDraft,
		bitbucket.PullRequestUpdate{Title: nil, Description: nil, Draft: &draft, Reviewers: nil})
	collectMsgs(cmd)

	testkit.AssertEqual(t, "detail fetches", fake.CallCount("GetPullRequest"), 1)
	if len(fake.UpdatePullRequestCalls) != 1 {
		t.Fatalf("update calls = %d, want one", len(fake.UpdatePullRequestCalls))
	}
	testkit.AssertSliceEqual(t, "reviewers",
		fake.UpdatePullRequestCalls[0].Update.Reviewers, []string{testThirdUUID})
}

func TestIsFork(t *testing.T) {
	t.Parallel()
	pr := newPR(testRepo, 101, testTitle)
	pr.Source.Repository.FullName = testWorkspace + "/" + testRepo
	pr.Destination.Repository.FullName = testWorkspace + "/" + testRepo
	testkit.AssertEqual(t, "same repository", isFork(&pr), false)

	pr.Source.Repository.FullName = "someone/" + testRepo
	testkit.AssertEqual(t, "fork", isFork(&pr), true)

	pr.Source.Repository.FullName = ""
	testkit.AssertEqual(t, "unknown source repository is not called a fork", isFork(&pr), false)
}

func TestSync_RefusesAForkPullRequest(t *testing.T) {
	t.Parallel()
	app := writeApp(t, &bitbuckettest.FakeClient{T: t})
	app.gitRepoRoot = t.TempDir()
	app.gitBranch = testBranch
	// writeTarget resolves the previewed pull request through the detail cache.
	cached := app.prCache[testPRKey]
	cached.Source.Repository.FullName = "someone/" + testRepo
	cached.Destination.Repository.FullName = testWorkspace + "/" + testRepo

	_, _ = app.Update(runeKey('S'))

	if app.form.IsVisible() {
		t.Error("no dialog for a fork pull request")
	}
	if got := plain(app.helpBar.View()); !strings.Contains(got, "source branch is in a fork") {
		t.Errorf("help bar = %q", got)
	}
}

func TestRunSync_RefusesWhenNoRemoteNamesTheRepository(t *testing.T) {
	t.Parallel()
	_, work := syncRepo(t)
	// Point the only remote somewhere that is not <workspace>/<slug>: there is
	// no "origin" fallback, because pushing into an unrelated repository is worse
	// than refusing.
	gitIn(t, work, "remote", "set-url", remoteOrigin, filepath.Join(t.TempDir(), "elsewhere.git"))

	result := runSync(syncParams{
		key: testPRKey, root: work, workspace: testWorkspace, slug: testRepo,
		source: testBranch, destination: branchMain, rebase: false, push: true, checkoutKey: "c",
	})

	if result.err == nil || !strings.Contains(result.err.Error(), "no git remote for "+testWorkspace+"/"+testRepo) {
		t.Errorf("err = %v, want the missing-remote refusal", result.err)
	}
}

func TestRunSync_RefusesWhenTheRemoteBranchIsAhead(t *testing.T) {
	t.Parallel()
	remote, work := syncRepo(t)
	gitIn(t, work, "push", "-q", remoteOrigin, testBranch)

	// Someone else pushes to the source branch; a rebase + force-with-lease from
	// this checkout would throw that commit away.
	other := t.TempDir()
	gitIn(t, other, "clone", "-q", remote, other)
	identify(t, other)
	gitIn(t, other, "checkout", "-q", testBranch)
	commitIn(t, other, "theirs.txt", "theirs\n")
	gitIn(t, other, "push", "-q", remoteOrigin, testBranch)
	theirs := strings.TrimSpace(gitOutput(t, remote, "rev-parse", testBranch))

	result := runSync(syncParams{
		key: testPRKey, root: work, workspace: testWorkspace, slug: testRepo,
		source: testBranch, destination: branchMain, rebase: true, push: true, checkoutKey: "c",
	})

	if result.err == nil || !strings.Contains(result.err.Error(), "pull first") {
		t.Fatalf("err = %v, want the remote-ahead refusal", result.err)
	}
	testkit.AssertEqual(t, "the remote branch is untouched",
		strings.TrimSpace(gitOutput(t, remote, "rev-parse", testBranch)), theirs)
}

// A SUCCESS answer whose merge_result is missing is still a finished merge:
// the pull request is read back instead of polling until the attempts run out.
func TestMerge_QueuedSuccessWithoutAResultReadsThePullRequestBack(t *testing.T) {
	t.Parallel()
	merged := withAuthor(newPR(testRepo, 101, testTitle), testUserUUID)
	merged.State = bitbucket.PullRequestMerged
	polls := 0
	fake := &bitbuckettest.FakeClient{T: t}
	fake.GetMergeTaskStatusFunc = func(_ context.Context, _ string) (bitbucket.MergeTaskStatus, error) {
		polls++
		return bitbucket.MergeTaskStatus{Status: bitbucket.MergeTaskSuccess, MergeResult: nil}, nil
	}
	fake.GetPullRequestFunc = func(_ context.Context, workspace, slug string, id int) (*bitbucket.PullRequest, error) {
		testkit.AssertEqual(t, "workspace", workspace, testWorkspace)
		testkit.AssertEqual(t, "slug", slug, testRepo)
		testkit.AssertEqual(t, "id", id, 101)
		return &merged, nil
	}
	fake.ListPullRequestStatusesFunc = func(_ context.Context, _, _ string, _ int) ([]bitbucket.CommitStatus, error) {
		return nil, nil
	}
	fake.ListPullRequestsFunc = func(
		_ context.Context, _, _ string, _ bitbucket.ListPullRequestsOptions,
	) ([]bitbucket.PullRequest, error) {
		return nil, nil
	}
	app := writeApp(t, fake)
	app.writesInFlight[testPRKey] = verbMerge

	_, cmd := app.Update(mergeQueuedMsg{
		key: testPRKey, workspace: testWorkspace, slug: testRepo, id: 101, taskURL: testTaskURL, attempt: 0,
	})
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "polls", polls, 1)
	testkit.AssertEqual(t, "pull request read back", fake.CallCount("GetPullRequest"), 1)
	testkit.AssertEqual(t, "cached state", app.prCache[testPRKey].State, bitbucket.PullRequestMerged)
	if _, inFlight := app.writesInFlight[testPRKey]; inFlight {
		t.Error("the write must be finished once the merge is known to have landed")
	}
}
