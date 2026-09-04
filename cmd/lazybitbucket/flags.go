package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/lsst25/lazybitbucket/pkg/config"
)

// repeatedFlag collects a string flag that may be given more than once, e.g.
// `--repo api-gateway --repo web-app`.
type repeatedFlag []string

func (r *repeatedFlag) String() string { return strings.Join(*r, ",") }

func (r *repeatedFlag) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("value must not be empty")
	}
	*r = append(*r, value)
	return nil
}

// cliOptions are the flags accepted by the default (TUI) command.
type cliOptions struct {
	demo      bool
	logFile   string
	debugFile string
	workspace string
	repos     repeatedFlag
}

// parseFlags parses the arguments of the default command. Usage and flag errors
// are written to out.
func parseFlags(args []string, out io.Writer) (*cliOptions, error) {
	opts := &cliOptions{}
	fs := flag.NewFlagSet("lazybitbucket", flag.ContinueOnError)
	fs.SetOutput(out)
	fs.BoolVar(&opts.demo, "demo", false, "Run against the built-in demo dataset (no Bitbucket account needed)")
	fs.StringVar(&opts.logFile, "log", "", "Log API requests to `FILE`")
	fs.StringVar(&opts.debugFile, "debug", "", "Write debug logs to `FILE`")
	fs.StringVar(&opts.workspace, "workspace", "", "Bitbucket workspace `SLUG` (overrides the config file)")
	fs.Var(&opts.repos, "repo", "Repository `SLUG` to show, repeatable (overrides the configured repositories)")
	if err := fs.Parse(args); err != nil {
		return nil, fmt.Errorf("parsing flags: %w", err)
	}
	return opts, nil
}

// applyTo overrides the loaded configuration with the values given on the
// command line. Unset flags leave the configuration untouched.
func (o *cliOptions) applyTo(cfg *config.Config) {
	if o.workspace != "" {
		cfg.Bitbucket.Workspace = o.workspace
	}
	if len(o.repos) > 0 {
		cfg.Bitbucket.Repositories = append([]string(nil), o.repos...)
	}
}
