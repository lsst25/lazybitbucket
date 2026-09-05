package bitbucket

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// MergeStrategy is how Bitbucket builds the merge commit. It is an open string
// enum: the six documented values are constants, and an unrecognised one is
// passed through rather than rejected locally, so a strategy the API learns
// tomorrow still works.
type MergeStrategy string

// The merge strategies POST .../merge accepts. The zero value "" is not one of
// them: it means "omit merge_strategy" and lets the repository's own default
// decide.
const (
	// MergeCommit always creates a merge commit. It is Bitbucket's default.
	MergeCommit MergeStrategy = "merge_commit"
	// MergeSquash replaces the branch's commits with a single one.
	MergeSquash MergeStrategy = "squash"
	// MergeFastForward moves the destination branch, and fails when the branch
	// is not already ahead of it.
	MergeFastForward MergeStrategy = "fast_forward"
	// MergeSquashFastForward squashes, then fast-forwards.
	MergeSquashFastForward MergeStrategy = "squash_fast_forward"
	// MergeRebaseFastForward rebases onto the destination, then fast-forwards.
	MergeRebaseFastForward MergeStrategy = "rebase_fast_forward"
	// MergeRebaseMerge rebases onto the destination, then creates a merge commit.
	MergeRebaseMerge MergeStrategy = "rebase_merge"
)

// AllMergeStrategies is the six documented strategies in the order the merge
// dialog offers them: the common ones first, the rebase variants last.
func AllMergeStrategies() []MergeStrategy {
	return []MergeStrategy{
		MergeCommit,
		MergeSquash,
		MergeFastForward,
		MergeSquashFastForward,
		MergeRebaseFastForward,
		MergeRebaseMerge,
	}
}

// Label is the strategy written for a human: "merge commit" rather than
// "merge_commit". A strategy this package does not know is shown verbatim.
func (s MergeStrategy) Label() string {
	switch s {
	case MergeCommit:
		return "merge commit"
	case MergeSquash:
		return "squash"
	case MergeFastForward:
		return "fast-forward"
	case MergeSquashFastForward:
		return "squash fast-forward"
	case MergeRebaseFastForward:
		return "rebase fast-forward"
	case MergeRebaseMerge:
		return "rebase merge"
	default:
		return string(s)
	}
}

// MergeOptions is the body of POST .../merge. Every member is optional: an
// empty MergeOptions merges with the repository's defaults.
type MergeOptions struct {
	// Strategy is merge_strategy; "" omits it and lets the server choose.
	Strategy MergeStrategy
	// CloseSourceBranch deletes the source branch after the merge. nil omits
	// the member, which makes the server fall back to the pull request's own
	// close_source_branch flag.
	CloseSourceBranch *bool
	// Message is the commit message; "" omits it and lets Bitbucket compose one.
	Message string
}

// MergeResult is what a merge request answered with. Exactly one member is set:
// PullRequest for a 200 (the merge completed and this is the merged pull
// request), TaskURL for a 202 (the merge was queued and this is the URL
// GetMergeTaskStatus polls).
type MergeResult struct {
	PullRequest *PullRequest
	TaskURL     string
}

// Queued reports whether the merge was accepted for asynchronous processing and
// has to be polled with GetMergeTaskStatus.
func (r MergeResult) Queued() bool { return r.TaskURL != "" }

// MergeTaskStatus is the answer of the merge task-status endpoint. Status is an
// open string enum; MergeResult is nil until it reads SUCCESS.
type MergeTaskStatus struct {
	Status      string       `json:"task_status"`
	MergeResult *PullRequest `json:"merge_result"`
}

// The task_status values Bitbucket documents. A merge that failed answers with
// an error envelope instead of a third status, which GetMergeTaskStatus turns
// into an *APIError.
const (
	MergeTaskPending = "PENDING"
	MergeTaskSuccess = "SUCCESS"
)

// mergeParametersType is the "type" member Bitbucket's merge body carries.
const mergeParametersType = "pullrequest_merge_parameters"

// mergeSegment is the path segment of the merge endpoint.
const mergeSegment = "/merge"

// mergeAsyncQuery asks Bitbucket to run the merge asynchronously. Without it
// the endpoint blocks until the merge is done and answers 555 when that takes
// longer than its gateway allows — leaving the caller unable to tell whether
// the merge landed. With it the answer is a 202 whose Location is polled with
// GetMergeTaskStatus (a merge that finishes at once still answers 202).
const mergeAsyncQuery = "?async=true"

// mergeParameters is the wire shape of the merge body. The optional members are
// omitted when unset so the server applies its own defaults rather than being
// told "no strategy" or "do not close the branch".
type mergeParameters struct {
	Type              string        `json:"type"`
	MergeStrategy     MergeStrategy `json:"merge_strategy,omitempty"`
	CloseSourceBranch *bool         `json:"close_source_branch,omitempty"`
	Message           string        `json:"message,omitempty"`
}

// errMergeTaskNoLocation is a 202 that named no task to poll.
var errMergeTaskNoLocation = errors.New("merge was queued but the response carried no Location header")

// MergePullRequest merges the pull request and returns either the merged pull
// request or, when Bitbucket queued the merge, the task URL to poll.
//
// Bitbucket decides for itself whether to answer synchronously: a merge it can
// finish quickly answers 200 with the merged pull request, and one it cannot
// answers 202 with a Location naming a task — MergeResult.Queued reports which,
// and GetMergeTaskStatus follows up on the second.
//
// A 409 means the refs moved while the merge was running (refresh and retry);
// a 555 means the merge ran past the gateway's deadline and may still complete,
// so the answer is to refresh rather than to merge again. Nothing here is
// retried automatically: a merge is not idempotent.
func (c *Client) MergePullRequest(
	ctx context.Context, workspace, repoSlug string, id int, opts MergeOptions,
) (MergeResult, error) {
	relPath := pullRequestPath(workspace, repoSlug, id) + mergeSegment + mergeAsyncQuery
	params := mergeParameters{
		Type:              mergeParametersType,
		MergeStrategy:     opts.Strategy,
		CloseSourceBranch: opts.CloseSourceBranch,
		Message:           opts.Message,
	}

	var merged PullRequest
	status, header, err := c.writeJSON(ctx, http.MethodPost, relPath, params, &merged)
	if err != nil {
		return MergeResult{PullRequest: nil, TaskURL: ""},
			fmt.Errorf("merge %s/%s#%d: %w", workspace, repoSlug, id, err)
	}

	switch status {
	case http.StatusOK:
		return MergeResult{PullRequest: &merged, TaskURL: ""}, nil
	case http.StatusAccepted:
		taskURL, err := c.mergeTaskURL(c.baseURL+relPath, header.Get("Location"))
		if err != nil {
			return MergeResult{PullRequest: nil, TaskURL: ""},
				fmt.Errorf("merge %s/%s#%d: %w", workspace, repoSlug, id, err)
		}
		return MergeResult{PullRequest: nil, TaskURL: taskURL}, nil
	default:
		return MergeResult{PullRequest: nil, TaskURL: ""},
			fmt.Errorf("merge %s/%s#%d: unexpected status %d", workspace, repoSlug, id, status)
	}
}

// mergeTaskURL resolves the 202's Location — relative or absolute — against the
// merge request's own URL and refuses a target off the API host. The poll
// carries the Authorization header, so the response does not get to choose
// where it goes.
func (c *Client) mergeTaskURL(requestURL, location string) (string, error) {
	if location == "" {
		return "", errMergeTaskNoLocation
	}

	requested, err := url.Parse(requestURL)
	if err != nil {
		return "", fmt.Errorf("parse merge URL %q: %w", requestURL, err)
	}
	target, err := requested.Parse(location)
	if err != nil {
		return "", fmt.Errorf("parse merge task link %q: %w", location, err)
	}
	if host, ok := c.sameOrigin(target); !ok {
		return "", fmt.Errorf("refusing to follow merge task link to %s (expected host %s)", target.Host, host)
	}
	return target.String(), nil
}

// GetMergeTaskStatus polls a queued merge. taskURL is MergeResult.TaskURL; it
// must be on the API host, which is re-checked here because the value has made
// a round trip through the caller since MergePullRequest vetted it.
//
// The endpoint reports a failed merge as a 200 carrying Bitbucket's error
// envelope rather than as an error status, so a body that decodes as one is
// returned as an *APIError with the message the server gave.
func (c *Client) GetMergeTaskStatus(ctx context.Context, taskURL string) (MergeTaskStatus, error) {
	empty := MergeTaskStatus{Status: "", MergeResult: nil}

	target, err := url.Parse(taskURL)
	if err != nil {
		return empty, fmt.Errorf("get merge task status: parse %q: %w", taskURL, err)
	}
	if host, ok := c.sameOrigin(target); !ok {
		return empty, fmt.Errorf("get merge task status: refusing to fetch %s (expected host %s)", target.Host, host)
	}

	relPath := strings.TrimPrefix(taskURL, c.baseURL)
	body, status, err := c.request(ctx, taskURL, relPath, acceptJSON, nil)
	if err != nil {
		return empty, fmt.Errorf("get merge task status: %w", err)
	}

	if apiErr := errorEnvelopeError(http.MethodGet, pathWithoutQuery(relPath), status, body); apiErr != nil {
		apiErr.AuthMethod = c.authMethod
		return empty, fmt.Errorf("get merge task status: %w", apiErr)
	}

	var task MergeTaskStatus
	if err := unmarshalBody(http.MethodGet, relPath, body, &task); err != nil {
		return empty, fmt.Errorf("get merge task status: %w", err)
	}
	return task, nil
}
