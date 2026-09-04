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

ROADMAP steps 0 and 1 are **implemented**. The domain layer is Bitbucket, `pkg/jira` is gone, and the
binary, config dir and packaging are renamed.

What exists:

- `pkg/bitbucket` — read-only Bitbucket Cloud REST client (auth, pagination, `APIError` + predicates,
  concurrent multi-repo PR fetch), `pkg/bitbucket/bitbuckettest` (strict fake client) and
  `pkg/bitbucket/demo` (deterministic dataset + fake REST server, an ordinary untagged package used by
  `--demo` and by integration tests).
- `pkg/config` — `bitbucket` (workspace, repositories, email, baseUrl), `pullRequestTabs`,
  `gui.pullRequestListFields`, `refreshInterval`, custom command contexts
  `pullrequests`/`builds`/`repos`/`detail`.
- `pkg/tui` — panels Status `[1]`, Pull Requests `[2]`, Builds `[3]`, Repos `[4]`, Detail `[0]`. PR list
  with Open/Mine/Reviewing tabs and configurable columns, detail pane with Markdown descriptions via
  glamour, Builds panel with drill-down into a pipeline run, its steps and each step's log, repo filter,
  `/` search, URL picker, copy URL, browser hand-off, source-branch checkout, custom commands, themes,
  and a `refreshInterval` auto-refresh of the active tab.
- `cmd/lazybitbucket` — CLI (`auth`, `logout`, `version`; `--demo --log --debug --workspace --repo`),
  credential wizard.

Hard constraints that still hold:

- **The client has no write methods.** Nothing in the tool issues a non-GET request to Bitbucket.
  Merging, approving, commenting and editing are later roadmap steps.
- No test touches the network. Unit tests use `bitbuckettest.FakeClient` or `httptest`; integration
  tests use `pkg/bitbucket/demo`.
- `pkg/bitbucket` and `pkg/bitbucket/demo` import nothing from `pkg/tui` or `pkg/config`.

Not built yet (ROADMAP step 2+): diff pane, review threads, commits list, editor integration on files,
and every write operation. The detail pane therefore has a single tab (Overview), and `config.Editor` /
`ResolveEditor` are validated but unused — nothing opens an editor yet. ROADMAP step 2 still lists
custom commands as unchecked; they are in fact implemented.

## Port plan (done)

| Package | Plan | Outcome |
| --- | --- | --- |
| `pkg/tui` | **Reuse** — the main asset | Kept. Panes renamed and re-shaped for PRs; app/message/layout structure unchanged. JQL, create/edit/transition, mentions, hierarchy, ADF and the navstack were deleted |
| `pkg/jira` | **Replace** with `pkg/bitbucket` | Deleted. `pkg/bitbucket` + `bitbuckettest` + `demo` replace it |
| `pkg/git` | **Reuse** mostly as-is | Kept; branch checkout carries over |
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
  `BITBUCKET_API_URL`.
- Packaging: GitHub `lsst25/lazybitbucket`, brew tap `lsst25/homebrew-tap`, AUR `lazybitbucket-bin` and
  `lazybitbucket-git`, nix attr `.#lazybitbucket`, nfpm package `lazybitbucket`.
- `LICENSE` keeps the upstream copyright notice, and the `CHANGELOG.md` compare links **stay** pointed at
  upstream — that is accurate history. Everything below the `[Unreleased]` section is upstream's.

Auth: Atlassian removed app passwords on 2026-07-28. There is no app-password mode. Two credential
shapes: a token alone (API token or access token) sent as `Authorization: Bearer`, or an Atlassian
account email plus an API token sent as HTTP Basic — the Basic username must be the **account email**,
not a Bitbucket username, or the server answers 401.

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
