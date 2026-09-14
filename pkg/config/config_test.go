package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

func pointerTo[T any](v T) *T { return &v }

// clearBitbucketEnv drops every BITBUCKET_* override so the ambient
// environment cannot leak into Load. Callers point LAZYBITBUCKET_CONFIG_DIR at
// a temp dir themselves, so paralleltest sees the t.Setenv call.
func clearBitbucketEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{envWorkspace, envRepositories, envEmail, envToken, envAPIURL, envReadOnly} {
		t.Setenv(key, "")
	}
}

func writeConfigFile(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", configFileName, err)
	}
}

func loadOrFatal(t *testing.T) *Config {
	t.Helper()
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg
}

func TestConfigDir_EnvOverrideWinsOverXDG(t *testing.T) {
	explicitDir := t.TempDir()
	xdgDir := t.TempDir()

	t.Setenv(envConfigDir, explicitDir)
	t.Setenv("XDG_CONFIG_HOME", xdgDir)

	testkit.AssertEqual(t, "ConfigDir", ConfigDir(), explicitDir)
}

func TestConfigDir_XDGConfigHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("XDG_CONFIG_HOME precedence is not used on Windows")
	}

	xdgDir := t.TempDir()
	t.Setenv(envConfigDir, "")
	t.Setenv("XDG_CONFIG_HOME", xdgDir)

	testkit.AssertEqual(t, "ConfigDir", ConfigDir(), filepath.Join(xdgDir, "lazybitbucket"))
}

func TestConfigDir_FallsBackToUserConfigDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("env fallback chain differs on Windows")
	}

	t.Setenv(envConfigDir, "")
	t.Setenv("XDG_CONFIG_HOME", "")

	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("os.UserConfigDir: %v", err)
	}
	testkit.AssertEqual(t, "ConfigDir", ConfigDir(), filepath.Join(userConfigDir, "lazybitbucket"))
}

func TestConfigDir_FallsBackToDotConfigWithoutHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("env fallback chain differs on Windows")
	}

	t.Setenv(envConfigDir, "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")

	testkit.AssertEqual(t, "ConfigDir", ConfigDir(), filepath.Join(".", ".config", "lazybitbucket"))
}

func TestConfigPath_JoinsConfigDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envConfigDir, dir)

	testkit.AssertEqual(t, "ConfigPath", ConfigPath(), filepath.Join(dir, configFileName))
}

func TestDefaultConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()

	testkit.AssertEqual(t, "GUI.Theme", cfg.GUI.Theme, "default")
	testkit.AssertEqual(t, "GUI.SidePanelWidth", cfg.GUI.SidePanelWidth, 40)
	testkit.AssertEqual(t, "GUI.CollapsedPanelHeight", cfg.GUI.CollapsedPanelHeight, 5)
	testkit.AssertSliceEqual(t, "GUI.PullRequestListFields", cfg.GUI.PullRequestListFields,
		[]string{"repo", "id", "title", "approvals", "build", "author", "updated"})
	testkit.AssertEqual(t, "RefreshInterval", cfg.RefreshInterval, "120s")
	testkit.AssertEqual(t, "len(PullRequestTabs)", len(cfg.PullRequestTabs), 3)
	if cfg.MaxResults != nil {
		t.Errorf("default MaxResults should be nil (unset), got %d", *cfg.MaxResults)
	}
	testkit.AssertEqual(t, "ResolveGlobalMaxResults", cfg.ResolveGlobalMaxResults(), DefaultMaxResults)
	testkit.AssertEqual(t, "ResolveBaseURL", cfg.ResolveBaseURL(), DefaultBaseURL)

	interval, err := cfg.ResolveRefreshInterval()
	if err != nil {
		t.Fatalf("ResolveRefreshInterval: %v", err)
	}
	testkit.AssertEqual(t, "refresh interval", interval, 120*time.Second)
}

func TestDefaultPullRequestTabs(t *testing.T) {
	t.Parallel()
	tabs := DefaultPullRequestTabs()
	if len(tabs) != 3 {
		t.Fatalf("len(tabs) = %d, want 3", len(tabs))
	}

	names := make([]string, 0, len(tabs))
	filters := make([]string, 0, len(tabs))
	for _, tab := range tabs {
		names = append(names, tab.Name)
		filters = append(filters, tab.Filter)
		testkit.AssertEqual(t, "state of tab "+tab.Name, tab.ResolveState(), "OPEN")
	}
	testkit.AssertSliceEqual(t, "names", names, []string{"Open", "Mine", "Reviewing"})
	testkit.AssertSliceEqual(t, "filters", filters, []string{"all", "mine", "reviewing"})
}

func TestPullRequestListFieldNames(t *testing.T) {
	t.Parallel()
	want := []string{"repo", "id", "title", "author", "approvals", "build", "updated", "branch", "state"}
	testkit.AssertSliceEqual(t, "PullRequestListFieldNames", PullRequestListFieldNames(), want)

	// The caller gets a copy: mutating it must not affect the package state.
	names := PullRequestListFieldNames()
	names[0] = "mutated"
	testkit.AssertEqual(t, "first field after mutation", PullRequestListFieldNames()[0], "repo")
}

func TestLoad_DefaultsWhenNoConfigFile(t *testing.T) {
	t.Setenv(envConfigDir, t.TempDir())
	clearBitbucketEnv(t)

	cfg := loadOrFatal(t)
	testkit.AssertEqual(t, "Bitbucket.Workspace", cfg.Bitbucket.Workspace, "")
	testkit.AssertEqual(t, "len(Repositories)", len(cfg.Bitbucket.Repositories), 0)
	testkit.AssertEqual(t, "GUI.Theme", cfg.GUI.Theme, "default")
	testkit.AssertEqual(t, "len(PullRequestTabs)", len(cfg.PullRequestTabs), 3)
}

func TestLoad_ReadsBitbucketSectionFromYAML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envConfigDir, dir)
	clearBitbucketEnv(t)
	writeConfigFile(t, dir, `bitbucket:
  workspace: acme
  repositories:
    - api-gateway
    - web-app
  email: user@example.com
  baseUrl: https://bitbucket.example.com/2.0/
`)

	cfg := loadOrFatal(t)
	testkit.AssertEqual(t, "Workspace", cfg.Bitbucket.Workspace, testWorkspace)
	testkit.AssertSliceEqual(t, "Repositories", cfg.Bitbucket.Repositories, []string{testRepo, "web-app"})
	testkit.AssertEqual(t, "Email", cfg.Bitbucket.Email, testEmail)
	testkit.AssertEqual(t, "Token", cfg.Bitbucket.Token, "")
	testkit.AssertEqual(t, "ResolveBaseURL", cfg.ResolveBaseURL(), "https://bitbucket.example.com/2.0")
}

func TestLoad_TokenIsNeverReadFromYAML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envConfigDir, dir)
	clearBitbucketEnv(t)
	writeConfigFile(t, dir, `bitbucket:
  token: from-file
`)

	cfg := loadOrFatal(t)
	testkit.AssertEqual(t, "Token", cfg.Bitbucket.Token, "")
}

func TestLoad_EnvOverridesWinOverFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envConfigDir, dir)
	clearBitbucketEnv(t)
	writeConfigFile(t, dir, `bitbucket:
  workspace: from-file
  repositories: [from-file-repo]
  email: file@example.com
  baseUrl: https://file.example.com
`)

	t.Setenv(envWorkspace, testWorkspace)
	t.Setenv(envRepositories, "api-gateway, web-app ,,infra")
	t.Setenv(envEmail, testEmail)
	t.Setenv(envToken, testToken)
	t.Setenv(envAPIURL, "https://env.example.com/2.0")

	cfg := loadOrFatal(t)
	testkit.AssertEqual(t, "Workspace", cfg.Bitbucket.Workspace, testWorkspace)
	testkit.AssertSliceEqual(t, "Repositories", cfg.Bitbucket.Repositories,
		[]string{testRepo, "web-app", "infra"})
	testkit.AssertEqual(t, "Email", cfg.Bitbucket.Email, testEmail)
	testkit.AssertEqual(t, "Token", cfg.Bitbucket.Token, testToken)
	testkit.AssertEqual(t, "ResolveBaseURL", cfg.ResolveBaseURL(), "https://env.example.com/2.0")
}

func TestLoad_EmptyRepositoriesEnvKeepsFileValue(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envConfigDir, dir)
	clearBitbucketEnv(t)
	writeConfigFile(t, dir, `bitbucket:
  repositories: [api-gateway]
`)
	t.Setenv(envRepositories, " , ")

	cfg := loadOrFatal(t)
	testkit.AssertSliceEqual(t, "Repositories", cfg.Bitbucket.Repositories, []string{testRepo})
}

func TestLoad_ReadsFullConfigFromYAML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envConfigDir, dir)
	clearBitbucketEnv(t)
	writeConfigFile(t, dir, `gui:
  theme: dracula
  sidePanelWidth: 55
  collapsedPanelHeight: 3
  pullRequestListFields: [id, title, state, branch]
  themeColors:
    primary: "#ff00ff"
keybinding:
  universal:
    quit: Q
    focusPullRequests: "2"
    focusBuilds: "3"
    focusRepos: "4"
  navigation:
    halfPageDown: ctrl+d
  pullRequests:
    checkout: c
    copyURL: y
  detail:
    back: backspace
pullRequestTabs:
  - name: Everything
    state: merged
    filter: all
    query: 'title ~ "fix"'
    maxResults: 10
maxResults: 25
refreshInterval: 45s
git:
  closeOnCheckout: true
editor: vim
rendererStyle: light
diffRenderer: delta --color-only --paging=never
`)

	cfg := loadOrFatal(t)
	testkit.AssertEqual(t, "GUI.Theme", cfg.GUI.Theme, "dracula")
	testkit.AssertEqual(t, "GUI.SidePanelWidth", cfg.GUI.SidePanelWidth, 55)
	testkit.AssertEqual(t, "GUI.CollapsedPanelHeight", cfg.GUI.CollapsedPanelHeight, 3)
	testkit.AssertSliceEqual(t, "GUI.PullRequestListFields", cfg.GUI.PullRequestListFields,
		[]string{"id", "title", "state", "branch"})
	testkit.AssertEqual(t, "GUI.ThemeColors[primary]", cfg.GUI.ThemeColors["primary"], "#ff00ff")
	testkit.AssertEqual(t, "Keybinding.Universal.Quit", cfg.Keybinding.Universal.Quit, "Q")
	testkit.AssertEqual(t, "Keybinding.Universal.FocusPullRequests", cfg.Keybinding.Universal.FocusPullRequests, "2")
	testkit.AssertEqual(t, "Keybinding.Universal.FocusBuilds", cfg.Keybinding.Universal.FocusBuilds, "3")
	testkit.AssertEqual(t, "Keybinding.Universal.FocusRepos", cfg.Keybinding.Universal.FocusRepos, "4")
	testkit.AssertEqual(t, "Keybinding.Navigation.HalfDown", cfg.Keybinding.Navigation.HalfDown, "ctrl+d")
	testkit.AssertEqual(t, "Keybinding.PullRequests.Checkout", cfg.Keybinding.PullRequests.Checkout, "c")
	testkit.AssertEqual(t, "Keybinding.PullRequests.CopyURL", cfg.Keybinding.PullRequests.CopyURL, "y")
	testkit.AssertEqual(t, "Keybinding.Detail.Back", cfg.Keybinding.Detail.Back, "backspace")
	testkit.AssertEqual(t, "len(PullRequestTabs)", len(cfg.PullRequestTabs), 1)

	tab := cfg.PullRequestTabs[0]
	testkit.AssertEqual(t, "tab.Name", tab.Name, "Everything")
	testkit.AssertEqual(t, "tab.ResolveState", tab.ResolveState(), "MERGED")
	testkit.AssertEqual(t, "tab.Query", tab.Query, `title ~ "fix"`)
	testkit.AssertEqual(t, "ResolveMaxResults(tab)", cfg.ResolveMaxResults(tab), 10)
	testkit.AssertEqual(t, "ResolveGlobalMaxResults", cfg.ResolveGlobalMaxResults(), 25)
	testkit.AssertEqual(t, "Git.CloseOnCheckout", cfg.Git.CloseOnCheckout, true)
	testkit.AssertEqual(t, "Editor", cfg.Editor, "vim")
	testkit.AssertEqual(t, "RendererStyle", cfg.RendererStyle, RendererStyleLight)
	testkit.AssertEqual(t, "DiffRenderer", cfg.DiffRenderer, "delta --color-only --paging=never")

	interval, err := cfg.ResolveRefreshInterval()
	if err != nil {
		t.Fatalf("ResolveRefreshInterval: %v", err)
	}
	testkit.AssertEqual(t, "refresh interval", interval, 45*time.Second)
}

func TestLoad_RejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		cfgYAML string
		wantErr string
	}{
		{
			"unknown list field",
			"gui:\n  pullRequestListFields: [repo, reviewers]\n",
			`unknown field "reviewers"`,
		},
		{
			"unknown tab state",
			"pullRequestTabs:\n  - name: Bad\n    state: closed\n",
			`unknown state "closed"`,
		},
		{
			"invalid refresh interval",
			"refreshInterval: soon\n",
			`invalid refreshInterval "soon"`,
		},
		{
			"negative refresh interval",
			"refreshInterval: -30s\n",
			"must not be negative",
		},
		{
			"unknown renderer style",
			"rendererStyle: solarized\n",
			"solarized",
		},
		{
			"unknown editorInTmux mode",
			"editorInTmux: split\n",
			`invalid editorInTmux "split"`,
		},
		{
			"unknown screen mode",
			"gui:\n  screenMode: wide\n",
			`invalid gui.screenMode "wide"`,
		},
		{
			"unknown start view",
			"gui:\n  startView: builds\n",
			`invalid gui.startView "builds"`,
		},
		{
			"invalid custom command template",
			"customCommands:\n  - key: \"y\"\n    name: broken\n    command: \"echo {{.Unclosed\"\n",
			"template parse error",
		},
		{
			"unknown custom command context",
			"customCommands:\n  - key: \"y\"\n    name: bad\n    command: echo\n    contexts: [issues]\n",
			`unknown context "issues"`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv(envConfigDir, dir)
			clearBitbucketEnv(t)
			writeConfigFile(t, dir, tc.cfgYAML)

			_, err := Load()
			if err == nil {
				t.Fatalf("Load() should reject %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestLoad_AcceptsEveryListFieldAndTabState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envConfigDir, dir)
	clearBitbucketEnv(t)
	writeConfigFile(t, dir, `gui:
  pullRequestListFields: [repo, id, title, author, approvals, build, updated, branch, state]
pullRequestTabs:
  - name: Open
  - name: Merged
    state: MERGED
  - name: Declined
    state: declined
  - name: Superseded
    state: Superseded
refreshInterval: "off"
`)

	cfg := loadOrFatal(t)
	testkit.AssertEqual(t, "len(PullRequestTabs)", len(cfg.PullRequestTabs), 4)

	interval, err := cfg.ResolveRefreshInterval()
	if err != nil {
		t.Fatalf("ResolveRefreshInterval: %v", err)
	}
	testkit.AssertEqual(t, "refresh interval", interval, time.Duration(0))
}

func TestLoad_CustomCommandRefreshFromYAML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envConfigDir, dir)
	clearBitbucketEnv(t)
	writeConfigFile(t, dir, `customCommands:
  - key: "y"
    name: "copy"
    command: "echo {{.Key}}"
    suspend: false
  - key: "w"
    name: "checkout"
    command: "echo {{.Key}}"
    refresh: true
`)

	cfg := loadOrFatal(t)
	if cfg.CustomCommands[0].Refresh {
		t.Error("first command: Refresh should default to false")
	}
	if cfg.CustomCommands[0].ShouldSuspend() {
		t.Error("first command: ShouldSuspend should be false when suspend: false")
	}
	if !cfg.CustomCommands[1].Refresh {
		t.Error("second command: Refresh should be true")
	}
}

func TestLoad_InvalidYAMLErrors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envConfigDir, dir)
	clearBitbucketEnv(t)
	writeConfigFile(t, dir, "gui: [unclosed")

	if _, err := Load(); err == nil {
		t.Fatal("Load() with invalid YAML should error")
	}
}

func TestLoad_ReadErrorWhenConfigFileIsDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envConfigDir, dir)
	clearBitbucketEnv(t)
	if err := os.Mkdir(filepath.Join(dir, configFileName), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(); err == nil {
		t.Fatal("Load() with unreadable config file should error")
	}
}

func TestSave_RoundTripsWithoutToken(t *testing.T) {
	t.Setenv(envConfigDir, t.TempDir())
	clearBitbucketEnv(t)

	cfg := DefaultConfig()
	cfg.Bitbucket.Workspace = testWorkspace
	cfg.Bitbucket.Repositories = []string{testRepo}
	cfg.Bitbucket.Email = testEmail
	cfg.Bitbucket.Token = testToken

	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	written, err := os.ReadFile(ConfigPath())
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	if strings.Contains(string(written), testToken) {
		t.Error("saved config must not contain the API token")
	}

	loaded := loadOrFatal(t)
	testkit.AssertEqual(t, "Workspace", loaded.Bitbucket.Workspace, testWorkspace)
	testkit.AssertEqual(t, "Email", loaded.Bitbucket.Email, testEmail)
	testkit.AssertEqual(t, "Token", loaded.Bitbucket.Token, "")
	testkit.AssertSliceEqual(t, "Repositories", loaded.Bitbucket.Repositories, []string{testRepo})
	testkit.AssertEqual(t, "len(PullRequestTabs)", len(loaded.PullRequestTabs), 3)
}

func TestSave_CreatesMissingConfigDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "lazybitbucket")
	t.Setenv(envConfigDir, dir)

	if err := Save(DefaultConfig()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(ConfigPath()); err != nil {
		t.Errorf("config file missing after Save: %v", err)
	}
}

func TestSave_FailsWhenConfigDirIsFile(t *testing.T) {
	blockingFile := filepath.Join(t.TempDir(), "occupied")
	if err := os.WriteFile(blockingFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envConfigDir, blockingFile)

	if err := Save(DefaultConfig()); err == nil {
		t.Fatal("Save() into a file path should error")
	}
}

func TestSave_NilConfigErrors(t *testing.T) {
	t.Setenv(envConfigDir, t.TempDir())

	if err := Save(nil); err == nil {
		t.Fatal("Save(nil) should error")
	}
}

func TestResolveMaxResults(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		global *int
		tab    PullRequestTabConfig
		want   int
	}{
		{"all unset → default", nil, PullRequestTabConfig{}, DefaultMaxResults},
		{"global only", pointerTo(25), PullRequestTabConfig{}, 25},
		{"tab overrides global", pointerTo(25), PullRequestTabConfig{MaxResults: pointerTo(75)}, 75},
		{"negative global ignored", pointerTo(-5), PullRequestTabConfig{}, DefaultMaxResults},
		{"zero tab falls back to global", pointerTo(40), PullRequestTabConfig{MaxResults: pointerTo(0)}, 40},
		{"large global not clamped", pointerTo(500), PullRequestTabConfig{}, 500},
		{"large tab override not clamped", pointerTo(50), PullRequestTabConfig{MaxResults: pointerTo(1000)}, 1000},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := &Config{MaxResults: tc.global}
			testkit.AssertEqual(t, "ResolveMaxResults", cfg.ResolveMaxResults(tc.tab), tc.want)
		})
	}
}

func TestResolveGlobalMaxResults(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		global *int
		want   int
	}{
		{"nil → default", nil, DefaultMaxResults},
		{"zero → default", pointerTo(0), DefaultMaxResults},
		{"negative → default", pointerTo(-1), DefaultMaxResults},
		{"set", pointerTo(125), 125},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := &Config{MaxResults: tc.global}
			testkit.AssertEqual(t, "ResolveGlobalMaxResults", cfg.ResolveGlobalMaxResults(), tc.want)
		})
	}
}

func TestResolveRefreshInterval(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		value   string
		want    time.Duration
		wantErr bool
	}{
		{"empty → 120s", "", 120 * time.Second, false},
		{"whitespace → 120s", "  ", 120 * time.Second, false},
		{"explicit duration", "30s", 30 * time.Second, false},
		{"minutes", "5m", 5 * time.Minute, false},
		{"zero disables", "0", 0, false},
		{"off disables", "off", 0, false},
		{"OFF is case-insensitive", "OFF", 0, false},
		{"0s parses to zero", "0s", 0, false},
		{"unparsable errors", "soon", 0, true},
		{"bare number errors", "30", 0, true},
		{"negative errors", "-1m", 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := &Config{RefreshInterval: tc.value}
			got, err := cfg.ResolveRefreshInterval()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ResolveRefreshInterval(%q) = %v, want error", tc.value, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveRefreshInterval(%q): %v", tc.value, err)
			}
			testkit.AssertEqual(t, "interval", got, tc.want)
		})
	}
}

func TestResolveEditor(t *testing.T) {
	tests := []struct {
		name   string
		editor string
		visual string
		envVar string
		want   string
	}{
		{"config wins", "helix", "vim", "nano", "helix"},
		{"VISUAL beats EDITOR", "", "vim", "nano", "vim"},
		{"EDITOR when no VISUAL", "", "", "nano", "nano"},
		{"default when nothing set", "", "", "", DefaultEditor},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("VISUAL", tc.visual)
			t.Setenv("EDITOR", tc.envVar)

			cfg := &Config{Editor: tc.editor}
			testkit.AssertEqual(t, "ResolveEditor", cfg.ResolveEditor(), tc.want)
		})
	}
}

func TestResolveDiffRenderer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		value    string
		wantName string
		wantArgs []string
		wantOK   bool
	}{
		{"unset is off", "", "", nil, false},
		{"whitespace only is off", "   \t ", "", nil, false},
		{"program alone", "delta", "delta", []string{}, true},
		{
			"program with flags", "delta --color-only --paging=never",
			"delta", []string{"--color-only", "--paging=never"}, true,
		},
		{"extra spaces collapse", "  diff-so-fancy   --patch  ", "diff-so-fancy", []string{"--patch"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := &Config{DiffRenderer: tc.value}

			name, args, ok := cfg.ResolveDiffRenderer()

			testkit.AssertEqual(t, "ok", ok, tc.wantOK)
			testkit.AssertEqual(t, "name", name, tc.wantName)
			testkit.AssertSliceEqual(t, "args", args, tc.wantArgs)
		})
	}
}

func TestResolveBaseURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"empty → default", "", DefaultBaseURL},
		{"whitespace → default", "   ", DefaultBaseURL},
		{"explicit", "https://bb.example.com/2.0", "https://bb.example.com/2.0"},
		{"trailing slash trimmed", "https://bb.example.com/2.0/", "https://bb.example.com/2.0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := &Config{Bitbucket: BitbucketConfig{BaseURL: tc.value}}
			testkit.AssertEqual(t, "ResolveBaseURL", cfg.ResolveBaseURL(), tc.want)
		})
	}
}

func TestPullRequestTabConfig_ResolveState(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		state string
		want  string
	}{
		{"empty → OPEN", "", "OPEN"},
		{"whitespace → OPEN", " ", "OPEN"},
		{"already uppercase", "MERGED", "MERGED"},
		{"lowercase uppercased", "declined", "DECLINED"},
		{"mixed case uppercased", "Superseded", "SUPERSEDED"},
		{"padded", " open ", "OPEN"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tab := PullRequestTabConfig{State: tc.state}
			testkit.AssertEqual(t, "ResolveState", tab.ResolveState(), tc.want)
		})
	}
}

func TestValidateRendererStyle(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"empty defaults to auto", "", false},
		{"auto", RendererStyleAuto, false},
		{"dark", RendererStyleDark, false},
		{"light", RendererStyleLight, false},
		{"notty", RendererStyleNoTTY, false},
		{"unknown value errors", "solarized", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateRendererStyle(tc.value)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("validateRendererStyle(%q) = nil, want error", tc.value)
				}
				if !strings.Contains(err.Error(), tc.value) {
					t.Errorf("error %q should include the bad value %q", err, tc.value)
				}
				return
			}
			if err != nil {
				t.Errorf("validateRendererStyle(%q) = %v, want nil", tc.value, err)
			}
		})
	}
}

func TestGUIConfig_PrefetchesDetailTabs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		prefetch *bool
		want     bool
	}{
		{"nil defaults to true", nil, true},
		{"explicit true", pointerTo(true), true},
		{"explicit false", pointerTo(false), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gui := GUIConfig{PrefetchDetailTabs: tc.prefetch}
			testkit.AssertEqual(t, "PrefetchesDetailTabs", gui.PrefetchesDetailTabs(), tc.want)
		})
	}
}

func TestLoad_PrefetchDetailTabsFromYAML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envConfigDir, dir)
	clearBitbucketEnv(t)
	writeConfigFile(t, dir, `gui:
  prefetchDetailTabs: false
`)

	cfg := loadOrFatal(t)
	testkit.AssertEqual(t, "PrefetchesDetailTabs", cfg.GUI.PrefetchesDetailTabs(), false)
}

func TestLoad_PrefetchDetailTabsDefaultsToOn(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envConfigDir, dir)
	clearBitbucketEnv(t)
	writeConfigFile(t, dir, `gui:
  theme: default
`)

	cfg := loadOrFatal(t)
	testkit.AssertEqual(t, "PrefetchesDetailTabs", cfg.GUI.PrefetchesDetailTabs(), true)
}

func TestCustomCommandConfig_ShouldSuspend(t *testing.T) {
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
			command := CustomCommandConfig{Suspend: tc.suspend}
			testkit.AssertEqual(t, "ShouldSuspend", command.ShouldSuspend(), tc.want)
		})
	}
}

// detailKeysFixture is a fully-populated DetailKeys with a distinct value per
// field, so a wrong or duplicated yaml tag shows up as a mismatch.
func detailKeysFixture() DetailKeys {
	return DetailKeys{
		FocusLeft:    "h",
		Back:         "esc",
		ScrollDown:   "J",
		ScrollUp:     "K",
		HalfPageDown: "ctrl+f",
		HalfPageUp:   "ctrl+b",
		OverviewTab:  "i",
		FilesTab:     "d",
		CommitsTab:   "m",
		CommentsTab:  "t",
		HideResolved: "f",
		NextFile:     "n",
		PrevFile:     "p",
		OpenEditor:   "e",
	}
}

// detailKeyYAMLNames are the yaml keys of every keybinding.detail field, in
// declaration order.
var detailKeyYAMLNames = []string{
	"focusLeft", "back", "scrollDown", "scrollUp", "halfPageDown", "halfPageUp",
	"overviewTab", "filesTab", "commitsTab", "commentsTab", "hideResolved",
	"nextFile", "prevFile", "openEditor",
}

func TestLoad_ReadsDetailKeybindingsFromYAML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envConfigDir, dir)
	clearBitbucketEnv(t)

	want := detailKeysFixture()
	var body strings.Builder
	body.WriteString("keybinding:\n  detail:\n")
	for _, name := range detailKeyYAMLNames {
		fmt.Fprintf(&body, "    %s: %q\n", name, detailKeyByYAMLName(t, want, name))
	}
	writeConfigFile(t, dir, body.String())

	testkit.AssertEqual(t, "Keybinding.Detail", loadOrFatal(t).Keybinding.Detail, want)
}

func TestSave_RoundTripsDetailKeybindings(t *testing.T) {
	t.Setenv(envConfigDir, t.TempDir())
	clearBitbucketEnv(t)

	cfg := DefaultConfig()
	cfg.Keybinding.Detail = detailKeysFixture()
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	written, err := os.ReadFile(ConfigPath())
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	for _, name := range detailKeyYAMLNames {
		if !strings.Contains(string(written), name+": ") {
			t.Errorf("saved config is missing the %q detail key:\n%s", name, written)
		}
	}

	testkit.AssertEqual(t, "Keybinding.Detail", loadOrFatal(t).Keybinding.Detail, detailKeysFixture())
}

// detailKeyByYAMLName maps a yaml key back to its DetailKeys field value.
func detailKeyByYAMLName(t *testing.T, keys DetailKeys, name string) string {
	t.Helper()
	switch name {
	case "focusLeft":
		return keys.FocusLeft
	case "back":
		return keys.Back
	case "scrollDown":
		return keys.ScrollDown
	case "scrollUp":
		return keys.ScrollUp
	case "halfPageDown":
		return keys.HalfPageDown
	case "halfPageUp":
		return keys.HalfPageUp
	case "overviewTab":
		return keys.OverviewTab
	case "filesTab":
		return keys.FilesTab
	case "commitsTab":
		return keys.CommitsTab
	case "commentsTab":
		return keys.CommentsTab
	case "hideResolved":
		return keys.HideResolved
	case "nextFile":
		return keys.NextFile
	case "prevFile":
		return keys.PrevFile
	case "openEditor":
		return keys.OpenEditor
	default:
		t.Fatalf("detailKeyByYAMLName: unknown yaml key %q", name)
		return ""
	}
}

// pullRequestKeysFixture is a fully-populated PullRequestKeys with a distinct
// value per field, so a wrong or duplicated yaml tag shows up as a mismatch.
func pullRequestKeysFixture() PullRequestKeys {
	return PullRequestKeys{
		Select:      "enter",
		Open:        "o",
		FocusRight:  "l",
		Browser:     "b",
		URLPicker:   "u",
		CopyURL:     "y",
		Checkout:    "c",
		Merge:       "M",
		Sync:        "S",
		Edit:        "E",
		Reviewers:   "V",
		ToggleDraft: "D",
		Approve:     "A",
		Comment:     "C",
		Create:      "N",
		Resolve:     "T",
	}
}

// pullRequestKeyYAMLNames are the yaml keys of every keybinding.pullRequests
// field, in declaration order.
var pullRequestKeyYAMLNames = []string{
	"select", "open", "focusRight", "browser", "urlPicker", "copyURL", "checkout",
	"merge", "sync", "edit", "reviewers", "toggleDraft", "approve", "comment", "create", "resolve",
}

func TestLoad_ReadsPullRequestKeybindingsFromYAML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envConfigDir, dir)
	clearBitbucketEnv(t)

	want := pullRequestKeysFixture()
	var body strings.Builder
	body.WriteString("keybinding:\n  pullRequests:\n")
	for _, name := range pullRequestKeyYAMLNames {
		fmt.Fprintf(&body, "    %s: %q\n", name, pullRequestKeyByYAMLName(t, want, name))
	}
	writeConfigFile(t, dir, body.String())

	testkit.AssertEqual(t, "Keybinding.PullRequests", loadOrFatal(t).Keybinding.PullRequests, want)
}

func TestSave_RoundTripsPullRequestKeybindings(t *testing.T) {
	t.Setenv(envConfigDir, t.TempDir())
	clearBitbucketEnv(t)

	cfg := DefaultConfig()
	cfg.Keybinding.PullRequests = pullRequestKeysFixture()
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	written, err := os.ReadFile(ConfigPath())
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	for _, name := range pullRequestKeyYAMLNames {
		if !strings.Contains(string(written), name+": ") {
			t.Errorf("saved config is missing the %q pull request key:\n%s", name, written)
		}
	}

	testkit.AssertEqual(t, "Keybinding.PullRequests",
		loadOrFatal(t).Keybinding.PullRequests, pullRequestKeysFixture())
}

func TestDefaultConfig_LeavesPullRequestKeysEmpty(t *testing.T) {
	t.Parallel()

	// The defaults live in the TUI's DefaultKeymap; the config file only carries
	// user overrides, so every key field must stay empty here.
	testkit.AssertEqual(t, "Keybinding.PullRequests", DefaultConfig().Keybinding.PullRequests, PullRequestKeys{})
}

// pullRequestKeyByYAMLName maps a yaml key back to its PullRequestKeys field value.
func pullRequestKeyByYAMLName(t *testing.T, keys PullRequestKeys, name string) string {
	t.Helper()
	switch name {
	case "select":
		return keys.Select
	case "open":
		return keys.Open
	case "focusRight":
		return keys.FocusRight
	case "browser":
		return keys.Browser
	case "urlPicker":
		return keys.URLPicker
	case "copyURL":
		return keys.CopyURL
	case "checkout":
		return keys.Checkout
	case "merge":
		return keys.Merge
	case "sync":
		return keys.Sync
	case "edit":
		return keys.Edit
	case "reviewers":
		return keys.Reviewers
	case "toggleDraft":
		return keys.ToggleDraft
	case "approve":
		return keys.Approve
	case "comment":
		return keys.Comment
	case "create":
		return keys.Create
	case "resolve":
		return keys.Resolve
	default:
		t.Fatalf("pullRequestKeyByYAMLName: unknown yaml key %q", name)
		return ""
	}
}

// buildsKeysFixture is a fully-populated BuildsKeys with a distinct value per
// field, so a wrong or duplicated yaml tag shows up as a mismatch.
func buildsKeysFixture() BuildsKeys {
	return BuildsKeys{
		Rerun: "P",
		Stop:  "X",
	}
}

// buildsKeyYAMLNames are the yaml keys of every keybinding.builds field, in
// declaration order.
var buildsKeyYAMLNames = []string{"rerun", "stop"}

func TestLoad_ReadsBuildsKeybindingsFromYAML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envConfigDir, dir)
	clearBitbucketEnv(t)

	want := buildsKeysFixture()
	var body strings.Builder
	body.WriteString("keybinding:\n  builds:\n")
	for _, name := range buildsKeyYAMLNames {
		fmt.Fprintf(&body, "    %s: %q\n", name, buildsKeyByYAMLName(t, want, name))
	}
	writeConfigFile(t, dir, body.String())

	testkit.AssertEqual(t, "Keybinding.Builds", loadOrFatal(t).Keybinding.Builds, want)
}

func TestSave_RoundTripsBuildsKeybindings(t *testing.T) {
	t.Setenv(envConfigDir, t.TempDir())
	clearBitbucketEnv(t)

	cfg := DefaultConfig()
	cfg.Keybinding.Builds = buildsKeysFixture()
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	written, err := os.ReadFile(ConfigPath())
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	if !strings.Contains(string(written), "builds:") {
		t.Errorf("saved config is missing the builds key group:\n%s", written)
	}
	for _, name := range buildsKeyYAMLNames {
		if !strings.Contains(string(written), name+": ") {
			t.Errorf("saved config is missing the %q builds key:\n%s", name, written)
		}
	}

	testkit.AssertEqual(t, "Keybinding.Builds", loadOrFatal(t).Keybinding.Builds, buildsKeysFixture())
}

func TestDefaultConfig_LeavesBuildsKeysEmpty(t *testing.T) {
	t.Parallel()

	// Like the pull request keys, the pipeline action defaults (P and X) live in
	// the TUI's DefaultKeymap; the config file only carries user overrides.
	testkit.AssertEqual(t, "Keybinding.Builds", DefaultConfig().Keybinding.Builds, BuildsKeys{})
}

// buildsKeyByYAMLName maps a yaml key back to its BuildsKeys field value.
func buildsKeyByYAMLName(t *testing.T, keys BuildsKeys, name string) string {
	t.Helper()
	switch name {
	case "rerun":
		return keys.Rerun
	case "stop":
		return keys.Stop
	default:
		t.Fatalf("buildsKeyByYAMLName: unknown yaml key %q", name)
		return ""
	}
}

// TestLoad_BuildsKeysDoNotCollideWithPullRequestKeys guards the reason
// KeybindingConfig has a `builds` group at all: it must be a separate yaml
// mapping that leaves the pull request bindings untouched.
func TestLoad_BuildsKeysDoNotCollideWithPullRequestKeys(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envConfigDir, dir)
	clearBitbucketEnv(t)
	writeConfigFile(t, dir, "keybinding:\n"+
		"  pullRequests:\n    select: \" \"\n    merge: m\n"+
		"  builds:\n    rerun: r\n    stop: s\n")

	cfg := loadOrFatal(t)
	testkit.AssertEqual(t, "Keybinding.PullRequests.Select", cfg.Keybinding.PullRequests.Select, " ")
	testkit.AssertEqual(t, "Keybinding.PullRequests.Merge", cfg.Keybinding.PullRequests.Merge, "m")
	testkit.AssertEqual(t, "Keybinding.Builds", cfg.Keybinding.Builds, BuildsKeys{Rerun: "r", Stop: "s"})
}

// pipelineKeysFixture is a fully-populated PipelineKeys with a distinct value
// per field, so a wrong or duplicated yaml tag shows up as a mismatch.
func pipelineKeysFixture() PipelineKeys {
	return PipelineKeys{
		OpenPullRequest: "L",
	}
}

// pipelineKeyYAMLNames are the yaml keys of every keybinding.pipelines field,
// in declaration order.
var pipelineKeyYAMLNames = []string{"openPullRequest"}

// pipelineKeyByYAMLName maps a yaml key back to its PipelineKeys field value.
func pipelineKeyByYAMLName(t *testing.T, keys PipelineKeys, name string) string {
	t.Helper()
	switch name {
	case "openPullRequest":
		return keys.OpenPullRequest
	default:
		t.Fatalf("pipelineKeyByYAMLName: unknown yaml key %q", name)
		return ""
	}
}

func TestLoad_ReadsPipelineKeybindingsFromYAML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envConfigDir, dir)
	clearBitbucketEnv(t)

	want := pipelineKeysFixture()
	var body strings.Builder
	body.WriteString("keybinding:\n  pipelines:\n")
	for _, name := range pipelineKeyYAMLNames {
		fmt.Fprintf(&body, "    %s: %q\n", name, pipelineKeyByYAMLName(t, want, name))
	}
	writeConfigFile(t, dir, body.String())

	testkit.AssertEqual(t, "Keybinding.Pipelines", loadOrFatal(t).Keybinding.Pipelines, want)
}

func TestSave_RoundTripsPipelineKeybindings(t *testing.T) {
	t.Setenv(envConfigDir, t.TempDir())
	clearBitbucketEnv(t)

	cfg := DefaultConfig()
	cfg.Keybinding.Pipelines = pipelineKeysFixture()
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	written, err := os.ReadFile(ConfigPath())
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	if !strings.Contains(string(written), "pipelines:") {
		t.Errorf("saved config is missing the pipelines key group:\n%s", written)
	}
	for _, name := range pipelineKeyYAMLNames {
		if !strings.Contains(string(written), name+": ") {
			t.Errorf("saved config is missing the %q pipelines key:\n%s", name, written)
		}
	}

	testkit.AssertEqual(t, "Keybinding.Pipelines", loadOrFatal(t).Keybinding.Pipelines, pipelineKeysFixture())
}

func TestDefaultConfig_LeavesPipelineKeysEmpty(t *testing.T) {
	t.Parallel()

	// Like the builds keys, the L default lives in the TUI's DefaultKeymap; the
	// config file only carries user overrides.
	testkit.AssertEqual(t, "Keybinding.Pipelines", DefaultConfig().Keybinding.Pipelines, PipelineKeys{})
}

// TestLoad_PipelineKeysDoNotCollideWithBuildsKeys guards the split the spec
// fixes in place: rerun and stop stay under `builds` and the pipelines group
// carries only what no other panel binds, so neither can shadow the other.
func TestLoad_PipelineKeysDoNotCollideWithBuildsKeys(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envConfigDir, dir)
	clearBitbucketEnv(t)
	writeConfigFile(t, dir, "keybinding:\n"+
		"  builds:\n    rerun: r\n    stop: s\n"+
		"  pipelines:\n    openPullRequest: o\n")

	cfg := loadOrFatal(t)
	testkit.AssertEqual(t, "Keybinding.Builds", cfg.Keybinding.Builds, BuildsKeys{Rerun: "r", Stop: "s"})
	testkit.AssertEqual(t, "Keybinding.Pipelines", cfg.Keybinding.Pipelines,
		PipelineKeys{OpenPullRequest: "o"})
}

func TestLoad_ReadsNextViewFromYAML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envConfigDir, dir)
	clearBitbucketEnv(t)
	writeConfigFile(t, dir, "keybinding:\n  universal:\n    nextView: ctrl+v\n")

	testkit.AssertEqual(t, "Keybinding.Universal.NextView",
		loadOrFatal(t).Keybinding.Universal.NextView, "ctrl+v")
}

func TestSave_RoundTripsNextView(t *testing.T) {
	t.Setenv(envConfigDir, t.TempDir())
	clearBitbucketEnv(t)

	cfg := DefaultConfig()
	cfg.Keybinding.Universal.NextView = "ctrl+v"
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	written, err := os.ReadFile(ConfigPath())
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	if !strings.Contains(string(written), "nextView: ") {
		t.Errorf("saved config is missing the universal nextView key:\n%s", written)
	}

	testkit.AssertEqual(t, "Keybinding.Universal.NextView",
		loadOrFatal(t).Keybinding.Universal.NextView, "ctrl+v")
}

func TestDefaultConfig_LeavesNextViewEmpty(t *testing.T) {
	t.Parallel()

	// shift+tab is the TUI's default; an empty config value means "no override".
	testkit.AssertEqual(t, "Keybinding.Universal.NextView",
		DefaultConfig().Keybinding.Universal.NextView, "")
}

func TestLoad_ReadsStartViewFromYAML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envConfigDir, dir)
	clearBitbucketEnv(t)
	writeConfigFile(t, dir, "gui:\n  startView: "+StartViewPipelines+"\n")

	testkit.AssertEqual(t, "GUI.StartView", loadOrFatal(t).GUI.StartView, StartViewPipelines)
}

func TestSave_RoundTripsStartView(t *testing.T) {
	t.Setenv(envConfigDir, t.TempDir())
	clearBitbucketEnv(t)

	cfg := DefaultConfig()
	cfg.GUI.StartView = StartViewPipelines
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	written, err := os.ReadFile(ConfigPath())
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	if !strings.Contains(string(written), "startView: ") {
		t.Errorf("saved config is missing the gui startView key:\n%s", written)
	}

	testkit.AssertEqual(t, "GUI.StartView", loadOrFatal(t).GUI.StartView, StartViewPipelines)
}

func TestDefaultConfig_LeavesStartViewEmpty(t *testing.T) {
	t.Parallel()

	// An empty value means StartViewPullRequests; the app resolves it, so the
	// default config does not need to spell it out.
	testkit.AssertEqual(t, "GUI.StartView", DefaultConfig().GUI.StartView, "")
}

func TestDefaultConfig_ReadOnlyIsOff(t *testing.T) {
	t.Parallel()

	testkit.AssertEqual(t, "Bitbucket.ReadOnly", DefaultConfig().Bitbucket.ReadOnly, false)
}

func TestLoad_ReadsReadOnlyFromYAML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envConfigDir, dir)
	clearBitbucketEnv(t)
	writeConfigFile(t, dir, "bitbucket:\n  workspace: "+testWorkspace+"\n  readOnly: true\n")

	cfg := loadOrFatal(t)
	testkit.AssertEqual(t, "Bitbucket.Workspace", cfg.Bitbucket.Workspace, testWorkspace)
	testkit.AssertEqual(t, "Bitbucket.ReadOnly", cfg.Bitbucket.ReadOnly, true)
}

func TestSave_RoundTripsReadOnly(t *testing.T) {
	t.Setenv(envConfigDir, t.TempDir())
	clearBitbucketEnv(t)

	cfg := DefaultConfig()
	cfg.Bitbucket.ReadOnly = true
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	written, err := os.ReadFile(ConfigPath())
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	if !strings.Contains(string(written), "readOnly: true") {
		t.Errorf("saved config is missing readOnly:\n%s", written)
	}

	testkit.AssertEqual(t, "Bitbucket.ReadOnly", loadOrFatal(t).Bitbucket.ReadOnly, true)
}

func TestLoad_ReadOnlyEnvOverride(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"unset", "", false},
		{"one", "1", true},
		{"true", "true", true},
		{"mixed case true", "TrUe", true},
		{"yes", "YES", true},
		{"padded", "  true  ", true},
		{"zero leaves it off", "0", false},
		{"false leaves it off", "false", false},
		{"no leaves it off", "no", false},
		{"garbage leaves it off", "maybe", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv(envConfigDir, dir)
			clearBitbucketEnv(t)
			t.Setenv(envReadOnly, tc.value)

			testkit.AssertEqual(t, "Bitbucket.ReadOnly", loadOrFatal(t).Bitbucket.ReadOnly, tc.want)
		})
	}
}

func TestLoad_ReadOnlyEnvNeverTurnsTheConfigFileOff(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"empty", ""},
		{"zero", "0"},
		{"false", "false"},
		{"no", "no"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv(envConfigDir, dir)
			clearBitbucketEnv(t)
			t.Setenv(envReadOnly, tc.value)
			writeConfigFile(t, dir, "bitbucket:\n  readOnly: true\n")

			testkit.AssertEqual(t, "Bitbucket.ReadOnly", loadOrFatal(t).Bitbucket.ReadOnly, true)
		})
	}
}

func TestLoad_ReadOnlyEnvWinsOverAFileThatLeavesItOff(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envConfigDir, dir)
	clearBitbucketEnv(t)
	t.Setenv(envReadOnly, "1")
	writeConfigFile(t, dir, "bitbucket:\n  readOnly: false\n")

	testkit.AssertEqual(t, "Bitbucket.ReadOnly", loadOrFatal(t).Bitbucket.ReadOnly, true)
}

func TestValidateEditorInTmux(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"empty defaults to window", "", false},
		{"window", EditorInTmuxWindow, false},
		{"inline", EditorInTmuxInline, false},
		{"unknown value errors", "split", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateEditorInTmux(tc.value)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("validateEditorInTmux(%q) = nil, want error", tc.value)
				}
				if !strings.Contains(err.Error(), tc.value) {
					t.Errorf("error %q should include the bad value %q", err, tc.value)
				}
				return
			}
			if err != nil {
				t.Errorf("validateEditorInTmux(%q) = %v, want nil", tc.value, err)
			}
		})
	}
}

func TestEditorOpensTmuxWindow(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{"": true, EditorInTmuxWindow: true, EditorInTmuxInline: false}
	for value, want := range cases {
		cfg := &Config{EditorInTmux: value}
		testkit.AssertEqual(t, "EditorOpensTmuxWindow("+value+")", cfg.EditorOpensTmuxWindow(), want)
	}
}

func TestValidateScreenMode(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"", ScreenModeNormal, ScreenModeHalf, ScreenModeFull} {
		if err := validateScreenMode(value); err != nil {
			t.Errorf("validateScreenMode(%q) = %v, want nil", value, err)
		}
	}
	err := validateScreenMode("wide")
	if err == nil || !strings.Contains(err.Error(), "wide") {
		t.Errorf("validateScreenMode(wide) = %v, want an error naming the value", err)
	}
}

func TestValidateStartView(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"", StartViewPullRequests, StartViewPipelines} {
		if err := validateStartView(value); err != nil {
			t.Errorf("validateStartView(%q) = %v, want nil", value, err)
		}
	}
	err := validateStartView("builds")
	if err == nil || !strings.Contains(err.Error(), "builds") {
		t.Errorf("validateStartView(builds) = %v, want an error naming the value", err)
	}
}
