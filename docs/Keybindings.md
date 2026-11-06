# Keybindings

Press `?` inside lazybitbucket to see the keys that apply to the panel you are in. Use `/` inside the help popup to filter them.

Keys are remappable in `config.yml` under the `keybinding` section — see [Configuration](Config.md#keybindings). Setting a key **replaces** every default for that action: `navigation.down: n` means `j`, `down` and `ctrl+j` stop navigating down.

There is no `keybinding.repos` group. The Repos, Builds and Pipelines panels reuse the pull request `select`, `open` and `focusRight` actions, so remapping those remaps them everywhere. There is a `keybinding.builds` group, but it carries only the two pipeline actions — `rerun` and `stop` — which no other panel has, and a `keybinding.pipelines` group carrying only `openPullRequest`.

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

The numbers belong to the view you are in. The table above is the pull requests view; in the [pipelines view](#pipelines-view) `2` focuses Pipelines, `3` and `4` both focus Repos, and `l` / `h` cycle Status → Pipelines → Repos → Status. The config keys keep their names either way.

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
| `⇧tab` | Switch between the pull requests view and the [pipelines view](#pipelines-view) | `universal.nextView` |

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
| `C` | Comment… — on the pull request, a thread, a file or a diff line, depending on where you are | `pullRequests.comment` |
| `N` | Open a pull request for the checked-out branch… | `pullRequests.create` |
| `T` | Resolve the conversation in hand, or reopen it… | `pullRequests.resolve` |

## Pull request actions

The nine keys above are the only ones that write — `P` and `X` write too, but to a pipeline run rather than the pull request (see [Rerun and stop a pipeline](#rerun-and-stop-a-pipeline)). They work from the pull request list and from the detail panel alike — whatever has focus, they act on the pull request the list cursor is on — and each one asks before it writes. Six open a dialog: nothing is sent until you confirm, and `esc` closes a dialog without a request. `C` is the seventh: it opens the [comment popup](#c--comment-or-reply) — the same input the title editor uses — and what you send there is the confirmation.

A refusal is a one-line message in the hint bar and no dialog:

| Message | Reason |
|---------|--------|
| `read-only mode: cannot merge` | [Read-only mode](Config.md#read-only-mode) is on. It refuses all nine, sync, commenting, creating and resolving included |
| `cannot merge: pull request is MERGED` | The pull request is not `OPEN` any more. `C` is the exception: a merged or declined pull request still takes comments |
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

### `C` — comment or reply

`C` opens the same input popup `E` opens for a title, headed with what the comment will answer or hang off:

```
 ╭─Reply to Ken Thompson───────────────────────────────╮
 │ thanks, that clears it up█                          │
 ╰─────────────────────────────────────────────────────╯
```

Long text wraps inside the box rather than scrolling sideways, so a paragraph stays readable while you type it. `enter` sends, `esc` cancels, and the hint bar carries the keys.

What the comment answers, or hangs off, depends on where you press it:

| Where | What `C` writes |
|-------|-----------------|
| Inside a [thread](#thread) | A reply to that conversation |
| Comments tab, on a thread | A reply to the thread under the cursor |
| Comments tab with no comments yet | The first comment on the pull request |
| Overview tab | A new comment on the pull request |
| [File diff](#file-diff), on a line | An inline comment on that line — see below |
| File diff, on a hunk header | A comment on the file, which is where Bitbucket shows it: above the first hunk |
| Files tab | A comment on the file under the cursor, without opening its diff |
| Commits tab | Nothing — there is no conversation and no file under the cursor, and the hint bar says so |

### Inline comments

`C` inside a file's diff anchors the comment to the line the cursor is on, and the popup's title says which:

```
  18  21 │             return resp, nil
  19  22 │         }
 ╭─Comment on retry.go:13──────────────────────────────╮
 │ off by one?█                                        │
 ╰─────────────────────────────────────────────────────╯
```

The side comes from the line itself: an added or a context line anchors to the new side of the diff, a removed line to the old side — the same line the `●` marker would appear on. The title shows the file's base name and the line; the panel behind it already carries the path.

Sending leaves you exactly where you were, cursor on the same line, with the `●` on it lit up (a digit once a line carries more than one conversation) and the hint bar naming where the comment landed — `comment posted on internal/upstream/retry.go:13`. Reviewing a file is a pass down it, and being thrown into a thread after every remark would break that pass.

`C` on a line that already carries a comment starts a *second* conversation there, which is what Bitbucket's own line threads do. To join the conversation that is already on the line, press `enter` on it — that jumps to the thread — and `C` there replies to it.

A line the diff has moved past is refused by Bitbucket, not by lazybitbucket: the failure is its own message in the hint bar, and the popup opens again with your text in it. Ranges spanning several lines are not implemented — a comment anchors to one line.

While the popup is open it owns the keyboard, exactly as the title editor does: every key is text, bindings and custom commands included, so a reply may say `q` without quitting. The three keys that are not text:

| Key | Action |
|-----|--------|
| `enter` | Send. An empty box just closes |
| `ctrl+e` | Continue in `editor` — the text so far is written to a temp `.md`, opened the way `E` opens a description, and posted when the editor exits. An empty file discards the comment. (In the title editor the same key stays readline's end-of-line) |
| `esc` | Close and drop what was typed |

A sent comment is folded straight into the conversation from Bitbucket's answer — no refetch, and mentions in it read as names because the answer carries the server's own rendering. A new comment on the pull request opens its thread, so the conversation you just started is what you are looking at.

Nothing typed is lost to a failure. A comment typed in the popup is asked again, prefilled with it and the error in the hint bar; one composed in the editor stays in its file and the hint bar names the path.

Editing a comment, deleting one and resolving a thread are not implemented — those still want the browser (`o`).

### `N` — open a pull request

`N` opens a pull request for the branch the terminal is on. It works from anywhere in the pull request view, because it acts on the checkout rather than on whatever the cursor is pointing at.

It reads first — the last commit's subject, the repository's default branch, its default reviewers, and whether the branch is on the remote at all — and asks the rest in one form:

```
 ╭────────────────────────────────────────────────────────────╮
 │ New pull request in web-app                                │
 │                                                            │
 │ Title                  feat: keyboard navigation, second … │
 │ Description            ‹ none ›                            │
 │ Target                 develop                             │
 │ Reviewers              ‹ default reviewers (2) ›           │
 │ Draft                  [ ]                                 │
 │ Push the branch first  [x]                                 │
 │ from feature/keyboard-nav                                  │
 │                                                            │
 │ enter: Create   j/k: other rows   esc: cancel              │
 ╰────────────────────────────────────────────────────────────╯
```

| Row | What it does |
|-----|--------------|
| Title | Prefilled with the subject of the last commit, or the branch name when there is no commit. `enter` edits it inline |
| Description | `none`, or `write in $EDITOR` — the editor opens after you confirm, the way `E` edits a description, and what you save becomes the description |
| Target | The repository's own default branch, editable. A pull request against its own source branch is refused before any request |
| Reviewers | `default reviewers (n)` when the repository has some, `none`, or `pick…` — which opens the same checklist `V` opens, over the default reviewers and the workspace members, with the defaults checked |
| Draft | Open it as a draft |
| Push the branch first | Only shown when the branch is not on the remote yet, and on by default: Bitbucket cannot open a pull request for a branch its own repository does not have. The push is `git push <remote> HEAD`, and a push that fails stops the create with git's own message |

Confirming runs whatever the form asked for — the push, then the editor, then the picker — and then opens the pull request. What was created goes to the top of the open lists, the cursor onto it and the detail panel into it, so the next keystroke acts on what you just opened.

`N` is refused, with a line in the hint bar and no form, when lazybitbucket was not started inside a git checkout, when that checkout is not one of the configured repositories, when `HEAD` is detached, and in read-only mode. The pipelines view has no branch of its own, so it says so there too.

Bitbucket's own refusals — a source branch it does not have, a pull request already open for the same pair of branches — arrive as its own message in the hint bar.

### `T` — resolve a conversation

Bitbucket resolves a *conversation*, not a comment: the state hangs off the comment that starts the thread, and the whole thread carries it. `T` therefore acts on the conversation in hand — the one the [thread section](#thread) is showing, or the one under the cursor on the Comments tab — whichever comment of it you happen to be looking at.

One key, two directions, and the dialog says which:

```
 ╭────────────────────────────────────────────────────────────╮
 │ Resolve internal/upstream/retry.go:16?                     │
 │                                                            │
 │ Demo User · 1 reply                                        │
 │                                                            │
 │ enter: confirm   esc: cancel                               │
 ╰────────────────────────────────────────────────────────────╯
```

On a conversation that is already resolved the question is `Reopen …?` instead, and confirming takes the resolution off. What you resolve leaves the list at once — [resolved conversations are hidden](#f--show-the-resolved-conversations) — and is marked `✔ resolved by …` wherever it is shown — the answer carries who resolved it and when — and the hint bar says `resolved internal/upstream/retry.go:16`.

Like commenting, and unlike the other writes, it is not refused on a merged or declined pull request.

### `f` — show the resolved conversations

Resolved conversations are **off the list from the start**: a thread someone has finished with is not what you opened the Comments tab for. `f` asks for them back, and `f` again puts them away. Set [`gui.showResolvedComments: true`](Config.md#gui) to start the other way round.

A resolved conversation that is on the list is marked twice over, so it is tellable apart at a glance from the open ones around it: a green `✔` leads the line, and the anchor is drawn in gray rather than the cyan an open conversation gets.

```
 ✔ internal/upstream/retry.go:13  resolved by Ken Thompson · 23h ago
   internal/upstream/retry.go:16 · 1 reply
```

It is a view, not a filter on the data: the Comments tab label says how many are being kept back —

```
[0] #103 api-gateway - Overview - Files (3) - Commits (3) - Comments (8 · 2 hidden)
```

— and everything that indexes the list follows it, so `enter`, `T`, `C`, `o` and `y` act on what you can see rather than on a row underneath a hidden one. Jumping to a specific conversation from a diff line (`enter` on a `●`) turns the filter off when that conversation is one of the hidden ones, rather than sending you to a list without it. A conversation open in the [thread section](#thread) stays on screen when it is resolved — only the list hides it — so `T` there can still reopen what it just resolved.

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

Both keys need the `write:pipeline:bitbucket` scope. It is neither one of the five read scopes nor the `write:pullrequest:bitbucket` the seven pull request actions use, and the two write scopes are independent, so a token can carry either without the other — see [the scopes in the README](../README.md#get-a-token). Unlike the six that dialog these two do not care what state the pull request is in: the pipelines of a merged pull request can still be rerun.

## Pipelines view

The Builds panel answers one question — what has *this pull request* been running — and nothing else. A branch build, a scheduled run or a run someone started by hand never appears there, because no pull request owns it. The pipelines view is the other half: what the repositories themselves have been running lately. `⇧tab` switches to it, and `⇧tab` switches back.

```
 Pull requests view              Pipelines view
 [1] Status                      [1] Status · Pipelines
 [2] Pull Requests               [2] Pipelines
 [3] Builds                      [3] Repos
 [4] Repos                       [0] Detail  (shared)
 [0] Detail
```

A view swaps the left column and nothing else — the detail panel `[0]` belongs to both. The panel numbers are per view, so Repos is `[3]` here rather than `[4]`; `3` and `4` both focus it, so muscle memory lands somewhere sane either way. Switching moves focus onto the view's `[2]` panel and re-points the detail panel at what that view has selected — the run here, the previewed pull request there. Everything else on the side you left is kept, selection, scroll and caches included, so a round trip puts you back where you were. The status panel says which side you are on: `[1] Status · Pipelines`. To start there rather than on the pull requests, set [`gui.startView`](Config.md#gui).

### The list

The Pipelines panel lists the recent runs of **every configured repository** merged into one list, newest first, with the repository slug in the row. A repository filter set on the Repos panel narrows the list to that repository, and changing the filter refetches it.

```
 ✔ #415  api-gateway  PR #103 bugfix/upstream-timeout  4m28s  2h ago
```

That is the state glyph, the build number, the repository, what the run was for, how long it took and how long ago it started. The target column is the flexible one: `PR #<id> <source branch>` for a pull request build, the branch for a branch build, and `target <type>` for anything else. When the window leaves it less than ten columns the fixed ones give way in order — the age first, then the duration, then the repository slug. A run that has not finished yet shows no duration.

The glyphs are the Builds panel's: `✔` successful, `✘` failed or errored, `●` in progress or pending, `■` stopped, `○` expired or a state this version does not know. `/` filters the list on everything the row draws — untruncated — plus the trigger and the creator.

Where the rows would be, the panel may say:

| Text | Meaning |
|------|---------|
| `Loading pipelines…` | The fetch is in flight |
| `No pipeline runs` | Every repository answered, none of them has run anything |
| `No matching runs` | The `/` filter matched nothing |

A repository that could not be listed does not take the others down with it: the runs that came back are shown and the help bar notes the gap — `2 of 6 repositories could not be listed`. Only when every repository fails does the panel show the error instead of rows, the way the Builds panel shows one.

### Keys

| Key | Action | Config key |
|-----|--------|------------|
| `j` / `k` | Move the cursor; the run under it is shown in the detail panel at once | `navigation.down`, `navigation.up` |
| `enter` / `space` | Move focus to the detail panel, on the selected run | `pullRequests.open`, `pullRequests.select` |
| `l` / `right` | Next panel (Repos) | `pullRequests.focusRight` |
| `h` / `left` / `esc` | Previous panel (Status) | `detail.focusLeft` |
| `o` | Open the run in a browser | `pullRequests.browser` |
| `y` | Copy the run's URL to the clipboard | `pullRequests.copyURL` |
| `r` | Refresh the selected run's steps | `universal.refresh` |
| `R` | Refetch the list | `universal.refreshAll` |
| `/` | Filter the list | `universal.search` |
| `P` | Rerun the selected run — see [Rerun and stop a pipeline](#rerun-and-stop-a-pipeline) | `builds.rerun` |
| `X` | Stop the selected run | `builds.stop` |
| `L` | Jump to the pull request the selected run ran for | `pipelines.openPullRequest` |

Selecting a row needs no request: the run is drawn from the list payload, which is the whole run. Its steps are fetched a moment later, once the cursor settles, and the detail panel says `Loading steps…` until they arrive. `enter` there opens a step's log, and `backspace` walks back out to the run — exactly the drill-down `enter` on a build opens in the other view. A standalone run has no pull request under it, so `backspace` stops at the run; `h` / `esc` leave the detail panel for the left column.

`P` and `X` are the same two keys the Builds panel has, with the same dialogs, the same refusals and the same [`write:pipeline:bitbucket` scope](#rerun-and-stop-a-pipeline) — one action wherever it is pressed, which is why they stay in the `keybinding.builds` group. What differs is only what happens afterwards: in this view a rerun opens the run it created in the detail panel and refetches the list, and a stop refetches the list. They work here with no pull request loaded at all.

`L` goes the other way, from a run back to the work that caused it. It switches to the pull requests view and puts the cursor on that pull request. Two refusals, both a line in the hint bar and nothing else:

| Message | Reason |
|---------|--------|
| `this run is not a pull request build` | The run's target is a branch, a tag or a manual selector — there is no pull request to go to |
| `pull request #103 is not in this tab` | The run names one, but the active pull request tab does not hold it. Switch tabs (`[` / `]`) or widen the filter and press `L` again |

### Refreshing

The list is fetched on the first switch into the view, on `R`, when the repository filter changes while the view is active, and after a rerun or a stop performed here. It is deliberately **not** on the [`refreshInterval`](Config.md#auto-refresh-refreshinterval) tick that keeps the pull request list warm: Bitbucket pages this endpoint at ten runs at a time, so one refresh costs a round of requests per configured repository, and a background tick doing that every couple of minutes would spend the hourly request budget on a panel you are not looking at. `R` when you want it fresh.

[Custom commands](Config.md#custom-commands) do not fire in this view — not on the Pipelines panel, not on the Repos panel beside it. The five contexts are all declared against the pull requests view, and four of the five need a selected pull request to fill their scopes; rather than have a command written against `{{.PR}}` fire with nothing behind it, no context is active here at all. They are not listed in the help bar or the `?` overlay here either, for the same reason.

### Keys that belong to the other view

The pull request keys are not merely absent from this view's help — they refuse. `M`, `S`, `E`, `V`, `D`, `A`, `c` and `u` all answer `no pull request in the pipelines view` and do nothing. The pull request the other view has under its cursor survives the switch, and acting on it from here would be a write against something nothing on this screen names; `⇧tab` takes you to where those keys mean something.

## Repos `[4]`

| Key | Action | Config key |
|-----|--------|------------|
| `enter` / `space` | Filter the pull request list to this repository | `pullRequests.open`, `pullRequests.select` |
| `o` | Open the repository in a browser | `pullRequests.browser` |
| `l` / `right` | Next panel (Status) | `pullRequests.focusRight` |
| `h` / `left` / `esc` | Previous panel (Builds) | `detail.focusLeft` |

The first row is `All repositories`, which clears the filter. Selecting a repository returns focus to the pull request list.

The same panel is `[3]` in the [pipelines view](#pipelines-view), where `l` goes on to Status and `h` back to Pipelines, and where the filter it sets narrows the run list instead.

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

Focusing the detail panel fetches the Files, Commits and Comments tabs together, so walking the tabs of the pull request in front of you costs no pause; `gui.prefetchDetailTabs: false` turns that off and goes back to fetching a tab the first time you open it, nothing at all for a tab you never visit. Either way the data is cached per pull request, and moving the cursor in the list — which only previews the pull request — fetches no tab. The tab label carries the item count once it is known (`Files (3)`), an ellipsis while the fetch is in flight; Comments is the exception, showing the pull request's own comment count from the start and the number of comments it loaded — replies included — once the threads are in. The four jump keys only act while the detail panel is focused; from a left panel they fall through to that panel. Pressed inside a file diff, they leave the diff and land on the tab.

`enter` is the tab's own key, and does something different in each:

| Tab | `enter` | Other keys |
|-----|---------|------------|
| Overview | — | `C` comments on the pull request, `o` opens the pull request, `u` the URL picker, `y` copies its URL |
| Files | Open the selected file's diff | `v` marks the file viewed — see [`v` — mark a file viewed](#v--mark-a-file-viewed), `n` / `p` move to the next / previous file, `e` opens the file in your editor, `C` comments on the file, `o` opens the pull request |
| Commits | Expand the full commit message | `o` opens the commit in a browser, `y` copies its URL |
| Comments | Open the thread in the panel | `C` replies to the thread under the cursor, `T` resolves or reopens it, `f` shows the resolved ones (hidden by default), `o` opens the thread in a browser, `y` copies its URL |

The Files tab lists a green `✔` on each file you marked viewed, a status letter (`A` added, `M` modified, `D` removed, `R` renamed, plus a gray `B` for a binary file) and the `+added −removed` counts. The Comments tab shows each thread as `path:line` (or `General`), whether it is resolved and its replies; a thread longer than eight lines is collapsed behind an ellipsis in the list and `enter` opens it whole — see [Thread](#thread). A comment body — like the Overview's description — shows an `@`-mention by display name, `@Grace Hopper` in the theme's blue for the `@{account_id}` the Markdown stores; a mention Bitbucket itself left unresolved, a departed colleague or one written inside a code span, stays as the id, which is what the web UI shows too.

### File diff

`enter` on the Files tab, or `n` / `p` from inside a diff, opens one file's syntax-highlighted diff. The cursor is a line cursor: it moves over context, added, removed and hunk-header lines.

| Key | Action | Config key |
|-----|--------|------------|
| `j` / `k` | Move the line cursor | `navigation.down`, `navigation.up` |
| `n` / `p` | Open the next / previous changed file; it stops at both ends rather than wrapping | `detail.nextFile`, `detail.prevFile` |
| `e` | Open the file in your editor, at the cursor's line — in a new tmux window when running inside tmux | `detail.openEditor` |
| `v` | Mark this file viewed and open the next file you have not viewed; `v` on a viewed file clears the mark | `detail.toggleViewed` |
| `C` | Comment on the line under the cursor — see [`C` — comment or reply](#c--comment-or-reply) | `pullRequests.comment` |
| `enter` | Jump to the review thread on this line, when the line is marked with `●` | `pullRequests.open` |
| `o` | Open the pull request's diff in a browser | `pullRequests.browser` |
| `y` | Copy that URL | `pullRequests.copyURL` |
| `backspace` | Back to the Files tab, with the cursor on the file you were reading | `detail.back` |

A `●` in the marker column means the line carries at least one inline comment (a digit when there are several). Comment markers need the review threads, which are fetched alongside the diff, so they may appear a moment after the diff itself.

`e` opens the new side of the diff: on an added or context line its own line, on a removed line the next line that still exists, on a hunk header the hunk's first line. Files are resolved against the git repository lazybitbucket was started in, which is never checked against the pull request's repository — when the checkout is on another branch the hint bar says so and the file still opens. Inside tmux the editor gets a new window and the session stays on screen; elsewhere the terminal is handed over until the editor exits. See [Editor](Config.md#editor) for the editors that understand a line number and for `editorInTmux`.

### `v` — mark a file viewed

`v` marks a changed file as viewed, the way the checkbox in the Bitbucket web UI does. On the Files tab it marks the file under the cursor. In a file diff it marks the file on screen.

After a mark, lazybitbucket goes to the next file that you have not viewed. On the Files tab the cursor moves to it. In a file diff its diff opens. The search wraps to the top of the list. When no file is left, a diff goes back to the Files tab and the hint bar says `every file viewed`. Thus, a review is a pass of `v` presses through the pull request.

`v` on a viewed file clears the mark, and the cursor stays where it is.

The Files tab shows a green `✔` and a gray path on each viewed file, and the tab label counts them:

```
 Files (5 · 2 viewed)
 ✔ M internal/rate/bucket.go          +2 −1
   A docs/rate.md                        +2
```

A mark is kept only while the file does not change. If a push changes a file, its mark goes away, like in the web UI. Marks on files that the push did not touch stay. To find the changed files, lazybitbucket sends one diffstat request between the old and the new head commit, limited to the marked files. Until the answer comes, the marks made before the push are not shown. If the request fails, the marks stay hidden, and lazybitbucket asks again the next time you open the Files tab. If Bitbucket no longer has the old commit, for example after a force push, the marks made at that commit are removed.

The marks stay on this computer only. Bitbucket's public API has no viewed-file state, so the web UI and lazybitbucket do not share their marks. The marks are kept in `viewed.json` in the [config directory](Config.md), and marks that did not change for 90 days are removed. You can delete the file safely. `v` writes nothing to Bitbucket, so it also works in read-only mode. Under `--demo` the marks stay in memory.

### Thread

`enter` on the Comments tab opens that conversation in the panel, the way `enter` on the Files tab opens a diff and `enter` on a pipeline step opens its log. The list collapses a long thread; the section shows it whole — anchor line, root comment and every reply — and scrolls.

| Key | Action | Config key |
|-----|--------|------------|
| `j` / `k` | Scroll the conversation | `navigation.down`, `navigation.up` |
| `C` | Reply to this conversation — see [`C` — comment or reply](#c--comment-or-reply) | `pullRequests.comment` |
| `T` | Resolve this conversation, or reopen it — see [`T` — resolve a conversation](#t--resolve-a-conversation) | `pullRequests.resolve` |
| `o` | Open the comment in a browser | `pullRequests.browser` |
| `y` | Copy the comment URL | `pullRequests.copyURL` |
| `backspace` | Back to the Comments tab, with the cursor on the thread you were reading | `detail.back` |
| `h` / `left` / `esc` | Back to the Comments tab, then to the left panels | `detail.focusLeft` |

The four tab keys (`i` / `f` / `m` / `t`) leave the conversation and land on their tab. A refresh that no longer carries the thread — it was deleted, or the comments were refetched after a push — leaves a line saying so rather than a stale copy of it.

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

The same two levels are what the [pipelines view](#pipelines-view) drills into, with one difference: a run reached from there has no pull request under it, so `backspace` stops at the run rather than walking on, and the run's header carries the repository slug the panel title would otherwise have given it.

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
