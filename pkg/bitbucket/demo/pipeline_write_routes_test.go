package demo

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// These tests drive the two pipeline write routes over raw HTTP, so the wire
// shapes — the 201 with its Location, the PENDING run that the first read
// promotes, the cloned steps, the build status a rerun grows, and the 204 of a
// stop — are asserted independently of the decoding in pkg/bitbucket.

// rerunRequest is the body a rerun carries: the target of an existing run,
// exactly as that run's own document answers it, in the shape the endpoint
// documents. It is what bitbucket.Client.RunPipeline builds from a run it read.
func rerunRequest(t *testing.T, server *Server, slug string, buildNumber int) runPipelineRequest {
	t.Helper()

	target := findPipeline(t, server.Data, slug, buildNumber).Target
	requested := runTarget{
		Type:              target.Type,
		RefType:           target.RefType,
		RefName:           target.RefName,
		Source:            target.Source,
		Destination:       target.Destination,
		Commit:            requestedCommitOf(target.Commit),
		DestinationCommit: requestedCommitOf(target.DestinationCommit),
		PullRequest:       nil,
		Selector:          nil,
	}
	if target.Selector != nil {
		requested.Selector = &runSelector{Type: target.Selector.Type, Pattern: target.Selector.Pattern}
	}
	if target.PullRequest != nil {
		requested.PullRequest = &runPullRequest{ID: pullRequestID(target.PullRequest.ID)}
	}
	return runPipelineRequest{Target: requested}
}

// requestedCommitOf is a commit of a run's target in the two-member shape a
// request body carries it in.
func requestedCommitOf(commit *bitbucket.CommitRef) *runCommitRef {
	if commit == nil {
		return nil
	}
	return &runCommitRef{Type: commit.Type, Hash: commit.Hash}
}

// runPipeline posts a run request and fails unless the server created a run.
func runPipeline(t *testing.T, server *Server, slug string, body any) bitbucket.Pipeline {
	t.Helper()

	got := postJSON(t, server.URL+pipelinesPath(slug), body, nil)
	if got.Status != http.StatusCreated {
		t.Fatalf("POST %s = status %d, want 201 (body %s)", pipelinesPath(slug), got.Status, got.Body)
	}
	return decodeJSON[bitbucket.Pipeline](t, got.Body)
}

// stepStates is a run's steps reduced to the word the UI shows for each, which
// is what a rerun and a stop have to get right.
func stepStates(steps []bitbucket.PipelineStep) []string {
	states := make([]string, 0, len(steps))
	for _, step := range steps {
		states = append(states, step.State.Summary())
	}
	return states
}

// stepNames is a run's steps reduced to their names, so a clone can be shown to
// have copied them.
func stepNames(steps []bitbucket.PipelineStep) []string {
	names := make([]string, 0, len(steps))
	for index, step := range steps {
		names = append(names, step.DisplayName(index))
	}
	return names
}

// pipelineSteps fetches a run's steps through the route, which is also what
// promotes a freshly created run.
func pipelineSteps(t *testing.T, server *Server, slug string, buildNumber int) []bitbucket.PipelineStep {
	t.Helper()

	body := getOK(t, server, pipelinePath(slug, buildNumber)+segmentSteps)
	return decodeJSON[listPage[bitbucket.PipelineStep]](t, body).Values
}

// pipelineStatuses fetches the build statuses of one pull request.
func pipelineStatuses(t *testing.T, server *Server, slug string, id int) []bitbucket.CommitStatus {
	t.Helper()

	body := getOK(t, server, statusesPath(slug, id))
	return decodeJSON[listPage[bitbucket.CommitStatus]](t, body).Values
}

// -----------------------------------------------------------------------------
// POST .../pipelines
// -----------------------------------------------------------------------------

// TestServer_RunPipelineRerunsAFailedRun is the whole rerun as the endpoint
// answers it: a 201 carrying a PENDING run triggered MANUAL, created by the
// account the demo runs as, against the same target the failed run had.
func TestServer_RunPipelineRerunsAFailedRun(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	source := findPipeline(t, server.Data, repoAPIGateway, buildFailed)
	start := time.Now().UTC()

	got := postJSON(t, server.URL+pipelinesPath(repoAPIGateway),
		rerunRequest(t, server, repoAPIGateway, buildFailed), nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusCreated)

	created := decodeJSON[bitbucket.Pipeline](t, got.Body)
	testkit.AssertEqual(t, "build_number", created.BuildNumber, buildRerun)
	testkit.AssertEqual(t, "uuid", created.UUID, pipelineUUID(buildRerun))
	testkit.AssertEqual(t, "state.name", created.State.Name, statePending)
	testkit.AssertEqual(t, "state.type", created.State.Type, "pipeline_state_pending")
	testkit.AssertEqual(t, "a pending run has no result", created.State.Result == nil, true)
	testkit.AssertEqual(t, "trigger", created.Trigger.Name, triggerManual)
	testkit.AssertEqual(t, "trigger.type", created.Trigger.Type, "pipeline_trigger_manual")
	if created.Creator == nil {
		t.Fatalf("the created run has no creator: %+v", created)
	}
	testkit.AssertEqual(t, "creator.uuid", created.Creator.UUID, uuidDemo)
	if created.Repository == nil {
		t.Fatalf("the created run has no repository: %+v", created)
	}
	testkit.AssertEqual(t, "repository.full_name", created.Repository.FullName, fullName(repoAPIGateway))
	if created.CreatedOn.Before(start) {
		t.Fatalf("created_on = %s, want a stamp at or after %s", created.CreatedOn, start)
	}
	// A run that has not finished carries no completed_on at all.
	testkit.AssertEqual(t, "completed_on is absent",
		func() bool { _, ok := decodeRaw(t, got.Body)[fieldCompletedOn]; return ok }(), false)

	// The pull request target is the failed run's, rebuilt: the title and the
	// links come from the dataset, not from the request.
	testkit.AssertEqual(t, "target.type", created.Target.Type, targetTypePullRequest)
	testkit.AssertEqual(t, "target.source", created.Target.Source, source.Target.Source)
	testkit.AssertEqual(t, "target.destination", created.Target.Destination, branchMain)
	testkit.AssertEqual(t, "target.commit.hash", created.Target.Commit.Hash, hashPR103)
	testkit.AssertEqual(t, "target.destination_commit.hash",
		created.Target.DestinationCommit.Hash, shortHash(hashMainAPI))
	testkit.AssertEqual(t, "target.selector.pattern", created.Target.Selector.Pattern, "**")
	if created.Target.PullRequest == nil {
		t.Fatalf("the pull request target carries no pull request: %+v", created.Target)
	}
	testkit.AssertEqual(t, "target.pullrequest.id", created.Target.PullRequest.ID, 103)
	testkit.AssertEqual(t, "target.pullrequest.title",
		created.Target.PullRequest.Title, source.Target.PullRequest.Title)

	// The Location names the run, and the run is in the dataset ahead of the
	// one it repeats.
	testkit.AssertEqual(t, "Location", got.Header.Get(headerLocation), created.Links.Self.Href)
	testkit.AssertSliceEqual(t, "build numbers, newest first",
		buildNumbersOf(server.Data.Pipelines[repoAPIGateway]),
		[]int{buildRerun, buildFailed, buildRunning, 412, 411})
}

// TestServer_RunPipelinePromotesTheRunOnTheFirstRead pins the one liberty this
// fake takes with the API: nothing here actually runs a pipeline, so a run
// created PENDING is moved to IN_PROGRESS by the first read of it. A rerun that
// stayed PENDING for ever would look broken.
func TestServer_RunPipelinePromotesTheRunOnTheFirstRead(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	runPipeline(t, server, repoAPIGateway, rerunRequest(t, server, repoAPIGateway, buildFailed))

	running := decodeJSON[bitbucket.Pipeline](t,
		getOK(t, server, pipelinePath(repoAPIGateway, buildRerun)))
	testkit.AssertEqual(t, "state.name", running.State.Name, stateInProgress)
	if running.State.Stage == nil {
		t.Fatalf("a running pipeline carries no stage: %+v", running.State)
	}
	testkit.AssertEqual(t, "state.stage.name", running.State.Stage.Name, stageRunning)
	testkit.AssertEqual(t, "state.stage.type", running.State.Stage.Type,
		"pipeline_state_in_progress_running")
	testkit.AssertEqual(t, "Summary()", running.State.Summary(), stageRunning)

	// Reading it again does not move it on: the promotion happens once.
	again := decodeJSON[bitbucket.Pipeline](t, getOK(t, server, pipelinePath(repoAPIGateway, buildRerun)))
	testkit.AssertEqual(t, "state after a second read", again.State.Name, stateInProgress)
}

// TestServer_RunPipelineClonesTheStepsOfTheRunItRepeats covers the steps a
// rerun starts with. They are the repeated run's, so the detail pane shows the
// same pipeline rather than an empty one, but every result and duration is gone
// — none of them has run yet.
func TestServer_RunPipelineClonesTheStepsOfTheRunItRepeats(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	runPipeline(t, server, repoAPIGateway, rerunRequest(t, server, repoAPIGateway, buildFailed))

	// Fetching the steps promotes the run as surely as fetching the run does,
	// which is why the first step is already running here.
	steps := pipelineSteps(t, server, repoAPIGateway, buildRerun)
	testkit.AssertEqual(t, "len(steps)", len(steps), 4)
	testkit.AssertSliceEqual(t, "step names", stepNames(steps),
		stepNames(pipelineSteps(t, server, repoAPIGateway, buildFailed)))
	testkit.AssertSliceEqual(t, "step states", stepStates(steps),
		[]string{stateInProgress, statePending, statePending, statePending})

	for index, step := range steps {
		label := "step " + strconv.Itoa(index)
		testkit.AssertEqual(t, label+" uuid", step.UUID, stepUUID(buildRerun, index))
		testkit.AssertEqual(t, label+" belongs to the new run", step.Pipeline.UUID,
			pipelineUUID(buildRerun))
		// Not one of them has finished, so not one of them has a result.
		testkit.AssertEqual(t, label+" has no result", step.State.Result == nil, true)
		testkit.AssertEqual(t, label+" duration", step.DurationInSeconds, 0)
		testkit.AssertEqual(t, label+" has not completed", step.CompletedOn.IsZero(), true)
	}

	// A step that has not run has no log, and the endpoint answers 404 for it
	// here exactly as the real one does.
	logPath := pipelinePath(repoAPIGateway, buildRerun) + segmentSteps + "/" + steps[0].UUID + "/log"
	testkit.AssertEqual(t, "log status", get(t, server.URL+logPath, nil).Status, http.StatusNotFound)
}

// TestServer_RunPipelineAddsABuildStatus is what makes a rerun visible where
// the user started it: the Builds panel reads statuses, so the new run needs
// one of its own beside the one its predecessor left behind.
func TestServer_RunPipelineAddsABuildStatus(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	before := pipelineStatuses(t, server, repoAPIGateway, 103)
	testkit.AssertEqual(t, "the fixture has one status", len(before), 1)
	start := time.Now().UTC()

	runPipeline(t, server, repoAPIGateway, rerunRequest(t, server, repoAPIGateway, buildFailed))

	after := pipelineStatuses(t, server, repoAPIGateway, 103)
	testkit.AssertEqual(t, "len(statuses)", len(after), 2)

	added := after[1]
	testkit.AssertEqual(t, "key is the predecessor's", added.Key, before[0].Key)
	testkit.AssertEqual(t, "name is the predecessor's", added.Name, before[0].Name)
	testkit.AssertEqual(t, "refname is the predecessor's", added.RefName, before[0].RefName)
	testkit.AssertEqual(t, "commit is the predecessor's", added.Commit.Hash, before[0].Commit.Hash)
	testkit.AssertEqual(t, "state", added.State, bitbucket.StatusInProgress)
	testkit.AssertEqual(t, "description", added.Description, "Build 415 is running")
	if added.CreatedOn.Before(start) {
		t.Fatalf("created_on = %s, want a stamp at or after %s", added.CreatedOn, start)
	}

	// The URL is how the Builds panel gets from the row to the run, so it has
	// to name the new build and not the old one.
	number, ok := added.BuildNumber()
	testkit.AssertEqual(t, "the URL names a build", ok, true)
	testkit.AssertEqual(t, "build number", number, buildRerun)
	// The predecessor keeps its own verdict: a rerun does not rewrite history.
	testkit.AssertEqual(t, "the failed status survives", after[0].State, bitbucket.StatusFailed)
}

// TestServer_RunPipelineWithARefTarget is the other target shape the Builds
// panel can hand over: a run against a branch, with no pull request anywhere
// near it — and with the ref_type left out, which the endpoint reads as a
// branch.
func TestServer_RunPipelineWithARefTarget(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	request := rerunRequest(t, server, repoAPIGateway, 411)
	request.Target.RefType = ""

	created := runPipeline(t, server, repoAPIGateway, request)
	testkit.AssertEqual(t, "build_number", created.BuildNumber, buildRerun)
	testkit.AssertEqual(t, "target.type", created.Target.Type, targetTypeRef)
	testkit.AssertEqual(t, "target.ref_type", created.Target.RefType, refTypeBranch)
	testkit.AssertEqual(t, "target.ref_name", created.Target.RefName, branchMain)
	testkit.AssertEqual(t, "target.commit.hash", created.Target.Commit.Hash, hashMainAPI)
	testkit.AssertEqual(t, "no pull request on a ref target", created.Target.PullRequest == nil, true)
	testkit.AssertEqual(t, "BranchName()", created.Target.BranchName(), branchMain)

	// The steps come from the run against the same commit, and no status is
	// added: a branch run posts none in this world.
	testkit.AssertEqual(t, "len(steps)", len(pipelineSteps(t, server, repoAPIGateway, buildRerun)), 3)
	testkit.AssertEqual(t, "no status was added",
		len(pipelineStatuses(t, server, repoAPIGateway, 103)), 1)
}

// TestServer_RunPipelineWithACommitTarget is the third shape: a bare commit,
// with neither a ref nor a pull request.
func TestServer_RunPipelineWithACommitTarget(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	created := runPipeline(t, server, repoAPIGateway, runPipelineRequest{Target: runTarget{
		Type:              targetTypeCommit,
		RefType:           "",
		RefName:           "",
		Source:            "",
		Destination:       "",
		Commit:            &runCommitRef{Type: "commit", Hash: hashPR101},
		DestinationCommit: nil,
		PullRequest:       nil,
		Selector:          &runSelector{Type: "custom", Pattern: "Deploy to production"},
	}})
	testkit.AssertEqual(t, "target.type", created.Target.Type, targetTypeCommit)
	testkit.AssertEqual(t, "target.commit.hash", created.Target.Commit.Hash, hashPR101)
	testkit.AssertEqual(t, "target.selector.type", created.Target.Selector.Type, "custom")
	testkit.AssertEqual(t, "no ref_name on a commit target", created.Target.RefName, "")
	// Build 412 ran against that commit, so its three steps are what the new
	// run starts with.
	testkit.AssertEqual(t, "len(steps)", len(pipelineSteps(t, server, repoAPIGateway, buildRerun)), 3)
}

// TestServer_RunPipelineAcceptsAQuotedPullRequestID is the one place the
// endpoint's documentation and its own documents disagree: the docs quote
// pullrequest.id as a string, every pipeline document answers it as a number,
// and the endpoint takes either. So does this.
func TestServer_RunPipelineAcceptsAQuotedPullRequestID(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	body := json.RawMessage(`{"target":{"type":"` + targetTypePullRequest + `",` +
		`"source":"bugfix/upstream-timeout","destination":"` + branchMain + `",` +
		`"commit":{"type":"commit","hash":"` + hashPR103 + `"},"pullrequest":{"id":"103"}}}`)

	created := runPipeline(t, server, repoAPIGateway, body)
	testkit.AssertEqual(t, "target.pullrequest.id", created.Target.PullRequest.ID, 103)
	testkit.AssertEqual(t, "target.pullrequest.title", created.Target.PullRequest.Title,
		"Fix upstream timeout handling on retried requests")
}

// TestServer_RunPipelineRejectsAnUnusableTarget walks every way a target can be
// refused. The client validates the same things before it sends anything, so
// these are the answers a caller that got it wrong anyway would see.
func TestServer_RunPipelineRejectsAnUnusableTarget(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		target  runTarget
		message string
	}{
		{
			name:    "unknown type",
			target:  runTarget{Type: "pipeline_tag_target"},
			message: `target.type: "pipeline_tag_target" is not a valid choice.`,
		},
		{
			name:    "no type at all",
			target:  runTarget{},
			message: `target.type: "" is not a valid choice.`,
		},
		{
			name:    "ref target without a ref_name",
			target:  runTarget{Type: targetTypeRef, RefType: refTypeBranch},
			message: "target.ref_name: This field is required.",
		},
		{
			name:    "commit target without a commit",
			target:  runTarget{Type: targetTypeCommit},
			message: "target.commit.hash: This field is required.",
		},
		{
			name: "pull request target without a source",
			target: runTarget{
				Type:        targetTypePullRequest,
				Destination: branchMain,
				Commit:      &runCommitRef{Type: "commit", Hash: hashPR103},
				PullRequest: &runPullRequest{ID: 103},
			},
			message: "target.source: This field is required.",
		},
		{
			name: "pull request target without a destination",
			target: runTarget{
				Type:        targetTypePullRequest,
				Source:      "bugfix/upstream-timeout",
				Commit:      &runCommitRef{Type: "commit", Hash: hashPR103},
				PullRequest: &runPullRequest{ID: 103},
			},
			message: "target.destination: This field is required.",
		},
		{
			name: "pull request target without a commit",
			target: runTarget{
				Type:        targetTypePullRequest,
				Source:      "bugfix/upstream-timeout",
				Destination: branchMain,
				PullRequest: &runPullRequest{ID: 103},
			},
			message: "target.commit.hash: This field is required.",
		},
		{
			name: "pull request target without a pull request",
			target: runTarget{
				Type:        targetTypePullRequest,
				Source:      "bugfix/upstream-timeout",
				Destination: branchMain,
				Commit:      &runCommitRef{Type: "commit", Hash: hashPR103},
			},
			message: "target.pullrequest.id: This field is required.",
		},
		{
			name: "pull request target naming a pull request in another repository",
			target: runTarget{
				Type:        targetTypePullRequest,
				Source:      "bugfix/upstream-timeout",
				Destination: branchMain,
				Commit:      &runCommitRef{Type: "commit", Hash: hashPR103},
				PullRequest: &runPullRequest{ID: 40},
			},
			message: "target.pullrequest.id: no pull request 40 in " + fullName(repoAPIGateway) + ".",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server := newTestServer(t)
			got := postJSON(t, server.URL+pipelinesPath(repoAPIGateway),
				runPipelineRequest{Target: testCase.target}, nil)
			testkit.AssertEqual(t, "status", got.Status, http.StatusBadRequest)
			testkit.AssertEqual(t, "error.message",
				decodeJSON[errorBody](t, got.Body).Error.Message, testCase.message)
			// A refused target starts nothing.
			testkit.AssertEqual(t, "no run was created",
				len(server.Data.Pipelines[repoAPIGateway]), 4)
		})
	}
}

func TestServer_RunPipelineUnknownRepositoryIsNotFound(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	got := postJSON(t, server.URL+pipelinesPath("no-such-repository"),
		rerunRequest(t, server, repoAPIGateway, buildFailed), nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusNotFound)
}

// -----------------------------------------------------------------------------
// POST .../pipelines/{ref}/stopPipeline
// -----------------------------------------------------------------------------

// TestServer_StopPipeline is the whole stop: a 204 with no document, and a run,
// its steps and its build status all left where a stop leaves them.
func TestServer_StopPipeline(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	start := time.Now().UTC()

	got := postJSON(t, server.URL+stopPipelinePath(repoAPIGateway, strconv.Itoa(buildRunning)), nil, nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusNoContent)
	testkit.AssertEqual(t, "a 204 carries no document", len(got.Body), 0)

	stopped := decodeJSON[bitbucket.Pipeline](t,
		getOK(t, server, pipelinePath(repoAPIGateway, buildRunning)))
	testkit.AssertEqual(t, "state.name", stopped.State.Name, stateCompleted)
	testkit.AssertEqual(t, "Summary()", stopped.State.Summary(), resultStopped)
	if stopped.State.Result == nil || stopped.State.Result.Terminator == nil {
		t.Fatalf("a stopped run carries no result with a terminator: %+v", stopped.State)
	}
	testkit.AssertEqual(t, "state.result.type", stopped.State.Result.Type,
		"pipeline_state_completed_stopped")
	// Who stopped it is the account the demo runs as, the way a real session
	// stops a run as whoever the token belongs to.
	testkit.AssertEqual(t, "terminator", stopped.State.Result.Terminator.UUID, uuidDemo)
	testkit.AssertEqual(t, "a stopped run has no stage", stopped.State.Stage == nil, true)
	if stopped.CompletedOn.Before(start) {
		t.Fatalf("completed_on = %s, want a stamp at or after %s", stopped.CompletedOn, start)
	}
	if stopped.DurationInSeconds < 1 {
		t.Fatalf("duration_in_seconds = %d, want at least a second", stopped.DurationInSeconds)
	}

	// The step that was running was cut short, the ones that had not started
	// never will, and the one that had already passed keeps its result.
	testkit.AssertSliceEqual(t, "step states",
		stepStates(pipelineSteps(t, server, repoAPIGateway, buildRunning)),
		[]string{resultSuccessful, resultStopped, resultNotRun, resultNotRun})

	statuses := pipelineStatuses(t, server, repoAPIGateway, 102)
	testkit.AssertEqual(t, "len(statuses)", len(statuses), 1)
	testkit.AssertEqual(t, "status state", statuses[0].State, bitbucket.StatusStopped)
	testkit.AssertEqual(t, "status description", statuses[0].Description, "Build 413 was stopped")
}

// The endpoint's own {pipeline_uuid} is a braced UUID, which the client sends
// percent-encoded — the build number is the convenience, not the contract.
func TestServer_StopPipelineByUUID(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	uuid := findPipeline(t, server.Data, repoAPIGateway, buildRunning).UUID

	got := postJSON(t, server.URL+stopPipelinePath(repoAPIGateway, uuid), nil, nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusNoContent)

	stopped := decodeJSON[bitbucket.Pipeline](t,
		getOK(t, server, pipelinePath(repoAPIGateway, buildRunning)))
	testkit.AssertEqual(t, "Summary()", stopped.State.Summary(), resultStopped)
}

func TestServer_StopPipelineAlreadyCompletedIsBadRequest(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	got := postJSON(t, server.URL+stopPipelinePath(repoAPIGateway, strconv.Itoa(buildFailed)), nil, nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusBadRequest)
	testkit.AssertEqual(t, "error.message",
		decodeJSON[errorBody](t, got.Body).Error.Message, msgPipelineCompleted)
	// Its verdict is untouched: a refused stop stops nothing.
	testkit.AssertEqual(t, "the run is untouched",
		findPipeline(t, server.Data, repoAPIGateway, buildFailed).State.Summary(), resultFailed)
}

func TestServer_StopPipelineUnknownIsNotFound(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	testkit.AssertEqual(t, "an unknown run",
		postJSON(t, server.URL+stopPipelinePath(repoAPIGateway, "999999"), nil, nil).Status,
		http.StatusNotFound)
	testkit.AssertEqual(t, "an unknown repository",
		postJSON(t, server.URL+stopPipelinePath("no-such-repository", strconv.Itoa(buildRunning)),
			nil, nil).Status,
		http.StatusNotFound)
}

// TestServer_StopAndRerunARerun is the loop the two routes make together: a run
// this server created is as stoppable as one the dataset shipped, and stopping
// it makes it rerunnable in turn.
func TestServer_StopAndRerunARerun(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	runPipeline(t, server, repoAPIGateway, rerunRequest(t, server, repoAPIGateway, buildFailed))

	// The rerun is still PENDING — nobody has read it — and a run that has not
	// completed can be stopped.
	testkit.AssertEqual(t, "stop status",
		postJSON(t, server.URL+stopPipelinePath(repoAPIGateway, strconv.Itoa(buildRerun)), nil, nil).Status,
		http.StatusNoContent)

	stopped := decodeJSON[bitbucket.Pipeline](t,
		getOK(t, server, pipelinePath(repoAPIGateway, buildRerun)))
	// The promotion does not fire behind a stop: the run is finished.
	testkit.AssertEqual(t, "Summary()", stopped.State.Summary(), resultStopped)
	testkit.AssertSliceEqual(t, "step states",
		stepStates(pipelineSteps(t, server, repoAPIGateway, buildRerun)),
		[]string{resultNotRun, resultNotRun, resultNotRun, resultNotRun})

	again := runPipeline(t, server, repoAPIGateway, rerunRequest(t, server, repoAPIGateway, buildRerun))
	testkit.AssertEqual(t, "build_number", again.BuildNumber, buildRerun+1)
	testkit.AssertEqual(t, "state", again.State.Name, statePending)
	testkit.AssertEqual(t, "len(steps)", len(pipelineSteps(t, server, repoAPIGateway, buildRerun+1)), 4)
}
