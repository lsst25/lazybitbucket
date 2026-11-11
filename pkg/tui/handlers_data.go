package tui

import (
	"slices"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// previewDebounce is how long the selection waits before it costs a request,
// so holding j down does not walk the whole list over the network.
const previewDebounce = 150 * time.Millisecond

// prefetchLimit is how many pull requests one tab load warms. The budget is
// roughly 1000 requests an hour, so the list is warmed from the top down.
const prefetchLimit = 15

// filterMine and filterReviewing are the client-side tab filters.
const (
	filterMine      = "mine"
	filterReviewing = "reviewing"
)

// noCurrentUserWarning is shown when the token cannot name its own account, so
// the Mine and Reviewing tabs cannot be evaluated.
const noCurrentUserWarning = "current user unknown — Mine/Reviewing tabs are empty"

// handleCurrentUserLoaded stores the account and re-evaluates every tab filter,
// because the pull request list may well have arrived first.
func (a *App) handleCurrentUserLoaded(msg currentUserLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		// Repository, project and workspace access tokens are not users and
		// cannot read /user. That is a limitation, not a failure.
		if bitbucket.IsUnauthorized(msg.err) || bitbucket.IsForbidden(msg.err) {
			a.setWarning(noCurrentUserWarning)
		} else {
			a.statusPanel.SetError(msg.err.Error())
		}
		return a, nil
	}
	a.currentUser = msg.user
	if msg.user != nil {
		a.splashInfo.User = msg.user.Name()
		if a.detailView.Mode() == views.ModeSplash {
			a.showSplash()
		}
	}
	a.refilterTabs("")
	// The Mine and Reviewing tabs were empty until this answer, so this may be
	// the first moment the branch's pull request exists to be found.
	if cmd := a.autoSelectGitBranch(); cmd != nil {
		return a, cmd
	}
	return a, a.previewSelected()
}

// handleReposLoaded records the repositories the list is drawn from and kicks
// off the first pull request fetch.
func (a *App) handleReposLoaded(msg reposLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		a.repoDiscoveryFailed = true
		a.statusPanel.SetOnline(false)
		a.statusPanel.SetError(msg.err.Error())
		return a, nil
	}

	a.repoDiscoveryFailed = false
	a.repos = msg.repos
	a.repoSlugs = make([]string, 0, len(msg.repos))
	for _, repo := range msg.repos {
		a.repoSlugs = append(a.repoSlugs, repo.Slug)
	}
	a.repoList.SetRepos(msg.repos)
	a.splashInfo.RepoCount = len(msg.repos)
	a.splashInfo.RepoNote = ""
	a.scopeToCheckout(msg.repos)

	if len(a.cfg.Bitbucket.Repositories) == 0 {
		switch {
		case len(msg.repos) == 0:
			note := "no repositories visible to this token — set bitbucket.repositories in config.yml"
			a.statusPanel.SetError(note)
			a.splashInfo.RepoNote = note
		case len(msg.repos) == repoDiscoveryLimit:
			a.setWarning("showing the first 25 repositories — set bitbucket.repositories to choose")
		}
	}
	if warning := refreshRateWarning(len(a.repoSlugs), a.refreshEvery); warning != "" {
		a.setWarning(warning)
	}
	if a.detailView.Mode() == views.ModeSplash {
		a.showSplash()
	}

	cmds := []tea.Cmd{a.fetchActiveTab()}
	// Starting in the pipelines view still loads the pull requests behind it —
	// the other view is one key away — but the list the user is looking at has
	// to be fetched too, and this is the first moment the repositories are
	// known.
	if a.view == viewPipelines && !a.pipelinesLoaded {
		cmds = append(cmds, a.fetchPipelines())
	}
	return a, tea.Batch(cmds...)
}

// handlePullRequestsLoaded stores one fetch key's result and re-filters every
// tab that shares that key — the default Open/Mine/Reviewing tabs cost one
// request per repository between them, not three.
func (a *App) handlePullRequestsLoaded(msg pullRequestsLoadedMsg) (tea.Model, tea.Cmd) {
	delete(a.inFlight, msg.fetchKey)
	a.logFlag.Store(false)

	failed, firstErr := 0, ""
	for _, result := range msg.results {
		if result.Err != nil {
			failed++
			if firstErr == "" {
				firstErr = result.Err.Error()
			}
		}
	}
	switch {
	case len(msg.results) > 0 && failed == len(msg.results):
		a.statusPanel.SetOnline(false)
		a.statusPanel.SetError(firstErr)
	case failed > 0:
		a.statusPanel.SetOnline(true)
		a.statusPanel.SetError(firstErr)
	default:
		a.statusPanel.SetOnline(true)
		a.statusPanel.SetError("")
	}

	if a.merged == nil {
		a.merged = make(map[string][]bitbucket.PullRequest)
	}
	a.merged[msg.fetchKey] = a.mergeFetched(msg.fetchKey, msg.results)
	a.refilterTabs(msg.fetchKey)

	if a.fetchKeyOf(a.prList.GetTabIndex()) != msg.fetchKey {
		return a, nil
	}

	a.updateRepoCounts(msg.fetchKey)

	var cmds []tea.Cmd
	// The user may have widened the view while this fetch was in flight; that
	// request was dropped, so ask for what the view needs and this did not cover.
	attempted := make(map[string]bool, len(msg.results))
	for _, result := range msg.results {
		attempted[result.Repo] = true
	}
	if cmd := a.fetchUnloadedPullRequests(msg.fetchKey, attempted); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if cmd := a.autoSelectGitBranch(); cmd != nil {
		cmds = append(cmds, cmd)
	} else if cmd := a.previewSelected(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if cmd := a.startPrefetch(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	return a, tea.Batch(cmds...)
}

// mergeFetched folds one fetch into what is already held for its key. The
// repositories the fetch covered are replaced by its answer, so a closed pull
// request leaves the list; the ones it did not cover keep their rows, because a
// scoped fetch asks for one repository and must not empty the others.
func (a *App) mergeFetched(fetchKey string, results []bitbucket.RepoPullRequests) []bitbucket.PullRequest {
	if a.loadedRepos == nil {
		a.loadedRepos = make(map[string]map[string]bool)
	}
	if a.loadedRepos[fetchKey] == nil {
		a.loadedRepos[fetchKey] = make(map[string]bool)
	}

	covered := make(map[string]bool, len(results))
	for _, result := range results {
		covered[result.Repo] = true
		a.loadedRepos[fetchKey][result.Repo] = result.Err == nil
	}

	var order []string
	kept := make(map[string][]bitbucket.PullRequest)
	for _, pr := range a.merged[fetchKey] {
		slug := pr.RepoSlug()
		if covered[slug] {
			continue
		}
		if _, seen := kept[slug]; !seen {
			order = append(order, slug)
		}
		kept[slug] = append(kept[slug], pr)
	}

	all := slices.Clone(results)
	for _, slug := range order {
		all = append(all, bitbucket.RepoPullRequests{Repo: slug, PullRequests: kept[slug], Err: nil})
	}
	return bitbucket.MergePullRequests(all)
}

// updateRepoCounts shows the open count of every repository this key has
// loaded. A repository that is not loaded stays unknown, and so does the All
// total while any repository is missing, because a partial sum would read as
// the whole.
func (a *App) updateRepoCounts(fetchKey string) {
	counts := countsBySlug(a.merged[fetchKey])
	complete := true
	for _, slug := range a.repoSlugs {
		if !a.loadedRepos[fetchKey][slug] {
			complete = false
			continue
		}
		if _, ok := counts[slug]; !ok {
			counts[slug] = 0
		}
	}
	a.repoList.SetCounts(counts)
	if !complete {
		a.repoList.MarkTotalUnknown()
	}
}

// autoSelectGitBranch moves the cursor to the pull request whose source branch
// is the branch checked out here.
//
// It is attempted on every load until it lands, rather than once against
// whatever had arrived first. The tabs fill in at different moments — Mine and
// Reviewing are empty until GET /user answers, which can be after the first
// list — so a single attempt would give up on a pull request that was simply
// not there yet, and never look again. Once it has landed, or once the user has
// taken the cursor themselves, it is settled and never moves anything again.
func (a *App) autoSelectGitBranch() tea.Cmd {
	if a.gitAutoSelect || a.gitBranch == "" {
		return nil
	}
	tab, pr, found := a.pullRequestForBranch()
	if !found {
		return nil
	}

	previous := a.prList.GetTabIndex()
	if tab != previous {
		a.prList.SetTabIndex(tab)
	}
	if !a.prList.SelectByKey(pr.Key()) {
		// The row is in the tab but a filter is hiding it. Leave everything as
		// it was and try again on the next load, when the filter may be gone.
		if tab != previous {
			a.prList.SetTabIndex(previous)
		}
		return nil
	}

	a.gitAutoSelect = true
	where := ""
	if tab != previous {
		where = " in " + a.tabName(tab)
	}
	a.helpBar.SetStatusMsg("selected #" + strconv.Itoa(pr.ID) + where +
		" — the pull request for " + a.gitBranch)
	return a.previewSelected()
}

// pullRequestForBranch is the loaded pull request whose source branch is the
// checked-out one, and the tab holding it. The active tab is searched first, so
// a pull request the user can already see is never passed over for the same
// branch somewhere else; the rest follow in their configured order, and a tab
// that has not been loaded holds nothing to find.
func (a *App) pullRequestForBranch() (int, *bitbucket.PullRequest, bool) {
	active := a.prList.GetTabIndex()
	order := make([]int, 0, len(a.tabConfigs()))
	order = append(order, active)
	for tab := range a.tabConfigs() {
		if tab != active {
			order = append(order, tab)
		}
	}

	for _, tab := range order {
		rows := a.prList.TabPullRequests(tab)
		for index := range rows {
			if a.branchPullRequest(&rows[index]) {
				return tab, &rows[index], true
			}
		}
	}
	return 0, nil, false
}

// branchPullRequest reports whether a pull request is the one the checked-out
// branch belongs to. When the checkout's own repositories are known the match
// is confined to them: one branch name can exist in several repositories, and
// selecting another repository's pull request would be worse than selecting
// none.
func (a *App) branchPullRequest(pr *bitbucket.PullRequest) bool {
	if pr.SourceBranch() != a.gitBranch {
		return false
	}
	if len(a.gitRemoteRepos) == 0 {
		return true
	}
	return slices.Contains(a.gitRemoteRepos, strings.ToLower(pr.Workspace()+"/"+pr.RepoSlug()))
}

// userTookTheCursor settles the branch auto-selection without it having landed:
// the user has moved the list cursor or changed the tab, and from that moment
// the cursor is theirs. It is deliberately not every keystroke — a `j` pressed
// against a list that has not arrived yet moves nothing, and giving up there is
// how the selection would miss the moment it was waiting for.
func (a *App) userTookTheCursor() {
	a.gitAutoSelect = true
}

// tabName is a tab's configured label, used to say where the cursor went.
func (a *App) tabName(tab int) string {
	tabs := a.tabConfigs()
	if tab < 0 || tab >= len(tabs) {
		return ""
	}
	return tabs[tab].Name
}

// handlePullRequestDetailLoaded applies a pull request fetched for its review
// data. A response from a superseded selection is dropped.
func (a *App) handlePullRequestDetailLoaded(msg pullRequestDetailLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.epoch != a.previewEpoch || msg.pr == nil {
		return a, nil
	}
	a.logFlag.Store(false)
	a.statusPanel.SetOnline(true)
	a.prCache[msg.pr.Key()] = msg.pr
	a.detailView.UpdatePullRequestData(msg.pr)
	a.patchMerged(msg.pr)
	a.prList.PatchPullRequest(msg.pr)
	a.refilterTabs("")
	return a, nil
}

// patchMerged replaces a pull request wherever a fetch result holds it, so the
// tab filters re-evaluate against the review data that just arrived rather
// than against the list row it was missing from.
func (a *App) patchMerged(pr *bitbucket.PullRequest) {
	key := pr.Key()
	for _, prs := range a.merged {
		for index := range prs {
			if prs[index].Key() == key {
				prs[index] = *pr
			}
		}
	}
}

// handleStatusesLoaded applies the previewed pull request's build statuses.
func (a *App) handleStatusesLoaded(msg statusesLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.epoch != a.previewEpoch || msg.key != a.previewKey {
		return a, nil
	}
	a.logFlag.Store(false)
	if msg.err != nil {
		a.buildsPanel.SetError(msg.key, msg.err.Error())
		a.statusPanel.SetError(msg.err.Error())
		return a, nil
	}
	a.statusPanel.SetOnline(true)
	a.cacheStatuses(msg.key, msg.statuses, msg.updatedOn)
	a.buildsPanel.SetStatuses(msg.key, msg.statuses)
	a.detailView.SetStatuses(msg.key, msg.statuses)
	return a, nil
}

// handlePreviewDebounce fires the fetches a selection deferred, unless the user
// has moved on.
func (a *App) handlePreviewDebounce(msg previewDebounceMsg) (tea.Model, tea.Cmd) {
	if msg.epoch != a.previewEpoch || msg.key != a.previewKey {
		return a, nil
	}
	pr := a.pullRequestByKey(msg.key)
	if pr == nil {
		return a, nil
	}
	var cmds []tea.Cmd
	if _, ok := a.statusCache[msg.key]; !ok {
		cmds = append(cmds, a.fetchStatuses(pr, a.previewEpoch))
	}
	if !pr.HasReviewData() {
		cmds = append(cmds, a.fetchPullRequestDetail(pr, a.previewEpoch))
	}
	if len(cmds) == 0 {
		return a, nil
	}
	a.logFlag.Store(true)
	return a, tea.Batch(cmds...)
}

// handlePrefetched fills the caches from a warmed pull request and chains the
// next one, so exactly one prefetch request is ever in flight. Failures are
// silent: a warm-up must never interrupt the user.
func (a *App) handlePrefetched(msg prefetchedMsg) (tea.Model, tea.Cmd) {
	if msg.epoch != a.prefetchEpoch {
		return a, nil
	}
	if msg.err == nil {
		a.cacheStatuses(msg.key, msg.statuses, msg.updatedOn)
		a.buildsPanel.SetStatuses(msg.key, msg.statuses)
		a.detailView.SetStatuses(msg.key, msg.statuses)
	}
	if msg.pr != nil {
		a.prCache[msg.key] = msg.pr
		a.patchMerged(msg.pr)
		a.prList.PatchPullRequest(msg.pr)
		a.refilterTabs("")
	}
	return a, a.prefetchNext()
}

// handlePipelineLoaded caches a pipeline run and its steps, and shows them when
// the detail panel is still on that run. A response that lands after the user
// backed out of the drill-down, or opened another run, is kept for their next
// visit but must not pull the panel back into a view they left.
func (a *App) handlePipelineLoaded(msg pipelineLoadedMsg) (tea.Model, tea.Cmd) {
	a.logFlag.Store(false)
	showing := a.detailView.Mode() == views.ModePipeline && a.detailView.PipelineBuild() == msg.build
	if msg.err != nil {
		if showing {
			a.detailView.SetPipelineError(msg.key, msg.err.Error())
		}
		a.statusPanel.SetError(msg.err.Error())
		return a, nil
	}
	a.statusPanel.SetOnline(true)
	a.pipelineCache[pipelineCacheKey(msg.key, msg.build)] = &pipelineEntry{pipeline: msg.pipeline, steps: msg.steps}
	if showing {
		a.detailView.SetPipeline(msg.key, msg.pipeline, msg.steps)
	}
	return a, nil
}

// handleStepSelected fetches the log of the step the user activated. The
// repository comes from the run in the pipelines view and from the pull request
// in the other one: a standalone run has no pull request to read it off.
func (a *App) handleStepSelected(msg views.StepSelectedMsg) (tea.Model, tea.Cmd) {
	if msg.StepUUID == "" {
		return a, nil
	}
	ref, ok := a.stepLogRepo()
	if !ok {
		return a, nil
	}
	a.detailView.SetStepLogLoading(msg.PipelineUUID, msg.StepUUID)
	return a, a.fetchStepLog(ref.workspace, ref.slug, msg.PipelineUUID, msg.StepUUID)
}

// stepLogRepo is the repository whose step log is being opened.
func (a *App) stepLogRepo() (pipelineRef, bool) {
	if a.view == viewPipelines {
		return parsePipelineRef(a.runKey)
	}
	pr := a.currentPullRequest()
	if pr == nil {
		return pipelineRef{}, false
	}
	return pipelineRef{workspace: pr.Workspace(), slug: pr.RepoSlug(), build: 0}, true
}

// handleStepLogLoaded shows a step's log tail.
func (a *App) handleStepLogLoaded(msg stepLogLoadedMsg) (tea.Model, tea.Cmd) {
	a.logFlag.Store(false)
	if msg.err != nil {
		a.detailView.SetStepLogError(msg.pipelineUUID, msg.stepUUID, msg.err.Error())
		a.statusPanel.SetError(msg.err.Error())
		return a, nil
	}
	a.statusPanel.SetOnline(true)
	a.detailView.SetStepLog(msg.pipelineUUID, msg.stepUUID, msg.text)
	return a, nil
}

// handlePullRequestSelected drives the whole right-hand side from the list
// cursor: detail, builds panel, and the debounced fetches behind them.
func (a *App) handlePullRequestSelected(msg views.PullRequestSelectedMsg) (tea.Model, tea.Cmd) {
	if msg.PullRequest == nil {
		return a, nil
	}
	key := msg.PullRequest.Key()
	pr := msg.PullRequest
	if cached, ok := a.prCache[key]; ok && cached != nil {
		pr = cached
	}

	a.previewKey = key
	a.previewEpoch++
	// A push moves the source commit, which makes every cached diff, diffstat
	// and commit list for this pull request wrong.
	a.dropStaleReviewCaches(pr)
	// Only re-point the panel when it is not already on this pull request:
	// a background refresh must not throw the user out of a pipeline or a
	// step log they are reading. PullRequestKey is "" on the splash and repo
	// screens, so those still fall through to SetPullRequest. In the pipelines
	// view the panel belongs to a run and is not this view's to re-point at
	// all — the pull request list is still refreshed behind it.
	switch {
	case a.view != viewPullRequests:
	case a.detailView.PullRequestKey() != key:
		a.detailView.SetPullRequest(pr)
	default:
		a.detailView.UpdatePullRequestData(pr)
	}
	a.buildsPanel.SetPullRequest(key)

	entry, cached := a.statusCache[key]
	if cached {
		a.buildsPanel.SetStatuses(key, entry.statuses)
		a.detailView.SetStatuses(key, entry.statuses)
	}
	if cached && pr.HasReviewData() {
		return a, nil
	}
	if !cached {
		a.buildsPanel.SetLoading(key)
	}

	epoch := a.previewEpoch
	return a, tea.Tick(a.previewDelay(), func(time.Time) tea.Msg {
		return previewDebounceMsg{key: key, epoch: epoch}
	})
}

// previewDelay is the debounce window, shortened by tests that drive the whole
// message chain synchronously.
func (a *App) previewDelay() time.Duration {
	if a.debounce > 0 {
		return a.debounce
	}
	return previewDebounce
}

// previewSelected re-runs the selection flow for whatever the cursor is on.
func (a *App) previewSelected() tea.Cmd {
	sel := a.prList.SelectedPullRequest()
	if sel == nil {
		return nil
	}
	_, cmd := a.handlePullRequestSelected(views.PullRequestSelectedMsg{PullRequest: sel})
	return cmd
}

// scopeToCheckout narrows the list to the repository lazybitbucket was started
// in, when that checkout's remotes name one of the repositories on the list. It
// happens once, only while no filter is set, and is undone the way any filter
// is — by picking "All repositories" — so a user who widens the view is not
// narrowed again on the next refresh.
func (a *App) scopeToCheckout(repos []bitbucket.Repository) {
	if a.autoScoped || a.repoFilter != "" || len(a.gitRemoteRepos) == 0 {
		return
	}
	for _, repo := range repos {
		if !slices.Contains(a.gitRemoteRepos, strings.ToLower(repo.FullName)) {
			continue
		}
		a.autoScoped = true
		a.repoFilter = repo.Slug
		a.prList.SetRepoFilter(repo.Slug)
		a.repoList.SetActiveSlug(repo.Slug)
		a.statusPanel.SetScope(scopeLabel(repo.Slug))
		a.helpBar.SetStatusMsg("showing " + repo.Slug + " — the repository of this checkout; pick All repositories to widen")
		return
	}
}

// selectRepo narrows the list to one repository (or clears the filter for the
// synthetic All row) and moves focus to the pull requests.
func (a *App) selectRepo(item *views.RepoItem) tea.Cmd {
	if item == nil {
		return nil
	}
	a.repoFilter = item.Slug
	a.prList.SetRepoFilter(item.Slug)
	a.repoList.SetActiveSlug(item.Slug)
	a.statusPanel.SetScope(scopeLabel(item.Slug))
	a.side = sideLeft
	a.leftFocus = a.mainPanel()
	a.updateFocusState()
	if a.view == viewPipelines {
		// The list is fetched per repository, so narrowing it is a new fetch
		// rather than a client-side filter.
		return a.fetchPipelines()
	}
	// A repository the view has not shown yet is fetched now; one already
	// loaded is shown as it is and left to the refresh tick.
	return tea.Batch(
		a.previewSelected(),
		a.startPrefetch(),
		a.fetchUnloadedPullRequests(a.fetchKeyOf(a.prList.GetTabIndex()), nil),
	)
}

// openBuild drills into a build status: its pipeline run when the status names
// one, the browser otherwise.
func (a *App) openBuild(status *bitbucket.CommitStatus) (tea.Model, tea.Cmd) {
	if status == nil {
		return a, nil
	}
	pr := a.currentPullRequest()
	build, ok := status.BuildNumber()
	if !ok || pr == nil {
		if status.URL != "" {
			a.openBrowser(status.URL)
		}
		return a, nil
	}

	key := pr.Key()
	// The panel is put into its pipeline mode before the focus moves, so
	// focusDetail's prefetch leaves the review tabs alone.
	a.detailView.SetPipelineLoading(key, build)
	focusCmd := a.focusDetail()

	if entry, hit := a.pipelineCache[pipelineCacheKey(key, build)]; hit && entry != nil {
		a.detailView.SetPipeline(key, entry.pipeline, entry.steps)
		return a, focusCmd
	}
	return a, tea.Batch(focusCmd, a.fetchPipeline(pr.Workspace(), pr.RepoSlug(), key, build))
}

// startPrefetch refills the prefetch queue for the tab in view and starts the
// chain. Bumping the epoch drops results from the previous queue.
func (a *App) startPrefetch() tea.Cmd {
	a.prefetchEpoch++
	a.prefetchQueue = nil
	prs := a.prList.CurrentPullRequests()
	for index := range prs {
		if len(a.prefetchQueue) >= prefetchLimit {
			break
		}
		if a.statusesStale(&prs[index]) {
			a.prefetchQueue = append(a.prefetchQueue, prs[index].Key())
		}
	}
	return a.prefetchNext()
}

// statusesStale decides whether a pull request's cached statuses are worth a
// request. Four ways to be stale: never fetched; the pull request has changed
// since; a build was still running (a finishing build does not touch the pull
// request's updated_on); or the build has not been reported at all and a
// refresh period has passed.
func (a *App) statusesStale(pr *bitbucket.PullRequest) bool {
	entry, ok := a.statusCache[pr.Key()]
	switch {
	case !ok:
		return true
	case !entry.updatedOn.Equal(pr.UpdatedOn):
		return true
	case bitbucket.RollupStatuses(entry.statuses) == bitbucket.StatusInProgress:
		return true
	case len(entry.statuses) == 0 && a.refreshEvery > 0 && a.nowFn().Sub(entry.fetchedAt) >= a.refreshEvery:
		return true
	default:
		return false
	}
}

func (a *App) cacheStatuses(key string, statuses []bitbucket.CommitStatus, updatedOn time.Time) {
	if a.statusCache == nil {
		a.statusCache = make(map[string]statusEntry)
	}
	a.statusCache[key] = statusEntry{statuses: statuses, updatedOn: updatedOn, fetchedAt: a.nowFn()}
	a.prList.SetBuildState(key, bitbucket.RollupStatuses(statuses))
}

// refilterTabs re-applies the client-side filter of every tab whose data is
// already in hand. An empty fetchKey means every tab. Tabs whose fetch has not
// landed are left untouched so they still count as uncached.
func (a *App) refilterTabs(fetchKey string) {
	current := a.prList.GetTabIndex()
	for tab := range a.tabConfigs() {
		key := a.fetchKeyOf(tab)
		if fetchKey != "" && key != fetchKey {
			continue
		}
		if _, ok := a.merged[key]; !ok {
			continue
		}
		filtered := a.applyTabFilter(tab)
		if tab == current {
			a.prList.SetPullRequests(filtered)
		} else {
			a.prList.SetPullRequestsForTab(tab, filtered)
		}
	}
}

// applyTabFilter narrows a fetch key's merged result to one tab's audience.
// Without a current user the Mine and Reviewing tabs are empty rather than
// wrong.
func (a *App) applyTabFilter(tab int) []bitbucket.PullRequest {
	source := a.merged[a.fetchKeyOf(tab)]
	tabs := a.tabConfigs()
	if tab < 0 || tab >= len(tabs) {
		return source
	}
	filter := strings.ToLower(strings.TrimSpace(tabs[tab].Filter))
	if filter != filterMine && filter != filterReviewing {
		return source
	}

	uuid := ""
	if a.currentUser != nil {
		uuid = a.currentUser.UUID
	}
	if uuid == "" {
		a.setWarning(noCurrentUserWarning)
		return nil
	}

	out := make([]bitbucket.PullRequest, 0, len(source))
	for index := range source {
		pr := &source[index]
		if (filter == filterMine && pr.IsAuthor(uuid)) ||
			(filter == filterReviewing && pr.IsReviewer(uuid)) {
			out = append(out, *pr)
		}
	}
	return out
}

// currentPullRequest is the pull request the right-hand side is showing, which
// is not always the list selection while a preview is in flight.
func (a *App) currentPullRequest() *bitbucket.PullRequest {
	if a.previewKey != "" {
		if pr := a.pullRequestByKey(a.previewKey); pr != nil {
			return pr
		}
	}
	return a.prList.SelectedPullRequest()
}

// pullRequestByKey resolves a key against the detail cache first, then the rows
// on screen, then every fetched list.
func (a *App) pullRequestByKey(key string) *bitbucket.PullRequest {
	if cached, ok := a.prCache[key]; ok && cached != nil {
		return cached
	}
	current := a.prList.CurrentPullRequests()
	for index := range current {
		if current[index].Key() == key {
			return &current[index]
		}
	}
	for _, prs := range a.merged {
		for index := range prs {
			if prs[index].Key() == key {
				return &prs[index]
			}
		}
	}
	return nil
}

// pipelineCacheKey addresses a run by repository and build number, which is
// what the status URL carries.
func pipelineCacheKey(prKey string, build int) string {
	repo, _, _ := strings.Cut(prKey, "#")
	return repo + "#" + strconv.Itoa(build)
}

// countsBySlug counts pull requests per repository for the Repos panel.
func countsBySlug(prs []bitbucket.PullRequest) map[string]int {
	counts := make(map[string]int)
	for index := range prs {
		counts[prs[index].RepoSlug()]++
	}
	return counts
}
