package views

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

const testUser = "user@example.com"

func makeStatusPanel() *StatusPanel {
	panel := NewStatusPanel(testWorkspace, testUser, testWorkspace)
	panel.SetSize(80, 10)
	return panel
}

func TestStatusPanel_NewStatusPanel_StartsCleanAndOnline(t *testing.T) {
	t.Parallel()
	panel := makeStatusPanel()

	testkit.AssertEqual(t, "error", panel.ErrorMessage(), "")
	out := stripANSI(panel.View())
	if !strings.Contains(out, "✓ "+testUser+" → "+testWorkspace) {
		t.Errorf("View() = %q, want the online summary line", out)
	}
}

func TestStatusPanel_SetVersion_SitsInTheBottomBorder(t *testing.T) {
	t.Parallel()
	panel := makeStatusPanel()
	bare := strings.Split(stripANSI(panel.View()), "\n")
	panel.SetVersion("v0.1.0")

	lines := strings.Split(stripANSI(panel.View()), "\n")

	testkit.AssertEqual(t, "the panel keeps its height", len(lines), len(bare))
	last := lines[len(lines)-1]
	if !strings.HasSuffix(last, "v0.1.0─╯") {
		t.Errorf("bottom border = %q, want the version at its right end", last)
	}
	testkit.AssertEqual(t, "border width", lipgloss.Width(last), 80)
}

func TestStatusPanel_SetVersion_IsDroppedWhenTheBorderCannotHoldIt(t *testing.T) {
	t.Parallel()
	panel := makeStatusPanel()
	panel.SetVersion("v0.1.0-12-gabc1234-dirty")
	panel.SetSize(20, 10)

	lines := strings.Split(stripANSI(panel.View()), "\n")

	last := lines[len(lines)-1]
	if strings.Contains(last, "v0.1") {
		t.Errorf("bottom border = %q, a version that does not fit is not cut", last)
	}
	testkit.AssertEqual(t, "border width", lipgloss.Width(last), 20)
}

func TestStatusPanel_SetScope_UpdatesTheSummaryLine(t *testing.T) {
	t.Parallel()
	panel := makeStatusPanel()
	panel.SetScope(testRepo)

	if !strings.Contains(stripANSI(panel.View()), "→ "+testRepo) {
		t.Errorf("View() = %q, want the new scope", stripANSI(panel.View()))
	}
}

func TestStatusPanel_SetOnline_TogglesTheIndicator(t *testing.T) {
	t.Parallel()
	panel := makeStatusPanel()

	panel.SetOnline(false)
	if !strings.Contains(stripANSI(panel.View()), "✗") {
		t.Error("an offline panel should show ✗")
	}

	panel.SetOnline(true)
	if !strings.Contains(stripANSI(panel.View()), "✓") {
		t.Error("an online panel should show ✓")
	}
}

func TestStatusPanel_SetReadOnly_MarksTheIdentityLine(t *testing.T) {
	t.Parallel()
	panel := makeStatusPanel()

	if strings.Contains(stripANSI(panel.View()), "read-only") {
		t.Error("a writable session says nothing about read-only mode")
	}

	panel.SetReadOnly(true)
	out := stripANSI(panel.View())
	if !strings.Contains(out, testUser+" → "+testWorkspace+" · read-only") {
		t.Errorf("View() = %q, want the marker on the identity line", out)
	}
	// It shares the first line rather than adding one, so the panel keeps its
	// height and the app's row reservation stays right.
	testkit.AssertEqual(t, "lines", strings.Count(out, "read-only"), 1)
}

func TestStatusPanel_SetReadOnly_KeepsTheMarkerWholeOnANarrowPanel(t *testing.T) {
	t.Parallel()
	// An 80-column terminal gives the side panel 28 columns, which is where the
	// marker used to be truncated away because it shared the account's budget.
	cases := []struct {
		name  string
		width int
		want  string
	}{
		{"side panel of an 80 column terminal", 28, " · read-only"},
		{"too narrow for the long marker", 16, " ·ro"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			panel := NewStatusPanel(testRepo, testUser, testWorkspace)
			panel.SetSize(tc.width, 6)
			panel.SetReadOnly(true)

			out := stripANSI(panel.View())
			if !strings.Contains(out, tc.want) {
				t.Errorf("View() = %q, want the marker %q intact", out, tc.want)
			}
			if strings.Contains(out, "read-…") {
				t.Errorf("View() = %q, want no sliced marker", out)
			}
			for _, line := range strings.Split(panel.View(), "\n") {
				if got := lipgloss.Width(line); got > tc.width {
					t.Errorf("line %q is %d columns wide, want at most %d", stripANSI(line), got, tc.width)
				}
			}
		})
	}
}

func TestStatusPanel_SetError_ShowsAndOutranksTheWarning(t *testing.T) {
	t.Parallel()
	panel := makeStatusPanel()
	panel.SetWarning("showing the first 25 repositories")
	panel.SetError("connection failed")

	testkit.AssertEqual(t, "ErrorMessage", panel.ErrorMessage(), "connection failed")
	out := stripANSI(panel.View())
	if !strings.Contains(out, "connection failed") {
		t.Errorf("View() = %q, want the error line", out)
	}
	if strings.Contains(out, "first 25") {
		t.Errorf("View() = %q, want the warning suppressed while there is an error", out)
	}

	panel.SetError("")
	if !strings.Contains(stripANSI(panel.View()), "first 25") {
		t.Error("clearing the error should reveal the warning")
	}
}

func TestStatusPanel_SetRateLimit(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		remaining int
		wantLine  bool
	}{
		{"unknown", -1, false},
		{"low", 150, true},
		{"zero", 0, true},
		{"comfortable", 500, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			panel := makeStatusPanel()
			panel.SetRateLimit(tc.remaining)

			out := stripANSI(panel.View())
			got := strings.Contains(out, "rate limit:")
			if got != tc.wantLine {
				t.Errorf("View() = %q, rate limit line present = %v, want %v", out, got, tc.wantLine)
			}
		})
	}
}

func TestStatusPanel_SetRateLimit_RendersTheRemainingCount(t *testing.T) {
	t.Parallel()
	panel := makeStatusPanel()
	panel.SetRateLimit(150)

	if !strings.Contains(stripANSI(panel.View()), "rate limit: 150 left") {
		t.Errorf("View() = %q, want the remaining count", stripANSI(panel.View()))
	}
}

func TestStatusPanel_View_CollapsedBarWhenHeightOne(t *testing.T) {
	t.Parallel()
	panel := makeStatusPanel()
	panel.SetSize(80, 1)

	out := stripANSI(panel.View())
	if !strings.Contains(out, "Status") {
		t.Errorf("collapsed bar = %q, want the Status label", out)
	}
	if strings.Contains(out, testUser) {
		t.Errorf("collapsed bar = %q, want no summary line", out)
	}
}

func TestStatusPanel_View_FallsBackToTheWorkspaceLabel(t *testing.T) {
	t.Parallel()
	panel := NewStatusPanel(testRepo, "", testWorkspace)
	panel.SetSize(80, 10)

	if !strings.Contains(stripANSI(panel.View()), testWorkspace+" → "+testRepo) {
		t.Errorf("View() = %q, want the workspace as the user label", stripANSI(panel.View()))
	}
}

func TestStatusPanel_View_LongUserIsTruncated(t *testing.T) {
	t.Parallel()
	longUser := strings.Repeat("a", 100) + "@example.com"
	panel := NewStatusPanel(testRepo, longUser, testWorkspace)
	panel.SetSize(40, 10)

	out := stripANSI(panel.View())
	if strings.Contains(out, longUser) {
		t.Error("a long user label should be truncated")
	}
	if !strings.Contains(out, "...") {
		t.Errorf("View() = %q, want a truncation ellipsis", out)
	}
}

func TestStatusPanel_SetFocused_AndUpdateAreInert(t *testing.T) {
	t.Parallel()
	panel := makeStatusPanel()
	panel.SetFocused(true)
	if panel.View() == "" {
		t.Error("a focused panel should still render")
	}
	if cmd := panel.Init(); cmd != nil {
		t.Error("Init() should return nil")
	}

	got, cmd := panel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if got != panel {
		t.Error("Update() should return the same panel")
	}
	if cmd != nil {
		t.Error("Update() should return no command")
	}
}

func TestStatusPanel_View_ClampsEveryLineToThePanelWidth(t *testing.T) {
	t.Parallel()
	const width = 35
	cases := []struct {
		name  string
		scope string
		user  string
	}{
		{"long repository slug", "terraform-infrastructure-modules", "demo@example.com"},
		{"wide runes in the scope", strings.Repeat("測", 24), strings.Repeat("a", 40) + "@example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			panel := NewStatusPanel(tc.scope, tc.user, testWorkspace)
			panel.SetSize(width, 10)

			for _, line := range strings.Split(panel.View(), "\n") {
				if got := lipgloss.Width(line); got > width {
					t.Errorf("line %q is %d columns wide, want at most %d", stripANSI(line), got, width)
				}
			}
		})
	}
}

func TestStatusPanel_SetViewName_SuffixesTheTitle(t *testing.T) {
	t.Parallel()
	panel := NewStatusPanel(testRepo, testAuthor, testWorkspace)
	panel.SetSize(60, 5)

	if !strings.Contains(stripANSI(panel.View()), "[1] Status") {
		t.Error("View() should show the plain title by default")
	}
	if strings.Contains(stripANSI(panel.View()), "·") {
		t.Error("an unnamed view should add no suffix")
	}

	panel.SetViewName("Pipelines")
	if !strings.Contains(stripANSI(panel.View()), "[1] Status · Pipelines") {
		t.Errorf("View() = %q, want the view name in the title", stripANSI(panel.View()))
	}

	panel.SetSize(60, 1)
	if !strings.Contains(stripANSI(panel.View()), "[1] Status · Pipelines") {
		t.Error("the collapsed bar should carry the view name too")
	}
}
