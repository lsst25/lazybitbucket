package components

import (
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lsst25/lazybitbucket/pkg/tui/theme"
)

// HelpItem represents a single keybinding hint.
type HelpItem struct {
	Key         string
	Description string
}

// HelpBar displays context-sensitive keybinding hints at the bottom of the screen.
type HelpBar struct {
	items     []HelpItem
	statusMsg string // transient message shown left of hints (green)
	version   string // shown dim at the right end when there is room
	width     int
}

// versionGap is the least space kept between the hints and the version, and the
// least number of columns the hints keep before the version is dropped.
const (
	versionGap     = 2
	versionMinRoom = 40
	// helpOverhead is the leading space plus the " ..." of a truncated line.
	helpOverhead = 5
)

// NewHelpBar creates a help bar with the given items.
func NewHelpBar(items []HelpItem) HelpBar {
	return HelpBar{items: items}
}

// SetItems replaces the current help items.
func (h *HelpBar) SetItems(items []HelpItem) {
	h.items = items
}

// SetStatusMsg sets a transient message shown left of hints.
func (h *HelpBar) SetStatusMsg(msg string) {
	h.statusMsg = msg
}

// releaseVersion matches a version number such as "0.1.0" or "1.2.3-4-gabc1234".
// A bare commit hash does not match, even when it starts with a digit.
var releaseVersion = regexp.MustCompile(`^\d+\.\d+`)

// SetVersion sets the version shown at the right end of the bar. A version
// number is shown as "v1.2.3"; anything else (a "dev" build) is shown as it
// is. An empty string shows nothing.
func (h *HelpBar) SetVersion(version string) {
	h.version = version
	if releaseVersion.MatchString(version) {
		h.version = "v" + version
	}
}

// SetWidth updates the help bar width.
func (h *HelpBar) SetWidth(w int) {
	h.width = w
}

func (h HelpBar) Init() tea.Cmd {
	return nil
}

func (h HelpBar) Update(msg tea.Msg) (HelpBar, tea.Cmd) {
	if msg, ok := msg.(tea.WindowSizeMsg); ok {
		h.width = msg.Width
	}
	return h, nil
}

func (h HelpBar) View() string {
	blueStyle := lipgloss.NewStyle().Foreground(theme.ColorBlue)
	sep := blueStyle.Render(" | ")

	availW := h.width

	// The version takes the right end only when the hints keep enough room; on
	// a narrow terminal the hints matter more.
	versionW := lipgloss.Width(h.version)
	showVersion := h.version != "" && h.width >= versionMinRoom+versionW+versionGap
	if showVersion {
		// The leading space and a possible " ..." come on top of the hints, so
		// they are reserved too; otherwise a truncated line would squeeze the
		// version out.
		availW -= versionW + versionGap + helpOverhead
	}

	// Status message (accent colour) on the left.
	prefix := ""
	if h.statusMsg != "" {
		statusStyle := lipgloss.NewStyle().Foreground(theme.ColorAccent).Bold(true)
		prefix = " " + statusStyle.Render(h.statusMsg) + " "
		availW -= lipgloss.Width(prefix)
	}

	var parts []string
	totalWidth := 0
	truncated := false
	for _, item := range h.items {
		part := blueStyle.Render(item.Description + ": " + item.Key)
		partWidth := lipgloss.Width(part) + 3 // " | "
		if availW > 0 && totalWidth+partWidth > availW {
			truncated = true
			break
		}
		parts = append(parts, part)
		totalWidth += partWidth
	}

	result := strings.Join(parts, sep)
	if truncated {
		result += blueStyle.Render(" ...")
	}
	left := " " + result
	if prefix != "" {
		left = prefix + result
	}
	if !showVersion {
		return left
	}
	pad := h.width - lipgloss.Width(left) - versionW - 1
	if pad < 1 {
		return left
	}
	dim := lipgloss.NewStyle().Foreground(theme.ColorGray)
	return left + strings.Repeat(" ", pad) + dim.Render(h.version)
}
