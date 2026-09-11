package tui

import (
	"slices"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// pipelinesPerRepo is how many runs each repository contributes to the list.
// The endpoint pages at 10, so this is two requests per repository and the
// reason the list is never on the background tick.
const pipelinesPerRepo = 20

// notAPullRequestBuild is the refusal for a run that was not started for a pull
// request — a branch build, a schedule or a manual run. There is no pull
// request to go to.
const notAPullRequestBuild = "this run is not a pull request build"

// pipelinesLoadedMsg carries one cross-repository pipeline fetch. failed and
// total are what the note beside the list is built from: a repository with
// Pipelines switched off fails every time, and blanking the whole list over it
// would hide the repositories that did answer.
type pipelinesLoadedMsg struct {
	entries []bitbucket.PipelineEntry
	failed  int
	total   int
	// err is set only when every repository failed, which is the one case the
	// panel has nothing to draw and shows the failure instead.
	err   error
	epoch int
}

// pipelineStepsLoadedMsg carries the steps of the run the pipelines view shows.
// The run itself needs no request: the list payload is the whole run.
type pipelineStepsLoadedMsg struct {
	runKey string
	steps  []bitbucket.PipelineStep
	err    error
	epoch  int
}

// pipelineRunDebounceMsg is the settle tick of the pipelines list cursor, so
// holding j down does not cost one step request per row.
type pipelineRunDebounceMsg struct {
	runKey string
	epoch  int
}

// fetchPipelines lists the recent runs of every repository in scope — the
// repository filter when one is set, every configured repository otherwise.
// The epoch is bumped first and carried, so a refetch started while an older
// one is in flight is the one that wins.
func (a *App) fetchPipelines() tea.Cmd {
	if a.workspace == "" || len(a.repoSlugs) == 0 {
		return nil
	}
	repos := slices.Clone(a.repoSlugs)
	if a.repoFilter != "" {
		repos = []string{a.repoFilter}
	}

	a.pipelinesEpoch++
	epoch := a.pipelinesEpoch
	ctx, client, workspace := a.context(), a.client, a.workspace
	opts := bitbucket.ListPipelinesOptions{TargetBranch: "", Sort: "", MaxItems: pipelinesPerRepo}

	a.pipelineList.SetLoading()
	a.logFlag.Store(true)

	return func() tea.Msg {
		results := bitbucket.FetchPipelines(ctx, client, workspace, repos, opts, 0)
		msg := pipelinesLoadedMsg{
			entries: bitbucket.MergePipelines(results),
			failed:  0, total: len(results), err: nil, epoch: epoch,
		}
		var firstErr error
		for _, result := range results {
			if result.Err != nil {
				msg.failed++
				if firstErr == nil {
					firstErr = result.Err
				}
			}
		}
		if msg.failed == msg.total && msg.total > 0 {
			msg.err = firstErr
		}
		return msg
	}
}

// handlePipelinesLoaded fills the list. A response from a superseded fetch is
// dropped; a partial failure is reported beside the runs that did come back,
// because a repository without Pipelines enabled must not blank the list.
func (a *App) handlePipelinesLoaded(msg pipelinesLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.epoch != a.pipelinesEpoch {
		return a, nil
	}
	a.logFlag.Store(false)
	a.pipelinesLoaded = true

	if msg.err != nil {
		a.statusPanel.SetOnline(false)
		a.statusPanel.SetError(msg.err.Error())
		a.pipelineList.SetError(msg.err.Error())
		return a, nil
	}
	a.statusPanel.SetOnline(true)
	a.pipelineEntries = msg.entries
	a.pipelineList.SetEntries(msg.entries)
	if msg.failed > 0 {
		a.helpBar.SetStatusMsg(strconv.Itoa(msg.failed) + " of " + strconv.Itoa(msg.total) +
			" repositories could not be listed")
	}
	if a.view != viewPipelines {
		return a, nil
	}
	// The cursor follows the run the view is pointed at, so a refetch does not
	// move the reader. A run that is not in the list at all — the run a rerun
	// has just created may not be listed yet — keeps the detail panel where it
	// is rather than yanking it onto whatever row the cursor kept.
	if ref, ok := parsePipelineRef(a.runKey); ok && !a.pipelineList.SelectByRun(ref.slug, ref.build) {
		return a, nil
	}
	return a, a.showSelectedRun()
}

// runKeyOf addresses a list row the way the caches and the detail panel's owner
// token do.
func (a *App) runKeyOf(entry *bitbucket.PipelineEntry) string {
	return pipelineRef{workspace: a.workspace, slug: entry.Repo, build: entry.Pipeline.BuildNumber}.key()
}

// showSelectedRun points the detail panel at the run under the pipelines
// cursor. The run needs no request — the list payload is the whole run — so it
// is on screen at once and only the steps are fetched, after a debounce.
//
// What it must not do is disturb a reader who is already on the run it is being
// asked to show, which is what separates a cursor move from a refetch landing
// underneath one: that is what settled decides.
func (a *App) showSelectedRun() tea.Cmd {
	entry := a.pipelineList.SelectedEntry()
	if entry == nil {
		// There is nothing to point at — the list is still loading, empty, or
		// filtered down to nothing. The panel is cleared rather than left as
		// it was, because on the first entry into the view what it is still
		// holding is the other view's pull request.
		a.runKey = ""
		a.showSplash()
		return nil
	}

	key := a.runKeyOf(entry)
	settled := a.runKey == key && a.detailView.PipelineOwner() == key
	a.runKey = key

	// A reader inside this run's own step log stays there: the log is a
	// drill-down, and a refetch behind it must not close it. A cursor move onto
	// another run is not that, and does re-point the panel.
	if settled && a.detailView.Mode() == views.ModeStepLog {
		return nil
	}

	run := entry.Pipeline
	if cached, ok := a.pipelineCache[key]; ok && cached != nil && cached.steps != nil {
		a.detailView.SetRun(key, &run, cached.steps)
		return nil
	}
	if settled {
		// The panel is already on this run with its steps on the way. Pointing
		// it at the run again would replace them with the loading line and buy
		// a second request for the steps already being fetched.
		return nil
	}
	a.detailView.SetRun(key, &run, nil)

	epoch := a.pipelinesEpoch
	return tea.Tick(a.previewDelay(), func(time.Time) tea.Msg {
		return pipelineRunDebounceMsg{runKey: key, epoch: epoch}
	})
}

// handlePipelineSelected follows the list cursor, which is what makes the
// detail panel show whichever run the reader walks onto.
func (a *App) handlePipelineSelected(msg views.PipelineSelectedMsg) (tea.Model, tea.Cmd) {
	if msg.Entry == nil {
		return a, nil
	}
	return a, a.showSelectedRun()
}

// handlePipelineRunDebounce fetches the steps a selection deferred, unless the
// user has moved on to another run or a refetch superseded them.
func (a *App) handlePipelineRunDebounce(msg pipelineRunDebounceMsg) (tea.Model, tea.Cmd) {
	if msg.epoch != a.pipelinesEpoch || msg.runKey != a.runKey {
		return a, nil
	}
	ref, ok := parsePipelineRef(msg.runKey)
	if !ok {
		return a, nil
	}
	return a, a.fetchRunSteps(ref)
}

// fetchRunSteps reads one standalone run's steps.
func (a *App) fetchRunSteps(ref pipelineRef) tea.Cmd {
	ctx, client := a.context(), a.client
	key, epoch := ref.key(), a.pipelinesEpoch
	number := strconv.Itoa(ref.build)
	a.logFlag.Store(true)
	return func() tea.Msg {
		steps, err := client.ListPipelineSteps(ctx, ref.workspace, ref.slug, number)
		return pipelineStepsLoadedMsg{runKey: key, steps: steps, err: err, epoch: epoch}
	}
}

// handlePipelineStepsLoaded fills in the steps of the run on screen and caches
// them, so walking back onto the run costs nothing. A response for another run
// or from a superseded fetch is dropped.
func (a *App) handlePipelineStepsLoaded(msg pipelineStepsLoadedMsg) (tea.Model, tea.Cmd) {
	a.logFlag.Store(false)
	if msg.epoch != a.pipelinesEpoch || msg.runKey != a.runKey {
		return a, nil
	}
	if msg.err != nil {
		a.detailView.SetRunStepsError(msg.runKey, msg.err.Error())
		a.statusPanel.SetError(msg.err.Error())
		return a, nil
	}
	a.statusPanel.SetOnline(true)
	if run := a.runForKey(msg.runKey); run != nil {
		a.pipelineCache[msg.runKey] = &pipelineEntry{pipeline: run, steps: msg.steps}
	}
	a.detailView.SetRunSteps(msg.runKey, msg.steps)
	return a, nil
}

// runForKey is the run a key names: the list row while the list holds it, else
// the run the detail panel is showing. The second source is what caches the
// steps of a run the list has not caught up with — the one a rerun has just
// created, whose steps would otherwise be fetched twice.
func (a *App) runForKey(key string) *bitbucket.Pipeline {
	if entry := a.pipelineEntryByKey(key); entry != nil {
		run := entry.Pipeline
		return &run
	}
	if a.detailView.PipelineOwner() != key {
		return nil
	}
	shown := a.detailView.Pipeline()
	if shown == nil {
		return nil
	}
	run := *shown
	return &run
}

// pipelineEntryByKey finds a fetched run by its key, nil when the list no
// longer holds it.
func (a *App) pipelineEntryByKey(key string) *bitbucket.PipelineEntry {
	for index := range a.pipelineEntries {
		if a.runKeyOf(&a.pipelineEntries[index]) == key {
			return &a.pipelineEntries[index]
		}
	}
	return nil
}

// refreshSelectedRun is what r does in the pipelines view: it drops the cached
// steps of the run on screen and reads them again. The run itself comes from
// the list, which R refetches.
func (a *App) refreshSelectedRun() tea.Cmd {
	ref, ok := parsePipelineRef(a.runKey)
	if !ok {
		return nil
	}
	delete(a.pipelineCache, a.runKey)
	return a.fetchRunSteps(ref)
}

// handleActionOpenPullRequest is L: it leaves the pipelines view for the pull
// request the selected run ran for. The pull request has to be on the tab in
// view, because that is where the cursor is being moved to; a run that is not a
// pull request build has nowhere to go at all.
func (a *App) handleActionOpenPullRequest() tea.Cmd {
	entry := a.pipelineList.SelectedEntry()
	if entry == nil {
		return nil
	}
	target := entry.Pipeline.Target.PullRequest
	if target == nil {
		a.helpBar.SetStatusMsg(notAPullRequestBuild)
		return nil
	}
	key := a.workspace + "/" + entry.Repo + "#" + strconv.Itoa(target.ID)
	if !a.prList.SelectByKey(key) {
		a.helpBar.SetStatusMsg("pull request #" + strconv.Itoa(target.ID) + " is not in this tab")
		return nil
	}

	a.view = viewPullRequests
	a.side = sideLeft
	a.leftFocus = focusPullRequests
	a.applyView()
	a.updateFocusState()
	return a.previewSelected()
}
