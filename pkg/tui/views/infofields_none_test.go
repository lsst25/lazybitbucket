package views

import (
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/theme"
)

func TestNoneStyle_GrayForeground(t *testing.T) {
	t.Parallel()
	var wantGray lipgloss.TerminalColor = theme.ColorGray
	testkit.AssertEqual(t, "foreground", noneStyle().GetForeground(), wantGray)
}
