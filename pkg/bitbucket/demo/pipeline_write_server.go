package demo

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
)

// The two routes here are the pipeline half of the write surface: POST
// .../pipelines starts a run for a target — which is how a completed run is
// rerun — and POST .../pipelines/{ref}/stopPipeline signals a running one to
// stop. They live apart from write_server.go, which is already long, and they
// authenticate nothing for the same reason the pull request writes do not.
//
// Designated fixtures, so a TUI walkthrough always has one of each to act on:
//
//   - api-gateway build 413 (pull request #102) is the only IN_PROGRESS run in
//     the dataset, and therefore the only one a stop can succeed against.
//   - api-gateway build 414 (pull request #103) is FAILED, which is what a
//     rerun is for; rerunning it creates build 415.
//
// A run this endpoint creates starts PENDING and is promoted to IN_PROGRESS by
// the first GET of it — see promotePendingRun.

// Target types, spelled as the API's pipeline_ref_target /
// pipeline_commit_target / pipeline_pullrequest_target union spells them.
const (
	targetTypeRef         = "pipeline_ref_target"
	targetTypeCommit      = "pipeline_commit_target"
	targetTypePullRequest = "pipeline_pullrequest_target"

	// refTypeBranch is the ref_type a ref target that names none is read as.
	// The API accepts branch | tag | named_branch | bookmark and a run against
	// a branch is overwhelmingly the common case.
	refTypeBranch = "branch"
)

// msgPipelineCompleted is the sentence the stop endpoint refuses a finished run
// with, quoted from its own documentation.
const msgPipelineCompleted = "The specified pipeline has already completed."

// runPipelineRequest is the body of POST .../pipelines. The endpoint takes a
// single target and nothing else.
type runPipelineRequest struct {
	Target runTarget `json:"target"`
}

// runTarget flattens the three target shapes the endpoint documents, the way
// bitbucket.PipelineTarget flattens them on the way back. Which members are
// required is decided by Type; see requestedTarget.
type runTarget struct {
	Type              string          `json:"type"`
	RefType           string          `json:"ref_type"`
	RefName           string          `json:"ref_name"`
	Source            string          `json:"source"`
	Destination       string          `json:"destination"`
	Commit            *runCommitRef   `json:"commit"`
	DestinationCommit *runCommitRef   `json:"destination_commit"`
	PullRequest       *runPullRequest `json:"pullrequest"`
	Selector          *runSelector    `json:"selector"`
}

// runCommitRef is a commit inside a target: the two members the request carries
// and none of the links the read model has.
type runCommitRef struct {
	Type string `json:"type"`
	Hash string `json:"hash"`
}

// runSelector is the bitbucket-pipelines.yml selector a target asks for.
type runSelector struct {
	Type    string `json:"type"`
	Pattern string `json:"pattern"`
}

// runPullRequest is the pull request a pull-request target runs for. Only the
// id is read: the title and the links of the run's target are taken from the
// dataset's own pull request, not from whatever the request claimed.
type runPullRequest struct {
	ID pullRequestID `json:"id"`
}

// pullRequestID is a pull request id in a request body. Bitbucket's own
// documentation quotes it as a string while every document it answers with
// carries it as a number, and the endpoint takes either — so this does too,
// rather than making a caller guess which spelling the fake wants.
type pullRequestID int

func (id *pullRequestID) UnmarshalJSON(data []byte) error {
	text := strings.Trim(string(data), `"`)
	if text == "" || text == "null" {
		return nil
	}
	value, err := strconv.Atoi(text)
	if err != nil {
		return fmt.Errorf("pullrequest.id %s: %w", data, err)
	}
	*id = pullRequestID(value)
	return nil
}

// -----------------------------------------------------------------------------
// Run
// -----------------------------------------------------------------------------

// handleRunPipeline starts a run for the target the body names and answers 201
// with the run it created, exactly as the real endpoint does — a pipeline
// document in state PENDING, triggered MANUAL, plus a Location naming it.
//
// The new run is not conjured out of nothing: its steps are cloned from the
// newest run in the repository that targeted the same commit, which for a rerun
// is the run being repeated, and a pull request target grows a build status
// beside the one that run left behind. That is what makes a rerun visible in
// the Builds panel the way it is visible in the web UI.
func (s *Server) handleRunPipeline(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeBody[runPipelineRequest](w, r)
	if !ok {
		return
	}

	s.dataMu.Lock()
	defer s.dataMu.Unlock()

	if _, found := s.findRepository(r); !found {
		writeRepositoryNotFound(w, r)
		return
	}
	slug := r.PathValue("slug")

	target, ok := s.requestedTarget(w, slug, body.Target)
	if !ok {
		return
	}

	pipeline := s.startPipeline(slug, target)
	w.Header().Set("Location", pipeline.Links.Self.Href)
	writeJSONStatus(w, http.StatusCreated, pipeline)
}

// startPipeline records a new run, its steps and — for a pull request target —
// its build status, and reports the run. The caller must hold dataMu for
// writing and must have validated the target.
func (s *Server) startPipeline(slug string, target bitbucket.PipelineTarget) bitbucket.Pipeline {
	buildNumber := s.nextBuildNumber(slug)
	uuid := pipelineUUID(buildNumber)
	now := time.Now().UTC()

	// The source is resolved before the new run is recorded, or the new run —
	// which targets the same commit — would find itself.
	source, hasSource := s.rerunSource(slug, target)

	pipeline := bitbucket.Pipeline{
		Type:        "pipeline",
		UUID:        uuid,
		BuildNumber: buildNumber,
		State:       bitbucket.PipelineState{Type: "pipeline_state_pending", Name: statePending},
		Creator:     ptr(s.Data.User),
		Repository:  ptr(repoRef(slug)),
		Target:      target,
		// A run started through the API is MANUAL however the run it repeats
		// was triggered.
		Trigger:   bitbucket.PipelineTrigger{Type: "pipeline_trigger_manual", Name: "MANUAL"},
		CreatedOn: now,
		RunNumber: 1,
	}
	pipeline.Links.Self = bitbucket.Link{
		Href: apiBase + "/repositories/" + fullName(slug) + "/pipelines/" + url.PathEscape(uuid),
	}
	pipeline.Links.Steps = bitbucket.Link{Href: pipeline.Links.Self.Href + "/steps"}

	s.Data.Pipelines[slug] = append([]bitbucket.Pipeline{pipeline}, s.Data.Pipelines[slug]...)
	s.Data.Steps[uuid] = newRunSteps(s.Data.Steps[source.UUID], uuid, buildNumber)
	s.pendingRuns[uuid] = true

	if hasSource && target.PullRequest != nil {
		s.trackRerun(slug, target.PullRequest.ID, source.BuildNumber, buildNumber, now)
	}
	return pipeline
}

// nextBuildNumber is the number the repository's next run gets. Build numbers
// are per repository and never reused, so it is one past the highest there is.
// The caller must hold dataMu.
func (s *Server) nextBuildNumber(slug string) int {
	highest := 0
	for _, pipeline := range s.Data.Pipelines[slug] {
		highest = max(highest, pipeline.BuildNumber)
	}
	return highest + 1
}

// rerunSource is the run a new run repeats: the newest one in the repository
// that targeted the same commit. It is what the new run's steps are cloned from
// and where its build status is copied from, and there is none for a target
// naming a commit nothing has been run against. The caller must hold dataMu.
func (s *Server) rerunSource(slug string, target bitbucket.PipelineTarget) (bitbucket.Pipeline, bool) {
	if target.Commit == nil {
		return bitbucket.Pipeline{}, false
	}
	for _, pipeline := range s.Data.Pipelines[slug] {
		if pipeline.Target.Commit != nil && pipeline.Target.Commit.Hash == target.Commit.Hash {
			return pipeline, true
		}
	}
	return bitbucket.Pipeline{}, false
}

// -----------------------------------------------------------------------------
// The requested target
// -----------------------------------------------------------------------------

// requestedTarget turns the body's target into the target the new run carries,
// or writes the 400 an unusable one is refused with and reports ok false. The
// caller must hold dataMu.
//
// The rebuild is deliberate rather than a copy: a target the server answers
// with carries links and a title the request never sent, so a rerun's target
// has to be assembled from the dataset the way data.go assembles the fixtures'.
func (s *Server) requestedTarget(
	w http.ResponseWriter, slug string, requested runTarget,
) (bitbucket.PipelineTarget, bool) {
	target := bitbucket.PipelineTarget{
		Type:     requested.Type,
		Selector: requestedSelector(requested.Selector),
		Commit:   requestedCommit(slug, requested.Commit),
	}

	switch requested.Type {
	case targetTypeRef:
		if requested.RefName == "" {
			writeBadRequest(w, missingTargetMember("ref_name"))
			return bitbucket.PipelineTarget{}, false
		}
		target.RefName = requested.RefName
		// A ref target that names no ref_type is a branch; the API's own
		// examples leave it out for exactly that case.
		target.RefType = requested.RefType
		if target.RefType == "" {
			target.RefType = refTypeBranch
		}
	case targetTypeCommit:
		if target.Commit == nil {
			writeBadRequest(w, missingTargetMember("commit.hash"))
			return bitbucket.PipelineTarget{}, false
		}
	case targetTypePullRequest:
		return s.requestedPullRequestTarget(w, slug, requested, target)
	default:
		writeBadRequest(w, `target.type: "`+requested.Type+`" is not a valid choice.`)
		return bitbucket.PipelineTarget{}, false
	}
	return target, true
}

// requestedPullRequestTarget completes a pull request target, which needs the
// most of the three: the branches on both sides, the commit, and a pull request
// that exists in this repository. The caller must hold dataMu.
func (s *Server) requestedPullRequestTarget(
	w http.ResponseWriter, slug string, requested runTarget, target bitbucket.PipelineTarget,
) (bitbucket.PipelineTarget, bool) {
	switch {
	case requested.Source == "":
		writeBadRequest(w, missingTargetMember("source"))
		return bitbucket.PipelineTarget{}, false
	case requested.Destination == "":
		writeBadRequest(w, missingTargetMember("destination"))
		return bitbucket.PipelineTarget{}, false
	case target.Commit == nil:
		writeBadRequest(w, missingTargetMember("commit.hash"))
		return bitbucket.PipelineTarget{}, false
	case requested.PullRequest == nil || requested.PullRequest.ID == 0:
		writeBadRequest(w, missingTargetMember("pullrequest.id"))
		return bitbucket.PipelineTarget{}, false
	}

	id := int(requested.PullRequest.ID)
	pullRequest, found := s.pullRequestByID(slug, id)
	if !found {
		writeBadRequest(w, "target.pullrequest.id: no pull request "+
			strconv.Itoa(id)+" in "+fullName(slug)+".")
		return bitbucket.PipelineTarget{}, false
	}

	target.Source = requested.Source
	target.Destination = requested.Destination
	target.DestinationCommit = requestedCommit(slug, requested.DestinationCommit)
	target.PullRequest = runPullRequestRef(slug, pullRequest)
	return target, true
}

// pullRequestByID is the repository's pull request with this id. The caller
// must hold dataMu.
func (s *Server) pullRequestByID(slug string, id int) (bitbucket.PullRequest, bool) {
	for _, pullRequest := range s.Data.PullRequests[slug] {
		if pullRequest.ID == id {
			return pullRequest, true
		}
	}
	return bitbucket.PullRequest{}, false
}

// missingTargetMember is the 400 a target that leaves out a member it needs is
// refused with, naming the member the way the API's other field errors do.
func missingTargetMember(member string) string {
	return "target." + member + ": This field is required."
}

// requestedCommit is the commit a target member names, or nil when the request
// left it out or sent it without a hash — the endpoint treats both as absent.
func requestedCommit(slug string, ref *runCommitRef) *bitbucket.CommitRef {
	if ref == nil || ref.Hash == "" {
		return nil
	}
	return newCommit(slug, ref.Hash)
}

// requestedSelector is the selector a target asked for, or nil when it asked
// for none — which is legal on a ref and a commit target.
func requestedSelector(selector *runSelector) *bitbucket.PipelineSelector {
	if selector == nil {
		return nil
	}
	return &bitbucket.PipelineSelector{Type: selector.Type, Pattern: selector.Pattern}
}

// runPullRequestRef is the pull request reference a pull-request target carries.
// The title and the links come from the dataset rather than from the request,
// because that is where the server gets them in production.
func runPullRequestRef(slug string, pullRequest bitbucket.PullRequest) *bitbucket.PipelinePullRequestRef {
	ref := &bitbucket.PipelinePullRequestRef{
		Type:  "pullrequest",
		ID:    pullRequest.ID,
		Title: pullRequest.Title,
	}
	number := strconv.Itoa(pullRequest.ID)
	full := fullName(slug)
	ref.Links.Self = bitbucket.Link{Href: apiBase + "/repositories/" + full + "/pullrequests/" + number}
	ref.Links.HTML = bitbucket.Link{Href: htmlBase + "/" + full + "/pull-requests/" + number}
	return ref
}

// -----------------------------------------------------------------------------
// The new run's steps and status
// -----------------------------------------------------------------------------

// newRunSteps is the step list a new run starts with: the source run's steps
// reset to PENDING, or a plain Build/Test pair when there is no run to copy.
// None of them carries a log, because none of them has run — the log endpoint
// answers 404 for a step in that state, here as in production.
func newRunSteps(source []bitbucket.PipelineStep, uuid string, buildNumber int) []bitbucket.PipelineStep {
	if len(source) == 0 {
		source = defaultRunSteps()
	}

	steps := make([]bitbucket.PipelineStep, 0, len(source))
	for index, step := range source {
		fresh := step
		fresh.UUID = stepUUID(buildNumber, index)
		fresh.State = bitbucket.PipelineStepState{Type: "pipeline_step_state_pending", Name: statePending}
		fresh.StartedOn = time.Time{}
		fresh.CompletedOn = time.Time{}
		fresh.DurationInSeconds = 0
		fresh.BuildSecondsUsed = 0
		fresh.RunNumber = 1
		fresh.Pipeline.UUID = uuid
		// A step that has not started has no commands either: they are what the
		// runner expanded the yml into, and this run has not been expanded yet.
		fresh.SetupCommands = nil
		fresh.ScriptCommands = nil
		fresh.TeardownCommands = nil
		steps = append(steps, fresh)
	}
	return steps
}

// defaultRunSteps is what a run against a commit nothing has been run against
// gets. There is no bitbucket-pipelines.yml in this world to read the real
// steps out of, so it is the shortest plausible pipeline — and no image, since
// that too would have come from the yml.
func defaultRunSteps() []bitbucket.PipelineStep {
	names := []string{"Build", "Test"}
	steps := make([]bitbucket.PipelineStep, 0, len(names))
	for _, name := range names {
		step := bitbucket.PipelineStep{Type: "pipeline_step", Name: name, MaxTime: 120}
		step.Trigger.Type = "pipeline_step_trigger_automatic"
		step.Pipeline.Type = "pipeline"
		steps = append(steps, step)
	}
	return steps
}

// trackRerun gives the new run the build status its predecessor had, so the
// Builds panel and the pull request's status list show the rerun as soon as it
// starts. A run whose predecessor left no status leaves none either: the
// statuses in this world are posted by whatever pipeline posted the first one,
// and inventing a key would invent a check that does not exist.
// The caller must hold dataMu for writing.
func (s *Server) trackRerun(slug string, pullRequestID, sourceBuild, buildNumber int, now time.Time) {
	key := bitbucket.PullRequestKey(Workspace, slug, pullRequestID)
	for _, status := range s.Data.Statuses[key] {
		if number, ok := status.BuildNumber(); !ok || number != sourceBuild {
			continue
		}
		fresh := status
		fresh.URL = htmlBase + "/" + fullName(slug) + "/pipelines/results/" + strconv.Itoa(buildNumber)
		fresh.State = bitbucket.StatusInProgress
		fresh.Description = "Build " + strconv.Itoa(buildNumber) + " is running"
		fresh.CreatedOn = now
		fresh.UpdatedOn = now
		s.Data.Statuses[key] = append(s.Data.Statuses[key], fresh)
		return
	}
}

// -----------------------------------------------------------------------------
// Promotion
// -----------------------------------------------------------------------------

// promotePendingRun moves a run this server created from PENDING to
// IN_PROGRESS the first time anyone reads it or its steps, and forgets it.
//
// It exists because nothing here actually runs a pipeline: a rerun that stayed
// PENDING for ever would look broken, and a fake that flipped it immediately
// would never show the state the API answers a freshly created run with. The
// queued merge's task does the same thing on its first poll.
//
// It takes the write lock, so the read handlers call it before taking theirs.
func (s *Server) promotePendingRun(r *http.Request) {
	s.dataMu.Lock()
	defer s.dataMu.Unlock()

	slug, index, ok := s.pipelineIndex(r)
	if !ok {
		return
	}
	pipeline := &s.Data.Pipelines[slug][index]
	if !s.pendingRuns[pipeline.UUID] {
		return
	}
	delete(s.pendingRuns, pipeline.UUID)

	pipeline.State = bitbucket.PipelineState{
		Type: "pipeline_state_in_progress",
		Name: stateInProgress,
		Stage: &bitbucket.PipelineStateStage{
			Type: "pipeline_state_in_progress_" + strings.ToLower(stageRunning),
			Name: stageRunning,
		},
	}

	steps := s.Data.Steps[pipeline.UUID]
	if len(steps) == 0 {
		return
	}
	steps[0].State = bitbucket.PipelineStepState{
		Type: "pipeline_step_state_in_progress",
		Name: stateInProgress,
	}
	steps[0].StartedOn = time.Now().UTC()
}

// pipelineIndex resolves the request's {ref} to where the run lives in the
// dataset, so a write can change it in place instead of a copy of it.
// findPipeline stays the answer for a read. The caller must hold dataMu, and
// must not append to the slice while it holds the index.
func (s *Server) pipelineIndex(r *http.Request) (string, int, bool) {
	if _, ok := s.findRepository(r); !ok {
		return "", 0, false
	}

	slug := r.PathValue("slug")
	ref := r.PathValue("ref")
	if buildNumber, err := strconv.Atoi(ref); err == nil {
		for index, pipeline := range s.Data.Pipelines[slug] {
			if pipeline.BuildNumber == buildNumber {
				return slug, index, true
			}
		}
		return "", 0, false
	}

	for index, pipeline := range s.Data.Pipelines[slug] {
		if sameUUID(pipeline.UUID, ref) {
			return slug, index, true
		}
	}
	return "", 0, false
}

// -----------------------------------------------------------------------------
// Stop
// -----------------------------------------------------------------------------

// handleStopPipeline signals a run to stop and answers 204 with no document,
// exactly as the real endpoint does. There is nothing asynchronous about it
// here: the run, its steps and its build status are all left in the state a
// stop leaves them in, so the next refresh shows STOPPED.
func (s *Server) handleStopPipeline(w http.ResponseWriter, r *http.Request) {
	s.dataMu.Lock()
	defer s.dataMu.Unlock()

	slug, index, ok := s.pipelineIndex(r)
	if !ok {
		writePipelineNotFound(w, r)
		return
	}

	pipeline := &s.Data.Pipelines[slug][index]
	if pipeline.State.IsCompleted() {
		writeBadRequest(w, msgPipelineCompleted)
		return
	}

	now := time.Now().UTC()
	pipeline.State = bitbucket.PipelineState{
		Type: "pipeline_state_completed",
		Name: stateCompleted,
		Result: &bitbucket.PipelineStateResult{
			Type:       "pipeline_state_completed_" + strings.ToLower(resultStopped),
			Name:       resultStopped,
			Terminator: ptr(s.Data.User),
		},
	}
	pipeline.CompletedOn = now
	// A run stops after it started, so it lasted at least a moment even when
	// the two stamps land in the same second.
	pipeline.DurationInSeconds = max(1, int(now.Sub(pipeline.CreatedOn).Seconds()))
	// A stopped run is finished, so nothing is left to promote.
	delete(s.pendingRuns, pipeline.UUID)

	stopSteps(s.Data.Steps[pipeline.UUID], now)
	s.stopStatus(pipeline.BuildNumber, now)

	w.WriteHeader(http.StatusNoContent)
}

// stopSteps is what a stop does to a run's steps: whatever was running was cut
// short, whatever had not started never will, and whatever had already finished
// keeps the result it earned.
func stopSteps(steps []bitbucket.PipelineStep, now time.Time) {
	for index := range steps {
		step := &steps[index]
		switch step.State.Name {
		case stateInProgress:
			step.State = bitbucket.PipelineStepState{
				Type: "pipeline_step_state_completed",
				Name: stateCompleted,
				Result: &bitbucket.PipelineStepResult{
					Type: "pipeline_step_state_completed_" + strings.ToLower(resultStopped),
					Name: resultStopped,
				},
			}
			step.CompletedOn = now
		case statePending:
			step.State = bitbucket.PipelineStepState{
				Type: "pipeline_step_state_completed",
				Name: stateCompleted,
				Result: &bitbucket.PipelineStepResult{
					Type: "pipeline_step_state_completed_not_run",
					Name: resultNotRun,
				},
			}
		default:
		}
	}
}

// stopStatus moves the build status that pointed at this run to STOPPED.
// Statuses are keyed by pull request and a stop is aimed at a run, so the only
// way from one to the other is the build number in the status URL — which is
// the same way the Builds panel gets from a status row to a run.
// The caller must hold dataMu for writing.
func (s *Server) stopStatus(buildNumber int, now time.Time) {
	for _, statuses := range s.Data.Statuses {
		for index := range statuses {
			status := &statuses[index]
			if number, ok := status.BuildNumber(); !ok || number != buildNumber {
				continue
			}
			status.State = bitbucket.StatusStopped
			status.Description = "Build " + strconv.Itoa(buildNumber) + " was stopped"
			status.UpdatedOn = now
		}
	}
}
