package tui

import (
	"context"
	"errors"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

func TestFetchCurrentUser(t *testing.T) {
	t.Parallel()

	t.Run("success carries the account", func(t *testing.T) {
		t.Parallel()
		fake := &bitbuckettest.FakeClient{T: t}
		fake.GetCurrentUserFunc = func(context.Context) (*bitbucket.Account, error) {
			return &bitbucket.Account{UUID: testUserUUID, DisplayName: "Demo User"}, nil
		}
		app := newAppWithFake(t, fake)

		msg, ok := app.fetchCurrentUser()().(currentUserLoadedMsg)

		if !ok {
			t.Fatal("fetchCurrentUser did not produce a currentUserLoadedMsg")
		}
		if msg.user == nil || msg.user.UUID != testUserUUID {
			t.Errorf("user = %+v, want %s", msg.user, testUserUUID)
		}
	})

	t.Run("the error is carried, not raised", func(t *testing.T) {
		t.Parallel()
		fake := &bitbuckettest.FakeClient{T: t}
		fake.GetCurrentUserFunc = func(context.Context) (*bitbucket.Account, error) {
			return nil, errors.New(testBoom)
		}
		app := newAppWithFake(t, fake)

		msg, ok := app.fetchCurrentUser()().(currentUserLoadedMsg)

		if !ok || msg.err == nil {
			t.Fatalf("msg = %+v, want a carried error", msg)
		}
	})
}

func TestFetchRepos_ConfiguredRepositoriesCostNoRequest(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	app := newAppWithFake(t, fake)

	msg, ok := app.fetchRepos()().(reposLoadedMsg)

	if !ok {
		t.Fatal("fetchRepos did not produce a reposLoadedMsg")
	}
	testkit.AssertEqual(t, "ListRepositories calls", fake.CallCount("ListRepositories"), 0)
	testkit.AssertEqual(t, "stub count", len(msg.repos), 2)
	testkit.AssertEqual(t, "stub full name", msg.repos[0].FullName, testWorkspace+"/"+testRepo)
}

func TestFetchRepos_DiscoversWhenNoneAreConfigured(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.ListRepositoriesFunc = func(_ context.Context, workspace string,
		opts bitbucket.ListRepositoriesOptions,
	) ([]bitbucket.Repository, error) {
		testkit.AssertEqual(t, "workspace", workspace, testWorkspace)
		testkit.AssertEqual(t, "role", opts.Role, "member")
		testkit.AssertEqual(t, "max items", opts.MaxItems, repoDiscoveryLimit)
		return []bitbucket.Repository{{Slug: testRepo}}, nil
	}
	app := newAppWithFake(t, fake)
	app.cfg.Bitbucket.Repositories = nil

	msg, ok := app.fetchRepos()().(reposLoadedMsg)

	if !ok {
		t.Fatal("fetchRepos did not produce a reposLoadedMsg")
	}
	testkit.AssertEqual(t, "repo count", len(msg.repos), 1)
}

func TestFetchPullRequests_OnePerRepositoryAndGuardedByInFlight(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.ListPullRequestsFunc = func(_ context.Context, _, repo string,
		opts bitbucket.ListPullRequestsOptions,
	) ([]bitbucket.PullRequest, error) {
		testkit.AssertEqual(t, "state count", len(opts.States), 1)
		testkit.AssertEqual(t, "state", string(opts.States[0]), testOpenState)
		return []bitbucket.PullRequest{newPR(repo, 101, testTitle)}, nil
	}
	app := newAppWithFake(t, fake)

	cmd := app.fetchPullRequests(testFetchKey)
	if cmd == nil {
		t.Fatal("expected a fetch command")
	}
	if second := app.fetchPullRequests(testFetchKey); second != nil {
		t.Error("a second fetch for a key already in flight must be dropped")
	}

	msg, ok := cmd().(pullRequestsLoadedMsg)
	if !ok {
		t.Fatal("the fetch did not produce a pullRequestsLoadedMsg")
	}
	testkit.AssertEqual(t, "fetch key", msg.fetchKey, testFetchKey)
	testkit.AssertEqual(t, "one result per repository", len(msg.results), 2)
	testkit.AssertEqual(t, "requests", fake.CallCount("ListPullRequests"), 2)
}

func TestFetchPullRequests_NeedsAWorkspaceAndRepositories(t *testing.T) {
	t.Parallel()
	app := newAppWithFake(t, &bitbuckettest.FakeClient{T: t})
	app.repoSlugs = nil

	if cmd := app.fetchPullRequests(testFetchKey); cmd != nil {
		t.Error("no repositories means no request")
	}
	if app.inFlight[testFetchKey] {
		t.Error("a dropped fetch must not be marked in flight")
	}
}

func TestFetchPullRequestDetail_CarriesTheEpoch(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.GetPullRequestFunc = func(_ context.Context, workspace, repo string, id int,
	) (*bitbucket.PullRequest, error) {
		testkit.AssertEqual(t, "workspace", workspace, testWorkspace)
		testkit.AssertEqual(t, "repo", repo, testRepo)
		testkit.AssertEqual(t, "id", id, 101)
		pr := newPR(testRepo, 101, testTitle)
		return &pr, nil
	}
	app := newAppWithFake(t, fake)
	pr := newPR(testRepo, 101, testTitle)

	msg, ok := app.fetchPullRequestDetail(&pr, 7)().(pullRequestDetailLoadedMsg)

	if !ok {
		t.Fatal("expected a pullRequestDetailLoadedMsg")
	}
	testkit.AssertEqual(t, "epoch", msg.epoch, 7)
	if msg.pr == nil {
		t.Error("the detail should have been carried")
	}
}

func TestFetchPullRequestDetail_FailureIsSilentButStillEpoched(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.GetPullRequestFunc = func(context.Context, string, string, int) (*bitbucket.PullRequest, error) {
		return nil, errors.New(testBoom)
	}
	app := newAppWithFake(t, fake)
	pr := newPR(testRepo, 101, testTitle)

	msg, ok := app.fetchPullRequestDetail(&pr, 3)().(pullRequestDetailLoadedMsg)

	if !ok {
		t.Fatal("expected a pullRequestDetailLoadedMsg")
	}
	if msg.pr != nil {
		t.Error("a failed detail fetch must not carry a pull request")
	}
	testkit.AssertEqual(t, "epoch", msg.epoch, 3)
}

func TestFetchStatuses_CarriesKeyAndUpdatedOn(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.ListPullRequestStatusesFunc = func(context.Context, string, string, int,
	) ([]bitbucket.CommitStatus, error) {
		return []bitbucket.CommitStatus{newStatus(testRepo, 412, bitbucket.StatusSuccessful)}, nil
	}
	app := newAppWithFake(t, fake)
	pr := newPR(testRepo, 101, testTitle)

	msg, ok := app.fetchStatuses(&pr, 2)().(statusesLoadedMsg)

	if !ok {
		t.Fatal("expected a statusesLoadedMsg")
	}
	testkit.AssertEqual(t, "key", msg.key, testPRKey)
	testkit.AssertEqual(t, "updatedOn", msg.updatedOn.Equal(pr.UpdatedOn), true)
	testkit.AssertEqual(t, "statuses", len(msg.statuses), 1)
}

func TestFetchPipeline_AsksForTheBuildNumberAndItsSteps(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.GetPipelineFunc = func(_ context.Context, _, _, ref string) (*bitbucket.Pipeline, error) {
		testkit.AssertEqual(t, "pipeline ref", ref, "412")
		return &bitbucket.Pipeline{BuildNumber: 412, UUID: "{run}"}, nil
	}
	fake.ListPipelineStepsFunc = func(context.Context, string, string, string) ([]bitbucket.PipelineStep, error) {
		return []bitbucket.PipelineStep{{UUID: "{step}", Name: "Build"}}, nil
	}
	app := newAppWithFake(t, fake)
	pr := newPR(testRepo, 101, testTitle)

	msg, ok := app.fetchPipeline(testWorkspace, testRepo, pr.Key(), 412)().(pipelineLoadedMsg)

	if !ok {
		t.Fatal("expected a pipelineLoadedMsg")
	}
	testkit.AssertEqual(t, "key", msg.key, testPRKey)
	testkit.AssertEqual(t, "build", msg.build, 412)
	testkit.AssertEqual(t, "steps", len(msg.steps), 1)
}

func TestFetchPipeline_StepFailureIsCarried(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.GetPipelineFunc = func(context.Context, string, string, string) (*bitbucket.Pipeline, error) {
		return &bitbucket.Pipeline{BuildNumber: 412}, nil
	}
	fake.ListPipelineStepsFunc = func(context.Context, string, string, string) ([]bitbucket.PipelineStep, error) {
		return nil, errors.New(testBoom)
	}
	app := newAppWithFake(t, fake)
	pr := newPR(testRepo, 101, testTitle)

	msg, ok := app.fetchPipeline(testWorkspace, testRepo, pr.Key(), 412)().(pipelineLoadedMsg)

	if !ok || msg.err == nil {
		t.Fatalf("msg = %+v, want the step error carried", msg)
	}
}

func TestFetchStepLog_AsksForTheTail(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.GetPipelineStepLogFunc = func(_ context.Context, _, _, pipelineRef, stepUUID string,
		opts bitbucket.StepLogOptions,
	) (string, error) {
		testkit.AssertEqual(t, "pipeline ref", pipelineRef, "{run}")
		testkit.AssertEqual(t, "step uuid", stepUUID, "{step}")
		testkit.AssertEqual(t, "tail bytes", opts.TailBytes, stepLogTailBytes)
		return "hello", nil
	}
	app := newAppWithFake(t, fake)

	msg, ok := app.fetchStepLog(testWorkspace, testRepo, "{run}", "{step}")().(stepLogLoadedMsg)

	if !ok {
		t.Fatal("expected a stepLogLoadedMsg")
	}
	testkit.AssertEqual(t, "text", msg.text, "hello")
}

func TestFetchCommands_AreNilWithoutAPullRequest(t *testing.T) {
	t.Parallel()
	app := newAppWithFake(t, &bitbuckettest.FakeClient{T: t})

	if app.fetchStatuses(nil, 0) != nil ||
		app.fetchPullRequestDetail(nil, 0) != nil ||
		app.fetchDiffStat(nil) != nil ||
		app.fetchCommits(nil) != nil ||
		app.fetchThreads(nil) != nil ||
		app.fetchFileDiff(nil, "a", "a") != nil {
		t.Error("every per-pull-request fetch must be nil without one")
	}
}

func TestFetchDiffStat_CarriesTheKeyAndTheSourceHash(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.ListPullRequestDiffStatFunc = func(_ context.Context, workspace, slug string, id int,
	) ([]bitbucket.DiffStatEntry, error) {
		testkit.AssertEqual(t, "workspace", workspace, testWorkspace)
		testkit.AssertEqual(t, "repo", slug, testRepo)
		testkit.AssertEqual(t, "id", id, 101)
		return testDiffStat(), nil
	}
	app := newAppWithFake(t, fake)
	pr := withSource(newPR(testRepo, 101, testTitle), testSourceHash)

	msg, ok := app.fetchDiffStat(&pr)().(diffStatLoadedMsg)

	if !ok {
		t.Fatal("expected a diffStatLoadedMsg")
	}
	testkit.AssertEqual(t, "key", msg.key, testPRKey)
	testkit.AssertEqual(t, "source hash", msg.hash, testSourceHash)
	testkit.AssertEqual(t, "entries", len(msg.entries), 2)
}

func TestFetchCommits_CarriesTheCommits(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	stubCommits(fake)
	app := newAppWithFake(t, fake)
	pr := withSource(newPR(testRepo, 101, testTitle), testSourceHash)

	msg, ok := app.fetchCommits(&pr)().(commitsLoadedMsg)

	if !ok {
		t.Fatal("expected a commitsLoadedMsg")
	}
	testkit.AssertEqual(t, "commits", len(msg.commits), 1)
	testkit.AssertEqual(t, "hash", msg.commits[0].ShortHash(), testCommitShort)
}

func TestFetchThreads_GroupsTheFlatCommentList(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	root := testThreads()[0].Root
	reply := root
	reply.ID = root.ID + 1
	reply.Parent = &bitbucket.CommentRef{ID: root.ID}
	fake.ListPullRequestCommentsFunc = func(context.Context, string, string, int,
		bitbucket.ListOptions,
	) ([]bitbucket.Comment, error) {
		return []bitbucket.Comment{root, reply}, nil
	}
	app := newAppWithFake(t, fake)
	pr := withSource(newPR(testRepo, 101, testTitle), testSourceHash)

	msg, ok := app.fetchThreads(&pr)().(threadsLoadedMsg)

	if !ok {
		t.Fatal("expected a threadsLoadedMsg")
	}
	testkit.AssertEqual(t, "threads", len(msg.threads), 1)
	testkit.AssertEqual(t, "replies", len(msg.threads[0].Replies), 1)
}

func TestFetchThreads_TheErrorIsCarried(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.ListPullRequestCommentsFunc = func(context.Context, string, string, int,
		bitbucket.ListOptions,
	) ([]bitbucket.Comment, error) {
		return nil, errors.New(testBoom)
	}
	app := newAppWithFake(t, fake)
	pr := withSource(newPR(testRepo, 101, testTitle), testSourceHash)

	msg, ok := app.fetchThreads(&pr)().(threadsLoadedMsg)

	if !ok || msg.err == nil {
		t.Fatalf("msg = %+v, want the comment error carried", msg)
	}
	if msg.threads != nil {
		t.Error("a failed fetch must not report threads")
	}
}

func TestFetchFileDiff_AsksForTheEscapedPathAndParsesTheAnswer(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.GetPullRequestDiffFunc = func(_ context.Context, _, _ string, _ int,
		opts bitbucket.DiffOptions,
	) (string, error) {
		testkit.AssertEqual(t, "escaped path", opts.Path, "docs/rate%20limits.md")
		testkit.AssertEqual(t, "diff url", opts.DiffURL, "https://api.bitbucket.org/diff/spec")
		testkit.AssertEqual(t, "context lines", opts.Context, 0)
		return testDiffText, nil
	}
	app := newAppWithFake(t, fake)
	pr := withSource(newPR(testRepo, 101, testTitle), testSourceHash)
	pr.Links.Diff = bitbucket.Link{Href: "https://api.bitbucket.org/diff/spec"}

	msg, ok := app.fetchFileDiff(&pr, testFileMD, "docs/rate%20limits.md")().(fileDiffLoadedMsg)

	if !ok {
		t.Fatal("expected a fileDiffLoadedMsg")
	}
	testkit.AssertEqual(t, "display path", msg.path, testFileMD)
	if msg.file == nil {
		t.Fatal("the parsed file should be the one that was asked for")
	}
	testkit.AssertEqual(t, "parsed path", msg.file.Path(), testFileMD)
}

func TestFetchFileDiff_ParseFailureIsCarried(t *testing.T) {
	t.Parallel()
	fake := &bitbuckettest.FakeClient{T: t}
	fake.GetPullRequestDiffFunc = func(_ context.Context, _, _ string, _ int,
		_ bitbucket.DiffOptions,
	) (string, error) {
		return "@@ -1,1 +1,1 @@\n-a\n+b\n", nil
	}
	app := newAppWithFake(t, fake)
	pr := withSource(newPR(testRepo, 101, testTitle), testSourceHash)

	msg, ok := app.fetchFileDiff(&pr, testFileGo, testFileGo)().(fileDiffLoadedMsg)

	if !ok || msg.err == nil {
		t.Fatalf("msg = %+v, want the parse failure carried", msg)
	}
}

func TestFileDiffKey_JoinsThePullRequestAndThePath(t *testing.T) {
	t.Parallel()

	testkit.AssertEqual(t, "cache key", fileDiffKey(testPRKey, testFileGo),
		testPRKey+"\x00"+testFileGo)
}

func TestListOptionsForKey_UsesTheTabBehindTheKey(t *testing.T) {
	t.Parallel()
	app := newAppWithFake(t, &bitbuckettest.FakeClient{T: t})

	opts := app.listOptionsForKey(testFetchKey)

	testkit.AssertEqual(t, "state", string(opts.States[0]), testOpenState)
	testkit.AssertEqual(t, "query", opts.Query, "")
	testkit.AssertEqual(t, "max items", opts.MaxItems, app.cfg.ResolveGlobalMaxResults())
}

func TestFetchKeyOf_SharesOneKeyAcrossTheDefaultTabs(t *testing.T) {
	t.Parallel()
	app := newAppWithFake(t, &bitbuckettest.FakeClient{T: t})

	testkit.AssertEqual(t, "tabs", len(app.tabConfigs()), 3)
	for tab := range app.tabConfigs() {
		testkit.AssertEqual(t, "fetch key", app.fetchKeyOf(tab), testFetchKey)
	}
	testkit.AssertEqual(t, "out of range", app.fetchKeyOf(99), testFetchKey)
}

func TestPipelineCacheKey(t *testing.T) {
	t.Parallel()

	testkit.AssertEqual(t, "cache key", pipelineCacheKey(testPRKey, 412), "acme/api-gateway#412")
}
