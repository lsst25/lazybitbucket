package tui

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
	"github.com/lsst25/lazybitbucket/pkg/viewed"
)

// testOldSourceHash is the head the pull request had before a push.
const testOldSourceHash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

// pressViewed renders the panel, which builds the block list the cursor reads,
// and presses the viewed key.
func pressViewed(t *testing.T, app *App) {
	t.Helper()
	_ = app.detailView.View()
	_, cmd := app.Update(runeKey('v'))
	runAll(t, app, cmd)
}

func TestToggleViewed_OnTheFilesTabMarksAndMovesOn(t *testing.T) {
	t.Parallel()
	app := fileDiffApp(t, &bitbuckettest.FakeClient{T: t})

	pressViewed(t, app)

	store := app.viewedStore()
	testkit.AssertEqual(t, "first file marked", store.Viewed(testPRKey, testFileGo, testSourceHash), true)
	testkit.AssertEqual(t, "view agrees", app.detailView.FileViewed(testFileGo), true)
	selected := app.detailView.SelectedFile()
	if selected == nil {
		t.Fatal("no file under the cursor after marking")
	}
	testkit.AssertEqual(t, "cursor moved to the next file", selected.Path(), testFileMD)

	rendered := plain(app.detailView.View())
	if !strings.Contains(rendered, "✔ M "+testFileGo) {
		t.Errorf("the marked file has no tick:\n%s", rendered)
	}
}

func TestToggleViewed_AgainClearsTheMark(t *testing.T) {
	t.Parallel()
	app := fileDiffApp(t, &bitbuckettest.FakeClient{T: t})
	pressViewed(t, app)
	_, _ = app.detailView.PrevFile()

	pressViewed(t, app)

	testkit.AssertEqual(t, "mark cleared", app.viewedStore().Viewed(testPRKey, testFileGo, testSourceHash), false)
	testkit.AssertEqual(t, "view agrees", app.detailView.FileViewed(testFileGo), false)
	if !strings.Contains(plain(app.helpBar.View()), "not viewed") {
		t.Errorf("help bar = %q, want the cleared note", plain(app.helpBar.View()))
	}
}

func TestToggleViewed_InADiffOpensTheNextFileThenGoesBack(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubDiff(fake)
	stubComments(fake)
	app := fileDiffApp(t, fake)
	runAll(t, app, app.openFileDiff(testFileGo))

	pressViewed(t, app)
	testkit.AssertEqual(t, "next diff open", app.detailView.FileDiffPath(), testFileMD)

	pressViewed(t, app)
	testkit.AssertEqual(t, "back on the list", app.detailView.Mode(), views.ModePullRequest)
	testkit.AssertEqual(t, "on the Files tab", app.detailView.ActiveTab(), views.TabFiles)
	testkit.AssertEqual(t, "both marked", len(app.viewedStore().ViewedPaths(testPRKey, testSourceHash)), 2)
	if !strings.Contains(plain(app.helpBar.View()), "every file viewed") {
		t.Errorf("help bar = %q, want the all-viewed note", plain(app.helpBar.View()))
	}
}

func TestToggleViewed_WorksInReadOnlyMode(t *testing.T) {
	t.Parallel()
	app := fileDiffApp(t, &bitbuckettest.FakeClient{T: t})
	app.readOnly = true

	pressViewed(t, app)

	testkit.AssertEqual(t, "marked", app.viewedStore().Viewed(testPRKey, testFileGo, testSourceHash), true)
}

func TestToggleViewed_DoesNothingOffTheFilesTab(t *testing.T) {
	t.Parallel()
	app := fileDiffApp(t, &bitbuckettest.FakeClient{T: t})
	runAll(t, app, app.jumpToTab(views.TabOverview))

	pressViewed(t, app)

	testkit.AssertEqual(t, "nothing marked", len(app.viewedStore().ViewedPaths(testPRKey, testSourceHash)), 0)
}

// pushedApp is reviewApp with both files marked at the head the branch had
// before a push, and the given comparison answer stubbed.
func pushedApp(
	t *testing.T, compare func(ctx context.Context, ws, slug, from, to string, paths []string) ([]bitbucket.DiffStatEntry, error),
) (*App, *bitbuckettest.FakeClient) {
	t.Helper()
	fake := &bitbuckettest.FakeClient{T: t}
	stubDiffStat(fake)
	fake.CompareDiffStatFunc = compare
	app := reviewApp(t, fake)
	store := viewed.New()
	store.Mark(testPRKey, testFileGo, testOldSourceHash)
	store.Mark(testPRKey, testFileMD, testOldSourceHash)
	app.SetViewedStore(store, nil)
	return app, fake
}

func TestViewedAfterAPush_KeepsTheFilesThePushDidNotTouch(t *testing.T) {
	t.Parallel()
	app, fake := pushedApp(t, func(_ context.Context, _, _, _, _ string, _ []string) ([]bitbucket.DiffStatEntry, error) {
		return testDiffStat()[:1], nil // only the Go file changed
	})

	runAll(t, app, app.jumpToTab(views.TabFiles))

	testkit.AssertEqual(t, "one comparison", fake.CallCount("CompareDiffStat"), 1)
	call := fake.CompareDiffStatCalls[0]
	testkit.AssertEqual(t, "newer commit first", call.From, testSourceHash)
	testkit.AssertEqual(t, "older commit second", call.To, testOldSourceHash)
	testkit.AssertSliceEqual(t, "only the marked paths", call.Paths, []string{testFileMD, testFileGo})

	testkit.AssertEqual(t, "changed file unmarked", app.detailView.FileViewed(testFileGo), false)
	testkit.AssertEqual(t, "untouched file still viewed", app.detailView.FileViewed(testFileMD), true)
	testkit.AssertEqual(t, "nothing stale left", len(app.viewedStore().Stale(testPRKey, testSourceHash)), 0)
}

func TestViewedAfterAPush_ACommitTheServerLostDropsTheMarks(t *testing.T) {
	t.Parallel()
	app, _ := pushedApp(t, func(_ context.Context, _, _, _, _ string, _ []string) ([]bitbucket.DiffStatEntry, error) {
		return nil, &bitbucket.APIError{Method: http.MethodGet, Status: http.StatusNotFound, Message: "Commit not found"}
	})

	runAll(t, app, app.jumpToTab(views.TabFiles))

	testkit.AssertEqual(t, "every mark dropped", len(app.viewedStore().Stale(testPRKey, testSourceHash)), 0)
	testkit.AssertEqual(t, "nothing shown", app.detailView.ViewedFileCount(), 0)
}

func TestViewedAfterAPush_AFailedComparisonKeepsTheMarksHidden(t *testing.T) {
	t.Parallel()
	app, _ := pushedApp(t, func(_ context.Context, _, _, _, _ string, _ []string) ([]bitbucket.DiffStatEntry, error) {
		return nil, errors.New("connection reset")
	})

	runAll(t, app, app.jumpToTab(views.TabFiles))

	testkit.AssertEqual(t, "marks kept for the next try", len(app.viewedStore().Stale(testPRKey, testSourceHash)[testOldSourceHash]), 2)
	testkit.AssertEqual(t, "but not shown", app.detailView.ViewedFileCount(), 0)
}

func TestViewedAfterAPush_AsksOnlyOnceWhileTheAnswerIsOnItsWay(t *testing.T) {
	t.Parallel()
	app, fake := pushedApp(t, func(_ context.Context, _, _, _, _ string, _ []string) ([]bitbucket.DiffStatEntry, error) {
		return nil, nil
	})
	pr := app.currentPullRequest()

	first := app.reconcileViewed(pr)
	second := app.reconcileViewed(pr)
	if first == nil {
		t.Fatal("the first call issued no comparison")
	}
	if second != nil {
		t.Error("the second call asked again while the first answer was in flight")
	}
	runAll(t, app, first)
	testkit.AssertEqual(t, "one comparison", fake.CallCount("CompareDiffStat"), 1)
}

func TestSetViewedStore_AnUnreadableFileFallsBackToMemory(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})

	app.SetViewedStore(nil, errors.New("parse viewed files: unexpected end of JSON input"))
	if !strings.Contains(app.statusWarning, "viewed files not loaded") {
		t.Errorf("status warning = %q, want the load failure", app.statusWarning)
	}

	store := app.viewedStore()
	store.Mark(testPRKey, testFileGo, testSourceHash)
	testkit.AssertEqual(t, "memory store works", store.Viewed(testPRKey, testFileGo, testSourceHash), true)
	if err := store.Save(); err != nil {
		t.Errorf("the fallback store tried to write: %v", err)
	}
}
