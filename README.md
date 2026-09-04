<p align="center">
  <a href="https://go.dev/"><img src="https://img.shields.io/github/go-mod/go-version/lsst25/lazybitbucket" alt="Go"></a>
  <a href="https://github.com/lsst25/lazybitbucket/releases"><img src="https://img.shields.io/github/v/release/lsst25/lazybitbucket" alt="Release"></a>
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
- **Detail pane** — branches, author, reviewers with approval state, and the description rendered from Markdown
- **Builds** — the commit statuses of the selected PR, with drill-down into the pipeline run, its steps and each step's log
- **Repo filter** — narrow the list to a single repository from the Repos panel
- **Search** — `/` filters the focused list as you type
- **Browser hand-off** — `o` opens what you are looking at, `y` copies the PR URL, `u` opens a picker of every URL the PR reaches (branches, commits, builds, links in the description)
- **Read-only** — the client has no write methods at all. lazybitbucket never merges, approves, comments or edits
- **Git integration** — check out the source branch of the selected PR; the PR matching your current branch is selected at startup
- **Custom commands** — bind shell commands to keys with template access to the focused PR, repo or build
- **Themes** — default ANSI palette plus all four Catppuccin flavors (Latte, Frappé, Macchiato, Mocha)
- **Adaptive** — side-by-side or stacked layout, mouse support

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

The release pipeline publishes these channels. They only exist once a release is cut, so build from source until then.

#### Homebrew (macOS, Linux)

```
brew install lsst25/tap/lazybitbucket
```

#### Arch Linux (AUR)

```
yay -S lazybitbucket-bin     # prebuilt binary
yay -S lazybitbucket-git     # build from source
```

#### Nix / NixOS

```
nix run github:lsst25/lazybitbucket
```

Or add to your flake inputs:

```nix
inputs.lazybitbucket.url = "github:lsst25/lazybitbucket";
```

#### deb / rpm / apk

Download the package for your distro from [Releases](https://github.com/lsst25/lazybitbucket/releases):

```
sudo dpkg -i lazybitbucket_*.deb
sudo rpm -i lazybitbucket_*.rpm
sudo apk add --allow-untrusted lazybitbucket_*.apk
```

#### Windows

Download the `.zip` from [Releases](https://github.com/lsst25/lazybitbucket/releases), extract `lazybitbucket.exe`, and add it to your `PATH`. Use [Windows Terminal](https://aka.ms/terminal) for best rendering.

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

A token without scopes does not work. lazybitbucket only reads, so the five read scopes are enough:

| Scope | Needed for |
|-------|------------|
| `read:pullrequest:bitbucket` | the pull request list and detail |
| `read:repository:bitbucket` | repository metadata and commit statuses |
| `read:pipeline:bitbucket` | pipeline runs, steps and step logs |
| `read:user:bitbucket` | who you are — the Mine / Reviewing tabs |
| `read:workspace:bitbucket` | the workspace itself (used by the connection test) |

API token scopes are a different namespace from OAuth 2.0 scopes and do **not** imply each other, so tick all five.

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

## Themes

lazybitbucket supports the default ANSI 16 palette, Catppuccin presets, auto light/dark selection, and custom palette overrides. Set `gui.theme` in `~/.config/lazybitbucket/config.yml`:

```yaml
gui:
  theme: catppuccin-mocha
```

See the [Themes docs](docs/Config.md#themes) for supported values, palette overrides, and examples.

## Usage

```
lazybitbucket                    # start
lazybitbucket auth               # re-run the credential wizard
lazybitbucket logout             # clear credentials
lazybitbucket version            # show version
lazybitbucket --demo             # built-in demo dataset (requires a -tags demo build)
lazybitbucket --workspace acme   # override the configured workspace
lazybitbucket --repo web-app     # override the configured repositories (repeatable)
lazybitbucket --log api.log      # log API requests to a file
lazybitbucket --debug debug.log  # log internal debug output to a file
```

Press `?` inside the app for all keybindings.

## Keys

| Key | Action |
|-----|--------|
| `0` `1` `2` `3` `4` | Focus Detail, Status, Pull Requests, Builds, Repos |
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
| `backspace` | Back out of the step log / pipeline view |
| `r` / `R` | Refresh the selection / the whole tab |
| `?` | Help |
| `q` | Quit |

Full list, including everything remappable: [Keybindings](docs/Keybindings.md).

## Documentation

- [Configuration](docs/Config.md) — config file, tabs, list columns, keybindings, themes, git, custom commands
- [Keybindings](docs/Keybindings.md) — full list of default keys
- [Roadmap](ROADMAP.md) — what is built and what is next

## Credits

lazybitbucket is a fork of [textfuel/lazyjira](https://github.com/textfuel/lazyjira) (MIT). The Bubbletea shell — panels, layout, theming, keybinding config, custom commands — comes from there; the domain layer is Bitbucket-specific.

## License

MIT
