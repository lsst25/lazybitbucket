<p align="center">
  <a href="https://go.dev/"><img src="https://img.shields.io/github/go-mod/go-version/lsst25/lazybitbucket" alt="Go"></a>
  <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License: MIT"></a>
</p>

# lazybitbucket

Terminal UI for **Bitbucket Cloud pull requests**. Like [lazygit](https://github.com/jesseduffield/lazygit), but for PRs.

Reviewing in the Bitbucket web UI means a lot of waiting and clicking: one tab per repository, a page load to see whether a build went green, another to find out who has approved. lazybitbucket puts the open pull requests of every repository you care about into one keyboard-driven list, with the description, the reviewers and the pipeline result a keystroke away.

> Screenshots and demo GIFs are pending — the recorded tapes now drive the Bitbucket UI, but the GIFs still have to be regenerated with [vhs](https://github.com/charmbracelet/vhs) (`make e2e-update`).

### Demo mode

Try it without a Bitbucket account. Demo mode serves a built-in dataset from a local fake API, so nothing leaves your machine (build from source required):

```
make build-demo
./lazybitbucket --demo
```

## Features

- **One list, many repositories** — open pull requests across all configured repos, with repo, id, title, author, approval state, build result and last-update time as columns
- **Tabs** — Open / Mine / Reviewing out of the box, configurable per state and filter
- **Detail pane** — four tabs per PR: Overview (branches, author, reviewers with approval state, Markdown description), Files, Commits and Comments. Focusing the pane fetches all three review tabs at once (`gui.prefetchDetailTabs: false` turns that off and leaves each tab to fetch itself the first time you open it), while a PR you only skim from the list still costs one request. `@`-mentions read as the display name Bitbucket resolved them to, in the theme's blue, not as the account id the Markdown stores
- **Diff pane** — the changed files with status and `+/−` counts, and a syntax-highlighted per-file unified diff with a line cursor; `n` / `p` walk the files, `enter` opens one
- **Open in your editor** — `e` opens the file under the cursor at that line in `$EDITOR` (`nvim` by default), in your local checkout; inside tmux it gets a new tmux window and the session stays put
- **Bring your own diff colours** — `diffRenderer: delta --color-only --paging=never` pipes each file's diff through delta (or anything that colours a diff line by line) instead of the built-in highlighting; the gutters, review markers and cursor stay lazybitbucket's
- **Commits** — hash, author, age and subject per commit; `enter` expands the full message
- **Review threads** — inline comments read as threads anchored to a file and line, with replies and resolved state, `@Grace Hopper`, in blue, where the comment stores an account id; `enter` opens one conversation in the panel, whole and scrolling, and commented lines are marked `●` in the diff where `enter` jumps to the thread
- **Builds** — the commit statuses of the selected PR, with drill-down into the pipeline run, its steps and each step's log
- **Pipelines view** — `⇧tab` swaps the pull request list for the recent pipeline runs of every configured repository, newest first: branch builds, scheduled and manual runs, not just the ones a PR owns. Same drill-down into steps and logs, same `P` / `X`, and `L` jumps back to the PR a run ran for
- **Repo filter** — narrow the list to a single repository from the Repos panel
- **Search** — `/` filters the focused list as you type
- **Browser hand-off** — `o` opens what you are looking at (the PR, a commit, a thread, the diff), `y` copies that URL, `u` opens a picker of every URL the PR reaches (builds, links in the description, plus commits and threads once those tabs have been opened)
- **Open a pull request** — `N` creates one for the branch you are on: the title prefilled from the last commit, the target from the repository's default branch, reviewers from its default reviewers or a checklist, a draft flag, and the description in `$EDITOR` if you want one. A branch that is not on the remote yet is offered a push first, because Bitbucket cannot open a PR for a branch it does not have
- **Merge** — `M` opens a dialog: one of the six strategies Bitbucket accepts, close-source-branch, an optional commit message. The merge runs asynchronously on Bitbucket and is polled until it lands
- **Sync with the target branch** — `S` merges or rebases the source branch onto its target and pushes, through git in your own checkout, because Bitbucket's "Sync now" has no API. Conflicts abort and are left to you
- **Edit** — `E` changes the title inline, or opens the description in `$EDITOR` and sends back what you saved
- **Reviewers and draft** — `V` is a checklist over the current reviewers, the repository's default reviewers and the workspace members; `D` flips the PR between draft and ready for review
- **Approve** — `A` approves the PR, and `A` again withdraws the approval you gave; the dialog always names the direction, and your own PRs are refused the way Bitbucket refuses them
- **Comment and reply** — `C` opens the same input popup the title editor uses, headed with what you are answering: a reply to the thread you are in or the one under the cursor, a new comment on the PR from the Overview tab. `ctrl+e` continues in `$EDITOR` when a reply wants paragraphs, and what you send appears in the thread straight away, rendered by Bitbucket. A merged PR still takes comments
- **Resolve conversations** — `T` marks the thread in hand resolved, and `T` again reopens it; the dialog always names which. Resolved threads drop off the Comments tab (the label says how many are out of sight) and `f` brings them back, marked with a green `✔` and who ended them
- **Mark files viewed** — `v` ticks the file in hand and goes to the next one you have not viewed, in the Files tab or in a diff; `v` again clears the tick. A push that changes a file clears its tick, like the web UI does. The ticks stay on your computer, because Bitbucket's API does not expose its own
- **Inline comments** — `C` on a line of a diff anchors the comment there, on the side that line lives on; the `●` marker lights up and you stay on the line, mid-review. On a hunk header or a row of the Files tab it comments on the file instead
- **Rerun and stop pipelines** — `P` starts a finished pipeline run again with the same commit and selector, `X` signals a running one to stop; both work on the Builds panel and inside the pipeline and step-log views, behind a confirmation that names the run
- **Read-only mode** — `--read-only` (or `bitbucket.readOnly: true`) makes the whole thing browse-only: the client refuses every non-GET request before it is sent, and the action keys disappear. Nothing else is ever written either — lazybitbucket cannot request changes, decline, or edit or delete a comment
- **Git integration** — check out the source branch of the selected PR; started inside a checkout, lazybitbucket scopes the list to that repository and puts the cursor on the PR for the branch you are on, whichever tab it is in
- **Custom commands** — bind shell commands to keys with template access to the focused PR, repo, build, or the changed file and line under the cursor
- **Themes** — default ANSI palette, all four Catppuccin flavors (Latte, Frappé, Macchiato, Mocha) and the three Kanagawa variants (Wave, Dragon, Lotus), with light/dark selection that follows your terminal
- **Adaptive** — side-by-side or stacked layout, mouse support, and lazygit's screen modes: `+` / `_` cycle normal → half → full, where the focused panel takes the whole window

## Installation

<details>
<summary><b>Go / From source</b></summary>

```
go install github.com/lsst25/lazybitbucket/cmd/lazybitbucket@latest
```

Or build manually:

```
git clone https://github.com/lsst25/lazybitbucket.git
cd lazybitbucket
make build
```

</details>

<details>
<summary><b>Packaged builds</b></summary>

There are no prebuilt releases, Homebrew formula or AUR packages. Build from source, or use Nix.

#### Nix / NixOS

```
nix run github:lsst25/lazybitbucket
```

Or add to your flake inputs:

```nix
inputs.lazybitbucket.url = "github:lsst25/lazybitbucket";
```

</details>

## Setup

Run `lazybitbucket`. On first launch a setup wizard asks for your credentials and workspace, tests the connection, and saves the result.

### Get a token

Atlassian **app passwords no longer exist** — they were removed on 28 July 2026. Use an **API token with scopes**:

1. Profile (upper right) → **Account settings**
2. **Security** tab → **Create and manage API tokens**
3. **Create API token with scopes**
4. Name and expiry (max one year) → **Next**
5. Pick **Bitbucket** as the app → **Next**
6. Tick the scopes below → **Next** → **Create**

The token is shown once and cannot be retrieved later.

A token without scopes does not work. These five read scopes cover everything lazybitbucket browses:

| Scope | Needed for |
|-------|------------|
| `read:pullrequest:bitbucket` | the pull request list and detail |
| `read:repository:bitbucket` | repository metadata, commit statuses, and the diff, diffstat and commits of a pull request |
| `read:pipeline:bitbucket` | pipeline runs, steps and step logs |
| `read:user:bitbucket` | who you are — the Mine / Reviewing tabs |
| `read:workspace:bitbucket` | the workspace itself (used by the connection test) |

API token scopes are a different namespace from OAuth 2.0 scopes and do **not** imply each other, so tick all five.

The actions that change something need a write scope, one each:

| Scope | Needed for |
|-------|------------|
| `write:pullrequest:bitbucket` | the seven [pull request actions](docs/Keybindings.md#pull-request-actions): editing the title or description, setting reviewers, toggling draft, approving, merging, and commenting |
| `write:pipeline:bitbucket` | [rerunning and stopping pipeline runs](docs/Keybindings.md#rerun-and-stop-a-pipeline) from the Builds panel or the [pipelines view](docs/Keybindings.md#pipelines-view) (`P` / `X`) |

The two are independent: the pull request actions do not need the pipeline scope, and `P` / `X` do not need the pull request one. Leave them off and every read still works while a write comes back `403` — no harm done, just a message in the hint bar. If you would rather not have the possibility at all, use [read-only mode](docs/Config.md#read-only-mode), which needs neither and refuses writes before they are sent. Sync needs no scope of its own: it is local git, so what it needs is push permission on the remote. On a repository, project or workspace access token the same permission is spelled `pullrequest:write`.

### Auth options

The wizard offers two, and both are API tokens under the hood:

1. **Token only, sent as `Authorization: Bearer <token>`** (the default). Works for account API tokens and for repository / project / workspace **access tokens**. Nothing else to type.
2. **Atlassian account email + API token, sent as HTTP Basic.** The username must be your **Atlassian account email** — a Bitbucket username or the token's name yields a 401. Your email is listed under *Email Aliases* on the Bitbucket Personal settings page.

A repository, project or workspace access token cannot call `/user`, so the wizard prints `Connected (token cannot read the current user; Mine/Reviewing tabs are disabled)` and those tabs stay empty. That is expected, not a failure.

Credentials are written to `~/.config/lazybitbucket/auth.json` with `0600` permissions. `lazybitbucket logout` deletes the file.

### Workspace and repositories

The workspace comes from the wizard, `bitbucket.workspace` in `config.yml`, or `--workspace`. Repositories are listed in `config.yml`:

```yaml
bitbucket:
  workspace: acme
  repositories:
    - api-gateway
    - web-app
    - infra
```

With no `repositories` set, lazybitbucket discovers the repositories you are a member of — the first 25, most recently updated first — and warns in the status bar when it hits that cap. An access token sees none that way, so it has to list them explicitly.

### Environment variables

These override both `config.yml` and `auth.json`, which makes them handy for a per-project shell or CI-ish scripting.

| Variable | Effect |
|----------|--------|
| `BITBUCKET_WORKSPACE` | Workspace slug |
| `BITBUCKET_REPOSITORIES` | Comma-separated repository slugs |
| `BITBUCKET_EMAIL` | Atlassian account email (switches to Basic auth when a token is also set) |
| `BITBUCKET_TOKEN` | API token or access token |
| `BITBUCKET_API_URL` | API base URL (defaults to `https://api.bitbucket.org/2.0`) |
| `LAZYBITBUCKET_CONFIG_DIR` | Override the whole config directory |
| `LAZYBITBUCKET_READ_ONLY` | `1`, `true` or `yes` turns on read-only mode; it cannot turn it off |

## Themes

lazybitbucket supports the default ANSI 16 palette, Catppuccin and Kanagawa presets, light/dark selection that follows your terminal background, and custom palette overrides. Set `gui.theme` in `~/.config/lazybitbucket/config.yml` to a preset, or to a family name such as `kanagawa` that picks Wave or Lotus from the background:

```yaml
gui:
  theme: kanagawa   # or a preset: catppuccin-mocha, kanagawa-lotus, ...
```

See the [Themes docs](docs/Config.md#themes) for supported values, palette overrides, and examples.

Diff code is syntax-highlighted with chroma, in the variant that matches the theme. To hand that column to an external program instead — `delta --color-only`, say — set `diffRenderer`; see the [Diff renderer docs](docs/Config.md#diff-renderer-diffrenderer) for the one-line-per-line contract it has to keep.

## Usage

```
lazybitbucket                    # start
lazybitbucket auth               # re-run the credential wizard
lazybitbucket logout             # clear credentials
lazybitbucket version            # show version
lazybitbucket --demo             # built-in demo dataset (requires a -tags demo build)
lazybitbucket --workspace acme   # override the configured workspace
lazybitbucket --repo web-app     # override the configured repositories (repeatable)
lazybitbucket --read-only        # browse only: refuse every request that would write
lazybitbucket --log api.log      # log API requests to a file
lazybitbucket --debug debug.log  # log internal debug output to a file
```

Press `?` inside the app for all keybindings.

## Keys

| Key | Action |
|-----|--------|
| `0` `1` `2` `3` `4` | Focus Detail, Status, Pull Requests, Builds, Repos |
| `⇧tab` | Switch between the pull requests view and the pipelines view |
| `j` / `k` | Move down / up |
| `space` / `enter` | Open the highlighted row |
| `tab` | Switch between the left panels and the detail panel |
| `h` / `l` | Previous / next panel |
| `[` / `]` | Previous / next tab |
| `J` / `K` | Scroll the detail pane without leaving the list |
| `/` | Search the focused list |
| `o` | Open in browser |
| `u` | URL picker |
| `y` | Copy the pull request URL |
| `c` | Check out the source branch |
| `M` | Merge the pull request |
| `S` | Sync the source branch with its target |
| `E` | Edit the title or the description |
| `V` | Set the reviewers |
| `D` | Toggle draft |
| `A` | Approve, or remove your approval |
| `P` | Rerun the pipeline run of the selected build |
| `X` | Stop the running pipeline run of the selected build |
| `L` | From a run in the pipelines view, jump to the pull request it ran for |
| `i` `d` `m` `t` | Jump to the Overview / Files / Commits / Comments tab of the PR |
| `n` / `p` | Next / previous changed file |
| `e` | Open the file under the cursor in your editor |
| `backspace` | Back out of the file diff / step log / pipeline view |
| `r` / `R` | Refresh the selection / the whole tab |
| `?` | Help |
| `q` | Quit |

Full list, including everything remappable: [Keybindings](docs/Keybindings.md).

## Documentation

- [Configuration](docs/Config.md) — config file, tabs, list columns, keybindings, editor, diff renderer, themes, git, custom commands
- [Keybindings](docs/Keybindings.md) — full list of default keys
- [Roadmap](ROADMAP.md) — what is built and what is next

## Credits

lazybitbucket is a fork of [textfuel/lazyjira](https://github.com/textfuel/lazyjira) (MIT). The Bubbletea shell — panels, layout, theming, keybinding config, custom commands — comes from there; the domain layer is Bitbucket-specific.

## License

MIT
