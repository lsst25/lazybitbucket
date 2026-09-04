package views

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/config"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
)

// navResolverForList maps the vim keys used by the list tests. Without a
// resolver ListBase.KeyNav is inert, so every focused fixture installs one.
func navResolverForList(key string) components.NavAction {
	switch key {
	case "j":
		return components.NavDown
	case "k":
		return components.NavUp
	case "g":
		return components.NavTop
	case "G":
		return components.NavBottom
	default:
		return components.NavNone
	}
}

// newPR builds an OPEN pull request with no review data: 2 days old, updated 3
// hours ago, so timeAgo renders "2d" and "3h".
func newPR(id int, repo, title string) bitbucket.PullRequest {
	return bitbucket.PullRequest{
		ID:        id,
		Title:     title,
		State:     bitbucket.PullRequestOpen,
		Author:    bitbucket.Account{UUID: "{author}", DisplayName: testAuthor},
		CreatedOn: time.Now().Add(-48 * time.Hour),
		UpdatedOn: time.Now().Add(-3 * time.Hour),
		Source:    bitbucket.PullRequestEndpoint{Branch: bitbucket.Branch{Name: "feature/x"}},
		Destination: bitbucket.PullRequestEndpoint{
			Branch:     bitbucket.Branch{Name: "main"},
			Repository: bitbucket.RepositoryRef{FullName: testWorkspace + "/" + repo},
		},
	}
}

// withApproval attaches one approving reviewer, which makes HasReviewData true.
func withApproval(pr bitbucket.PullRequest) bitbucket.PullRequest {
	reviewer := bitbucket.Account{UUID: "{reviewer}", DisplayName: testReviewer}
	pr.Reviewers = []bitbucket.Account{reviewer}
	pr.Participants = []bitbucket.Participant{{
		User:     reviewer,
		Role:     bitbucket.RoleReviewer,
		Approved: true,
		State:    bitbucket.ParticipantApproved,
	}}
	return pr
}

func makeFocusedPRList(prs ...bitbucket.PullRequest) *PullRequestList {
	list := NewPullRequestList()
	list.ResolveNav = navResolverForList
	list.SetTabs(config.DefaultPullRequestTabs())
	list.SetSize(80, 24)
	list.SetFocused(true)
	list.SetPullRequests(prs)
	return list
}

func TestPullRequestList_View_ShowsRepoIDAndTitle(t *testing.T) {
	t.Parallel()
	list := makeFocusedPRList(newPR(101, testRepo, testPRTitle))

	out := stripANSI(list.View())
	for _, want := range []string{testRepo, "#101", "Add gateway"} {
		if !strings.Contains(out, want) {
			t.Errorf("View() = %q, want it to contain %q", out, want)
		}
	}
}

func TestPullRequestList_View_CollapsedBarWhenHeightOne(t *testing.T) {
	t.Parallel()
	list := makeFocusedPRList(newPR(101, testRepo, testPRTitle))
	list.SetSize(80, 1)

	out := stripANSI(list.View())
	if !strings.Contains(out, "Open") {
		t.Errorf("collapsed bar = %q, want the active tab label", out)
	}
	if strings.Contains(out, "#101") {
		t.Errorf("collapsed bar = %q, want no rows", out)
	}
}

func TestPullRequestList_View_NoTabsUsesDefaultTitle(t *testing.T) {
	t.Parallel()
	list := NewPullRequestList()
	list.SetSize(80, 24)

	out := stripANSI(list.View())
	if !strings.Contains(out, "[2] Pull Requests") {
		t.Errorf("View() = %q, want the default title", out)
	}
}

func TestPullRequestList_View_AllColumnsAtWidth120(t *testing.T) {
	t.Parallel()
	list := makeFocusedPRList(withApproval(newPR(101, testRepo, testPRTitle)))
	list.SetFields(config.PullRequestListFieldNames())
	list.SetBuildState(testPRKey, bitbucket.StatusSuccessful)
	list.SetSize(120, 24)

	out := stripANSI(list.View())
	for _, want := range []string{testRepo, "#101", testPRTitle, testAuthor, "✓1", "✔", "3h", "feature/x", "OPEN"} {
		if !strings.Contains(out, want) {
			t.Errorf("View() at width 120 = %q, want it to contain %q", out, want)
		}
	}
}

func TestPullRequestList_View_NarrowWidthDropsTrailingColumns(t *testing.T) {
	t.Parallel()
	list := makeFocusedPRList(withApproval(newPR(101, testRepo, testPRTitle)))
	list.SetBuildState(testPRKey, bitbucket.StatusFailed)
	// Panel width 40 leaves a 38-column list, the usual side-panel width.
	list.SetSize(40, 24)

	out := stripANSI(list.View())
	for _, want := range []string{"api-gate", "#101", "Add gateway", "✓1", "✘"} {
		if !strings.Contains(out, want) {
			t.Errorf("View() at width 38 = %q, want it to contain %q", out, want)
		}
	}
	for _, unwanted := range []string{testAuthor, "Grace"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("View() at width 38 = %q, want the author column dropped", out)
		}
	}
}

func TestPullRequestList_BuildGlyphs_CoverEveryRollupState(t *testing.T) {
	t.Parallel()
	cases := []struct {
		state bitbucket.CommitStatusState
		glyph string
	}{
		{bitbucket.StatusSuccessful, "✔"},
		{bitbucket.StatusFailed, "✘"},
		{bitbucket.StatusInProgress, "●"},
		{bitbucket.StatusStopped, "■"},
	}
	for _, tc := range cases {
		t.Run(string(tc.state), func(t *testing.T) {
			t.Parallel()
			statuses := []bitbucket.CommitStatus{{
				Key:       "prs:**",
				State:     tc.state,
				UpdatedOn: time.Now().Add(-time.Hour),
			}}
			list := makeFocusedPRList(newPR(101, testRepo, testPRTitle))
			list.SetBuildState(testPRKey, bitbucket.RollupStatuses(statuses))

			out := stripANSI(list.View())
			if !strings.Contains(out, tc.glyph) {
				t.Errorf("View() = %q, want build glyph %q", out, tc.glyph)
			}
		})
	}
}

func TestPullRequestList_Update_FocusedNavEmitsSelected(t *testing.T) {
	t.Parallel()
	list := makeFocusedPRList(
		newPR(101, testRepo, testPRTitle),
		newPR(102, testRepo, "Second"),
	)

	_, cmd := list.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if cmd == nil {
		t.Fatal("Update() returned no command after a cursor move")
	}
	msg, ok := cmd().(PullRequestSelectedMsg)
	if !ok {
		t.Fatalf("msg = %T, want PullRequestSelectedMsg", cmd())
	}
	if msg.PullRequest == nil {
		t.Fatal("PullRequestSelectedMsg carried no pull request")
	}
	testkit.AssertEqual(t, "selected key", msg.PullRequest.Key(), testPRKey2)
}

func TestPullRequestList_Update_UnfocusedIsInert(t *testing.T) {
	t.Parallel()
	list := makeFocusedPRList(newPR(101, testRepo, testPRTitle), newPR(102, testRepo, "Second"))
	list.SetFocused(false)

	_, cmd := list.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if cmd != nil {
		t.Error("unfocused Update() should return no command")
	}
	testkit.AssertEqual(t, "cursor", list.Cursor, 0)
}

func TestPullRequestList_SetPullRequests_KeepsSelectionByKey(t *testing.T) {
	t.Parallel()
	list := makeFocusedPRList(
		newPR(101, testRepo, testPRTitle),
		newPR(102, testRepo, "Second"),
	)
	list.Cursor = 1

	// The refresh reorders the two pull requests.
	list.SetPullRequests([]bitbucket.PullRequest{
		newPR(102, testRepo, "Second"),
		newPR(101, testRepo, testPRTitle),
	})

	testkit.AssertEqual(t, "cursor", list.Cursor, 0)
	sel := list.SelectedPullRequest()
	if sel == nil {
		t.Fatal("SelectedPullRequest() = nil")
	}
	testkit.AssertEqual(t, "selected key", sel.Key(), testPRKey2)
}

func TestPullRequestList_PatchPullRequest_KeepsCursor(t *testing.T) {
	t.Parallel()
	list := makeFocusedPRList(
		newPR(101, testRepo, testPRTitle),
		newPR(102, testRepo, "Second"),
	)
	list.Cursor = 1

	patched := withApproval(newPR(101, testRepo, "Patched title"))
	list.PatchPullRequest(&patched)

	testkit.AssertEqual(t, "cursor", list.Cursor, 1)
	testkit.AssertEqual(t, "selected key", list.SelectedPullRequest().Key(), testPRKey2)
	testkit.AssertEqual(t, "patched title", list.CurrentPullRequests()[0].Title, "Patched title")
	if !list.CurrentPullRequests()[0].HasReviewData() {
		t.Error("patched pull request should carry the new review data")
	}
}

func TestPullRequestList_PatchPullRequest_NilIsIgnored(t *testing.T) {
	t.Parallel()
	list := makeFocusedPRList(newPR(101, testRepo, testPRTitle))
	list.PatchPullRequest(nil)
	testkit.AssertEqual(t, "item count", list.ItemCount(), 1)
}

func TestPullRequestList_TabCache_ClickTabAtShowsThatTabsRows(t *testing.T) {
	t.Parallel()
	list := makeFocusedPRList(newPR(101, testRepo, testPRTitle))
	list.SetPullRequestsForTab(1, []bitbucket.PullRequest{newPR(77, testRepo2, "Mine only")})

	testkit.AssertEqual(t, "tab 0 cached", list.HasCachedTab(), true)

	if !list.ClickTabAt(tabColumn(t, list, "Mine")) {
		t.Fatal("ClickTabAt did not switch to the Mine tab")
	}
	testkit.AssertEqual(t, "tab index", list.GetTabIndex(), 1)
	testkit.AssertEqual(t, "active tab name", list.ActiveTab().Name, "Mine")

	out := stripANSI(list.View())
	if !strings.Contains(out, "Mine only") {
		t.Errorf("View() = %q, want the clicked tab's rows", out)
	}
	if strings.Contains(out, "#101") {
		t.Errorf("View() = %q, want the previous tab's rows gone", out)
	}
}

func TestPullRequestList_ClickTabAt_SameTabReportsNoChange(t *testing.T) {
	t.Parallel()
	list := makeFocusedPRList(newPR(101, testRepo, testPRTitle))
	if list.ClickTabAt(5) {
		t.Error("clicking the active tab should report no change")
	}
}

func TestPullRequestList_NextPrevTab_LoadFromCache(t *testing.T) {
	t.Parallel()
	list := makeFocusedPRList(newPR(101, testRepo, testPRTitle))
	list.SetPullRequestsForTab(2, []bitbucket.PullRequest{newPR(9, testRepo2, "Reviewing")})

	list.PrevTab()
	testkit.AssertEqual(t, "tab index", list.GetTabIndex(), 2)
	if !strings.Contains(stripANSI(list.View()), "Reviewing") {
		t.Error("PrevTab did not load the cached rows")
	}

	list.NextTab()
	testkit.AssertEqual(t, "tab index after NextTab", list.GetTabIndex(), 0)
	testkit.AssertEqual(t, "rows restored", len(list.CurrentPullRequests()), 1)
}

func TestPullRequestList_SetTabIndex_RejectsOutOfRange(t *testing.T) {
	t.Parallel()
	list := makeFocusedPRList(newPR(101, testRepo, testPRTitle))
	list.SetTabIndex(99)
	testkit.AssertEqual(t, "tab index", list.GetTabIndex(), 0)
}

func TestPullRequestList_SetRepoFilter_NarrowsRows(t *testing.T) {
	t.Parallel()
	list := makeFocusedPRList(
		newPR(101, testRepo, testPRTitle),
		newPR(40, testRepo2, "Web change"),
	)

	list.SetRepoFilter(testRepo2)
	testkit.AssertEqual(t, "repo filter", list.RepoFilter(), testRepo2)
	testkit.AssertEqual(t, "item count", list.ItemCount(), 1)

	out := stripANSI(list.View())
	if !strings.Contains(out, "Web change") || strings.Contains(out, "#101") {
		t.Errorf("View() = %q, want only the %s rows", out, testRepo2)
	}

	list.SetRepoFilter("")
	testkit.AssertEqual(t, "item count after clearing", list.ItemCount(), 2)
}

func TestPullRequestList_SetFilter_MatchesEveryIndexedField(t *testing.T) {
	t.Parallel()
	queries := []string{testRepo2, "#40", "Web change", "grace", "feature/x"}
	for _, query := range queries {
		t.Run(query, func(t *testing.T) {
			t.Parallel()
			list := makeFocusedPRList(
				newPR(101, testRepo, testPRTitle),
				newPR(40, testRepo2, "Web change"),
			)
			list.SetFilter(query)
			if list.ItemCount() == 0 {
				t.Errorf("SetFilter(%q) matched nothing", query)
			}
		})
	}
}

func TestPullRequestList_ClearFilter_KeepsCursor(t *testing.T) {
	t.Parallel()
	list := makeFocusedPRList(
		newPR(101, testRepo, testPRTitle),
		newPR(40, testRepo2, "Web change"),
	)
	list.SetFilter("web")
	testkit.AssertEqual(t, "filtered count", list.ItemCount(), 1)

	list.ClearFilter()
	testkit.AssertEqual(t, "item count", list.ItemCount(), 2)
	testkit.AssertEqual(t, "cursor follows the pull request", list.SelectedPullRequest().ID, 40)
}

func TestPullRequestList_SelectedPullRequest_NilWhenEmpty(t *testing.T) {
	t.Parallel()
	list := makeFocusedPRList()
	if list.SelectedPullRequest() != nil {
		t.Error("SelectedPullRequest() should be nil on an empty list")
	}
	testkit.AssertEqual(t, "content height floor", list.ContentHeight(), 7)
}

func TestPullRequestList_View_DraftAndApprovalGlyphs(t *testing.T) {
	t.Parallel()
	draft := newPR(102, testRepo, "Work in progress")
	draft.Draft = true
	reviewer := bitbucket.Account{UUID: "{reviewer}", DisplayName: testReviewer}
	draft.Reviewers = []bitbucket.Account{reviewer}
	draft.Participants = []bitbucket.Participant{{
		User:  reviewer,
		Role:  bitbucket.RoleReviewer,
		State: bitbucket.ParticipantChangesRequested,
	}}

	list := makeFocusedPRList(draft)
	list.SetSize(120, 24)

	out := stripANSI(list.View())
	for _, want := range []string{"[draft]", "✗1"} {
		if !strings.Contains(out, want) {
			t.Errorf("View() = %q, want it to contain %q", out, want)
		}
	}
}

func TestPullRequestList_View_UnknownReviewDataShowsQuestionMark(t *testing.T) {
	t.Parallel()
	list := makeFocusedPRList(newPR(101, testRepo, testPRTitle))
	list.SetSize(120, 24)

	if !strings.Contains(stripANSI(list.View()), "?") {
		t.Error("a pull request without review data should render the ? approvals cell")
	}
}

func TestPullRequestList_BuildTitle_SlidingWindowKeepsActiveTab(t *testing.T) {
	t.Parallel()
	list := NewPullRequestList()
	list.SetTabs([]config.PullRequestTabConfig{
		{Name: "Open"}, {Name: "Mine"}, {Name: "Reviewing"}, {Name: "Merged"}, {Name: "Declined"},
	})
	list.SetTabIndex(4)

	title := stripANSI(list.buildTitle(24))
	if !strings.Contains(title, "Declined") {
		t.Errorf("title = %q, want the active tab", title)
	}
	if strings.Contains(title, "Open") {
		t.Errorf("title = %q, want the leading tabs dropped", title)
	}
	if got := len([]rune(title)); got > 24 {
		t.Errorf("title width = %d, want at most 24", got)
	}
}

func TestPullRequestList_BuildTitle_TruncatesAnOversizedActiveLabel(t *testing.T) {
	t.Parallel()
	list := NewPullRequestList()
	list.SetTabs([]config.PullRequestTabConfig{{Name: strings.Repeat("long", 10)}})

	title := stripANSI(list.buildTitle(20))
	if got := len([]rune(title)); got > 20 {
		t.Errorf("title width = %d, want at most 20", got)
	}
	if !strings.HasSuffix(title, "…") {
		t.Errorf("title = %q, want a truncation ellipsis", title)
	}
}

// tabColumn is the display column of a tab label in the panel's title bar, so a
// click test can address what was actually drawn rather than a hardcoded index.
func tabColumn(t *testing.T, list *PullRequestList, label string) int {
	t.Helper()
	titleBar, _, _ := strings.Cut(stripANSI(list.View()), "\n")
	idx := strings.Index(titleBar, label)
	if idx < 0 {
		t.Fatalf("title bar %q does not contain %q", titleBar, label)
	}
	return lipgloss.Width(titleBar[:idx])
}

// assertTabColumnsSelect checks every column of a drawn label maps back to its
// own tab. restore is re-applied before each click so the drawn window (which
// depends on the active tab) stays the one tabColumn measured.
func assertTabColumnsSelect(t *testing.T, list *PullRequestList, label string, want, restore int) {
	t.Helper()
	list.SetTabIndex(restore)
	first := tabColumn(t, list, label)
	for x := first; x < first+lipgloss.Width(label); x++ {
		list.SetTabIndex(restore)
		list.ClickTabAt(x)
		if got := list.GetTabIndex(); got != want {
			t.Errorf("ClickTabAt(%d) on %q selected tab %d, want %d", x, label, got, want)
		}
	}
}

func TestPullRequestList_ClickTabAt_UsesTheDrawnWindow(t *testing.T) {
	t.Parallel()
	list := makeFocusedPRList(newPR(101, testRepo, testPRTitle))
	// Too narrow for "[2] Open - Mine - Reviewing", so the title slides to the
	// window around the active tab and no longer starts at tab 0.
	list.SetSize(28, 10)
	list.SetTabIndex(2)

	titleBar, _, _ := strings.Cut(stripANSI(list.View()), "\n")
	if !strings.Contains(titleBar, "Mine") || !strings.Contains(titleBar, "Reviewing") {
		t.Fatalf("title bar = %q, want the sliding window around the active tab", titleBar)
	}
	if strings.Contains(titleBar, "Open") {
		t.Fatalf("title bar = %q, want the first tab scrolled out of the window", titleBar)
	}

	assertTabColumnsSelect(t, list, "Mine", 1, 2)
	assertTabColumnsSelect(t, list, "Reviewing", 2, 2)
}

func TestPullRequestList_ClickTabAt_MeasuresDisplayWidthNotBytes(t *testing.T) {
	t.Parallel()
	list := makeFocusedPRList(newPR(101, testRepo, testPRTitle))
	list.SetTabs([]config.PullRequestTabConfig{
		{Name: "Ünïcödé", Filter: "all"},
		{Name: "Mine", Filter: "mine"},
	})

	assertTabColumnsSelect(t, list, "Mine", 1, 0)
	assertTabColumnsSelect(t, list, "Ünïcödé", 0, 1)
}
