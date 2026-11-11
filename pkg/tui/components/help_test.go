package components

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

func TestHelpBar_ViewRendersItems(t *testing.T) {
	t.Parallel()
	bar := NewHelpBar([]HelpItem{
		{Key: "q", Description: "quit"},
		{Key: "?", Description: "help"},
	})
	bar.SetWidth(120)
	out := bar.View()
	plain := stripANSI(out)
	if !strings.Contains(plain, "quit: q") {
		t.Errorf("expected 'quit: q' in output, got %q", plain)
	}
	if !strings.Contains(plain, "help: ?") {
		t.Errorf("expected 'help: ?' in output, got %q", plain)
	}
}

func TestHelpBar_SetItemsReplacesItems(t *testing.T) {
	t.Parallel()
	bar := NewHelpBar([]HelpItem{{Key: "q", Description: "quit"}})
	bar.SetWidth(120)
	bar.SetItems([]HelpItem{{Key: "r", Description: "refresh"}})
	out := stripANSI(bar.View())
	if strings.Contains(out, "quit") {
		t.Error("old item should be replaced")
	}
	if !strings.Contains(out, "refresh: r") {
		t.Errorf("expected new item in output, got %q", out)
	}
}

func TestHelpBar_SetStatusMsgAppearsInView(t *testing.T) {
	t.Parallel()
	bar := NewHelpBar(nil)
	bar.SetWidth(120)
	bar.SetStatusMsg("saved!")
	out := stripANSI(bar.View())
	if !strings.Contains(out, "saved!") {
		t.Errorf("expected status msg in output, got %q", out)
	}
}

func TestHelpBar_SetWidthUpdatesWidth(t *testing.T) {
	t.Parallel()
	bar := NewHelpBar(nil)
	bar.SetWidth(80)
	testkit.AssertEqual(t, "width", bar.width, 80)
}

func TestHelpBar_UpdateHandlesWindowSize(t *testing.T) {
	t.Parallel()
	bar := NewHelpBar(nil)
	updated, _ := bar.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	testkit.AssertEqual(t, "width after window size", updated.width, 100)
}

func TestHelpBar_TruncatesWhenTooNarrow(t *testing.T) {
	t.Parallel()
	items := make([]HelpItem, 10)
	for i := range items {
		items[i] = HelpItem{Key: "x", Description: "longdescription"}
	}
	bar := NewHelpBar(items)
	bar.SetWidth(20)
	out := bar.View()
	if !strings.Contains(stripANSI(out), "...") {
		t.Errorf("expected truncation marker '...' in narrow bar, got %q", stripANSI(out))
	}
}

func TestHelpBar_InitReturnsNil(t *testing.T) {
	t.Parallel()
	bar := NewHelpBar(nil)
	cmd := bar.Init()
	testkit.AssertEqual(t, "Init returns nil", cmd == nil, true)
}

func TestHelpBar_VersionSitsAtTheRightEnd(t *testing.T) {
	t.Parallel()
	bar := NewHelpBar([]HelpItem{{Key: "q", Description: "quit"}})
	bar.SetWidth(80)
	bar.SetVersion("0.1.0")

	out := stripANSI(bar.View())

	if !strings.HasSuffix(out, "v0.1.0") {
		t.Errorf("view = %q, want it to end with the version", out)
	}
	testkit.AssertEqual(t, "width", len([]rune(out)), 80-1)
	if !strings.Contains(out, "quit: q") {
		t.Errorf("view = %q, the hints must stay", out)
	}
}

func TestHelpBar_VersionKeepsAWordLikeDev(t *testing.T) {
	t.Parallel()
	bar := NewHelpBar(nil)
	bar.SetWidth(80)
	bar.SetVersion("dev-abc1234")

	if out := stripANSI(bar.View()); !strings.HasSuffix(out, "dev-abc1234") {
		t.Errorf("view = %q, want the dev version as it is", out)
	}
}

func TestHelpBar_VersionIsNotMistakenForACommitHash(t *testing.T) {
	t.Parallel()
	bar := NewHelpBar(nil)
	bar.SetWidth(80)
	bar.SetVersion("07388ec-dirty")

	// A hash that happens to start with a digit is not a version number.
	if out := stripANSI(bar.View()); strings.Contains(out, "v07388ec") {
		t.Errorf("view = %q, a commit hash must not get a v prefix", out)
	}
}

func TestHelpBar_VersionGivesWayOnANarrowTerminal(t *testing.T) {
	t.Parallel()
	bar := NewHelpBar([]HelpItem{{Key: "q", Description: "quit"}})
	bar.SetWidth(30)
	bar.SetVersion("0.1.0")

	if out := stripANSI(bar.View()); strings.Contains(out, "0.1.0") {
		t.Errorf("view = %q, a narrow bar keeps its hints and drops the version", out)
	}
}

func TestHelpBar_VersionDoesNotCrowdOutTheHints(t *testing.T) {
	t.Parallel()
	items := make([]HelpItem, 0, 20)
	for range 20 {
		items = append(items, HelpItem{Key: "k", Description: "some action"})
	}
	bar := NewHelpBar(items)
	bar.SetWidth(60)
	bar.SetVersion("0.1.0")

	out := stripANSI(bar.View())

	if !strings.HasSuffix(out, "v0.1.0") {
		t.Errorf("view = %q, want the version kept at the right end", out)
	}
	if got := len([]rune(out)); got > 60 {
		t.Errorf("view is %d columns wide, want at most 60", got)
	}
}
