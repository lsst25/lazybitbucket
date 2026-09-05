# Keybindings

Press `?` inside lazybitbucket to see the keys that apply to the panel you are in. Use `/` inside the help popup to filter them.

Keys are remappable in `config.yml` under the `keybinding` section — see [Configuration](Config.md#keybindings). Setting a key **replaces** every default for that action: `navigation.down: n` means `j`, `down` and `ctrl+j` stop navigating down.

There is no `keybinding.repos` group. The Repos and Builds panels reuse the pull request `select`, `open` and `focusRight` actions, so remapping those remaps them everywhere. There is a `keybinding.builds` group, but it carries only the two pipeline actions — `rerun` and `stop` — which no other panel has.

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
| `+` / `_` | Next / previous screen mode: normal → half (side panels take half the width) → full (the focused panel fills the window) | `universal.nextScreenMode`, `universal.prevScreenMode` |

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
| `c` | Fetch and check out the source branch (fast-forwards a stale local branch) | `pullRequests.checkout` |
| `[` / `]` | Previous / next tab (Open / Mine / Reviewing by default) | `universal.prevTab`, `universal.nextTab` |
| `M` | Merge… | `pullRequests.merge` |
| `S` | Sync with the target branch… | `pullRequests.sync` |
| `E` | Edit the title or the description… | `pullRequests.edit` |
| `V` | Reviewers… | `pullRequests.reviewers` |
| `D` | Toggle draft… | `pullRequests.toggleDraft` |
| `A` | Approve, or remove your approval… | `pullRequests.approve` |

## Pull request actions

The six keys above are the only ones that change a pull request — `P` and `X` write too, but to a pipeline run rather than the pull request (see [Rerun and stop a pipeline](#rerun-and-stop-a-pipeline)). They work from the pull request list and from the detail panel alike — whatever has focus, they act on the pull request the list cursor is on — and each one opens a dialog first. Nothing is sent until you confirm; `esc` closes a dialog without a request.

A refusal is a one-line message in the hint bar and no dialog:

| Message | Reason |
|---------|--------|
| `read-only mode: cannot merge` | [Read-only mode](Config.md#read-only-mode) is on. It refuses all six, sync included |
| `cannot merge: pull request is MERGED` | The pull request is not `OPEN` any more |
| `merge already in progress` | The previous write on this pull request has not answered yet |
| `cannot approve your own pull request` | `A` on a pull request you opened. Bitbucket refuses it, so lazybitbucket does not ask |

While a write is in flight the hint bar says `merging #101…`; when it lands it says `merged #101` and the list and the detail panel are replaced with what the server returned. A rejected write shows Bitbucket's own message, field detail included (`title: This field is required.`), rather than a Go error.

### Dialog keys

The merge, sync, draft, approve, rerun and stop dialogs are the same form widget, and its keys are fixed — they are not remappable:

| Key | Action |
|-----|--------|
| `j` / `k` / `down` / `up` / `tab` | Move between rows |
| `h` / `l` / `left` / `right` | Cycle a choice (`‹ squash ›`) |
| `space` | Toggle a checkbox, or cycle a choice |
| `enter` | On a text row, start editing it; anywhere else, confirm the dialog |
| `esc` / `q` | Cancel |

The hint bar reads `j/k move  h/l change  space toggle  enter confirm  esc cancel` while a form is open. While a text row is being edited every key types into it — `q`, `j`, `h` and `space` included — and only `enter` (keep) and `esc` (revert) leave the editor; the dialog's own footer says `enter: keep   esc: revert` for as long as that lasts.

### `M` — merge

```
 Merge #101 into main
 ─────────────────────────────
 Strategy              ‹ merge commit ›
 Close source branch   [x]
 Message
 ─────────────────────────────
 enter: merge   esc: cancel
```

Strategy cycles through the six the API accepts, in this order: merge commit, squash, fast-forward, squash fast-forward, rebase fast-forward, rebase merge. A repository that disallows the one you pick answers 400 and the message says so. Close source branch starts on the pull request's own flag. An empty message lets Bitbucket compose the commit message.

lazybitbucket asks Bitbucket to merge asynchronously (a synchronous merge that outlives the server's gateway would fail with a timeout and leave you guessing) and polls the result: at once, then every two seconds. If the merge is still running after about a minute the hint bar says so and `r` picks the answer up later.

### `S` — sync with the target branch

```
 Sync feature/x with main
 ─────────────────────────────
 Method   ‹ merge ›
 Push     [x]
 ─────────────────────────────
 enter: sync   esc: cancel
```

This one is not an API call: Bitbucket's own "Sync now" button has no public endpoint, so lazybitbucket runs git in your checkout — fetch, then `merge` or `rebase`, then a push. It needs the checkout to be ready for it; see [Sync with the target branch](Config.md#sync-with-the-target-branch) for the preconditions and what it does with conflicts.

### `E` — edit title or description

`E` opens a two-row chooser.

- **Title** opens an inline input prefilled with the current title. `enter` saves, `esc` cancels; an empty or unchanged title is refused with a hint-bar message and no request.
- **Description** writes the description to a temporary Markdown file and hands the terminal to [your editor](Config.md#editor) — the same `editor` setting the diff pane's `e` uses, without a line number. Saving and quitting sends the new text; leaving it untouched sends nothing. The temporary file is deleted either way.

### `V` — reviewers

`V` opens a checklist rather than a form — the same widget as the URL picker, in its multi-select mode:

```
 Reviewers for #101
 ─────────────────────────────
 [x] Jane Doe            · reviewer
 [x] Ravi Patel          · default reviewer
 [ ] Chris Wong          · member
```

| Key | Action |
|-----|--------|
| `j` / `k` | Move the cursor |
| `space` | Check or uncheck the account under the cursor |
| `enter` | Save the checked set |
| `/` | Filter the list by name |
| `esc` / `h` / `q` | Cancel |

The hint bar reads `space toggle  enter save  / filter  esc cancel` while it is open.

The candidates are the current reviewers first, then the pull request's reviewing participants, then the repository's effective default reviewers, then the workspace members — deduplicated, with the author left out because Bitbucket refuses to make them a reviewer. They are fetched when the picker opens (`loading reviewers…` until they arrive). A token that may not read the default reviewers or the member list still gets a picker, with a note saying what is missing.

Saving sends the whole checked set, so unchecking someone removes them.

### `D` — toggle draft

A confirmation dialog — `Mark #101 as draft?` or `Mark #101 ready for review?` — with `enter` to confirm and `esc` / `q` to cancel.

### `A` — approve

One key, two directions, and the dialog says which one you are about to take:

```
 Approve #101?
 ─────────────────────────────
 Add a token bucket rate limiter
 ─────────────────────────────
 enter: confirm   esc: cancel
```

If your approval is already on the pull request the question is `Remove your approval from #101?` instead, and confirming withdraws it. Which of the two you get is decided by the participant list, so `A` never does the opposite of what it asked: lazybitbucket reads the pull request first (`checking your approval…`) and opens the dialog on that fresh copy, whatever the row or the cached detail said — a list payload carries no participants at all, and a cached detail keeps the participants it was fetched with while list refreshes move on without it. With credentials that have no account behind them there is nothing to look up, and the dialog opens at once.

`A` on a pull request you opened is refused outright with `cannot approve your own pull request`, without a request: Bitbucket answers that one 400. On credentials with no account behind them — a repository, project or workspace access token, which `GET /user` answers 403 for — there is no approval of your own to find, so `A` only ever approves.

The pull request is read back afterwards rather than patched from the answer: the approve endpoint replies with a single participant and the withdrawal with nothing at all, while the approvals column in the list and the reviewer line in the detail pane are drawn from the whole participant set. So after `approved #101` (or `approval removed #101`) both of them move. Should that read-back fail after the approval landed, the help bar still says `approved #101`, followed by `refresh failed: …` — the approval is on the server, and `r` reloads the pull request.

## Builds `[3]`

The Builds panel lists the commit statuses of the selected pull request.

| Key | Action | Config key |
|-----|--------|------------|
| `enter` / `space` | Drill into the pipeline run of the selected build | `pullRequests.open`, `pullRequests.select` |
| `o` | Open the build result in a browser | `pullRequests.browser` |
| `l` / `right` | Next panel (Repos) | `pullRequests.focusRight` |
| `h` / `left` / `esc` | Previous panel (Pull Requests) | `detail.focusLeft` |
| `P` | Rerun the pipeline run of the selected build — see [Rerun and stop a pipeline](#rerun-and-stop-a-pipeline) | `builds.rerun` |
| `X` | Stop the running pipeline of the selected build | `builds.stop` |

A status whose URL carries no pipeline build number opens in the browser instead.

### Rerun and stop a pipeline

`P` and `X` are the only Builds panel keys that change anything on Bitbucket, and what they change is the pipeline run behind the selected build rather than the pull request. They work in the detail panel too, while it shows a pipeline run or a step log, so the log of a step that failed and the key that runs it again are one keystroke apart. Both open a dialog first; `esc` closes it without a request.

- `P` **rerun** starts a new run with the same target as one that has finished — same commit, same selector, same pull request. It is what *Rerun* does in the web UI. The target comes from the old run, so a rerun replays the commit that run was for, not whatever the branch points at now; push and let Bitbucket start its own run if what you want is the branch head.
- `X` **stop** signals a run that is still `PENDING` or `IN_PROGRESS` to stop. Bitbucket acknowledges the signal, not the outcome: the run keeps its state until it has wound itself down, so it turns `STOPPED` a refresh or two later.

Both read the run back before they ask (`checking pipeline #414…`). They have to — a Builds panel row is a commit status, which names a run through the build number in its URL and says nothing about that run's state. The dialog is built from that fresh copy:

```
 Rerun pipeline #414?
 ─────────────────────────────
 FAILED · PUSH · by Linus Torvalds
 PR #103 bugfix/upstream-timeout → main
 ─────────────────────────────
 enter: rerun   esc: cancel
```

The two rows are the run's state, trigger and creator, and its target — the same target line the pipeline view carries under its header. `X` asks `Stop pipeline #413?` with the same two rows, and confirms with `enter: stop`.

A refusal is a one-line message in the hint bar and no dialog:

| Message | Reason |
|---------|--------|
| `select a build in the Builds panel (3) first` | The key was pressed where there is no run to act on — the pull request list, a file diff, a review thread |
| `this build is not a Bitbucket pipeline` | The selected status comes from some other CI system, so its URL carries no build number to act on |
| `cannot rerun: pipeline #413 is still running` | `P` on a run that has not finished. Stop it first, or wait for it |
| `cannot stop: pipeline #414 has already completed` | `X` on a run that is over |
| `read-only mode: cannot rerun pipeline` | [Read-only mode](Config.md#read-only-mode) is on. It hides both keys and refuses both |
| `rerun pipeline already in progress` | The previous rerun or stop on this run has not answered yet |

The first two are decided before anything is fetched; the two `cannot …` lines come out of the read-back, so they describe the run as the server has it rather than as the build column drew it.

While the write is in flight the hint bar says `rerunning pipeline #414…` or `stopping pipeline #413…`. Then:

- A rerun reports the run it created — `pipeline #415 started (rerun of #414)` — and, as long as its pull request is still the one on screen, the detail panel drills straight into it, the same view `enter` on a build opens, so its steps fill in as Bitbucket schedules them. The pull request's build statuses are refetched as well, so the new run also appears in the Builds panel.
- A stop says `stop requested for pipeline #413` and forgets what it had cached about the run: the build statuses are refetched at once, and so is the run itself when the detail panel is showing it. From a step log nothing moves — the log you were reading stays on screen, and the run is read again the next time you open it; `r` there refreshes the pull request and its build statuses, not the run on screen.

A rejected write is reported in Bitbucket's own words — `stop pipeline failed: The specified pipeline has already completed.` when a run ends between the read-back and your `enter`.

Both keys need the `write:pipeline:bitbucket` scope. It is neither one of the five read scopes nor the `write:pullrequest:bitbucket` the six pull request actions use, and the two write scopes are independent, so a token can carry either without the other — see [the scopes in the README](../README.md#get-a-token). Unlike those six actions these two do not care what state the pull request is in: the pipelines of a merged pull request can still be rerun.

## Repos `[4]`

| Key | Action | Config key |
|-----|--------|------------|
| `enter` / `space` | Filter the pull request list to this repository | `pullRequests.open`, `pullRequests.select` |
| `o` | Open the repository in a browser | `pullRequests.browser` |
| `l` / `right` | Next panel (Status) | `pullRequests.focusRight` |
| `h` / `left` / `esc` | Previous panel (Builds) | `detail.focusLeft` |

The first row is `All repositories`, which clears the filter. Selecting a repository returns focus to the pull request list.

## Detail panel `[0]`

The detail panel shows one thing at a time. A pull request is split into four tabs — Overview, Files, Commits, Comments — and the Files tab drills into a single file's diff.

Common keys, whatever is on screen:

| Key | Action | Config key |
|-----|--------|------------|
| `j` / `k` | Move the cursor, or scroll in a text view | `navigation.down`, `navigation.up` |
| `ctrl+d` / `ctrl+u` | Half page down / up | `navigation.halfPageDown`, `navigation.halfPageUp` |
| `g` / `G` | Jump to the top / bottom | `navigation.top`, `navigation.bottom` |
| `h` / `left` / `esc` | Back one level, then out to the left panels | `detail.focusLeft` |
| `backspace` | Back one level only: file diff → Files tab, step log → pipeline → pull request | `detail.back` |
| `r` | Refresh the selected pull request, including the data of the active tab and the diff on screen | `universal.refresh` |

### Pull request tabs

| Key | Action | Config key |
|-----|--------|------------|
| `[` / `]` | Previous / next tab | `universal.prevTab`, `universal.nextTab` |
| `i` | Overview tab | `detail.overviewTab` |
| `d` | Files tab — the changed files | `detail.filesTab` |
| `m` | Commits tab | `detail.commitsTab` |
| `t` | Comments tab — the review threads | `detail.commentsTab` |

A tab's data is fetched the first time you open it and cached per pull request, so nothing is requested for a tab you never visit. The tab label carries the item count once it is known (`Files (3)`), an ellipsis while the fetch is in flight. The four jump keys only act while the detail panel is focused; from a left panel they fall through to that panel. Pressed inside a file diff, they leave the diff and land on the tab.

`enter` is the tab's own key, and does something different in each:

| Tab | `enter` | Other keys |
|-----|---------|------------|
| Overview | — | `o` opens the pull request, `u` the URL picker, `y` copies its URL |
| Files | Open the selected file's diff | `n` / `p` move to the next / previous file, `e` opens the file in your editor, `o` opens the pull request |
| Commits | Expand the full commit message | `o` opens the commit in a browser, `y` copies its URL |
| Comments | Expand the whole thread | `o` opens the thread in a browser, `y` copies its URL |

The Files tab lists a status letter (`A` added, `M` modified, `D` removed, `R` renamed, plus a gray `B` for a binary file) and the `+added −removed` counts. The Comments tab shows each thread as `path:line` (or `General`), whether it is resolved and its replies.

### File diff

`enter` on the Files tab, or `n` / `p` from inside a diff, opens one file's syntax-highlighted diff. The cursor is a line cursor: it moves over context, added, removed and hunk-header lines.

| Key | Action | Config key |
|-----|--------|------------|
| `j` / `k` | Move the line cursor | `navigation.down`, `navigation.up` |
| `n` / `p` | Open the next / previous changed file; it stops at both ends rather than wrapping | `detail.nextFile`, `detail.prevFile` |
| `e` | Open the file in your editor, at the cursor's line — in a new tmux window when running inside tmux | `detail.openEditor` |
| `enter` | Jump to the review thread on this line, when the line is marked with `●` | `pullRequests.open` |
| `o` | Open the pull request's diff in a browser | `pullRequests.browser` |
| `y` | Copy that URL | `pullRequests.copyURL` |
| `backspace` | Back to the Files tab, with the cursor on the file you were reading | `detail.back` |

A `●` in the marker column means the line carries at least one inline comment (a digit when there are several). Comment markers need the review threads, which are fetched alongside the diff, so they may appear a moment after the diff itself.

`e` opens the new side of the diff: on an added or context line its own line, on a removed line the next line that still exists, on a hunk header the hunk's first line. Files are resolved against the git repository lazybitbucket was started in, which is never checked against the pull request's repository — when the checkout is on another branch the hint bar says so and the file still opens. Inside tmux the editor gets a new window and the session stays on screen; elsewhere the terminal is handed over until the editor exits. See [Editor](Config.md#editor) for the editors that understand a line number and for `editorInTmux`.

### Pipeline run and step log

| Key | Action | Config key |
|-----|--------|------------|
| `j` / `k` | Move between pipeline steps, or scroll the log | `navigation.down`, `navigation.up` |
| `enter` / `space` | Open the log of the selected step | `pullRequests.open`, `pullRequests.select` |
| `backspace` | Back one level: step log → pipeline → pull request | `detail.back` |
| `h` / `left` / `esc` | Back one level, then to the left panels | `detail.focusLeft` |
| `o` | Open the pipeline run in a browser | `pullRequests.browser` |
| `P` | Rerun this run | `builds.rerun` |
| `X` | Stop this run | `builds.stop` |

`P` and `X` act on the run on screen here — including from inside a step log, which is where a failed step is usually read — and behave exactly as they do on the Builds panel; see [Rerun and stop a pipeline](#rerun-and-stop-a-pipeline).

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
