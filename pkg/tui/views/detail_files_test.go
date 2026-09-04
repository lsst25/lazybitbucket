package views

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// testDiffStat is the changed-file list the Files tab tests render: one file per
// status plus a binary one, so every glyph and both count columns appear.
func testDiffStat() []bitbucket.DiffStatEntry {
	return []bitbucket.DiffStatEntry{
		{
			Status: bitbucket.DiffStatModified, LinesAdded: 12, LinesRemoved: 3,
			New: &bitbucket.CommitFile{Path: testDiffPath, EscapedPath: testDiffPath},
		},
		{
			Status: bitbucket.DiffStatAdded, LinesAdded: 120,
			New: &bitbucket.CommitFile{Path: "pkg/new/thing.go"},
		},
		{
			Status: bitbucket.DiffStatRemoved, LinesRemoved: 9,
			Old: &bitbucket.CommitFile{Path: "pkg/old/gone.go"},
		},
		{
			Status: bitbucket.DiffStatRenamed, LinesAdded: 1, LinesRemoved: 1,
			Old: &bitbucket.CommitFile{Path: "a.go"},
			New: &bitbucket.CommitFile{Path: "b.go"},
		},
		{
			Status: bitbucket.DiffStatModified,
			New:    &bitbucket.CommitFile{Path: "docs/logo.png", Attributes: []string{"binary"}},
		},
	}
}

// filesDetail returns a focused detail view on the Files tab with the diffstat
// loaded and View() called, so the block cache is populated.
func filesDetail(t *testing.T) *DetailView {
	t.Helper()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())
	view.SetDiffStat(testPRKey, testDiffStat())
	view.SetActiveTab(TabFiles)
	view.View()
	return view
}

func TestDetailView_Files_RendersOneBlockPerChangedFile(t *testing.T) {
	t.Parallel()
	out := stripANSI(filesDetail(t).View())

	for _, want := range []string{
		"M " + testDiffPath, "+12 −3",
		"A pkg/new/thing.go", "+120",
		"D pkg/old/gone.go", "−9",
		"R a.go → b.go", "+1 −1",
		"M docs/logo.png B",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("view does not contain %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "1 of 5") {
		t.Errorf("footer should count the five files:\n%s", out)
	}
}

func TestDetailView_Files_PlaceholderStates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		setup func(view *DetailView)
		want  string
	}{
		{"before the fetch", func(*DetailView) {}, "Loading files…"},
		{"loading", func(v *DetailView) { v.SetDiffStatLoading(testPRKey) }, "Loading files…"},
		{
			"error",
			func(v *DetailView) { v.SetDiffStatError(testPRKey, "diff stat too large") },
			"diff stat too large",
		},
		{"empty", func(v *DetailView) { v.SetDiffStat(testPRKey, nil) }, "No changes"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			view := makeFocusedDetail()
			view.SetPullRequest(detailPR())
			view.SetActiveTab(TabFiles)
			tc.setup(view)

			out := stripANSI(view.View())
			if !strings.Contains(out, tc.want) {
				t.Errorf("view does not contain %q:\n%s", tc.want, out)
			}
			// A placeholder is the only block, so the cursor has exactly one
			// thing to sit on and SelectedFile has nothing to hand back.
			testkit.AssertEqual(t, "block count", view.listTabItemCount(), 1)
			if got := view.SelectedFile(); got != nil {
				t.Errorf("SelectedFile() = %v, want nil", got)
			}
		})
	}
}

func TestDetailView_Files_TabLabelsCarryTheCount(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())

	testkit.AssertEqual(t, "unknown", stripANSI(view.buildTitle(90)),
		"[0] #101 "+testRepo+" - Overview - Files - Commits - Comments")

	view.SetDiffStatLoading(testPRKey)
	if got := stripANSI(view.buildTitle(90)); !strings.Contains(got, "Files (…)") {
		t.Errorf("title = %q, want an ellipsis count while loading", got)
	}

	view.SetDiffStat(testPRKey, testDiffStat())
	view.SetCommits(testPRKey, nil)
	if got := stripANSI(view.buildTitle(90)); !strings.Contains(got, "Files (5) - Commits (0)") {
		t.Errorf("title = %q, want the loaded counts", got)
	}
}

func TestDetailView_BuildTitle_DropsCountsThenSlugToFit(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())
	view.SetDiffStat(testPRKey, testDiffStat())
	view.SetCommits(testPRKey, testCommits())
	view.SetThreads(testPRKey, testThreads())

	full := stripANSI(view.buildTitle(90))
	testkit.AssertEqual(t, "full title",
		full, "[0] #101 "+testRepo+" - Overview - Files (5) - Commits (2) - Comments (4)")

	// At 60 columns the counts go first; the slug survives because the bar
	// then fits.
	tight := stripANSI(view.buildTitle(60))
	testkit.AssertEqual(t, "counts dropped",
		tight, "[0] #101 "+testRepo+" - Overview - Files - Commits - Comments")

	// At 50 the slug goes too.
	tighter := stripANSI(view.buildTitle(50))
	testkit.AssertEqual(t, "slug dropped",
		tighter, "[0] #101 - Overview - Files - Commits - Comments")

	// And a title that still does not fit is truncated to the border's budget.
	clipped := stripANSI(view.buildTitle(30))
	if got := lipgloss.Width(clipped); got > 30 {
		t.Errorf("buildTitle(30) = %q, %d columns", clipped, got)
	}
}

func TestDetailView_ClickTab_HitTestsRenderedWidths(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())
	view.SetDiffStat(testPRKey, testDiffStat())
	// View records the zones the click is tested against.
	view.View()

	// "╭─" then "[0] #101 api-gateway" then " - Overview - Files (5) - …".
	// Column 2 + 20 + 3 is the "O" of Overview.
	testkit.AssertEqual(t, "click on the active tab changes nothing", view.ClickTab(25), false)

	filesStart := 2 + len("[0] #101 "+testRepo) + len(" - Overview - ")
	testkit.AssertEqual(t, "click on Files", view.ClickTab(filesStart+1), true)
	testkit.AssertEqual(t, "tab", view.ActiveTab(), TabFiles)

	testkit.AssertEqual(t, "click past the bar", view.ClickTab(200), false)
	testkit.AssertEqual(t, "tab unchanged", view.ActiveTab(), TabFiles)
	testkit.AssertEqual(t, "click on the border", view.ClickTab(0), false)
}

func TestDetailView_Files_EnterEmitsFileSelected(t *testing.T) {
	t.Parallel()
	view := filesDetail(t)
	view.ScrollBy(1)

	_, cmd := view.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter on a file emitted nothing")
	}
	msg, ok := cmd().(FileSelectedMsg)
	if !ok {
		t.Fatalf("msg = %T, want FileSelectedMsg", cmd())
	}
	testkit.AssertEqual(t, "key", msg.PullRequestKey, testPRKey)
	testkit.AssertEqual(t, "path", msg.Path, "pkg/new/thing.go")
}

func TestDetailView_Files_DoubleClickOpensTheFile(t *testing.T) {
	t.Parallel()
	view := filesDetail(t)

	// relY 1 is the first body row; the Files tab has no list header.
	if cmd := view.ClickItem(1); cmd != nil {
		t.Error("a single click should not open a file")
	}
	cmd := view.ClickItem(1)
	if cmd == nil {
		t.Fatal("a double click should open the file")
	}
	msg, ok := cmd().(FileSelectedMsg)
	if !ok {
		t.Fatalf("msg = %T, want FileSelectedMsg", cmd())
	}
	testkit.AssertEqual(t, "path", msg.Path, testDiffPath)
}

func TestDetailView_Files_SelectedFileTracksTheCursor(t *testing.T) {
	t.Parallel()
	view := filesDetail(t)

	testkit.AssertEqual(t, "first file", view.SelectedFile().Path(), testDiffPath)
	view.ScrollBy(3)
	testkit.AssertEqual(t, "fourth file", view.SelectedFile().Path(), "b.go")

	view.SetActiveTab(TabOverview)
	if got := view.SelectedFile(); got != nil {
		t.Errorf("SelectedFile() outside the tab = %v, want nil", got)
	}
}

func TestDetailView_Review_StaleKeysAreDropped(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())

	view.SetDiffStat(testPRKey2, testDiffStat())
	view.SetDiffStatLoading(testPRKey2)
	view.SetDiffStatError(testPRKey2, "boom")
	view.SetCommits(testPRKey2, testCommits())
	view.SetCommitsLoading(testPRKey2)
	view.SetCommitsError(testPRKey2, "boom")
	view.SetThreads(testPRKey2, testThreads())
	view.SetThreadsLoading(testPRKey2)
	view.SetThreadsError(testPRKey2, "boom")

	if got := view.DiffStat(); got != nil {
		t.Errorf("DiffStat() = %v, want nil", got)
	}
	if got := view.Commits(); got != nil {
		t.Errorf("Commits() = %v, want nil", got)
	}
	if got := view.Threads(); got != nil {
		t.Errorf("Threads() = %v, want nil", got)
	}
	if strings.Contains(stripANSI(view.buildTitle(90)), "(") {
		t.Error("a dropped response must not put a count in the tab bar")
	}
}

func TestDetailView_Review_SelectingAnotherPullRequestResetsTheTabs(t *testing.T) {
	t.Parallel()
	view := filesDetail(t)
	testkit.AssertEqual(t, "tab", view.ActiveTab(), TabFiles)

	// A background refresh of the same pull request keeps everything.
	same := detailPR()
	same.Title = "Refreshed"
	view.SetPullRequest(same)
	testkit.AssertEqual(t, "tab kept", view.ActiveTab(), TabFiles)
	testkit.AssertEqual(t, "files kept", len(view.DiffStat()), 5)

	// UpdatePullRequestData keeps them too.
	view.UpdatePullRequestData(same)
	testkit.AssertEqual(t, "files still there", len(view.DiffStat()), 5)

	other := newPR(102, testRepo, "Another")
	view.SetPullRequest(&other)
	testkit.AssertEqual(t, "tab reset", view.ActiveTab(), TabOverview)
	if got := view.DiffStat(); got != nil {
		t.Errorf("DiffStat() = %v, want nil after selecting another pull request", got)
	}
}

func TestDetailView_NextAndPrevFile_MoveTheCursorInTheFilesTab(t *testing.T) {
	t.Parallel()
	view := filesDetail(t)

	if _, ok := view.PrevFile(); ok {
		t.Error("PrevFile() at the first file should report false")
	}
	path, ok := view.NextFile()
	testkit.AssertEqual(t, "next ok", ok, true)
	testkit.AssertEqual(t, "next path", path, "pkg/new/thing.go")
	testkit.AssertEqual(t, "cursor moved", view.SelectedFile().Path(), "pkg/new/thing.go")

	view.ScrollBy(9) // clamp to the last file
	if _, ok := view.NextFile(); ok {
		t.Error("NextFile() at the last file should report false")
	}
	back, ok := view.PrevFile()
	testkit.AssertEqual(t, "prev ok", ok, true)
	testkit.AssertEqual(t, "prev path", back, "b.go")
}

func TestDetailView_NextFile_IsInertOutsideTheFileViews(t *testing.T) {
	t.Parallel()
	view := makeFocusedDetail()
	view.SetPullRequest(detailPR())
	view.SetDiffStat(testPRKey, testDiffStat())

	if _, ok := view.NextFile(); ok {
		t.Error("NextFile() on the Overview tab should report false")
	}

	view.SetActiveTab(TabFiles)
	view.SetDiffStat(testPRKey, nil)
	if _, ok := view.NextFile(); ok {
		t.Error("NextFile() with no changed files should report false")
	}
}

func TestDetailView_Files_RefetchKeepsTheListAndTheCursor(t *testing.T) {
	t.Parallel()
	view := filesDetail(t)
	view.ScrollBy(3)
	testkit.AssertEqual(t, "cursor", view.SelectedFile().Path(), "b.go")

	view.SetDiffStatLoading(testPRKey)
	out := stripANSI(view.View())

	if strings.Contains(out, "Loading files…") {
		t.Errorf("a refetch must not replace the list it is refreshing:\n%s", out)
	}
	testkit.AssertEqual(t, "block count", view.listTabItemCount(), 5)
	if !strings.Contains(out, "4 of 5") {
		t.Errorf("footer should still count five files, cursor on the fourth:\n%s", out)
	}
	testkit.AssertEqual(t, "cursor kept", view.SelectedFile().Path(), "b.go")
	// The in-flight state is the tab label's job, not the list's.
	if title := stripANSI(view.buildTitle(90)); !strings.Contains(title, "Files (…)") {
		t.Errorf("tab label should carry the in-flight state, got %q", title)
	}

	// And the cursor survives the response landing.
	view.SetDiffStat(testPRKey, testDiffStat())
	view.View()
	testkit.AssertEqual(t, "cursor after the response", view.SelectedFile().Path(), "b.go")
}

func TestDetailView_Files_RefetchErrorKeepsTheListAndReportsItAbove(t *testing.T) {
	t.Parallel()
	view := filesDetail(t)
	view.ScrollBy(3)

	view.SetDiffStatError(testPRKey, "403 forbidden")
	out := stripANSI(view.View())

	for _, want := range []string{"403 forbidden", "M " + testDiffPath, "4 of 5"} {
		if !strings.Contains(out, want) {
			t.Errorf("view does not contain %q:\n%s", want, out)
		}
	}
	testkit.AssertEqual(t, "block count", view.listTabItemCount(), 5)
	testkit.AssertEqual(t, "cursor kept", view.SelectedFile().Path(), "b.go")
}
