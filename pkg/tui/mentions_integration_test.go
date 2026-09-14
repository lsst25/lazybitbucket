package tui

import (
	"context"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/demo"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// demoAccountID is the account id the demo dataset gives a display name. A
// mention is written against the id, so a test that composes one has to ask the
// dataset rather than spell the id out and drift from it.
func demoAccountID(t *testing.T, displayName string) string {
	t.Helper()
	for _, account := range demo.NewDataset().Members {
		if account.DisplayName == displayName {
			return account.AccountID
		}
	}
	t.Fatalf("no demo account named %q", displayName)
	return ""
}

// detailText is the whole scrollable body of the detail panel rather than the
// screenful the viewport happens to sit on: every half-page offset is rendered
// and the renders joined, so a line below the fold still reaches the assertion
// whole. Consecutive windows overlap by half a page, so no line falls between
// two of them.
func detailText(t *testing.T, app *App) string {
	t.Helper()
	var out strings.Builder
	for range 40 {
		out.WriteString(plain(app.detailView.View()))
		out.WriteByte('\n')
		app.detailView.ScrollBy(max(app.detailView.VisibleRows()/2, 1))
	}
	return out.String()
}

// expandThread walks the Comments tab down to the first thread match accepts
// and opens it in the read-only modal. The list collapses a block at
// maxBlockLines, so the modal is the only place a long thread can be read whole
// — and the only place a mention near its end can be asserted on.
func expandThread(t *testing.T, app *App, match func(bitbucket.Thread) bool) {
	t.Helper()
	for range 20 {
		_ = app.detailView.View()
		thread := app.detailView.SelectedThread()
		if thread == nil {
			t.Fatal("the Comments tab should have a thread under the cursor")
		}
		if match(*thread) {
			_, cmd := app.detailView.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if cmd == nil {
				t.Fatal("Enter on a thread should expand it")
			}
			runAll(t, app, cmd)
			if !app.modal.IsVisible() {
				t.Fatal("the expanded thread should open in a modal")
			}
			return
		}
		app.detailView.ScrollBy(1)
	}
	t.Fatal("no matching thread in the Comments tab")
}

func TestDemo_OverviewShowsTheMentionedDisplayName(t *testing.T) {
	t.Parallel()
	app := demoReviewApp(t, demoPRKey103)

	testkit.AssertEqual(t, "tab", app.detailView.ActiveTab(), views.TabOverview)
	rendered := detailText(t, app)

	// The account id the description is written against, read back as the name
	// Bitbucket's own rendering gives it.
	if !strings.Contains(rendered, mentionDemoUser+" the budget maths") {
		t.Errorf("the Overview should name the mentioned reviewer:\n%s", rendered)
	}
	// #103's description mentions one account, and the demo dataset renders it,
	// so nothing of the raw form may survive.
	if strings.Contains(rendered, "@{") {
		t.Errorf("the Overview still shows a raw mention:\n%s", rendered)
	}
}

func TestDemo_CommentsResolveWhatBitbucketRenderedAndKeepWhatItDidNot(t *testing.T) {
	t.Parallel()
	app := demoReviewApp(t, demoPRKey103)

	runAll(t, app, app.jumpToTab(views.TabComments))
	// The demo's checklist thread: a general root naming the colleague who has
	// left, and a reply naming two people, one of them twice.
	expandThread(t, app, func(thread bitbucket.Thread) bool {
		return !thread.IsInline() && strings.Contains(thread.Root.Content.Raw, "Before this merges")
	})

	rendered := plain(app.modal.View())
	// Two people named in one line, one of them twice: every occurrence the
	// rendering resolved reads as a name.
	want := mentionDemoUser + " " + mentionKen + " - " + mentionKen + " owns"
	if !strings.Contains(rendered, want) {
		t.Errorf("the thread is missing %q:\n%s", want, rendered)
	}
	// The colleague who has left is a mention Bitbucket could not resolve: it
	// is literal text in the html it served, so it stays literal here too.
	if !strings.Contains(rendered, "@{"+demo.AccountIDGone+"} wrote") {
		t.Errorf("an unresolvable mention should stay literal:\n%s", rendered)
	}
}

func TestDemo_EditingTheDescriptionResolvesTheNewMention(t *testing.T) {
	t.Parallel()
	app := demoReviewApp(t, demoPRKey103)
	app.cfg.Editor = testEditor

	previewed := app.pullRequestByKey(demoPRKey103)
	if previewed == nil {
		t.Fatalf("%s should be previewed", demoPRKey103)
	}
	original := previewed.Description
	if cmd := app.editDescription(demoPRKey103); cmd == nil {
		t.Fatal("editing a description should suspend into the editor")
	}
	body := "Rebased onto main. @{" + demoAccountID(t, "Ken Thompson") + "} please take another look.\n"
	if err := os.WriteFile(app.descriptionEditPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	_, cmd := app.Update(descriptionEditedMsg{
		key: demoPRKey103, path: app.descriptionEditPath, original: original, err: nil,
	})
	runAll(t, app, cmd)

	// The server re-rendered the description it was handed, so the account id
	// the editor typed comes back as a name without a second request.
	written := app.prCache[demoPRKey103]
	if written == nil {
		t.Fatalf("the edited pull request should be cached, help bar = %q", plain(app.helpBar.View()))
	}
	testkit.AssertEqual(t, "cached readable description", written.ReadableDescription(),
		"Rebased onto main. @Ken Thompson please take another look.")
	rendered := detailText(t, app)
	if !strings.Contains(rendered, "Rebased onto main. "+mentionKen+" please") {
		t.Errorf("the Overview should name the newly mentioned reviewer:\n%s", rendered)
	}
	if strings.Contains(rendered, "@{") {
		t.Errorf("the Overview still shows a raw mention:\n%s", rendered)
	}
}

func TestMentions_WithoutARenderingTheRawIDIsShown(t *testing.T) {
	t.Parallel()
	const (
		inDescription = "Ping " + testMentionRaw + " on this."
		inComment     = "Ping " + testMentionRaw + " about this line."
	)

	fake := &bitbuckettest.FakeClient{T: t}
	comment := testThreads()[0].Root
	// Neither body carries html here — a comment whose content.html is empty
	// and a pull request with no summary at all, the shape of a payload built
	// client-side rather than anything the list endpoint serves. Without that
	// oracle there is nothing to resolve the id against, so the raw mention is
	// what the reader gets — the id, never a guess.
	comment.Content = bitbucket.RenderedContent{Raw: inComment, HTML: ""}
	fake.ListPullRequestCommentsFunc = func(_ context.Context, _, _ string, _ int,
		_ bitbucket.ListOptions,
	) ([]bitbucket.Comment, error) {
		return []bitbucket.Comment{comment}, nil
	}

	app := reviewApp(t, fake)
	pr := app.prCache[testPRKey]
	pr.Description = inDescription
	pr.Summary = bitbucket.RenderedContent{}
	app.detailView.UpdatePullRequestData(pr)

	if rendered := detailText(t, app); !strings.Contains(rendered, inDescription) {
		t.Errorf("an unrendered description mention should stay raw:\n%s", rendered)
	}

	runAll(t, app, app.jumpToTab(views.TabComments))
	if rendered := detailText(t, app); !strings.Contains(rendered, inComment) {
		t.Errorf("an unrendered comment mention should stay raw:\n%s", rendered)
	}
}
