package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// The two keys: T resolves or reopens, f hides the resolved conversations.
const (
	resolveKey = 'T'
	hideKey    = 'f'
)

// stubResolve answers both directions and records what was asked for.
func stubResolve(fake *bitbuckettest.FakeClient) {
	fake.ResolveCommentFunc = func(context.Context, string, string, int, int64,
	) (*bitbucket.CommentResolution, error) {
		user := bitbucket.Account{UUID: testUserUUID, DisplayName: "Yurii Tatar"}
		return &bitbucket.CommentResolution{
			Type: "comment_resolution", User: &user, CreatedOn: testNow,
		}, nil
	}
	fake.ReopenCommentFunc = func(context.Context, string, string, int, int64) error { return nil }
}

// answerResolveDialog presses enter on the resolve dialog, failing the test
// when there is none open.
func answerResolveDialog(t *testing.T, app *App) {
	t.Helper()
	if !app.form.IsVisible() {
		t.Fatalf("the resolve dialog should be open, help bar = %q", plain(app.helpBar.View()))
	}
	testkit.AssertEqual(t, "dialog", app.form.ID(), formResolve)
	confirmDialog(t, app)
}

func TestResolve_AsksFirstAndThenResolvesTheThreadUnderTheCursor(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubResolve(fake)
	app := commentApp(t, fake)

	_, cmd := app.Update(runeKey(resolveKey))
	runAll(t, app, cmd)

	// Nothing is written until the dialog is answered.
	testkit.AssertEqual(t, "calls before the dialog", len(fake.ResolveCommentCalls), 0)
	if got := plain(app.form.View()); !strings.Contains(got, "Resolve "+testFileGo+":13?") {
		t.Errorf("dialog = %q, want it to name the conversation", got)
	}

	answerResolveDialog(t, app)

	if len(fake.ResolveCommentCalls) != 1 {
		t.Fatalf("ResolveComment calls = %d, want 1", len(fake.ResolveCommentCalls))
	}
	call := fake.ResolveCommentCalls[0]
	testkit.AssertEqual(t, "workspace", call.Workspace, testWorkspace)
	testkit.AssertEqual(t, "repo", call.RepoSlug, testRepo)
	testkit.AssertEqual(t, "id", call.ID, 101)
	// The thread resolves through the comment that starts it.
	testkit.AssertEqual(t, "comment", call.CommentID, testThreadRoot)

	// The thread carries the resolution without a refetch.
	threads := app.detailView.Threads()
	if len(threads) != 1 || !threads[0].Resolved() {
		t.Fatalf("threads = %+v, want the first one resolved", threads)
	}
	if got := plain(app.helpBar.View()); !strings.Contains(got, "resolved "+testFileGo+":13") {
		t.Errorf("help bar = %q, want it to say what was resolved", got)
	}
}

func TestResolve_ASecondPressReopens(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubResolve(fake)
	app := commentApp(t, fake)

	_, cmd := app.Update(runeKey(resolveKey))
	runAll(t, app, cmd)
	answerResolveDialog(t, app)

	// Resolving it took it off the list, so reopening starts by asking for the
	// resolved ones back. The same key then offers the other direction.
	_, cmd = app.Update(runeKey(hideKey))
	runAll(t, app, cmd)
	_, cmd = app.Update(runeKey(resolveKey))
	runAll(t, app, cmd)
	if got := plain(app.form.View()); !strings.Contains(got, "Reopen "+testFileGo+":13?") {
		t.Errorf("dialog = %q, want the reopen question", got)
	}
	answerResolveDialog(t, app)

	testkit.AssertEqual(t, "reopen calls", len(fake.ReopenCommentCalls), 1)
	testkit.AssertEqual(t, "comment", fake.ReopenCommentCalls[0].CommentID, testThreadRoot)
	if app.detailView.Threads()[0].Resolved() {
		t.Error("the conversation should be open again")
	}
	if got := plain(app.helpBar.View()); !strings.Contains(got, "reopened") {
		t.Errorf("help bar = %q, want it to say it was reopened", got)
	}
}

func TestResolve_WorksOnTheConversationInTheSection(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubResolve(fake)
	app := commentApp(t, fake)
	openTheThread(t, app)

	_, cmd := app.Update(runeKey(resolveKey))
	runAll(t, app, cmd)
	answerResolveDialog(t, app)

	testkit.AssertEqual(t, "comment", fake.ResolveCommentCalls[0].CommentID, testThreadRoot)
	// Resolving does not walk the reader out of the conversation.
	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModeThread)
	// The section shows the conversation whatever the list is filtering, and
	// marks it.
	rendered := plain(app.detailView.View())
	if !strings.Contains(rendered, "✔ "+testFileGo+":13") {
		t.Errorf("the section should mark it resolved:\n%s", rendered)
	}
	if !strings.Contains(rendered, "resolved by") {
		t.Errorf("the marker should say who resolved it:\n%s", rendered)
	}
}

func TestResolve_RefusalsBeforeAnythingIsWritten(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, app *App)
		want  string
	}{
		{
			name: "no conversation in hand",
			setup: func(t *testing.T, app *App) {
				t.Helper()
				runAll(t, app, app.jumpToTab(views.TabOverview))
			},
			want: noThreadHere,
		},
		{
			name:  "read-only mode",
			setup: func(_ *testing.T, app *App) { app.readOnly = true },
			want:  "read-only",
		},
		{
			name:  "another write in flight",
			setup: func(_ *testing.T, app *App) { app.writesInFlight[testPRKey] = verbMerge },
			want:  verbMerge,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := &bitbuckettest.FakeClient{T: t}
			app := commentApp(t, fake)
			tc.setup(t, app)

			_, cmd := app.Update(runeKey(resolveKey))
			runAll(t, app, cmd)

			if app.form.IsVisible() {
				t.Error("no dialog should open")
			}
			testkit.AssertEqual(t, "calls", len(fake.ResolveCommentCalls), 0)
			if got := plain(app.helpBar.View()); !strings.Contains(got, tc.want) {
				t.Errorf("help bar = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolve_AFailureSaysWhyAndChangesNothing(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.ResolveCommentFunc = func(context.Context, string, string, int, int64,
	) (*bitbucket.CommentResolution, error) {
		return nil, errors.New("No such comment on this pull request.")
	}
	app := commentApp(t, fake)

	_, cmd := app.Update(runeKey(resolveKey))
	runAll(t, app, cmd)
	answerResolveDialog(t, app)

	if app.detailView.Threads()[0].Resolved() {
		t.Error("a failed resolve must not mark the conversation resolved")
	}
	if got := plain(app.helpBar.View()); !strings.Contains(got, "No such comment") {
		t.Errorf("help bar = %q, want the server's own words", got)
	}
	if _, busy := app.writesInFlight[testPRKey]; busy {
		t.Error("the failed write should not stay in flight")
	}
}

func TestResolve_AnAnswerWithoutAResolutionStillMarksItResolved(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.ResolveCommentFunc = func(context.Context, string, string, int, int64,
	) (*bitbucket.CommentResolution, error) {
		// The endpoint answered, but carried no resolution document.
		return nil, nil
	}
	app := commentApp(t, fake)
	app.currentUser = &bitbucket.Account{UUID: testUserUUID, DisplayName: "Yurii Tatar"}

	_, cmd := app.Update(runeKey(resolveKey))
	runAll(t, app, cmd)
	answerResolveDialog(t, app)

	// The thread is resolved on Bitbucket, so it is resolved here, recorded
	// against whoever pressed the key.
	thread := app.detailView.Threads()[0]
	if !thread.Resolved() {
		t.Fatal("the conversation should be resolved")
	}
	testkit.AssertEqual(t, "who", thread.Root.Resolution.User.Name(), "Yurii Tatar")
}

// -----------------------------------------------------------------------------
// Hiding resolved conversations
// -----------------------------------------------------------------------------

func TestHideResolved_AResolvedConversationLeavesTheListAtOnce(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubResolve(fake)
	app := commentApp(t, fake)

	_, cmd := app.Update(runeKey(resolveKey))
	runAll(t, app, cmd)
	answerResolveDialog(t, app)

	// Resolved conversations are hidden from the start, so the one just
	// finished with is out of the way without a key being pressed.
	rendered := plain(app.detailView.View())
	if strings.Contains(rendered, "Is one token enough here?") {
		t.Errorf("the resolved conversation should be off the list:\n%s", rendered)
	}
	// The tab label says where it went.
	if !strings.Contains(rendered, "1 hidden") {
		t.Errorf("the tab should say how many are hidden:\n%s", rendered)
	}
}

func TestHideResolved_ShowsThemAgainAndMarksThem(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubResolve(fake)
	app := commentApp(t, fake)

	_, cmd := app.Update(runeKey(resolveKey))
	runAll(t, app, cmd)
	answerResolveDialog(t, app)

	_, cmd = app.Update(runeKey(hideKey))
	runAll(t, app, cmd)

	rendered := plain(app.detailView.View())
	if !strings.Contains(rendered, "Is one token enough here?") {
		t.Errorf("the conversation should be listed again:\n%s", rendered)
	}
	// And it is tellable apart from the open ones at a glance, with who ended
	// it and when.
	if !strings.Contains(rendered, "✔ "+testFileGo+":13") {
		t.Errorf("a shown resolved conversation should be marked:\n%s", rendered)
	}
	if !strings.Contains(rendered, "resolved by Yurii Tatar") {
		t.Errorf("the marker should say who resolved it:\n%s", rendered)
	}
	if got := plain(app.helpBar.View()); !strings.Contains(got, "showing every conversation") {
		t.Errorf("help bar = %q, want it to say what happened", got)
	}

	// And the same key puts them away again.
	_, cmd = app.Update(runeKey(hideKey))
	runAll(t, app, cmd)
	if strings.Contains(plain(app.detailView.View()), "Is one token enough here?") {
		t.Error("the conversation should be hidden again")
	}
}

func TestHideResolved_TheCursorFollowsTheListItCanSee(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubResolve(fake)
	app := commentApp(t, fake)

	_, cmd := app.Update(runeKey(resolveKey))
	runAll(t, app, cmd)
	answerResolveDialog(t, app)
	_ = app.detailView.View()

	// The only thread is hidden, so there is nothing under the cursor — and
	// nothing for the keys that act on it to act on.
	if thread := app.detailView.SelectedThread(); thread != nil {
		t.Errorf("SelectedThread() = %+v, want nil while it is hidden", thread)
	}
	_, cmd = app.Update(runeKey(resolveKey))
	runAll(t, app, cmd)
	if app.form.IsVisible() {
		t.Error("T must not act on a conversation the reader cannot see")
	}
}

func TestDemo_ResolveAndReopenThroughTheRealRoutes(t *testing.T) {
	t.Parallel()
	app := demoReviewApp(t, demoPRKey103)
	runAll(t, app, app.jumpToTab(views.TabComments))
	_ = app.detailView.View()

	// Walk to a conversation nobody has resolved.
	for range 20 {
		thread := app.detailView.SelectedThread()
		if thread == nil {
			t.Fatal("the Comments tab should have a conversation under the cursor")
		}
		if !thread.Resolved() {
			break
		}
		app.detailView.ScrollBy(1)
		_ = app.detailView.View()
	}
	rootID := app.detailView.SelectedThread().Root.ID

	_, cmd := app.Update(runeKey(resolveKey))
	runAll(t, app, cmd)
	answerResolveDialog(t, app)

	// Through the real routes: resolved, and recorded against the demo user.
	resolved := threadByRoot(t, app, rootID)
	if !resolved.Resolved() {
		t.Fatal("the conversation should be resolved")
	}
	testkit.AssertEqual(t, "who", resolved.Root.Resolution.User.UUID, app.currentUser.UUID)

	// And a refetch says the same, so it really is on the server.
	runAll(t, app, app.fetchThreads(app.prCache[demoPRKey103]))
	if !threadByRoot(t, app, rootID).Resolved() {
		t.Error("the server did not keep the resolution")
	}

	// Reopening it: the resolved ones are off the list now, so they are asked
	// back first — the same two keystrokes a reader would use.
	_, cmd = app.Update(runeKey(hideKey))
	runAll(t, app, cmd)
	_ = app.detailView.View()
	if !app.detailView.FocusThread(rootID) {
		t.Fatal("the resolved conversation should be back on the list")
	}
	_, cmd = app.Update(runeKey(resolveKey))
	runAll(t, app, cmd)
	answerResolveDialog(t, app)
	runAll(t, app, app.fetchThreads(app.prCache[demoPRKey103]))
	if threadByRoot(t, app, rootID).Resolved() {
		t.Error("the conversation should be open again")
	}
}

// threadByRoot is the conversation rooted at rootID, as the panel holds it.
func threadByRoot(t *testing.T, app *App, rootID int64) bitbucket.Thread {
	t.Helper()
	for _, thread := range app.detailView.Threads() {
		if thread.Root.ID == rootID {
			return thread
		}
	}
	t.Fatalf("no thread rooted at %d", rootID)
	return bitbucket.Thread{}
}

func TestHideResolved_TheConfigDecidesWhereTheFilterStarts(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	app := commentApp(t, fake)

	// The default: the filter is on, whatever the threads happen to be.
	testkit.AssertEqual(t, "hidden by default", app.detailView.HidesResolvedThreads(), true)

	// gui.showResolvedComments: true starts the other way round.
	shown := true
	app.cfg.GUI.ShowResolvedComments = &shown
	app.detailView.SetShowResolvedThreads(app.cfg.GUI.ShowsResolvedComments())
	testkit.AssertEqual(t, "shown when the config says so",
		app.detailView.HidesResolvedThreads(), false)
}
