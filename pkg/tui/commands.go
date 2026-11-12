package tui

import (
	"errors"
	"fmt"
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
// loop, and so does the external diff renderer when one is configured.
//
// rendered is one finished code cell per line of the diff document the file
// was parsed out of, indexed by diff.Line.Source; it is nil when no renderer
// is configured and when the run failed, which rendererErr then explains. A
// renderer failure is never the file's own failure: err stays nil and the diff
// is shown with the built-in colouring.
type fileDiffLoadedMsg struct {
	key         string
	path        string
	file        *diff.File
	hash        string
	err         error
	rendered    []string
	rendererErr error
}

// The write messages. Every one of them is keyed by pull request rather than
// stamped with an epoch: a write answers for the pull request it named, and the
// handler patches that pull request wherever it is held even when the user has
// walked off to another one. What the key does gate is the on-screen half —
// only the previewed pull request's detail pane and review data are refreshed.

// pullRequestWrittenMsg is the result of every merge and every update: the pull
// request as the server left it, or the error that stopped it. verb is the
// action's own name, which is what the help bar reports.
type pullRequestWrittenMsg struct {
	key  string
	verb string
	pr   *bitbucket.PullRequest
	err  error
	// readErr is set when the write itself landed but reading the pull request
	// back failed. pr is then the cached copy patched locally, or nil when no
	// faithful patch was possible, and id names the pull request for the
	// help bar. The write is reported as done, with the read failure appended.
	readErr error
	id      int
}

// mergeQueuedMsg is a merge Bitbucket answered 202 to: it names the task to
// poll and how many polls have already been spent on it.
type mergeQueuedMsg struct {
	key       string
	workspace string
	slug      string
	id        int
	taskURL   string
	attempt   int
}

// reviewerCandidatesMsg carries the reviewer picker's rows, already ordered and
// deduplicated. note is the degradation line for a candidate source that could
// not be listed, which is not fatal — the picker still opens. err is fatal: the
// current reviewer set could not be read, and a picker that misstates it would
// unassign people on confirm.
//
// pr is set only when the pull request had to be read to establish that set. It
// is the freshest copy the app has, so the handler keeps it rather than paying
// for the same request again on the PUT that follows.
type reviewerCandidatesMsg struct {
	key        string
	pr         *bitbucket.PullRequest
	candidates []reviewerCandidate
	note       string
	err        error
}

// approvalStateMsg carries the pull request the approve key read to find out
// which way it goes. It exists because the dialog states the direction, and a
// list payload carries no participants to state it from — so the read happens
// before the dialog rather than inside the write it confirms.
//
// pr is the freshest copy the app has and is kept whatever the user does next;
// err means the direction is unknown and no dialog opens.
type approvalStateMsg struct {
	key string
	pr  *bitbucket.PullRequest
	err error
}

// pipelineActionStateMsg carries the run a pipeline key read before opening its
// dialog. Neither key can be answered from what the app already holds: the
// build row's status is the pull request's view of the run and lags it, and a
// cached run is as old as the last time the drill-down was opened — so a
// refusal built on either could name a state the run left minutes ago.
//
// verb says which key asked, because the refusal, the dialog and the failure
// line all differ between the two, and ref is the run, which is what every one
// of those lines names. prKey is the pull request the run was reached through,
// and is empty in the pipelines view, where a run stands on its own.
type pipelineActionStateMsg struct {
	ref      pipelineRef
	prKey    string
	verb     string
	pipeline *bitbucket.Pipeline
	err      error
}

// pipelineWrittenMsg is the result of a rerun or a stop. ref is the run that
// was acted on — the one that was rerun, not the one the rerun created — and
// pipeline is the run the server created, which only a rerun has: the stop
// endpoint answers 204 and nothing else. prKey is the pull request the write
// was started from, empty in the pipelines view.
type pipelineWrittenMsg struct {
	ref      pipelineRef
	prKey    string
	verb     string
	pipeline *bitbucket.Pipeline
	err      error
}

// descriptionEditedMsg is delivered when the editor holding a description temp
// file exits. The path and the text the file started with travel with it so the
// handler can read the file back, compare and delete it.
type descriptionEditedMsg struct {
	key      string
	path     string
	original string
	err      error
}

// syncDoneMsg is the result of a sync-with-target run: what happened, in the
// words the help bar shows, plus the branch git ended up on.
type syncDoneMsg struct {
	key     string
	summary string
	branch  string
	pushed  bool
	err     error
}

// errorMsg is a failure worth interrupting the user for: it opens the error
// modal as well as marking the status panel offline.
type errorMsg struct{ err error }

// autoFetchTickMsg fires the periodic refresh.
type autoFetchTickMsg struct{}

// Git messages. There is no branch-creation path: checkout only.
// gitCheckoutDoneMsg reports a finished checkout. note is an extra sentence
// for the hint bar — the branch was fast-forwarded, or could not be — or "".
type gitCheckoutDoneMsg struct {
	name string
	note string
}
type gitErrorMsg struct{ err error }

func autoFetchTick(every time.Duration) tea.Cmd {
	return tea.Tick(every, func(time.Time) tea.Msg { return autoFetchTickMsg{} })
}

// gitCheckoutBranch checks out an existing local branch, or creates a local
// branch tracking a remote one. It never creates a branch from scratch: the
// pull request's source branch exists somewhere or the user must fetch.
// gitCheckoutBranch checks out the pull request's source branch in the
// checkout lazybitbucket was started in. It fetches that branch first from the
// remote that points at the pull request's repository, so a branch pushed after
// the last fetch is found and a local branch that fell behind is fast-forwarded.
// It never creates a branch from thin air: a name known neither locally nor on
// that remote is an error.
func (a *App) gitCheckoutBranch(workspace, slug, branch string) tea.Cmd {
	repoPath := a.gitRepoPath
	return func() tea.Msg {
		remote, hasRemote := git.RemoteFor(repoPath, workspace, slug)
		var fetchErr error
		if hasRemote {
			fetchErr = git.Fetch(repoPath, remote, branch)
		}
		fetched := hasRemote && fetchErr == nil
		remoteRef := remote + "/" + branch

		switch {
		case git.BranchExists(repoPath, branch):
			if err := git.Checkout(repoPath, branch); err != nil {
				return gitErrorMsg{err: err}
			}
			return gitCheckoutDoneMsg{name: branch, note: catchUpNote(repoPath, branch, remote, fetched)}
		case fetched && git.IsRemoteBranch(repoPath, remoteRef):
			if err := git.CheckoutTracking(repoPath, remoteRef); err != nil {
				return gitErrorMsg{err: err}
			}
			return gitCheckoutDoneMsg{name: branch, note: ""}
		case git.IsRemoteBranch(repoPath, branch):
			// The name already carries its remote (origin/feature).
			if err := git.CheckoutTracking(repoPath, branch); err != nil {
				return gitErrorMsg{err: err}
			}
			local := branch
			if _, after, ok := strings.Cut(branch, "/"); ok {
				local = after
			}
			return gitCheckoutDoneMsg{name: local, note: ""}
		default:
			return gitErrorMsg{err: checkoutNotFound(workspace, slug, branch, remote, hasRemote, fetchErr)}
		}
	}
}

// catchUpNote fast-forwards the freshly checked-out local branch onto the
// remote copy that was just fetched, and says what happened when there is
// something to say. A branch that is ahead or already equal needs nothing; one
// that has diverged is left alone, because only the user can decide what to do.
func catchUpNote(repoPath, branch, remote string, fetched bool) string {
	remoteRef := remote + "/" + branch
	if !fetched || !git.IsRemoteBranch(repoPath, remoteRef) {
		return ""
	}
	behind, err := git.IsAncestor(repoPath, "HEAD", remoteRef)
	if err != nil {
		return ""
	}
	ahead, err := git.IsAncestor(repoPath, remoteRef, "HEAD")
	if err != nil {
		return ""
	}
	switch {
	case behind && ahead, ahead:
		return "" // equal, or the checkout is ahead: nothing to bring in
	case behind:
		if err := git.FastForward(repoPath, remoteRef); err != nil {
			return "could not fast-forward to " + remoteRef + ": " + err.Error()
		}
		return "fast-forwarded to " + remoteRef
	default:
		return branch + " has diverged from " + remoteRef + " — not updated"
	}
}

// checkoutNotFound words the failure of a checkout that found no branch.
func checkoutNotFound(workspace, slug, branch, remote string, hasRemote bool, fetchErr error) error {
	switch {
	case !hasRemote:
		return errors.New("branch " + branch + " not found locally, and no remote of this checkout points at " +
			workspace + "/" + slug)
	case fetchErr != nil && !errors.Is(fetchErr, git.ErrNoRemoteBranch):
		return fmt.Errorf("fetch %s %s: %w", remote, branch, fetchErr)
	default:
		return errors.New("branch " + branch + " not found locally or on " + remote)
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

// scopeRepos is the set of repositories a refresh asks for: the one repository
// the list is filtered to, or every repository when there is no filter. The
// others are fetched when the user first widens the view (see
// ensureScopeLoaded), not at startup.
func (a *App) scopeRepos() []string {
	if a.repoFilter != "" && slices.Contains(a.repoSlugs, a.repoFilter) {
		return []string{a.repoFilter}
	}
	return slices.Clone(a.repoSlugs)
}

// fetchPullRequests lists the pull requests behind one fetch key for the
// repositories in scope, loaded or not: it is the refresh. A second request
// for a key already in flight is dropped.
func (a *App) fetchPullRequests(fetchKey string) tea.Cmd {
	return a.requestPullRequests(fetchKey, a.scopeRepos())
}

// requestPullRequests is the one place a list request starts. A request for a
// key already in flight is dropped; the answer of the one in flight asks for
// whatever the view still lacks (see handlePullRequestsLoaded).
func (a *App) requestPullRequests(fetchKey string, repos []string) tea.Cmd {
	if a.workspace == "" || len(repos) == 0 || a.inFlight[fetchKey] {
		return nil
	}
	if a.inFlight == nil {
		a.inFlight = make(map[string]bool)
	}
	a.inFlight[fetchKey] = true

	ctx, client := a.context(), a.client
	workspace := a.workspace
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

// fetchPipeline reads a run and its steps in one command. It takes the
// repository and the cache key rather than a pull request, because the
// pipelines view drills into runs that have none.
func (a *App) fetchPipeline(workspace, slug, key string, build int) tea.Cmd {
	ctx, client := a.context(), a.client
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

// fetchStepLog reads one step's log tail. Like fetchPipeline it takes the
// repository, so both views can ask for a log.
func (a *App) fetchStepLog(workspace, slug, pipelineUUID, stepUUID string) tea.Cmd {
	ctx, client := a.context(), a.client
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
	// The renderer is resolved here, on the event loop, so the closure reads no
	// app state: whether it runs at all depends on a flag a previous failure
	// may have set.
	renderer, rendererArgs, useRenderer := a.diffRendererCommand()
	a.logFlag.Store(true)
	return func() tea.Msg {
		msg := fileDiffLoadedMsg{
			key: key, path: path, file: nil, hash: hash, err: nil, rendered: nil, rendererErr: nil,
		}
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
		if useRenderer {
			msg.rendered, msg.rendererErr = renderDiffLines(ctx, renderer, rendererArgs, text)
		}
		return msg
	}
}

// diffRendererCommand is the external diff renderer the next file diff should
// go through, and whether there is one: it has to be configured, the terminal
// has to be able to show colour at all (rendererStyle notty says it cannot, and
// a terminal that cannot render chroma's colours cannot render delta's either),
// and a failure that condemns the program must not already have turned it off
// for this session.
func (a *App) diffRendererCommand() (name string, args []string, ok bool) {
	if a.diffRendererDisabled || a.cfg == nil || a.cfg.RendererStyle == config.RendererStyleNoTTY {
		return "", nil, false
	}
	return a.cfg.ResolveDiffRenderer()
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
