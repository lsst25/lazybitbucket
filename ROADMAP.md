# Roadmap

High-level plan for lazybitbucket. Ordered by value: each step should leave a usable tool.
Details live in issues/PRs, not here.

## 0. Foundation

- [ ] `pkg/bitbucket` client: auth (app password / access token), pagination, error handling, fake server for tests
- [ ] Config: workspace + list of repos, credentials, editor
- [ ] Rename `cmd/lazyjira` → `cmd/lazybitbucket`, config dir, packaging

## 1. Browse and read PRs

- [ ] List open PRs across the configured repos in one view (repo, title, author, state, approvals)
- [ ] PR detail pane: description (Markdown via glamour), reviewers, approval state, source/target branch
- [ ] Pipeline / build status shown per PR, with drill-down into the latest run

## 2. Review

- [ ] Diff pane: file list + syntax-highlighted per-file diff, jump between files
- [ ] Open the selected file (at the hunk line) in the configured editor, nvim by default
- [ ] Custom commands on a PR (reuse lazyjira's custom-command infra), e.g. `tmux-pr-sessionizer` to start a Claude review session
- [ ] Commits list per PR
- [ ] Review threads: read inline comments anchored to file and line

## 3. Act on PRs

- [ ] Merge with strategy choice (merge commit / squash / fast-forward), optional close-source-branch
- [ ] Sync with target branch (rebase or merge), driven through local git
- [ ] Edit title / description
- [ ] Set / remove reviewers
- [ ] Toggle draft state

## 4. Find and create

- [ ] Search / filter PRs the way Bitbucket does: state, author, reviewer, target branch, text; saved filters
- [ ] Create a PR from the current branch: title, description, target, reviewers, draft flag

## Later / maybe

- [ ] Approve / request changes, post and resolve comments
- [ ] Trigger or re-run pipelines
- [ ] CLI mode for scripting
