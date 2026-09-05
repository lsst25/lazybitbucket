# Roadmap

High-level plan for lazybitbucket. Ordered by value: each step should leave a usable tool.
Details live in issues/PRs, not here.

## 0. Foundation

- [x] `pkg/bitbucket` client: auth (API token / access token), pagination, error handling, fake client and demo server for tests
- [x] Config: workspace + list of repos, credentials, editor
- [x] Rename `cmd/lazyjira` → `cmd/lazybitbucket`, config dir, packaging

## 1. Browse and read PRs

- [x] List open PRs across the configured repos in one view (repo, title, author, state, approvals)
- [x] PR detail pane: description (Markdown via glamour), reviewers, approval state, source/target branch
- [x] Pipeline / build status shown per PR, with drill-down into the latest run

## 2. Review

- [x] Diff pane: file list + syntax-highlighted per-file diff, jump between files
- [x] Open the selected file (at the hunk line) in the configured editor, nvim by default
- [x] Custom commands on a PR (reuse lazyjira's custom-command infra), e.g. `tmux-pr-sessionizer` to start a Claude review session
- [x] Commits list per PR
- [x] Review threads: read inline comments anchored to file and line

## 3. Act on PRs

- [x] Merge with strategy choice (merge commit / squash / fast-forward), optional close-source-branch
- [x] Sync with target branch (rebase or merge), driven through local git
- [x] Edit title / description
- [x] Set / remove reviewers
- [x] Toggle draft state
- [x] Approve, and remove your own approval (`A`)

## 4. Find and create

- [ ] Search / filter PRs the way Bitbucket does: state, author, reviewer, target branch, text; saved filters
- [ ] Create a PR from the current branch: title, description, target, reviewers, draft flag

## Later / maybe

- [ ] Request changes, post and resolve comments (approving is done — see step 3)
- [ ] Trigger or re-run pipelines
- [ ] CLI mode for scripting
