package theme

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// appearanceTimeout bounds the macOS appearance lookup. It runs once, before
// the TUI starts, and a stuck `defaults` must not hold the start-up hostage.
const appearanceTimeout = 2 * time.Second

// HasDarkBackground reports whether the terminal background is dark. It is
// what "auto" and the theme families consult, and the Markdown and diff
// renderers use it too, so every light/dark decision lands the same way.
//
// lipgloss asks the terminal for its background colour (OSC 11). Under tmux
// and screen that question is never sent — a multiplexer may be attached to
// several terminals at once, so termenv refuses to ask and reports "dark"
// whatever the terminal shows. On macOS the system appearance is a faithful
// stand-in there: a terminal that follows it (Ghostty, iTerm2, Terminal.app)
// is dark exactly when the appearance is. COLORFGBG, the environment variable
// termenv reads when it cannot ask, keeps precedence over the appearance.
// Everywhere else the answer is lipgloss's.
func HasDarkBackground() bool {
	if isMultiplexed(os.Getenv("TERM")) && os.Getenv("COLORFGBG") == "" && runtime.GOOS == "darwin" {
		if dark, ok := macOSDarkAppearance(); ok {
			return dark
		}
	}
	return lipgloss.HasDarkBackground()
}

// isMultiplexed reports whether TERM names a terminal multiplexer, the case in
// which termenv will not query the terminal background.
func isMultiplexed(term string) bool {
	return strings.HasPrefix(term, "tmux") || strings.HasPrefix(term, "screen")
}

// macOSDarkAppearance asks the macOS user defaults for the interface style.
// ok is false when the answer is unusable and the caller should fall back.
func macOSDarkAppearance() (dark, ok bool) {
	ctx, cancel := context.WithTimeout(context.Background(), appearanceTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "defaults", "read", "-g", "AppleInterfaceStyle").Output()
	return appearanceIsDark(out, err)
}

// appearanceIsDark interprets `defaults read -g AppleInterfaceStyle`. The key
// holds "Dark" when the appearance is dark and does not exist at all when it
// is light — the command then exits 1 — so a clean exit-1 is a positive
// "light" answer, not a failure. Anything else (defaults missing, a timeout, a
// different exit code) is reported as unusable.
func appearanceIsDark(out []byte, err error) (dark, ok bool) {
	if err == nil {
		return strings.Contains(string(out), "Dark"), true
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, true
	}
	return false, false
}
