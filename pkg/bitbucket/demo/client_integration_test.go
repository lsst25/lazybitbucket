package demo

import (
	"strconv"
	"strings"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// These tests drive the demo server through the real bitbucket.Client instead of
// raw HTTP, so they cover the two halves together: the wire shapes this package
// produces and the decoding, pagination, query building and error mapping in
// pkg/bitbucket. Every route in ClientInterface is exercised at least once.

const unknownSlug = "no-such-repo"

// newDemoClient points a real client at a demo server that is closed with the
// test.
func newDemoClient(t *testing.T, server *Server) *bitbucket.Client {
	t.Helper()

	return bitbucket.NewClient(bitbucket.ClientOpts{
		BaseURL:    server.URL,
		Token:      "demo-token",
		Email:      "",
		UserAgent:  "",
		HTTPClient: nil,
	})
}

func TestClientGetCurrentUserAndWorkspace(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	client := newDemoClient(t, server)

	user, err := client.GetCurrentUser(t.Context())
	if err != nil {
		t.Fatalf("GetCurrentUser: %v", err)
	}
	testkit.AssertEqual(t, "user.UUID", user.UUID, uuidDemo)
	testkit.AssertEqual(t, "user.Nickname", user.Nickname, "demo")
	testkit.AssertEqual(t, "user.Links.Avatar.Href",
		user.Links.Avatar.Href, htmlBase+"/account/demo/avatar/")

	workspace, err := client.GetWorkspace(t.Context(), Workspace)
	if err != nil {
		t.Fatalf("GetWorkspace: %v", err)
	}
	testkit.AssertEqual(t, "workspace.Slug", workspace.Slug, Workspace)
	testkit.AssertEqual(t, "workspace.Name", workspace.Name, "Acme Corp")

	testkit.AssertEqual(t, "LastAuthorization()", server.LastAuthorization(), demoAuthorization)
}

func TestClientGetWorkspaceUnknownIsNotFound(t *testing.T) {
	t.Parallel()

	client := newDemoClient(t, newTestServer(t))

	workspace, err := client.GetWorkspace(t.Context(), "not-acme")
	if err == nil {
		t.Fatalf("GetWorkspace(not-acme) = %+v, want a not-found error", workspace)
	}
	if !bitbucket.IsNotFound(err) {
		t.Fatalf("IsNotFound(%v) = false, want true", err)
	}
}

func TestClientListAndGetRepositories(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	client := newDemoClient(t, server)

	repositories, err := client.ListRepositories(t.Context(), Workspace,
		bitbucket.ListRepositoriesOptions{Role: "", Query: "", Sort: "", MaxItems: 0})
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	// Newest update first, which is the order the repository picker shows.
	testkit.AssertSliceEqual(t, "repository slugs", slugsOf(repositories),
		[]string{repoAPIGateway, repoWebApp, repoInfra})

	repository, err := client.GetRepository(t.Context(), Workspace, repoInfra)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	testkit.AssertEqual(t, "repository.FullName", repository.FullName, Workspace+"/"+repoInfra)
	testkit.AssertEqual(t, "repository.MainBranch", repository.MainBranch.Name, branchMain)
	testkit.AssertEqual(t, "len(repository.Links.Clone)", len(repository.Links.Clone), 2)

	_, err = client.GetRepository(t.Context(), Workspace, unknownSlug)
	if !bitbucket.IsNotFound(err) {
		t.Fatalf("GetRepository(%s) error = %v, want a not-found error", unknownSlug, err)
	}
}

// TestClientListPullRequestsFollowsNextLinks lowers the server's page ceiling
// below what the client asks for, so the five api-gateway pull requests arrive
// over three pages and the client has to follow two "next" links.
func TestClientListPullRequestsFollowsNextLinks(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	server.SetMaxPageLen(2)
	client := newDemoClient(t, server)

	// ListPullRequests issues its pages one after another, so counting them from
	// the hook needs no synchronisation.
	requests := 0
	client.SetOnRequest(func(_ bitbucket.RequestLog) { requests++ })

	pullRequests, err := client.ListPullRequests(t.Context(), Workspace, repoAPIGateway,
		bitbucket.ListPullRequestsOptions{
			States: []bitbucket.PullRequestState{
				bitbucket.PullRequestOpen,
				bitbucket.PullRequestMerged,
				bitbucket.PullRequestDeclined,
			},
			Query:    "",
			Sort:     "",
			MaxItems: 5,
			Fields:   "",
		})
	if err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}

	testkit.AssertSliceEqual(t, "pull request ids", idsOf(pullRequests), []int{103, 102, 101, 98, 95})
	testkit.AssertEqual(t, "requests issued", requests, 3)
	for index := 1; index < len(pullRequests); index++ {
		if pullRequests[index-1].UpdatedOn.Before(pullRequests[index].UpdatedOn) {
			t.Fatalf("pull request %d is newer than its predecessor %d",
				pullRequests[index].ID, pullRequests[index-1].ID)
		}
	}
	// The default fields projection asks for review data, and the demo server
	// serves it on list items.
	if !pullRequests[0].HasReviewData() {
		t.Error("pull request 103 came back without review data")
	}
}

func TestClientListPullRequestsStates(t *testing.T) {
	t.Parallel()

	client := newDemoClient(t, newTestServer(t))

	open, err := client.ListPullRequests(t.Context(), Workspace, repoAPIGateway,
		bitbucket.ListPullRequestsOptions{States: nil, Query: "", Sort: "", MaxItems: 0, Fields: ""})
	if err != nil {
		t.Fatalf("ListPullRequests(default): %v", err)
	}
	testkit.AssertSliceEqual(t, "open pull request ids", idsOf(open), []int{103, 102, 101})

	merged, err := client.ListPullRequests(t.Context(), Workspace, repoAPIGateway,
		bitbucket.ListPullRequestsOptions{
			States:   []bitbucket.PullRequestState{bitbucket.PullRequestMerged},
			Query:    "",
			Sort:     "",
			MaxItems: 0,
			Fields:   "",
		})
	if err != nil {
		t.Fatalf("ListPullRequests(MERGED): %v", err)
	}
	testkit.AssertSliceEqual(t, "merged pull request ids", idsOf(merged), []int{98})
	testkit.AssertEqual(t, "merge commit", merged[0].MergeCommit.Hash, shortHash(hashMergePR98))
}

func TestClientGetPullRequest(t *testing.T) {
	t.Parallel()

	client := newDemoClient(t, newTestServer(t))

	pullRequest, err := client.GetPullRequest(t.Context(), Workspace, repoWebApp, 40)
	if err != nil {
		t.Fatalf("GetPullRequest: %v", err)
	}
	testkit.AssertEqual(t, "pull request key", pullRequest.Key(),
		bitbucket.PullRequestKey(Workspace, repoWebApp, 40))
	testkit.AssertEqual(t, "source branch", pullRequest.SourceBranch(), "feature/dark-mode")
	testkit.AssertEqual(t, "destination branch", pullRequest.DestinationBranch(), branchDevelop)
	testkit.AssertEqual(t, "approvals", pullRequest.ReviewSummary().Approved, 1)
	testkit.AssertEqual(t, "pending reviewers", pullRequest.ReviewSummary().Pending, 1)

	_, err = client.GetPullRequest(t.Context(), Workspace, repoWebApp, 4040)
	if !bitbucket.IsNotFound(err) {
		t.Fatalf("GetPullRequest(4040) error = %v, want a not-found error", err)
	}
}

func TestClientListPullRequestStatuses(t *testing.T) {
	t.Parallel()

	client := newDemoClient(t, newTestServer(t))

	// Two statuses under different keys on the same commit: a stopped pipeline
	// run and a successful end-to-end suite.
	statuses, err := client.ListPullRequestStatuses(t.Context(), Workspace, repoWebApp, 40)
	if err != nil {
		t.Fatalf("ListPullRequestStatuses: %v", err)
	}
	testkit.AssertEqual(t, "len(statuses)", len(statuses), 2)
	testkit.AssertEqual(t, "rollup", bitbucket.RollupStatuses(statuses), bitbucket.StatusStopped)

	failing, err := client.ListPullRequestStatuses(t.Context(), Workspace, repoAPIGateway, 103)
	if err != nil {
		t.Fatalf("ListPullRequestStatuses(103): %v", err)
	}
	testkit.AssertEqual(t, "rollup", bitbucket.RollupStatuses(failing), bitbucket.StatusFailed)
	buildNumber, ok := failing[0].BuildNumber()
	if !ok {
		t.Fatalf("status url %q carries no build number", failing[0].URL)
	}
	testkit.AssertEqual(t, "build number", buildNumber, 414)
}

func TestClientListPipelinesByTargetBranch(t *testing.T) {
	t.Parallel()

	client := newDemoClient(t, newTestServer(t))

	pipelines, err := client.ListPipelines(t.Context(), Workspace, repoAPIGateway,
		bitbucket.ListPipelinesOptions{TargetBranch: "feature/rate-limiter", Sort: "", MaxItems: 0})
	if err != nil {
		t.Fatalf("ListPipelines: %v", err)
	}
	testkit.AssertEqual(t, "len(pipelines)", len(pipelines), 1)
	testkit.AssertEqual(t, "build number", pipelines[0].BuildNumber, 412)
	testkit.AssertEqual(t, "state", pipelines[0].State.Summary(), resultSuccessful)

	all, err := client.ListPipelines(t.Context(), Workspace, repoAPIGateway,
		bitbucket.ListPipelinesOptions{TargetBranch: "", Sort: "", MaxItems: 0})
	if err != nil {
		t.Fatalf("ListPipelines(all): %v", err)
	}
	testkit.AssertSliceEqual(t, "build numbers", buildNumbersOf(all), []int{414, 413, 412, 411})
	// The running run has no completed_on, and the demo server omits the field
	// rather than sending a zero timestamp.
	testkit.AssertEqual(t, "413 state", all[1].State.Summary(), stageRunning)
	if !all[1].CompletedOn.IsZero() {
		t.Errorf("running pipeline CompletedOn = %s, want the zero time", all[1].CompletedOn)
	}
}

func TestClientGetPipelineByUUIDAndBuildNumber(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	client := newDemoClient(t, server)
	uuid := findPipeline(t, server.Data, repoAPIGateway, 412).UUID

	byUUID, err := client.GetPipeline(t.Context(), Workspace, repoAPIGateway, uuid)
	if err != nil {
		t.Fatalf("GetPipeline(uuid): %v", err)
	}
	byNumber, err := client.GetPipeline(t.Context(), Workspace, repoAPIGateway, strconv.Itoa(412))
	if err != nil {
		t.Fatalf("GetPipeline(build number): %v", err)
	}
	testkit.AssertEqual(t, "uuid", byUUID.UUID, uuid)
	testkit.AssertEqual(t, "same run", byNumber.UUID, byUUID.UUID)
	testkit.AssertEqual(t, "HTMLURL()", byUUID.HTMLURL(),
		htmlBase+"/"+Workspace+"/"+repoAPIGateway+"/pipelines/results/412")

	_, err = client.GetPipeline(t.Context(), Workspace, repoAPIGateway, "999999")
	if !bitbucket.IsNotFound(err) {
		t.Fatalf("GetPipeline(999999) error = %v, want a not-found error", err)
	}
}

func TestClientListPipelineStepsAndLog(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	client := newDemoClient(t, server)
	uuid := findPipeline(t, server.Data, repoAPIGateway, 412).UUID

	steps, err := client.ListPipelineSteps(t.Context(), Workspace, repoAPIGateway, uuid)
	if err != nil {
		t.Fatalf("ListPipelineSteps: %v", err)
	}
	testkit.AssertEqual(t, "len(steps)", len(steps), 3)
	names := make([]string, 0, len(steps))
	for index, step := range steps {
		names = append(names, step.DisplayName(index))
	}
	testkit.AssertSliceEqual(t, "step names", names, []string{"Build", "Unit tests", "Publish image"})

	whole, err := client.GetPipelineStepLog(t.Context(), Workspace, repoAPIGateway, uuid,
		steps[1].UUID, bitbucket.StepLogOptions{TailBytes: 0})
	if err != nil {
		t.Fatalf("GetPipelineStepLog: %v", err)
	}
	testkit.AssertEqual(t, "log", whole, server.Data.Logs[steps[1].UUID])
	if !strings.Contains(whole, "internal/ratelimit") {
		t.Errorf("log = %q, want the unit test output in it", whole)
	}

	const tail = 40
	tailed, err := client.GetPipelineStepLog(t.Context(), Workspace, repoAPIGateway, uuid,
		steps[1].UUID, bitbucket.StepLogOptions{TailBytes: tail})
	if err != nil {
		t.Fatalf("GetPipelineStepLog(tail): %v", err)
	}
	testkit.AssertEqual(t, "len(tail)", len(tailed), tail)
	testkit.AssertEqual(t, "tail", tailed, whole[len(whole)-tail:])
}

func TestClientGetPipelineStepLogMissingIsNotFound(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	client := newDemoClient(t, server)
	uuid := findPipeline(t, server.Data, repoAPIGateway, 412).UUID

	_, err := client.GetPipelineStepLog(t.Context(), Workspace, repoAPIGateway, uuid,
		"{00000000-0000-0000-0000-00000000dead}", bitbucket.StepLogOptions{TailBytes: 0})
	if !bitbucket.IsNotFound(err) {
		t.Fatalf("GetPipelineStepLog(unknown step) error = %v, want a not-found error", err)
	}
}

func TestClientFetchPullRequestsAcrossRepositories(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	client := newDemoClient(t, server)

	repos := []string{repoAPIGateway, repoWebApp, repoInfra, unknownSlug}
	results := bitbucket.FetchPullRequests(t.Context(), client, Workspace, repos,
		bitbucket.ListPullRequestsOptions{States: nil, Query: "", Sort: "", MaxItems: 0, Fields: ""}, 0)

	testkit.AssertEqual(t, "len(results)", len(results), len(repos))
	testkit.AssertSliceEqual(t, "result order",
		[]string{results[0].Repo, results[1].Repo, results[2].Repo, results[3].Repo}, repos)
	testkit.AssertEqual(t, "len(api-gateway)", len(results[0].PullRequests), 3)
	testkit.AssertEqual(t, "len(web-app)", len(results[1].PullRequests), 2)
	testkit.AssertEqual(t, "len(infra)", len(results[2].PullRequests), 1)

	// One unreadable repository fails on its own without blanking the others.
	if !bitbucket.IsNotFound(results[3].Err) {
		t.Fatalf("%s error = %v, want a not-found error", unknownSlug, results[3].Err)
	}

	merged := bitbucket.MergePullRequests(results)
	testkit.AssertEqual(t, "len(merged)", len(merged), 6)
	for index := 1; index < len(merged); index++ {
		if merged[index-1].UpdatedOn.Before(merged[index].UpdatedOn) {
			t.Fatalf("merged list is not newest-first at %d", index)
		}
	}
	testkit.AssertEqual(t, "LastAuthorization()", server.LastAuthorization(), demoAuthorization)
}

// -----------------------------------------------------------------------------
// Review data
// -----------------------------------------------------------------------------

func TestClientListPullRequestCommits(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	client := newDemoClient(t, server)

	commits, err := client.ListPullRequestCommits(t.Context(), Workspace, repoAPIGateway, 103,
		bitbucket.ListOptions{MaxItems: 0})
	if err != nil {
		t.Fatalf("ListPullRequestCommits: %v", err)
	}

	testkit.AssertEqual(t, "len(commits)", len(commits), 3)
	testkit.AssertEqual(t, "commits[0].Hash", commits[0].Hash, hashPR103)
	testkit.AssertEqual(t, "commits[0].ShortHash()", commits[0].ShortHash(), hashPR103[:7])
	testkit.AssertEqual(t, "commits[0].AuthorName()", commits[0].AuthorName(), "Linus Torvalds")
	testkit.AssertEqual(t, "commits[0].Subject()", commits[0].Subject(),
		"upstream: derive a fresh deadline per retry attempt")
	if !strings.Contains(commits[0].Message, "Fixes INC-4471.") {
		t.Errorf("commits[0].Message = %q, want the body after the subject", commits[0].Message)
	}
	testkit.AssertEqual(t, "commits[0].HTMLURL()", commits[0].HTMLURL(),
		htmlBase+"/"+Workspace+"/"+repoAPIGateway+"/commits/"+hashPR103)

	// A merged pull request has no source branch left, so the list is empty
	// rather than an error.
	merged, err := client.ListPullRequestCommits(t.Context(), Workspace, repoAPIGateway, 98,
		bitbucket.ListOptions{MaxItems: 0})
	if err != nil {
		t.Fatalf("ListPullRequestCommits(#98): %v", err)
	}
	testkit.AssertEqual(t, "len(merged commits)", len(merged), 0)
}

// TestClientListPullRequestCommitsFollowsTheOpaqueNext is the iterator
// pagination path: the envelope has no size and its next link carries a token,
// so the client can only get the second page by following the link.
func TestClientListPullRequestCommitsFollowsTheOpaqueNext(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	server.SetMaxPageLen(2)
	client := newDemoClient(t, server)

	requests := 0
	client.SetOnRequest(func(_ bitbucket.RequestLog) { requests++ })

	commits, err := client.ListPullRequestCommits(t.Context(), Workspace, repoWebApp, 40,
		bitbucket.ListOptions{MaxItems: 0})
	if err != nil {
		t.Fatalf("ListPullRequestCommits: %v", err)
	}

	testkit.AssertEqual(t, "len(commits)", len(commits), 4)
	testkit.AssertEqual(t, "requests", requests, 2)
	testkit.AssertEqual(t, "commits[0].Hash", commits[0].Hash, hashPR40)
	testkit.AssertEqual(t, "commits[3].AuthorName()", commits[3].AuthorName(), "Barbara Liskov")
}

func TestClientListPullRequestCommentsAndThreads(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	client := newDemoClient(t, server)

	comments, err := client.ListPullRequestComments(t.Context(), Workspace, repoAPIGateway, 103,
		bitbucket.ListOptions{MaxItems: 0})
	if err != nil {
		t.Fatalf("ListPullRequestComments: %v", err)
	}
	testkit.AssertEqual(t, "len(comments)", len(comments), 9)

	threads := bitbucket.BuildThreads(comments)
	testkit.AssertEqual(t, "len(threads)", len(threads), 5)

	// Inline threads first, down the file: the resolved one on line 13, then the
	// unresolved one on line 16 with its reply, then the pending draft.
	testkit.AssertEqual(t, "threads[0].Resolved()", threads[0].Resolved(), true)
	testkit.AssertEqual(t, "threads[1].Path()", threads[1].Path(), pathRetryGo)
	testkit.AssertEqual(t, "threads[1] replies", len(threads[1].Replies), 1)
	testkit.AssertEqual(t, "threads[1] reply author",
		threads[1].Replies[0].User.Nickname, "linus")
	testkit.AssertEqual(t, "threads[2].Root.Pending", threads[2].Root.Pending, true)
	testkit.AssertEqual(t, "threads[3].Root.Deleted", threads[3].Root.Deleted, true)
	testkit.AssertEqual(t, "threads[4] replies", len(threads[4].Replies), 2)
	if threads[4].LastActivity().Before(threads[4].Root.CreatedOn) {
		t.Errorf("thread LastActivity() = %v, older than its root", threads[4].LastActivity())
	}

	// The markers the diff view draws, keyed by new-side line number.
	lines := bitbucket.CommentedLines(threads)
	testkit.AssertEqual(t, "threads on retry.go:16", lines[pathRetryGo][16], 1)
	testkit.AssertEqual(t, "threads on retry.go:13", lines[pathRetryGo][13], 1)

	// The file-level anchor on another pull request lands under line 0.
	fileLevel, err := client.ListPullRequestComments(t.Context(), Workspace, repoWebApp, 40,
		bitbucket.ListOptions{MaxItems: 0})
	if err != nil {
		t.Fatalf("ListPullRequestComments(#40): %v", err)
	}
	testkit.AssertEqual(t, "file-level marker",
		bitbucket.CommentedLines(bitbucket.BuildThreads(fileLevel))[pathThemeTS][0], 1)
}

// TestClientListPullRequestDiffStat goes through the 302: the client asks the
// pull request endpoint and the response it decodes comes from the repository
// revspec resource, whose own next links it then follows.
func TestClientListPullRequestDiffStat(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	client := newDemoClient(t, server)

	entries, err := client.ListPullRequestDiffStat(t.Context(), Workspace, repoAPIGateway, 103)
	if err != nil {
		t.Fatalf("ListPullRequestDiffStat: %v", err)
	}

	testkit.AssertSliceEqual(t, "paths", pathsOf(entries),
		[]string{pathRetryGo, pathRetryTestGo, "docs/rfc-014.md"})
	testkit.AssertEqual(t, "entries[0].Status", entries[0].Status, bitbucket.DiffStatModified)
	testkit.AssertEqual(t, "entries[0].LinesAdded", entries[0].LinesAdded, 6)
	testkit.AssertEqual(t, "entries[0].LinesRemoved", entries[0].LinesRemoved, 3)
	testkit.AssertEqual(t, "entries[1].Status", entries[1].Status, bitbucket.DiffStatAdded)
	testkit.AssertEqual(t, "entries[1].OldPath()", entries[1].OldPath(), "")
	testkit.AssertEqual(t, "LastAuthorization()", server.LastAuthorization(), demoAuthorization)

	// The rename and the binary file live on other pull requests.
	renamed, err := client.ListPullRequestDiffStat(t.Context(), Workspace, repoAPIGateway, 102)
	if err != nil {
		t.Fatalf("ListPullRequestDiffStat(#102): %v", err)
	}
	testkit.AssertEqual(t, "renamed status", renamed[0].Status, bitbucket.DiffStatRenamed)
	testkit.AssertEqual(t, "renamed old path", renamed[0].OldPath(), "internal/auth/token.go")
	testkit.AssertEqual(t, "renamed new path", renamed[0].Path(), "internal/auth/introspect.go")

	binary, err := client.ListPullRequestDiffStat(t.Context(), Workspace, repoWebApp, 40)
	if err != nil {
		t.Fatalf("ListPullRequestDiffStat(#40): %v", err)
	}
	last := binary[len(binary)-1]
	testkit.AssertEqual(t, "binary path", last.Path(), "public/screenshots/dark.png")
	testkit.AssertEqual(t, "binary IsBinary()", last.IsBinary(), true)
	testkit.AssertEqual(t, "binary LinesAdded", last.LinesAdded, 0)
}

// TestClientListPullRequestDiffStatPaginatesThroughTheRedirect makes the target
// hand back more than one page, so the paginator has to follow a next link that
// points at the redirected-to resource rather than at the pull request.
func TestClientListPullRequestDiffStatPaginatesThroughTheRedirect(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	server.SetMaxPageLen(1)
	client := newDemoClient(t, server)

	entries, err := client.ListPullRequestDiffStat(t.Context(), Workspace, repoAPIGateway, 103)
	if err != nil {
		t.Fatalf("ListPullRequestDiffStat: %v", err)
	}
	testkit.AssertSliceEqual(t, "paths", pathsOf(entries),
		[]string{pathRetryGo, pathRetryTestGo, "docs/rfc-014.md"})
}

// TestClientGetPullRequestDiffUsesTheSameOriginLink covers the shortcut the app
// takes: the pull request payload's diff link is same-origin, so the client
// requests it directly and merges path= into whatever it already carries.
func TestClientGetPullRequestDiffUsesTheSameOriginLink(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	client := newDemoClient(t, server)
	pullRequest := demoPullRequest(t, server, repoAPIGateway, 103)

	whole, err := client.GetPullRequestDiff(t.Context(), Workspace, repoAPIGateway, 103,
		bitbucket.DiffOptions{Path: "", Context: 0, DiffURL: pullRequest.Links.Diff.Href})
	if err != nil {
		t.Fatalf("GetPullRequestDiff: %v", err)
	}
	testkit.AssertEqual(t, "diff", whole, server.Data.Diffs[reviewKey(repoAPIGateway, 103)])

	oneFile, err := client.GetPullRequestDiff(t.Context(), Workspace, repoAPIGateway, 103,
		bitbucket.DiffOptions{Path: pathRetryGo, Context: 0, DiffURL: pullRequest.Links.Diff.Href})
	if err != nil {
		t.Fatalf("GetPullRequestDiff(path): %v", err)
	}
	testkit.AssertEqual(t, "sections", strings.Count(oneFile, "diff --git "), 1)
	if !strings.Contains(oneFile, pathRetryGo) {
		t.Fatalf("per-file diff does not mention %s:\n%s", pathRetryGo, oneFile)
	}
}

// TestClientGetPullRequestDiffFiltersByPathThroughTheRedirect is the case a
// followed redirect got wrong: the Location carries only the server's own
// parameters, so path= has to be merged onto it by the client or the second hop
// answers with every file in the pull request.
func TestClientGetPullRequestDiffFiltersByPathThroughTheRedirect(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	client := newDemoClient(t, server)
	const routerGo = "internal/router/router.go"

	whole, err := client.GetPullRequestDiff(t.Context(), Workspace, repoAPIGateway, 101,
		bitbucket.DiffOptions{Path: "", Context: 0, DiffURL: ""})
	if err != nil {
		t.Fatalf("GetPullRequestDiff: %v", err)
	}
	if strings.Count(whole, "diff --git ") < 2 {
		t.Fatalf("the fixture is not the multi-file diff this test needs:\n%s", whole)
	}

	oneFile, err := client.GetPullRequestDiff(t.Context(), Workspace, repoAPIGateway, 101,
		bitbucket.DiffOptions{Path: routerGo, Context: 0, DiffURL: ""})
	if err != nil {
		t.Fatalf("GetPullRequestDiff(path): %v", err)
	}

	testkit.AssertEqual(t, "sections", strings.Count(oneFile, "diff --git "), 1)
	if !strings.Contains(oneFile, routerGo) {
		t.Fatalf("per-file diff does not mention %s:\n%s", routerGo, oneFile)
	}
}

// TestClientGetPullRequestDiffFollowsTheRedirect is the same fetch without the
// link: the pull request endpoint 302s and the Authorization header has to
// survive the hop, or the target would answer as an anonymous request.
func TestClientGetPullRequestDiffFollowsTheRedirect(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	client := newDemoClient(t, server)

	whole, err := client.GetPullRequestDiff(t.Context(), Workspace, repoAPIGateway, 101,
		bitbucket.DiffOptions{Path: "", Context: 0, DiffURL: ""})
	if err != nil {
		t.Fatalf("GetPullRequestDiff: %v", err)
	}
	testkit.AssertEqual(t, "diff", whole, server.Data.Diffs[reviewKey(repoAPIGateway, 101)])
	testkit.AssertEqual(t, "LastAuthorization()", server.LastAuthorization(), demoAuthorization)

	// A diff link on another host is refused, and the request falls back to the
	// pull request endpoint instead of leaking the token.
	foreign, err := client.GetPullRequestDiff(t.Context(), Workspace, repoAPIGateway, 101,
		bitbucket.DiffOptions{
			Path:    "",
			Context: 0,
			DiffURL: apiBase + "/repositories/" + Workspace + "/" + repoAPIGateway + "/diff/whatever",
		})
	if err != nil {
		t.Fatalf("GetPullRequestDiff(foreign link): %v", err)
	}
	testkit.AssertEqual(t, "diff via fallback", foreign, whole)
}

// TestClientDiffTooLargeIsToldApart is the 555: not a transient 5xx, and both
// diff methods report it the same way.
func TestClientDiffTooLargeIsToldApart(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	client := newDemoClient(t, server)
	pullRequest := demoPullRequest(t, server, repoAPIGateway, 98)

	_, err := client.ListPullRequestDiffStat(t.Context(), Workspace, repoAPIGateway, 98)
	if !bitbucket.IsDiffTooLarge(err) {
		t.Fatalf("ListPullRequestDiffStat(#98) error = %v, want a diff-too-large error", err)
	}

	_, err = client.GetPullRequestDiff(t.Context(), Workspace, repoAPIGateway, 98,
		bitbucket.DiffOptions{Path: "", Context: 0, DiffURL: pullRequest.Links.Diff.Href})
	if !bitbucket.IsDiffTooLarge(err) {
		t.Fatalf("GetPullRequestDiff(#98) error = %v, want a diff-too-large error", err)
	}

	_, err = client.GetPullRequestDiff(t.Context(), Workspace, repoAPIGateway, 98,
		bitbucket.DiffOptions{Path: "", Context: 0, DiffURL: ""})
	if !bitbucket.IsDiffTooLarge(err) {
		t.Fatalf("GetPullRequestDiff(#98, via redirect) error = %v, want a diff-too-large error", err)
	}
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

func slugsOf(repositories []bitbucket.Repository) []string {
	slugs := make([]string, 0, len(repositories))
	for _, repository := range repositories {
		slugs = append(slugs, repository.Slug)
	}
	return slugs
}

func idsOf(pullRequests []bitbucket.PullRequest) []int {
	ids := make([]int, 0, len(pullRequests))
	for _, pullRequest := range pullRequests {
		ids = append(ids, pullRequest.ID)
	}
	return ids
}

func buildNumbersOf(pipelines []bitbucket.Pipeline) []int {
	numbers := make([]int, 0, len(pipelines))
	for _, pipeline := range pipelines {
		numbers = append(numbers, pipeline.BuildNumber)
	}
	return numbers
}
