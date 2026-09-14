package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// openTheThread puts the Comments tab on screen with its fixture thread loaded
// and opens it, which is Enter on the cursor row.
func openTheThread(t *testing.T, app *App) {
	t.Helper()
	runAll(t, app, app.jumpToTab(views.TabComments))
	_ = app.detailView.View()
	_, cmd := app.handleActivate()
	runAll(t, app, cmd)
}

func TestThreadSection_EnterOpensTheConversationInThePanel(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubComments(fake)
	app := reviewApp(t, fake)

	openTheThread(t, app)

	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModeThread)
	if app.modal.IsVisible() {
		t.Error("a thread must no longer open in a modal")
	}
	rendered := plain(app.detailView.View())
	if !strings.Contains(rendered, "Is one token enough here?") {
		t.Errorf("the section should carry the comment body:\n%s", rendered)
	}
	if !strings.Contains(rendered, "[0] Thread "+testFileGo+":13") {
		t.Errorf("the title should name the conversation's anchor:\n%s", rendered)
	}
}

func TestThreadSection_ALongThreadOpensWithoutAModal(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	// A conversation the list has to collapse: the block expansion that used to
	// answer Enter must not fire behind the section it now opens.
	root := testThreads()[0].Root
	root.Content = bitbucket.RenderedContent{
		Raw: strings.Repeat("a wordy review remark. ", 40),
	}
	fake.ListPullRequestCommentsFunc = func(context.Context, string, string, int,
		bitbucket.ListOptions,
	) ([]bitbucket.Comment, error) {
		return []bitbucket.Comment{root}, nil
	}
	app := reviewApp(t, fake)

	runAll(t, app, app.jumpToTab(views.TabComments))
	rendered := plain(app.detailView.View())
	if !strings.Contains(rendered, "...") {
		t.Fatalf("the list should collapse this thread:\n%s", rendered)
	}
	_, cmd := app.handleActivate()
	runAll(t, app, cmd)

	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModeThread)
	if app.modal.IsVisible() {
		t.Error("a modal must not open over the thread section")
	}
}

func TestThreadSection_BackspaceReturnsToTheCommentsTab(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubComments(fake)
	app := reviewApp(t, fake)
	openTheThread(t, app)

	// The same key that walks out of a file diff or a step log.
	app.Update(tea.KeyMsg{Type: tea.KeyBackspace})

	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModePullRequest)
	testkit.AssertEqual(t, "tab", app.detailView.ActiveTab(), views.TabComments)
	testkit.AssertEqual(t, "focus stays on the panel", app.side, sideRight)
}

func TestThreadSection_BrowserAndCopyTargetTheComment(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubComments(fake)
	app := reviewApp(t, fake)
	openTheThread(t, app)

	if got := app.browserURL(); !strings.Contains(got, "comment-9001") {
		t.Errorf("browser URL inside a thread = %q, want the comment", got)
	}
}

func TestThreadSection_ATabKeyLeavesIt(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubComments(fake)
	stubCommits(fake)
	app := reviewApp(t, fake)
	openTheThread(t, app)

	runAll(t, app, app.jumpToTab(views.TabCommits))

	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModePullRequest)
	testkit.AssertEqual(t, "tab", app.detailView.ActiveTab(), views.TabCommits)
}

func TestThreadSection_SurvivesABackgroundRefresh(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubComments(fake)
	app := reviewApp(t, fake)
	openTheThread(t, app)

	// The list refreshing behind the panel must not yank a reader out of the
	// conversation, exactly as it must not yank one out of a file diff.
	pr := app.prCache[testPRKey]
	app.detailView.SetPullRequest(pr)

	testkit.AssertEqual(t, "mode", app.detailView.Mode(), views.ModeThread)
}
