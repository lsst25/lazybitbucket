package config

import (
	"bytes"
	"strings"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

const echoCommand = "echo x"

func resolveOne(t *testing.T, command CustomCommandConfig) ResolvedCustomCommand {
	t.Helper()
	cfg := &Config{CustomCommands: []CustomCommandConfig{command}}
	resolved, err := cfg.ResolveCustomCommands()
	if err != nil {
		t.Fatalf("ResolveCustomCommands: %v", err)
	}
	if len(resolved) != 1 {
		t.Fatalf("len(resolved) = %d, want 1", len(resolved))
	}
	return resolved[0]
}

func TestResolveCustomCommands_DefaultContexts(t *testing.T) {
	t.Parallel()
	got := resolveOne(t, CustomCommandConfig{Key: "y", Name: "Copy", Command: "echo {{.ID}}"})

	testkit.AssertSliceEqual(t, "Contexts", got.Contexts, DefaultCommandContexts)
	testkit.AssertSliceEqual(t, "DefaultCommandContexts", DefaultCommandContexts,
		[]Context{CtxPullRequests, CtxDetail})
	testkit.AssertEqual(t, "Scopes", got.Scopes, ScopePullRequest|ScopeRepo)
}

func TestResolveCustomCommands_ContextScopes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		contexts []string
		want     ScopeMask
	}{
		{"pullrequests", []string{"pullrequests"}, ScopePullRequest | ScopeRepo},
		{"builds", []string{"builds"}, ScopePullRequest | ScopeRepo | ScopeBuild},
		{"repos", []string{"repos"}, ScopeRepo},
		{"detail", []string{"detail"}, ScopePullRequest | ScopeRepo},
		{"diff", []string{"diff"}, ScopePullRequest | ScopeRepo | ScopeFile},
		{"repos and builds union", []string{"repos", "builds"}, ScopePullRequest | ScopeRepo | ScopeBuild},
		{"detail and diff union", []string{"detail", "diff"}, ScopePullRequest | ScopeRepo | ScopeFile},
		{
			"every context",
			[]string{"pullrequests", "builds", "repos", "detail", "diff"},
			ScopePullRequest | ScopeRepo | ScopeBuild | ScopeFile,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := resolveOne(t, CustomCommandConfig{
				Key: "a", Name: "cmd", Command: echoCommand, Contexts: tc.contexts,
			})
			testkit.AssertEqual(t, "Scopes", got.Scopes, tc.want)
			testkit.AssertEqual(t, "len(Contexts)", len(got.Contexts), len(tc.contexts))
		})
	}
}

func TestScopeMask_BitsAreDistinct(t *testing.T) {
	t.Parallel()
	testkit.AssertEqual(t, "ScopePullRequest", ScopePullRequest, ScopeMask(1))
	testkit.AssertEqual(t, "ScopeRepo", ScopeRepo, ScopeMask(2))
	testkit.AssertEqual(t, "ScopeBuild", ScopeBuild, ScopeMask(4))
	testkit.AssertEqual(t, "ScopeFile", ScopeFile, ScopeMask(8))
	testkit.AssertEqual(t, "ScopeChangedFiles", ScopeChangedFiles, ScopeMask(16))
}

func TestResolveCustomCommands_ChangedFilesIsOptIn(t *testing.T) {
	t.Parallel()

	plain := resolveOne(t, CustomCommandConfig{Key: "a", Name: "cmd", Command: echoCommand})
	testkit.AssertEqual(t, "NeedsChangedFiles", plain.NeedsChangedFiles(), false)
	testkit.AssertEqual(t, "Scopes&ScopeChangedFiles", plain.Scopes&ScopeChangedFiles, ScopeMask(0))

	got := resolveOne(t, CustomCommandConfig{
		Key: "E", Name: "edit all", Command: echoCommand,
		Contexts: []string{"diff"}, ChangedFiles: true,
	})
	testkit.AssertEqual(t, "NeedsChangedFiles", got.NeedsChangedFiles(), true)
	testkit.AssertEqual(t, "Scopes", got.Scopes,
		ScopePullRequest|ScopeRepo|ScopeFile|ScopeChangedFiles)
}

func TestResolveCustomCommands_ChangedFilesNeedsAPullRequestContext(t *testing.T) {
	t.Parallel()
	cfg := &Config{CustomCommands: []CustomCommandConfig{{
		Key: "E", Name: "edit all", Command: echoCommand,
		Contexts: []string{"pullrequests", "repos"}, ChangedFiles: true,
	}}}

	_, err := cfg.ResolveCustomCommands()

	if err == nil {
		t.Fatal("the repos context never has a pull request; changedFiles must be rejected")
	}
	if !strings.Contains(err.Error(), "changedFiles") || !strings.Contains(err.Error(), `"repos"`) {
		t.Errorf("error = %v, want it to name changedFiles and the repos context", err)
	}
}

func TestCtxDiff_IsNotADefaultContext(t *testing.T) {
	t.Parallel()
	// The diff context is opt-in: a command that omits `contexts:` must not
	// suddenly appear on the Files tab and the per-file diff view.
	testkit.AssertSliceEqual(t, "DefaultCommandContexts", DefaultCommandContexts,
		[]Context{CtxPullRequests, CtxDetail})

	got := resolveOne(t, CustomCommandConfig{Key: "y", Name: "Copy", Command: echoCommand})
	testkit.AssertEqual(t, "HasContext(CtxDiff)", got.HasContext(CtxDiff), false)
	testkit.AssertEqual(t, "Scopes&ScopeFile", got.Scopes&ScopeFile, ScopeMask(0))
}

func TestContextScopes_CoversEveryContext(t *testing.T) {
	t.Parallel()
	for _, ctx := range []Context{CtxPullRequests, CtxBuilds, CtxRepos, CtxDetail, CtxDiff} {
		scopes, ok := contextScopes[ctx]
		if !ok {
			t.Errorf("context %q has no scope mask", ctx)
			continue
		}
		if scopes == 0 {
			t.Errorf("context %q has an empty scope mask", ctx)
		}
	}
	testkit.AssertEqual(t, "len(contextScopes)", len(contextScopes), 5)
}

func TestResolveCustomCommands_EmptyFieldsError(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		cmd  CustomCommandConfig
	}{
		{"empty key", CustomCommandConfig{Name: "x", Command: echoCommand}},
		{"empty name", CustomCommandConfig{Key: "a", Command: echoCommand}},
		{"empty command", CustomCommandConfig{Key: "a", Name: "x"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := &Config{CustomCommands: []CustomCommandConfig{tc.cmd}}
			if _, err := cfg.ResolveCustomCommands(); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestResolveCustomCommands_UnknownContextError(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		CustomCommands: []CustomCommandConfig{
			{Key: "a", Name: "x", Command: echoCommand, Contexts: []string{"bogus"}},
		},
	}
	_, err := cfg.ResolveCustomCommands()
	if err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Errorf("expected unknown context error, got %v", err)
	}
}

func TestResolveCustomCommands_LegacyJiraContextsRejected(t *testing.T) {
	t.Parallel()
	for _, legacy := range []string{"issues", "info", "projects", "detail.comments"} {
		cfg := &Config{
			CustomCommands: []CustomCommandConfig{
				{Key: "a", Name: "x", Command: echoCommand, Contexts: []string{legacy}},
			},
		}
		if _, err := cfg.ResolveCustomCommands(); err == nil {
			t.Errorf("context %q should no longer be accepted", legacy)
		}
	}
}

func TestResolveCustomCommands_DuplicateKeySameContextError(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		CustomCommands: []CustomCommandConfig{
			{Key: "a", Name: "first", Command: "echo 1", Contexts: []string{"pullrequests"}},
			{Key: "a", Name: "second", Command: "echo 2", Contexts: []string{"pullrequests"}},
		},
	}
	if _, err := cfg.ResolveCustomCommands(); err == nil {
		t.Error("expected duplicate key error")
	}
}

func TestResolveCustomCommands_DuplicateKeyDisjointContextsAllowed(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		CustomCommands: []CustomCommandConfig{
			{Key: "a", Name: "first", Command: "echo 1", Contexts: []string{"pullrequests"}},
			{Key: "a", Name: "second", Command: "echo 2", Contexts: []string{"repos"}},
		},
	}
	if _, err := cfg.ResolveCustomCommands(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResolveCustomCommands_RefreshAndSuspendPassThrough(t *testing.T) {
	t.Parallel()
	plain := resolveOne(t, CustomCommandConfig{Key: "y", Name: "Copy", Command: echoCommand})
	if plain.Refresh {
		t.Error("Refresh should default to false")
	}
	if !plain.ShouldSuspend() {
		t.Error("ShouldSuspend should default to true")
	}

	explicit := resolveOne(t, CustomCommandConfig{
		Key: "w", Name: "Checkout", Command: echoCommand, Refresh: true, Suspend: pointerTo(false),
	})
	if !explicit.Refresh {
		t.Error("Refresh should be true when set")
	}
	if explicit.ShouldSuspend() {
		t.Error("ShouldSuspend should be false when suspend: false")
	}
}

func TestResolveCustomCommands_InvalidTemplateError(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		CustomCommands: []CustomCommandConfig{
			{Key: "a", Name: "x", Command: "echo {{.Unclosed"},
		},
	}
	if _, err := cfg.ResolveCustomCommands(); err == nil {
		t.Error("expected template parse error")
	}
}

func TestResolveCustomCommands_ShellescapeFunc(t *testing.T) {
	t.Parallel()
	resolved := resolveOne(t, CustomCommandConfig{
		Key: "s", Name: "Shellescape", Command: "echo {{.Title | shellescape}}",
	})

	var buf bytes.Buffer
	data := struct{ Title string }{Title: "what's up"}
	if err := resolved.Template.Execute(&buf, data); err != nil {
		t.Fatalf("template execute error: %v", err)
	}
	testkit.AssertEqual(t, "output", buf.String(), `echo 'what'\''s up'`)
}

func TestResolveCustomCommands_SlugifyFunc(t *testing.T) {
	t.Parallel()
	resolved := resolveOne(t, CustomCommandConfig{
		Key: "b", Name: "Branch", Command: "git checkout -b {{.Title | slugify}}",
	})

	var buf bytes.Buffer
	data := struct{ Title string }{Title: "Fix Login Bug #42!"}
	if err := resolved.Template.Execute(&buf, data); err != nil {
		t.Fatalf("template execute error: %v", err)
	}
	testkit.AssertEqual(t, "output", buf.String(), "git checkout -b fix-login-bug-42")
}

func TestResolveCustomCommands_MissingKeyIsAnExecutionError(t *testing.T) {
	t.Parallel()
	resolved := resolveOne(t, CustomCommandConfig{Key: "m", Name: "Missing", Command: "echo {{.Nope}}"})

	var buf bytes.Buffer
	if err := resolved.Template.Execute(&buf, map[string]string{"ID": "1"}); err == nil {
		t.Error("expected missingkey=error to reject an unknown field")
	}
}

func TestResolveCustomCommands_EmptyListYieldsEmptySlice(t *testing.T) {
	t.Parallel()
	cfg := &Config{}
	resolved, err := cfg.ResolveCustomCommands()
	if err != nil {
		t.Fatalf("ResolveCustomCommands: %v", err)
	}
	testkit.AssertEqual(t, "len(resolved)", len(resolved), 0)
}

func TestSlugify(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"hello", "hello"},
		{"Hello World", "hello-world"},
		{"Fix Bug #42!", "fix-bug-42"},
		{"  leading and trailing  ", "leading-and-trailing"},
		{"multiple   spaces", "multiple-spaces"},
		{"--already--dashed--", "already-dashed"},
		{"snake_case_name", "snake-case-name"},
		{"path/to/thing", "path-to-thing"},
		{"!!!", ""},
		{"123-foo", "123-foo"},
		{"a\tb\nc", "a-b-c"},
		{"Übung 1", "uebung-1"},
		{"Größe", "groesse"},
		{"über alles", "ueber-alles"},
		{"straße", "strasse"},
		{"café", "cafe"},
		{"naïve", "naive"},
		{"jalapeño piñata", "jalapeno-pinata"},
	}
	for _, tc := range cases {
		if got := slugify(tc.in); got != tc.want {
			t.Errorf("slugify(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestResolvedCustomCommand_ShouldSuspend(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		suspend *bool
		want    bool
	}{
		{"nil defaults to true", nil, true},
		{"explicit true", pointerTo(true), true},
		{"explicit false", pointerTo(false), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			resolved := ResolvedCustomCommand{Suspend: tc.suspend}
			testkit.AssertEqual(t, "ShouldSuspend", resolved.ShouldSuspend(), tc.want)
		})
	}
}

func TestResolvedCustomCommand_HasContext(t *testing.T) {
	t.Parallel()
	resolved := ResolvedCustomCommand{Contexts: []Context{CtxPullRequests, CtxDetail}}
	tests := []struct {
		name    string
		context Context
		want    bool
	}{
		{"bound context", CtxPullRequests, true},
		{"other bound context", CtxDetail, true},
		{"unbound repos", CtxRepos, false},
		{"unbound builds", CtxBuilds, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			testkit.AssertEqual(t, "HasContext("+string(tc.context)+")", resolved.HasContext(tc.context), tc.want)
		})
	}
}
