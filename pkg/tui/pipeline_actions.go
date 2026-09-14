package tui

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// The two pipeline write actions. They are the only writes in the app that do
// not act on a pull request: a rerun and a stop act on the run behind a build,
// which is why they live here rather than in actions.go and why they keep their
// own in-flight map. Like the six pull request writes, each opens a dialog and
// only what the user confirms in it ever reaches Bitbucket.
//
// The verbs read as sentences because they are shown as ones: "read-only mode:
// cannot rerun pipeline", "stop pipeline already in progress", "rerun pipeline
// failed: …". The bare "rerun" would name nothing in any of the three.
const (
	verbRerunPipeline = "rerun pipeline"
	verbStopPipeline  = "stop pipeline"
)

// The form ids of the two dialogs, and the confirm labels under them. The
// labels are the verbs without the noun: the title has already said which run
// this is about.
const (
	formRerun    = "rerun"
	formStop     = "stop"
	confirmRerun = "rerun"
	confirmStop  = "stop"
)

// notAPipelineBuild is the refusal for a build row that names no Bitbucket run.
// Third-party reporters — a Jenkins or a GitHub Actions build — post statuses
// with their own URLs, and there is no run behind those to rerun or to stop.
const notAPipelineBuild = "this build is not a Bitbucket pipeline"

// -----------------------------------------------------------------------------
// Guards and bookkeeping
// -----------------------------------------------------------------------------

// pipelineRef is the run a pipeline key acts on: where it lives and which run
// it is. It is what lets P and X work in the pipelines view, where there is no
// pull request to read a repository off — the two keys are about a run, and a
// pull request was only ever the path the pull request view reached one by.
type pipelineRef struct {
	workspace string
	slug      string
	build     int
}

// key is the run's address in the caches and in the in-flight map, in the
// pipelineCacheKey shape.
func (r pipelineRef) key() string {
	return r.workspace + "/" + r.slug + "#" + strconv.Itoa(r.build)
}

// parsePipelineRef reads a run key back into its parts, reporting false for
// anything that is not one. It is how a key that travelled through a message
// or through App.runKey becomes a request again.
func parsePipelineRef(key string) (pipelineRef, bool) {
	repo, number, found := strings.Cut(key, "#")
	if !found {
		return pipelineRef{}, false
	}
	workspace, slug, found := strings.Cut(repo, "/")
	if !found || workspace == "" || slug == "" {
		return pipelineRef{}, false
	}
	build, err := strconv.Atoi(number)
	if err != nil || build <= 0 {
		return pipelineRef{}, false
	}
	return pipelineRef{workspace: workspace, slug: slug, build: build}, true
}

// pipelineActionTarget is the run a pipeline key should act on, or false with
// the reason already on the help bar. It is writeTarget's counterpart, and
// refuses in the same order — with one refusal missing on purpose: the pull
// request's state is not checked, because the pipeline of a merged or declined
// pull request is still a run that can be rerun.
func (a *App) pipelineActionTarget(verb string) (pipelineRef, bool) {
	ref, ok := a.pipelineActionRef()
	if !ok {
		return pipelineRef{}, false
	}
	if a.readOnly {
		a.helpBar.SetStatusMsg("read-only mode: cannot " + verb)
		return pipelineRef{}, false
	}
	if a.overlayVisible() {
		return pipelineRef{}, false
	}
	if running, busy := a.pipelineWrites[ref.key()]; busy {
		a.helpBar.SetStatusMsg(running + " already in progress")
		return pipelineRef{}, false
	}
	return ref, true
}

// pipelineActionRef resolves the run under the cursor in whichever view is on
// screen: the pipelines list or the standalone run in the detail panel in the
// pipelines view, the Builds panel row or the run drilled into from it in the
// pull request view.
func (a *App) pipelineActionRef() (pipelineRef, bool) {
	if a.view == viewPipelines {
		return a.pipelinesViewRef()
	}
	return a.pullRequestsViewRef()
}

// pipelinesViewRef reads the run off the pipelines list or off the run the
// detail panel is showing, which are the two places that view puts one under
// the cursor.
func (a *App) pipelinesViewRef() (pipelineRef, bool) {
	switch {
	case a.side == sideLeft && a.leftFocus == focusPipelines:
		if entry := a.pipelineList.SelectedEntry(); entry != nil {
			return pipelineRef{workspace: a.workspace, slug: entry.Repo, build: entry.Pipeline.BuildNumber}, true
		}
	// A pull request's run can still be on the panel here — it is what the
	// other view left there — and its owner token is a pull request key, which
	// has the same shape as a run key and would be read as build #<id>. Only a
	// standalone run names a run this view can act on.
	case a.side == sideRight && !a.detailView.PipelineOwnerIsPR():
		switch a.detailView.Mode() {
		case views.ModePipeline, views.ModeStepLog:
			// The owner token of a standalone run is its key, so it carries
			// the repository the run belongs to as well as its number.
			if ref, ok := parsePipelineRef(a.detailView.PipelineOwner()); ok {
				return ref, true
			}
		default:
		}
	}

	a.helpBar.SetStatusMsg("select a run in the Pipelines panel (" +
		a.keymap.Keys(ActFocusPullRequests) + ") first")
	return pipelineRef{}, false
}

// pullRequestsViewRef resolves the run through the pull request the Builds
// panel belongs to, which is the only thing in that view that names a
// repository.
func (a *App) pullRequestsViewRef() (pipelineRef, bool) {
	build, ok := a.pipelineActionBuild()
	if !ok {
		return pipelineRef{}, false
	}
	pr := a.currentPullRequest()
	if pr == nil {
		return pipelineRef{}, false
	}
	return pipelineRef{workspace: pr.Workspace(), slug: pr.RepoSlug(), build: build}, true
}

// pipelineActionBuild is the run under the cursor in the pull request view: the
// build row selected in the Builds panel, or the run the detail panel is
// showing. Those are the two places where a run is what the user is looking at;
// anywhere else the key would have to guess which run was meant, so it says
// where to select one instead.
func (a *App) pipelineActionBuild() (int, bool) {
	switch {
	case a.side == sideLeft && a.leftFocus == focusBuilds:
		status := a.buildsPanel.SelectedStatus()
		if status == nil {
			break
		}
		build, ok := status.BuildNumber()
		if !ok {
			a.helpBar.SetStatusMsg(notAPipelineBuild)
			return 0, false
		}
		return build, true

	case a.side == sideRight:
		switch a.detailView.Mode() {
		case views.ModePipeline, views.ModeStepLog:
			// PipelineBuild answers while the run is still loading, which is
			// exactly when a rerun of a failed build is pressed.
			if build := a.detailView.PipelineBuild(); build > 0 {
				return build, true
			}
		default:
		}
	}

	a.helpBar.SetStatusMsg("select a build in the Builds panel (" + a.keymap.Keys(ActFocusBuilds) + ") first")
	return 0, false
}

// beginPipelineWrite records that a write is running against a run, which is
// what stops a second P or X from racing the first.
func (a *App) beginPipelineWrite(ref pipelineRef, verb string) {
	if a.pipelineWrites == nil {
		a.pipelineWrites = make(map[string]string)
	}
	a.pipelineWrites[ref.key()] = verb
	a.logFlag.Store(true)
}

// finishPipelineWrite clears the in-flight record, on success and on failure
// alike.
func (a *App) finishPipelineWrite(ref pipelineRef) {
	delete(a.pipelineWrites, ref.key())
}

// -----------------------------------------------------------------------------
// The read before the dialog
// -----------------------------------------------------------------------------

// handleActionRerunPipeline starts the rerun flow: a run has to be completed to
// be repeated, and only the server knows whether it is.
func (a *App) handleActionRerunPipeline() tea.Cmd {
	ref, ok := a.pipelineActionTarget(verbRerunPipeline)
	if !ok {
		return nil
	}
	return a.fetchPipelineState(ref, verbRerunPipeline)
}

// handleActionStopPipeline starts the stop flow, which is the same flow with
// the state test the other way round.
func (a *App) handleActionStopPipeline() tea.Cmd {
	ref, ok := a.pipelineActionTarget(verbStopPipeline)
	if !ok {
		return nil
	}
	return a.fetchPipelineState(ref, verbStopPipeline)
}

// pipelineActionPRKey is the pull request a pipeline write was started from, or
// "" when it was started in the pipelines view, where there is none. It is what
// decides whose build statuses the write invalidates and whose detail pane it
// drills into afterwards.
func (a *App) pipelineActionPRKey() string {
	if a.view == viewPipelines {
		return ""
	}
	if pr := a.currentPullRequest(); pr != nil {
		return pr.Key()
	}
	return ""
}

// fetchPipelineState reads the run before either dialog opens. Nothing the app
// holds is fresh enough to refuse on: a build status is the pull request's copy
// of the run and lags it, and a cached run is as old as the drill-down that
// fetched it — so "cannot stop: it has already completed" could name a run that
// is still going, and worse, a dialog could offer to stop one that is not.
//
// It is also where the target of a rerun comes from: a rerun repeats the
// original run's commit and selector, and only the run itself carries them.
func (a *App) fetchPipelineState(ref pipelineRef, verb string) tea.Cmd {
	ctx, client := a.context(), a.client
	prKey := a.pipelineActionPRKey()
	number := strconv.Itoa(ref.build)

	a.helpBar.SetStatusMsg("checking pipeline #" + number + "…")
	a.logFlag.Store(true)

	return func() tea.Msg {
		pipeline, err := client.GetPipeline(ctx, ref.workspace, ref.slug, number)
		return pipelineActionStateMsg{ref: ref, prKey: prKey, verb: verb, pipeline: pipeline, err: err}
	}
}

// handlePipelineActionState opens the dialog over the run that was just read,
// or refuses in the run's own words. The read is not cached: it is a snapshot
// taken to answer one question, and storing it would put a run in the pipeline
// cache without the steps that view needs.
func (a *App) handlePipelineActionState(msg pipelineActionStateMsg) (tea.Model, tea.Cmd) {
	a.logFlag.Store(false)

	if msg.err != nil {
		a.helpBar.SetStatusMsg(msg.verb + " failed: " + a.writeErrorText(msg.verb, msg.err))
		return a, nil
	}
	// Either there is no run to state a dialog over, or the read took long
	// enough for the user to walk off the run or open an overlay — and a dialog
	// that appears over something else is a dialog nobody asked for.
	if msg.pipeline == nil || a.overlayVisible() || !a.pipelineStillInView(msg.ref, msg.prKey) {
		return a, nil
	}

	number := strconv.Itoa(msg.ref.build)
	switch msg.verb {
	case verbRerunPipeline:
		if !msg.pipeline.State.IsCompleted() {
			// Bitbucket would start a second run alongside the first, which is
			// never what "rerun" was pressed for.
			a.helpBar.SetStatusMsg("cannot rerun: pipeline #" + number + " is still running")
			return a, nil
		}
	case verbStopPipeline:
		if msg.pipeline.State.IsCompleted() {
			a.helpBar.SetStatusMsg("cannot stop: pipeline #" + number + " has already completed")
			return a, nil
		}
	default:
		return a, nil
	}

	a.showPipelineDialog(msg.ref, msg.prKey, msg.verb, msg.pipeline)
	return a, nil
}

// pipelineStillInView reports whether what the user is looking at is still what
// the read was started for: the same run in the pipelines view, the same pull
// request in the pull request view — where the run itself is the build row's,
// and walking off the pull request takes the whole Builds panel with it.
func (a *App) pipelineStillInView(ref pipelineRef, prKey string) bool {
	if a.view == viewPipelines {
		return a.runKey == ref.key()
	}
	pr := a.currentPullRequest()
	return pr != nil && prKey != "" && pr.Key() == prKey
}

// showPipelineDialog asks before either write. Like the draft and approve
// dialogs it is a form of note rows, so the only keys are enter and esc; the
// two rows are how the run is recognised, because a build number on its own
// says nothing about which commit and which pull request are about to be built
// again.
//
// Everything the write needs is captured here rather than read back off the App
// when the dialog is confirmed: the run behind the cursor can change while the
// question is on screen, and a rerun of whatever is selected then would repeat
// the wrong commit.
func (a *App) showPipelineDialog(ref pipelineRef, prKey, verb string, p *bitbucket.Pipeline) {
	fields := []components.FormField{
		{Kind: components.FormNote, Note: pipelineSummaryNote(p)},
		{Kind: components.FormNote, Note: views.PipelineTargetLine(p.Target)},
	}
	number := strconv.Itoa(ref.build)

	if verb == verbStopPipeline {
		uuid := p.UUID
		a.armForm(formStop, func(components.FormConfirmedMsg) tea.Cmd {
			return a.stopPipeline(ref, prKey, uuid)
		})
		a.form.Show(formStop, "Stop pipeline #"+number+"?", confirmStop, fields)
		return
	}

	target := p.Target
	a.armForm(formRerun, func(components.FormConfirmedMsg) tea.Cmd {
		return a.rerunPipeline(ref, prKey, target)
	})
	a.form.Show(formRerun, "Rerun pipeline #"+number+"?", confirmRerun, fields)
}

// pipelineSummaryNote is the dialog's first row: how the run ended, what
// started it and who, which together tell one run from the several a build
// number may have been through.
func pipelineSummaryNote(p *bitbucket.Pipeline) string {
	note := p.State.Summary() + " · " + p.Trigger.Name
	if p.Creator != nil {
		note += " · by " + p.Creator.Name()
	}
	return note
}

// -----------------------------------------------------------------------------
// The writes
// -----------------------------------------------------------------------------

// rerunPipeline starts a new run with the target of the one that was read. The
// target is the original run's, not the branch's head: a rerun repeats the
// commit that failed, which is the only thing that makes it a rerun.
func (a *App) rerunPipeline(ref pipelineRef, prKey string, target bitbucket.PipelineTarget) tea.Cmd {
	ctx, client := a.context(), a.client

	a.beginPipelineWrite(ref, verbRerunPipeline)
	a.helpBar.SetStatusMsg("rerunning pipeline #" + strconv.Itoa(ref.build) + "…")

	return func() tea.Msg {
		created, err := client.RunPipeline(ctx, ref.workspace, ref.slug, target)
		return pipelineWrittenMsg{
			ref: ref, prKey: prKey, verb: verbRerunPipeline, pipeline: created, err: err,
		}
	}
}

// stopPipeline signals the run to stop. The endpoint answers 204, so there is
// nothing to hand back and the new state is read from the refetch the result
// triggers.
func (a *App) stopPipeline(ref pipelineRef, prKey, uuid string) tea.Cmd {
	ctx, client := a.context(), a.client

	a.beginPipelineWrite(ref, verbStopPipeline)
	a.helpBar.SetStatusMsg("stopping pipeline #" + strconv.Itoa(ref.build) + "…")

	return func() tea.Msg {
		err := client.StopPipeline(ctx, ref.workspace, ref.slug, uuid)
		return pipelineWrittenMsg{
			ref: ref, prKey: prKey, verb: verbStopPipeline, pipeline: nil, err: err,
		}
	}
}

// handlePipelineWritten applies what a pipeline write left behind.
func (a *App) handlePipelineWritten(msg pipelineWrittenMsg) (tea.Model, tea.Cmd) {
	a.finishPipelineWrite(msg.ref)
	a.logFlag.Store(false)

	if msg.err != nil {
		a.helpBar.SetStatusMsg(msg.verb + " failed: " + a.writeErrorText(msg.verb, msg.err))
		return a, nil
	}
	switch msg.verb {
	case verbRerunPipeline:
		return a.pipelineRerunStarted(msg)
	case verbStopPipeline:
		return a.pipelineStopRequested(msg)
	default:
		return a, nil
	}
}

// pipelineRerunStarted opens the run the rerun created. The user pressed the key
// to watch a build again, so the detail panel drills straight into the new run
// the way Enter on a build row would, and the build statuses are refetched
// because the new run's own status only exists on the server.
func (a *App) pipelineRerunStarted(msg pipelineWrittenMsg) (tea.Model, tea.Cmd) {
	if msg.pipeline == nil {
		// A run the server did not name is one there is nothing to say about
		// and nothing to open; the client refuses this case before it gets here.
		return a, nil
	}
	a.statusPanel.SetOnline(true)
	build := msg.pipeline.BuildNumber
	a.helpBar.SetStatusMsg("pipeline #" + strconv.Itoa(build) +
		" started (rerun of #" + strconv.Itoa(msg.ref.build) + ")")

	// Where the write was started decides what it invalidates, and the message
	// is what remembers that: an empty pull request key is the pipelines view's
	// signature. Reading a.view here instead would take the wrong branch for a
	// write whose answer lands after the user has switched views.
	if msg.prKey == "" {
		newRef := pipelineRef{workspace: msg.ref.workspace, slug: msg.ref.slug, build: build}
		if a.view != viewPipelines {
			// The list is still refetched, so the new run is there when the
			// user comes back, but nothing on a screen they have left moves.
			return a, a.fetchPipelines()
		}
		// The run the server answered with is the whole run, so it goes on
		// screen at once, exactly as a row of the list would; only its steps
		// and the list itself still have to be read. The list fetch is issued
		// first on purpose: it bumps the epoch the step fetch has to carry.
		a.runKey = newRef.key()
		a.detailView.SetRun(a.runKey, msg.pipeline, nil)
		focusCmd := a.focusDetail()
		listCmd := a.fetchPipelines()
		return a, tea.Batch(focusCmd, listCmd, a.fetchRunSteps(newRef))
	}

	delete(a.statusCache, msg.prKey)
	// The drill-down is only for the pull request on screen. A rerun started
	// from a row the user has since walked off still leaves the cache drop, so
	// the builds panel is right when they walk back.
	pr := a.pullRequestByKey(msg.prKey)
	if msg.prKey != a.previewKey || pr == nil {
		return a, nil
	}
	a.detailView.SetPipelineLoading(msg.prKey, build)
	focusCmd := a.focusDetail()
	return a, tea.Batch(
		focusCmd,
		a.fetchPipeline(pr.Workspace(), pr.RepoSlug(), msg.prKey, build),
		a.fetchStatuses(pr, a.previewEpoch),
	)
}

// pipelineStopRequested reports the signal and drops what it made stale. The
// stop is a request rather than a result — Bitbucket signals the run and the
// steps wind down — so the wording promises only what has happened, and the
// refetch shows the rest.
func (a *App) pipelineStopRequested(msg pipelineWrittenMsg) (tea.Model, tea.Cmd) {
	a.helpBar.SetStatusMsg("stop requested for pipeline #" + strconv.Itoa(msg.ref.build))
	delete(a.pipelineCache, msg.ref.key())

	// As in pipelineRerunStarted, the branch is the write's own origin rather
	// than whichever view happens to be on screen when the answer lands.
	if msg.prKey == "" {
		// The run the panel is showing comes from the list payload, so the list
		// is what has to be read again for the new state to appear.
		return a, a.fetchPipelines()
	}

	delete(a.statusCache, msg.prKey)
	pr := a.pullRequestByKey(msg.prKey)
	if msg.prKey != a.previewKey || pr == nil {
		return a, nil
	}
	cmds := []tea.Cmd{a.fetchStatuses(pr, a.previewEpoch)}
	// Only the run's own view is refetched, and only when it is the run in
	// view: SetPipeline leaves ModeStepLog for ModePipeline, so a refetch while
	// the user is reading a step's log would throw them out of it.
	if a.detailView.Mode() == views.ModePipeline && a.detailView.PipelineBuild() == msg.ref.build {
		cmds = append(cmds, a.fetchPipeline(pr.Workspace(), pr.RepoSlug(), msg.prKey, msg.ref.build))
	}
	return a, tea.Batch(cmds...)
}
