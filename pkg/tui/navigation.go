package tui

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// platformCommand returns the OS-specific command name and args for an action.
func platformCommand(action, arg string) (name string, args []string) {
	switch action {
	case "open":
		switch runtime.GOOS {
		case "darwin":
			return "open", []string{arg}
		case "windows":
			return "rundll32", []string{"url.dll,FileProtocolHandler", arg}
		default:
			return "xdg-open", []string{arg}
		}
	case "copy":
		switch runtime.GOOS {
		case "darwin":
			return "pbcopy", nil
		case "windows":
			return "clip", nil
		default:
			return "xclip", []string{"-selection", "clipboard"}
		}
	}
	return "", nil
}

// externalRunner is the single seam every external process goes through. It
// lives on App rather than in a package-level variable so parallel tests can
// capture browser and clipboard dispatch without racing each other.
type externalRunner func(input string, waitForExit bool, name string, args ...string)

func execExternal(input string, waitForExit bool, name string, args ...string) {
	if name == "" {
		return
	}
	ctx := context.Background()
	if waitForExit {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	if waitForExit {
		_ = cmd.Run()
		return
	}
	_ = cmd.Start()
}

// runExternal dispatches through the app's seam, falling back to a real
// process for an App that was built without one.
func (a *App) runExternal(input string, waitForExit bool, name string, args ...string) {
	run := a.exec
	if run == nil {
		run = execExternal
	}
	run(input, waitForExit, name, args...)
}

func (a *App) copyToClipboard(text string) {
	name, args := platformCommand("copy", "")
	a.runExternal(text, true, name, args...)
}

func (a *App) openBrowser(url string) {
	if url == "" {
		return
	}
	name, args := platformCommand("open", url)
	a.runExternal("", false, name, args...)
}
