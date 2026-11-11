// Command lazybitbucket is a keyboard-driven terminal UI for Bitbucket Cloud
// pull requests.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime/debug"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/config"
	"github.com/lsst25/lazybitbucket/pkg/tui"
	"github.com/lsst25/lazybitbucket/pkg/tui/theme"
	"github.com/lsst25/lazybitbucket/pkg/viewed"
)

// version is set at build time via ldflags; main resolves what is shown.
var version = devVersion

func main() {
	info, _ := debug.ReadBuildInfo()
	version = resolveVersion(version, info)
	os.Exit(dispatch(os.Args[1:], os.Stdout, os.Stderr))
}

// dispatch runs the subcommand named by the first argument, or the TUI when
// there is none, and returns the process exit code.
func dispatch(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "auth":
			if err := runAuth(args[1:], os.Stdin); err != nil {
				fmt.Fprintf(stderr, "Error: %v\n", err)
				return 1
			}
			return 0
		case "logout":
			if err := config.ClearCredentials(); err != nil {
				fmt.Fprintf(stderr, "Error: %v\n", err)
				return 1
			}
			fmt.Fprintln(stdout, "Credentials cleared.")
			return 0
		case "--version", "version":
			fmt.Fprintf(stdout, "lazybitbucket %s\n", version)
			return 0
		}
	}

	if err := run(args, stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0 // usage was already printed by the flag set
		}
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

// run starts the terminal UI. Flag usage and errors are written to stderr.
func run(args []string, stderr io.Writer) error {
	opts, err := parseFlags(args, stderr)
	if err != nil {
		return err
	}

	if opts.debugFile != "" {
		f, err := os.OpenFile(opts.debugFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return fmt.Errorf("opening debug log: %w", err)
		}
		defer func() { _ = f.Close() }()
		slog.SetDefault(slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug})))
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	opts.applyTo(cfg)

	// The theme must exist before the app is built: views capture theme.Default.
	if err := theme.Init(theme.Options{
		Preset:      cfg.GUI.Theme,
		Colors:      cfg.GUI.ThemeColors,
		ColorsDark:  cfg.GUI.ThemeDark,
		ColorsLight: cfg.GUI.ThemeLight,
	}); err != nil {
		return fmt.Errorf("theme: %w", err)
	}

	client, authMethod, cleanup, err := startClient(cfg, opts)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}

	tui.Version = version
	app := tui.NewAppWithAuth(cfg, client, authMethod)
	defer app.Shutdown()
	if authMethod != tui.AuthDemo {
		// The demo's pull requests are not real: their marks stay in memory
		// rather than landing in the user's file.
		app.SetViewedStore(viewed.Open(config.ViewedPath()))
	}

	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("running the terminal UI: %w", err)
	}
	return nil
}

// startClient builds the API client the app talks to: the demo server under
// --demo, otherwise the credentials resolved from auth.json, the environment or
// the wizard. The returned cleanup is nil when there is nothing to close.
func startClient(cfg *config.Config, opts *cliOptions) (bitbucket.ClientInterface, tui.AuthMethod, func(), error) {
	if opts.demo {
		client, authMethod, cleanup, err := startDemo(cfg)
		if err != nil {
			return nil, "", nil, fmt.Errorf("demo: %w", err)
		}
		return client, authMethod, cleanup, nil
	}

	client, authMethod, err := resolveClient(cfg)
	if err != nil {
		return nil, "", nil, err
	}

	cleanup := func() {}
	if opts.logFile != "" {
		f, err := os.OpenFile(opts.logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return nil, "", nil, fmt.Errorf("opening log file: %w", err)
		}
		client.SetLogger(f)
		cleanup = func() { _ = f.Close() }
	}
	return client, authMethod, cleanup, nil
}
