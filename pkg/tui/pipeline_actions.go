package tui

import (
	"strconv"

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

// pipelineActionTarget is the pull request and the run a pipeline key should act
// on, or false with the reason already on the help bar. It is writeTarget's
// counterpart, and refuses in the same order — with one refusal missing on
// purpose: the pull request's state is not checked, because the pipeline of a
// merged or declined pull request is still a run that can be rerun.
func (a *App) pipelineActionTarget(verb string) (*bitbucket.PullRequest, int, bool) {
	build, ok := a.pipelineActionBuild()
	if !ok {
		return nil, 0, false
	}
	if a.readOnly {
		a.helpBar.SetStatusMsg("read-only mode: cannot " + verb)
		return nil, 0, false
	}
	if a.overlayVisible() {
		return nil, 0, false
	}
	pr := a.currentPullRequest()
	if pr == nil {
		return nil, 0, false
	}
	if running, busy := a.pipelineWrites[pipelineCacheKey(pr.Key(), build)]; busy {
		a.helpBar.SetStatusMsg(running + " already in progress")
		return nil, 0, false
	}
	return pr, build, true
}

// pipelineActionBuild is the run under the cursor: the build row selected in the
// Builds panel, or the run the detail panel is showing. Those are the two places
// where a run is what the user is looking at; anywhere else the key would have
// to guess which run was meant, so it says where to select one instead.
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
func (a *App) beginPipelineWrite(key string, build int, verb string) {
	if a.pipelineWrites == nil {
		a.pipelineWrites = make(map[string]string)
	}
	a.pipelineWrites[pipelineCacheKey(key, build)] = verb
	a.logFlag.Store(true)
}

// finishPipelineWrite clears the in-flight record, on success and on failure
// alike.
func (a *App) finishPipelineWrite(key string, build int) {
	delete(a.pipelineWrites, pipelineCacheKey(key, build))
}

// -----------------------------------------------------------------------------
// The read before the dialog
// -----------------------------------------------------------------------------

// handleActionRerunPipeline starts the rerun flow: a run has to be completed to
// be repeated, and only the server knows whether it is.
func (a *App) handleActionRerunPipeline() tea.Cmd {
	pr, build, ok := a.pipelineActionTarget(verbRerunPipeline)
	if !ok {
		return nil
	}
	return a.fetchPipelineState(pr, build, verbRerunPipeline)
}

// handleActionStopPipeline starts the stop flow, which is the same flow with
// the state test the other way round.
func (a *App) handleActionStopPipeline() tea.Cmd {
	pr, build, ok := a.pipelineActionTarget(verbStopPipeline)
	if !ok {
		return nil
	}
	return a.fetchPipelineState(pr, build, verbStopPipeline)
}

// fetchPipelineState reads the run before either dialog opens. Nothing the app
// holds is fresh enough to refuse on: a build status is the pull request's copy
// of the run and lags it, and a cached run is as old as the drill-down that
// fetched it — so "cannot stop: it has already completed" could name a run that
// is still going, and worse, a dialog could offer to stop one that is not.
//
// It is also where the target of a rerun comes from: a rerun repeats the
// original run's commit and selector, and only the run itself carries them.
func (a *App) fetchPipelineState(pr *bitbucket.PullRequest, build int, verb string) tea.Cmd {
	key := pr.Key()
	ctx, client := a.context(), a.client
	workspace, slug := pr.Workspace(), pr.RepoSlug()
	ref := strconv.Itoa(build)

	a.helpBar.SetStatusMsg("checking pipeline #" + ref + "…")
	a.logFlag.Store(true)

	return func() tea.Msg {
		pipeline, err := client.GetPipeline(ctx, workspace, slug, ref)
		return pipelineActionStateMsg{key: key, build: build, verb: verb, pipeline: pipeline, err: err}
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
	// enough for the user to walk off the pull request or open an overlay — and
	// a dialog that appears over something else is a dialog nobody asked for.
	pr := a.currentPullRequest()
	if msg.pipeline == nil || pr == nil || pr.Key() != msg.key || a.overlayVisible() {
		return a, nil
	}

	number := strconv.Itoa(msg.build)
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

	a.showPipelineDialog(msg.key, msg.build, msg.verb, msg.pipeline)
	return a, nil
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
func (a *App) showPipelineDialog(key string, build int, verb string, p *bitbucket.Pipeline) {
	fields := []components.FormField{
		{Kind: components.FormNote, Note: pipelineSummaryNote(p)},
		{Kind: components.FormNote, Note: views.PipelineTargetLine(p.Target)},
	}
	number := strconv.Itoa(build)

	if verb == verbStopPipeline {
		uuid := p.UUID
		a.armForm(formStop, func(components.FormConfirmedMsg) tea.Cmd {
			return a.stopPipeline(key, build, uuid)
		})
		a.form.Show(formStop, "Stop pipeline #"+number+"?", confirmStop, fields)
		return
	}

	target := p.Target
	a.armForm(formRerun, func(components.FormConfirmedMsg) tea.Cmd {
		return a.rerunPipeline(key, build, target)
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
func (a *App) rerunPipeline(key string, build int, target bitbucket.PipelineTarget) tea.Cmd {
	pr := a.pullRequestByKey(key)
	if pr == nil {
		return nil
	}
	ctx, client := a.context(), a.client
	workspace, slug := pr.Workspace(), pr.RepoSlug()

	a.beginPipelineWrite(key, build, verbRerunPipeline)
	a.helpBar.SetStatusMsg("rerunning pipeline #" + strconv.Itoa(build) + "…")

	return func() tea.Msg {
		created, err := client.RunPipeline(ctx, workspace, slug, target)
		return pipelineWrittenMsg{
			key: key, build: build, verb: verbRerunPipeline, pipeline: created, err: err,
		}
	}
}

// stopPipeline signals the run to stop. The endpoint answers 204, so there is
// nothing to hand back and the new state is read from the refetch the result
// triggers.
func (a *App) stopPipeline(key string, build int, uuid string) tea.Cmd {
	pr := a.pullRequestByKey(key)
	if pr == nil {
		return nil
	}
	ctx, client := a.context(), a.client
	workspace, slug := pr.Workspace(), pr.RepoSlug()

	a.beginPipelineWrite(key, build, verbStopPipeline)
	a.helpBar.SetStatusMsg("stopping pipeline #" + strconv.Itoa(build) + "…")

	return func() tea.Msg {
		err := client.StopPipeline(ctx, workspace, slug, uuid)
		return pipelineWrittenMsg{
			key: key, build: build, verb: verbStopPipeline, pipeline: nil, err: err,
		}
	}
}

// handlePipelineWritten applies what a pipeline write left behind.
func (a *App) handlePipelineWritten(msg pipelineWrittenMsg) (tea.Model, tea.Cmd) {
	a.finishPipelineWrite(msg.key, msg.build)
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
		" started (rerun of #" + strconv.Itoa(msg.build) + ")")
	delete(a.statusCache, msg.key)

	// The drill-down is only for the pull request on screen. A rerun started
	// from a row the user has since walked off still leaves the cache drop, so
	// the builds panel is right when they walk back.
	pr := a.pullRequestByKey(msg.key)
	if msg.key != a.previewKey || pr == nil {
		return a, nil
	}
	a.detailView.SetPipelineLoading(msg.key, build)
	a.side = sideRight
	a.updateFocusState()
	return a, tea.Batch(a.fetchPipeline(pr, build), a.fetchStatuses(pr, a.previewEpoch))
}

// pipelineStopRequested reports the signal and drops what it made stale. The
// stop is a request rather than a result — Bitbucket signals the run and the
// steps wind down — so the wording promises only what has happened, and the
// refetch shows the rest.
func (a *App) pipelineStopRequested(msg pipelineWrittenMsg) (tea.Model, tea.Cmd) {
	a.helpBar.SetStatusMsg("stop requested for pipeline #" + strconv.Itoa(msg.build))
	delete(a.pipelineCache, pipelineCacheKey(msg.key, msg.build))
	delete(a.statusCache, msg.key)

	pr := a.pullRequestByKey(msg.key)
	if msg.key != a.previewKey || pr == nil {
		return a, nil
	}
	cmds := []tea.Cmd{a.fetchStatuses(pr, a.previewEpoch)}
	// Only the run's own view is refetched, and only when it is the run in
	// view: SetPipeline leaves ModeStepLog for ModePipeline, so a refetch while
	// the user is reading a step's log would throw them out of it.
	if a.detailView.Mode() == views.ModePipeline && a.detailView.PipelineBuild() == msg.build {
		cmds = append(cmds, a.fetchPipeline(pr, msg.build))
	}
	return a, tea.Batch(cmds...)
}
