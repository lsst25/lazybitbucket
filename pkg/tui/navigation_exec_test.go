package tui

import (
	"slices"
	"testing"
)

// execCapture records the single external command an App dispatched through
// its exec seam. The seam is per-App, so every subtest gets its own capture and
// the subtests can run in parallel.
type execCapture struct {
	input       string
	waitForExit bool
	name        string
	args        []string
}

func appWithExecCapture() (*App, *execCapture) {
	got := &execCapture{}
	app := newTestApp()
	app.exec = func(input string, waitForExit bool, name string, args ...string) {
		*got = execCapture{input: input, waitForExit: waitForExit, name: name, args: args}
	}
	return app, got
}

func TestExternalCommandDispatch(t *testing.T) {
	t.Parallel()

	t.Run("openBrowser launches without waiting", func(t *testing.T) {
		t.Parallel()
		app, got := appWithExecCapture()
		url := "https://bitbucket.org/" + testWorkspace + "/" + testRepo

		app.openBrowser(url)

		if got.name == "" {
			t.Fatal("openBrowser did not dispatch a command")
		}
		if got.waitForExit {
			t.Error("browser launch should be fire and forget")
		}
		if got.input != "" {
			t.Error("browser launch should not feed stdin")
		}
		if !slices.Contains(got.args, url) {
			t.Errorf("args %v should carry the url", got.args)
		}
	})

	t.Run("copyToClipboard feeds stdin and waits", func(t *testing.T) {
		t.Parallel()
		app, got := appWithExecCapture()

		app.copyToClipboard("payload")

		if got.name == "" {
			t.Fatal("copyToClipboard did not dispatch a command")
		}
		if !got.waitForExit {
			t.Error("clipboard write should wait for the pipe to drain")
		}
		if got.input != "payload" {
			t.Errorf("stdin = %q, want payload", got.input)
		}
	})
}
