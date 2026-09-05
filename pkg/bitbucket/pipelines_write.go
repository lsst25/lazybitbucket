package bitbucket

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// ErrPipelineTarget is wrapped when a run's target cannot be turned into a
// request body. A rerun is assembled from the target the API answered with for
// the original run, so this means the payload is not one this package knows how
// to repeat — an unfamiliar target type, or a run missing a member its own type
// requires. There is nothing to send and nothing to retry, so it is reported
// before a request is built rather than left to the server's 400.
var ErrPipelineTarget = errors.New("pipeline target is incomplete")

// The three target shapes POST .../pipelines accepts, which PipelineTarget
// flattens on the way in. They discriminate an open union: a type this package
// does not know is refused with ErrPipelineTarget instead of being guessed at,
// because each type defines which members are mandatory and there is no safe
// default body.
const (
	targetRef         = "pipeline_ref_target"
	targetCommit      = "pipeline_commit_target"
	targetPullRequest = "pipeline_pullrequest_target"
)

// defaultRefType is the ref_type a ref target is sent with when the run being
// rerun carries none. ref_type is one of branch, tag, named_branch or bookmark
// and the endpoint wants it spelled out; a run that answered with none has
// always been a branch run.
const defaultRefType = "branch"

// commitRefType is the "type" member a commit reference carries inside a
// target. The endpoint's own documentation includes it on ref targets as well
// as commit and pull request ones, so it is always sent.
const commitRefType = "commit"

// stopPipelineSegment is the action path that signals a run to stop.
const stopPipelineSegment = "/stopPipeline"

// pipelineRunRequest is the body of POST .../pipelines: nothing but the target
// to run.
type pipelineRunRequest struct {
	Target pipelineTargetRequest `json:"target"`
}

// pipelineTargetRequest is a target as the run endpoint wants it, which is not
// the shape the read model has: PipelineTarget flattens all three unions and
// carries whatever the API answered with, while a request must carry exactly
// the members its own type defines. Everything optional is omitted rather than
// sent empty — the endpoint validates the members it finds, so an empty string
// is a different request from an absent one.
type pipelineTargetRequest struct {
	Type              string                   `json:"type"`
	RefType           string                   `json:"ref_type,omitempty"`
	RefName           string                   `json:"ref_name,omitempty"`
	Source            string                   `json:"source,omitempty"`
	Destination       string                   `json:"destination,omitempty"`
	DestinationCommit *commitRequest           `json:"destination_commit,omitempty"`
	Commit            *commitRequest           `json:"commit,omitempty"`
	PullRequest       *pullRequestIDRequest    `json:"pullrequest,omitempty"`
	Selector          *pipelineSelectorRequest `json:"selector,omitempty"`
}

// commitRequest is a commit reference on the wire: the discriminator and the
// hash. CommitRef cannot be reused here — its links member carries no omitempty
// and would serialize as a pair of empty objects the endpoint has no use for.
type commitRequest struct {
	Type string `json:"type"`
	Hash string `json:"hash"`
}

// pullRequestIDRequest is the pull request a pull-request target runs for. The
// endpoint's documentation quotes the id as a string while the read model
// carries a number and the server coerces either, so the number is sent as one.
type pullRequestIDRequest struct {
	ID int `json:"id"`
}

// pipelineSelectorRequest is the bitbucket-pipelines.yml selector that picked
// the original run. PipelineSelector cannot be reused: a "default" selector
// carries no pattern, and sending "pattern": "" asks for a pipeline named by
// the empty string rather than for the default one.
type pipelineSelectorRequest struct {
	Type    string `json:"type,omitempty"`
	Pattern string `json:"pattern,omitempty"`
}

// RunPipeline starts a new run for target — the target of an existing run, read
// from GET /pipelines/{ref} — and returns the run the server created (state
// PENDING, trigger MANUAL). It is how a run is rerun: same commit, same
// selector, same pull request.
//
// A repository without Pipelines enabled, or a revision with no
// bitbucket-pipelines.yml, answers 400; both arrive as an *APIError whose
// UserMessage is the sentence to show. Nothing is retried: starting a build is
// not idempotent.
func (c *Client) RunPipeline(
	ctx context.Context, workspace, repoSlug string, target PipelineTarget,
) (*Pipeline, error) {
	body, err := runTargetRequest(target)
	if err != nil {
		return nil, fmt.Errorf("run pipeline %s/%s: %w", workspace, repoSlug, err)
	}

	relPath := repoPath(workspace, repoSlug) + "/pipelines"

	var created Pipeline
	status, _, err := c.writeJSON(ctx, http.MethodPost, relPath, pipelineRunRequest{Target: body}, &created)
	if err != nil {
		return nil, fmt.Errorf("run pipeline %s/%s: %w", workspace, repoSlug, err)
	}
	// writeJSON decodes the body on a 200 or a 201, and the endpoint documents
	// 201. Any other 2xx — or one of those two without a uuid — would otherwise
	// hand back a zero Pipeline the caller then reports as a started build.
	if !decodedStatus(status) || created.UUID == "" {
		return nil, fmt.Errorf("run pipeline %s/%s: unexpected status %d without a pipeline body",
			workspace, repoSlug, status)
	}
	return &created, nil
}

// StopPipeline signals a running pipeline to stop. pipelineUUID is
// Pipeline.UUID (braced); pipelinePath percent-encodes the braces.
//
// The endpoint answers 204 and a 200 is tolerated: neither carries information,
// and the point of the call is the state it leaves behind, which the caller
// reads back with GetPipeline. A run that already completed is a 400 carrying
// the server's own sentence.
func (c *Client) StopPipeline(ctx context.Context, workspace, repoSlug, pipelineUUID string) error {
	relPath := pipelinePath(workspace, repoSlug, pipelineUUID) + stopPipelineSegment

	status, _, err := c.writeJSON(ctx, http.MethodPost, relPath, nil, nil)
	if err != nil {
		return fmt.Errorf("stop pipeline %s/%s %s: %w", workspace, repoSlug, pipelineUUID, err)
	}
	switch status {
	case http.StatusNoContent, http.StatusOK:
		return nil
	default:
		return fmt.Errorf("stop pipeline %s/%s %s: unexpected status %d",
			workspace, repoSlug, pipelineUUID, status)
	}
}

// runTargetRequest is the target member of the run body, or the error saying
// why target cannot be rerun. The per-type validation lives here rather than
// being left to the server: the API answers a bare 400 for a missing member,
// and the caller can say which member it was.
func runTargetRequest(target PipelineTarget) (pipelineTargetRequest, error) {
	request := pipelineTargetRequest{
		Type:              target.Type,
		RefType:           "",
		RefName:           "",
		Source:            "",
		Destination:       "",
		DestinationCommit: nil,
		Commit:            nil,
		PullRequest:       nil,
		Selector:          selectorRequest(target.Selector),
	}

	switch target.Type {
	case targetRef:
		// commit and selector are optional on a ref target: without a commit
		// the run picks up the ref's current head, which is what a rerun of a
		// run that named no commit did too.
		if target.RefName == "" {
			return pipelineTargetRequest{}, missingTargetMember(targetRef, "ref_name")
		}
		request.RefType = orString(target.RefType, defaultRefType)
		request.RefName = target.RefName
		request.Commit = commitRequestFor(target.Commit)
	case targetCommit:
		if target.Commit == nil || target.Commit.Hash == "" {
			return pipelineTargetRequest{}, missingTargetMember(targetCommit, "commit.hash")
		}
		request.Commit = commitRequestFor(target.Commit)
	case targetPullRequest:
		if err := validatePullRequestTarget(target); err != nil {
			return pipelineTargetRequest{}, err
		}
		request.Source = target.Source
		request.Destination = target.Destination
		request.DestinationCommit = commitRequestFor(target.DestinationCommit)
		request.Commit = commitRequestFor(target.Commit)
		request.PullRequest = &pullRequestIDRequest{ID: target.PullRequest.ID}
	default:
		return pipelineTargetRequest{}, fmt.Errorf("%w: unknown target type %q", ErrPipelineTarget, target.Type)
	}

	return request, nil
}

// validatePullRequestTarget checks the four members a pull request target
// cannot be rerun without. destination_commit and selector are not among them:
// the endpoint accepts a pull request target that carries neither.
func validatePullRequestTarget(target PipelineTarget) error {
	switch {
	case target.Source == "":
		return missingTargetMember(targetPullRequest, "source")
	case target.Destination == "":
		return missingTargetMember(targetPullRequest, "destination")
	case target.Commit == nil || target.Commit.Hash == "":
		return missingTargetMember(targetPullRequest, "commit.hash")
	case target.PullRequest == nil || target.PullRequest.ID == 0:
		return missingTargetMember(targetPullRequest, "pullrequest.id")
	default:
		return nil
	}
}

// missingTargetMember is the ErrPipelineTarget a target without a member its
// own type requires deserves. It names both, because "pipeline target is
// incomplete" on its own tells the user nothing about which run cannot be
// repeated or why.
func missingTargetMember(targetType, member string) error {
	return fmt.Errorf("%w: %s carries no %s", ErrPipelineTarget, targetType, member)
}

// commitRequestFor is the wire form of a commit reference, or nil when there is
// none: an absent commit is omitted rather than sent as an empty hash, which
// the endpoint would reject.
func commitRequestFor(ref *CommitRef) *commitRequest {
	if ref == nil || ref.Hash == "" {
		return nil
	}
	return &commitRequest{Type: commitRefType, Hash: ref.Hash}
}

// selectorRequest is the wire form of a selector, or nil when the run carried
// none worth repeating. An empty selector object would ask the server to match
// a pipeline with no type and no pattern.
func selectorRequest(selector *PipelineSelector) *pipelineSelectorRequest {
	if selector == nil || (selector.Type == "" && selector.Pattern == "") {
		return nil
	}
	return &pipelineSelectorRequest{Type: selector.Type, Pattern: selector.Pattern}
}
