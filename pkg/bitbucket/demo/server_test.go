package demo

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

func TestServer_GetCurrentUser(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	account := decodeJSON[bitbucket.Account](t, getOK(t, server, pathUser))

	testkit.AssertEqual(t, "UUID", account.UUID, uuidDemo)
	testkit.AssertEqual(t, "Name()", account.Name(), "Demo User")
	testkit.AssertEqual(t, "Nickname", account.Nickname, "demo")
}

func TestServer_GetWorkspace(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	workspace := decodeJSON[bitbucket.Workspace](t, getOK(t, server, pathWorkspace))

	testkit.AssertEqual(t, "Slug", workspace.Slug, Workspace)
	testkit.AssertEqual(t, "Name", workspace.Name, "Acme Corp")
	testkit.AssertEqual(t, "links.self", workspace.Links.Self.Href,
		apiBase+"/workspaces/"+Workspace)
}

func TestServer_GetWorkspaceUnknownIsNotFound(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	got := get(t, server.URL+"/workspaces/other", nil)

	testkit.AssertEqual(t, "status", got.Status, http.StatusNotFound)
	testkit.AssertEqual(t, "Content-Type", got.Header.Get("Content-Type"), contentTypeJSON)

	envelope := decodeJSON[errorBody](t, got.Body)
	testkit.AssertEqual(t, "type", envelope.Type, "error")
	testkit.AssertEqual(t, "error.message", envelope.Error.Message,
		"No workspace with identifier 'other'.")
}

func TestServer_ListRepositories(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	body := getOK(t, server, pathRepositories)
	page := decodeJSON[listPage[bitbucket.Repository]](t, body)

	testkit.AssertEqual(t, "size", page.Size, 3)
	testkit.AssertEqual(t, "len(values)", len(page.Values), 3)
	// Default order is -updated_on: api-gateway (2h) before web-app (3h) before infra (8h).
	testkit.AssertEqual(t, "values[0].Slug", page.Values[0].Slug, repoAPIGateway)
	testkit.AssertEqual(t, "values[1].Slug", page.Values[1].Slug, repoWebApp)
	testkit.AssertEqual(t, "values[2].Slug", page.Values[2].Slug, repoInfra)
	testkit.AssertEqual(t, "values[0].MainBranchName()", page.Values[0].MainBranchName(), branchMain)
	testkit.AssertEqual(t, "values[1].MainBranchName()", page.Values[1].MainBranchName(), branchDevelop)

	if _, ok := decodeRaw(t, body)[fieldSize]; !ok {
		t.Fatalf("list envelope has no %q member: %s", fieldSize, body)
	}
}

func TestServer_ListRepositoriesAscendingSort(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	page := decodeJSON[listPage[bitbucket.Repository]](t,
		getOK(t, server, pathRepositories+"?sort=updated_on"))

	testkit.AssertEqual(t, "values[0].Slug", page.Values[0].Slug, repoInfra)
}

func TestServer_GetRepository(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	repository := decodeJSON[bitbucket.Repository](t,
		getOK(t, server, pathRepositories+"/"+repoWebApp))

	testkit.AssertEqual(t, "FullName", repository.FullName, Workspace+"/"+repoWebApp)
	testkit.AssertEqual(t, "WorkspaceSlug()", repository.WorkspaceSlug(), Workspace)
	testkit.AssertEqual(t, "HTMLURL()", repository.HTMLURL(), htmlBase+"/"+Workspace+"/"+repoWebApp)
	testkit.AssertEqual(t, "len(clone)", len(repository.Links.Clone), 2)
}

func TestServer_GetRepositoryUnknownIsNotFound(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	got := get(t, server.URL+pathRepositories+"/nope", nil)

	testkit.AssertEqual(t, "status", got.Status, http.StatusNotFound)
	testkit.AssertEqual(t, "error.message", decodeJSON[errorBody](t, got.Body).Error.Message,
		"Repository "+Workspace+"/nope not found.")
}

func TestServer_ListPullRequestsDefaultsToOpen(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	body := getOK(t, server, pullRequestsPath(repoAPIGateway))
	page := decodeJSON[listPage[bitbucket.PullRequest]](t, body)

	testkit.AssertEqual(t, "size", page.Size, 3)
	ids := make([]int, 0, len(page.Values))
	for _, pullRequest := range page.Values {
		testkit.AssertEqual(t, "State", pullRequest.State, bitbucket.PullRequestOpen)
		ids = append(ids, pullRequest.ID)
	}
	// -updated_on: #103 (2h), #102 (5h), #101 (26h).
	testkit.AssertSliceEqual(t, "ids", ids, []int{103, 102, 101})
}

func TestServer_ListPullRequestsCarriesReviewData(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	body := getOK(t, server, pullRequestsPath(repoAPIGateway))

	// The list endpoint always projects reviewers and participants, so the TUI
	// can be exercised with review data without a per-PR detail fetch.
	values := decodeJSON[listPage[map[string]any]](t, body).Values
	if len(values) == 0 {
		t.Fatalf("no values in %s", body)
	}
	for _, key := range []string{fieldReviewers, fieldParticipants} {
		if _, ok := values[0][key]; !ok {
			t.Fatalf("list item has no %q member: %v", key, values[0])
		}
	}

	page := decodeJSON[listPage[bitbucket.PullRequest]](t, body)
	first := page.Values[0]
	testkit.AssertEqual(t, "ID", first.ID, 103)
	testkit.AssertEqual(t, "HasReviewData()", first.HasReviewData(), true)
	testkit.AssertEqual(t, "IsReviewer(demo)", first.IsReviewer(uuidDemo), true)
	testkit.AssertEqual(t, "ReviewSummary().ChangesRequested",
		first.ReviewSummary().ChangesRequested, 1)
}

func TestServer_ListPullRequestsStateFilter(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	merged := decodeJSON[listPage[bitbucket.PullRequest]](t,
		getOK(t, server, pullRequestsPath(repoAPIGateway)+"?state=MERGED"))
	testkit.AssertEqual(t, "merged size", merged.Size, 1)
	testkit.AssertEqual(t, "merged id", merged.Values[0].ID, 98)

	both := decodeJSON[listPage[bitbucket.PullRequest]](t,
		getOK(t, server, pullRequestsPath(repoAPIGateway)+"?state=MERGED&state=DECLINED"))
	testkit.AssertEqual(t, "merged+declined size", both.Size, 2)
	testkit.AssertSliceEqual(t, "ids", []int{both.Values[0].ID, both.Values[1].ID}, []int{98, 95})

	declined := both.Values[1]
	testkit.AssertEqual(t, "State", declined.State, bitbucket.PullRequestDeclined)
	if declined.Reason == "" {
		t.Fatal("declined pull request has an empty reason")
	}
}

func TestServer_ListPullRequestsPaginationFollowsAbsoluteNext(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	first := decodeJSON[listPage[bitbucket.PullRequest]](t,
		getOK(t, server, pullRequestsPath(repoAPIGateway)+queryPageLenTwo))
	testkit.AssertEqual(t, "page", first.Page, 1)
	testkit.AssertEqual(t, "pagelen", first.PageLen, 2)
	testkit.AssertEqual(t, "size", first.Size, 3)
	testkit.AssertEqual(t, "len(values)", len(first.Values), 2)

	// The client refuses a next link that points at another origin, so it must be
	// absolute and same-origin.
	if !strings.HasPrefix(first.Next, server.URL) {
		t.Fatalf("next = %q, want prefix %q", first.Next, server.URL)
	}

	got := get(t, first.Next, nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusOK)

	second := decodeJSON[listPage[bitbucket.PullRequest]](t, got.Body)
	testkit.AssertEqual(t, "page", second.Page, 2)
	testkit.AssertEqual(t, "len(values)", len(second.Values), 1)
	testkit.AssertEqual(t, "values[0].ID", second.Values[0].ID, 101)
	testkit.AssertEqual(t, "next", second.Next, "")

	if _, ok := decodeRaw(t, got.Body)[fieldNext]; ok {
		t.Fatalf("last page still carries a %q member: %s", fieldNext, got.Body)
	}
}

func TestServer_GetPullRequest(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	pullRequest := decodeJSON[bitbucket.PullRequest](t,
		getOK(t, server, pullRequestPath(repoAPIGateway, 101)))

	testkit.AssertEqual(t, "Key()", pullRequest.Key(),
		bitbucket.PullRequestKey(Workspace, repoAPIGateway, 101))
	testkit.AssertEqual(t, "RepoSlug()", pullRequest.RepoSlug(), repoAPIGateway)
	testkit.AssertEqual(t, "SourceBranch()", pullRequest.SourceBranch(), "feature/rate-limiter")
	testkit.AssertEqual(t, "DestinationBranch()", pullRequest.DestinationBranch(), branchMain)
	testkit.AssertEqual(t, "IsAuthor(demo)", pullRequest.IsAuthor(uuidDemo), true)
	testkit.AssertEqual(t, "ReviewSummary().Approved", pullRequest.ReviewSummary().Approved, 1)
	testkit.AssertEqual(t, "ReviewSummary().Pending", pullRequest.ReviewSummary().Pending, 1)

	// Inside a pull request Bitbucket abbreviates commit hashes to 12 characters.
	if pullRequest.Source.Commit == nil {
		t.Fatal("source commit is nil")
	}
	testkit.AssertEqual(t, "len(source.commit.hash)", len(pullRequest.Source.Commit.Hash), 12)
	testkit.AssertEqual(t, "source.commit.hash", pullRequest.Source.Commit.Hash, hashPR101[:12])
}

func TestServer_GetPullRequestUnknownIsNotFound(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	got := get(t, server.URL+pullRequestPath(repoAPIGateway, 4242), nil)

	testkit.AssertEqual(t, "status", got.Status, http.StatusNotFound)
	testkit.AssertEqual(t, "type", decodeJSON[errorBody](t, got.Body).Type, "error")
}

func TestServer_ListPullRequestStatuses(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	page := decodeJSON[listPage[bitbucket.CommitStatus]](t,
		getOK(t, server, pullRequestPath(repoWebApp, 40)+"/statuses"))

	testkit.AssertEqual(t, "size", page.Size, 2)
	testkit.AssertEqual(t, "values[0].State", page.Values[0].State, bitbucket.StatusStopped)
	testkit.AssertEqual(t, "values[1].State", page.Values[1].State, bitbucket.StatusSuccessful)

	// Two statuses on one commit, distinguished only by key.
	if page.Values[0].Key == page.Values[1].Key {
		t.Fatalf("both statuses share key %q", page.Values[0].Key)
	}
	if page.Values[0].Commit == nil || page.Values[1].Commit == nil {
		t.Fatal("a status carries no commit")
	}
	testkit.AssertEqual(t, "same commit",
		page.Values[0].Commit.Hash, page.Values[1].Commit.Hash)
	// Statuses carry the full 40-character hash, unlike the pull request payload.
	testkit.AssertEqual(t, "len(commit.hash)", len(page.Values[0].Commit.Hash), 40)

	testkit.AssertEqual(t, "RollupStatuses",
		bitbucket.RollupStatuses(page.Values), bitbucket.StatusStopped)

	number, ok := page.Values[1].BuildNumber()
	testkit.AssertEqual(t, "BuildNumber ok", ok, true)
	testkit.AssertEqual(t, "BuildNumber", number, 88)
}

func TestServer_ListPullRequestStatusesEmpty(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	body := getOK(t, server, pullRequestPath(repoWebApp, 41)+"/statuses")
	page := decodeJSON[listPage[bitbucket.CommitStatus]](t, body)

	testkit.AssertEqual(t, "size", page.Size, 0)
	testkit.AssertEqual(t, "len(values)", len(page.Values), 0)
	if _, ok := decodeRaw(t, body)[fieldValues]; !ok {
		t.Fatalf("empty envelope has no %q member: %s", fieldValues, body)
	}
}

func TestServer_ListPipelines(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	page := decodeJSON[listPage[bitbucket.Pipeline]](t,
		getOK(t, server, pipelinesPath(repoAPIGateway)))

	testkit.AssertEqual(t, "size", page.Size, 4)
	numbers := make([]int, 0, len(page.Values))
	for _, pipeline := range page.Values {
		numbers = append(numbers, pipeline.BuildNumber)
	}
	testkit.AssertSliceEqual(t, "build numbers newest first", numbers, []int{414, 413, 412, 411})
}

func TestServer_ListPipelinesFiltersByTargetBranch(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	page := decodeJSON[listPage[bitbucket.Pipeline]](t,
		getOK(t, server, pipelinesPath(repoAPIGateway)+"?target.branch="+
			url.QueryEscape("feature/rate-limiter")))

	testkit.AssertEqual(t, "size", page.Size, 1)
	testkit.AssertEqual(t, "build number", page.Values[0].BuildNumber, 412)
	testkit.AssertEqual(t, "target.BranchName()", page.Values[0].Target.BranchName(),
		"feature/rate-limiter")
}

func TestServer_ListPipelinesPagination(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	first := decodeJSON[listPage[bitbucket.Pipeline]](t,
		getOK(t, server, pipelinesPath(repoAPIGateway)+queryPageLenTwo))

	testkit.AssertEqual(t, "len(values)", len(first.Values), 2)
	if !strings.HasPrefix(first.Next, server.URL) {
		t.Fatalf("next = %q, want prefix %q", first.Next, server.URL)
	}

	got := get(t, first.Next, nil)
	second := decodeJSON[listPage[bitbucket.Pipeline]](t, got.Body)
	testkit.AssertEqual(t, "values[0].BuildNumber", second.Values[0].BuildNumber, 412)
}

func TestServer_GetPipelineByBuildNumber(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	pipeline := decodeJSON[bitbucket.Pipeline](t,
		getOK(t, server, pipelinePath(repoAPIGateway, 414)))

	testkit.AssertEqual(t, "UUID", pipeline.UUID, pipelineUUID(414))
	testkit.AssertEqual(t, "State.Summary()", pipeline.State.Summary(), resultFailed)
	testkit.AssertEqual(t, "HTMLURL()", pipeline.HTMLURL(),
		htmlBase+"/"+Workspace+"/"+repoAPIGateway+"/pipelines/results/414")
	if pipeline.Target.PullRequest == nil {
		t.Fatal("pull request target carries no pullrequest")
	}
	testkit.AssertEqual(t, "target.pullrequest.id", pipeline.Target.PullRequest.ID, 103)
	if pipeline.Target.Commit == nil {
		t.Fatal("target carries no commit")
	}
	// Pipelines carry the full hash on the target and the abbreviated one on the
	// destination commit, exactly like the real payload.
	testkit.AssertEqual(t, "len(target.commit.hash)", len(pipeline.Target.Commit.Hash), 40)
	if pipeline.Target.DestinationCommit == nil {
		t.Fatal("pull request target carries no destination commit")
	}
	testkit.AssertEqual(t, "len(destination_commit.hash)",
		len(pipeline.Target.DestinationCommit.Hash), 12)
}

func TestServer_GetPipelineByBracedUUID(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	uuid := pipelineUUID(413)
	path := pipelinesPath(repoAPIGateway) + "/" + url.PathEscape(uuid)

	if !strings.Contains(path, "%7B") {
		t.Fatalf("path %q is not brace-escaped", path)
	}

	body := getOK(t, server, path)
	pipeline := decodeJSON[bitbucket.Pipeline](t, body)
	testkit.AssertEqual(t, "UUID", pipeline.UUID, uuid)
	testkit.AssertEqual(t, "BuildNumber", pipeline.BuildNumber, 413)
	testkit.AssertEqual(t, "State.Summary()", pipeline.State.Summary(), stageRunning)
	testkit.AssertEqual(t, "IsCompleted()", pipeline.State.IsCompleted(), false)
	testkit.AssertEqual(t, "CompletedOn.IsZero()", pipeline.CompletedOn.IsZero(), true)

	// A run that has not finished carries no completed_on at all.
	if _, ok := decodeRaw(t, body)[fieldCompletedOn]; ok {
		t.Fatalf("running pipeline still carries %q: %s", fieldCompletedOn, body)
	}
}

func TestServer_GetPipelineUnknownIsNotFound(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	got := get(t, server.URL+pipelinePath(repoAPIGateway, 999), nil)

	testkit.AssertEqual(t, "status", got.Status, http.StatusNotFound)
	testkit.AssertEqual(t, "error.message", decodeJSON[errorBody](t, got.Body).Error.Message,
		"No pipeline 999 in "+Workspace+"/"+repoAPIGateway+".")
}

func TestServer_ListPipelineSteps(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	page := decodeJSON[listPage[bitbucket.PipelineStep]](t,
		getOK(t, server, pipelinePath(repoAPIGateway, 414)+segmentSteps))

	testkit.AssertEqual(t, "size", page.Size, 4)
	testkit.AssertEqual(t, "steps[0].DisplayName(0)", page.Values[0].DisplayName(0), "Build")
	testkit.AssertEqual(t, "steps[1].State.Summary()", page.Values[1].State.Summary(), resultFailed)
	testkit.AssertEqual(t, "steps[2].State.Summary()", page.Values[2].State.Summary(), resultNotRun)
	testkit.AssertEqual(t, "steps[3].State.Summary()", page.Values[3].State.Summary(), resultNotRun)

	if page.Values[1].State.Result == nil || page.Values[1].State.Result.Error == nil {
		t.Fatal("failed step carries no error")
	}
	if page.Values[1].State.Result.Error.Message == "" {
		t.Fatal("failed step error has an empty message")
	}
	// A step that never ran never started either.
	testkit.AssertEqual(t, "steps[2].StartedOn.IsZero()", page.Values[2].StartedOn.IsZero(), true)
	testkit.AssertEqual(t, "steps[0].Duration()", page.Values[0].Duration().String(), "1m1s")
}

func TestServer_GetPipelineStepLog(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	steps := decodeJSON[listPage[bitbucket.PipelineStep]](t,
		getOK(t, server, pipelinePath(repoAPIGateway, 412)+segmentSteps))
	if len(steps.Values) < 2 {
		t.Fatalf("len(steps) = %d, want at least 2", len(steps.Values))
	}

	step := steps.Values[1]
	path := pipelinePath(repoAPIGateway, 412) + segmentSteps + "/" + url.PathEscape(step.UUID) + "/log"

	got := get(t, server.URL+path, nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusOK)
	testkit.AssertEqual(t, "Content-Type", got.Header.Get("Content-Type"), contentTypeText)
	if !strings.HasPrefix(string(got.Body), "+ go test ./...") {
		t.Fatalf("log = %q, want it to start with the script command", got.Body)
	}
	testkit.AssertEqual(t, "log", string(got.Body), server.Data.Logs[step.UUID])
}

func TestServer_GetPipelineStepLogHonoursRangeTail(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	steps := decodeJSON[listPage[bitbucket.PipelineStep]](t,
		getOK(t, server, pipelinePath(repoAPIGateway, 412)+segmentSteps))
	step := steps.Values[1]
	path := pipelinePath(repoAPIGateway, 412) + segmentSteps + "/" + url.PathEscape(step.UUID) + "/log"

	full := server.Data.Logs[step.UUID]
	const tail = 40

	header := http.Header{}
	header.Set("Range", "bytes=-"+strconv.Itoa(tail))
	got := get(t, server.URL+path, header)

	testkit.AssertEqual(t, "status", got.Status, http.StatusPartialContent)
	testkit.AssertEqual(t, "len(body)", len(got.Body), tail)
	testkit.AssertEqual(t, "body", string(got.Body), full[len(full)-tail:])
	testkit.AssertEqual(t, "Content-Range", got.Header.Get("Content-Range"),
		"bytes "+strconv.Itoa(len(full)-tail)+"-"+strconv.Itoa(len(full)-1)+
			"/"+strconv.Itoa(len(full)))
}

func TestServer_GetPipelineStepLogMissingIsNotFound(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	// The last step of build 414 was NOT_RUN, so it has no log.
	steps := decodeJSON[listPage[bitbucket.PipelineStep]](t,
		getOK(t, server, pipelinePath(repoAPIGateway, 414)+segmentSteps))
	step := steps.Values[len(steps.Values)-1]
	path := pipelinePath(repoAPIGateway, 414) + segmentSteps + "/" + url.PathEscape(step.UUID) + "/log"

	got := get(t, server.URL+path, nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusNotFound)
	testkit.AssertEqual(t, "type", decodeJSON[errorBody](t, got.Body).Type, "error")
}

func TestServer_UnknownPathIsNotFoundEnvelope(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	got := get(t, server.URL+"/does-not-exist", nil)

	testkit.AssertEqual(t, "status", got.Status, http.StatusNotFound)
	testkit.AssertEqual(t, "Content-Type", got.Header.Get("Content-Type"), contentTypeJSON)
	testkit.AssertEqual(t, "type", decodeJSON[errorBody](t, got.Body).Type, "error")
}

func TestServer_AcceptsAPIVersionPrefix(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	account := decodeJSON[bitbucket.Account](t, getOK(t, server, apiPrefix+pathUser))

	testkit.AssertEqual(t, "UUID", account.UUID, uuidDemo)
}

func TestServer_LastAuthorization(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	testkit.AssertEqual(t, "before any request", server.LastAuthorization(), "")

	header := http.Header{}
	header.Set(headerAuthorization, demoAuthorization)
	got := get(t, server.URL+pathUser, header)

	testkit.AssertEqual(t, "status", got.Status, http.StatusOK)
	testkit.AssertEqual(t, "LastAuthorization()", server.LastAuthorization(), demoAuthorization)

	// A later request without the header leaves the recorded value alone.
	_ = get(t, server.URL+pathUser, nil)
	testkit.AssertEqual(t, "LastAuthorization() after anonymous request",
		server.LastAuthorization(), demoAuthorization)
}

// TestWireKeysAreLowercase is what lets the encoder skip any key rewriting: it
// walks every JSON body the server serves and fails on a key that starts with a
// capital, which is what encoding/json would emit for a struct field with no
// json tag.
func TestWireKeysAreLowercase(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	paths := []string{
		pathUser,
		pathWorkspace,
		pathRepositories,
		pathRepositories + "/" + repoAPIGateway,
		pullRequestsPath(repoAPIGateway),
		pullRequestPath(repoAPIGateway, 103),
		pullRequestPath(repoAPIGateway, 103) + "/statuses",
		pipelinesPath(repoAPIGateway),
		pipelinePath(repoAPIGateway, 413),
		pipelinePath(repoAPIGateway, 413) + segmentSteps,
		pullRequestPath(repoAPIGateway, 103) + segmentCommits,
		pullRequestPath(repoAPIGateway, 103) + segmentComments,
	}

	for _, path := range paths {
		var body any
		if err := json.Unmarshal(getOK(t, server, path), &body); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		assertLowercaseKeys(t, path, body)
	}

	// The diffstat is served by the redirect target, which is addressed by the
	// absolute link the dataset carries rather than by a path.
	href := demoPullRequest(t, server, repoAPIGateway, 103).Links.DiffStat.Href
	var diffstat any
	if err := json.Unmarshal(get(t, href, nil).Body, &diffstat); err != nil {
		t.Fatalf("decode %s: %v", href, err)
	}
	assertLowercaseKeys(t, href, diffstat)
}

func assertLowercaseKeys(t *testing.T, path string, value any) {
	t.Helper()

	switch typed := value.(type) {
	case map[string]any:
		for key, member := range typed {
			if key != "" && key[0] >= 'A' && key[0] <= 'Z' {
				t.Errorf("%s: key %q starts with a capital, so its field has no json tag", path, key)
			}
			assertLowercaseKeys(t, path, member)
		}
	case []any:
		for _, member := range typed {
			assertLowercaseKeys(t, path, member)
		}
	}
}
