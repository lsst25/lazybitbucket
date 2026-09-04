package views

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

func testStatuses() []bitbucket.CommitStatus {
	now := time.Now()
	return []bitbucket.CommitStatus{
		{
			Key:       "prs:api:develop",
			Name:      "Pipeline - pullrequests: **",
			RefName:   "develop",
			State:     bitbucket.StatusSuccessful,
			URL:       "https://bitbucket.org/acme/api-gateway/pipelines/results/42",
			UpdatedOn: now.Add(-3 * time.Hour),
		},
		{
			Key:       "prs:lint",
			Name:      "Lint",
			RefName:   "feature/x",
			State:     bitbucket.StatusFailed,
			UpdatedOn: now.Add(-30 * time.Minute),
		},
	}
}

func makeFocusedBuildsPanel() *BuildsPanel {
	panel := NewBuildsPanel()
	panel.ResolveNav = navResolverForList
	panel.SetSize(80, 24)
	panel.SetFocused(true)
	panel.SetPullRequest(testPRKey)
	return panel
}

func TestBuildsPanel_View_NoPullRequestSelected(t *testing.T) {
	t.Parallel()
	panel := NewBuildsPanel()
	panel.SetSize(80, 10)

	out := stripANSI(panel.View())
	if !strings.Contains(out, "No pull request selected") {
		t.Errorf("View() = %q, want the empty-selection placeholder", out)
	}
	testkit.AssertEqual(t, "content height floor", panel.ContentHeight(), 3)
}

func TestBuildsPanel_View_LoadingThenStatuses(t *testing.T) {
	t.Parallel()
	panel := makeFocusedBuildsPanel()
	panel.SetLoading(testPRKey)

	if !strings.Contains(stripANSI(panel.View()), "Loading builds") {
		t.Error("View() should show the loading placeholder")
	}

	panel.SetStatuses(testPRKey, testStatuses())
	out := stripANSI(panel.View())
	for _, want := range []string{"✘", "Lint", "feature/x", "30m", "✔", "develop", "3h"} {
		if !strings.Contains(out, want) {
			t.Errorf("View() = %q, want it to contain %q", out, want)
		}
	}
}

func TestBuildsPanel_SetStatuses_SortsNewestFirst(t *testing.T) {
	t.Parallel()
	panel := makeFocusedBuildsPanel()
	panel.SetStatuses(testPRKey, testStatuses())

	got := panel.Statuses()
	if len(got) != 2 {
		t.Fatalf("len(Statuses()) = %d, want 2", len(got))
	}
	testkit.AssertEqual(t, "newest first", got[0].Name, "Lint")
}

func TestBuildsPanel_View_EmptyStatuses(t *testing.T) {
	t.Parallel()
	panel := makeFocusedBuildsPanel()
	panel.SetStatuses(testPRKey, nil)

	if !strings.Contains(stripANSI(panel.View()), "No builds") {
		t.Error("View() should show the empty placeholder")
	}
}

func TestBuildsPanel_SetError_ShowsTheMessage(t *testing.T) {
	t.Parallel()
	panel := makeFocusedBuildsPanel()
	panel.SetLoading(testPRKey)
	panel.SetError(testPRKey, "list statuses acme/api-gateway#101: status 500")

	out := stripANSI(panel.View())
	if !strings.Contains(out, "status 500") {
		t.Errorf("View() = %q, want the error text", out)
	}
}

func TestBuildsPanel_StaleKeysAreDropped(t *testing.T) {
	t.Parallel()
	panel := makeFocusedBuildsPanel()

	panel.SetStatuses(testPRKey2, testStatuses())
	if len(panel.Statuses()) != 0 {
		t.Error("SetStatuses with a stale key should be dropped")
	}

	panel.SetError(testPRKey2, "boom")
	if strings.Contains(stripANSI(panel.View()), "boom") {
		t.Error("SetError with a stale key should be dropped")
	}

	panel.SetLoading(testPRKey2)
	if strings.Contains(stripANSI(panel.View()), "Loading builds") {
		t.Error("SetLoading with a stale key should be dropped")
	}
}

func TestBuildsPanel_SetPullRequest_ResetsOnlyOnKeyChange(t *testing.T) {
	t.Parallel()
	panel := makeFocusedBuildsPanel()
	panel.SetStatuses(testPRKey, testStatuses())
	panel.Cursor = 1

	panel.SetPullRequest(testPRKey)
	testkit.AssertEqual(t, "key", panel.Key(), testPRKey)
	testkit.AssertEqual(t, "cursor preserved", panel.Cursor, 1)
	testkit.AssertEqual(t, "statuses preserved", len(panel.Statuses()), 2)

	panel.SetPullRequest(testPRKey2)
	testkit.AssertEqual(t, "cursor reset", panel.Cursor, 0)
	testkit.AssertEqual(t, "statuses cleared", len(panel.Statuses()), 0)
}

func TestBuildsPanel_SelectedStatus_ReturnsACopy(t *testing.T) {
	t.Parallel()
	panel := makeFocusedBuildsPanel()
	if panel.SelectedStatus() != nil {
		t.Error("SelectedStatus() should be nil with no statuses")
	}

	panel.SetStatuses(testPRKey, testStatuses())
	status := panel.SelectedStatus()
	if status == nil {
		t.Fatal("SelectedStatus() = nil")
	}
	status.Name = "mutated"
	testkit.AssertEqual(t, "panel unchanged", panel.SelectedStatus().Name, "Lint")
}

func TestBuildsPanel_Update_MovesTheCursor(t *testing.T) {
	t.Parallel()
	panel := makeFocusedBuildsPanel()
	panel.SetStatuses(testPRKey, testStatuses())

	got, cmd := panel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if cmd != nil {
		t.Error("the builds panel emits no message on a cursor move")
	}
	testkit.AssertEqual(t, "cursor", got.Cursor, 1)
	testkit.AssertEqual(t, "selected", got.SelectedStatus().Name, "Pipeline - pullrequests: **")
}

func TestBuildsPanel_Update_UnfocusedIsInert(t *testing.T) {
	t.Parallel()
	panel := makeFocusedBuildsPanel()
	panel.SetStatuses(testPRKey, testStatuses())
	panel.SetFocused(false)

	got, _ := panel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	testkit.AssertEqual(t, "cursor", got.Cursor, 0)
}

func TestBuildsPanel_SetFilter_NarrowsRows(t *testing.T) {
	t.Parallel()
	panel := makeFocusedBuildsPanel()
	panel.SetStatuses(testPRKey, testStatuses())

	panel.SetFilter("lint")
	out := stripANSI(panel.View())
	if !strings.Contains(out, "Lint") || strings.Contains(out, "develop") {
		t.Errorf("View() = %q, want only the Lint row", out)
	}

	panel.ClearFilter()
	if !strings.Contains(stripANSI(panel.View()), "develop") {
		t.Error("ClearFilter() should restore every row")
	}
}

func TestBuildsPanel_View_CollapsedBarWhenHeightOne(t *testing.T) {
	t.Parallel()
	panel := makeFocusedBuildsPanel()
	panel.SetStatuses(testPRKey, testStatuses())
	panel.SetSize(80, 1)

	out := stripANSI(panel.View())
	if !strings.Contains(out, "[3] Builds") {
		t.Errorf("collapsed bar = %q, want the panel title", out)
	}
	if strings.Contains(out, "Lint") {
		t.Errorf("collapsed bar = %q, want no rows", out)
	}
}

func TestBuildsPanel_View_FallsBackToTheStatusKeyWhenUnnamed(t *testing.T) {
	t.Parallel()
	panel := makeFocusedBuildsPanel()
	panel.SetStatuses(testPRKey, []bitbucket.CommitStatus{{
		Key:       "prs:unnamed",
		State:     bitbucket.StatusInProgress,
		UpdatedOn: time.Now().Add(-time.Minute),
	}})

	out := stripANSI(panel.View())
	if !strings.Contains(out, "prs:unnamed") {
		t.Errorf("View() = %q, want the status key as the row label", out)
	}
	if !strings.Contains(out, "●") {
		t.Errorf("View() = %q, want the in-progress glyph", out)
	}
}

func TestBuildsPanel_SetFilter_SelectionFollowsTheVisibleRow(t *testing.T) {
	t.Parallel()
	panel := makeFocusedBuildsPanel()
	panel.SetStatuses(testPRKey, testStatuses())

	// "Lint" is newest and sits under the cursor; the filter leaves only the
	// other row, so the selection has to move with it.
	panel.SetFilter("pullrequests")

	out := stripANSI(panel.View())
	if !strings.Contains(out, "1 of 1") {
		t.Errorf("View() = %q, want the footer to count only the matching row", out)
	}
	if strings.Contains(out, "Lint") {
		t.Errorf("View() = %q, want the filtered-out row gone", out)
	}
	testkit.AssertEqual(t, "item count", panel.ItemCount(), 1)

	status := panel.SelectedStatus()
	if status == nil {
		t.Fatal("SelectedStatus() = nil while a row is highlighted")
	}
	testkit.AssertEqual(t, "selection matches the drawn row", status.Key, "prs:api:develop")

	panel.ClearFilter()
	testkit.AssertEqual(t, "item count restored", panel.ItemCount(), 2)
}

func TestBuildsPanel_SetFilter_NoMatchesSelectsNothing(t *testing.T) {
	t.Parallel()
	panel := makeFocusedBuildsPanel()
	panel.SetStatuses(testPRKey, testStatuses())

	panel.SetFilter("nothing-matches-this")

	if !strings.Contains(stripANSI(panel.View()), "No matching builds") {
		t.Errorf("View() = %q, want the empty-filter placeholder", stripANSI(panel.View()))
	}
	if panel.SelectedStatus() != nil {
		t.Error("SelectedStatus() should be nil while no row is drawn")
	}
}

func TestBuildsPanel_SelectByKey(t *testing.T) {
	t.Parallel()
	panel := makeFocusedBuildsPanel()
	panel.SetStatuses(testPRKey, testStatuses())

	if !panel.SelectByKey("prs:api:develop") {
		t.Fatal("SelectByKey() did not find the status")
	}
	testkit.AssertEqual(t, "cursor", panel.Cursor, 1)
	if panel.SelectByKey("prs:missing") {
		t.Error("SelectByKey() should report an unknown key as not found")
	}
}
