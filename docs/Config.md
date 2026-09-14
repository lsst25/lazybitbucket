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
| `LAZYBITBUCKET_READ_ONLY` | `1`, `true` or `yes` turns on [read-only mode](#read-only-mode). Any other value, including `0` and `false`, leaves the config alone — it can only turn the mode on |

The token is never read from or written to `config.yml`. It lives in `auth.json` or in the environment.

## Default config

Do not copy the entire thing into your config. Only add the settings you want to change.

```yaml
bitbucket:
    workspace: ""
    repositories: []
    email: ""
    baseUrl: ""       # "" → https://api.bitbucket.org/2.0
    readOnly: false   # true → browse only; every write is refused
gui:
    theme: default
    themeColors: {}   # optional palette overrides (see Themes section)
    themeDark: {}     # applied on dark presets only
    themeLight: {}    # applied on light presets only
    sidePanelWidth: 40
    collapsedPanelHeight: 5
    screenMode: normal       # normal | half | full — the layout at start-up
    startView: pullRequests  # pullRequests | pipelines — the view at start-up
    prefetchDetailTabs: true # fetch Files, Commits and Comments together when the detail panel takes focus
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
        nextScreenMode: "+"
        prevScreenMode: _
        nextView: shift+tab
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
        merge: M
        sync: S
        edit: E
        reviewers: V
        toggleDraft: D
        approve: A
        comment: C
        create: N
        resolve: T
    builds:
        rerun: P
        stop: X
    pipelines:
        openPullRequest: L
    # There is no `repos` group: the Repos, Builds and Pipelines panels reuse
    # pullRequests.select, pullRequests.open and pullRequests.focusRight;
    # `builds` adds only the two pipeline actions, `pipelines` only the jump
    # back to a run's pull request.
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
        hideResolved: f
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
diffRenderer: ""        # "" → the built-in chroma highlighting
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
  readOnly: false
```

| Field | Description |
|-------|-------------|
| `workspace` | Workspace slug. Required. Also settable with `BITBUCKET_WORKSPACE` or `--workspace` |
| `repositories` | Repository slugs to fetch pull requests from. When lazybitbucket is started inside a checkout whose git remote points at one of the listed repositories, the list opens scoped to that repository (the Repos panel marks it; pick `All repositories` to widen). When empty, the first 25 repositories you are a member of are discovered, most recently updated first; the status panel warns when that cap is hit, and says `no repositories visible to this token` when discovery comes back empty (which is what an access token sees) |
| `email` | Atlassian account email. Present only for the Basic auth option; with a token and no email, the token is sent as `Authorization: Bearer` |
| `baseUrl` | API base URL. Only useful for testing against a stand-in server |
| `readOnly` | Browse-only mode: every request that would change something is refused. See below |

There is no `token` key. App passwords no longer exist; use an API token with scopes and let the setup wizard store it in `auth.json`, or pass `BITBUCKET_TOKEN`. See the [Setup section of the README](../README.md#setup) for the scopes.

### Read-only mode

```yaml
bitbucket:
  readOnly: true
```

Read-only means **browse only**. The API client refuses every non-GET request with an error before a socket is opened, so nothing lazybitbucket does can reach Bitbucket and change it, and the nine [pull request actions](Keybindings.md#pull-request-actions) — `M`, `S`, `E`, `V`, `D`, `A`, `C`, `N` and `T` — together with the two [pipeline actions](Keybindings.md#rerun-and-stop-a-pipeline) `P` and `X` are hidden from the help surfaces and answer `read-only mode: cannot <action>` if you press them anyway. Sync is refused too: it is driven by local git rather than the API, but it ends in a push, which is a write.

Three ways to turn it on, and none of them can turn it off:

| Source | Where |
|--------|-------|
| `readOnly: true` | under `bitbucket` in `config.yml` |
| `--read-only` | on the command line |
| `LAZYBITBUCKET_READ_ONLY` | in the environment, set to `1`, `true` or `yes` |

`--read-only=false` and `LAZYBITBUCKET_READ_ONLY=0` do not widen a session that the config file put in read-only mode — they simply leave the setting alone. To go back to a writing session, remove `readOnly` from `config.yml`.

The status panel shows `· read-only` on its first line while the mode is on.

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

`screenMode` is the layout lazybitbucket starts in, the same three modes lazygit has: `normal` (the default) is the split described above; `half` gives the side panels and the detail panel half the window each, whatever `sidePanelWidth` says; `full` hands the whole window to the focused panel — the detail panel when it is focused, else the focused left panel — and the other panels are not drawn until you leave the mode or switch focus, which moves the full screen along. `+` and `_` cycle the modes at runtime (`universal.nextScreenMode` / `universal.prevScreenMode`). On a terminal too narrow for the side-by-side layout `half` behaves like `normal`.

`startView` is the view lazybitbucket opens in: `pullRequests` (the default) is the pull request list with its Builds and Repos panels, `pipelines` is the [pipelines view](Keybindings.md#pipelines-view), the recent runs of every configured repository. Either way `⇧tab` switches between the two at runtime (`universal.nextView`), and the view you start in is the only thing this setting decides — it is not remembered across runs. An unknown value is an error at startup, named the way an unknown `screenMode` is.

`prefetchDetailTabs` decides when the detail panel's Files, Commits and Comments tabs are fetched. On — the default — all three are fetched together the moment the detail panel takes focus, because a reader who moves into the panel is about to walk the tabs and three requests now beat a pause on each tab later; a tab entered while its fetch is still in flight waits for that answer instead of asking again, and a tab whose fetch failed says so when you open it rather than the moment it failed. Set it to `false` and each tab fetches itself the first time you open it, as before. Either way moving the cursor in the pull request list fetches no tab at all — the preview costs what it always did — so the price of the prefetch is three requests per pull request you actually focus, against the hourly request budget noted below.

`theme` selects the color palette. Supported values: `default` (ANSI 16, original look); a preset name — the four [Catppuccin](https://github.com/catppuccin/catppuccin) flavors (`catppuccin-latte`, `catppuccin-frappe`, `catppuccin-macchiato`, `catppuccin-mocha`) and the three [Kanagawa](https://github.com/rebelot/kanagawa.nvim) variants (`kanagawa-wave`, `kanagawa-dragon`, `kanagawa-lotus`); or a family name that follows your terminal background — `auto` and `catppuccin` (`catppuccin-mocha` on dark, `catppuccin-latte` on light) and `kanagawa` (`kanagawa-wave` on dark, `kanagawa-lotus` on light). Omitting the `theme` key or setting it to `""` keeps the `default` ANSI 16 palette. An unknown theme name is an error. Hex-based themes require a terminal with truecolor support.

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
| `kanagawa-wave`        | dark    | [Kanagawa](https://github.com/rebelot/kanagawa.nvim) Wave — the default variant. The Kanagawa presets paint the chrome in the theme's blue (`accent`), as a Kanagawa tmux or nvim does; green stays for success |
| `kanagawa-dragon`      | dark    | Kanagawa Dragon — darker, desaturated |
| `kanagawa-lotus`       | light   | Kanagawa Lotus                       |

#### Theme families

A family name picks the dark or the light preset of one theme from your terminal background, so a
terminal that follows the system appearance gets a matching palette without a config edit:

| Family       | Dark background    | Light background   |
|--------------|--------------------|--------------------|
| `auto`       | `catppuccin-mocha` | `catppuccin-latte` |
| `catppuccin` | `catppuccin-mocha` | `catppuccin-latte` |
| `kanagawa`   | `kanagawa-wave`    | `kanagawa-lotus`   |

The background is read from the terminal. Under `tmux` and `screen` the terminal cannot be asked —
it may be attached to several — and the answer would always be dark; on macOS lazybitbucket reads
the system appearance (`defaults read -g AppleInterfaceStyle`) there instead, which is what a
terminal that follows the appearance is showing. `COLORFGBG`, when set, wins over the appearance.
Elsewhere detection can still fail (over `ssh`, for instance) and lands on the dark variant; name
the variant explicitly when it picks wrong. `rendererStyle: auto` uses the same detection, so the
Markdown and diff colours switch together with the palette.

```yaml
gui:
  theme: kanagawa
```

Prefer the green chrome of the other presets? Override the accent:

```yaml
gui:
  theme: kanagawa
  themeDark:
    accent: "#98BB6C"   # springGreen
  themeLight:
    accent: "#6f894e"   # lotusGreen
```

#### Per-color overrides

Three optional maps let you override individual palette entries on top of any preset:

- **`themeColors`** — applied to every preset.
- **`themeDark`** — applied only when the active preset is a dark variant.
- **`themeLight`** — applied only when the active preset is a light variant.

The precedence is preset → `themeColors` → (`themeDark` or `themeLight`). Empty values and unknown keys are ignored, so configs stay forward-compatible.

Supported keys:

| Key         | Role                                                  |
|-------------|-------------------------------------------------------|
| `accent`    | Chrome — panel titles, the focused border, the active tab, key hints, dialog frames. Same as `green` in every preset except Kanagawa, which uses its blue |
| `green`     | Success — approvals, passing builds, merged state, added lines (and the accent, outside Kanagawa) |
| `blue`      | Secondary accent, open pull requests, `@`-mentions in descriptions and comments |
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
| `auto` | Picks `dark` or `light` based on the detected terminal background, the way [theme families](#theme-families) do: under `tmux` and `screen` the macOS system appearance decides. Detection can fail over `ssh`; set the value explicitly when it picks wrong |
| `dark` | Forces the dark theme |
| `light` | Forces the light theme |
| `notty` | Plain output without ANSI styling |

Any other value causes lazybitbucket to exit on startup with an error naming the invalid setting.

The same setting picks the syntax-highlighting theme of the diff pane, which is rendered with [chroma](https://github.com/alecthomas/chroma): `monokai` on a dark background, `github` on a light one. `notty` turns highlighting off — added lines are green and removed lines red, and nothing else is coloured. The lexer is chosen per file, from the file name first and its content second.

To colour the diff with an external program such as delta instead, see [Diff renderer](#diff-renderer-diffrenderer).

## Editor

```yaml
editor: nvim
editorInTmux: window   # window (default) | inline
```

Resolution order: `editor` → `$VISUAL` → `$EDITOR` → `nvim`.

`e` in the detail panel opens the file you are reading in this editor: on the Files tab the selected file at line 1, inside a file diff the file at the cursor's line. Outside tmux, lazybitbucket hands the terminal over and picks the session back up when the editor exits.

**Inside tmux** (`$TMUX` is set) the file opens in a **new tmux window** instead, started in the checkout's root and named after the file, so the session stays where it is: tmux switches to the new window as it creates it and closes it when the editor exits, which brings you back. The editor command is passed to tmux as one quoted shell line, so paths with spaces or quotes survive. tmux's own error (`no server running`, for instance) lands on the status panel. Set `editorInTmux: inline` to keep the suspend-and-return behaviour inside tmux too. The setting has no effect outside tmux.

The value may carry flags of its own (`code --wait`); only its first field names the program. That name decides how the line number is passed, because there is no common syntax for it:

| Editors | Invocation | Dialect |
|---------|------------|---------|
| `vim`, `nvim`, `vi`, `gvim`, `nano`, `micro`, `emacs`, `kak` | `nvim +12 /repo/path/file.go` | `+LINE` before the path |
| `code`, `codium`, `code-insiders` | `code --goto /repo/path/file.go:12` | `--goto path:LINE` |
| `subl`, `sublime_text`, `zed`, `hx` | `zed /repo/path/file.go:12` | `path:LINE` |
| anything else | `myeditor /repo/path/file.go` | the path alone, no line |

The name is matched on the program's base name, so `/opt/homebrew/bin/nvim` is still `nvim`. Configured flags keep their place: `editor: code --wait` runs `code --wait --goto path:12`.

The same setting opens the **pull request description** for editing (`E` → Description). That one is not a file in your checkout: lazybitbucket writes the description to a temporary `.md` file, opens it with no line number (`nvim /tmp/lazybitbucket-web-app-41-123.md`), reads it back when the editor exits and deletes it. Because it has to wait for the file, the description editor always runs inline, tmux or not — `editorInTmux` does not apply to it. A graphical editor that returns immediately therefore needs its wait flag — `editor: code --wait` — or the file is read back before you have typed anything.

Diff paths are repository-relative and are resolved against the **root of the git repository lazybitbucket was started in** — not the working directory, so running it from a subdirectory is fine. That checkout is never matched against the pull request's repository, because nothing maps a Bitbucket repository to a local clone. What you get is therefore whatever is on disk:

- not started inside a git repository → `not inside a git repository`
- the file is not in the working tree → `<path> not found in the working tree — check out the source branch (c)`
- the pull request deleted the file → `file was removed in this pull request`
- the checkout is on another branch → the hint bar says `editing <branch>, PR source is <source>` and the file opens anyway

`c` checks out the pull request's source branch, which is the usual way to make the two agree.

## Diff renderer (`diffRenderer`)

The file diff pane colours code with [chroma](https://github.com/alecthomas/chroma) by default. `diffRenderer` hands that job to an external program instead — the same idea as lazygit's `git.paging.pager`, and [delta](https://github.com/dandavison/delta) is what it is built around:

```yaml
diffRenderer: delta --color-only --paging=never
```

Unset — the default — keeps chroma.

The value is a command line, split like `editor`: the first field names the program, the rest are flags. lazybitbucket pipes the file's whole unified diff to the program on **stdin** and reads its **stdout**.

### The contract

**One output line per input line.** The program may colour a line; it may not add, drop, wrap, split or reorder one. That is what `--color-only` means to delta, and it is why the flag is not optional here: delta's normal output draws its own file headers, line numbers and side-by-side layout, none of which line up with anything.

The reason is that the pane keeps its own furniture around each line. The line-number gutters, the `+` / `-` marker column and the `●` review-thread markers that take it over, the cursor bar, the hunk headers, `enter` on a commented line and `e` on the line under the cursor are all still lazybitbucket's, drawn from the parsed diff. Only the code column comes from the program, matched to its parsed line by position in the document. A program that prints a different number of lines cannot be matched up at all.

Two details are handled for you: the leading `+`, `-` or space of each line is dropped (the pane draws that column itself), and delta's trailing erase-in-line sequence is stripped and its background repainted out to the width of the code column, so a full-width row tint still looks like one inside a panel.

### Light and dark

The dark/light choice is the program's own — `rendererStyle` does not reach it. Pass delta's flag yourself:

```yaml
diffRenderer: delta --color-only --paging=never --dark
```

or point the key at a wrapper script that decides — the value is not run through a shell, so give the wrapper its full path (a leading `~` is not expanded), and it is fine for it to run delta without `exec`. `rendererStyle: notty` is the one exception: it says the terminal cannot show colour, so the external renderer does not run either.

### When it goes wrong

A renderer failure never costs you the diff — it is shown with the built-in colouring — and the complaint goes to the status panel as `diffRenderer: …`, once per distinct message rather than once per file.

| Failure | Effect |
|---------|--------|
| The program cannot be run — not on `$PATH`, or a path that names nothing | Reported, and the renderer is off for the rest of the session |
| The output does not line up (`<program>: add --color-only (or print one line per input line) … printed N lines for M lines of diff`) | Reported, and the renderer is off for the rest of the session — usually a missing `--color-only` |
| Non-zero exit | Reported with the program's first line of standard error; the next file is still tried |
| No output within 15 seconds | Reported; the next file is still tried |

### Cost

The diff is fetched per file, so the program runs once per file you open, off the event loop — and its output is cached with the diff, per pull request and file, until the source branch moves. Scrolling a file that is already open costs nothing.

## Keybindings

All keybindings are remappable. See [Keybindings](Keybindings.md) for the full list of defaults.

```yaml
keybinding:
  universal:
    quit: "q"
    help: "?"
    search: "/"
    nextView: "shift+tab"
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
    merge: "M"
    sync: "S"
    edit: "E"
    reviewers: "V"
    toggleDraft: "D"
    approve: "A"
    comment: "C"
    create: "N"
    resolve: "T"
  builds:
    rerun: "P"
    stop: "X"
  pipelines:
    openPullRequest: "L"
  detail:
    back: "backspace"
```

Only include keys you want to change. Missing keys keep their defaults.

There is deliberately **no `keybinding.repos` group**. The keymap is keyed by action, and the Repos, Builds and Pipelines panels reuse `pullRequests.select`, `pullRequests.open` and `pullRequests.focusRight`; a separate group would silently overwrite the pull request bindings. Remapping `pullRequests.select` remaps it on all four panels. There **is** a `keybinding.builds` group, but it holds only the two pipeline actions `rerun` and `stop`, which no other panel has, and a `keybinding.pipelines` group holding only `openPullRequest` (`L`), which only the Pipelines panel has; selecting, opening and focus-right on either panel still come from `pullRequests`.

`rerun` and `stop` stay under `builds` even though the Pipelines panel answers to them too: they are one action against one pipeline run, whether it was reached through a build status or through the run list, and splitting them across two groups would mean remapping the same key twice to keep the app consistent with itself.

The `navigation` section controls list navigation keys used across all panels. These default to vim-style keys (`j`/`k`/`g`/`G`/`ctrl+d`/`ctrl+u`).

The `pullRequests` section also carries the nine [pull request actions](Keybindings.md#pull-request-actions) — `merge`, `sync`, `edit`, `reviewers`, `toggleDraft`, `approve`, `comment`, `create` and `resolve` (`M`/`S`/`E`/`V`/`D`/`A`/`C`/`N`/`T`) — which act on the selected pull request from the list and from the detail panel alike, except `create`, which acts on the checked-out branch, and `resolve`, which acts on the conversation in hand; `comment` is the one of them that depends on where it is pressed: a reply on the Comments tab and in an open thread, a comment on the pull request from Overview, and an inline comment on a line of a file's diff. The `builds` section carries the two [pipeline actions](Keybindings.md#rerun-and-stop-a-pipeline) — `rerun` and `stop` (`P`/`X`) — which act on the pipeline run behind the selected build, from the Builds panel, from the Pipelines panel and from the detail panel's pipeline and step-log views. The `pipelines` section carries `openPullRequest` (`L`), which jumps from a run in the [pipelines view](Keybindings.md#pipelines-view) to the pull request it ran for. The keys *inside* the dialogs any of them open are fixed and not remappable.

`universal.nextView` (`shift+tab`) switches between the two views; which one the app starts in is [`gui.startView`](#gui).

The `detail` section includes keys for scrolling the detail panel from any left panel without switching focus (defaults: `J`/`K` for line scroll, `ctrl+f`/`ctrl+b` for half-page), plus the keys the detail panel answers to itself: `overviewTab`/`filesTab`/`commitsTab`/`commentsTab` (`i`/`d`/`m`/`t`) jump straight to one pull request tab, `hideResolved` (`f`) leaves the resolved conversations off the Comments tab, `nextFile`/`prevFile` (`n`/`p`) step through the changed files, and `openEditor` (`e`) opens the file under the cursor in [your editor](#editor). Those eight only act while the detail panel is focused.

Setting a navigation key replaces all defaults for that action. For example, `down: "n"` means only `n` navigates down, `j`, arrow down and `ctrl+j` will no longer work.

## Git integration

```yaml
git:
  closeOnCheckout: false
```

`c` on the selected pull request checks out its source branch in the repository lazybitbucket was started in. It first fetches that branch from the remote whose URL points at the pull request's repository (see the sync section for how the remote is found), so a branch pushed after your last fetch is still found; a local branch that exists is checked out and fast-forwarded when the remote copy is ahead, left alone with a note when it has diverged, and created as a tracking branch when it only exists on the remote. A branch known neither locally nor on that remote is an error — `c` never creates one. With `closeOnCheckout: true` lazybitbucket exits after a successful checkout, which is convenient when you launch it from a shell alias.

The current branch is also used at startup: the pull request whose source branch is the one you have checked out is selected automatically, and the hint bar says so — `selected #103 — the pull request for bugfix/upstream-timeout`.

It looks in every tab that has loaded, not only the one in view: the tab you are on wins, and otherwise it switches to the tab holding the pull request and names it (`selected #103 in Reviewing — …`). That matters because the Mine and Reviewing tabs stay empty until Bitbucket answers who you are, which can be after the first list arrives — so the selection is attempted on each list that lands until it succeeds, rather than once against whatever was there first. When the checkout's remotes name a repository, only that repository's pull requests are considered; the same branch name in two repositories is common enough that selecting the wrong one would be worse than selecting none.

It stops for good the moment you move the list cursor or switch tabs yourself. Nothing moves under you after that, and a background refresh never re-runs it.

## Pull request actions

`M` merge, `S` sync, `E` edit, `V` reviewers, `D` draft, `A` approve, `C` comment, `N` create and `T` resolve are the only keys that write. There is nothing to configure for them beyond the [key names](#keybindings) and [`bitbucket.readOnly`](#read-only-mode), which refuses all nine; the dialogs remember nothing between invocations. `C` asks in the same input popup `E` uses for a title, writes an inline comment when it is pressed on a line of a diff, and is the one write a merged or declined pull request still accepts — see [`C` — comment or reply](Keybindings.md#c--comment-or-reply). `A` needs the same `write:pullrequest:bitbucket` scope the other API writes do, and refuses your own pull requests locally — Bitbucket does not let an author approve their own work. What each dialog looks like is in [Keybindings](Keybindings.md#pull-request-actions) — what follows is the behaviour behind them.

`P` rerun and `X` stop are writes as well, but against a pipeline run rather than a pull request: they are bound under [`keybinding.builds`](#keybindings), `readOnly` refuses them like the other six, and they need `write:pipeline:bitbucket` — a scope neither the six actions nor anything lazybitbucket reads asks for. [Rerun and stop a pipeline](Keybindings.md#rerun-and-stop-a-pipeline) has the dialogs and the refusals.

### Merge strategies

The merge dialog offers the six the API accepts, in this order:

| Strategy | What Bitbucket does |
|----------|---------------------|
| merge commit | Always creates a merge commit. Bitbucket's own default |
| squash | Replaces the branch's commits with a single one |
| fast-forward | Moves the destination branch, and fails when the branch is not already ahead of it |
| squash fast-forward | Squashes, then fast-forwards |
| rebase fast-forward | Rebases onto the destination, then fast-forwards |
| rebase merge | Rebases onto the destination, then creates a merge commit |

A repository can allow only some of them. lazybitbucket does not know which — it offers all six and lets the server decide, so picking a strategy the repository disallows comes back as a `400` whose message names the problem. The list is fixed and is not read from the repository's merge settings.

Bitbucket answers a merge either with the merged pull request or with `202 Accepted` and a task to poll, which is what a large or busy repository tends to do. On a `202` lazybitbucket polls that task every two seconds for up to thirty attempts — about a minute — and reports the merged pull request when it lands. If the task is still running by then the hint bar says so and `r` picks the result up whenever you like; the merge is not cancelled.

### Sync with the target branch

Bitbucket's "Sync now" button has no public API, so `S` does the same work with local git, in the repository lazybitbucket was started in. It needs:

- a git repository — the same one the diff pane resolves files against, found from the working directory upwards, so starting in a subdirectory is fine;
- the pull request's **source branch** checked out (`c` does that). Sync refuses with `check out the source branch first (c)` otherwise, and re-checks live before it touches anything;
- a **clean working tree**. Staged, unstaged or untracked changes are refused with `working tree has uncommitted changes`.

Nothing checks that the checkout really is the pull request's repository — as with the editor, there is no mapping from a Bitbucket repository to a local clone. What is matched is the **remote**: the one whose fetch URL points at `<workspace>/<slug>`, in either the `git@bitbucket.org:acme/api.git` or the `https://bitbucket.org/acme/api.git` form, case-insensitively and with or without the `.git` suffix. When several match, the one on bitbucket.org wins over a mirror; when none matches the sync is refused — there is deliberately no `origin` fallback, because pushing into an unrelated repository would be worse than doing nothing. Pull requests whose source branch lives in a fork are refused too.

The run is `git fetch <remote> <target>`, then a fetch of the source branch to check that its remote copy holds nothing your checkout lacks (otherwise the sync stops with "pull first" — a rebase followed by a forced push would throw those commits away), then `git merge --no-edit <remote>/<target>` or `git rebase <remote>/<target>` depending on the Method row, then — when the Push checkbox is on — `git push <remote> HEAD` for a merge and `git push --force-with-lease <remote> HEAD` for a rebase, because a rebase rewrites the branch. Remote commands run with terminal prompts disabled and give up after two minutes.

A conflict aborts: `git merge --abort` / `git rebase --abort` run before the error is reported, so the working tree is left exactly as it was found and you can resolve it yourself. Git is run with terminal prompts disabled, so a repository that would ask for credentials fails with a message instead of hanging the UI behind an invisible prompt.

### Reviewers

Saving the reviewer picker sends the **complete** checked set, not a diff. That is not a choice: Bitbucket clears any reviewer a `PUT` leaves out, so every edit — including a title or description change — carries the full reviewer list along with it. When the list payload did not include the reviewers, lazybitbucket fetches the pull request first rather than guessing.

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
| `changedFiles` | Optional, default `false`. Fills `{{.Files}}` with the pull request's whole changed-file list. Needs a context that has a pull request (anything but `repos`), and costs a diffstat request. |

The command string is a Go template with `missingkey=error`, parsed at startup: a bad template, an unknown context, an empty `key`/`name`/`command`, or the same key twice in the same context is a startup error naming the offending entry.

**Suspend semantics.** With `suspend: true` lazybitbucket hands over the terminal (stdin, stdout, stderr) and resumes when the child exits — the right mode for anything interactive (an editor, `tmux`, a pager). With `suspend: false` the command runs in the background, its output is captured, and the last non-empty line is shown in the help bar (or, on a non-zero exit, in the status panel). Background commands are killed with `SIGTERM` when lazybitbucket quits. Commands run through `$SHELL -c` (`sh` when `$SHELL` is unset).

### Contexts

A command fires when one of its declared contexts matches the current UI state. The contexts — not the template text — decide which scopes have to be filled: a command declared in `builds` always needs a selected pull request, repository **and** build. When one of them is missing the command does not run and the help bar says `<name>: no build selected` (or `no pull request` / `no repository`).

`changedFiles` is the one exception: it is asked for per command rather than implied by a context, because filling it costs a request the other scopes do not.

Custom commands are matched before built-in keys, so a binding can shadow any action. If every key bound to `quit` is shadowed in every context, lazybitbucket prints a warning on stderr and on the status panel at startup rather than leaving you with no way out.

| Context | Active when | Scopes available |
|---------|-------------|------------------|
| `pullrequests` | The Pull Requests panel is focused | `PR`, `Repo` |
| `builds` | The Builds panel is focused | `PR`, `Repo`, `Build` |
| `repos` | The Repos panel is focused, or the detail panel is showing a repository | `Repo` |
| `detail` | The right detail panel is showing a pull request, a pipeline run or a step log | `PR`, `Repo` |
| `diff` | The detail panel is on the Files tab or showing one file's diff | `PR`, `Repo`, `File` |

All five are contexts of the pull requests view. Nothing matches while the [pipelines view](Keybindings.md#pipelines-view) is on screen, the Repos panel there included, so no custom command fires there — four of the five scopes need a selected pull request, and that view has none.

### Template variables

**Pull request scope** (`pullrequests`, `builds`, `detail`):

| Variable | Description |
|----------|-------------|
| `{{.PR.ID}}` | Pull request id, unique per repository |
| `{{.PR.Title}}` | Title |
| `{{.PR.Description}}` | Raw Markdown description, exactly as Bitbucket stores it: a mention is `@{account_id}`, not the display name the detail pane shows |
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

### Changed files

`changedFiles: true` fills `{{.Files}}` with **every** file the pull request touches, in diffstat order — the same list the Files tab shows, not just the one under the cursor. Each entry carries the file-scope fields:

| Variable | Description |
|----------|-------------|
| `{{.Files}}` | The changed files, in diffstat order; empty for a pull request that changes nothing |
| `.Path` | Repository-relative path, the new side for a rename |
| `.OldPath` | The path before a rename; empty otherwise |
| `.Status` | `added`, `removed`, `modified` or `renamed` |
| `.Line` | Always `1` — the list names files, not positions |

The list is not fetched with the pull request. The first press of such a key fetches the diffstat (the help bar says `<name>: loading changed files…`) and runs the command when it arrives; after that it comes from the same cache the Files tab uses, and a push to the source branch invalidates it. A failed diffstat — Bitbucket's "diff too large" included — reports on the status panel and runs nothing.

An example — open every file the pull request changes in nvim, one tab per file, skipping the ones it deleted:

```yaml
customCommands:
  - key: "ctrl+e"
    name: "Open changed files in nvim"
    changedFiles: true
    contexts: [pullrequests, detail, diff]
    command: >-
      cd {{.GitRepoPath | shellescape}} &&
      nvim -p {{range .Files}}{{if ne .Status "removed"}}{{shellescape .Path}} {{end}}{{end}}
```

The paths are repository-relative, so the command has to `cd` into a checkout of that repository first — `{{.GitRepoPath}}` is the one lazybitbucket was started in, which is the right one only when it is the pull request's repository.

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
