package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Valid values for Config.RendererStyle. Empty string is treated as
// RendererStyleAuto. Selects the glamour theme used to render Markdown
// (pull request descriptions, comments).
const (
	RendererStyleAuto  = "auto"
	RendererStyleDark  = "dark"
	RendererStyleLight = "light"
	RendererStyleNoTTY = "notty"
)

func validateRendererStyle(value string) error {
	switch value {
	case "", RendererStyleAuto, RendererStyleDark, RendererStyleLight, RendererStyleNoTTY:
		return nil
	default:
		return fmt.Errorf("unknown rendererStyle %q; valid: %q (default), %q, %q, %q",
			value, RendererStyleAuto, RendererStyleDark, RendererStyleLight, RendererStyleNoTTY)
	}
}

// DefaultBaseURL is the Bitbucket Cloud REST base URL used when
// bitbucket.baseUrl is unset. It mirrors bitbucket.DefaultBaseURL; the value
// is duplicated so pkg/config stays independent of the API client.
const DefaultBaseURL = "https://api.bitbucket.org/2.0"

// appName is the directory name used under the user config dir.
const appName = "lazybitbucket"

// DefaultEditor is used when neither `editor`, $VISUAL nor $EDITOR is set.
const DefaultEditor = "nvim"

// DefaultRefreshInterval is the auto-refresh period applied when
// `refreshInterval` is unset.
const DefaultRefreshInterval = "120s"

// refreshOffKeyword disables auto-refresh when used as `refreshInterval`.
const refreshOffKeyword = "off"

type Config struct {
	Bitbucket       BitbucketConfig        `yaml:"bitbucket"`
	GUI             GUIConfig              `yaml:"gui"`
	Keybinding      KeybindingConfig       `yaml:"keybinding"`
	PullRequestTabs []PullRequestTabConfig `yaml:"pullRequestTabs"`
	MaxResults      *int                   `yaml:"maxResults"`
	RefreshInterval string                 `yaml:"refreshInterval"`
	Git             GitConfig              `yaml:"git"`
	CustomCommands  []CustomCommandConfig  `yaml:"customCommands"`
	// Editor is the command the diff view's open-in-editor key runs to open the
	// selected file at the cursor's line. Resolved through ResolveEditor, which
	// falls back to $VISUAL, $EDITOR and DefaultEditor.
	Editor        string `yaml:"editor"`
	RendererStyle string `yaml:"rendererStyle"`
}

// BitbucketConfig holds the connection settings. Token is never persisted to
// config.yml (yaml:"-"); it comes from the environment or auth.json.
type BitbucketConfig struct {
	Workspace    string   `yaml:"workspace"`
	Repositories []string `yaml:"repositories"`
	Email        string   `yaml:"email"`
	Token        string   `yaml:"-"`
	BaseURL      string   `yaml:"baseUrl"`
	// ReadOnly refuses every request that would modify Bitbucket: the API
	// client fails a non-GET request with bitbucket.ErrReadOnly before any
	// socket is opened, and the TUI hides and refuses the write keys. It is
	// browse-only mode, so it also blocks the sync action (which pushes).
	// Turned on by `bitbucket.readOnly: true`, the --read-only flag or
	// LAZYBITBUCKET_READ_ONLY=1|true|yes; none of them can turn it off.
	ReadOnly bool `yaml:"readOnly"`
}

type CustomCommandConfig struct {
	Key      string   `yaml:"key"`
	Name     string   `yaml:"name"`
	Command  string   `yaml:"command"`
	Suspend  *bool    `yaml:"suspend,omitempty"` // default: true
	Refresh  bool     `yaml:"refresh,omitempty"` // default: false
	Contexts []string `yaml:"contexts,omitempty"`
}

// ShouldSuspend returns true when the TUI should be suspended for this command (default: true)
func (c CustomCommandConfig) ShouldSuspend() bool {
	return c.Suspend == nil || *c.Suspend
}

type GitConfig struct {
	CloseOnCheckout bool `yaml:"closeOnCheckout"`
}

// Pull request states accepted by a tab's `state:` key.
const (
	stateOpen       = "OPEN"
	stateMerged     = "MERGED"
	stateDeclined   = "DECLINED"
	stateSuperseded = "SUPERSEDED"
)

var pullRequestStates = []string{stateOpen, stateMerged, stateDeclined, stateSuperseded}

// Field names accepted in gui.pullRequestListFields.
const (
	FieldRepo      = "repo"
	FieldID        = "id"
	FieldTitle     = "title"
	FieldAuthor    = "author"
	FieldApprovals = "approvals"
	FieldBuild     = "build"
	FieldUpdated   = "updated"
	FieldBranch    = "branch"
	FieldState     = "state"
)

var pullRequestListFields = []string{
	FieldRepo, FieldID, FieldTitle, FieldAuthor, FieldApprovals,
	FieldBuild, FieldUpdated, FieldBranch, FieldState,
}

// PullRequestListFieldNames returns every field name accepted in
// gui.pullRequestListFields, in canonical order.
func PullRequestListFieldNames() []string {
	return slices.Clone(pullRequestListFields)
}

// DefaultPullRequestListFields returns the columns shown when
// gui.pullRequestListFields is unset.
func DefaultPullRequestListFields() []string {
	// Order matters: a narrow pull request list drops columns from the end, so
	// the approval and build signals sit ahead of author and updated (see the
	// narrow-width rule in pkg/tui/views).
	return []string{FieldRepo, FieldID, FieldTitle, FieldApprovals, FieldBuild, FieldAuthor, FieldUpdated}
}

// PullRequestTabConfig is one tab above the pull request list. State selects
// the Bitbucket PR state to query; Filter narrows the result client-side.
type PullRequestTabConfig struct {
	Name       string `yaml:"name"`
	State      string `yaml:"state"`
	Filter     string `yaml:"filter"`
	Query      string `yaml:"query"`
	MaxResults *int   `yaml:"maxResults"`
}

// ResolveState returns the uppercased tab state, defaulting to OPEN.
func (t PullRequestTabConfig) ResolveState() string {
	if state := strings.ToUpper(strings.TrimSpace(t.State)); state != "" {
		return state
	}
	return stateOpen
}

// DefaultPullRequestTabs returns the default pull request tab configuration
func DefaultPullRequestTabs() []PullRequestTabConfig {
	return []PullRequestTabConfig{
		{Name: "Open", Filter: "all"},
		{Name: "Mine", Filter: "mine"},
		{Name: "Reviewing", Filter: "reviewing"},
	}
}

// KeybindingConfig groups the user-overridable keys by panel.
//
// There is deliberately no `repos` key group: the keymap is keyed by action and
// the Repos and Builds panels reuse the select/open/focus-right actions from
// `pullRequests`. A separate group would silently overwrite the pull request
// bindings — the dead-config bug lazyjira had with `projects`.
type KeybindingConfig struct {
	Universal    UniversalKeys   `yaml:"universal"`
	Navigation   NavigationKeys  `yaml:"navigation"`
	PullRequests PullRequestKeys `yaml:"pullRequests"`
	Detail       DetailKeys      `yaml:"detail"`
}

type NavigationKeys struct {
	Down     string `yaml:"down"`
	Up       string `yaml:"up"`
	Top      string `yaml:"top"`
	Bottom   string `yaml:"bottom"`
	HalfDown string `yaml:"halfPageDown"`
	HalfUp   string `yaml:"halfPageUp"`
}

type UniversalKeys struct {
	Quit              string `yaml:"quit"`
	Help              string `yaml:"help"`
	Search            string `yaml:"search"`
	SwitchPanel       string `yaml:"switchPanel"`
	Refresh           string `yaml:"refresh"`
	RefreshAll        string `yaml:"refreshAll"`
	PrevTab           string `yaml:"prevTab"`
	NextTab           string `yaml:"nextTab"`
	FocusDetail       string `yaml:"focusDetail"`
	FocusStatus       string `yaml:"focusStatus"`
	FocusPullRequests string `yaml:"focusPullRequests"`
	FocusBuilds       string `yaml:"focusBuilds"`
	FocusRepos        string `yaml:"focusRepos"`
}

// PullRequestKeys are the keys active with a pull request selected, in the
// pull request list or the detail panel. The five write actions at the bottom
// are refused in read-only mode (BitbucketConfig.ReadOnly).
type PullRequestKeys struct {
	Select     string `yaml:"select"`
	Open       string `yaml:"open"`
	FocusRight string `yaml:"focusRight"`
	Browser    string `yaml:"browser"`
	URLPicker  string `yaml:"urlPicker"`
	CopyURL    string `yaml:"copyURL"`
	Checkout   string `yaml:"checkout"`
	// Merge opens the merge dialog (strategy, close source branch, message).
	// Default: M.
	Merge string `yaml:"merge"`
	// Sync opens the sync-with-target-branch dialog, which runs local git.
	// Default: S.
	Sync string `yaml:"sync"`
	// Edit opens the title/description chooser. Default: E.
	Edit string `yaml:"edit"`
	// Reviewers opens the reviewer picker. Default: V.
	Reviewers string `yaml:"reviewers"`
	// ToggleDraft flips the pull request between draft and ready for review,
	// after a confirmation. Default: D.
	ToggleDraft string `yaml:"toggleDraft"`
}

// DetailKeys are the keys active while the detail panel is focused: panel
// navigation, the four pull request tabs (Overview, Files, Commits, Comments)
// and the diff view's file stepping and open-in-editor keys.
type DetailKeys struct {
	FocusLeft    string `yaml:"focusLeft"`
	Back         string `yaml:"back"`
	ScrollDown   string `yaml:"scrollDown"`
	ScrollUp     string `yaml:"scrollUp"`
	HalfPageDown string `yaml:"halfPageDown"`
	HalfPageUp   string `yaml:"halfPageUp"`
	// Tab jumps: switch the detail panel straight to one pull request tab.
	OverviewTab string `yaml:"overviewTab"`
	FilesTab    string `yaml:"filesTab"`
	CommitsTab  string `yaml:"commitsTab"`
	CommentsTab string `yaml:"commentsTab"`
	// Diff view: step through the changed files and open the selected file in
	// Config.Editor at the cursor's line.
	NextFile   string `yaml:"nextFile"`
	PrevFile   string `yaml:"prevFile"`
	OpenEditor string `yaml:"openEditor"`
}

type GUIConfig struct {
	Theme                 string            `yaml:"theme"`
	ThemeColors           map[string]string `yaml:"themeColors,omitempty"`
	ThemeDark             map[string]string `yaml:"themeDark,omitempty"`
	ThemeLight            map[string]string `yaml:"themeLight,omitempty"`
	SidePanelWidth        int               `yaml:"sidePanelWidth"`
	CollapsedPanelHeight  int               `yaml:"collapsedPanelHeight"`
	PullRequestListFields []string          `yaml:"pullRequestListFields"`
}

// DefaultConfig returns a Config populated with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		GUI: GUIConfig{
			Theme:                 "default",
			SidePanelWidth:        40,
			CollapsedPanelHeight:  5,
			PullRequestListFields: DefaultPullRequestListFields(),
		},
		PullRequestTabs: DefaultPullRequestTabs(),
		RefreshInterval: DefaultRefreshInterval,
	}
}

// DefaultMaxResults is the fallback page size used when neither the global
// `maxResults` nor a tab-specific override is set. It applies per repository
// per tab; Bitbucket caps the pull request page size at 50.
const DefaultMaxResults = 50

// ResolveGlobalMaxResults returns the effective page size for queries that are
// not tied to a configured tab: the global config value if set and positive,
// otherwise the compile-time default. A nil or non-positive pointer is treated
// as "unset".
func (c *Config) ResolveGlobalMaxResults() int {
	if c.MaxResults != nil && *c.MaxResults > 0 {
		return *c.MaxResults
	}
	return DefaultMaxResults
}

// ResolveMaxResults returns the effective page size for a given tab: per-tab
// override first, then the global/default chain. A nil or non-positive pointer
// is treated as "unset".
func (c *Config) ResolveMaxResults(tab PullRequestTabConfig) int {
	if tab.MaxResults != nil && *tab.MaxResults > 0 {
		return *tab.MaxResults
	}
	return c.ResolveGlobalMaxResults()
}

// ResolveRefreshInterval returns the auto-refresh period. An unset value means
// DefaultRefreshInterval; "0" and "off" disable auto-refresh (zero duration);
// anything time.ParseDuration rejects, and negative durations, are an error
// (surfaced by Load).
func (c *Config) ResolveRefreshInterval() (time.Duration, error) {
	value := strings.TrimSpace(c.RefreshInterval)
	if value == "" {
		value = DefaultRefreshInterval
	}
	if value == "0" || strings.EqualFold(value, refreshOffKeyword) {
		return 0, nil
	}

	interval, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid refreshInterval %q: %w", c.RefreshInterval, err)
	}
	if interval < 0 {
		return 0, fmt.Errorf("invalid refreshInterval %q: must not be negative", c.RefreshInterval)
	}
	return interval, nil
}

// ResolveEditor returns the editor command used by the diff view's
// open-in-editor key: `editor` from the config file, then $VISUAL, then
// $EDITOR, then DefaultEditor.
func (c *Config) ResolveEditor() string {
	if c.Editor != "" {
		return c.Editor
	}
	if editor := os.Getenv("VISUAL"); editor != "" {
		return editor
	}
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor
	}
	return DefaultEditor
}

// ResolveBaseURL returns the Bitbucket REST base URL without a trailing slash,
// falling back to DefaultBaseURL.
func (c *Config) ResolveBaseURL() string {
	if base := strings.TrimRight(strings.TrimSpace(c.Bitbucket.BaseURL), "/"); base != "" {
		return base
	}
	return DefaultBaseURL
}

// ConfigDir returns the lazybitbucket configuration directory path.
// Order of precedence: LAZYBITBUCKET_CONFIG_DIR env, XDG_CONFIG_HOME/lazybitbucket,
// os.UserConfigDir()/lazybitbucket, ~/.config/lazybitbucket
func ConfigDir() string {
	if dir := os.Getenv("LAZYBITBUCKET_CONFIG_DIR"); dir != "" {
		return dir
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, appName)
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, appName)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".config", appName)
}

// ConfigPath returns the full path to the config file
func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.yml")
}

// Load reads the config file, merges it with defaults, applies the BITBUCKET_*
// environment overrides and validates the result.
func Load() (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(ConfigPath())
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	if err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	}

	applyEnvOverrides(cfg)

	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// applyEnvOverrides applies the BITBUCKET_* environment variables, which
// always take precedence over the config file.
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("BITBUCKET_WORKSPACE"); v != "" {
		cfg.Bitbucket.Workspace = v
	}
	if repos := splitRepositories(os.Getenv("BITBUCKET_REPOSITORIES")); len(repos) > 0 {
		cfg.Bitbucket.Repositories = repos
	}
	if v := os.Getenv("BITBUCKET_EMAIL"); v != "" {
		cfg.Bitbucket.Email = v
	}
	if v := os.Getenv("BITBUCKET_TOKEN"); v != "" {
		cfg.Bitbucket.Token = v
	}
	if v := os.Getenv("BITBUCKET_API_URL"); v != "" {
		cfg.Bitbucket.BaseURL = v
	}
	if truthyEnv(os.Getenv("LAZYBITBUCKET_READ_ONLY")) {
		cfg.Bitbucket.ReadOnly = true
	}
}

// truthyEnv reports whether an environment variable asks for a boolean switch
// to be turned on. Anything else — including "0", "false", "no" and an unset
// variable — leaves the config value alone: the read-only switch can only be
// turned on from the environment, never off.
func truthyEnv(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

// splitRepositories parses a comma-separated repository slug list, trimming
// whitespace and dropping empty entries.
func splitRepositories(value string) []string {
	parts := strings.Split(value, ",")
	repos := make([]string, 0, len(parts))
	for _, part := range parts {
		if slug := strings.TrimSpace(part); slug != "" {
			repos = append(repos, slug)
		}
	}
	return repos
}

func validate(cfg *Config) error {
	if _, err := cfg.ResolveCustomCommands(); err != nil {
		return err
	}
	if err := validateRendererStyle(cfg.RendererStyle); err != nil {
		return err
	}
	if _, err := cfg.ResolveRefreshInterval(); err != nil {
		return err
	}
	for i, tab := range cfg.PullRequestTabs {
		if err := validateTabState(i, tab); err != nil {
			return err
		}
	}
	return validateListFields(cfg.GUI.PullRequestListFields)
}

func validateTabState(index int, tab PullRequestTabConfig) error {
	if slices.Contains(pullRequestStates, tab.ResolveState()) {
		return nil
	}
	return fmt.Errorf("pullRequestTabs[%d] (%q): unknown state %q; valid: %s",
		index, tab.Name, tab.State, strings.Join(pullRequestStates, ", "))
}

func validateListFields(fields []string) error {
	for _, field := range fields {
		if !slices.Contains(pullRequestListFields, field) {
			return fmt.Errorf("gui.pullRequestListFields: unknown field %q; valid: %s",
				field, strings.Join(pullRequestListFields, ", "))
		}
	}
	return nil
}

// Save writes the config to the config file. The API token is never persisted
// because the Token field carries the yaml:"-" tag
func Save(cfg *Config) error {
	if cfg == nil {
		return errors.New("save config: nil config")
	}

	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(ConfigPath(), data, 0o644)
}
