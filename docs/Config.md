# Configuration

lazybitbucket is configured through a YAML file.

## Config file location

| OS | Path |
|----|------|
| Linux | `~/.config/lazybitbucket/config.yml` |
| macOS | `~/Library/Application Support/lazybitbucket/config.yml` |
| Windows | `%AppData%\lazybitbucket\config.yml` |

You can override the config directory with the `LAZYBITBUCKET_CONFIG_DIR` environment variable. `XDG_CONFIG_HOME` is also respected on Linux.

Resolution order: `LAZYBITBUCKET_CONFIG_DIR` → `$XDG_CONFIG_HOME/lazybitbucket` → the OS user config dir → `~/.config/lazybitbucket`.

## Environment variables

These always take priority over the config file and `auth.json`.

| Variable | Description |
|----------|-------------|
| `BITBUCKET_WORKSPACE` | Workspace slug |
| `BITBUCKET_REPOSITORIES` | Comma-separated repository slugs |
| `BITBUCKET_EMAIL` | Atlassian account email. Setting it together with a token switches auth to HTTP Basic |
| `BITBUCKET_TOKEN` | API token or access token. Sent as `Authorization: Bearer` unless an email is also set |
| `BITBUCKET_API_URL` | API base URL. Defaults to `https://api.bitbucket.org/2.0` |
| `LAZYBITBUCKET_CONFIG_DIR` | Override the config directory |

The token is never read from or written to `config.yml`. It lives in `auth.json` or in the environment.

## Default config

Do not copy the entire thing into your config. Only add the settings you want to change.

```yaml
bitbucket:
    workspace: ""
    repositories: []
    email: ""
    baseUrl: ""       # "" → https://api.bitbucket.org/2.0
gui:
    theme: default
    themeColors: {}   # optional palette overrides (see Themes section)
    themeDark: {}     # applied on dark presets only
    themeLight: {}    # applied on light presets only
    sidePanelWidth: 40
    collapsedPanelHeight: 5
    pullRequestListFields:   # default order; columns drop from the end on narrow lists
        - repo
        - id
        - title
        - approvals
        - build
        - author
        - updated
keybinding:
    universal:
        quit: q
        help: '?'
        search: /
        switchPanel: tab
        refresh: r
        refreshAll: R
        prevTab: '['
        nextTab: ']'
        focusDetail: "0"
        focusStatus: "1"
        focusPullRequests: "2"
        focusBuilds: "3"
        focusRepos: "4"
    navigation:
        down: j
        up: k
        top: g
        bottom: G
        halfPageDown: ctrl+d
        halfPageUp: ctrl+u
    pullRequests:
        select: ' '
        open: enter
        focusRight: l
        browser: o
        urlPicker: u
        copyURL: "y"
        checkout: c
    # There is no `repos` group: the Repos and Builds panels reuse
    # pullRequests.select, pullRequests.open and pullRequests.focusRight.
    detail:
        focusLeft: h
        back: backspace
        scrollDown: J
        scrollUp: K
        halfPageDown: ctrl+f
        halfPageUp: ctrl+b
        overviewTab: i
        filesTab: d
        commitsTab: m
        commentsTab: t
        nextFile: n
        prevFile: p
        openEditor: e
pullRequestTabs:
    - name: Open
      filter: all
    - name: Mine
      filter: mine
    - name: Reviewing
      filter: reviewing
# maxResults: 50        # unset by default; 50 is the built-in fallback
refreshInterval: 120s   # "0" or "off" disables the background refresh
git:
    closeOnCheckout: false
customCommands: []
editor: ""              # "" → $VISUAL, then $EDITOR, then nvim
rendererStyle: auto     # auto | dark | light | notty
```

## Bitbucket

```yaml
bitbucket:
  workspace: acme
  repositories:
    - api-gateway
    - web-app
    - infra
  email: you@example.com   # only when you authenticate with Basic auth
```

| Field | Description |
|-------|-------------|
| `workspace` | Workspace slug. Required. Also settable with `BITBUCKET_WORKSPACE` or `--workspace` |
| `repositories` | Repository slugs to fetch pull requests from. When empty, the first 25 repositories you are a member of are discovered, most recently updated first; the status panel warns when that cap is hit, and says `no repositories visible to this token` when discovery comes back empty (which is what an access token sees) |
| `email` | Atlassian account email. Present only for the Basic auth option; with a token and no email, the token is sent as `Authorization: Bearer` |
| `baseUrl` | API base URL. Only useful for testing against a stand-in server |

There is no `token` key. App passwords no longer exist; use an API token with scopes and let the setup wizard store it in `auth.json`, or pass `BITBUCKET_TOKEN`. See the [Setup section of the README](../README.md#setup) for the scopes.

## GUI

```yaml
gui:
  sidePanelWidth: 40
  collapsedPanelHeight: 5
  pullRequestListFields:
    - repo
    - id
    - title
    - build
```

`sidePanelWidth` controls the left panel width in columns. It automatically shrinks on narrow terminals.

`collapsedPanelHeight` sets the height of non-focused left panels in lines (default 5, minimum 3).

`theme` selects the color palette. Supported values: `default` (ANSI 16, original look), `auto`, [Catppuccin](https://github.com/catppuccin/catppuccin) presets (`catppuccin-latte`, `catppuccin-frappe`, `catppuccin-macchiato`, `catppuccin-mocha`). Omitting the `theme` key or setting it to `""` keeps the `default` ANSI 16 palette. Use `theme: auto` to opt into runtime detection: lazybitbucket inspects your terminal background and picks `catppuccin-mocha` (dark) or `catppuccin-latte` (light). An unknown theme name is an error. Hex-based themes require a terminal with truecolor support.

```yaml
gui:
  theme: catppuccin-mocha
```

### Pull request list fields

Controls which columns appear in the pull request list, in the order you list them. An unknown field name is an error at startup that names the offending field.

| Field | Width | Description |
|-------|-------|-------------|
| `repo` | widest slug in view, capped at a quarter of the list width (minimum 8) | Repository slug |
| `id` | widest id in view | Pull request id, like `#101` |
| `title` | fills whatever is left, floor of 5 | Title, prefixed with `[draft]` for draft PRs |
| `author` | 12 chars | Author display name |
| `approvals` | 5 chars | `✓N` approved, `✗N` changes requested, `·` none, `?` review data not loaded yet |
| `build` | 1 char | `✔` successful, `✘` failed, `●` in progress, `■` stopped, blank when unknown |
| `updated` | 8 chars | Time since the last update |
| `branch` | 20 chars | Source branch |
| `state` | 8 chars | `OPEN`, `MERGED`, `DECLINED`, `SUPERSEDED` |

`title` is the flexible column: it takes whatever width the fixed columns leave. When the list is too narrow, columns are dropped **from the end of your list** — never the title — until the title has at least 12 columns. Put the signals you care most about early in the list; the defaults order them `repo, id, title, approvals, build, author, updated` so approvals and the build glyph survive the side panel and author and updated go first.

### Themes

#### Built-in presets

| Preset                 | Variant | Notes                                |
|------------------------|---------|--------------------------------------|
| `default`              | dark    | ANSI 16 palette — inherits your terminal colors |
| `catppuccin-latte`     | light   | Catppuccin Latte                     |
| `catppuccin-frappe`    | dark    | Catppuccin Frappé (warmest dark)     |
| `catppuccin-macchiato` | dark    | Catppuccin Macchiato (medium)        |
| `catppuccin-mocha`     | dark    | Catppuccin Mocha (deepest dark)      |

#### Per-color overrides

Three optional maps let you override individual palette entries on top of any preset:

- **`themeColors`** — applied to every preset.
- **`themeDark`** — applied only when the active preset is a dark variant.
- **`themeLight`** — applied only when the active preset is a light variant.

The precedence is preset → `themeColors` → (`themeDark` or `themeLight`). Empty values and unknown keys are ignored, so configs stay forward-compatible.

Supported keys:

| Key         | Role                                                  |
|-------------|-------------------------------------------------------|
| `green`     | Primary accent — titles, active borders, success     |
| `blue`      | Secondary accent, open pull requests                  |
| `red`       | Errors, failed builds, changes requested              |
| `yellow`    | Warnings, in-progress builds                          |
| `cyan`      | Search mode indicators                                |
| `magenta`   | Emphasis                                              |
| `orange`    | Names, metadata                                       |
| `white`     | Foreground text — use a **dark** color on light themes |
| `gray`      | Subtitles, hint bars                                  |
| `highlight` | Selection / cursor background                         |

Values are any lipgloss-accepted color: hex (`"#a6e3a1"`), ANSI 16 (`"4"`), or ANSI 256 (`"208"`).

If a value is not a recognized color format, that key falls back to your terminal's default color and a warning is logged (visible with `--debug`).

#### Example: Rose Pine

Auto-detect between Main (dark) and Dawn (light). Palette from [Rose Pine](https://rosepinetheme.com) — All natural pine, faux fur and a bit of soho vibes for the classy minimalist.

```yaml
gui:
  theme: auto

  themeDark:
    white:     "#e0def4"   # Text
    gray:      "#6e6a86"   # Muted
    green:     "#31748f"   # Pine
    blue:      "#9ccfd8"   # Foam
    cyan:      "#9ccfd8"
    red:       "#eb6f92"   # Love
    yellow:    "#f6c177"   # Gold
    magenta:   "#c4a7e7"   # Iris
    orange:    "#ebbcba"   # Rose
    highlight: "#403d52"

  themeLight:
    white:     "#575279"
    gray:      "#9893a5"
    green:     "#286983"
    blue:      "#56949f"
    cyan:      "#56949f"
    red:       "#b4637a"
    yellow:    "#ea9d34"
    magenta:   "#907aa9"
    orange:    "#d7827e"
    highlight: "#dfdad9"
```

#### Example: tweak a Catppuccin preset

Keep Mocha but swap the accent to a softer green and brighten the selection background:

```yaml
gui:
  theme: catppuccin-mocha
  themeColors:
    green:     "#94e2d5"
    highlight: "#45475a"
```

## Pull request tabs

Tabs are filters over the pull requests of the configured repositories. The defaults are Open / Mine / Reviewing:

```yaml
pullRequestTabs:
  - name: Open
    filter: all
  - name: Mine
    filter: mine
  - name: Reviewing
    filter: reviewing
```

Defining `pullRequestTabs` replaces the defaults entirely.

| Field | Description |
|-------|-------------|
| `name` | Tab label |
| `state` | `OPEN` (default), `MERGED`, `DECLINED` or `SUPERSEDED`. Case-insensitive; an unknown value is an error at startup |
| `filter` | `all` (default), `mine` (you are the author), `reviewing` (you are a reviewer) |
| `query` | Optional raw Bitbucket `q=` expression, passed through to the API |
| `maxResults` | Per-tab page size override |

`mine` and `reviewing` need to know who you are. A repository, project or workspace access token cannot read the current user, so those tabs stay empty and the status panel says `current user unknown — Mine/Reviewing tabs are empty`.

A longer example:

```yaml
pullRequestTabs:
  - name: Open
    filter: all
  - name: Mine
    filter: mine
  - name: Reviewing
    filter: reviewing
    maxResults: 20
  - name: To develop
    state: OPEN
    query: 'destination.branch.name = "develop"'
  - name: Merged
    state: MERGED
    maxResults: 25
```

## Page size (`maxResults`)

The number of pull requests fetched per repository per tab. Can be set globally and overridden per tab:

```yaml
maxResults: 25
pullRequestTabs:
  - name: Open
    filter: all
    maxResults: 50   # per-tab override
  - name: Merged
    state: MERGED    # inherits the global 25
```

Resolution order: per-tab `maxResults` → global `maxResults` → built-in default (50). Values `<= 0` are treated as unset. Bitbucket caps a single pull request page at 50 items, so larger values are served through several requests.

## Auto refresh (`refreshInterval`)

```yaml
refreshInterval: 120s
```

A Go duration string. Unset means `120s`. `0` or `off` disables the background refresh; an unparseable value is an error at startup. The refresh re-fetches the active tab's lists — build statuses and review data are only re-fetched when a pull request's `updated_on` has changed, so a warm cache costs one request per repository.

`r` refreshes the selected pull request (detail plus build statuses). `R` refreshes the whole tab.

An interval short enough to spend more than about 600 of Bitbucket's ~1000 requests per hour on auto-refresh alone raises a `refresh interval too short for N repositories` warning on the status panel.

## Markdown rendering (`rendererStyle`)

Pull request descriptions are Markdown, rendered with [glamour](https://github.com/charmbracelet/glamour).

```yaml
rendererStyle: auto
```

| Value | Behavior |
|-------|----------|
| *(unset, default)* | Same as `auto` |
| `auto` | Picks `dark` or `light` based on the detected terminal background. Detection can fail under `tmux` or `ssh`; set the value explicitly when it picks wrong |
| `dark` | Forces the dark theme |
| `light` | Forces the light theme |
| `notty` | Plain output without ANSI styling |

Any other value causes lazybitbucket to exit on startup with an error naming the invalid setting.

The same setting picks the syntax-highlighting theme of the diff pane, which is rendered with [chroma](https://github.com/alecthomas/chroma): `monokai` on a dark background, `github` on a light one. `notty` turns highlighting off — added lines are green and removed lines red, and nothing else is coloured. The lexer is chosen per file, from the file name first and its content second.

## Editor

```yaml
editor: nvim
```

Resolution order: `editor` → `$VISUAL` → `$EDITOR` → `nvim`.

`e` in the detail panel opens the file you are reading in this editor: on the Files tab the selected file at line 1, inside a file diff the file at the cursor's line. lazybitbucket hands the terminal over and picks the session back up when the editor exits.

The value may carry flags of its own (`code --wait`); only its first field names the program. That name decides how the line number is passed, because there is no common syntax for it:

| Editors | Invocation | Dialect |
|---------|------------|---------|
| `vim`, `nvim`, `vi`, `gvim`, `nano`, `micro`, `emacs`, `kak` | `nvim +12 /repo/path/file.go` | `+LINE` before the path |
| `code`, `codium`, `code-insiders` | `code --goto /repo/path/file.go:12` | `--goto path:LINE` |
| `subl`, `sublime_text`, `zed`, `hx` | `zed /repo/path/file.go:12` | `path:LINE` |
| anything else | `myeditor /repo/path/file.go` | the path alone, no line |

The name is matched on the program's base name, so `/opt/homebrew/bin/nvim` is still `nvim`. Configured flags keep their place: `editor: code --wait` runs `code --wait --goto path:12`.

Diff paths are repository-relative and are resolved against the **root of the git repository lazybitbucket was started in** — not the working directory, so running it from a subdirectory is fine. That checkout is never matched against the pull request's repository, because nothing maps a Bitbucket repository to a local clone. What you get is therefore whatever is on disk:

- not started inside a git repository → `not inside a git repository`
- the file is not in the working tree → `<path> not found in the working tree — check out the source branch (c)`
- the pull request deleted the file → `file was removed in this pull request`
- the checkout is on another branch → the hint bar says `editing <branch>, PR source is <source>` and the file opens anyway

`c` checks out the pull request's source branch, which is the usual way to make the two agree.

## Keybindings

All keybindings are remappable. See [Keybindings](Keybindings.md) for the full list of defaults.

```yaml
keybinding:
  universal:
    quit: "q"
    help: "?"
    search: "/"
  navigation:
    down: "j"
    up: "k"
    top: "g"
    bottom: "G"
    halfPageDown: "ctrl+d"
    halfPageUp: "ctrl+u"
  pullRequests:
    browser: "o"
    copyURL: "y"
    checkout: "c"
    focusRight: "l"
  detail:
    back: "backspace"
```

Only include keys you want to change. Missing keys keep their defaults.

There is deliberately **no `keybinding.repos` group**. The keymap is keyed by action, and the Repos and Builds panels reuse `pullRequests.select`, `pullRequests.open` and `pullRequests.focusRight`; a separate group would silently overwrite the pull request bindings. Remapping `pullRequests.select` remaps it on all three panels.

The `navigation` section controls list navigation keys used across all panels. These default to vim-style keys (`j`/`k`/`g`/`G`/`ctrl+d`/`ctrl+u`).

The `detail` section includes keys for scrolling the detail panel from any left panel without switching focus (defaults: `J`/`K` for line scroll, `ctrl+f`/`ctrl+b` for half-page), plus the keys the detail panel answers to itself: `overviewTab`/`filesTab`/`commitsTab`/`commentsTab` (`i`/`d`/`m`/`t`) jump straight to one pull request tab, `nextFile`/`prevFile` (`n`/`p`) step through the changed files, and `openEditor` (`e`) opens the file under the cursor in [your editor](#editor). Those seven only act while the detail panel is focused.

Setting a navigation key replaces all defaults for that action. For example, `down: "n"` means only `n` navigates down, `j`, arrow down and `ctrl+j` will no longer work.

## Git integration

```yaml
git:
  closeOnCheckout: false
```

`c` on the selected pull request checks out its source branch in the repository lazybitbucket was started in — tracking the remote branch when it is not present locally. With `closeOnCheckout: true` lazybitbucket exits after a successful checkout, which is convenient when you launch it from a shell alias.

The current branch is also used at startup: if it matches the source branch of a pull request in the first tab, that pull request is selected automatically.

## Custom commands

Bind shell commands to keys, with Go template access to the focused pull request, repository or build. Custom bindings take precedence over built-in keys, so they can be used to override any action.

```yaml
customCommands:
  - key: "ctrl+y"
    name: "Copy branch name"
    command: "printf %s {{.PR.SourceBranch | shellescape}} | pbcopy"
    suspend: false
  - key: "ctrl+o"
    name: "Open build log"
    command: "open {{.Build.URL | shellescape}}"
    contexts: [builds]
    suspend: false
  - key: "ctrl+s"
    name: "Review session"
    command: >-
      tmux-pr-sessionizer {{.Repo.Slug | shellescape}}
      {{.PR.SourceBranch | shellescape}}
      pr-{{.PR.ID}}
    contexts: [pullrequests, detail]
```

Each command has:

| Field | Description |
|-------|-------------|
| `key` | Key binding. Supports single letters, modifiers (`ctrl+x`, `alt+x`), and special keys (`tab`, `enter`). |
| `name` | Label shown in the help overlay and help bar. |
| `command` | Shell command string. Rendered as a Go template before execution. |
| `contexts` | Optional list of UI contexts the command fires in. Defaults to `[pullrequests, detail]`. |
| `suspend` | Optional. `true` (default) hands the terminal to the child process; set `false` for background commands like clipboard copies or notifications. |
| `refresh` | Optional, default `false`. After the command exits successfully, refresh the selected pull request — the same work as `r` (detail plus build statuses). Only a command that asks for it costs the extra requests. |

The command string is a Go template with `missingkey=error`, parsed at startup: a bad template, an unknown context, an empty `key`/`name`/`command`, or the same key twice in the same context is a startup error naming the offending entry.

**Suspend semantics.** With `suspend: true` lazybitbucket hands over the terminal (stdin, stdout, stderr) and resumes when the child exits — the right mode for anything interactive (an editor, `tmux`, a pager). With `suspend: false` the command runs in the background, its output is captured, and the last non-empty line is shown in the help bar (or, on a non-zero exit, in the status panel). Background commands are killed with `SIGTERM` when lazybitbucket quits. Commands run through `$SHELL -c` (`sh` when `$SHELL` is unset).

### Contexts

A command fires when one of its declared contexts matches the current UI state. The contexts — not the template text — decide which scopes have to be filled: a command declared in `builds` always needs a selected pull request, repository **and** build. When one of them is missing the command does not run and the help bar says `<name>: no build selected` (or `no pull request` / `no repository`).

Custom commands are matched before built-in keys, so a binding can shadow any action. If every key bound to `quit` is shadowed in every context, lazybitbucket prints a warning on stderr and on the status panel at startup rather than leaving you with no way out.

| Context | Active when | Scopes available |
|---------|-------------|------------------|
| `pullrequests` | The Pull Requests panel is focused | `PR`, `Repo` |
| `builds` | The Builds panel is focused | `PR`, `Repo`, `Build` |
| `repos` | The Repos panel is focused, or the detail panel is showing a repository | `Repo` |
| `detail` | The right detail panel is showing a pull request, a pipeline run or a step log | `PR`, `Repo` |
| `diff` | The detail panel is on the Files tab or showing one file's diff | `PR`, `Repo`, `File` |

### Template variables

**Pull request scope** (`pullrequests`, `builds`, `detail`):

| Variable | Description |
|----------|-------------|
| `{{.PR.ID}}` | Pull request id, unique per repository |
| `{{.PR.Title}}` | Title |
| `{{.PR.Description}}` | Raw Markdown description |
| `{{.PR.State}}` | `OPEN`, `MERGED`, `DECLINED` or `SUPERSEDED` |
| `{{.PR.Draft}}` | `true` for a draft pull request |
| `{{.PR.Author}}` | Author display name |
| `{{.PR.AuthorNickname}}` | Author nickname |
| `{{.PR.SourceBranch}}` | Source branch name |
| `{{.PR.DestinationBranch}}` | Destination branch name |
| `{{.PR.SourceCommit}}` | Source commit hash |
| `{{.PR.DestinationCommit}}` | Destination commit hash |
| `{{.PR.URL}}` | Fully qualified pull request URL |
| `{{.PR.Repo}}` | Repository slug |
| `{{.PR.RepoFullName}}` | `workspace/slug` |
| `{{.PR.Workspace}}` | Workspace slug |
| `{{.PR.CommentCount}}` | Number of comments |
| `{{.PR.TaskCount}}` | Number of tasks |
| `{{.PR.Reviewers}}` | Number of reviewers |
| `{{.PR.Approvals}}` | Number of approvals |
| `{{.PR.ChangesRequested}}` | Number of reviewers requesting changes |

**Repository scope** (`pullrequests`, `builds`, `repos`, `detail`):

| Variable | Description |
|----------|-------------|
| `{{.Repo.Slug}}` | Repository slug |
| `{{.Repo.Name}}` | Repository display name |
| `{{.Repo.FullName}}` | `workspace/slug` |
| `{{.Repo.Workspace}}` | Workspace slug |
| `{{.Repo.URL}}` | Repository URL |
| `{{.Repo.MainBranch}}` | Main branch name |

**Build scope** (`builds`):

| Variable | Description |
|----------|-------------|
| `{{.Build.Name}}` | Build / status name |
| `{{.Build.State}}` | `SUCCESSFUL`, `FAILED`, `INPROGRESS` or `STOPPED` |
| `{{.Build.Key}}` | Status key reported by the build system |
| `{{.Build.RefName}}` | Ref the build ran on |
| `{{.Build.URL}}` | Build result URL |
| `{{.Build.BuildNumber}}` | Pipeline build number, when the URL carries one |

**File scope** (`diff`):

| Variable | Description |
|----------|-------------|
| `{{.File.Path}}` | Repository-relative path of the changed file, as it appears in the diff |
| `{{.File.OldPath}}` | The path before a rename; empty otherwise |
| `{{.File.Status}}` | `added`, `removed`, `modified` or `renamed` |
| `{{.File.Line}}` | New-side line under the cursor inside a file diff; `1` on the Files tab |

`diff` is the more specific of the two detail contexts: a command declared in `detail` keeps firing on the Files tab and inside a file diff, and a `diff` command bound to the same key wins there. With no file selected the command does not run and the help bar says `<name>: no file selected`.

An example — blame the line the cursor is on:

```yaml
customCommands:
  - key: "ctrl+b"
    name: "Blame this line"
    command: >-
      git -C {{.GitRepoPath | shellescape}} blame -L {{.File.Line}},+20
      -- {{.File.Path | shellescape}} | less
    contexts: [diff]
```

### Shared fields

Available in every context:

| Variable | Description |
|----------|-------------|
| `{{.Workspace}}` | Active workspace slug |
| `{{.GitBranch}}` | Current git branch, if lazybitbucket was started in a git repo |
| `{{.GitRepoPath}}` | Git repository path |

### Template helpers

| Helper | Description |
|--------|-------------|
| `{{.X \| shellescape}}` | Wraps the value in single quotes with inner quotes escaped. Use for any template value that could contain shell metacharacters. |
| `{{.X \| slugify}}` | Lowercases and dash-separates the value. Useful for branch and session names. |

## Files

| File | Description |
|------|-------------|
| `config.yml` | Main configuration |
| `auth.json` | Credentials, created automatically with `0600` permissions |
