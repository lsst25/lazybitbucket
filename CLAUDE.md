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

The fork, the repo rename, and the Go module rename are **done**. The domain port has **not started**.

As of now this is still functionally lazyjira with a new module path. `pkg/jira` is untouched Jira code,
the binary is still `lazyjira`, and the config still lives in `~/.config/lazyjira`. Do not assume any
Bitbucket support exists yet.

## Port plan

| Package | Files | Plan |
| --- | --- | --- |
| `pkg/tui` | 155 | **Reuse.** The main asset. Add PR-shaped panes; keep the app/message/navstack structure. |
| `pkg/jira` | 26 | **Replace** with `pkg/bitbucket`. New client, models, and rendering. |
| `pkg/git` | 10 | **Reuse** mostly as-is — branch ops and branch-name generation carry over. |
| `pkg/config` | 6 | **Adapt.** Auth moves from Jira token to Bitbucket app password / access token. |
| `pkg/ascii` | 2 | Reuse. |
| `cmd/lazyjira` | 4 | **Rename** to `cmd/lazybitbucket`. |

### Coupling is shallower than it looks (measured)

Of the 155 Go files in `pkg/tui`, only **20 non-test files** import `pkg/jira` (56 test files do).
And `pkg/jira` imports **nothing** from `pkg/tui` — the dependency runs one way.

So the domain swap is a bounded job against ~20 files plus their tests, not a rewrite of the TUI. Verify
this before relying on it:

```
grep -rl 'lazybitbucket/pkg/jira' pkg/tui --include='*.go' | grep -v _test | wc -l
```

### The domain shapes do not line up

This is the real work, and it is not a find-and-replace. A Jira issue is a flat bag of fields; a Bitbucket
PR is a composite of several resources fetched separately:

- **diff** — per-file hunks, with no Jira analogue at all. Needs a new pane and a syntax-highlighted
  viewer (`chroma` is already a dependency).
- **review threads** — comments anchored to a file *and line*, and nested. Jira comments are a flat list.
- **commits** — a list per PR.
- **build statuses / pipelines** — pass/fail state to surface per PR.
- **approvals** — reviewer state (approved / changes requested / none).

Expect to add panes rather than just re-label existing ones. Jira's ADF rendering
(`seflue/adf-converter`) is Atlassian-wiki-specific; Bitbucket PR descriptions and comments are Markdown,
so that dependency can likely go away in favour of the existing `glamour` renderer.

## Build and test

```
go build ./...        # or: make build
go test ./...         # or: make test
make check            # lint + vet + build + test
make lint-fix
make tidy
```

Baseline before the domain port: `go build ./...` clean, all 12 test packages passing. Keep it that way —
if a change makes the suite red, that is a regression, not the starting state.

Go 1.25+ (toolchain declared in `go.mod`). Nix flake and goreleaser config are inherited and work.

## Repo setup

- Module: `github.com/lsst25/lazybitbucket` (no `/v2` — versioning restarts for this project)
- `origin` → `git@github.com:lsst25/lazybitbucket.git`
- `upstream` → `git@github.com:textfuel/lazyjira.git`, so `git fetch upstream && git merge upstream/main`
  can still pull their fixes. Expect conflicts in `pkg/jira` once that package is replaced.
- **No tags.** The 49 inherited upstream tags were deleted so releases can start clean.
- `fetch.pruneTags = false` is set repo-locally on purpose. The global `~/.gitconfig` sets it to `true`,
  which forces an explicit `refs/tags/*` refspec onto every fetch and would re-sync all 49 upstream tags
  back on the next `git fetch upstream`. `--no-tags` cannot suppress that. Do not "clean up" this override.

## Renames still pending

Deliberately not done yet — they touch packaging and user-visible behaviour:

- `cmd/lazyjira/` → `cmd/lazybitbucket/`, and the binary name. Touches `Makefile`, `.goreleaser.yml`,
  `.golangci.yml:74`, `flake.nix`, `nix/`, `aur/lazyjira-git/PKGBUILD`, `e2e/tape.sh`, docs.
- Config dir, in `pkg/config/config.go:341-357` — `LAZYJIRA_CONFIG_DIR` env var and the
  `~/.config/lazyjira` path. This is user-visible behaviour, not just a string swap.
- Upstream URLs in `README.md`, `CONTRIBUTING.md`, `.goreleaser.yml`, `nix/package.nix`, the AUR PKGBUILD.
  Note the `CHANGELOG.md` compare links **should** stay pointed at upstream — that is accurate history.

## Conventions

- Match the surrounding lazyjira style. It is a well-linted codebase (`.golangci.yml` is strict —
  `exhaustruct`, `wrapcheck`, `funlen` and friends are on). Run `make lint` before assuming a change is done.
- Tests sit next to their subject as `*_test.go` and are plentiful. New domain code gets tests; the fake
  server pattern in `pkg/jira/demoserver.go` and `pkg/jira/jiratest/fake.go` is the model to copy for
  `pkg/bitbucket`.
- Keep the Bitbucket client free of TUI imports; the domain layer must stay independently testable.
