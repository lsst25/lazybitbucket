# Contributing to lazybitbucket

Thanks for your interest in contributing!

## Getting started

```bash
git clone https://github.com/lsst25/lazybitbucket.git
cd lazybitbucket
make build
```

## Development workflow

1. Fork the repo and create a feature branch
2. Make your changes
3. Run `make check` (lint + vet + build + test) - this must pass
4. Run `make check-demo` if you touched anything the `demo` build tag reaches
5. Open a pull request against `main`

## Running locally

```bash
make build
./lazybitbucket
```

To test without a Bitbucket account:

```bash
make build-demo
./lazybitbucket --demo
```

`--demo` needs a build with the `demo` tag: only `cmd/lazybitbucket/demo.go` sits behind
`//go:build demo`, and `demo_stub.go` replaces it in a normal build with an error telling you to
rebuild. The dataset and the fake REST server themselves live in `pkg/bitbucket/demo`, which is an
ordinary untagged package — it is compiled, vetted and tested by `go test ./...`, and integration
tests use it. Keep it that way: nothing in `pkg/bitbucket/demo` may depend on a build tag, and
nothing in it may import `pkg/tui` or `pkg/config`.

`make check-demo` lints, vets and builds with `-tags demo` so the tagged file cannot rot.

## Layout

| Path | What |
|------|------|
| `cmd/lazybitbucket/` | CLI entry point, flag parsing, credential wizard |
| `pkg/bitbucket/` | Bitbucket Cloud REST client, models, errors. Writes (merge, update) go through one helper that `ClientOpts.ReadOnly` switches off |
| `pkg/bitbucket/bitbuckettest/` | Strict fake client for unit tests |
| `pkg/bitbucket/demo/` | Deterministic demo dataset + fake REST server |
| `pkg/config/` | Config file, credentials, custom command definitions |
| `pkg/tui/` | Bubbletea app: panels, keymap, handlers |
| `pkg/tui/views/`, `pkg/tui/components/`, `pkg/tui/theme/` | Panes, reusable widgets, palette |
| `pkg/git/` | Branch operations |
| `e2e/` | vhs tape sources and the preprocessor that expands them |

Two rules worth stating up front: the Bitbucket client never imports the TUI, and no test touches
the network. Unit tests use `bitbuckettest.FakeClient` or `httptest`; integration tests use
`pkg/bitbucket/demo`.

## Nix

If you have Nix with flakes enabled, you can get a complete dev environment
without installing Go or other tools globally:

```bash
nix develop
make check
```

### Adding a Go dependency

After you add a new package to `go.mod`, refresh the Nix lockfile

```bash
make nix-deps
```

You need `gomod2nix` for this. Get it one of two ways

- With Nix `nix develop -c make nix-deps`
- With Go `go install github.com/nix-community/gomod2nix@latest`

Commit `gomod2nix.toml` together with `go.mod` and `go.sum`. If you skip it the `nix` CI job fails with a checksum error

## Code style

- Go standard formatting (`gofmt`)
- Linting via `golangci-lint` (run with `make lint`)
- Keep functions focused and small
- Follow existing patterns in the codebase
- Tests sit next to their subject as `*_test.go`, use the internal test package, call `t.Parallel()`
  unless they set env vars, and keep fixtures inline
- `pkg/tui` tests build their app and fixtures from the helpers in `pkg/tui/testsupport_test.go`
  (`newTestApp`, `newAppWithFake`, `sizedApp`, `runAll`, `collectMsgs`, `newPR`, `newStatus`) rather
  than hand-rolling one
- Docs are linted too: `make lint-docs` runs `markdownlint-cli` over `README.md`, `CHANGELOG.md` and
  `docs/*.md` (needs `npx`)

## e2e tapes

`e2e/tapes/*.tape.sh` are shorthand sources; `e2e/tape.sh` expands them into VHS tape syntax and
generates a horizontal and a vertical variant. `make e2e` runs them, `make e2e-update` refreshes the
GIFs in `e2e/golden/`. Both need [vhs](https://github.com/charmbracelet/vhs) installed.

The directive list is the comment block at the top of `e2e/tape.sh`; anything it does not recognise is
passed through to VHS unchanged, so raw `Type`/`Sleep` lines are fine. Keystrokes must match the
defaults in `pkg/tui/keymap.go` and the demo dataset in `pkg/bitbucket/demo/data.go` — the tapes drive
`./lazybitbucket --demo`, so `make e2e` needs a `make build-demo` binary (the Makefile targets do that
for you). Without vhs you can still check a tape expands with `./e2e/tape.sh generate-all`; delete the
generated `.tape` files afterwards, they are build output.

## Reporting bugs

Open an issue on GitHub with:
- Steps to reproduce
- Expected vs actual behavior
- Terminal and OS info

## Feature requests

Open an issue describing the use case. Check [ROADMAP.md](ROADMAP.md) first.
