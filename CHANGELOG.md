# Changelog

All notable changes to this project will be documented in this file.

Format based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
This project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- The build results of the rows in the list arrive together. The status requests for the first 15
  rows now run at the same time, not one after the other, so the builds column is complete about
  0.6 s after the list and not 5 s after it. The number of requests is the same
- The pull request list asks up to eight repositories at the same time, not two. With eight
  repositories and no repository filter, the rows show after about 1.5 s and not 3 s. The step from
  one repository to All repositories gains the same
- The rows whose build results are read ahead are the rows on screen. With a repository filter set,
  the rows that the filter hides no longer take the place of the visible ones

## [0.1.1] - 2026-11-12

### Fixed

- The pull request list reads the repository that is in scope from every place that changes the tab
  or the repository filter. Before, a repository picked in the pipelines view left the pull request
  list empty after the switch back, `L` on a run of that repository said `is not in this tab`, and a
  tab with its own state or query kept the rows of the old scope. `L` now says that the pull request
  is still loading while its repository is read
- A refresh replaces the rows of a repository under the name that was asked for. A repository whose
  pull requests spell the slug differently no longer gains the same rows again on each refresh
- A repository that was out of scope for longer than `refreshInterval` is read again when you return
  to it. Before, it kept the rows of its first read
- The warning about a short `refreshInterval` counts the repositories that the refresh reads, which
  are the ones in scope, and goes away when the scope becomes small enough

### Changed

- The version of the app shows in the bottom border of the Status panel, where the list panels show
  their position. It showed at the right end of the help bar, where it took the room of one to three
  key hints on a terminal narrower than about 180 columns
- The version reads the same on every surface: `v0.1.0` in the Status panel, on the splash and from
  `lazybitbucket version`. A build that has no version number shows `dev-` and the commit; this
  includes a Nix build, which knows only the commit

### Removed

- The release pipeline that came with lazyjira: the `release` workflow, `.goreleaser.yml`, the AUR
  `PKGBUILD`, `make release`, and the Homebrew, AUR, deb, rpm, apk and Windows install notes and the
  release badge in the README. This project does not publish releases. A `v*` tag releases nothing;
  `make tag V=0.2.0` creates one after it checks that this file has a section for the version

## [0.1.0] - 2026-11-11

lazyjira is now lazybitbucket: a terminal UI for Bitbucket Cloud pull requests. The Bubbletea shell
(panels, layout, theming, keybinding config, custom commands, git integration, packaging) is
inherited; the domain layer is new. Versioning restarts for this project — the entries below this
one are upstream lazyjira history, and their compare links still point at upstream.

### Added

- The version of the app shows in dim gray at the right end of the help bar. A narrow terminal drops
  it so the hints keep their room. `make build` takes the version from the git tag (`git describe`),
  and `lazybitbucket version` prints the same. A build without a tag shows the commit; `go install`
  shows the module version
- `v` marks a changed file as viewed, on the Files tab and in a file diff, and goes to the next file
  that you have not viewed; `v` on a viewed file clears the mark. The Files tab shows a green `✔`
  and counts the marks in its label (`Files (5 · 2 viewed)`). A push that changes a file removes its
  mark: one diffstat request between the old and the new head finds the changed files, and the other
  marks stay. Bitbucket's public API has no viewed-file state, so the marks stay on this computer, in
  `viewed.json` in the config directory. `v` writes nothing to Bitbucket and works in read-only mode.
  The key is `keybinding.detail.toggleViewed`
- `T` resolves the conversation in hand and reopens it — one key in two directions, like `A`, with a
  dialog that names which. Bitbucket hangs the state off the comment that starts a thread, so it acts
  on the whole conversation whichever of its comments is under the cursor, and it works on the
  Comments tab and inside an open thread alike. What the answer records — who resolved it and when —
  is folded straight in, so the `✔` appears without a refetch; an answer that carries no
  resolution document still marks it resolved, against whoever pressed the key. Like commenting it is
  allowed on a merged or declined pull request
- Resolved conversations are off the Comments tab from the start — a thread someone has finished
  with is not what the tab is for — and `f` asks for them back. The tab label says how many are out
  of sight (`Comments (8 · 2 hidden)`); a resolved conversation that is shown is marked with a green
  `✔`, the name of whoever ended it and when, and its anchor is drawn gray rather than cyan.
  `gui.showResolvedComments: true` starts the other way round. Everything that indexes the list —
  the cursor, `enter`, `T`, `C`, `o`, `y` — follows what is on screen; jumping to a hidden
  conversation from a `●` in a diff turns the filter off rather than landing on a list without it,
  and a conversation open in the thread section stays on screen when it is resolved
- `N` opens a pull request for the checked-out branch: one form for the title (prefilled from the
  last commit's subject), the target (the repository's own default branch), the reviewers (its
  default reviewers, nobody, or a checklist over them and the workspace members), a draft flag and a
  description written in `$EDITOR`. A branch the remote does not have yet is offered a push first —
  Bitbucket cannot open a pull request for a branch its own repository has never seen — and a push
  that fails stops the create with git's own message. What is created goes to the top of the open
  lists with the cursor on it and the detail panel in it. It is refused outside a checkout, in a
  checkout of a repository the configuration does not name, on a detached HEAD and in read-only mode
- `C` on a line of a file's diff writes an inline comment anchored to it, on the side that line lives
  on — the new side for an added or context line, the old side for a removed one. The popup is headed
  with the line it is anchored to, sending leaves the cursor exactly where it was with the line's `●`
  marker lit up, and the hint bar says where the comment landed: a review is a pass down a file, and
  being thrown into a thread after every remark would break it. A line that already carries a
  conversation takes a second one — joining the first is `enter` and then `C`. A hunk header, and a
  row of the Files tab, comment on the file itself, which is where Bitbucket shows them. Multi-line
  ranges are not implemented
- `C` writes a comment: a reply to the conversation on screen or the thread under the cursor on the
  Comments tab, a new comment on the pull request from the Overview tab. It asks the way `E` asks for
  a title — the same input popup, headed with what the comment answers or hangs off — and it takes
  every key while it is open, `enter` sends, `esc` drops it and `ctrl+e` continues in `$EDITOR` for a
  comment that wants paragraphs (in the title editor that key stays readline's end-of-line). What Bitbucket answers with is folded straight into the thread, rendering and all, so
  a new comment needs no refetch and its mentions read as names; a new conversation is opened as
  soon as it exists. Nothing typed is lost to a failure — the text goes back into the box, or stays
  in the editor's file with the hint bar naming it. Unlike the other writes it is not refused on a
  merged or declined pull request, and like them it is refused, and unlisted, in read-only mode.
  Editing, deleting and resolving are still the browser's job
- `enter` on a review thread opens the conversation in the detail panel — the same drill-down a
  changed file's diff and a pipeline step's log get — instead of a read-only modal over the layout.
  The list still collapses a thread longer than eight lines; the section shows it whole and scrolls,
  `backspace` comes back to the Comments tab with the cursor on the thread just read, `o` and `y`
  target that comment, and a tab key leaves for its tab. It is also where replying to a comment will
  go, which a modal had nowhere to put
- `@`-mentions in a description and in review comments read as the display name — `@Grace Hopper`,
  painted in the theme's `blue` — rather than the `@{account_id}` Bitbucket stores. The names come out of the rendered html served
  next to the raw Markdown, so nothing extra is fetched, and an occurrence Bitbucket itself left
  literal — a departed colleague, an id written inside a code span — stays an id, exactly as the web
  UI shows it. Read side only: composing a mention is still out of scope, and everything that
  round-trips the text (the description editor, `{{.PR.Description}}`) keeps the raw form
- Moving the focus onto the detail panel fetches its Files, Commits and Comments tabs together
  instead of one per first tab entry: a reader who moves in is about to walk the tabs, and three
  requests now beat a pause on each of them later. A tab entered while its fetch is still in flight
  waits for that answer rather than asking again, a tab whose prefetch failed says so when you open
  it rather than the moment it failed, moving the cursor in the list still fetches no tab at all,
  and `gui.prefetchDetailTabs: false` goes back to fetching each tab on first entry
- The Comments tab carries a number from the start: the pull request's own `comment_count` before and
  while the threads load, and the number of comments actually loaded — roots and replies, deleted
  placeholders excluded — once they are in. Files and Commits are unchanged; no count comes with a
  pull request for either
- Started inside a checkout whose remote points at one of the listed repositories, the pull request
  list opens scoped to that repository, with the cursor on the pull request for the branch you have
  checked out — in whichever tab holds it, which it says (`selected #103 in Reviewing — the pull
  request for bugfix/upstream-timeout`). It is attempted on every list that lands until it succeeds,
  because the Mine and Reviewing tabs are empty until Bitbucket answers who you are, and it stops for
  good once you move the cursor or switch tabs yourself
- Screen modes as in lazygit: `+` / `_` cycle normal → half (side panels take half the width) → full
  (the focused panel fills the window); `gui.screenMode` picks the mode to start in
- Kanagawa themes: `kanagawa-wave`, `kanagawa-dragon` and `kanagawa-lotus` presets, and theme
  families — `kanagawa`, `catppuccin` and the existing `auto` — that pick the dark or the light
  variant from the terminal background. Under `tmux` and `screen`, where the terminal cannot be
  asked, the macOS system appearance decides; `rendererStyle: auto` follows the same detection.
  The palette gained an `accent` role for the chrome (titles, focused border, active tab, key
  hints, dialog frames): every existing preset keeps it equal to green, Kanagawa uses its blue
- Inside tmux, `e` opens the file at the cursor's line in a new tmux window, started in the
  checkout's root and named after the file, instead of suspending the session; `editorInTmux:
  inline` restores the suspend-and-return behaviour. The description editor still runs inline
- `diffRenderer` hands the diff pane's code column to an external program — `diffRenderer: delta
  --color-only --paging=never` — modelled on lazygit's `git.paging.pager`. The program reads a
  file's unified diff on stdin and must print one line per line it reads; the gutters, `+`/`-` and
  review markers, cursor and `e` stay lazybitbucket's. It runs once per file opened, off the event
  loop, and is cached with the diff. A failure falls back to the built-in chroma highlighting and
  is reported on the status panel; a program that is missing or whose output does not line up is
  turned off for the session. `rendererStyle: notty` turns it off, and chroma remains the default

- `pkg/bitbucket`: a Bitbucket Cloud REST client — API tokens and access tokens (Bearer) or Atlassian
  email + API token (Basic), pagination that follows `next`, typed `APIError` with not-found /
  unauthorized / forbidden / rate-limited predicates, and a concurrent multi-repository pull request
  fetch. Its writes are merging and editing a pull request, approving one, and rerunning or stopping
  a pipeline run; declining, commenting and running an arbitrary custom pipeline have no methods at all
- `pkg/bitbucket/bitbuckettest`: strict fake client for unit tests
- `pkg/bitbucket/demo`: a deterministic demo dataset and fake REST server, an ordinary untagged
  package, used by `--demo` and by integration tests
- Pull request list across all configured repositories, with configurable columns (repo, id, title,
  author, approvals, build, updated, branch, state) and Open / Mine / Reviewing tabs
- Pull request detail pane: branches, author, reviewers with approval state, and the description
  rendered from Markdown with glamour
- Builds panel showing the commit statuses of the selected pull request, with drill-down into the
  pipeline run and its steps
- Repos panel that filters the pull request list to a single repository
- Repository discovery when `bitbucket.repositories` is not configured
- `refreshInterval` for a background refresh of the active tab
- Pipeline drill-down reaches each step's log, and `backspace` walks back out (step log → pipeline →
  pull request)
- Pull request detail is split into four tabs — Overview, Files, Commits, Comments — on `i`, `d`, `m`,
  `t` and the `[` / `]` cycle. Each tab's data is fetched the first time it is opened and cached per
  pull request, and dropped when the source branch moves
- Diff pane: the changed files with status, `+/−` counts and binary marker, and a per-file unified
  diff with a line cursor, syntax highlighting through chroma (`monokai` / `github`, following
  `rendererStyle`), and `n` / `p` to walk the changed files
- `e` opens the file under the cursor in `editor` (`$VISUAL`, `$EDITOR`, then `nvim`) at the
  cursor's line, resolved against the git repository lazybitbucket was started in. Line numbers are
  passed in the dialect the editor understands (`+LINE`, `--goto path:LINE`, `path:LINE`)
- Commits tab: hash, author, age and subject per commit; `enter` expands the full message
- Comments tab: pull request comments read as threads — root plus replies, anchored to a file and
  line when inline, with resolved, pending and deleted states. Commented lines are marked `●` in the
  diff, and `enter` on such a line jumps to its thread
- `o` and `y` follow the active tab: the selected commit, the selected thread, or the pull request's
  diff from inside a file diff. The URL picker gained Commits and Comments sections
- `diff` custom command context, active on the Files tab and inside a file diff, with a `File` scope
  (`{{.File.Path}}`, `{{.File.OldPath}}`, `{{.File.Status}}`, `{{.File.Line}}`)
- `pkg/diff`: a lenient unified-diff parser (renames, added/removed files, binary files, missing
  hunk counts, `\ No newline at end of file`, quoted paths)
- `pkg/bitbucket`: `ListPullRequestCommits`, `ListPullRequestComments`, `ListPullRequestDiffStat` and
  `GetPullRequestDiff`, thread building from the flat comment list, and `IsDiffTooLarge` for
  Bitbucket's 555 "diff too large" answer
- Merge a pull request from the list or the detail panel (`M`): a dialog picks one of the six strategies
  the API accepts (merge commit, squash, fast-forward, squash fast-forward, rebase fast-forward, rebase
  merge), toggles close-source-branch and takes an optional commit message. Merges are requested asynchronously
  (`async=true`): the `202 Accepted` task is polled at once, then every two seconds for up to a minute, and
  reported when it lands
- Sync the source branch with its target (`S`), by merge or rebase, driven through git in the local
  checkout because Bitbucket's "Sync now" has no public endpoint. It needs the source branch checked
  out and a clean tree, picks the remote whose URL points at the pull request's repository (else
  `origin`), pushes with `--force-with-lease` after a rebase, and aborts a conflicting merge or rebase
  before reporting it
- Edit the title or the description (`E`): the title inline, the description in the configured `editor`
  through a temporary Markdown file, with an unchanged description sending nothing
- Set and remove reviewers (`V`): a checklist over the current reviewers, the pull request's reviewing
  participants, the repository's effective default reviewers and the workspace members, deduplicated
  and without the author, filterable with `/`
- Toggle draft state (`D`), behind a confirmation
- Approve a pull request, or withdraw the approval you already gave, on one key (`A`) behind a
  confirmation that always names the direction it is about to take. Which direction that is comes
  from the participant list, which is read from the server first when the selected row does not
  carry it, so a stale view cannot silently unapprove. Your own pull request is refused locally —
  Bitbucket answers that one `400` — and credentials with no account behind them (an access token)
  get an approve-only key
- Rerun a finished pipeline run (`P`) and stop a running one (`X`), from the Builds panel and from the
  detail panel while it shows a pipeline run or a step log. A rerun repeats the original run's target —
  same commit, same selector, same pull request — and the new run is opened in the detail panel;
  a stop signals the run and reports `stop requested`. Both read the run back before they ask, so the
  dialog names its real state, and both refuse locally what the run cannot do (rerunning one that is
  still going, stopping one that is over). They need the `write:pipeline:bitbucket` scope, are bound
  under the new `keybinding.builds` group, and are refused in read-only mode like the pull request
  writes
- A second view, on `shift+tab`: the pipelines view lists the recent runs of every configured
  repository, newest first, instead of the pull request list — branch builds, scheduled and manual
  runs included, which the Builds panel never showed because no pull request owns them. The detail
  panel is shared, so a run drills into its steps and each step's log exactly as before, and `P` / `X`
  work there with no pull request loaded at all; `L` jumps from a run back to the pull request it ran
  for. The Repos filter narrows the list, `/` filters it, `R` refetches it — never the background
  tick, which would cost a round of requests per repository. `gui.startView: pipelines` opens there,
  `keybinding.universal.nextView` rebinds the switch and the new `keybinding.pipelines` group rebinds
  `L`. Custom commands do not fire in this view
- Read-only mode — `bitbucket.readOnly: true`, the `--read-only` flag or `LAZYBITBUCKET_READ_ONLY=1`,
  none of which can turn it off again. The client refuses every non-GET request with `ErrReadOnly`
  before a socket is opened, the status panel shows `read-only`, and the six pull request action keys
  and the two pipeline ones disappear from the help surfaces and answer with a hint-bar message.
  Sync is refused too: it ends in a push
- `pkg/bitbucket`: `FetchPipelines` and `MergePipelines` — the cross-repository pipeline fetch behind
  the pipelines view, modelled on the pull request one: one result per repository in input order, a
  failure isolated to its own repository, and a merge into one list newest run first
- `pkg/bitbucket`: `MergePullRequest`, `GetMergeTaskStatus`, `UpdatePullRequest`,
  `ApprovePullRequest`, `UnapprovePullRequest`, `ListEffectiveDefaultReviewers`,
  `ListWorkspaceMembers`, `RunPipeline` and `StopPipeline`, plus `MergeStrategy` with the six
  documented values, `MergeOptions`, `PullRequestUpdate`, `PullRequest.ReviewerUUIDs` and the
  `IsReadOnly` predicate
- `pkg/git`: `IsClean`, `RemoteFor`, `Fetch`, `MergeBranch`, `RebaseOnto`, `Push` and `ErrConflict`
- `pkg/bitbucket/demo`: write routes on the fake server (`PUT`/merge/merge task status,
  `POST` and `DELETE` on approve, running and stopping a pipeline, effective default reviewers,
  workspace members) with production-shaped validation, and two
  designated fixtures — `api-gateway` #103 answers the queued `202` and its task, `web-app` #41
  answers the `409` of a lost race
- `--demo` (needs a `-tags demo` build), `--workspace`, the repeatable `--repo` flag and `--read-only`

### Fixed

- A resolved conversation is recognised as resolved. Bitbucket marks one in a collection by sending
  `"resolution": {}` — the object, with every member stripped — and sends no resolution member at all
  on a conversation that is open, so reading the members to decide (as this did) made every resolved
  thread look open: no `✔`, and nothing for the filter to hide. The presence of the object is now the
  whole signal, and the comment list asks for `+values.resolution.user`,
  `+values.resolution.created_on` and `+values.resolution.type` so the marker can also say who
  resolved it and when. Verified against the API on 2026-09-16; the demo server now strips the
  members the same way, so the trap is reproduced rather than papered over

### Changed

- A start inside a checkout asks only for the pull requests of that repository, which is one list
  request and not one for each configured repository. The other repositories are read when you pick
  them, or all of them when you pick All repositories. Until a repository is read, the Repos panel
  shows no count for it, and no total
- A keybinding set in the config now takes its key from whatever default held it, instead of leaving
  two actions on one key for `Keymap.Match` to pick between by map order. An action left with no key
  at all is simply unbound, and an action's other keys are untouched
- `c` fetches the source branch from the remote that points at the pull request's repository before
  checking it out, fast-forwards a local branch the remote has moved past, and reports a diverged one
  instead of touching it

- Renamed `cmd/lazyjira` to `cmd/lazybitbucket`; the binary and the nix attribute are
  `lazybitbucket`
- Config directory moved to `~/.config/lazybitbucket`; the override env var is now
  `LAZYBITBUCKET_CONFIG_DIR`
- Config schema replaced: `bitbucket` (workspace, repositories, email, baseUrl), `pullRequestTabs`,
  `gui.pullRequestListFields`, `refreshInterval`. Credentials are an API token or access token, in
  `auth.json` or `BITBUCKET_TOKEN`
- Custom command contexts are now `pullrequests`, `builds`, `repos`, `detail` and `diff`, with `PR`,
  `Repo`, `Build` and `File` template scopes
- Keybinding groups are `universal`, `navigation`, `pullRequests`, `builds` and `detail`; `builds`
  holds the two pipeline actions `rerun` and `stop` (`P`/`X`) and nothing else; `detail` gained
  `overviewTab`, `filesTab`, `commitsTab`, `commentsTab`, `nextFile`, `prevFile` and `openEditor`.
  There is no `repos` group any more: the Repos and Builds panels reuse `pullRequests.select`,
  `pullRequests.open` and `pullRequests.focusRight`, so those bindings can no longer be silently
  overwritten
- Panels are Status / Pull Requests / Builds / Repos on `1`–`4`, detail on `0`
- The client is no longer read-only by construction. It issues `POST` and `PUT` for the merge and
  update endpoints, through a write path that never follows a redirect and never retries a
  non-idempotent request, and a client built with `ClientOpts.ReadOnly` refuses all of them with
  `ErrReadOnly` before the request exists. Read-only is now a mode rather than a missing feature
- `APIError` carries `error.fields`, and `UserMessage()` appends the first per-field validation
  message in sorted key order, so a rejected write reads `title: This field is required.` instead of
  the envelope's generic `Bad request`
- `keybinding.pullRequests` gained `merge`, `sync`, `edit`, `reviewers` and `toggleDraft`
  (`M`/`S`/`E`/`V`/`D`), and `bitbucket` gained `readOnly`
- Docs rewritten for the Bitbucket workflow; `docs/Custom_Fields.md` removed

### Removed

- Everything Jira: `pkg/jira`, JQL search and history, issue create/edit/transition, comments,
  the info panel and custom fields, ADF conversion and rendering, mentions, issue hierarchy, and
  Jira Server / Data Center and mTLS support
- App-password authentication is not implemented — Atlassian removed app passwords on 2026-07-28

## [2.19.2] - 2026-07-08

### Fixed

- Nix: importing the package outside a flake now works, and cross compilation is possible. `nix/package.nix` used to import `nixpkgs` without a `system`, which broke every manual import. It now takes `lib` and `buildGoApplication` as inputs instead of importing `nixpkgs` itself, and a new `nix/build-go-application.nix` resolves `buildGoApplication` from the pinned `flake.lock` for non flake users (#103)

## [2.19.1] - 2026-07-02

### Added

- Nix: Create a standalone `lazyjira` package and shell for non flake user (#96)

### Fixed

- Confirming a search in the issue list now moves the preview and the current issue to the ticket the cursor lands on, routing the selection through the same path as normal navigation. Actions taken right after a search target the searched ticket instead of the one that was selected before the search (#104)

## [2.19.0] - 2026-06-22

### Added

- `@Name` in a comment or description now resolves to a real Jira mention on submit when exactly one user of the issue's project matches that name. Other names stay as literal text, code spans and fenced blocks are left untouched, and existing mentions are not rewritten. This is Cloud only, Data Center writes are unchanged. When the project's user cache is still cold the write is deferred one tick to fetch users, falling back to literal text on error (#95)

## [2.18.0] - 2026-06-19

### Added

- Color themes with optional auto-detection. `gui.theme` selects a bundled palette: the original `default` ANSI 16 look, or the Catppuccin presets `catppuccin-latte` (light), `catppuccin-frappe`, `catppuccin-macchiato` and `catppuccin-mocha` (dark). `theme: auto` inspects the terminal background at startup and picks `catppuccin-mocha` or `catppuccin-latte` to match. Omitting the key or leaving it empty keeps the legacy ANSI 16 palette, so existing configs are unchanged. An unknown name is an error, and hex-based presets need a truecolor terminal (#88)
- Per-color palette overrides on top of any preset, through three optional maps. `themeColors` applies to every preset, while `themeDark` and `themeLight` apply only on dark or light variants, so a single `auto` config can carry both. Precedence is preset, then `themeColors`, then `themeDark` or `themeLight`. Ten keys are overridable (`green`, `blue`, `red`, `yellow`, `cyan`, `magenta`, `orange`, `white`, `gray`, `highlight`) and accept hex, ANSI 16 or ANSI 256 values. Unknown keys and empty values are ignored, and an unrecognized color falls back to the terminal default with a warning shown under `--debug` (#88)

## [2.17.0] - 2026-06-15

### Added

- Create subtasks under the selected issue, from the task list and the Info panel's Sub tab. The parent is set automatically and the type picker is limited to subtask types. The action appears only where a subtask is valid, so epics and existing subtasks are excluded. The key is configurable like the other issue actions (#94)

### Changed

- The create form now names which required fields are still empty on submit, instead of only counting them, so it is clear what to fill in before the request goes out (#94)
- Reporter is now pre-filled with the current user when creating an issue or subtask, matching Jira's own create defaults. It can still be changed or cleared (#94)
- Create errors now split into pre-form and submit stages. A failed fetch of the parent's issue types aborts with a readable message instead of opening an empty form, while submit errors stay inline. Messages carry Jira's own error text instead of an internal request wrapper (#94)

## [2.16.1] - 2026-06-11

### Changed

- Full unit-test suite across the whole repository, raising coverage from about 50% to about 80% (stdlib `testing`, table-driven cases, race detector). New test linters are enforced in `.golangci.yml` (thelper, tparallel, usetesting, testableexamples, paralleltest) (#92)
- CI runs the test suite with coverage and publishes the total percent to the workflow run summary

### Fixed

- Data race in the author color cache. The shared color map was written without a lock, so rendering many colored author names at once (comments and history) could hit a concurrent map write and crash. Lookups are now guarded by a read-write mutex with double-checked locking (#92)

## [2.16.0] - 2026-06-09

### Added

- Optional Glamour renderer for ADF previews in the detail view and the create form. Set `renderer: glamour` in config.yml to route descriptions and comments through the adf-converter display module and Glamour for richer styling of headings, lists, code blocks and colored text spans. The builtin renderer stays the default. `rendererStyle` (auto, dark, light, notty) selects the Glamour theme. Auto detects the terminal background via lipgloss and can be set explicitly when detection fails under tmux or ssh (#80)
- `--debug <file>` flag that writes structured debug logs to a file for troubleshooting editor launches and other actions (#82)

### Fixed

- Editors configured with arguments in `$EDITOR` or `$VISUAL` now launch correctly. The value is split shell-style into a binary and its arguments, so `EDITOR="code --wait"` or `EDITOR="nvim --cmd 'set ft=md'"` work instead of trying to exec a binary named after the whole string (#82)

## [2.15.0] - 2026-05-21

### Added

- Pluggable ADF-to-Markdown converter. The roundtrip now goes through an `ADFConverter` interface with two implementations: the builtin converter (default and fallback) and `adf-converter` v0.1.0. Set `converter: adf-converter` in config.yml to enable the placeholder-based converter for lossless preservation of panels, tables, status badges and other complex elements. The active converter flows through the full edit lifecycle, including the create-form description preview (#74)

### Changed

- CI now runs Dependabot updates, `govulncheck` vulnerability scanning, GitHub dependency review and a `make check-demo` build check. The `golangci-lint` version is pinned via go tool directives (#76)

## [2.14.0] - 2026-05-15

### Added

- Custom icons for issue priorities in the list view. Configure via `gui.priorityIcons` map in config.yml. Plain text works too.
- `slugify` template function for custom commands. Use in templates like `git switch -c wip/{{ .Summary | slugify }}` to produce URL- and branch-safe slugs. German umlauts (ä→ae, ß→ss) and other accented letters are transliterated to ASCII (#72)

### Changed

- Branch generation with `git.asciiOnly: true` now transliterates German umlauts (ä→ae, ß→ss) and strips other accents (é→e) instead of dropping non-ASCII characters. Issue types like "Lösung" now produce "loesung" instead of "Lsung" (#72)

## [2.13.0] - 2026-05-05

### Added

- Walk through issue hierarchy from the issues list. Space opens an issue's children, Backspace opens its parent, Esc pops back to the previous view. Each step pushes a snapshot of the panel state so cursor, focus and Info tab are restored on the way back. Children walks also work from the Sub and Lnk tabs in the Info panel. Shares a single ad-hoc tab next to the existing ones, similar to the JQL tab (#68)

### Fixed

- Mouse click and wheel scroll on the issues list now update the active issue. Browser open, copy URL, transition and other actions used to target the previously selected issue because the mouse path skipped the canonical selection update
- Demo build (`make check-demo`) failed to compile after the parent-link children feature landed. The demo client now implements `GetChildren` and returns subtasks of the requested parent

## [2.12.0] - 2026-05-03

### Added

- Custom icons for issue statuses in the list view. Configure via `gui.statusIcons` map in config.yml. Plain text works too (#64)

### Fixed

- Sprint field shows None on Jira instances where the `sprint` alias does not resolve to the real custom field id. The id is now discovered at startup via `/field` and used in both reads and writes. Affects older Server/DC and other instances that map sprint to non-default ids like `customfield_10010` (#48)

## [2.11.1] - 2026-04-30

### Fixed

- Branch modal: names that do not match an existing remote ref now create a local branch. Previously any `/` in the input forced remote tracking, breaking prefixed conventions like `feature/PROJ-1-foo` (#62)

## [2.11.0] - 2026-04-30

### Added

- Catppuccin theme support. Pick from Latte, Frappé, Macchiato or Mocha via `gui.theme` in config. Default ANSI palette stays the same. Catppuccin flavors use hex colors and need a terminal with truecolor support (#59)

### Changed

- CI step names mirror the `make` targets they run. Failed jobs now point at the exact command to run locally
- CONTRIBUTING documents how to refresh `gomod2nix.toml` after a Go dependency change. Both `nix develop -c make nix-deps` and `go install gomod2nix` paths are listed. Skipping the refresh fails the nix CI job with a checksum error (#60)

## [2.10.2] - 2026-04-26

### Changed

- Go module path updated to `github.com/lsst25/lazybitbucket` to follow Go modules v2+ convention

## [2.10.1] - 2026-04-18

### Fixed

- String shorthand in `projects` list (e.g. `- ORCH` instead of `- key: ORCH`) no longer panics on startup. Both forms can now be mixed in the same list (#53)

## [2.10.0] - 2026-04-18

### Added

- Configurable custom commands. Bind shell commands to keys with Go template access to the focused issue, project or comment. Commands declare which UI contexts they fire in and take precedence over built-in keys. Includes `suspend` flag and `shellescape` template helper (#42)
- `maxResults` option to control how many issues are fetched per query. Can be set globally or per tab in `issueTabs`. Default remains 50 (#45)
- Context-sensitive preview for Sub/Lnk tabs. Moving the cursor in subtasks or links previews that issue in the detail pane. Actions target the previewed issue. Preview resets when leaving the tab (#55)

## [2.9.0] - 2026-04-14

### Added

- Custom icons for issue types in the list view. Configure via `gui.typeIcons` map in config.yml (#38)
- `gui.collapsedPanelHeight` option to set the height of non-focused side panels. Default is 5 lines which was often too small to read the info panel without switching focus (#46)

## [2.8.2] - 2026-04-13

### Fixed

- Issue rows no longer wrap to two lines when the panel is narrow. Extra content is cut off instead
- Updated column now lines up vertically. Summary column was not padded so dates floated left on short summaries

## [2.8.1] - 2026-04-10

## [2.8.0] - 2026-04-10

### Added

- Search and filter in keybindings help popup. Press `/` in the `?` menu to filter by key or description (#15)
- Scroll detail panel without switching focus. `J`/`K` scrolls one line, `ctrl+f`/`ctrl+b` scrolls half page from any left panel (#20)
- Navigation keys (`j`/`k`/`g`/`G`/`ctrl+d`/`ctrl+u`) are now configurable via `keybinding.navigation` in config.yml

## [2.7.4] - 2026-04-10

### Added

- Optimistic UI updates. Fields update on screen instantly without waiting for the API
- All paginated API methods fetch every page. Boards, sprints, changelog and labels were capped at one page
- Custom fields without predefined options fall back to inline input or external editor instead of showing an error
- `SetBuiltinFieldValue` and `PatchIssueFields` helpers for consistent field patching across issue list and info panel

### Fixed

- Overlay stack now renders all visible layers so stacked modals display correctly
- `PatchIssue` syncs all fields including sprint, labels, components and custom fields

## [2.7.3] - 2026-04-10

### Added

- Branch name templates support `{{.ParentKey}}` for subtask workflows like `PROJ-100/PROJ-142_summary` (#34)
- Documented `{{.ProjectKey}}`, `{{.Number}}` and `{{.Type}}` template variables that were available but missing from docs (#34)

### Fixed

- Assignee list was capped at 100 users. Now fetches all pages so large orgs see everyone (#35)

## [2.7.2] - 2026-04-03

### Added

- Edit custom fields in info panel. Supports select, multiselect, person, text and textarea
- Field type auto-detected from Jira value. Can also set `type` in `fields` config
- Fields with explicit `type: text` or `type: textarea` skip the CreateMeta API call
- `multiline` option in `fields` config opens external editor for text fields
- CreateMeta cached per project and issue type so repeated edits skip the API call
- Enter on linked issue opens it in detail view without leaving the info panel
- Space on linked issue navigates to it in the issues list
- Space on issues panel opens issue detail
- Info panel label width adjusts to fit custom field names
- None and Unknown values shown in gray
- Ctrl+J / Ctrl+K as alternative navigation keys everywhere (lists, modals, search)

### Fixed

- Person field edit could update the wrong issue if you changed selection before picking a name
- Auto-refresh no longer jumps detail and info panels when you are focused elsewhere
- Editing a field not available in issue metadata now shows an error instead of an empty input

### Removed

- Issue select feature with star marker and pin-to-top

## [2.7.1] - 2026-04-02

### Changed

- Space key disabled in issues panel. Enter is the only way to open issue detail (#25)
- Enter in projects panel now selects the project, same as space
- Double-click, git branch detection and issue creation no longer mark issue as active

### Fixed

- Detail panel updates when switching issue tabs with `[]`
- Detail panel shows first issue after selecting a project instead of staying on project preview
- Projects list limited to 100 items on Jira Cloud. Now fetches all projects with pagination

## [2.7.0] - 2026-04-01

### Added

- Create issues from TUI (n in issues panel, ctrl+n to duplicate): two-phase overlay with type picker and field form
- Configurable issue tabs with JQL templates and {{.ProjectKey}}/{{.UserEmail}} placeholders
- Create form prefills fields from active tab JQL (e.g. assignee from "Mine" tab)
- Duplicate issue (d key): prefills all fields from the source issue
- Custom field support in create form (select, multiselect, text, number)
- Demo mode: issue creation with in-memory data, create metadata endpoint
- Config: `gui.prefillFromTab` option (default true)
- Auto-select newly created issue in the list. Switches to All tab if the issue does not match the current tab. Config: `gui.selectCreatedIssue` (default true)
- e2e test tape for issue creation flow

### Fixed

- ADF code blocks now wrap long lines instead of breaking the panel border
- ADF headings wrap to fit panel width

## [2.6.8] - 2026-04-01

### Fixed

- AUR PKGBUILD: pkgver() now uses git describe for tag-based versioning per Arch Wiki guidelines

## [2.6.7] - 2026-04-01

### Fixed

- Text wrapping now measures display width of Unicode and emoji instead of counting bytes. Panels no longer overflow with multi-byte characters
- Info panel field values truncated by visual width instead of byte length
- ADF list markers use correct display width for indentation
- Stripped carriage returns from wiki markup and ADF text to prevent terminal corruption with Jira Server line endings

## [2.6.6] - 2026-03-30

### Fixed

- Assignee list: current user now always appears first, matched by account ID instead of email. Fixes cases where Jira Cloud hides emails due to privacy settings (#16)
- Assignee modal now scrolls to keep the cursor visible when navigating long lists
- Selected project now pins to the top of the projects list
- Project keys that are reserved JQL words (like DO, IN, IS) no longer cause search errors

### Added

- Assignable users are cached per project and prefetched in background after project switch

## [2.6.5] - 2026-03-30

### Fixed

- Search backspace now correctly deletes multi-byte Unicode characters instead of producing broken glyphs
- Issues list selection no longer jumps to top after confirming search

## [2.6.4] - 2026-03-30

### Fixed

- Homebrew: switched back from Cask to Formula — Cask quarantines unsigned CLI binaries, causing macOS Gatekeeper to block execution

## [2.6.3] - 2026-03-30

### Fixed

- Homebrew tap: removed stale Formula that shadowed the Cask, causing `brew upgrade` to stay on v2.4.0

## [2.6.2] - 2026-03-30

### Fixed

- Homebrew formula not updating since v2.4.0: switched goreleaser from `homebrew_casks` back to `brews`

## [2.6.1] - 2026-03-29

### Changed

- Nix flake: switched from vendorHash to gomod2nix for reproducible builds
- CI: added nix build check to catch outdated dependency hashes
- CONTRIBUTING: added Nix dev environment section

## [2.6.0] - 2026-03-29

### Added

- Jira Server and Data Center support (REST API v2) with automatic endpoint adaptation
- Client certificate authentication (mTLS): `certFile`, `keyFile`, `caFile`, `insecure` in config
- Setup wizard: choose between Cloud and Server/Data Center, prompts adapt accordingly
- Server/DC uses Bearer PAT auth (no email needed), Cloud keeps Basic auth
- Jira wiki markup rendering: bold, italic, links, headings, code blocks converted to plain text
- Error modal with red border for API errors (previously only shown in status bar)
- Issues list updates immediately after editing summary, status, or assignee
- Config: `serverType` field (`cloud`, `server`, `datacenter`) and TLS settings
- Environment variables: `JIRA_SERVER_TYPE`, `JIRA_TLS_CERT`, `JIRA_TLS_KEY`, `JIRA_TLS_CA`, `JIRA_TLS_INSECURE`
- README: clarified that API token and PAT are the same thing, added Server/DC setup instructions

### Fixed

- Edit Summary: long text now wraps instead of being truncated with "..."
- Edit Summary: space key now works (was silently ignored)
- Edit Summary: cursor at end of full line wraps to next line instead of breaking the border
- Edit Summary: ANSI escape codes no longer split across wrapped lines
- Confirm changes diff view: lines wrap instead of being truncated
- Description editor: opens with content on Server/DC (was empty due to ADF/string mismatch)
- Changelog tab: works on Server/DC (uses `?expand=changelog` instead of separate endpoint)
- Status panel: shows host when email is empty (Server/DC)

## [2.5.1] - 2026-03-28

### Fixed

- Arrow keys (up/down) now work for navigating filtered results during `/` search
- Info panel: cursor stays on the selected element after confirming search with Enter (previously jumped to wrong item)

### Added

- Documentation: [Configuration](docs/Config.md), [Keybindings](docs/Keybindings.md), [Custom Fields](docs/Custom_Fields.md)
- README: documentation links, expanded roadmap

### Changed

- Config: annotated unimplemented options with TODO markers (theme, language, mouse toggle, cache, auto-refresh, etc.)

## [2.5.0] - 2026-03-28

### Added

- Info panel: subtasks, links and fields extracted into a dedicated left panel with three tabs (Info/Lnk/Sub)
- Navigate linked issues and subtasks directly from the info panel (Enter to preview, Space to open)
- Edit fields (priority, assignee, sprint, etc.) right from the info panel with e key
- Sprint management: move issues between sprints via the Agile API (MoveToSprint)
- Info panel has its own keybindings section in help overlay
- Mouse support for the info panel (click, scroll, tab switching)
- Number key 3 focuses info panel, projects moved to 4
- Arrow keys cycle through all four left panels (lazygit style)
- Batch prefetch for issue details

### Changed

- Detail panel tabs simplified: removed Sub, Lnk, Info tabs (moved to info panel)
- Left panel navigation reworked: up/down arrows cycle status/issues/info/projects instead of jumping to detail
- Agile API client refactored: doAgile/doAgileMethod avoid mutating baseURL
- e2e tests consolidated into a single preview tape

## [2.4.3] - 2026-03-27

### Fixed

- Cursor warp on panel switch

## [2.4.2] - 2026-03-25

### Changed

- Release notes now include link to CHANGELOG.md

## [2.4.1] - 2026-03-25

### Added

- CI workflow: golangci-lint + vet + build on PRs and main
- Required status checks on main branch
- GitHub issue templates (bug report, feature request)
- Pull request template
- CONTRIBUTING.md

### Changed

- Homebrew distribution: brews -> homebrew_casks (goreleaser v2)
- Refactored app.go: extracted handlers into handlers_keys, handlers_data, handlers_jql, handlers_modal
- OverlayStack: unified modal intercept/render dispatch for all overlay panels
- DRY helpers for modal, inputmodal, jqlmodal, diffview components
- Unit tests for modal, overlaystack, text utilities

## [2.4.0] - 2026-03-25

### Added

- Git integration: create branches from issues with configurable name templates (b key)
- Git integration: search and checkout existing branches by issue key (B key)
- Branch format rules with conditions by issue type (feat/*, fix/*, fallback)
- Auto-detect current issue from branch name
- CHANGELOG.md

## [2.3.0] - 2026-03-24

### Added

- JQL search modal with two-panel UI (input + suggestions/history) (s key)
- JQL autocomplete: field names and values from Jira API
- JQL syntax highlighting in the search input
- JQL history persistence (plain text file, max 50 entries)
- JQL search results appear as a temporary tab in the issues panel
- Custom readline-style text input with cursor, Home/End, Ctrl+A/E/W/K/U
- `make check` target (lint + vet + build)

## [2.2.0] - 2026-03-21

### Added

- Edit fields: transition, priority, assignee changes from TUI (t/p/a keys)
- Comment viewing and posting (c/n keys)
- Input modal component for text entry
- Diff view component for description change history
- ADF-to-Markdown renderer for rich text display in edit/comment workflows

## [2.1.0] - 2026-03-20

### Added

- Rich ADF (Atlassian Document Format) rendering in issue detail
- Support for mentions, emoji, lists, links, code blocks, inline cards
- Windows installation guide in README

## [1.0.0] - 2026-03-18

### Added

- Panel layout inspired by lazygit: Status, Issues, Projects, Detail
- Jira Cloud REST API v3 integration
- Interactive setup wizard on first launch
- Issue list with All/Assigned tabs
- Issue detail with tabs: Body, Sub, Cmt, Lnk, Info, Hist
- Project switcher with auto-fetch from Jira API
- Transition issues (t key) with modal picker
- URL picker (u key) with in-app navigation for Jira links
- History tab with diff for large field changes
- Author color coding consistent across all views
- Search/filter with / key (per-panel)
- Prefetch and cache all issue details for instant navigation
- Auto-refresh every 30 seconds
- Open in browser (o key), copy URL (y key)
- Mouse support: click panels, scroll, click tabs
- Vertical layout for narrow terminals (< 80 cols)
- Responsive side panel width
- Cross-platform: macOS, Linux, Windows
- Homebrew install via tap

[Unreleased]: https://github.com/lsst25/lazybitbucket/compare/v0.1.1...HEAD
[0.1.1]: https://github.com/lsst25/lazybitbucket/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/lsst25/lazybitbucket/tree/v0.1.0
[2.19.2]: https://github.com/textfuel/lazyjira/compare/v2.19.1...v2.19.2
[2.19.1]: https://github.com/textfuel/lazyjira/compare/v2.19.0...v2.19.1
[2.19.0]: https://github.com/textfuel/lazyjira/compare/v2.18.0...v2.19.0
[2.18.0]: https://github.com/textfuel/lazyjira/compare/v2.17.0...v2.18.0
[2.17.0]: https://github.com/textfuel/lazyjira/compare/v2.16.1...v2.17.0
[2.16.1]: https://github.com/textfuel/lazyjira/compare/v2.16.0...v2.16.1
[2.16.0]: https://github.com/textfuel/lazyjira/compare/v2.15.0...v2.16.0
[2.15.0]: https://github.com/textfuel/lazyjira/compare/v2.14.0...v2.15.0
[2.14.0]: https://github.com/textfuel/lazyjira/compare/v2.13.0...v2.14.0
[2.13.0]: https://github.com/textfuel/lazyjira/compare/v2.12.0...v2.13.0
[2.12.0]: https://github.com/textfuel/lazyjira/compare/v2.11.1...v2.12.0
[2.11.1]: https://github.com/textfuel/lazyjira/compare/v2.11.0...v2.11.1
[2.11.0]: https://github.com/textfuel/lazyjira/compare/v2.10.2...v2.11.0
[2.10.2]: https://github.com/textfuel/lazyjira/compare/v2.10.1...v2.10.2
[2.10.1]: https://github.com/textfuel/lazyjira/compare/v2.10.0...v2.10.1
[2.10.0]: https://github.com/textfuel/lazyjira/compare/v2.9.0...v2.10.0
[2.9.0]: https://github.com/textfuel/lazyjira/compare/v2.8.2...v2.9.0
[2.8.2]: https://github.com/textfuel/lazyjira/compare/v2.8.1...v2.8.2
[2.8.1]: https://github.com/textfuel/lazyjira/compare/v2.8.0...v2.8.1
[2.8.0]: https://github.com/textfuel/lazyjira/compare/v2.7.4...v2.8.0
[2.7.4]: https://github.com/textfuel/lazyjira/compare/v2.7.3...v2.7.4
[2.7.3]: https://github.com/textfuel/lazyjira/compare/v2.7.2...v2.7.3
[2.7.2]: https://github.com/textfuel/lazyjira/compare/v2.7.1...v2.7.2
[2.7.1]: https://github.com/textfuel/lazyjira/compare/v2.7.0...v2.7.1
[2.7.0]: https://github.com/textfuel/lazyjira/compare/v2.6.8...v2.7.0
[2.6.8]: https://github.com/textfuel/lazyjira/compare/v2.6.7...v2.6.8
[2.6.7]: https://github.com/textfuel/lazyjira/compare/v2.6.6...v2.6.7
[2.6.6]: https://github.com/textfuel/lazyjira/compare/v2.6.5...v2.6.6
[2.6.5]: https://github.com/textfuel/lazyjira/compare/v2.6.4...v2.6.5
[2.6.4]: https://github.com/textfuel/lazyjira/compare/v2.6.3...v2.6.4
[2.6.3]: https://github.com/textfuel/lazyjira/compare/v2.6.2...v2.6.3
[2.6.2]: https://github.com/textfuel/lazyjira/compare/v2.6.1...v2.6.2
[2.6.1]: https://github.com/textfuel/lazyjira/compare/v2.6.0...v2.6.1
[2.6.0]: https://github.com/textfuel/lazyjira/compare/v2.5.1...v2.6.0
[2.5.1]: https://github.com/textfuel/lazyjira/compare/v2.5.0...v2.5.1
[2.5.0]: https://github.com/textfuel/lazyjira/compare/v2.4.3...v2.5.0
[2.4.3]: https://github.com/textfuel/lazyjira/compare/v2.4.2...v2.4.3
[2.4.2]: https://github.com/textfuel/lazyjira/compare/v2.4.1...v2.4.2
[2.4.1]: https://github.com/textfuel/lazyjira/compare/v2.4.0...v2.4.1
[2.4.0]: https://github.com/textfuel/lazyjira/compare/v2.3.0...v2.4.0
[2.3.0]: https://github.com/textfuel/lazyjira/compare/v2.2.0...v2.3.0
[2.2.0]: https://github.com/textfuel/lazyjira/compare/v2.1.0...v2.2.0
[2.1.0]: https://github.com/textfuel/lazyjira/compare/v2.0.3...v2.1.0
[1.0.0]: https://github.com/textfuel/lazyjira/releases/tag/v1.1.0
