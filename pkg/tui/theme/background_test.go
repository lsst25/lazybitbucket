package theme

import (
	"errors"
	"os/exec"
	"testing"
)

func TestIsMultiplexed(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"tmux":           true,
		"tmux-256color":  true,
		"screen":         true,
		"screen.xterm":   true,
		"xterm-256color": false,
		"xterm-ghostty":  false,
		"alacritty":      false,
		"":               false,
	}
	for term, want := range cases {
		if got := isMultiplexed(term); got != want {
			t.Errorf("isMultiplexed(%q) = %v, want %v", term, got, want)
		}
	}
}

// exitWith runs a shell that exits with code, so the test has a real
// *exec.ExitError to hand to appearanceIsDark.
func exitWith(t *testing.T, code string) error {
	t.Helper()
	err := exec.CommandContext(t.Context(), "sh", "-c", "exit "+code).Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("sh -c 'exit %s' did not produce an *exec.ExitError: %v", code, err)
	}
	return err
}

func TestAppearanceIsDark(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		out      string
		err      error
		wantDark bool
		wantOK   bool
	}{
		{name: "dark appearance", out: "Dark\n", wantDark: true, wantOK: true},
		{name: "key present but not Dark", out: "Light\n", wantDark: false, wantOK: true},
		{name: "key missing is light", err: exitWith(t, "1"), wantDark: false, wantOK: true},
		{name: "other exit code is unusable", err: exitWith(t, "2"), wantDark: false, wantOK: false},
		{name: "defaults not runnable is unusable", err: exec.ErrNotFound, wantDark: false, wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dark, ok := appearanceIsDark([]byte(tc.out), tc.err)
			if dark != tc.wantDark || ok != tc.wantOK {
				t.Errorf("appearanceIsDark(%q, %v) = (%v, %v), want (%v, %v)",
					tc.out, tc.err, dark, ok, tc.wantDark, tc.wantOK)
			}
		})
	}
}
