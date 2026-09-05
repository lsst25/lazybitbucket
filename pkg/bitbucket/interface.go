package bitbucket

import (
	"context"
	"time"
)

// DefaultBaseURL is the Bitbucket Cloud REST API v2 root.
const DefaultBaseURL = "https://api.bitbucket.org/2.0"

// AuthMethod is how the client authenticates. App passwords were removed by
// Atlassian on 2026-07-28, so there is no app-password mode: the two supported
// credential shapes are a bare token (Bearer) and an Atlassian account email
// plus API token (Basic).
type AuthMethod string

// Supported authentication methods.
const (
	// AuthBearer sends "Authorization: Bearer <token>" — API tokens and
	// repository/project/workspace access tokens.
	AuthBearer AuthMethod = "bearer"
	// AuthBasic sends "Authorization: Basic base64(<atlassian account email>:<api token>)".
	// The username MUST be the Atlassian account email; a Bitbucket username or
	// the token's name yields 401.
	AuthBasic AuthMethod = "basic"
	// AuthNone sends no Authorization header.
	AuthNone AuthMethod = "none"
)

// RequestLog is one completed HTTP request, handed to the hook installed with
// SetOnRequest. It is reported for error statuses too.
type RequestLog struct {
	Method string
	// Path is the path plus query relative to the base URL, e.g.
	// "/repositories/ws/repo/pullrequests?state=OPEN".
	Path    string
	Status  int
	Elapsed time.Duration
	// RateLimitRemaining is X-RateLimit-Remaining when present, else -1.
	RateLimitRemaining int
}

// ClientInterface is the Bitbucket Cloud API surface the TUI depends on. Every
// method takes workspace and repoSlug explicitly: the client is not bound to a
// workspace.
//
// Most of it is reads. The write methods are exactly six — MergePullRequest,
// UpdatePullRequest, ApprovePullRequest, UnapprovePullRequest, RunPipeline and
// StopPipeline — and every one of them fails with ErrReadOnly, before a request
// is built, on a client constructed with ClientOpts.ReadOnly. GetMergeTaskStatus
// (the poll a queued merge needs) and the two candidate listings the reviewer
// picker feeds on are GETs that only a write flow ever issues. Requesting
// changes, declining and commenting are still deliberately absent, as is
// starting an arbitrary custom pipeline: RunPipeline only repeats the target of
// a run that already exists.
type ClientInterface interface {
	GetCurrentUser(ctx context.Context) (*Account, error)
	GetWorkspace(ctx context.Context, workspace string) (*Workspace, error)
	ListRepositories(ctx context.Context, workspace string, opts ListRepositoriesOptions) ([]Repository, error)
	GetRepository(ctx context.Context, workspace, repoSlug string) (*Repository, error)
	ListPullRequests(ctx context.Context, workspace, repoSlug string, opts ListPullRequestsOptions) ([]PullRequest, error)
	GetPullRequest(ctx context.Context, workspace, repoSlug string, id int) (*PullRequest, error)
	ListPullRequestStatuses(ctx context.Context, workspace, repoSlug string, id int) ([]CommitStatus, error)
	// ListPullRequestCommits returns the commits newest first: that is the
	// order the endpoint answers in, and it is passed on unchanged.
	ListPullRequestCommits(ctx context.Context, workspace, repoSlug string, id int, opts ListOptions) ([]Commit, error)
	ListPullRequestComments(ctx context.Context, workspace, repoSlug string, id int, opts ListOptions) ([]Comment, error)
	ListPullRequestDiffStat(ctx context.Context, workspace, repoSlug string, id int) ([]DiffStatEntry, error)
	GetPullRequestDiff(ctx context.Context, workspace, repoSlug string, id int, opts DiffOptions) (string, error)
	ListPipelines(ctx context.Context, workspace, repoSlug string, opts ListPipelinesOptions) ([]Pipeline, error)
	GetPipeline(ctx context.Context, workspace, repoSlug, pipelineRef string) (*Pipeline, error)
	ListPipelineSteps(ctx context.Context, workspace, repoSlug, pipelineRef string) ([]PipelineStep, error)
	GetPipelineStepLog(ctx context.Context, workspace, repoSlug, pipelineRef, stepUUID string, opts StepLogOptions) (string, error)
	// MergePullRequest merges the pull request. Bitbucket answers either with
	// the merged pull request or, for a merge it queued, a task URL to poll.
	MergePullRequest(ctx context.Context, workspace, repoSlug string, id int, opts MergeOptions) (MergeResult, error)
	// GetMergeTaskStatus polls a queued merge. taskURL is MergeResult.TaskURL
	// and must be on the API host.
	GetMergeTaskStatus(ctx context.Context, taskURL string) (MergeTaskStatus, error)
	// UpdatePullRequest changes title, description, draft state and reviewers,
	// and returns the pull request as the server left it. The update must carry
	// the complete reviewer set — see PullRequestUpdate.Reviewers.
	UpdatePullRequest(ctx context.Context, workspace, repoSlug string, id int, update PullRequestUpdate) (*PullRequest, error)
	// ApprovePullRequest records the credentials' own approval and returns the
	// participant row the server built for it. Approving one's own pull request
	// is a 400.
	ApprovePullRequest(ctx context.Context, workspace, repoSlug string, id int) (*Participant, error)
	// UnapprovePullRequest withdraws the credentials' own approval. The endpoint
	// answers 204, so there is nothing to return but the failure.
	UnapprovePullRequest(ctx context.Context, workspace, repoSlug string, id int) error
	// RunPipeline starts a new run for target — the target of an existing run —
	// and returns the run the server created, PENDING and triggered MANUAL. It
	// is how a run is rerun; a target it cannot repeat is refused locally with
	// ErrPipelineTarget.
	RunPipeline(ctx context.Context, workspace, repoSlug string, target PipelineTarget) (*Pipeline, error)
	// StopPipeline signals a running pipeline to stop. pipelineUUID is
	// Pipeline.UUID (braced); a run that already completed answers 400.
	StopPipeline(ctx context.Context, workspace, repoSlug, pipelineUUID string) error
	// ListEffectiveDefaultReviewers returns the repository's default reviewers,
	// project-inherited ones included. A 403 here is not fatal to the caller.
	ListEffectiveDefaultReviewers(ctx context.Context, workspace, repoSlug string) ([]Account, error)
	// ListWorkspaceMembers returns the workspace's accounts, bounded by
	// opts.MaxItems (0 means 500).
	ListWorkspaceMembers(ctx context.Context, workspace string, opts ListOptions) ([]Account, error)
	// SetOnRequest installs the request hook. Implementations may call it from
	// several goroutines concurrently (FetchPullRequests fans out), so the hook
	// must be goroutine-safe.
	SetOnRequest(fn func(RequestLog))
}

// ListRepositoriesOptions narrows GET /repositories/{workspace}.
type ListRepositoriesOptions struct {
	Role     string // "", "member", "contributor", "admin", "owner"
	Query    string // raw q= expression, optional
	Sort     string // "" → "-updated_on"
	MaxItems int    // 0 → 100; stops following next once reached
}

// ListPullRequestsOptions narrows GET .../pullrequests.
type ListPullRequestsOptions struct {
	States   []PullRequestState // empty → OPEN (API default); repeated state= params otherwise
	Query    string             // raw q= expression, optional
	Sort     string             // "" → "-updated_on"
	MaxItems int                // 0 → 50; stops following next once reached
	// Fields defaults to "+values.reviewers,+values.participants" so list items
	// carry review data. Set it to "-" to send no fields parameter.
	Fields string
}

// ListOptions bounds a plain paginated sub-resource of a pull request — the
// commit list and the comment list, neither of which has anything to filter on.
type ListOptions struct {
	MaxItems int // 0 → the method's own default; stops following next once reached
}

// DiffOptions narrows GET .../pullrequests/{id}/diff.
type DiffOptions struct {
	// Path limits the diff to one file; "" is the whole pull request diff. It
	// is the API's escaped_path, i.e. DiffStatEntry.EscapedPath.
	Path string
	// Context is the number of context lines around each hunk; 0 means the
	// API default of three.
	Context int
	// DiffURL is an optional pre-resolved diff link — PullRequest.Links.Diff.Href
	// or the Location the /diff endpoint redirected to. When it is set and
	// shares scheme and host with the client's BaseURL it is fetched instead of
	// the /pullrequests/{id}/diff path, which keeps whatever undocumented query
	// parameters (from_pullrequest_id, exclude_files) the API attached to it. A
	// foreign-host link is ignored rather than followed, so credentials never
	// reach a host the API chose.
	DiffURL string
}

// ListPipelinesOptions narrows GET .../pipelines.
type ListPipelinesOptions struct {
	TargetBranch string // target.branch= (matches ref_name for branch runs, source for PR runs)
	Sort         string // "" → "-created_on"
	MaxItems     int    // 0 → 10
}

// StepLogOptions narrows GET .../steps/{uuid}/log.
type StepLogOptions struct {
	TailBytes int // >0 → Range: bytes=-N; 0 → whole log
}
