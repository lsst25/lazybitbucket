package bitbucket

import (
	"net/http"
	"strings"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// statusesEnvelope holds two statuses on the same commit under different keys —
// the shape that makes RollupStatuses necessary.
const statusesEnvelope = `{
  "pagelen": 100,
  "page": 1,
  "values": [
    {
      "key": "prs:**:feature/rate-limit",
      "type": "build",
      "state": "STOPPED",
      "name": "Pipeline - pullrequests: **",
      "description": "",
      "refname": "feature/rate-limit",
      "url": "https://bitbucket.org/acme/api-gateway/pipelines/results/10471",
      "commit": {"type": "commit", "hash": "3f1a2b3c4d5e6f708192a3b4c5d6e7f809a1b2c3",
                 "links": {"self": {"href": "s"}, "html": {"href": "h"}}},
      "repository": {"type": "repository", "name": "api-gateway", "full_name": "acme/api-gateway",
                     "uuid": "{85dc8e4b}", "links": {"self": {"href": "s"}, "html": {"href": "h"},
                                                     "avatar": {"href": "a"}}},
      "created_on": "2026-09-03T14:27:01.326890+00:00",
      "updated_on": "2026-09-03T14:28:44.231234+00:00",
      "links": {"self": {"href": "s"}, "commit": {"href": "c"}}
    },
    {
      "key": "prs:**:develop",
      "type": "build",
      "state": "SUCCESSFUL",
      "name": "Pipeline - pullrequests: **",
      "description": "",
      "refname": "feature/rate-limit",
      "url": "https://bitbucket.org/acme/api-gateway/pipelines/results/10474",
      "commit": {"type": "commit", "hash": "3f1a2b3c4d5e6f708192a3b4c5d6e7f809a1b2c3",
                 "links": {"self": {"href": "s"}, "html": {"href": "h"}}},
      "created_on": "2026-09-03T14:29:01.326890+00:00",
      "updated_on": "2026-09-03T14:35:44.231234+00:00",
      "links": {"self": {"href": "s"}, "commit": {"href": "c"}}
    }
  ]
}`

// pipelinesEnvelope holds a pull-request-target FAILED run and a branch-target
// STOPPED run with a terminator.
const pipelinesEnvelope = `{
  "pagelen": 10,
  "page": 1,
  "values": [
    {
      "type": "pipeline",
      "uuid": "{210f1f44-67d9-4b63-b755-7f46d0af56d4}",
      "build_number": 9445,
      "state": {"name": "COMPLETED", "type": "pipeline_state_completed",
                "result": {"name": "FAILED", "type": "pipeline_state_completed_failed"}},
      "creator": {"type": "user", "uuid": "{e5743438-cc31-49a0-96b7-9febd7b946f4}",
                  "display_name": "Ada Lovelace", "nickname": "ada",
                  "links": {"self": {"href": "s"}, "html": {"href": "h"}, "avatar": {"href": "a"}}},
      "repository": {"type": "repository", "name": "api-gateway", "full_name": "acme/api-gateway",
                     "uuid": "{85dc8e4b}", "links": {"self": {"href": "s"}, "html": {"href": "h"},
                                                     "avatar": {"href": "a"}}},
      "target": {
        "type": "pipeline_pullrequest_target",
        "selector": {"type": "pull-requests", "pattern": "**"},
        "commit": {"type": "commit", "hash": "3f1a2b3c4d5e6f708192a3b4c5d6e7f809a1b2c3"},
        "source": "feature/rate-limit",
        "destination": "main",
        "destination_commit": {"type": "commit", "hash": "ef5690e8743c"},
        "pullrequest": {"type": "pullrequest", "id": 101, "title": "Add rate limiting to the gateway",
                        "draft": false, "queued": false,
                        "links": {"self": {"href": "s"}, "html": {"href": "h"}}}
      },
      "trigger": {"name": "PUSH", "type": "pipeline_trigger_push"},
      "created_on": "2026-07-02T08:49:21.053693027Z",
      "completed_on": "2026-07-02T08:50:44.957762592Z",
      "run_number": 1,
      "duration_in_seconds": 66,
      "build_seconds_used": 100,
      "first_successful": false,
      "expired": false,
      "has_variables": false,
      "links": {"self": {"href": "s"}, "steps": {"href": "st"}}
    },
    {
      "type": "pipeline",
      "uuid": "{094f94a2-f2d3-4355-8260-4ac0498ff517}",
      "build_number": 26,
      "state": {"name": "COMPLETED", "type": "pipeline_state_completed",
                "result": {"name": "STOPPED", "type": "pipeline_state_completed_stopped",
                           "terminator": {"type": "user", "uuid": "{781a664d-34db-4bc7-9c0d-bd3716d0d1de}",
                                          "display_name": "Linus Torvalds", "nickname": "linus",
                                          "links": {"self": {"href": "s"}, "html": {"href": "h"},
                                                    "avatar": {"href": "a"}}}}},
      "target": {"type": "pipeline_ref_target", "ref_type": "branch", "ref_name": "kube/deploy-test",
                 "selector": {"type": "custom", "pattern": "build"},
                 "commit": {"type": "commit", "hash": "aaaabbbbccccddddeeeeffff0000111122223333"}},
      "trigger": {"name": "MANUAL", "type": "pipeline_trigger_manual"},
      "created_on": "2020-09-23T14:03:16.2Z",
      "completed_on": "2020-09-23T14:08:33.031Z",
      "run_number": 1,
      "duration_in_seconds": 309,
      "build_seconds_used": 618
    }
  ]
}`

// stepsEnvelope pairs a modern failed step with a legacy one that carries
// neither a name nor teardown commands.
const stepsEnvelope = `{
  "pagelen": 100,
  "values": [
    {
      "type": "pipeline_step",
      "uuid": "{9520b082-5efb-45c5-8538-9aeab8f74301}",
      "name": "Cancel Previous Pipelines",
      "trigger": {"type": "pipeline_step_trigger_automatic"},
      "state": {"name": "COMPLETED", "type": "pipeline_step_state_completed",
                "result": {"name": "FAILED", "type": "pipeline_step_state_completed_failed",
                           "error": {"type": "pipeline_step_error", "key": "agent.git.git-branch-not-found",
                                     "message": "The branch was not found"}}},
      "started_on": "2026-07-02T08:49:39.304503031Z",
      "completed_on": "2026-07-02T08:50:14.41318653Z",
      "duration_in_seconds": 35,
      "run_number": 1,
      "image": {"name": "alpine:latest"},
      "maxTime": 1,
      "build_seconds_used": 35,
      "pipeline": {"type": "pipeline", "uuid": "{210f1f44-67d9-4b63-b755-7f46d0af56d4}"},
      "setup_commands": [{"commandType": "user", "name": "umask 000", "command": "umask 000"}],
      "script_commands": [{"commandType": "user", "name": "apk add curl", "command": "apk add curl"}],
      "teardown_commands": [{"commandType": "system", "name": "Upload Metrics",
                             "command": "echo up", "action": "UPLOAD_METRICS"}]
    },
    {
      "type": "pipeline_step",
      "uuid": "{8fc2d263-a4a6-4e15-b898-2c0f827f881c}",
      "trigger": {"type": "pipeline_step_trigger_automatic"},
      "state": {"name": "COMPLETED", "type": "pipeline_step_state_completed",
                "result": {"name": "NOT_RUN", "type": "pipeline_step_state_completed_not_run"}},
      "started_on": "2017-11-02T17:10:22.427Z",
      "completed_on": "2017-11-02T17:10:36.585Z",
      "duration_in_seconds": 14,
      "run_number": 1,
      "image": {"name": "debian:8.6"},
      "maxTime": 120,
      "build_seconds_used": 14,
      "pipeline": {"type": "pipeline", "uuid": "{b6484b8a-a198-4d94-96e8-a0256068ac13}"}
    }
  ]
}`

func TestListPullRequestStatuses_RequestShapeAndParsing(t *testing.T) {
	t.Parallel()

	client, recorded := newRecordingClient(t, bearerOpts(),
		testkit.StubResponse{Status: http.StatusOK, Body: statusesEnvelope})

	statuses, err := client.ListPullRequestStatuses(t.Context(), testWorkspace, testRepoSlug, 101)
	if err != nil {
		t.Fatalf("ListPullRequestStatuses() error = %v", err)
	}

	testkit.AssertEqual(t, "path", recorded.Path, testPullRequestPath+"/101/statuses")
	testkit.AssertEqual(t, paramPageLen, recorded.Query.Get(paramPageLen), "100")

	if len(statuses) != 2 {
		t.Fatalf("status count = %d, want 2", len(statuses))
	}
	testkit.AssertEqual(t, "State", statuses[0].State, StatusStopped)
	testkit.AssertEqual(t, "RefName", statuses[0].RefName, "feature/rate-limit")

	build, ok := statuses[0].BuildNumber()
	testkit.AssertEqual(t, "BuildNumber() ok", ok, true)
	testkit.AssertEqual(t, "BuildNumber()", build, 10471)

	// The newest status per key wins, and the rollup is what the list column shows.
	testkit.AssertEqual(t, "RollupStatuses()", RollupStatuses(statuses), StatusStopped)
	testkit.AssertEqual(t, "LatestStatuses() count", len(LatestStatuses(statuses)), 2)
}

func TestListPipelines_RequestShape(t *testing.T) {
	t.Parallel()

	client, recorded := newRecordingClient(t, bearerOpts(),
		testkit.StubResponse{Status: http.StatusOK, Body: emptyPage})

	opts := ListPipelinesOptions{TargetBranch: "feature/rate-limit", Sort: "", MaxItems: 0}
	if _, err := client.ListPipelines(t.Context(), testWorkspace, testRepoSlug, opts); err != nil {
		t.Fatalf("ListPipelines() error = %v", err)
	}

	testkit.AssertEqual(t, "path", recorded.Path, testPipelinesPath)
	testkit.AssertEqual(t, paramPageLen, recorded.Query.Get(paramPageLen), "10")
	testkit.AssertEqual(t, paramSort, recorded.Query.Get(paramSort), "-created_on")
	testkit.AssertEqual(t, "target.branch", recorded.Query.Get("target.branch"), "feature/rate-limit")
}

func TestListPipelines_OmitsEmptyBranchFilter(t *testing.T) {
	t.Parallel()

	client, recorded := newRecordingClient(t, bearerOpts(),
		testkit.StubResponse{Status: http.StatusOK, Body: emptyPage})

	if _, err := client.ListPipelines(t.Context(), testWorkspace, testRepoSlug,
		ListPipelinesOptions{TargetBranch: "", Sort: "-run_creation_date", MaxItems: 3}); err != nil {
		t.Fatalf("ListPipelines() error = %v", err)
	}

	if _, ok := recorded.Query["target.branch"]; ok {
		t.Error("target.branch parameter present, want absent")
	}
	testkit.AssertEqual(t, paramSort, recorded.Query.Get(paramSort), "-run_creation_date")
	testkit.AssertEqual(t, paramPageLen, recorded.Query.Get(paramPageLen), "3")
}

func TestListPipelines_ParsesBothTargetKinds(t *testing.T) {
	t.Parallel()

	client, _ := newRecordingClient(t, bearerOpts(),
		testkit.StubResponse{Status: http.StatusOK, Body: pipelinesEnvelope})

	pipelines, err := client.ListPipelines(t.Context(), testWorkspace, testRepoSlug,
		ListPipelinesOptions{TargetBranch: "", Sort: "", MaxItems: 0})
	if err != nil {
		t.Fatalf("ListPipelines() error = %v", err)
	}
	if len(pipelines) != 2 {
		t.Fatalf("pipeline count = %d, want 2", len(pipelines))
	}

	pullRequestRun := pipelines[0]
	testkit.AssertEqual(t, "BuildNumber", pullRequestRun.BuildNumber, 9445)
	testkit.AssertEqual(t, "State.Summary()", pullRequestRun.State.Summary(), "FAILED")
	testkit.AssertEqual(t, "Target.BranchName()", pullRequestRun.Target.BranchName(), "feature/rate-limit")
	testkit.AssertEqual(t, "HTMLURL()", pullRequestRun.HTMLURL(),
		"https://bitbucket.org/acme/api-gateway/pipelines/results/9445")
	if pullRequestRun.Target.PullRequest == nil {
		t.Fatal("Target.PullRequest = nil, want the pull request reference")
	}
	testkit.AssertEqual(t, "Target.PullRequest.ID", pullRequestRun.Target.PullRequest.ID, 101)

	branchRun := pipelines[1]
	testkit.AssertEqual(t, "Target.BranchName()", branchRun.Target.BranchName(), "kube/deploy-test")
	if branchRun.State.Result == nil || branchRun.State.Result.Terminator == nil {
		t.Fatal("STOPPED run lost its terminator")
	}
	testkit.AssertEqual(t, "Terminator", branchRun.State.Result.Terminator.Name(), "Linus Torvalds")
	testkit.AssertEqual(t, "HTMLURL() without repository", branchRun.HTMLURL(), "")
}

// escapedPathClient records the raw, still-encoded request target, which
// testkit.RecordedRequest cannot show because it decodes the path.
func escapedPathClient(t *testing.T, body string) (*Client, *string) {
	t.Helper()

	escaped := new(string)
	client := newHandlerClient(t, func(w http.ResponseWriter, r *http.Request) {
		*escaped = r.URL.EscapedPath()
		writeJSON(t, w, http.StatusOK, body)
	})
	return client, escaped
}

func TestGetPipeline_EscapesBracedUUID(t *testing.T) {
	t.Parallel()

	client, escaped := escapedPathClient(t, `{"type":"pipeline","uuid":"`+testPipelineUUID+`","build_number":9445}`)

	pipeline, err := client.GetPipeline(t.Context(), testWorkspace, testRepoSlug, testPipelineUUID)
	if err != nil {
		t.Fatalf("GetPipeline() error = %v", err)
	}

	testkit.AssertEqual(t, "escaped path", *escaped,
		testPipelinesPath+"/%7B210f1f44-67d9-4b63-b755-7f46d0af56d4%7D")
	testkit.AssertEqual(t, "UUID", pipeline.UUID, testPipelineUUID)
}

func TestGetPipeline_AcceptsBuildNumber(t *testing.T) {
	t.Parallel()

	client, escaped := escapedPathClient(t, `{"type":"pipeline","uuid":"`+testPipelineUUID+`","build_number":9445}`)

	if _, err := client.GetPipeline(t.Context(), testWorkspace, testRepoSlug, "9445"); err != nil {
		t.Fatalf("GetPipeline() error = %v", err)
	}

	testkit.AssertEqual(t, "escaped path", *escaped, testPipelinesPath+"/9445")
}

func TestListPipelineSteps_RequestShapeAndParsing(t *testing.T) {
	t.Parallel()

	client, recorded := newRecordingClient(t, bearerOpts(),
		testkit.StubResponse{Status: http.StatusOK, Body: stepsEnvelope})

	steps, err := client.ListPipelineSteps(t.Context(), testWorkspace, testRepoSlug, testPipelineUUID)
	if err != nil {
		t.Fatalf("ListPipelineSteps() error = %v", err)
	}

	testkit.AssertEqual(t, "path", recorded.Path, testPipelinesPath+"/"+testPipelineUUID+"/steps")
	testkit.AssertEqual(t, paramPageLen, recorded.Query.Get(paramPageLen), "100")

	if len(steps) != 2 {
		t.Fatalf("step count = %d, want 2", len(steps))
	}
	testkit.AssertEqual(t, "DisplayName(0)", steps[0].DisplayName(0), "Cancel Previous Pipelines")
	testkit.AssertEqual(t, "State.Summary()", steps[0].State.Summary(), "FAILED")
	if steps[0].State.Result == nil || steps[0].State.Result.Error == nil {
		t.Fatal("failed step lost its error")
	}
	testkit.AssertEqual(t, "error message", steps[0].State.Result.Error.Message, "The branch was not found")
	testkit.AssertEqual(t, "teardown commands", len(steps[0].TeardownCommands), 1)

	// The legacy step carries neither a name nor teardown commands.
	testkit.AssertEqual(t, "DisplayName(1)", steps[1].DisplayName(1), "Step 2")
	testkit.AssertEqual(t, "legacy teardown commands", len(steps[1].TeardownCommands), 0)
	testkit.AssertEqual(t, "legacy State.Summary()", steps[1].State.Summary(), "NOT_RUN")
}

func TestGetPipelineStepLog_TailRequestsARange(t *testing.T) {
	t.Parallel()

	const logText = "+ go test ./...\nok\n"
	client, recorded := newRecordingClient(t, bearerOpts(),
		testkit.StubResponse{Status: http.StatusPartialContent, Body: logText})

	stepUUID := "{9520b082-5efb-45c5-8538-9aeab8f74301}"
	log, err := client.GetPipelineStepLog(t.Context(), testWorkspace, testRepoSlug, testPipelineUUID, stepUUID,
		StepLogOptions{TailBytes: 65536})
	if err != nil {
		t.Fatalf("GetPipelineStepLog() error = %v", err)
	}

	testkit.AssertEqual(t, "log", log, logText)
	testkit.AssertEqual(t, "path", recorded.Path,
		testPipelinesPath+"/"+testPipelineUUID+"/steps/"+stepUUID+"/log")
	// Not acceptText: the API serves the log as an octet stream and answers 406
	// for a client that offers only text.
	testkit.AssertEqual(t, "Accept", recorded.Header.Get("Accept"), acceptLog)
	testkit.AssertEqual(t, "Range", recorded.Header.Get("Range"), "bytes=-65536")
}

func TestGetPipelineStepLog_WholeLogSendsNoRange(t *testing.T) {
	t.Parallel()

	client, recorded := newRecordingClient(t, bearerOpts(),
		testkit.StubResponse{Status: http.StatusOK, Body: "done\n"})

	if _, err := client.GetPipelineStepLog(t.Context(), testWorkspace, testRepoSlug, testPipelineUUID,
		"{9520b082}", StepLogOptions{TailBytes: 0}); err != nil {
		t.Fatalf("GetPipelineStepLog() error = %v", err)
	}

	if _, ok := recorded.Header["Range"]; ok {
		t.Errorf("Range header present = %q, want absent", recorded.Header.Get("Range"))
	}
}

func TestGetPipelineStepLog_EscapesBothUUIDs(t *testing.T) {
	t.Parallel()

	client, escaped := escapedPathClient(t, "log body")

	if _, err := client.GetPipelineStepLog(t.Context(), testWorkspace, testRepoSlug, testPipelineUUID,
		"{9520b082-5efb-45c5-8538-9aeab8f74301}", StepLogOptions{TailBytes: 0}); err != nil {
		t.Fatalf("GetPipelineStepLog() error = %v", err)
	}

	if strings.ContainsAny(*escaped, "{}") {
		t.Errorf("escaped path %q still carries raw braces", *escaped)
	}
	if !strings.HasSuffix(*escaped, "/steps/%7B9520b082-5efb-45c5-8538-9aeab8f74301%7D/log") {
		t.Errorf("escaped path %q does not end with the escaped step log path", *escaped)
	}
}

func TestGetPipelineStepLog_NotFound(t *testing.T) {
	t.Parallel()

	client, _ := newRecordingClient(t, bearerOpts(), testkit.StubResponse{
		Status: http.StatusNotFound,
		Body:   `{"type":"error","error":{"message":"No log for this step"}}`,
	})

	_, err := client.GetPipelineStepLog(t.Context(), testWorkspace, testRepoSlug, testPipelineUUID,
		"{9520b082}", StepLogOptions{TailBytes: 100})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !IsNotFound(err) {
		t.Errorf("IsNotFound(%v) = false, want true", err)
	}
	if !strings.Contains(err.Error(), "get step log acme/api-gateway") {
		t.Errorf("error %q lacks the method prefix", err.Error())
	}
}
