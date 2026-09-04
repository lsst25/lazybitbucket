package tui

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/config"
	"github.com/lsst25/lazybitbucket/pkg/diff"
	"github.com/lsst25/lazybitbucket/pkg/git"
)

// repoDiscoveryLimit is how many repositories are listed when none are
// configured. One page, one request; the status bar says when it is hit.
const repoDiscoveryLimit = 25

// stepLogTailBytes is how much of a step log is fetched — the tail is the part
// that explains a failure.
const stepLogTailBytes = 64 << 10

// currentUserLoadedMsg carries the account behind the credentials. Repository
// and project access tokens cannot read /user, so err is expected there.
type currentUserLoadedMsg struct {
	user *bitbucket.Account
	err  error
}

// reposLoadedMsg carries the configured or discovered repositories.
type reposLoadedMsg struct {
	repos []bitbucket.Repository
	err   error
}

// pullRequestsLoadedMsg carries one cross-repository fetch, keyed by the
// (state, query) pair every tab sharing it re-filters from.
type pullRequestsLoadedMsg struct {
	fetchKey string
	results  []bitbucket.RepoPullRequests
}

// pullRequestDetailLoadedMsg carries a single pull request fetched because the
// list item lacked review data. See App.previewEpoch.
type pullRequestDetailLoadedMsg struct {
	pr    *bitbucket.PullRequest
	epoch int
}

// statusesLoadedMsg carries the build statuses of the previewed pull request.
type statusesLoadedMsg struct {
	key       string
	statuses  []bitbucket.CommitStatus
	updatedOn time.Time
	err       error
	epoch     int
}

// previewDebounceMsg is delivered when a selection's debounce tick expires.
type previewDebounceMsg struct {
	key   string
	epoch int
}

// prefetchedMsg carries one warmed pull request. The handler chains the next
// prefetch, so exactly one prefetch request is in flight at a time.
type prefetchedMsg struct {
	key       string
	epoch     int
	statuses  []bitbucket.CommitStatus
	updatedOn time.Time
	pr        *bitbucket.PullRequest
	err       error
}

// pipelineLoadedMsg carries a pipeline run and its steps.
type pipelineLoadedMsg struct {
	key      string
	build    int
	pipeline *bitbucket.Pipeline
	steps    []bitbucket.PipelineStep
	err      error
}

// stepLogLoadedMsg carries one pipeline step's log tail.
type stepLogLoadedMsg struct {
	pipelineUUID string
	stepUUID     string
	text         string
	err          error
}

// The four review messages carry no epoch. A response is correct for the pull
// request it names, so the handler caches it unconditionally and the view
// setter drops it when the user has walked off. Each carries the source commit
// hash the request described, which is what stamps the cache entry.

// diffStatLoadedMsg carries the changed files of a pull request.
type diffStatLoadedMsg struct {
	key     string
	entries []bitbucket.DiffStatEntry
	hash    string
	err     error
}

// commitsLoadedMsg carries the commits of a pull request.
type commitsLoadedMsg struct {
	key     string
	commits []bitbucket.Commit
	hash    string
	err     error
}

// threadsLoadedMsg carries the review conversation of a pull request, already
// grouped into threads.
type threadsLoadedMsg struct {
	key     string
	threads []bitbucket.Thread
	hash    string
	err     error
}

// fileDiffLoadedMsg carries one file's parsed diff. Parsing happens in the
// command rather than the handler, so a large diff is walked off the event
// loop.
type fileDiffLoadedMsg struct {
	key  string
	path string
	file *diff.File
	hash string
	err  error
}

// errorMsg is a failure worth interrupting the user for: it opens the error
// modal as well as marking the status panel offline.
type errorMsg struct{ err error }

// autoFetchTickMsg fires the periodic refresh.
type autoFetchTickMsg struct{}

// Git messages. There is no branch-creation path: checkout only.
type gitCheckoutDoneMsg struct{ name string }
type gitErrorMsg struct{ err error }

func autoFetchTick(every time.Duration) tea.Cmd {
	return tea.Tick(every, func(time.Time) tea.Msg { return autoFetchTickMsg{} })
}

// gitCheckoutBranch checks out an existing local branch, or creates a local
// branch tracking a remote one. It never creates a branch from scratch: the
// pull request's source branch exists somewhere or the user must fetch.
func (a *App) gitCheckoutBranch(name string) tea.Cmd {
	repoPath := a.gitRepoPath
	return func() tea.Msg {
		switch git.ResolveBranchAction(repoPath, name) {
		case git.ActionCheckout:
			if err := git.Checkout(repoPath, name); err != nil {
				return gitErrorMsg{err: err}
			}
			return gitCheckoutDoneMsg{name: name}
		case git.ActionCheckoutTracking:
			if err := git.CheckoutTracking(repoPath, name); err != nil {
				return gitErrorMsg{err: err}
			}
			local := name
			if _, after, ok := strings.Cut(name, "/"); ok {
				local = after
			}
			return gitCheckoutDoneMsg{name: local}
		default:
			return gitErrorMsg{err: errors.New("branch " + name + " not found locally or on a remote — fetch first")}
		}
	}
}

func (a *App) fetchCurrentUser() tea.Cmd {
	ctx, client := a.context(), a.client
	return func() tea.Msg {
		user, err := client.GetCurrentUser(ctx)
		return currentUserLoadedMsg{user: user, err: err}
	}
}

// fetchRepos returns the configured repositories as stubs without a request,
// or discovers the first page of repositories the user is a member of.
func (a *App) fetchRepos() tea.Cmd {
	workspace := a.workspace
	if len(a.cfg.Bitbucket.Repositories) > 0 {
		slugs := slices.Clone(a.cfg.Bitbucket.Repositories)
		return func() tea.Msg {
			repos := make([]bitbucket.Repository, 0, len(slugs))
			for _, slug := range slugs {
				repos = append(repos, bitbucket.Repository{Slug: slug, FullName: workspace + "/" + slug})
			}
			return reposLoadedMsg{repos: repos, err: nil}
		}
	}
	ctx, client := a.context(), a.client
	a.logFlag.Store(true)
	return func() tea.Msg {
		repos, err := client.ListRepositories(ctx, workspace, bitbucket.ListRepositoriesOptions{
			Role:     "member",
			Query:    "",
			Sort:     "",
			MaxItems: repoDiscoveryLimit,
		})
		return reposLoadedMsg{repos: repos, err: err}
	}
}

// listOptionsForKey builds the request options of the first tab that shares a
// fetch key; tabs with the same key are identical as far as the API goes.
func (a *App) listOptionsForKey(fetchKey string) bitbucket.ListPullRequestsOptions {
	tabs := a.tabConfigs()
	tab := tabs[0]
	for index := range tabs {
		if a.fetchKeyOf(index) == fetchKey {
			tab = tabs[index]
			break
		}
	}
	maxItems := config.DefaultMaxResults
	if a.cfg != nil {
		maxItems = a.cfg.ResolveMaxResults(tab)
	}
	return bitbucket.ListPullRequestsOptions{
		States:   []bitbucket.PullRequestState{bitbucket.PullRequestState(tab.ResolveState())},
		Query:    tab.Query,
		Sort:     "",
		MaxItems: maxItems,
		Fields:   "",
	}
}

// fetchPullRequests lists the pull requests behind one fetch key across every
// repository. A second request for a key already in flight is dropped.
func (a *App) fetchPullRequests(fetchKey string) tea.Cmd {
	if a.workspace == "" || len(a.repoSlugs) == 0 || a.inFlight[fetchKey] {
		return nil
	}
	if a.inFlight == nil {
		a.inFlight = make(map[string]bool)
	}
	a.inFlight[fetchKey] = true

	ctx, client := a.context(), a.client
	workspace := a.workspace
	repos := slices.Clone(a.repoSlugs)
	opts := a.listOptionsForKey(fetchKey)
	a.logFlag.Store(true)

	return func() tea.Msg {
		results := bitbucket.FetchPullRequests(ctx, client, workspace, repos, opts, 0)
		return pullRequestsLoadedMsg{fetchKey: fetchKey, results: results}
	}
}

func (a *App) fetchPullRequestDetail(pr *bitbucket.PullRequest, epoch int) tea.Cmd {
	if pr == nil {
		return nil
	}
	ctx, client := a.context(), a.client
	workspace, slug, id := pr.Workspace(), pr.RepoSlug(), pr.ID
	return func() tea.Msg {
		detail, err := client.GetPullRequest(ctx, workspace, slug, id)
		if err != nil {
			return pullRequestDetailLoadedMsg{pr: nil, epoch: epoch}
		}
		return pullRequestDetailLoadedMsg{pr: detail, epoch: epoch}
	}
}

func (a *App) fetchStatuses(pr *bitbucket.PullRequest, epoch int) tea.Cmd {
	if pr == nil {
		return nil
	}
	ctx, client := a.context(), a.client
	workspace, slug, id := pr.Workspace(), pr.RepoSlug(), pr.ID
	key, updatedOn := pr.Key(), pr.UpdatedOn
	return func() tea.Msg {
		statuses, err := client.ListPullRequestStatuses(ctx, workspace, slug, id)
		return statusesLoadedMsg{key: key, statuses: statuses, updatedOn: updatedOn, err: err, epoch: epoch}
	}
}

// prefetchNext pops the head of the prefetch queue and warms it: statuses
// always, detail as well when the list item carries no review data. Errors are
// carried, not raised — a prefetch failure must never interrupt the user.
func (a *App) prefetchNext() tea.Cmd {
	for len(a.prefetchQueue) > 0 {
		key := a.prefetchQueue[0]
		a.prefetchQueue = a.prefetchQueue[1:]
		pr := a.pullRequestByKey(key)
		if pr == nil {
			continue
		}
		ctx, client := a.context(), a.client
		workspace, slug, id := pr.Workspace(), pr.RepoSlug(), pr.ID
		updatedOn := pr.UpdatedOn
		needDetail := !pr.HasReviewData()
		epoch := a.prefetchEpoch
		return func() tea.Msg {
			msg := prefetchedMsg{
				key: key, epoch: epoch, statuses: nil,
				updatedOn: updatedOn, pr: nil, err: nil,
			}
			statuses, err := client.ListPullRequestStatuses(ctx, workspace, slug, id)
			msg.statuses, msg.err = statuses, err
			if needDetail {
				if detail, detailErr := client.GetPullRequest(ctx, workspace, slug, id); detailErr == nil {
					msg.pr = detail
				} else if msg.err == nil {
					msg.err = detailErr
				}
			}
			return msg
		}
	}
	return nil
}

func (a *App) fetchPipeline(pr *bitbucket.PullRequest, build int) tea.Cmd {
	if pr == nil {
		return nil
	}
	ctx, client := a.context(), a.client
	workspace, slug := pr.Workspace(), pr.RepoSlug()
	key := pr.Key()
	ref := strconv.Itoa(build)
	a.logFlag.Store(true)
	return func() tea.Msg {
		msg := pipelineLoadedMsg{key: key, build: build, pipeline: nil, steps: nil, err: nil}
		pipeline, err := client.GetPipeline(ctx, workspace, slug, ref)
		if err != nil {
			msg.err = err
			return msg
		}
		msg.pipeline = pipeline
		steps, err := client.ListPipelineSteps(ctx, workspace, slug, ref)
		if err != nil {
			msg.err = err
			return msg
		}
		msg.steps = steps
		return msg
	}
}

func (a *App) fetchStepLog(pr *bitbucket.PullRequest, pipelineUUID, stepUUID string) tea.Cmd {
	if pr == nil {
		return nil
	}
	ctx, client := a.context(), a.client
	workspace, slug := pr.Workspace(), pr.RepoSlug()
	a.logFlag.Store(true)
	return func() tea.Msg {
		text, err := client.GetPipelineStepLog(ctx, workspace, slug, pipelineUUID, stepUUID,
			bitbucket.StepLogOptions{TailBytes: stepLogTailBytes})
		return stepLogLoadedMsg{pipelineUUID: pipelineUUID, stepUUID: stepUUID, text: text, err: err}
	}
}

// fetchDiffStat lists the files a pull request changes, with their statuses and
// +/- counts. It is the Files tab's only request; the per-file diffs come
// separately, one file at a time.
func (a *App) fetchDiffStat(pr *bitbucket.PullRequest) tea.Cmd {
	if pr == nil {
		return nil
	}
	ctx, client := a.context(), a.client
	workspace, slug, id := pr.Workspace(), pr.RepoSlug(), pr.ID
	key, hash := pr.Key(), sourceHash(pr)
	a.logFlag.Store(true)
	return func() tea.Msg {
		entries, err := client.ListPullRequestDiffStat(ctx, workspace, slug, id)
		return diffStatLoadedMsg{key: key, entries: entries, hash: hash, err: err}
	}
}

// fetchCommits lists a pull request's commits.
func (a *App) fetchCommits(pr *bitbucket.PullRequest) tea.Cmd {
	if pr == nil {
		return nil
	}
	ctx, client := a.context(), a.client
	workspace, slug, id := pr.Workspace(), pr.RepoSlug(), pr.ID
	key, hash := pr.Key(), sourceHash(pr)
	a.logFlag.Store(true)
	return func() tea.Msg {
		commits, err := client.ListPullRequestCommits(ctx, workspace, slug, id, bitbucket.ListOptions{MaxItems: 0})
		return commitsLoadedMsg{key: key, commits: commits, hash: hash, err: err}
	}
}

// fetchThreads reads a pull request's comments and groups them into threads.
// The API returns one flat list, so the grouping is client-side and belongs off
// the event loop with the request.
func (a *App) fetchThreads(pr *bitbucket.PullRequest) tea.Cmd {
	if pr == nil {
		return nil
	}
	ctx, client := a.context(), a.client
	workspace, slug, id := pr.Workspace(), pr.RepoSlug(), pr.ID
	key, hash := pr.Key(), sourceHash(pr)
	a.logFlag.Store(true)
	return func() tea.Msg {
		comments, err := client.ListPullRequestComments(ctx, workspace, slug, id, bitbucket.ListOptions{MaxItems: 0})
		if err != nil {
			return threadsLoadedMsg{key: key, threads: nil, hash: hash, err: err}
		}
		return threadsLoadedMsg{key: key, threads: bitbucket.BuildThreads(comments), hash: hash, err: nil}
	}
}

// fetchFileDiff reads and parses one file's diff. escaped is the diffstat's
// escaped_path, which is what the endpoint's path= parameter wants; path is the
// display path the view is showing and the parsed file is matched against.
func (a *App) fetchFileDiff(pr *bitbucket.PullRequest, path, escaped string) tea.Cmd {
	if pr == nil {
		return nil
	}
	ctx, client := a.context(), a.client
	workspace, slug, id := pr.Workspace(), pr.RepoSlug(), pr.ID
	key, hash := pr.Key(), sourceHash(pr)
	opts := bitbucket.DiffOptions{Path: escaped, Context: 0, DiffURL: pr.Links.Diff.Href}
	a.logFlag.Store(true)
	return func() tea.Msg {
		msg := fileDiffLoadedMsg{key: key, path: path, file: nil, hash: hash, err: nil}
		text, err := client.GetPullRequestDiff(ctx, workspace, slug, id, opts)
		if err != nil {
			msg.err = err
			return msg
		}
		files, parseErr := diff.Parse(text)
		if parseErr != nil {
			msg.err = parseErr
			return msg
		}
		msg.file = pickDiffFile(files, path)
		return msg
	}
}

// pickDiffFile is the parsed file the view should show. The endpoint is asked
// for one path but answers with a whole diff document, and a rename or an
// escaped path can make the header disagree with what was asked for, so the
// first file is the fallback.
func pickDiffFile(files []diff.File, path string) *diff.File {
	for index := range files {
		if files[index].Path() == path {
			return &files[index]
		}
	}
	if len(files) > 0 {
		return &files[0]
	}
	return nil
}
