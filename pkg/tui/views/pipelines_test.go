package views

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

func testPipelineEntries() []bitbucket.PipelineEntry {
	now := time.Now()
	return []bitbucket.PipelineEntry{
		{
			Repo: testRepo,
			Pipeline: bitbucket.Pipeline{
				UUID:              testPipelineUUID,
				BuildNumber:       415,
				DurationInSeconds: 268,
				CreatedOn:         now.Add(-2 * time.Hour),
				CompletedOn:       now.Add(-2*time.Hour + 268*time.Second),
				State: bitbucket.PipelineState{
					Name:   "COMPLETED",
					Result: &bitbucket.PipelineStateResult{Name: "SUCCESSFUL"},
				},
				Trigger: bitbucket.PipelineTrigger{Name: "PUSH"},
				Creator: &bitbucket.Account{DisplayName: testAuthor},
				Target: bitbucket.PipelineTarget{
					Type:        "pipeline_pullrequest_target",
					Source:      testRunBranch,
					Destination: "develop",
					PullRequest: &bitbucket.PipelinePullRequestRef{ID: 103},
				},
			},
		},
		{
			Repo: testRepo2,
			Pipeline: bitbucket.Pipeline{
				BuildNumber: 41,
				CreatedOn:   now.Add(-30 * time.Minute),
				State:       bitbucket.PipelineState{Name: "IN_PROGRESS"},
				Trigger:     bitbucket.PipelineTrigger{Name: "SCHEDULE"},
				Target: bitbucket.PipelineTarget{
					Type:    "pipeline_ref_target",
					RefName: "main",
				},
			},
		},
	}
}

func makeFocusedPipelineList() *PipelineList {
	list := NewPipelineList()
	list.ResolveNav = navResolverForList
	list.SetSize(80, 24)
	list.SetFocused(true)
	list.SetEntries(testPipelineEntries())
	return list
}

func TestPipelineList_View_ListsEveryRepository(t *testing.T) {
	t.Parallel()
	list := makeFocusedPipelineList()

	out := stripANSI(list.View())
	for _, want := range []string{
		"[2] Pipelines", "✔", "#415", testRepo, "PR #103", testRunBranch, "4m28s", "2h ago",
		"●", "#41", testRepo2, "main", "1 of 2",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("View() = %q, want it to contain %q", out, want)
		}
	}
	testkit.AssertEqual(t, "content height floor", NewPipelineList().ContentHeight(), 7)
}

func TestPipelineList_View_CollapsesToATitleBar(t *testing.T) {
	t.Parallel()
	list := makeFocusedPipelineList()
	list.SetSize(80, 1)

	out := stripANSI(list.View())
	if !strings.Contains(out, "[2] Pipelines") || !strings.Contains(out, "1 of 2") {
		t.Errorf("View() = %q, want the collapsed bar with its footer", out)
	}
}

func TestPipelineList_Row_DropsColumnsAsItNarrows(t *testing.T) {
	t.Parallel()
	entry := testPipelineEntries()[0]

	cases := []struct {
		name  string
		width int
		want  []string
		gone  []string
	}{
		{name: "everything fits", width: 80,
			want: []string{"#415", testRepo, "PR #103", "4m28s", "2h ago"}},
		{name: "the age goes first", width: 44,
			want: []string{"#415", testRepo, "4m28s"}, gone: []string{"ago"}},
		{name: "then the duration", width: 30,
			want: []string{"#415", testRepo}, gone: []string{"4m28s"}},
		{name: "then the repository", width: 20,
			want: []string{"#415", "PR #103"}, gone: []string{testRepo, "4m28s"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			row := pipelineRow(entry, tc.width, true)
			for _, want := range tc.want {
				if !strings.Contains(row, want) {
					t.Errorf("row at %d = %q, want it to contain %q", tc.width, row, want)
				}
			}
			for _, gone := range tc.gone {
				if strings.Contains(row, gone) {
					t.Errorf("row at %d = %q, want it to have dropped %q", tc.width, row, gone)
				}
			}
		})
	}
}

func TestPipelineList_Row_OmitsTheDurationOfARunningBuild(t *testing.T) {
	t.Parallel()
	entry := testPipelineEntries()[1]
	entry.Pipeline.DurationInSeconds = 90

	row := pipelineRow(entry, 80, true)
	if strings.Contains(row, "1m30s") {
		t.Errorf("row = %q, want no duration while the run has not completed", row)
	}
}

func TestPipelineGlyph_PerState(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		state bitbucket.PipelineState
		want  string
	}{
		{name: "successful", want: "✔", state: bitbucket.PipelineState{
			Name: "COMPLETED", Result: &bitbucket.PipelineStateResult{Name: "SUCCESSFUL"}}},
		{name: "failed", want: "✘", state: bitbucket.PipelineState{
			Name: "COMPLETED", Result: &bitbucket.PipelineStateResult{Name: "FAILED"}}},
		{name: "error", want: "✘", state: bitbucket.PipelineState{
			Name: "COMPLETED", Result: &bitbucket.PipelineStateResult{Name: "ERROR"}}},
		{name: "stopped", want: "■", state: bitbucket.PipelineState{
			Name: "COMPLETED", Result: &bitbucket.PipelineStateResult{Name: "STOPPED"}}},
		{name: "expired", want: "○", state: bitbucket.PipelineState{
			Name: "COMPLETED", Result: &bitbucket.PipelineStateResult{Name: "EXPIRED"}}},
		{name: "in progress", want: "●", state: bitbucket.PipelineState{
			Name: "IN_PROGRESS", Stage: &bitbucket.PipelineStateStage{Name: "RUNNING"}}},
		{name: "pending", want: "●", state: bitbucket.PipelineState{Name: "PENDING"}},
		{name: "unknown", want: "○", state: bitbucket.PipelineState{Name: "SOMETHING_NEW"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			glyph, _ := pipelineGlyph(tc.state)
			testkit.AssertEqual(t, "glyph", glyph, tc.want)
		})
	}
}

func TestPipelineList_Filter_MatchesTheRowAndTheRunsMetadata(t *testing.T) {
	t.Parallel()
	list := makeFocusedPipelineList()

	cases := []struct {
		name  string
		query string
		want  int
	}{
		{name: "repository slug", query: "web-app", want: 1},
		{name: "branch, untruncated", query: testRunBranch, want: 1},
		{name: "build number", query: "#415", want: 1},
		{name: "trigger", query: "schedule", want: 1},
		{name: "creator", query: "grace", want: 1},
		{name: "nothing", query: "zzz", want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			filtered := makeFocusedPipelineList()
			filtered.SetFilter(tc.query)
			testkit.AssertEqual(t, "visible rows", filtered.ItemCount(), tc.want)
		})
	}

	list.SetFilter("zzz")
	if !strings.Contains(stripANSI(list.View()), "No matching runs") {
		t.Error("View() should show the no-match placeholder")
	}
	list.ClearFilter()
	testkit.AssertEqual(t, "rows after clearing", list.ItemCount(), 2)
}

func TestPipelineList_SelectByRun(t *testing.T) {
	t.Parallel()
	list := makeFocusedPipelineList()

	testkit.AssertEqual(t, "found", list.SelectByRun(testRepo2, 41), true)
	entry := list.SelectedEntry()
	if entry == nil {
		t.Fatal("SelectedEntry() = nil")
	}
	testkit.AssertEqual(t, "selected repo", entry.Repo, testRepo2)
	testkit.AssertEqual(t, "same build number in another repository", list.SelectByRun(testRepo, 41), false)
	testkit.AssertEqual(t, "cursor unmoved by a miss", list.Cursor, 1)
}

func TestPipelineList_SetEntries_KeepsTheSelectedRun(t *testing.T) {
	t.Parallel()
	list := makeFocusedPipelineList()
	list.SelectByRun(testRepo2, 41)

	refetched := testPipelineEntries()
	refetched[1].Pipeline.State = bitbucket.PipelineState{
		Name: "COMPLETED", Result: &bitbucket.PipelineStateResult{Name: "SUCCESSFUL"},
	}
	list.SetEntries(refetched)

	entry := list.SelectedEntry()
	if entry == nil {
		t.Fatal("SelectedEntry() = nil")
	}
	testkit.AssertEqual(t, "still on the same run", entry.Pipeline.BuildNumber, 41)
	testkit.AssertEqual(t, "fresh payload", entry.Pipeline.State.Summary(), "SUCCESSFUL")

	list.SetEntries(testPipelineEntries()[:1])
	testkit.AssertEqual(t, "a vanished selection falls back to the top", list.Cursor, 0)
}

func TestPipelineList_SelectedEntry_ReturnsACopy(t *testing.T) {
	t.Parallel()
	list := makeFocusedPipelineList()

	entry := list.SelectedEntry()
	if entry == nil {
		t.Fatal("SelectedEntry() = nil")
	}
	entry.Repo = "mutated"
	testkit.AssertEqual(t, "stored entry untouched", list.Entries()[0].Repo, testRepo)
}

func TestPipelineList_Placeholders(t *testing.T) {
	t.Parallel()
	list := NewPipelineList()
	list.SetSize(80, 10)

	if !strings.Contains(stripANSI(list.View()), "No pipeline runs") {
		t.Error("View() should show the empty placeholder")
	}

	list.SetLoading()
	if !strings.Contains(stripANSI(list.View()), "Loading pipelines…") {
		t.Error("View() should show the loading placeholder")
	}

	list.SetError("list pipelines acme/api-gateway: status 500")
	out := stripANSI(list.View())
	if !strings.Contains(out, "status 500") {
		t.Errorf("View() = %q, want the error text", out)
	}

	list.SetEntries(testPipelineEntries())
	if strings.Contains(stripANSI(list.View()), "status 500") {
		t.Error("SetEntries should clear the error")
	}
}

func TestPipelineList_Update_EmitsTheSelectedRun(t *testing.T) {
	t.Parallel()
	list := makeFocusedPipelineList()

	_, cmd := list.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if cmd == nil {
		t.Fatal("moving the cursor should emit a selection")
	}
	msg, ok := cmd().(PipelineSelectedMsg)
	if !ok {
		t.Fatalf("cmd() = %T, want PipelineSelectedMsg", cmd())
	}
	if msg.Entry == nil {
		t.Fatal("PipelineSelectedMsg.Entry = nil")
	}
	testkit.AssertEqual(t, "second row", msg.Entry.Repo, testRepo2)

	list.SetFocused(false)
	if _, cmd := list.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}); cmd != nil {
		t.Error("an unfocused list should ignore keys")
	}
}

func TestPipelineList_ClickAt_SelectsTheRow(t *testing.T) {
	t.Parallel()
	list := makeFocusedPipelineList()

	list.ClickAt(2)
	testkit.AssertEqual(t, "cursor", list.Cursor, 1)
	testkit.AssertEqual(t, "a click past the last row is ignored", list.ClickAt(9), false)
}
