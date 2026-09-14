# lazybitbucket

## What we are building

A keyboard-driven terminal UI for **Bitbucket Cloud pull requests** — "lazygit, but for Bitbucket PRs".
Browse open PRs, read the diff, walk review threads, check pipeline status, and act on a PR without
leaving the terminal.

This repo is a fork of [textfuel/lazyjira](https://github.com/textfuel/lazyjira) (MIT). The strategy is
deliberate: **keep the TUI shell, replace the domain layer.** lazyjira already solves the boring parts —
Bubbletea app wiring, panes, navigation stack, theming, keybinding config, custom commands, git branch
integration, release packaging. What it does not have is any concept of a pull request.

## Current status

ROADMAP steps 0, 1, 2 and 3 are **implemented**. The domain layer is Bitbucket, `pkg/jira` is gone, the
binary, config dir and packaging are renamed, the review surface — diff pane, editor hand-off, commits
and review threads — is in, and the tool now acts on a PR as well as reading it: merge, sync, edit
title/description, reviewers, draft, approve — and on the pipeline behind a build: rerun and stop —
plus a browse-only read-only mode.

What exists:

- `pkg/bitbucket` — Bitbucket Cloud REST client (auth, pagination, `APIError` + predicates, concurrent
  multi-repo PR fetch, and the same fan-out for pipeline runs — `FetchPipelines` answering one
  `RepoPipelines` per repository in input order, each failure isolated, and `MergePipelines`
  flattening those into `PipelineEntry` values newest run first) plus a small write surface: `MergePullRequest` (`MergeOptions`, `MergeStrategy`
  with six values and `Label()`, `MergeResult.Queued()` for the 202) and `GetMergeTaskStatus`,
  `UpdatePullRequest` (`PullRequestUpdate`: title, description, draft, reviewers),
  `ApprovePullRequest` (POST with no body, answers a `Participant`) and `UnapprovePullRequest`
  (DELETE, answers 204), plus `ListEffectiveDefaultReviewers` and `ListWorkspaceMembers`, and the two
  pipeline writes `RunPipeline` (POST `/pipelines` with the target of an existing run — the rerun,
  answered 201 with the new run) and `StopPipeline` (POST `…/stopPipeline`, 204), guarded by
  `ErrPipelineTarget`, which refuses a target that cannot be turned into a request body before any
  request. Writes go through `writeJSON` → `do` on the no-redirect client (it decodes the answer on
  200 **or** 201, the run endpoint being a 201 Created), are never retried, and are refused with
  `ErrReadOnly`
  (predicate `IsReadOnly`) when `ClientOpts.ReadOnly` is set. `ErrEmptyUpdate` and
  `ErrReviewersUnknown` guard the update body — `Reviewers` is the complete set and is always sent,
  because Bitbucket clears what a PUT omits. `APIError` carries `Fields` (`error.fields`) and
  `UserMessage()` appends the first field message, which is what the TUI shows. `mentions.go` makes
  the `@{account_id}` a mention is stored as readable: `MentionNames` reads the display names out of
  the html Bitbucket serves next to the raw Markdown, and `ResolveMentions` rewrites the raw with
  them — the html is the oracle for the names *and* for which occurrences were rendered at all, so an
  id left literal (a departed colleague, a mention inside a code span) stays literal. A name is
  escaped as numeric HTML entities, which glamour decodes everywhere while honouring only a fixed set
  of backslash escapes; the result is therefore for a Markdown renderer, never for a text round trip.
  `ResolveMentionsFunc` is the same with a hook on each mention's text, which is what the detail pane
  calls; `RenderedContent.Readable` and `PullRequest.ReadableDescription` are the plain form. Also
  `pkg/bitbucket/bitbuckettest` (strict fake client) and `pkg/bitbucket/demo` (deterministic dataset +
  fake REST server, an ordinary untagged package used by `--demo` and by integration tests; its write
  routes mirror production validation, with `api-gateway` #103 the queued-202 fixture, `web-app` #41
  the 409 and the author of the own-approval 400, `api-gateway` #102 the reviewer-not-yet-acted
  approval fixture and `web-app` #40 the one the demo user has already approved; its accounts carry
  the two real account-id shapes and every description and comment fixture carries the html
  `renderHTML` produced for it, mentions included).
- `pkg/diff` — a lenient unified-diff parser (files, hunks, per-side line numbers, renames, binary
  files, `\ No newline at end of file`, quoted paths). Pure, stdlib only.
- `pkg/git` — branch checkout, plus the sync helpers in `sync.go`: `IsClean`, `RemoteFor` (the remote
  whose URL points at `<workspace>/<slug>`, else `origin`), `Fetch`, `MergeBranch`, `RebaseOnto`,
  `Push` (`--force-with-lease` after a rebase) and `ErrConflict` (both operations abort before
  returning). Git runs with `GIT_TERMINAL_PROMPT=0` so a credential prompt cannot hang the UI.
- `pkg/config` — `bitbucket` (workspace, repositories, email, baseUrl, `readOnly`), `pullRequestTabs`,
  `gui.pullRequestListFields`, `refreshInterval`, `editor`, custom command contexts
  `pullrequests`/`builds`/`repos`/`detail`/`diff` (plus the per-command `changedFiles: true`, the one
  scope a command opts into rather than inheriting from its context, because it costs a diffstat
  request — it fills `{{.Files}}` with the whole changed-file list), `keybinding.pullRequests` with
  `merge`/`sync`/`edit`/`reviewers`/`toggleDraft`/`approve`, and `keybinding.builds` with
  `rerun`/`stop` — the only group besides `pullRequests` that carries write actions. Also
  `gui.startView` (`pullRequests` | `pipelines`, validated like `gui.screenMode`),
  `gui.prefetchDetailTabs` (`*bool`, nil means true, resolved by `PrefetchesDetailTabs()` the way
  `CustomCommandConfig.Suspend` is by `ShouldSuspend()`),
  `keybinding.universal.nextView` (`shift+tab`) and `keybinding.pipelines` with the single
  `openPullRequest` (`L`); rerun and stop stay under `builds` because they are one action wherever
  they are pressed.
- `pkg/tui` — two views over a shared detail panel, cycled with `shift+tab` (`views_switch.go`: the
  `appView` type, `cycleView`, `mainPanel`, and the re-point of the detail panel on a switch). The pull
  requests view has panels Status `[1]`, Pull Requests `[2]`, Builds `[3]`, Repos `[4]`, Detail `[0]`;
  the pipelines view swaps the left column for Status `[1]`, Pipelines `[2]`, Repos `[3]` — panel
  numbers are per view. The Pipelines panel (`views/pipelines.go`) lists recent runs of every
  configured repository, newest first, narrowed by the Repos filter; `pipelines_data.go` holds its
  fetch, its debounced selection, the steps drill-down and `L` back to a run's pull request. The list
  is never on the `refreshInterval` tick, and custom commands fire in neither of that view's panels. PR list
  with Open/Mine/Reviewing tabs and configurable columns, Builds panel with drill-down into a pipeline
  run, its steps and each step's log, repo filter, `/` search, URL picker, copy URL, browser hand-off,
  source-branch checkout, custom commands, themes, and a `refreshInterval` auto-refresh of the active
  tab. The detail pane has four PR tabs — Overview (Markdown description via glamour), Files, Commits,
  Comments — fetched on first tab entry, or all three at once when the detail panel takes focus
  (`gui.prefetchDetailTabs`, default on; `focusDetail` in `app.go`, `prefetchDetailTabs` and the
  `reviewInFlight` marks that deduplicate a tab entered mid-prefetch in `review.go` — a failed prefetch
  stays quiet until its tab is opened, and an answer for a head the branch has left is dropped), and
  cached per PR key, stamped with the
  source commit so a push invalidates it. Overview and Comments render the readable body, resolving
  `@{account_id}` through the `summary.html` / `content.html` that came with it, and paint each
  mention in `theme.Mention` (the palette's blue): `views/mentions.go` wraps a mention in U+FFF9 /
  U+FFFB — zero-width for glamour and lipgloss alike — before the Markdown pass and `paintMentions`
  swaps the markers for the style afterwards, re-asserting the colour after glamour's own SGR codes
  and replaying them once the mention closes. Commit messages are not resolved, and neither is
  anything that round-trips the text — the description editor and `{{.PR.Description}}` keep the raw form. Behind the
  Files tab is `ModeFileDiff`: a per-file unified diff with a line cursor, chroma syntax
  highlighting, `●` markers on commented lines, `n`/`p` to walk the files and `e` to open the file
  at the cursor's line in `config.ResolveEditor()` (`editor.go`).
  The six write flows live in `actions.go` on `M` merge, `S` sync, `E` edit, `V` reviewers, `D` draft,
  `A` approve — they work from the list and the detail panel alike, always open a dialog first, are
  refused for a non-`OPEN` PR, while another write on that PR is in flight, and in read-only mode. The
  merge, sync, draft and approve dialogs are `components.FormModal` (choice / toggle / text / note rows);
  reviewers reuse `Modal.ShowChecklist` and the title editor `InputModal`; the description goes through
  a temp `.md` file and `tea.ExecProcess`. A queued merge polls the task every 2s, 30 attempts. `A` is
  one key in two directions: it approves, or removes the approval the current user already gave, and it
  decides from `pr.Participants` — a row that carries none is read back first (`approvalStateMsg`) so
  the dialog title is never a lie, an author is refused locally with `cannot approve your own pull
  request`, and a nil `currentUser` (access token) makes it approve-only. Two further writes act on
  the pipeline behind a build rather than on the PR: `P` rerun and `X` stop, in `pipeline_actions.go`,
  live on the selected Builds row, on the selected Pipelines row, and in the detail panel's
  `ModePipeline`/`ModeStepLog` — and nowhere else. They resolve the run through a `pipelineRef`
  (workspace, slug, build) rather than through the current PR, which is what lets them work in the
  pipelines view where there is none. They track their own in-flight map (`pipelineWrites`, keyed by `pipelineCacheKey` — a PR key
  and a pipeline key share the `ws/slug#N` shape and would collide), read the run back before the
  dialog (`pipelineActionStateMsg`) so a refusal names the run's real state rather than the build
  status's, and do not require the PR to be `OPEN`, because a merged PR's pipeline can still be
  rerun. A rerun drills the detail panel into the run it created. The status panel shows
  `read-only` next to the auth method.
- `cmd/lazybitbucket` — CLI (`auth`, `logout`, `version`;
  `--demo --log --debug --workspace --repo --read-only`), credential wizard.

Hard constraints that still hold:

- **Every write goes through `writeJSON`/`do` and is refused under `ClientOpts.ReadOnly`** with
  `ErrReadOnly`, before a request is built. `writeJSON` sends no body and no `Content-Type` for a nil
  payload, which is what the approve endpoints take. Read-only is browse-only: the TUI blocks sync too,
  because it pushes. Requesting changes, declining, commenting, review tasks and running an arbitrary
  custom pipeline definition still have no methods at all.
- **No test may reach the real API.** Unit tests use `bitbuckettest.FakeClient` or `httptest`;
  integration tests use `pkg/bitbucket/demo`; **every `bitbucket.NewClient` in a test passes a local
  `BaseURL`** — never the default, which is the real host.
- `pkg/bitbucket` and `pkg/bitbucket/demo` import nothing from `pkg/tui` or `pkg/config`.

Not built yet (ROADMAP step 4+): the richer search / saved filters and PR creation of step 4, and
everything left under "Later / maybe" — requesting changes, declining, posting or resolving comments,
review tasks, triggering a custom pipeline definition, a CLI mode. In the pipelines view specifically:
filter tabs over the run list, custom commands, and auto-refresh on the background tick. Also not built: the activity feed,
side-by-side diffs, whole-file views and conflict display (deliberate step-2 non-goals), and
side-by-side conflict resolution for a failed sync — a conflict aborts and is handed back to the user.

## Port plan (done)

| Package | Plan | Outcome |
| --- | --- | --- |
| `pkg/tui` | **Reuse** — the main asset | Kept. Panes renamed and re-shaped for PRs; app/message/layout structure unchanged. JQL, create/edit/transition, mentions, hierarchy, ADF and the navstack were deleted |
| `pkg/jira` | **Replace** with `pkg/bitbucket` | Deleted. `pkg/bitbucket` + `bitbuckettest` + `demo` replace it |
| `pkg/git` | **Reuse** mostly as-is | Kept; branch checkout carries over, and step 3 added `sync.go` |
| `pkg/config` | **Adapt** | New schema; auth is an API token or access token |
| `pkg/ascii` | Reuse | Kept |
| `cmd/lazyjira` | **Rename** | `cmd/lazybitbucket` |

The domain shapes never lined up — a Jira issue is a flat bag of fields, a Bitbucket PR is a composite
of resources fetched separately (detail, statuses, pipelines, steps). That is why the port added panes
rather than re-labelling existing ones, and why the app caches per PR key (`workspace/slug#id`).
`seflue/adf-converter` is gone: Bitbucket descriptions and comments are Markdown, rendered with
`glamour`.

## Build and test

```
go build ./...        # or: make build
go test -race ./...   # or: make test
make check            # lint + vet + build + test
make check-demo       # lint + vet + build with -tags demo
make lint-fix
make tidy
```

`go build ./...` clean and `go test -race ./...` green is the baseline. If a change makes the suite red,
that is a regression, not the starting state.

Go 1.25+ (toolchain declared in `go.mod`). Nix flake and goreleaser config are inherited and work.
`--demo` needs `make build-demo`: only `cmd/lazybitbucket/demo.go` is behind the `demo` build tag.

## Repo setup

- Module: `github.com/lsst25/lazybitbucket` (no `/v2` — versioning restarts for this project)
- `origin` → `git@github.com:lsst25/lazybitbucket.git`
- `upstream` → `git@github.com:textfuel/lazyjira.git`, so `git fetch upstream && git merge upstream/main`
  can still pull their fixes. `pkg/jira` no longer exists here, so upstream changes to it do not apply;
  expect conflicts in `pkg/tui` and `pkg/config` instead.
- **No tags.** The 49 inherited upstream tags were deleted so releases can start clean.
- `fetch.pruneTags = false` is set repo-locally on purpose. The global `~/.gitconfig` sets it to `true`,
  which forces an explicit `refs/tags/*` refspec onto every fetch and would re-sync all 49 upstream tags
  back on the next `git fetch upstream`. `--no-tags` cannot suppress that. Do not "clean up" this override.

## Naming and paths

Everything user-visible is `lazybitbucket`:

- Binary `lazybitbucket`, built from `./cmd/lazybitbucket`.
- Config dir resolution (`pkg/config`): `LAZYBITBUCKET_CONFIG_DIR` → `$XDG_CONFIG_HOME/lazybitbucket` →
  `os.UserConfigDir()/lazybitbucket` → `~/.config/lazybitbucket`. Files: `config.yml`, `auth.json` (0600).
- Env overrides: `BITBUCKET_WORKSPACE`, `BITBUCKET_REPOSITORIES`, `BITBUCKET_EMAIL`, `BITBUCKET_TOKEN`,
  `BITBUCKET_API_URL`, and `LAZYBITBUCKET_READ_ONLY` (`1|true|yes`, one-way: it can only turn read-only
  mode on, as can `--read-only`).
- Packaging: GitHub `lsst25/lazybitbucket`, brew tap `lsst25/homebrew-tap`, AUR `lazybitbucket-bin` and
  `lazybitbucket-git`, nix attr `.#lazybitbucket`, nfpm package `lazybitbucket`.
- `LICENSE` keeps the upstream copyright notice, and the `CHANGELOG.md` compare links **stay** pointed at
  upstream — that is accurate history. Everything below the `[Unreleased]` section is upstream's.

Auth: Atlassian removed app passwords on 2026-07-28. There is no app-password mode. Two credential
shapes: a token alone (API token or access token) sent as `Authorization: Bearer`, or an Atlassian
account email plus an API token sent as HTTP Basic — the Basic username must be the **account email**,
not a Bitbucket username, or the server answers 401. The five read scopes cover browsing; the six PR write
actions additionally want `write:pullrequest:bitbucket` (merging and approving included) and the two
pipeline ones `write:pipeline:bitbucket`, both of which read-only mode makes unnecessary.

## Conventions

- Match the surrounding lazyjira style. It is a well-linted codebase; run `make lint` before assuming a
  change is done.
- `.golangci.yml` is strict but **does not** enable `exhaustruct`, `wrapcheck` or `funlen` — do not write
  code as if it did. What is on and bites in practice:
  - `unused` — an unused unexported function fails the build. Delete dead code rather than keeping it
    "for later".
  - `exhaustive` with `default-signifies-exhaustive: true` — every switch over an API enum needs a
    `default:` branch. All wire enums are open `string` types.
  - `errorlint` — `errors.As`/`errors.Is`, `%w` when wrapping.
  - `noctx`, `bodyclose`, `nilerr`, `gosec`, `gocritic`, `revive`, `misspell`, `nakedret`.
  - `goconst` with `min-occurrences: 4` — a string repeated four times needs a constant. In test files
    that means a `testconsts_test.go`.
  - `gocognit` with `min-complexity: 50`.
  - `prealloc`, `unconvert`, `perfsprint`, `intrange`, `modernize`, `copyloopvar`, `fatcontext`.
  - Tests: `thelper`, `tparallel`, `paralleltest`, `usetesting`, `testableexamples`.
  - Existing exclusions live at the bottom of `.golangci.yml`; add one only with a comment saying why.
- Tests sit next to their subject as `*_test.go` and are plentiful. Internal test package, `t.Parallel()`
  unless the test calls `t.Setenv`, `t.Context()` over `context.Background()`, `testkit.AssertEqual`,
  inline fixtures, no golden files, no `//nolint` in tests.
- `pkg/tui` test helpers live in `pkg/tui/testsupport_test.go` — `newTestApp`, `newAppWithFake`,
  `sizedApp`, `runAll`, `collectMsgs`, `newPR`/`withAuthor`/`withReviewer`/`newStatus`, `plain`,
  `runeKey`. Build an app or a fixture from there instead of hand-rolling one; shared string literals
  for tests sit in the package's `testconsts_test.go`.
- Keep the Bitbucket client free of TUI imports; the domain layer must stay independently testable.
