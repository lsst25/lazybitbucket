package views

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lsst25/lazybitbucket/pkg/tui/components"
	"github.com/lsst25/lazybitbucket/pkg/tui/theme"
)

// rateLimitWarnBelow is when the remaining Bitbucket request budget starts
// being worth showing. Bitbucket allows roughly 1000 requests per hour on the
// repository endpoints, so a couple of hundred left is the point at which a
// refresh loop is about to start failing.
const rateLimitWarnBelow = 200

// rateLimitUnknown is the SetRateLimit value meaning "no header seen".
const rateLimitUnknown = -1

const statusPanelTitle = "[1] Status"

// StatusPanel is the [1] panel: the connection indicator, the account, what the
// list is scoped to, and the most recent error, warning or rate-limit note.
type StatusPanel struct {
	scope     string
	user      string
	workspace string
	online    bool
	errText   string
	warnText  string
	rateLimit int
	width     int
	height    int
	focused   bool
	theme     *theme.Theme
}

// NewStatusPanel constructs an online panel. scope is what the pull request
// list is currently showing (a repository slug, or the workspace); user is the
// account label; workspace is the fallback label when the user is unknown.
func NewStatusPanel(scope, user, workspace string) *StatusPanel {
	return &StatusPanel{
		scope:     scope,
		user:      user,
		workspace: workspace,
		online:    true,
		rateLimit: rateLimitUnknown,
		theme:     theme.Default,
	}
}

// SetScope sets what the pull request list is scoped to.
func (s *StatusPanel) SetScope(scope string) { s.scope = scope }

// SetOnline toggles the connection indicator.
func (s *StatusPanel) SetOnline(online bool) { s.online = online }

// SetError sets the global error line; "" clears it.
func (s *StatusPanel) SetError(err string) { s.errText = err }

// ErrorMessage returns the current error line.
func (s *StatusPanel) ErrorMessage() string { return s.errText }

// SetWarning sets a non-error hint (repository discovery cap, refresh interval
// note). It is only shown while there is no error.
func (s *StatusPanel) SetWarning(warning string) { s.warnText = warning }

// SetRateLimit records the remaining request budget. A negative value means
// unknown and renders nothing; a value at or above rateLimitWarnBelow is
// comfortable and also renders nothing.
func (s *StatusPanel) SetRateLimit(remaining int) { s.rateLimit = remaining }

// SetSize records the panel geometry; it must precede View.
func (s *StatusPanel) SetSize(w, h int) { s.width = w; s.height = h }

// SetFocused toggles focus.
func (s *StatusPanel) SetFocused(focused bool) { s.focused = focused }

// Init satisfies the bubbletea component shape; the panel has no startup work.
func (s *StatusPanel) Init() tea.Cmd { return nil }

// Update is a no-op: the panel is driven entirely by its setters.
func (s *StatusPanel) Update(tea.Msg) (*StatusPanel, tea.Cmd) { return s, nil }

// View renders the panel, collapsing to a single title bar at height 1.
func (s *StatusPanel) View() string {
	if s.height <= 1 {
		return components.RenderCollapsedBar(statusPanelTitle, s.scope, s.width, s.focused)
	}

	_, innerHeight := components.PanelDimensions(s.width, s.height)
	contentW := max(s.width-2, 1)

	indicator := theme.StatusColor("done").Render("✓")
	if !s.online {
		indicator = theme.StatusColor("").Render("✗")
	}

	userLabel := s.user
	if userLabel == "" {
		userLabel = s.workspace
	}
	// The indicator plus its trailing space is 2 columns and " → " is 3; the
	// scope gets at most half the panel so a long repository slug cannot crowd
	// the account out, and both parts are measured in display columns rather
	// than bytes.
	scope := components.TruncateEnd(s.scope, max(contentW/2, 8))
	maxUser := contentW - (2 + 3 + lipgloss.Width(scope))
	userLabel = components.TruncateMiddle(userLabel, max(maxUser, 3))

	// Truncate the plain text, not the assembled line: the indicator carries
	// ANSI and TruncateEnd slices runes, which could cut an escape in half.
	head := components.TruncateEnd(userLabel+" → "+scope, max(contentW-2, 1))
	lines := []string{indicator + " " + head}

	switch {
	case s.errText != "":
		lines = append(lines, " "+theme.StatusColor("").Render(components.TruncateEnd(s.errText, contentW-1)))
	case s.warnText != "":
		lines = append(lines, " "+s.theme.Subtitle.Render(components.TruncateEnd(s.warnText, contentW-1)))
	}

	if s.rateLimit >= 0 && s.rateLimit < rateLimitWarnBelow {
		note := "rate limit: " + strconv.Itoa(s.rateLimit) + " left"
		lines = append(lines, " "+s.theme.WarningText.Render(components.TruncateEnd(note, contentW-1)))
	}

	return components.RenderPanel(statusPanelTitle, strings.Join(lines, "\n"), s.width, innerHeight, s.focused)
}
