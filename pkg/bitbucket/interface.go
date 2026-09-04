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

// ClientInterface is the read-only Bitbucket Cloud API surface the TUI depends
// on. Every method takes workspace and repoSlug explicitly: the client is not
// bound to a workspace. There are deliberately no write methods.
type ClientInterface interface {
	GetCurrentUser(ctx context.Context) (*Account, error)
	GetWorkspace(ctx context.Context, workspace string) (*Workspace, error)
	ListRepositories(ctx context.Context, workspace string, opts ListRepositoriesOptions) ([]Repository, error)
	GetRepository(ctx context.Context, workspace, repoSlug string) (*Repository, error)
	ListPullRequests(ctx context.Context, workspace, repoSlug string, opts ListPullRequestsOptions) ([]PullRequest, error)
	GetPullRequest(ctx context.Context, workspace, repoSlug string, id int) (*PullRequest, error)
	ListPullRequestStatuses(ctx context.Context, workspace, repoSlug string, id int) ([]CommitStatus, error)
	ListPipelines(ctx context.Context, workspace, repoSlug string, opts ListPipelinesOptions) ([]Pipeline, error)
	GetPipeline(ctx context.Context, workspace, repoSlug, pipelineRef string) (*Pipeline, error)
	ListPipelineSteps(ctx context.Context, workspace, repoSlug, pipelineRef string) ([]PipelineStep, error)
	GetPipelineStepLog(ctx context.Context, workspace, repoSlug, pipelineRef, stepUUID string, opts StepLogOptions) (string, error)
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
