package bitbucket

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

var (
	errRepoUnavailable   = errors.New("repository unavailable")
	errPipelinesDisabled = errors.New("pipelines are not enabled for this repository")
)

// stubClient is a goroutine-safe ClientInterface for the fetch helpers, which
// call the client from several goroutines at once. Only ListPullRequests and
// ListPipelines are exercised; the rest satisfy the interface.
type stubClient struct {
	mu          sync.Mutex
	calls       []string
	inFlight    int
	maxInFlight int
	list        func(ctx context.Context, workspace, repoSlug string) ([]PullRequest, error)
	// listPipelines is nil in the pull request tests; the pipeline tests set it
	// and share the same call log and in-flight counters.
	listPipelines func(ctx context.Context, workspace, repoSlug string) ([]Pipeline, error)
}

var _ ClientInterface = (*stubClient)(nil)

func (s *stubClient) ListPullRequests(
	ctx context.Context, workspace, repoSlug string, _ ListPullRequestsOptions,
) ([]PullRequest, error) {
	s.mu.Lock()
	s.calls = append(s.calls, repoSlug)
	s.inFlight++
	s.maxInFlight = max(s.maxInFlight, s.inFlight)
	list := s.list
	s.mu.Unlock()

	prs, err := list(ctx, workspace, repoSlug)

	s.mu.Lock()
	s.inFlight--
	s.mu.Unlock()

	return prs, err
}

func (s *stubClient) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func (s *stubClient) peakInFlight() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.maxInFlight
}

func (s *stubClient) GetCurrentUser(context.Context) (*Account, error) { return nil, nil }

func (s *stubClient) GetWorkspace(context.Context, string) (*Workspace, error) { return nil, nil }

func (s *stubClient) ListRepositories(
	context.Context, string, ListRepositoriesOptions,
) ([]Repository, error) {
	return nil, nil
}

func (s *stubClient) GetRepository(context.Context, string, string) (*Repository, error) {
	return nil, nil
}

func (s *stubClient) GetPullRequest(context.Context, string, string, int) (*PullRequest, error) {
	return nil, nil
}

func (s *stubClient) ListPullRequestStatuses(
	context.Context, string, string, int,
) ([]CommitStatus, error) {
	return nil, nil
}

func (s *stubClient) ListPullRequestCommits(
	context.Context, string, string, int, ListOptions,
) ([]Commit, error) {
	return nil, nil
}

func (s *stubClient) ListPullRequestComments(
	context.Context, string, string, int, ListOptions,
) ([]Comment, error) {
	return nil, nil
}

func (s *stubClient) ListPullRequestDiffStat(
	context.Context, string, string, int,
) ([]DiffStatEntry, error) {
	return nil, nil
}

func (s *stubClient) GetPullRequestDiff(
	context.Context, string, string, int, DiffOptions,
) (string, error) {
	return "", nil
}

func (s *stubClient) ListPipelines(
	ctx context.Context, workspace, repoSlug string, _ ListPipelinesOptions,
) ([]Pipeline, error) {
	s.mu.Lock()
	s.calls = append(s.calls, repoSlug)
	s.inFlight++
	s.maxInFlight = max(s.maxInFlight, s.inFlight)
	list := s.listPipelines
	s.mu.Unlock()

	if list == nil {
		return nil, nil
	}

	pipelines, err := list(ctx, workspace, repoSlug)

	s.mu.Lock()
	s.inFlight--
	s.mu.Unlock()

	return pipelines, err
}

func (s *stubClient) GetPipeline(context.Context, string, string, string) (*Pipeline, error) {
	return nil, nil
}

func (s *stubClient) ListPipelineSteps(context.Context, string, string, string) ([]PipelineStep, error) {
	return nil, nil
}

func (s *stubClient) GetPipelineStepLog(
	context.Context, string, string, string, string, StepLogOptions,
) (string, error) {
	return "", nil
}

func (s *stubClient) MergePullRequest(
	context.Context, string, string, int, MergeOptions,
) (MergeResult, error) {
	return MergeResult{PullRequest: nil, TaskURL: ""}, nil
}

func (s *stubClient) GetMergeTaskStatus(context.Context, string) (MergeTaskStatus, error) {
	return MergeTaskStatus{Status: "", MergeResult: nil}, nil
}

func (s *stubClient) UpdatePullRequest(
	context.Context, string, string, int, PullRequestUpdate,
) (*PullRequest, error) {
	return nil, nil
}

func (s *stubClient) ApprovePullRequest(context.Context, string, string, int) (*Participant, error) {
	return nil, nil
}

func (s *stubClient) UnapprovePullRequest(context.Context, string, string, int) error {
	return nil
}

func (s *stubClient) RunPipeline(context.Context, string, string, PipelineTarget) (*Pipeline, error) {
	return nil, nil
}

func (s *stubClient) StopPipeline(context.Context, string, string, string) error {
	return nil
}

func (s *stubClient) ListEffectiveDefaultReviewers(context.Context, string, string) ([]Account, error) {
	return nil, nil
}

func (s *stubClient) ListWorkspaceMembers(context.Context, string, ListOptions) ([]Account, error) {
	return nil, nil
}

func (s *stubClient) SetOnRequest(func(RequestLog)) {}

func noOptions() ListPullRequestsOptions {
	return ListPullRequestsOptions{States: nil, Query: "", Sort: "", MaxItems: 0, Fields: ""}
}

func TestFetchPullRequests_PreservesInputOrder(t *testing.T) {
	t.Parallel()

	repos := []string{"web-app", testRepoSlug, "infra"}
	client := &stubClient{
		mu: sync.Mutex{}, calls: nil, inFlight: 0, maxInFlight: 0,
		list: func(_ context.Context, _, repoSlug string) ([]PullRequest, error) {
			return []PullRequest{{ID: len(repoSlug)}}, nil
		},
	}

	results := FetchPullRequests(t.Context(), client, testWorkspace, repos, noOptions(), 0)

	if len(results) != len(repos) {
		t.Fatalf("result count = %d, want %d", len(results), len(repos))
	}
	for index, repo := range repos {
		testkit.AssertEqual(t, "Repo", results[index].Repo, repo)
		if results[index].Err != nil {
			t.Errorf("%s: Err = %v, want nil", repo, results[index].Err)
		}
		if len(results[index].PullRequests) != 1 {
			t.Fatalf("%s: pull request count = %d, want 1", repo, len(results[index].PullRequests))
		}
		testkit.AssertEqual(t, "ID", results[index].PullRequests[0].ID, len(repo))
	}
	testkit.AssertEqual(t, "calls", client.callCount(), len(repos))
}

func TestFetchPullRequests_IsolatesPerRepoErrors(t *testing.T) {
	t.Parallel()

	client := &stubClient{
		mu: sync.Mutex{}, calls: nil, inFlight: 0, maxInFlight: 0,
		list: func(_ context.Context, _, repoSlug string) ([]PullRequest, error) {
			if repoSlug == "infra" {
				return nil, errRepoUnavailable
			}
			return []PullRequest{{ID: 1}}, nil
		},
	}

	results := FetchPullRequests(t.Context(), client, testWorkspace,
		[]string{testRepoSlug, "infra", "web-app"}, noOptions(), 2)

	if results[0].Err != nil || results[2].Err != nil {
		t.Errorf("healthy repositories carry errors: %v, %v", results[0].Err, results[2].Err)
	}
	if !errors.Is(results[1].Err, errRepoUnavailable) {
		t.Errorf("infra Err = %v, want %v", results[1].Err, errRepoUnavailable)
	}
	if results[1].PullRequests != nil {
		t.Errorf("infra PullRequests = %v, want nil", results[1].PullRequests)
	}
	testkit.AssertEqual(t, "healthy repo still fetched", len(results[2].PullRequests), 1)
}

func TestFetchPullRequests_RunsWorkersConcurrentlyUpToTheBound(t *testing.T) {
	t.Parallel()

	const concurrency = 2

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	arrived := make(chan struct{}, 8)
	release := make(chan struct{})
	client := &stubClient{
		mu: sync.Mutex{}, calls: nil, inFlight: 0, maxInFlight: 0,
		list: func(ctx context.Context, _, _ string) ([]PullRequest, error) {
			arrived <- struct{}{}
			select {
			case <-release:
				return []PullRequest{{ID: 1}}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	}

	done := make(chan []RepoPullRequests, 1)
	go func() {
		done <- FetchPullRequests(ctx, client, testWorkspace,
			[]string{"a", "b", "c", "d", "e"}, noOptions(), concurrency)
	}()

	for range concurrency {
		select {
		case <-arrived:
		case <-time.After(10 * time.Second):
			cancel()
			<-done
			t.Fatalf("fewer than %d workers ran concurrently", concurrency)
		}
	}
	close(release)
	results := <-done

	testkit.AssertEqual(t, "results", len(results), 5)
	testkit.AssertEqual(t, "peak in-flight", client.peakInFlight(), concurrency)
}

func TestFetchPullRequests_SerialWhenConcurrencyIsOne(t *testing.T) {
	t.Parallel()

	client := &stubClient{
		mu: sync.Mutex{}, calls: nil, inFlight: 0, maxInFlight: 0,
		list: func(context.Context, string, string) ([]PullRequest, error) {
			return []PullRequest{{ID: 1}}, nil
		},
	}

	results := FetchPullRequests(t.Context(), client, testWorkspace,
		[]string{"a", "b", "c", "d"}, noOptions(), 1)

	testkit.AssertEqual(t, "results", len(results), 4)
	testkit.AssertEqual(t, "peak in-flight", client.peakInFlight(), 1)
}

func TestFetchPullRequests_StopsOnCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	client := &stubClient{
		mu: sync.Mutex{}, calls: nil, inFlight: 0, maxInFlight: 0,
		list: func(ctx context.Context, _, _ string) ([]PullRequest, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return []PullRequest{{ID: 1}}, nil
		},
	}

	results := FetchPullRequests(ctx, client, testWorkspace,
		[]string{testRepoSlug, "web-app", "infra"}, noOptions(), 2)

	if len(results) != 3 {
		t.Fatalf("result count = %d, want 3", len(results))
	}
	for _, result := range results {
		if !errors.Is(result.Err, context.Canceled) {
			t.Errorf("%s: Err = %v, want context.Canceled", result.Repo, result.Err)
		}
		if result.PullRequests != nil {
			t.Errorf("%s: PullRequests = %v, want nil", result.Repo, result.PullRequests)
		}
	}
}

func TestFetchPullRequests_EmptyRepoList(t *testing.T) {
	t.Parallel()

	client := &stubClient{
		mu: sync.Mutex{}, calls: nil, inFlight: 0, maxInFlight: 0,
		list: func(context.Context, string, string) ([]PullRequest, error) {
			t.Error("client called for an empty repository list")
			return nil, nil
		},
	}

	results := FetchPullRequests(t.Context(), client, testWorkspace, nil, noOptions(), 0)
	testkit.AssertEqual(t, "results", len(results), 0)
}

func TestMergePullRequests_Ordering(t *testing.T) {
	t.Parallel()

	newest := time.Date(2026, time.September, 3, 16, 0, 0, 0, time.UTC)
	older := time.Date(2026, time.September, 1, 9, 0, 0, 0, time.UTC)

	results := []RepoPullRequests{
		{Repo: "web-app", Err: nil, PullRequests: []PullRequest{
			{ID: 20, UpdatedOn: newest},
			{ID: 21, UpdatedOn: older},
		}},
		{Repo: testRepoSlug, Err: nil, PullRequests: []PullRequest{
			{ID: 10, UpdatedOn: newest},
			{ID: 11, UpdatedOn: older},
			{ID: 12, UpdatedOn: older},
		}},
		{Repo: "infra", Err: errRepoUnavailable, PullRequests: nil},
	}

	merged := MergePullRequests(results)

	got := make([]int, 0, len(merged))
	for _, pr := range merged {
		got = append(got, pr.ID)
	}
	// Newest first; ties break on repository slug, then on descending id.
	testkit.AssertSliceEqual(t, "ids", got, []int{10, 20, 12, 11, 21})
}

func TestMergePullRequests_SkipsFailedRepositories(t *testing.T) {
	t.Parallel()

	results := []RepoPullRequests{
		{Repo: "infra", Err: errRepoUnavailable, PullRequests: []PullRequest{{ID: 7}}},
		{Repo: testRepoSlug, Err: nil, PullRequests: []PullRequest{{ID: 101}}},
	}

	merged := MergePullRequests(results)

	if len(merged) != 1 {
		t.Fatalf("merged count = %d, want 1", len(merged))
	}
	testkit.AssertEqual(t, "ID", merged[0].ID, 101)
}

func TestMergePullRequests_EmptyInput(t *testing.T) {
	t.Parallel()

	testkit.AssertEqual(t, "merged", len(MergePullRequests(nil)), 0)
}

func noPipelineOptions() ListPipelinesOptions {
	return ListPipelinesOptions{TargetBranch: "", Sort: "", MaxItems: 0}
}

// pipelineAt is the smallest run the fetch and merge tests need: the fields
// they order and identify by, nothing else.
func pipelineAt(build int, created time.Time) Pipeline {
	return Pipeline{BuildNumber: build, CreatedOn: created}
}

func TestFetchPipelines_PreservesInputOrder(t *testing.T) {
	t.Parallel()

	repos := []string{"web-app", testRepoSlug, "infra"}
	client := &stubClient{
		mu: sync.Mutex{}, calls: nil, inFlight: 0, maxInFlight: 0,
		listPipelines: func(_ context.Context, _, repoSlug string) ([]Pipeline, error) {
			return []Pipeline{pipelineAt(len(repoSlug), time.Time{})}, nil
		},
	}

	results := FetchPipelines(t.Context(), client, testWorkspace, repos, noPipelineOptions(), 0)

	if len(results) != len(repos) {
		t.Fatalf("result count = %d, want %d", len(results), len(repos))
	}
	for index, repo := range repos {
		testkit.AssertEqual(t, "Repo", results[index].Repo, repo)
		if results[index].Err != nil {
			t.Errorf("%s: Err = %v, want nil", repo, results[index].Err)
		}
		if len(results[index].Pipelines) != 1 {
			t.Fatalf("%s: pipeline count = %d, want 1", repo, len(results[index].Pipelines))
		}
		testkit.AssertEqual(t, "BuildNumber", results[index].Pipelines[0].BuildNumber, len(repo))
	}
	testkit.AssertEqual(t, "calls", client.callCount(), len(repos))
}

func TestFetchPipelines_IsolatesPerRepoErrors(t *testing.T) {
	t.Parallel()

	client := &stubClient{
		mu: sync.Mutex{}, calls: nil, inFlight: 0, maxInFlight: 0,
		listPipelines: func(_ context.Context, _, repoSlug string) ([]Pipeline, error) {
			if repoSlug == "infra" {
				return nil, errPipelinesDisabled
			}
			return []Pipeline{pipelineAt(1, time.Time{})}, nil
		},
	}

	results := FetchPipelines(t.Context(), client, testWorkspace,
		[]string{testRepoSlug, "infra", "web-app"}, noPipelineOptions(), 2)

	if results[0].Err != nil || results[2].Err != nil {
		t.Errorf("healthy repositories carry errors: %v, %v", results[0].Err, results[2].Err)
	}
	if !errors.Is(results[1].Err, errPipelinesDisabled) {
		t.Errorf("infra Err = %v, want %v", results[1].Err, errPipelinesDisabled)
	}
	if results[1].Pipelines != nil {
		t.Errorf("infra Pipelines = %v, want nil", results[1].Pipelines)
	}
	testkit.AssertEqual(t, "healthy repo still fetched", len(results[2].Pipelines), 1)
}

func TestFetchPipelines_StopsOnCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	client := &stubClient{
		mu: sync.Mutex{}, calls: nil, inFlight: 0, maxInFlight: 0,
		listPipelines: func(ctx context.Context, _, _ string) ([]Pipeline, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return []Pipeline{pipelineAt(1, time.Time{})}, nil
		},
	}

	results := FetchPipelines(ctx, client, testWorkspace,
		[]string{testRepoSlug, "web-app", "infra"}, noPipelineOptions(), 2)

	if len(results) != 3 {
		t.Fatalf("result count = %d, want 3", len(results))
	}
	for _, result := range results {
		if !errors.Is(result.Err, context.Canceled) {
			t.Errorf("%s: Err = %v, want context.Canceled", result.Repo, result.Err)
		}
		if result.Pipelines != nil {
			t.Errorf("%s: Pipelines = %v, want nil", result.Repo, result.Pipelines)
		}
	}
}

func TestFetchPipelines_EmptyRepoList(t *testing.T) {
	t.Parallel()

	client := &stubClient{
		mu: sync.Mutex{}, calls: nil, inFlight: 0, maxInFlight: 0,
		listPipelines: func(context.Context, string, string) ([]Pipeline, error) {
			t.Error("client called for an empty repository list")
			return nil, nil
		},
	}

	results := FetchPipelines(t.Context(), client, testWorkspace, nil, noPipelineOptions(), 0)
	testkit.AssertEqual(t, "results", len(results), 0)
}

func TestMergePipelines_Ordering(t *testing.T) {
	t.Parallel()

	newest := time.Date(2026, time.September, 3, 16, 0, 0, 0, time.UTC)
	older := time.Date(2026, time.September, 1, 9, 0, 0, 0, time.UTC)

	results := []RepoPipelines{
		{Repo: "web-app", Err: nil, Pipelines: []Pipeline{
			pipelineAt(20, newest),
			pipelineAt(21, older),
		}},
		{Repo: testRepoSlug, Err: nil, Pipelines: []Pipeline{
			pipelineAt(10, newest),
			pipelineAt(11, older),
			pipelineAt(12, older),
		}},
		{Repo: "infra", Err: errPipelinesDisabled, Pipelines: nil},
	}

	merged := MergePipelines(results)

	builds := make([]int, 0, len(merged))
	repos := make([]string, 0, len(merged))
	for _, entry := range merged {
		builds = append(builds, entry.Pipeline.BuildNumber)
		repos = append(repos, entry.Repo)
	}
	// Newest first; ties break on repository slug, then on descending build number.
	testkit.AssertSliceEqual(t, "builds", builds, []int{10, 20, 12, 11, 21})
	testkit.AssertSliceEqual(t, "repos", repos,
		[]string{testRepoSlug, "web-app", testRepoSlug, testRepoSlug, "web-app"})
}

func TestMergePipelines_SkipsFailedRepositories(t *testing.T) {
	t.Parallel()

	results := []RepoPipelines{
		{Repo: "infra", Err: errPipelinesDisabled, Pipelines: []Pipeline{pipelineAt(7, time.Time{})}},
		{Repo: testRepoSlug, Err: nil, Pipelines: []Pipeline{pipelineAt(101, time.Time{})}},
	}

	merged := MergePipelines(results)

	if len(merged) != 1 {
		t.Fatalf("merged count = %d, want 1", len(merged))
	}
	testkit.AssertEqual(t, "Repo", merged[0].Repo, testRepoSlug)
	testkit.AssertEqual(t, "BuildNumber", merged[0].Pipeline.BuildNumber, 101)
}

func TestMergePipelines_EmptyInput(t *testing.T) {
	t.Parallel()

	testkit.AssertEqual(t, "merged", len(MergePipelines(nil)), 0)
}
