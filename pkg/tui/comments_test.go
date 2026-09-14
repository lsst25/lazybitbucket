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

// typeComment types text into the open composer and presses Enter.
func typeComment(t *testing.T, app *App, text string) {
	t.Helper()
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

func TestComment_ReplyReachesBitbucketAndJoinsTheThread(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubCreateComment(fake)
	app := commentApp(t, fake)

	_, cmd := app.Update(runeKey(commentKey))
	runAll(t, app, cmd)
	if !app.detailView.Composing() {
		t.Fatal("C should open the composer on the Comments tab")
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
	if app.detailView.Composing() {
		t.Error("the composer should have closed")
	}
}

func TestComment_ReplyFromInsideTheThreadSection(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubCreateComment(fake)
	app := commentApp(t, fake)
	openTheThread(t, app)

	_, cmd := app.Update(runeKey(commentKey))
	runAll(t, app, cmd)
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

	_, cmd := app.Update(runeKey(commentKey))
	runAll(t, app, cmd)
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

	_, cmd := app.Update(runeKey(commentKey))
	runAll(t, app, cmd)
	// q quits, / searches, M merges. Inside a reply they are letters.
	for _, r := range []rune{'q', '/', 'M'} {
		app.Update(runeKey(r))
	}

	if !app.detailView.Composing() {
		t.Fatal("the composer should still be open")
	}
	testkit.AssertEqual(t, "text", app.detailView.ComposerText(), "q/M")
	if app.searchBar.IsActive() {
		t.Error("the search bar must not have opened")
	}
	if app.overlayVisible() {
		t.Error("no dialog may open while a reply is being written")
	}
}

func TestComment_IsRefusedInReadOnlyMode(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	app := commentApp(t, fake)
	app.readOnly = true

	_, cmd := app.Update(runeKey(commentKey))
	runAll(t, app, cmd)

	if app.detailView.Composing() {
		t.Error("read-only mode must not even open the composer")
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

	_, cmd := app.Update(runeKey(commentKey))
	runAll(t, app, cmd)
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

	_, cmd := app.Update(runeKey(commentKey))
	runAll(t, app, cmd)
	typeComment(t, app, "worth keeping")

	if !app.detailView.Composing() {
		t.Fatal("a failed post should reopen the composer")
	}
	testkit.AssertEqual(t, "text", app.detailView.ComposerText(), "worth keeping")
	testkit.AssertEqual(t, "parent", app.detailView.ComposerParent(), testThreadRoot)
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

	if app.detailView.Composing() {
		t.Error("a write in flight should keep the composer shut")
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
		key: testPRKey, parentID: testThreadRoot, path: path, err: nil,
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
		key: testPRKey, parentID: testThreadRoot, path: path, err: nil,
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

	_, cmd := app.Update(runeKey(commentKey))
	runAll(t, app, cmd)
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

	_, cmd := app.Update(runeKey(commentKey))
	runAll(t, app, cmd)
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
	rootID := before[0].Root.ID
	replies := len(before[0].Replies)

	_, cmd := app.Update(runeKey(commentKey))
	runAll(t, app, cmd)
	typeComment(t, app, "one more round and this is good")

	// Through the real client, the real routes and the real thread rebuild.
	after := app.detailView.Threads()
	if len(after) != len(before) {
		t.Fatalf("threads = %d, want the same %d", len(after), len(before))
	}
	testkit.AssertEqual(t, "the reply joined its thread", len(after[0].Replies), replies+1)
	testkit.AssertEqual(t, "root", after[0].Root.ID, rootID)

	written := after[0].Replies[len(after[0].Replies)-1]
	testkit.AssertEqual(t, "author", written.User.UUID, demo.NewDataset().User.UUID)
	testkit.AssertEqual(t, "raw", written.Content.Raw, "one more round and this is good")
	// The server rendered it, which is what makes a mention in it readable.
	if written.Content.HTML == "" {
		t.Error("the written comment should carry the server's rendering")
	}

	// And the server really has it: a refetch says the same.
	runAll(t, app, app.fetchThreads(app.prCache[demoPRKey103]))
	testkit.AssertEqual(t, "after a refetch", len(app.detailView.Threads()[0].Replies), replies+1)
}

func TestDemo_ReplyIsRefusedInReadOnlyMode(t *testing.T) {
	t.Parallel()
	app := demoReviewApp(t, demoPRKey103)
	app.readOnly = true
	runAll(t, app, app.jumpToTab(views.TabComments))
	_ = app.detailView.View()

	_, cmd := app.Update(runeKey(commentKey))
	runAll(t, app, cmd)

	if app.detailView.Composing() {
		t.Error("browse-only mode must not open the composer")
	}
}

func TestComment_ACustomCommandDoesNotFireFromInsideTheComposer(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	app := commentApp(t, fake)
	// A custom command on a letter and on the editor key: custom commands
	// override built-in bindings, but not the composer, which is text.
	app.cfg.CustomCommands = []config.CustomCommandConfig{
		{Key: "x", Name: "Say hello", Contexts: []string{"detail"}, Command: "echo hello"},
		{Key: "ctrl+e", Name: "Open in nvim", Contexts: []string{"detail"}, Command: "echo nvim"},
	}
	app.initCustomCommands()

	_, cmd := app.Update(runeKey(commentKey))
	runAll(t, app, cmd)
	_, cmd = app.Update(runeKey('x'))

	if cmd != nil {
		t.Errorf("a custom command fired from inside the composer: %T", cmd())
	}
	testkit.AssertEqual(t, "text", app.detailView.ComposerText(), "x")
	if !app.detailView.Composing() {
		t.Error("the composer should still be open")
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

	_, cmd := app.Update(runeKey(commentKey))
	runAll(t, app, cmd)
	if !app.detailView.Composing() {
		t.Fatal("C should open the composer inside a file diff")
	}
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

	_, cmd := app.Update(runeKey(commentKey))
	runAll(t, app, cmd)
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

	_, cmd := app.Update(runeKey(commentKey))
	runAll(t, app, cmd)
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

	_, cmd := app.Update(runeKey(commentKey))
	runAll(t, app, cmd)
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

	_, cmd := app.Update(runeKey(commentKey))
	runAll(t, app, cmd)
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

	_, cmd := app.Update(runeKey(commentKey))
	runAll(t, app, cmd)
	typeComment(t, app, "worth keeping")

	if !app.detailView.Composing() {
		t.Fatal("a failed post should reopen the composer")
	}
	anchor := app.detailView.ComposerAnchor()
	if anchor == nil {
		t.Fatal("the reopened composer should still know where the comment goes")
	}
	testkit.AssertEqual(t, "line", anchor.Line, 12)
	testkit.AssertEqual(t, "text", app.detailView.ComposerText(), "worth keeping")
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

	_, cmd := app.Update(runeKey(commentKey))
	runAll(t, app, cmd)
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
