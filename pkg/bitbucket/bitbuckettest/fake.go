// Package bitbuckettest provides a FakeClient that implements
// bitbucket.ClientInterface for use in unit tests. By default every method
// calls t.Fatalf, so unexpected calls surface immediately with a clear message.
// Tests opt-in by assigning a *Func field for each method they expect to be
// called.
//
// Unlike jiratest.FakeClient the fake is goroutine-safe: bitbucket.FetchPullRequests
// calls the client from several goroutines, so every method records its call
// under a mutex and dispatches the stub outside it. Read the XxxCalls slices
// only after the code under test has returned; use CallCount while goroutines
// may still be running.
package bitbuckettest

import (
	"context"
	"sync"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
)

// Call-record structs. One per method that takes arguments worth asserting.

// GetWorkspaceCall records one GetWorkspace call.
type GetWorkspaceCall struct {
	Ctx       context.Context
	Workspace string
}

// ListRepositoriesCall records one ListRepositories call.
type ListRepositoriesCall struct {
	Ctx       context.Context
	Workspace string
	Opts      bitbucket.ListRepositoriesOptions
}

// GetRepositoryCall records one GetRepository call.
type GetRepositoryCall struct {
	Ctx       context.Context
	Workspace string
	RepoSlug  string
}

// ListPullRequestsCall records one ListPullRequests call.
type ListPullRequestsCall struct {
	Ctx       context.Context
	Workspace string
	RepoSlug  string
	Opts      bitbucket.ListPullRequestsOptions
}

// GetPullRequestCall records one GetPullRequest call.
type GetPullRequestCall struct {
	Ctx       context.Context
	Workspace string
	RepoSlug  string
	ID        int
}

// ListPullRequestStatusesCall records one ListPullRequestStatuses call.
type ListPullRequestStatusesCall struct {
	Ctx       context.Context
	Workspace string
	RepoSlug  string
	ID        int
}

// ListPipelinesCall records one ListPipelines call.
type ListPipelinesCall struct {
	Ctx       context.Context
	Workspace string
	RepoSlug  string
	Opts      bitbucket.ListPipelinesOptions
}

// GetPipelineCall records one GetPipeline call.
type GetPipelineCall struct {
	Ctx         context.Context
	Workspace   string
	RepoSlug    string
	PipelineRef string
}

// ListPipelineStepsCall records one ListPipelineSteps call.
type ListPipelineStepsCall struct {
	Ctx         context.Context
	Workspace   string
	RepoSlug    string
	PipelineRef string
}

// GetPipelineStepLogCall records one GetPipelineStepLog call.
type GetPipelineStepLogCall struct {
	Ctx         context.Context
	Workspace   string
	RepoSlug    string
	PipelineRef string
	StepUUID    string
	Opts        bitbucket.StepLogOptions
}

// FakeClient is a strict-fail implementation of bitbucket.ClientInterface.
//
// Usage:
//
//	fake := &bitbuckettest.FakeClient{T: t}
//	fake.ListPullRequestsFunc = func(_ context.Context, _, repo string,
//		_ bitbucket.ListPullRequestsOptions) ([]bitbucket.PullRequest, error) {
//		return []bitbucket.PullRequest{{ID: 1, Title: repo}}, nil
//	}
//	// ... use fake as bitbucket.ClientInterface ...
//	if len(fake.ListPullRequestsCalls) != 1 || fake.ListPullRequestsCalls[0].RepoSlug != "api-gateway" {
//		t.Errorf("unexpected calls: %+v", fake.ListPullRequestsCalls)
//	}
type FakeClient struct {
	T *testing.T

	// Function-field overrides. Default (nil) -> t.Fatalf on call.
	GetCurrentUserFunc          func(ctx context.Context) (*bitbucket.Account, error)
	GetWorkspaceFunc            func(ctx context.Context, workspace string) (*bitbucket.Workspace, error)
	ListRepositoriesFunc        func(ctx context.Context, workspace string, opts bitbucket.ListRepositoriesOptions) ([]bitbucket.Repository, error)
	GetRepositoryFunc           func(ctx context.Context, workspace, repoSlug string) (*bitbucket.Repository, error)
	ListPullRequestsFunc        func(ctx context.Context, workspace, repoSlug string, opts bitbucket.ListPullRequestsOptions) ([]bitbucket.PullRequest, error)
	GetPullRequestFunc          func(ctx context.Context, workspace, repoSlug string, id int) (*bitbucket.PullRequest, error)
	ListPullRequestStatusesFunc func(ctx context.Context, workspace, repoSlug string, id int) ([]bitbucket.CommitStatus, error)
	ListPipelinesFunc           func(ctx context.Context, workspace, repoSlug string, opts bitbucket.ListPipelinesOptions) ([]bitbucket.Pipeline, error)
	GetPipelineFunc             func(ctx context.Context, workspace, repoSlug, pipelineRef string) (*bitbucket.Pipeline, error)
	ListPipelineStepsFunc       func(ctx context.Context, workspace, repoSlug, pipelineRef string) ([]bitbucket.PipelineStep, error)
	GetPipelineStepLogFunc      func(ctx context.Context, workspace, repoSlug, pipelineRef, stepUUID string, opts bitbucket.StepLogOptions) (string, error)
	SetOnRequestFunc            func(fn func(bitbucket.RequestLog))

	// Call recorders (populated before the *Func is invoked). Guarded by mu:
	// read them only after the code under test has returned, or use CallCount.
	GetCurrentUserCalls          []context.Context
	GetWorkspaceCalls            []GetWorkspaceCall
	ListRepositoriesCalls        []ListRepositoriesCall
	GetRepositoryCalls           []GetRepositoryCall
	ListPullRequestsCalls        []ListPullRequestsCall
	GetPullRequestCalls          []GetPullRequestCall
	ListPullRequestStatusesCalls []ListPullRequestStatusesCall
	ListPipelinesCalls           []ListPipelinesCall
	GetPipelineCalls             []GetPipelineCall
	ListPipelineStepsCalls       []ListPipelineStepsCall
	GetPipelineStepLogCalls      []GetPipelineStepLogCall
	SetOnRequestCalls            int

	// mu guards every recorder field and every *Func read. Stubs are always
	// dispatched outside the lock so they may block or call back into the fake.
	mu sync.Mutex
}

func (f *FakeClient) fatal(name string) {
	f.T.Helper()
	f.T.Fatalf("bitbuckettest.FakeClient: unexpected call to %s (no *Func configured)", name)
}

// CallCount returns how many times the named interface method has been called.
// It takes the same lock the recorders are written under, so it is safe to call
// while the code under test still has goroutines in flight (the concurrency
// bound of bitbucket.FetchPullRequests is asserted this way). An unknown method
// name returns -1 rather than a plausible 0, so a typo shows up as a failure.
func (f *FakeClient) CallCount(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch method {
	case "GetCurrentUser":
		return len(f.GetCurrentUserCalls)
	case "GetWorkspace":
		return len(f.GetWorkspaceCalls)
	case "ListRepositories":
		return len(f.ListRepositoriesCalls)
	case "GetRepository":
		return len(f.GetRepositoryCalls)
	case "ListPullRequests":
		return len(f.ListPullRequestsCalls)
	case "GetPullRequest":
		return len(f.GetPullRequestCalls)
	case "ListPullRequestStatuses":
		return len(f.ListPullRequestStatusesCalls)
	case "ListPipelines":
		return len(f.ListPipelinesCalls)
	case "GetPipeline":
		return len(f.GetPipelineCalls)
	case "ListPipelineSteps":
		return len(f.ListPipelineStepsCalls)
	case "GetPipelineStepLog":
		return len(f.GetPipelineStepLogCalls)
	case "SetOnRequest":
		return f.SetOnRequestCalls
	default:
		return -1
	}
}

// --- bitbucket.ClientInterface implementation ---

// GetCurrentUser records the call and dispatches GetCurrentUserFunc.
func (f *FakeClient) GetCurrentUser(ctx context.Context) (*bitbucket.Account, error) {
	f.mu.Lock()
	f.GetCurrentUserCalls = append(f.GetCurrentUserCalls, ctx)
	fn := f.GetCurrentUserFunc
	f.mu.Unlock()

	if fn == nil {
		f.fatal("GetCurrentUser")
		return nil, nil
	}
	return fn(ctx)
}

// GetWorkspace records the call and dispatches GetWorkspaceFunc.
func (f *FakeClient) GetWorkspace(ctx context.Context, workspace string) (*bitbucket.Workspace, error) {
	f.mu.Lock()
	f.GetWorkspaceCalls = append(f.GetWorkspaceCalls, GetWorkspaceCall{Ctx: ctx, Workspace: workspace})
	fn := f.GetWorkspaceFunc
	f.mu.Unlock()

	if fn == nil {
		f.fatal("GetWorkspace")
		return nil, nil
	}
	return fn(ctx, workspace)
}

// ListRepositories records the call and dispatches ListRepositoriesFunc.
func (f *FakeClient) ListRepositories(ctx context.Context, workspace string, opts bitbucket.ListRepositoriesOptions) ([]bitbucket.Repository, error) {
	f.mu.Lock()
	f.ListRepositoriesCalls = append(f.ListRepositoriesCalls, ListRepositoriesCall{Ctx: ctx, Workspace: workspace, Opts: opts})
	fn := f.ListRepositoriesFunc
	f.mu.Unlock()

	if fn == nil {
		f.fatal("ListRepositories")
		return nil, nil
	}
	return fn(ctx, workspace, opts)
}

// GetRepository records the call and dispatches GetRepositoryFunc.
func (f *FakeClient) GetRepository(ctx context.Context, workspace, repoSlug string) (*bitbucket.Repository, error) {
	f.mu.Lock()
	f.GetRepositoryCalls = append(f.GetRepositoryCalls, GetRepositoryCall{Ctx: ctx, Workspace: workspace, RepoSlug: repoSlug})
	fn := f.GetRepositoryFunc
	f.mu.Unlock()

	if fn == nil {
		f.fatal("GetRepository")
		return nil, nil
	}
	return fn(ctx, workspace, repoSlug)
}

// ListPullRequests records the call and dispatches ListPullRequestsFunc.
func (f *FakeClient) ListPullRequests(ctx context.Context, workspace, repoSlug string, opts bitbucket.ListPullRequestsOptions) ([]bitbucket.PullRequest, error) {
	f.mu.Lock()
	f.ListPullRequestsCalls = append(f.ListPullRequestsCalls, ListPullRequestsCall{Ctx: ctx, Workspace: workspace, RepoSlug: repoSlug, Opts: opts})
	fn := f.ListPullRequestsFunc
	f.mu.Unlock()

	if fn == nil {
		f.fatal("ListPullRequests")
		return nil, nil
	}
	return fn(ctx, workspace, repoSlug, opts)
}

// GetPullRequest records the call and dispatches GetPullRequestFunc.
func (f *FakeClient) GetPullRequest(ctx context.Context, workspace, repoSlug string, id int) (*bitbucket.PullRequest, error) {
	f.mu.Lock()
	f.GetPullRequestCalls = append(f.GetPullRequestCalls, GetPullRequestCall{Ctx: ctx, Workspace: workspace, RepoSlug: repoSlug, ID: id})
	fn := f.GetPullRequestFunc
	f.mu.Unlock()

	if fn == nil {
		f.fatal("GetPullRequest")
		return nil, nil
	}
	return fn(ctx, workspace, repoSlug, id)
}

// ListPullRequestStatuses records the call and dispatches ListPullRequestStatusesFunc.
func (f *FakeClient) ListPullRequestStatuses(ctx context.Context, workspace, repoSlug string, id int) ([]bitbucket.CommitStatus, error) {
	f.mu.Lock()
	f.ListPullRequestStatusesCalls = append(f.ListPullRequestStatusesCalls,
		ListPullRequestStatusesCall{Ctx: ctx, Workspace: workspace, RepoSlug: repoSlug, ID: id})
	fn := f.ListPullRequestStatusesFunc
	f.mu.Unlock()

	if fn == nil {
		f.fatal("ListPullRequestStatuses")
		return nil, nil
	}
	return fn(ctx, workspace, repoSlug, id)
}

// ListPipelines records the call and dispatches ListPipelinesFunc.
func (f *FakeClient) ListPipelines(ctx context.Context, workspace, repoSlug string, opts bitbucket.ListPipelinesOptions) ([]bitbucket.Pipeline, error) {
	f.mu.Lock()
	f.ListPipelinesCalls = append(f.ListPipelinesCalls, ListPipelinesCall{Ctx: ctx, Workspace: workspace, RepoSlug: repoSlug, Opts: opts})
	fn := f.ListPipelinesFunc
	f.mu.Unlock()

	if fn == nil {
		f.fatal("ListPipelines")
		return nil, nil
	}
	return fn(ctx, workspace, repoSlug, opts)
}

// GetPipeline records the call and dispatches GetPipelineFunc.
func (f *FakeClient) GetPipeline(ctx context.Context, workspace, repoSlug, pipelineRef string) (*bitbucket.Pipeline, error) {
	f.mu.Lock()
	f.GetPipelineCalls = append(f.GetPipelineCalls, GetPipelineCall{Ctx: ctx, Workspace: workspace, RepoSlug: repoSlug, PipelineRef: pipelineRef})
	fn := f.GetPipelineFunc
	f.mu.Unlock()

	if fn == nil {
		f.fatal("GetPipeline")
		return nil, nil
	}
	return fn(ctx, workspace, repoSlug, pipelineRef)
}

// ListPipelineSteps records the call and dispatches ListPipelineStepsFunc.
func (f *FakeClient) ListPipelineSteps(ctx context.Context, workspace, repoSlug, pipelineRef string) ([]bitbucket.PipelineStep, error) {
	f.mu.Lock()
	f.ListPipelineStepsCalls = append(f.ListPipelineStepsCalls,
		ListPipelineStepsCall{Ctx: ctx, Workspace: workspace, RepoSlug: repoSlug, PipelineRef: pipelineRef})
	fn := f.ListPipelineStepsFunc
	f.mu.Unlock()

	if fn == nil {
		f.fatal("ListPipelineSteps")
		return nil, nil
	}
	return fn(ctx, workspace, repoSlug, pipelineRef)
}

// GetPipelineStepLog records the call and dispatches GetPipelineStepLogFunc.
func (f *FakeClient) GetPipelineStepLog(ctx context.Context, workspace, repoSlug, pipelineRef, stepUUID string, opts bitbucket.StepLogOptions) (string, error) {
	f.mu.Lock()
	f.GetPipelineStepLogCalls = append(f.GetPipelineStepLogCalls, GetPipelineStepLogCall{
		Ctx:         ctx,
		Workspace:   workspace,
		RepoSlug:    repoSlug,
		PipelineRef: pipelineRef,
		StepUUID:    stepUUID,
		Opts:        opts,
	})
	fn := f.GetPipelineStepLogFunc
	f.mu.Unlock()

	if fn == nil {
		f.fatal("GetPipelineStepLog")
		return "", nil
	}
	return fn(ctx, workspace, repoSlug, pipelineRef, stepUUID, opts)
}

// SetOnRequest records the call and dispatches SetOnRequestFunc.
func (f *FakeClient) SetOnRequest(fn func(bitbucket.RequestLog)) {
	f.mu.Lock()
	f.SetOnRequestCalls++
	stub := f.SetOnRequestFunc
	f.mu.Unlock()

	if stub == nil {
		f.fatal("SetOnRequest")
		return
	}
	stub(fn)
}

var _ bitbucket.ClientInterface = (*FakeClient)(nil)
