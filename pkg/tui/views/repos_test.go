package views

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

func testRepositories() []bitbucket.Repository {
	return []bitbucket.Repository{
		{
			Slug:       testRepo,
			Name:       "API Gateway",
			FullName:   testWorkspace + "/" + testRepo,
			MainBranch: &bitbucket.Branch{Name: "main"},
		},
		{
			Slug:     testRepo2,
			Name:     "Web App",
			FullName: testWorkspace + "/" + testRepo2,
		},
	}
}

func makeFocusedRepoList() *RepoList {
	list := NewRepoList()
	list.ResolveNav = navResolverForList
	list.SetSize(80, 24)
	list.SetFocused(true)
	list.SetRepos(testRepositories())
	return list
}

func TestRepoList_SetRepos_PrependsAllRow(t *testing.T) {
	t.Parallel()
	list := makeFocusedRepoList()

	testkit.AssertEqual(t, "item count", list.ItemCount(), 3)
	first := list.SelectedRepo()
	if first == nil {
		t.Fatal("SelectedRepo() = nil")
	}
	testkit.AssertEqual(t, "first row is All", first.All, true)
	testkit.AssertEqual(t, "All row has no slug", first.Slug, "")
	testkit.AssertEqual(t, "All row label", first.Label(), allReposLabel)
}

func TestRepoList_NewRepoList_HoldsOnlyTheAllRow(t *testing.T) {
	t.Parallel()
	list := NewRepoList()
	testkit.AssertEqual(t, "item count", list.ItemCount(), 1)
	testkit.AssertEqual(t, "content height floor", list.ContentHeight(), 5)
}

func TestRepoList_SelectedRepo_ReturnsACopy(t *testing.T) {
	t.Parallel()
	list := makeFocusedRepoList()
	list.Cursor = 1

	repo := list.SelectedRepo()
	if repo == nil {
		t.Fatal("SelectedRepo() = nil")
	}
	repo.Slug = "mutated"
	testkit.AssertEqual(t, "list unchanged", list.SelectedRepo().Slug, testRepo)
}

func TestRepoList_SetCounts_SumsIntoTheAllRow(t *testing.T) {
	t.Parallel()
	list := makeFocusedRepoList()
	list.SetCounts(map[string]int{testRepo: 3, testRepo2: 2})

	out := stripANSI(list.View())
	for _, want := range []string{"5 open", "3 open", "2 open"} {
		if !strings.Contains(out, want) {
			t.Errorf("View() = %q, want it to contain %q", out, want)
		}
	}
}

func TestRepoList_SetCounts_UnknownCountsRenderNoNumber(t *testing.T) {
	t.Parallel()
	list := makeFocusedRepoList()

	out := stripANSI(list.View())
	if strings.Contains(out, "open") {
		t.Errorf("View() = %q, want no counts before SetCounts", out)
	}

	list.SetCounts(map[string]int{testRepo: 4})
	out = stripANSI(list.View())
	if !strings.Contains(out, "4 open") {
		t.Errorf("View() = %q, want the known count", out)
	}
	if strings.Count(out, "open") != 2 { // api-gateway plus the All row
		t.Errorf("View() = %q, want only the known count and the All total", out)
	}
}

func TestRepoList_SetRepos_KeepsKnownCounts(t *testing.T) {
	t.Parallel()
	list := makeFocusedRepoList()
	list.SetCounts(map[string]int{testRepo: 7, testRepo2: 1})
	list.SetRepos(testRepositories())

	if !strings.Contains(stripANSI(list.View()), "7 open") {
		t.Error("a repository refresh should not blank the known counts")
	}
}

func TestRepoList_SetActiveSlug_MarksTheRow(t *testing.T) {
	t.Parallel()
	list := makeFocusedRepoList()
	list.SetActiveSlug(testRepo2)
	testkit.AssertEqual(t, "active slug", list.ActiveSlug(), testRepo2)

	out := stripANSI(list.View())
	if !strings.Contains(out, "*"+testRepo2) {
		t.Errorf("View() = %q, want the active marker on %s", out, testRepo2)
	}

	list.SetActiveSlug("")
	out = stripANSI(list.View())
	if !strings.Contains(out, "*"+allReposLabel) {
		t.Errorf("View() = %q, want the active marker on the All row", out)
	}
}

func TestRepoList_SetFilter_NarrowsRows(t *testing.T) {
	t.Parallel()
	list := makeFocusedRepoList()
	list.SetFilter("web")

	testkit.AssertEqual(t, "item count", list.ItemCount(), 1)
	out := stripANSI(list.View())
	if !strings.Contains(out, testRepo2) || strings.Contains(out, testRepo) {
		t.Errorf("View() = %q, want only %s", out, testRepo2)
	}

	list.SetFilter("")
	testkit.AssertEqual(t, "item count after clearing", list.ItemCount(), 3)
}

func TestRepoList_Update_FocusedNavEmitsHovered(t *testing.T) {
	t.Parallel()
	list := makeFocusedRepoList()

	_, cmd := list.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if cmd == nil {
		t.Fatal("Update() returned no command after a cursor move")
	}
	msg, ok := cmd().(RepoHoveredMsg)
	if !ok {
		t.Fatalf("msg = %T, want RepoHoveredMsg", cmd())
	}
	if msg.Repo == nil {
		t.Fatal("RepoHoveredMsg carried no repository")
	}
	testkit.AssertEqual(t, "hovered slug", msg.Repo.Slug, testRepo)
	testkit.AssertEqual(t, "hovered main branch", msg.Repo.MainBranch, "main")
}

func TestRepoList_Update_UnfocusedIsInert(t *testing.T) {
	t.Parallel()
	list := makeFocusedRepoList()
	list.SetFocused(false)

	_, cmd := list.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if cmd != nil {
		t.Error("unfocused Update() should return no command")
	}
}

func TestRepoList_View_CollapsedBarWhenHeightOne(t *testing.T) {
	t.Parallel()
	list := makeFocusedRepoList()
	list.SetSize(80, 1)

	out := stripANSI(list.View())
	if !strings.Contains(out, "[4] Repos") {
		t.Errorf("collapsed bar = %q, want the panel title", out)
	}
	if strings.Contains(out, testRepo2) {
		t.Errorf("collapsed bar = %q, want no rows", out)
	}
}

func TestRepoList_View_TruncatesLongSlugsByDisplayWidth(t *testing.T) {
	t.Parallel()
	list := NewRepoList()
	list.SetSize(24, 10)
	list.SetRepos([]bitbucket.Repository{{Slug: strings.Repeat("wide-ünïcode-", 5)}})

	for _, line := range strings.Split(stripANSI(list.View()), "\n") {
		if len([]rune(line)) > 24 {
			t.Errorf("line %q is %d columns wide, want at most 24", line, len([]rune(line)))
		}
	}
}

func TestRepoList_SetPanelNumber_ChangesTheTitle(t *testing.T) {
	t.Parallel()
	list := makeFocusedRepoList()

	if !strings.Contains(stripANSI(list.View()), "[4] Repos") {
		t.Error("View() should default to the pull request view's number")
	}

	list.SetPanelNumber(3)
	out := stripANSI(list.View())
	if !strings.Contains(out, "[3] Repos") {
		t.Errorf("View() = %q, want the pipelines view's number", out)
	}

	list.SetSize(40, 1)
	if !strings.Contains(stripANSI(list.View()), "[3] Repos") {
		t.Error("the collapsed bar should carry the number too")
	}
}
