package bitbuckettest

import (
	"context"
	"sync"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

const (
	testWorkspace = "acme"
	testRepo      = "api-gateway"
)

func TestFakeClient_RecordsCallBeforeDispatch(t *testing.T) {
	t.Parallel()

	fake := &FakeClient{T: t}
	fake.ListPullRequestsFunc = func(_ context.Context, _, repoSlug string,
		_ bitbucket.ListPullRequestsOptions,
	) ([]bitbucket.PullRequest, error) {
		// The call must already be recorded when the stub runs.
		testkit.AssertEqual(t, "calls during dispatch", len(fake.ListPullRequestsCalls), 1)
		return []bitbucket.PullRequest{{ID: 7, Title: repoSlug}}, nil
	}

	opts := bitbucket.ListPullRequestsOptions{
		States:   []bitbucket.PullRequestState{bitbucket.PullRequestOpen},
		MaxItems: 5,
	}
	prs, err := fake.ListPullRequests(t.Context(), testWorkspace, testRepo, opts)
	if err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}

	testkit.AssertEqual(t, "pull requests", len(prs), 1)
	testkit.AssertEqual(t, "pull request title", prs[0].Title, testRepo)
	testkit.AssertEqual(t, "recorded calls", len(fake.ListPullRequestsCalls), 1)

	call := fake.ListPullRequestsCalls[0]
	testkit.AssertEqual(t, "workspace", call.Workspace, testWorkspace)
	testkit.AssertEqual(t, "repo slug", call.RepoSlug, testRepo)
	testkit.AssertEqual(t, "max items", call.Opts.MaxItems, 5)
	if call.Ctx == nil {
		t.Error("call context is nil")
	}
}

func TestFakeClient_SetOnRequestCounts(t *testing.T) {
	t.Parallel()

	var installed int
	fake := &FakeClient{T: t}
	fake.SetOnRequestFunc = func(fn func(bitbucket.RequestLog)) {
		if fn == nil {
			t.Error("SetOnRequest received a nil hook")
		}
		installed++
	}

	fake.SetOnRequest(func(bitbucket.RequestLog) {})
	fake.SetOnRequest(func(bitbucket.RequestLog) {})

	testkit.AssertEqual(t, "SetOnRequestCalls", fake.SetOnRequestCalls, 2)
	testkit.AssertEqual(t, "installed hooks", installed, 2)
	testkit.AssertEqual(t, "CallCount", fake.CallCount("SetOnRequest"), 2)
}

func TestFakeClient_CallCount(t *testing.T) {
	t.Parallel()

	fake := &FakeClient{T: t}
	fake.GetCurrentUserFunc = func(context.Context) (*bitbucket.Account, error) {
		return &bitbucket.Account{DisplayName: "Demo User"}, nil
	}
	fake.GetPipelineStepLogFunc = func(_ context.Context, _, _, _, _ string,
		_ bitbucket.StepLogOptions,
	) (string, error) {
		return "log", nil
	}

	if _, err := fake.GetCurrentUser(t.Context()); err != nil {
		t.Fatalf("GetCurrentUser: %v", err)
	}
	if _, err := fake.GetPipelineStepLog(t.Context(), testWorkspace, testRepo, "{uuid}", "{step}",
		bitbucket.StepLogOptions{TailBytes: 16}); err != nil {
		t.Fatalf("GetPipelineStepLog: %v", err)
	}

	testkit.AssertEqual(t, "GetCurrentUser", fake.CallCount("GetCurrentUser"), 1)
	testkit.AssertEqual(t, "GetPipelineStepLog", fake.CallCount("GetPipelineStepLog"), 1)
	testkit.AssertEqual(t, "GetPullRequest", fake.CallCount("GetPullRequest"), 0)
	testkit.AssertEqual(t, "unknown method", fake.CallCount("NoSuchMethod"), -1)
	testkit.AssertEqual(t, "step log tail", fake.GetPipelineStepLogCalls[0].Opts.TailBytes, 16)
}

func TestFakeClient_ConcurrentCallsAreRecorded(t *testing.T) {
	t.Parallel()

	const goroutines = 8

	release := make(chan struct{})
	var inFlight sync.WaitGroup
	inFlight.Add(goroutines)

	fake := &FakeClient{T: t}
	fake.ListPullRequestsFunc = func(_ context.Context, _, _ string,
		_ bitbucket.ListPullRequestsOptions,
	) ([]bitbucket.PullRequest, error) {
		// Dispatch happens outside the fake's lock, so every stub can block
		// here at once without deadlocking the recorders.
		inFlight.Done()
		<-release
		return nil, nil
	}

	var done sync.WaitGroup
	for range goroutines {
		done.Go(func() {
			_, _ = fake.ListPullRequests(t.Context(), testWorkspace, testRepo,
				bitbucket.ListPullRequestsOptions{})
		})
	}

	inFlight.Wait()
	// Readable while the goroutines are still parked inside the stub.
	testkit.AssertEqual(t, "in-flight call count", fake.CallCount("ListPullRequests"), goroutines)
	close(release)
	done.Wait()

	testkit.AssertEqual(t, "recorded calls", len(fake.ListPullRequestsCalls), goroutines)
}
