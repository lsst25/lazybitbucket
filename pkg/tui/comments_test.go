package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/demo"
	"github.com/lsst25/lazybitbucket/pkg/config"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// commentKey is the C key as the keymap resolves it.
const commentKey = 'C'

// writtenComment is what a stubbed CreateComment answers with: the comment as
// the server would have built it, which is what the app folds into the thread.
func writtenComment(id, parentID int64, raw string) *bitbucket.Comment {
	comment := &bitbucket.Comment{
		Type:      "pullrequest_comment",
		ID:        id,
		CreatedOn: testNow,
		UpdatedOn: testNow,
		Content:   bitbucket.RenderedContent{Raw: raw, HTML: "<p>" + raw + "</p>"},
		User:      bitbucket.Account{UUID: testUserUUID, DisplayName: "Yurii Tatar"},
	}
	if parentID != 0 {
		comment.Parent = &bitbucket.CommentRef{ID: parentID}
	}
	return comment
}

// stubCreateComment answers every post with the comment the server would have
// created, and records what was asked for.
func stubCreateComment(fake *bitbuckettest.FakeClient) {
	fake.CreateCommentFunc = func(_ context.Context, _, _ string, _ int,
		create bitbucket.CommentCreate,
	) (*bitbucket.Comment, error) {
		return writtenComment(9500, create.ParentID, create.Raw), nil
	}
}

// commentApp is reviewApp with the Comments tab loaded and on screen.
func commentApp(t *testing.T, fake *bitbuckettest.FakeClient) *App {
	t.Helper()
	stubComments(fake)
	app := reviewApp(t, fake)
	runAll(t, app, app.jumpToTab(views.TabComments))
	_ = app.detailView.View()
	return app
}

// typeComment types text into the open comment popup and confirms it, which is
// the same path the title editor takes: the overlay stack owns the keys, and
// Enter answers with an InputConfirmedMsg the app then dispatches.
func typeComment(t *testing.T, app *App, text string) {
	t.Helper()
	if !app.input.IsVisible() {
		t.Fatal("the comment popup should be open")
	}
	for _, r := range text {
		key := runeKey(r)
		if r == ' ' {
			key = tea.KeyMsg{Type: tea.KeySpace}
		}
		if _, cmd := app.Update(key); cmd != nil {
			runAll(t, app, cmd)
		}
	}
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	runAll(t, app, cmd)
}

// openComment presses C and fails unless the popup opened.
func openComment(t *testing.T, app *App) {
	t.Helper()
	_, cmd := app.Update(runeKey(commentKey))
	runAll(t, app, cmd)
	if !app.input.IsVisible() {
		t.Fatalf("C should open the comment popup, help bar = %q", plain(app.helpBar.View()))
	}
}

func TestComment_ReplyReachesBitbucketAndJoinsTheThread(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubCreateComment(fake)
	app := commentApp(t, fake)

	openComment(t, app)
	// The popup names what is being answered, the way the title editor names
	// what is being edited.
	if got := plain(app.input.View()); !strings.Contains(got, "Reply to") {
		t.Errorf("popup = %q, want it to name the conversation", got)
	}

	typeComment(t, app, "rotating weekly still leaves a window")

	if len(fake.CreateCommentCalls) != 1 {
		t.Fatalf("CreateComment calls = %d, want 1", len(fake.CreateCommentCalls))
	}
	call := fake.CreateCommentCalls[0]
	testkit.AssertEqual(t, "workspace", call.Workspace, testWorkspace)
	testkit.AssertEqual(t, "repo", call.RepoSlug, testRepo)
	testkit.AssertEqual(t, "id", call.ID, 101)
	testkit.AssertEqual(t, "parent", call.Create.ParentID, testThreadRoot)
	testkit.AssertEqual(t, "raw", call.Create.Raw, "rotating weekly still leaves a window")

	// The answer is folded into the conversation without a second request.
	rendered := plain(app.detailView.View())
	if !strings.Contains(rendered, "rotating weekly still leaves a window") {
		t.Errorf("the reply should be in the thread:\n%s", rendered)
	}
	threads := app.detailView.Threads()
	if len(threads) != 1 || len(threads[0].Replies) != 1 {
		t.Fatalf("threads = %+v, want one thread with one reply", threads)
	}
	testkit.AssertEqual(t, "the cache carries it too",
		len(app.threadsCache[testPRKey].value[0].Replies), 1)
	if app.input.IsVisible() {
		t.Error("the popup should have closed")
	}
}

func TestComment_ReplyFromInsideTheThreadSection(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubCreateComment(fake)
	app := commentApp(t, fake)
	openTheThread(t, app)

	openComment(t, app)
	typeComment(t, app, "agreed")

	testkit.AssertEqual(t, "parent", fake.CreateCommentCalls[0].Create.ParentID, testThreadRoot)
	// Replying does not walk the reader out of the conversation.
	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModeThread)
	if !strings.Contains(plain(app.detailView.View()), "agreed") {
		t.Error("the section should show the new reply")
	}
}

func TestComment_NewCommentFromTheOverviewOpensItsThread(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubCreateComment(fake)
	app := commentApp(t, fake)
	runAll(t, app, app.jumpToTab(views.TabOverview))

	openComment(t, app)
	typeComment(t, app, "ready from my side")

	testkit.AssertEqual(t, "parent", fake.CreateCommentCalls[0].Create.ParentID, int64(0))
	// A conversation nothing on screen named: the panel goes to it.
	testkit.AssertEqual(t, "tab", app.detailView.ActiveTab(), views.TabComments)
	if !strings.Contains(plain(app.detailView.View()), "ready from my side") {
		t.Error("the new comment should be on screen")
	}
}

func TestComment_TheComposerSwallowsBindings(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	app := commentApp(t, fake)

	openComment(t, app)
	// q quits, / searches, M merges. Inside the popup they are letters, exactly
	// as they are while a title is being edited.
	for _, r := range []rune{'q', '/', 'M'} {
		app.Update(runeKey(r))
	}

	if !app.input.IsVisible() {
		t.Fatal("the popup should still be open")
	}
	testkit.AssertEqual(t, "text", app.input.Value(), "q/M")
	if app.searchBar.IsActive() {
		t.Error("the search bar must not have opened")
	}
	if app.modal.IsVisible() || app.form.IsVisible() {
		t.Error("no dialog may open while a comment is being written")
	}
}

func TestComment_IsRefusedInReadOnlyMode(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	app := commentApp(t, fake)
	app.readOnly = true

	_, cmd := app.Update(runeKey(commentKey))
	runAll(t, app, cmd)

	if app.input.IsVisible() {
		t.Error("read-only mode must not even open the popup")
	}
	if got := plain(app.helpBar.View()); !strings.Contains(got, "read-only") {
		t.Errorf("help bar = %q, want the read-only refusal", got)
	}
	testkit.AssertEqual(t, "calls", len(fake.CreateCommentCalls), 0)
}

func TestComment_IsAllowedOnAMergedPullRequest(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubCreateComment(fake)
	app := commentApp(t, fake)
	// Merging closes every other write; a conversation outlives the branch.
	pr := app.prCache[testPRKey]
	pr.State = bitbucket.PullRequestMerged

	openComment(t, app)
	typeComment(t, app, "noting this for the follow-up")

	testkit.AssertEqual(t, "calls", len(fake.CreateCommentCalls), 1)
}

func TestComment_AFailedPostGivesTheTextBack(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.CreateCommentFunc = func(context.Context, string, string, int,
		bitbucket.CommentCreate,
	) (*bitbucket.Comment, error) {
		return nil, errors.New("network is unreachable")
	}
	app := commentApp(t, fake)

	openComment(t, app)
	typeComment(t, app, "worth keeping")

	if !app.input.IsVisible() {
		t.Fatal("a failed post should ask again")
	}
	testkit.AssertEqual(t, "text", app.input.Value(), "worth keeping")
	if app.pendingComment == nil {
		t.Fatal("the reopened popup should still know where the comment goes")
	}
	testkit.AssertEqual(t, "parent", app.pendingComment.target.ParentID, testThreadRoot)
	if got := plain(app.helpBar.View()); !strings.Contains(got, "network is unreachable") {
		t.Errorf("help bar = %q, want the failure", got)
	}
	if _, busy := app.writesInFlight[testPRKey]; busy {
		t.Error("the failed write should not stay in flight")
	}
}

func TestComment_IsRefusedWhileAnotherWriteRuns(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	app := commentApp(t, fake)
	app.writesInFlight[testPRKey] = verbMerge

	_, cmd := app.Update(runeKey(commentKey))
	runAll(t, app, cmd)

	if app.input.IsVisible() {
		t.Error("a write in flight should keep the popup shut")
	}
	if got := plain(app.helpBar.View()); !strings.Contains(got, verbMerge) {
		t.Errorf("help bar = %q, want it to name the running write", got)
	}
}

func TestComment_EditorTextIsPostedAndTheFileKeptOnFailure(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.CreateCommentFunc = func(context.Context, string, string, int,
		bitbucket.CommentCreate,
	) (*bitbucket.Comment, error) {
		return nil, errors.New("bitbucket said no")
	}
	app := commentApp(t, fake)

	path := filepath.Join(t.TempDir(), "reply.md")
	body := "A longer thought.\n\nWith a second paragraph.\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	_, cmd := app.Update(commentEditedMsg{
		key:    testPRKey,
		target: views.CommentTarget{ParentID: testThreadRoot, Anchor: nil, Title: "Reply"},
		path:   path, err: nil,
	})
	runAll(t, app, cmd)

	if len(fake.CreateCommentCalls) != 1 {
		t.Fatalf("CreateComment calls = %d, want 1", len(fake.CreateCommentCalls))
	}
	testkit.AssertEqual(t, "raw", fake.CreateCommentCalls[0].Create.Raw, body)
	// The post failed, so the text stays where the editor left it and the help
	// bar says where: a paragraph does not fit back into a one-line box.
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the temp file should survive a failed post: %v", err)
	}
	if got := plain(app.helpBar.View()); !strings.Contains(got, path) {
		t.Errorf("help bar = %q, want it to name the file", got)
	}
}

func TestComment_AnEmptyEditorFileDiscardsTheComment(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	app := commentApp(t, fake)

	path := filepath.Join(t.TempDir(), "reply.md")
	if err := os.WriteFile(path, []byte("\n  \n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, cmd := app.Update(commentEditedMsg{
		key:    testPRKey,
		target: views.CommentTarget{ParentID: testThreadRoot, Anchor: nil, Title: "Reply"},
		path:   path, err: nil,
	})
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "calls", len(fake.CreateCommentCalls), 0)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("a discarded comment's file should be removed")
	}
	if got := plain(app.helpBar.View()); !strings.Contains(got, "discarded") {
		t.Errorf("help bar = %q, want it to say the comment was discarded", got)
	}
}

func TestComment_PostedReplyKeepsTheDiffMarkers(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubCreateComment(fake)
	app := commentApp(t, fake)

	openComment(t, app)
	typeComment(t, app, "one more thing")

	// The diff's markers are rebuilt from the threads the reply joined. They
	// count conversations rather than comments, so a reply leaves the count
	// where it was — and proves the rebuild ran rather than dropping the map.
	markers := app.commentMarkers(testPRKey, testFileGo)
	if markers[13] != 1 {
		t.Errorf("markers for line 13 = %d, want the one conversation", markers[13])
	}
}

func TestComment_HelpBarShowsTheComposerKeysWhileItIsOpen(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	app := commentApp(t, fake)

	openComment(t, app)
	_ = app.View()

	got := plain(app.helpBar.View())
	for _, want := range []string{"send", "editor", "cancel"} {
		if !strings.Contains(got, want) {
			t.Errorf("help bar = %q, want it to name %q", got, want)
		}
	}
}

// -----------------------------------------------------------------------------
// Against the demo server
// -----------------------------------------------------------------------------

func TestDemo_ReplyIsWrittenAndReadBackInTheThread(t *testing.T) {
	t.Parallel()
	app := demoReviewApp(t, demoPRKey103)
	runAll(t, app, app.jumpToTab(views.TabComments))
	_ = app.detailView.View()

	before := app.detailView.Threads()
	if len(before) == 0 {
		t.Fatal("the fixture pull request should have comments")
	}
	// The conversation under the cursor, which is not the first of the whole
	// set: the resolved ones are not on the list.
	selected := app.detailView.SelectedThread()
	if selected == nil {
		t.Fatal("the Comments tab should have a conversation under the cursor")
	}
	rootID := selected.Root.ID
	replies := len(selected.Replies)

	openComment(t, app)
	typeComment(t, app, "one more round and this is good")

	// Through the real client, the real routes and the real thread rebuild.
	after := app.detailView.Threads()
	if len(after) != len(before) {
		t.Fatalf("threads = %d, want the same %d", len(after), len(before))
	}
	answered := threadByRoot(t, app, rootID)
	testkit.AssertEqual(t, "the reply joined its thread", len(answered.Replies), replies+1)

	written := answered.Replies[len(answered.Replies)-1]
	testkit.AssertEqual(t, "author", written.User.UUID, demo.NewDataset().User.UUID)
	testkit.AssertEqual(t, "raw", written.Content.Raw, "one more round and this is good")
	// The server rendered it, which is what makes a mention in it readable.
	if written.Content.HTML == "" {
		t.Error("the written comment should carry the server's rendering")
	}

	// And the server really has it: a refetch says the same.
	runAll(t, app, app.fetchThreads(app.prCache[demoPRKey103]))
	testkit.AssertEqual(t, "after a refetch", len(threadByRoot(t, app, rootID).Replies), replies+1)
}

func TestDemo_ReplyIsRefusedInReadOnlyMode(t *testing.T) {
	t.Parallel()
	app := demoReviewApp(t, demoPRKey103)
	app.readOnly = true
	runAll(t, app, app.jumpToTab(views.TabComments))
	_ = app.detailView.View()

	_, cmd := app.Update(runeKey(commentKey))
	runAll(t, app, cmd)

	if app.input.IsVisible() {
		t.Error("browse-only mode must not open the popup")
	}
}

func TestComment_ACustomCommandDoesNotFireFromInsideThePopup(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	app := commentApp(t, fake)
	// A custom command on a letter and on the editor key: custom commands
	// override built-in bindings, but never the popup, which is text.
	app.cfg.CustomCommands = []config.CustomCommandConfig{
		{Key: "x", Name: "Say hello", Contexts: []string{"detail"}, Command: "echo hello"},
		{Key: "ctrl+e", Name: "Open in nvim", Contexts: []string{"detail"}, Command: "echo nvim"},
	}
	app.initCustomCommands()

	openComment(t, app)
	_, cmd := app.Update(runeKey('x'))

	if cmd != nil {
		t.Errorf("a custom command fired from inside the popup: %T", cmd())
	}
	testkit.AssertEqual(t, "text", app.input.Value(), "x")
	if !app.input.IsVisible() {
		t.Error("the popup should still be open")
	}
}

// -----------------------------------------------------------------------------
// Inline comments
// -----------------------------------------------------------------------------

// diffCommentApp is an app inside one file's diff, threads loaded, with the
// line cursor on row.
func diffCommentApp(t *testing.T, fake *bitbuckettest.FakeClient, row int) *App {
	t.Helper()
	stubDiff(fake)
	stubComments(fake)
	stubDiffStat(fake)
	app := reviewApp(t, fake)
	runAll(t, app, app.jumpToTab(views.TabFiles))
	_ = app.detailView.View()
	runAll(t, app, app.openFileDiff(testFileGo))
	_ = app.detailView.View()
	for range row {
		app.Update(runeKey('j'))
	}
	return app
}

func TestComment_InlineCommentCarriesTheLineUnderTheCursor(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubCreateComment(fake)
	// Row 5 is the second added line, new-side 13 — the line the fixture thread
	// is anchored to as well, so this is also a second conversation on it.
	app := diffCommentApp(t, fake, 5)

	openComment(t, app)
	typeComment(t, app, "off by one?")

	if len(fake.CreateCommentCalls) != 1 {
		t.Fatalf("CreateComment calls = %d, want 1", len(fake.CreateCommentCalls))
	}
	create := fake.CreateCommentCalls[0].Create
	if create.Anchor == nil {
		t.Fatal("an inline comment should carry an anchor")
	}
	testkit.AssertEqual(t, "path", create.Anchor.Path, testFileGo)
	testkit.AssertEqual(t, "line", create.Anchor.Line, 13)
	testkit.AssertEqual(t, "old side", create.Anchor.OldSide, false)
	// A second conversation on the line, not a reply to the one already there.
	testkit.AssertEqual(t, "parent", create.ParentID, int64(0))
}

func TestComment_InlineCommentLeavesTheReaderInTheDiff(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.CreateCommentFunc = func(_ context.Context, _, _ string, _ int,
		create bitbucket.CommentCreate,
	) (*bitbucket.Comment, error) {
		comment := writtenComment(9600, 0, create.Raw)
		line := create.Anchor.Line
		comment.Inline = &bitbucket.CommentInline{Path: create.Anchor.Path, To: &line}
		return comment, nil
	}
	// Row 4 is the first added line, new-side 12, which carries no thread yet.
	app := diffCommentApp(t, fake, 4)
	lineBefore, _ := app.detailView.SelectedDiffLine()

	openComment(t, app)
	typeComment(t, app, "this reads better as a guard clause")

	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModeFileDiff)
	testkit.AssertEqual(t, "file", app.detailView.FileDiffPath(), testFileGo)
	lineAfter, _ := app.detailView.SelectedDiffLine()
	testkit.AssertEqual(t, "the cursor has not moved", lineAfter.NewNo, lineBefore.NewNo)
	// The marker on that line lights up from the thread the comment created.
	if got := app.commentMarkers(testPRKey, testFileGo)[12]; got != 1 {
		t.Errorf("markers for line 12 = %d, want 1", got)
	}
	if !strings.Contains(plain(app.detailView.View()), "●") {
		t.Error("the new comment should be marked in the diff")
	}
	if got := plain(app.helpBar.View()); !strings.Contains(got, testFileGo+":12") {
		t.Errorf("help bar = %q, want it to name where the comment landed", got)
	}
}

func TestComment_ARemovedLineAnchorsToTheOldSide(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubCreateComment(fake)
	// Row 3 is the removed line, old-side 12.
	app := diffCommentApp(t, fake, 3)

	openComment(t, app)
	typeComment(t, app, "why did this go?")

	create := fake.CreateCommentCalls[0].Create
	testkit.AssertEqual(t, "line", create.Anchor.Line, 12)
	testkit.AssertEqual(t, "old side", create.Anchor.OldSide, true)
}

func TestComment_AHunkHeaderAnchorsToTheFile(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubCreateComment(fake)
	app := diffCommentApp(t, fake, 0)

	openComment(t, app)
	typeComment(t, app, "this whole hunk wants a test")

	create := fake.CreateCommentCalls[0].Create
	testkit.AssertEqual(t, "path", create.Anchor.Path, testFileGo)
	testkit.AssertEqual(t, "line", create.Anchor.Line, 0)
}

func TestComment_TheFilesTabCommentsOnTheSelectedFile(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubCreateComment(fake)
	stubDiffStat(fake)
	stubComments(fake)
	app := reviewApp(t, fake)
	runAll(t, app, app.jumpToTab(views.TabFiles))
	_ = app.detailView.View()

	openComment(t, app)
	typeComment(t, app, "worth splitting this file")

	create := fake.CreateCommentCalls[0].Create
	testkit.AssertEqual(t, "path", create.Anchor.Path, testFileGo)
	testkit.AssertEqual(t, "line", create.Anchor.Line, 0)
	// And the Files tab is still what is on screen.
	testkit.AssertEqual(t, "tab", app.detailView.ActiveTab(), views.TabFiles)
}

func TestComment_AFailedInlinePostKeepsTheAnchor(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.CreateCommentFunc = func(context.Context, string, string, int,
		bitbucket.CommentCreate,
	) (*bitbucket.Comment, error) {
		return nil, errors.New("that line is no longer part of the diff")
	}
	app := diffCommentApp(t, fake, 4)

	openComment(t, app)
	typeComment(t, app, "worth keeping")

	if !app.input.IsVisible() {
		t.Fatal("a failed post should ask again")
	}
	if app.pendingComment == nil || app.pendingComment.target.Anchor == nil {
		t.Fatal("the reopened popup should still know where the comment goes")
	}
	testkit.AssertEqual(t, "line", app.pendingComment.target.Anchor.Line, 12)
	testkit.AssertEqual(t, "text", app.input.Value(), "worth keeping")
}

func TestDemo_InlineCommentIsWrittenAndMarksTheLine(t *testing.T) {
	t.Parallel()
	app := demoReviewApp(t, demoPRKey103)
	runAll(t, app, app.jumpToTab(views.TabFiles))
	_ = app.detailView.View()
	file := app.detailView.DiffStat()[0].Path()
	runAll(t, app, app.openFileDiff(file))
	_ = app.detailView.View()

	// Walk to the first row that is a line of the file rather than a header.
	app.Update(runeKey('j'))
	line, ok := app.detailView.SelectedDiffLine()
	if !ok {
		t.Fatal("the diff should have a line under the cursor")
	}

	openComment(t, app)
	typeComment(t, app, "is this still needed?")

	// Through the real routes: the server anchored it, and the thread it built
	// hangs off the file the reader is looking at.
	found := false
	for _, thread := range app.detailView.Threads() {
		if thread.Root.Content.Raw == "is this still needed?" {
			found = true
			testkit.AssertEqual(t, "path", thread.Path(), file)
			anchored, _, hasLine := thread.Root.Inline.Line()
			if !hasLine {
				t.Fatal("the written comment should be anchored to a line")
			}
			testkit.AssertEqual(t, "line", anchored, max(line.NewNo, line.OldNo))
		}
	}
	if !found {
		t.Fatal("the written comment is not in the thread list")
	}
	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModeFileDiff)
}

// commentTempFiles lists the comment temp files the app has written into dir.
func commentTempFiles(t *testing.T, dir string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "lazybitbucket-*-comment-*.md"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	return matches
}

func TestComment_CtrlEHandsThePopupsTextToTheEditor(t *testing.T) {
	// No t.Parallel: the temp directory is where the hand-off is observed, and
	// it is set through the environment.
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)

	fake := &bitbuckettest.FakeClient{T: t}
	app := commentApp(t, fake)
	app.cfg.Editor = testEditor

	openComment(t, app)
	for _, r := range "a longer thought" {
		key := runeKey(r)
		if r == ' ' {
			key = tea.KeyMsg{Type: tea.KeySpace}
		}
		app.Update(key)
	}

	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyCtrlE})

	if cmd == nil {
		t.Fatal("ctrl+e should suspend into the editor")
	}
	if app.input.IsVisible() {
		t.Error("the popup closes while the editor has the terminal")
	}
	if app.pendingComment != nil {
		t.Error("the popup is no longer the thing writing the comment")
	}

	// The text went with it: the editor opens on what was already typed.
	files := commentTempFiles(t, tmp)
	if len(files) != 1 {
		t.Fatalf("temp files = %v, want exactly one", files)
	}
	body, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read the temp file: %v", err)
	}
	testkit.AssertEqual(t, "temp file", string(body), "a longer thought")
}

func TestComment_TheTitleEditorKeepsCtrlEAsEndOfLine(t *testing.T) {
	// No t.Parallel: see above.
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)

	fake := &bitbuckettest.FakeClient{T: t}
	app := commentApp(t, fake)
	app.cfg.Editor = testEditor

	// The same popup widget, opened for a title: ctrl+e is readline's
	// end-of-line there and must go on meaning that.
	runAll(t, app, app.editTitle(testPRKey))
	app.Update(tea.KeyMsg{Type: tea.KeyCtrlE})

	if !app.input.IsVisible() {
		t.Error("the title editor should still be open")
	}
	if files := commentTempFiles(t, tmp); len(files) != 0 {
		t.Errorf("the title editor handed off to an editor: %v", files)
	}
}
