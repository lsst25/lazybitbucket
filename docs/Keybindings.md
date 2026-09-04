# Keybindings

Press `?` inside lazybitbucket to see the keys that apply to the panel you are in. Use `/` inside the help popup to filter them.

Keys are remappable in `config.yml` under the `keybinding` section — see [Configuration](Config.md#keybindings). Setting a key **replaces** every default for that action: `navigation.down: n` means `j`, `down` and `ctrl+j` stop navigating down.

There is no `keybinding.repos` group. The Repos and Builds panels reuse the pull request `select`, `open` and `focusRight` actions, so remapping those remaps them everywhere.

## Panels

| Key | Action | Config key |
|-----|--------|------------|
| `0` | Focus the detail panel | `universal.focusDetail` |
| `1` | Focus the status panel | `universal.focusStatus` |
| `2` | Focus the pull requests panel | `universal.focusPullRequests` |
| `3` | Focus the builds panel | `universal.focusBuilds` |
| `4` | Focus the repos panel | `universal.focusRepos` |
| `tab` | Switch between the left panels and the detail panel | `universal.switchPanel` |
| `l` / `right` | Next panel on the left; from the left side, cycles Status → Pull Requests → Builds → Repos → Status | `pullRequests.focusRight` |
| `h` / `left` / `esc` | Previous panel on the left; from the detail panel, back to the left side | `detail.focusLeft` |

## Universal

These work everywhere except inside the help popup and modals.

| Key | Action | Config key |
|-----|--------|------------|
| `q` / `ctrl+c` | Quit | `universal.quit` |
| `?` | Show the keybindings that apply here | `universal.help` |
| `/` | Search / filter the focused list | `universal.search` |
| `r` | Refresh the selected pull request (detail and build statuses) | `universal.refresh` |
| `R` | Refresh the pull request list of the active tab | `universal.refreshAll` |
| `[` / `]` | Previous / next tab | `universal.prevTab`, `universal.nextTab` |

## Navigation

| Key | Action | Config key |
|-----|--------|------------|
| `j` / `down` / `ctrl+j` | Move down | `navigation.down` |
| `k` / `up` / `ctrl+k` | Move up | `navigation.up` |
| `g` / `home` | Jump to the top | `navigation.top` |
| `G` / `end` | Jump to the bottom | `navigation.bottom` |
| `ctrl+d` | Half page down | `navigation.halfPageDown` |
| `ctrl+u` | Half page up | `navigation.halfPageUp` |

From any left panel except Status, these scroll the detail panel without moving focus:

| Key | Action | Config key |
|-----|--------|------------|
| `J` | Scroll the detail panel down | `detail.scrollDown` |
| `K` | Scroll the detail panel up | `detail.scrollUp` |
| `ctrl+f` | Half page the detail panel down | `detail.halfPageDown` |
| `ctrl+b` | Half page the detail panel up | `detail.halfPageUp` |

## Pull requests `[2]`

| Key | Action | Config key |
|-----|--------|------------|
| `enter` / `space` | Open the pull request in the detail panel | `pullRequests.open`, `pullRequests.select` |
| `l` / `right` | Next panel (Builds) | `pullRequests.focusRight` |
| `o` | Open the pull request in a browser | `pullRequests.browser` |
| `u` | URL picker — every URL reachable from the pull request | `pullRequests.urlPicker` |
| `y` | Copy the pull request URL to the clipboard | `pullRequests.copyURL` |
| `c` | Check out the source branch | `pullRequests.checkout` |
| `[` / `]` | Previous / next tab (Open / Mine / Reviewing by default) | `universal.prevTab`, `universal.nextTab` |

## Builds `[3]`

The Builds panel lists the commit statuses of the selected pull request.

| Key | Action | Config key |
|-----|--------|------------|
| `enter` / `space` | Drill into the pipeline run of the selected build | `pullRequests.open`, `pullRequests.select` |
| `o` | Open the build result in a browser | `pullRequests.browser` |
| `l` / `right` | Next panel (Repos) | `pullRequests.focusRight` |
| `h` / `left` / `esc` | Previous panel (Pull Requests) | `detail.focusLeft` |

A status whose URL carries no pipeline build number opens in the browser instead.

## Repos `[4]`

| Key | Action | Config key |
|-----|--------|------------|
| `enter` / `space` | Filter the pull request list to this repository | `pullRequests.open`, `pullRequests.select` |
| `o` | Open the repository in a browser | `pullRequests.browser` |
| `l` / `right` | Next panel (Status) | `pullRequests.focusRight` |
| `h` / `left` / `esc` | Previous panel (Builds) | `detail.focusLeft` |

The first row is `All repositories`, which clears the filter. Selecting a repository returns focus to the pull request list.

## Detail panel `[0]`

Showing a pull request or a repository:

| Key | Action | Config key |
|-----|--------|------------|
| `j` / `k` | Scroll | `navigation.down`, `navigation.up` |
| `ctrl+d` / `ctrl+u` | Half page down / up | `navigation.halfPageDown`, `navigation.halfPageUp` |
| `o` | Open in a browser | `pullRequests.browser` |
| `u` | URL picker | `pullRequests.urlPicker` |
| `y` | Copy the pull request URL | `pullRequests.copyURL` |
| `h` / `left` / `esc` | Back to the left panels | `detail.focusLeft` |
| `[` / `]` | Previous / next detail tab | `universal.prevTab`, `universal.nextTab` |

The pull request view has a single tab today (Overview), so `[` / `]` there is a no-op until the Diff, Commits and Comments tabs land.

Showing a pipeline run or a step log:

| Key | Action | Config key |
|-----|--------|------------|
| `j` / `k` | Move between pipeline steps, or scroll the log | `navigation.down`, `navigation.up` |
| `enter` / `space` | Open the log of the selected step | `pullRequests.open`, `pullRequests.select` |
| `backspace` | Back one level: step log → pipeline → pull request | `detail.back` |
| `h` / `left` / `esc` | Back one level, then to the left panels | `detail.focusLeft` |
| `o` | Open the pipeline run in a browser | `pullRequests.browser` |

## Help popup

| Key | Action |
|-----|--------|
| `/` | Filter the list |
| `j` / `k` (or `down` / `up`) | Move the cursor |
| `g` / `G` | Jump to top / bottom |
| `ctrl+d` / `ctrl+u` | Half page down / up |
| `enter` | Confirm the filter and keep the cursor on the match |
| `esc` | Cancel the filter, or close the popup |
| `q` / `?` | Close the popup |

## Search bar

| Key | Action |
|-----|--------|
| `/` | Open it on the focused list |
| `enter` | Drop the filter and keep the cursor on what the search landed on (on the Repos panel this also applies that repository filter) |
| `esc` | Clear every list filter |
| `up` / `down` / `ctrl+j` / `ctrl+k` | Move the list cursor while typing |

## URL picker and modals

| Key | Action |
|-----|--------|
| `/` | Search the entries |
| `enter` | Open the selected URL |
| `esc` | Cancel |

## Custom commands

Custom commands are matched **before** built-in keys, so a custom binding can shadow any action. Their keys are listed in the help popup and the hint bar of the panel they apply to. See [Custom commands](Config.md#custom-commands).
