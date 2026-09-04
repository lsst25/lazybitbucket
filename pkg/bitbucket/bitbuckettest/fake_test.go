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

func TestFakeClient_ReviewMethodsRecordTheirArguments(t *testing.T) {
	t.Parallel()

	const prID = 103

	fake := &FakeClient{T: t}
	fake.ListPullRequestCommitsFunc = func(_ context.Context, _, _ string, _ int,
		_ bitbucket.ListOptions,
	) ([]bitbucket.Commit, error) {
		return []bitbucket.Commit{{Hash: "06fd23caff6b0a76ce2f42c983988394dca1f17f"}}, nil
	}
	fake.ListPullRequestCommentsFunc = func(_ context.Context, _, _ string, _ int,
		_ bitbucket.ListOptions,
	) ([]bitbucket.Comment, error) {
		return []bitbucket.Comment{{ID: 822302793}}, nil
	}
	fake.ListPullRequestDiffStatFunc = func(_ context.Context, _, _ string, _ int,
	) ([]bitbucket.DiffStatEntry, error) {
		return []bitbucket.DiffStatEntry{{Status: bitbucket.DiffStatModified}}, nil
	}
	fake.GetPullRequestDiffFunc = func(_ context.Context, _, _ string, _ int,
		_ bitbucket.DiffOptions,
	) (string, error) {
		return "diff --git a/x b/x\n", nil
	}

	ctx := t.Context()

	commits, err := fake.ListPullRequestCommits(ctx, testWorkspace, testRepo, prID,
		bitbucket.ListOptions{MaxItems: 7})
	if err != nil {
		t.Fatalf("ListPullRequestCommits: %v", err)
	}
	comments, err := fake.ListPullRequestComments(ctx, testWorkspace, testRepo, prID,
		bitbucket.ListOptions{MaxItems: 9})
	if err != nil {
		t.Fatalf("ListPullRequestComments: %v", err)
	}
	entries, err := fake.ListPullRequestDiffStat(ctx, testWorkspace, testRepo, prID)
	if err != nil {
		t.Fatalf("ListPullRequestDiffStat: %v", err)
	}
	diff, err := fake.GetPullRequestDiff(ctx, testWorkspace, testRepo, prID,
		bitbucket.DiffOptions{Path: "pkg/tui/app.go", Context: 5, DiffURL: "https://api.example/diff"})
	if err != nil {
		t.Fatalf("GetPullRequestDiff: %v", err)
	}

	testkit.AssertEqual(t, "commits", len(commits), 1)
	testkit.AssertEqual(t, "comments", len(comments), 1)
	testkit.AssertEqual(t, "diffstat entries", len(entries), 1)
	testkit.AssertEqual(t, "diff", diff, "diff --git a/x b/x\n")

	testkit.AssertEqual(t, "ListPullRequestCommits", fake.CallCount("ListPullRequestCommits"), 1)
	testkit.AssertEqual(t, "ListPullRequestComments", fake.CallCount("ListPullRequestComments"), 1)
	testkit.AssertEqual(t, "ListPullRequestDiffStat", fake.CallCount("ListPullRequestDiffStat"), 1)
	testkit.AssertEqual(t, "GetPullRequestDiff", fake.CallCount("GetPullRequestDiff"), 1)

	testkit.AssertEqual(t, "commits max items", fake.ListPullRequestCommitsCalls[0].Opts.MaxItems, 7)
	testkit.AssertEqual(t, "comments pull request", fake.ListPullRequestCommentsCalls[0].ID, prID)
	testkit.AssertEqual(t, "diffstat repo", fake.ListPullRequestDiffStatCalls[0].RepoSlug, testRepo)
	testkit.AssertEqual(t, "diff path", fake.GetPullRequestDiffCalls[0].Opts.Path, "pkg/tui/app.go")
	testkit.AssertEqual(t, "diff context", fake.GetPullRequestDiffCalls[0].Opts.Context, 5)
	testkit.AssertEqual(t, "diff url", fake.GetPullRequestDiffCalls[0].Opts.DiffURL, "https://api.example/diff")
}
