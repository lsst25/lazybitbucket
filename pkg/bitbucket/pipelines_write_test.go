package bitbucket

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// createdPipelineFixture is what POST .../pipelines answers with: the run the
// server just created, PENDING and triggered by hand.
const createdPipelineFixture = `{
  "type": "pipeline",
  "uuid": "{9c2b1f0a-1d3e-4c5b-9a7f-2e8d6c4b1a09}",
  "build_number": 415,
  "state": {"type": "pipeline_state_pending", "name": "PENDING"},
  "trigger": {"type": "pipeline_trigger_manual", "name": "MANUAL"},
  "creator": {"type": "user", "uuid": "{e5743438-cc31-49a0-96b7-9febd7b946f4}", "display_name": "Ada Lovelace"},
  "repository": {"type": "repository", "name": "api-gateway", "full_name": "acme/api-gateway"},
  "target": {"type": "pipeline_pullrequest_target", "source": "feature/rate-limit", "destination": "main"},
  "created_on": "2026-09-05T09:12:00.000000+00:00",
  "run_number": 1
}`

// The hashes the documented request bodies use, kept apart so a body that puts
// the destination commit where the source commit belongs is visible.
const (
	testTargetCommitHash      = "1a372fc4c1b8e0f2a9d7b3c5e6f80912a3b4c5d6"
	testDestinationCommitHash = "9f848b7"
)

// commitRef is a commit the way a pipeline payload carries one: the hash plus
// the links the read model keeps. No request body may repeat those links, which
// is why every body assertion goes through this helper.
func commitRef(hash string) *CommitRef {
	ref := &CommitRef{Type: "commit", Hash: hash}
	ref.Links.Self = Link{Href: "https://api.example/commits/" + hash, Name: ""}
	ref.Links.HTML = Link{Href: "https://bitbucket.org/acme/api-gateway/commits/" + hash, Name: ""}
	return ref
}

// pullRequestRunTarget is the target of an existing pull request run, as
// GET /pipelines/{ref} answers it — the input a rerun is built from.
func pullRequestRunTarget() PipelineTarget {
	return PipelineTarget{
		Type:              "pipeline_pullrequest_target",
		Selector:          &PipelineSelector{Type: "pull-requests", Pattern: "**"},
		Commit:            commitRef(testTargetCommitHash),
		RefType:           "",
		RefName:           "",
		Source:            "pull-request-branch",
		Destination:       "master",
		DestinationCommit: commitRef(testDestinationCommitHash),
		PullRequest: &PipelinePullRequestRef{
			Type:   "pullrequest",
			ID:     3,
			Title:  "Add rate limiting to the gateway",
			Draft:  false,
			Queued: false,
		},
	}
}

// refRunTarget is the target of a branch run. refType is what the run answered
// with, which is "" for a run the API described without one.
func refRunTarget(refType string) PipelineTarget {
	return PipelineTarget{
		Type:              "pipeline_ref_target",
		Selector:          &PipelineSelector{Type: "custom", Pattern: "Deploy to production"},
		Commit:            commitRef(testTargetCommitHash),
		RefType:           refType,
		RefName:           "master",
		Source:            "",
		Destination:       "",
		DestinationCommit: nil,
		PullRequest:       nil,
	}
}

// commitRunTarget is the target of a run started against a bare commit.
func commitRunTarget() PipelineTarget {
	return PipelineTarget{
		Type:              "pipeline_commit_target",
		Selector:          nil,
		Commit:            commitRef(testTargetCommitHash),
		RefType:           "",
		RefName:           "",
		Source:            "",
		Destination:       "",
		DestinationCommit: nil,
		PullRequest:       nil,
	}
}

// assertJSONBody compares a recorded request body with want as decoded JSON, so
// the assertion is about which members went on the wire — not the order Go
// happened to marshal them in. A member that should have been omitted, and one
// sent as a string where the server wants a number, both fail here.
func assertJSONBody(t *testing.T, got []byte, want string) {
	t.Helper()

	var gotBody, wantBody map[string]any
	if err := json.Unmarshal(got, &gotBody); err != nil {
		t.Fatalf("unmarshal request body %q: %v", got, err)
	}
	if err := json.Unmarshal([]byte(want), &wantBody); err != nil {
		t.Fatalf("unmarshal want body %q: %v", want, err)
	}
	if !reflect.DeepEqual(gotBody, wantBody) {
		t.Errorf("request body = %v, want %v", gotBody, wantBody)
	}
}

func TestRunPipeline_SendsTheDocumentedTargetBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target PipelineTarget
		want   string
	}{
		{
			name:   "pull request target carries source, destination, commit and the id",
			target: pullRequestRunTarget(),
			want: `{"target": {"type": "pipeline_pullrequest_target",
			              "source": "pull-request-branch", "destination": "master",
			              "destination_commit": {"type": "commit", "hash": "` + testDestinationCommitHash + `"},
			              "commit": {"type": "commit", "hash": "` + testTargetCommitHash + `"},
			              "pullrequest": {"id": 3},
			              "selector": {"type": "pull-requests", "pattern": "**"}}}`,
		},
		{
			name:   "ref target keeps the ref_type the run had",
			target: refRunTarget("tag"),
			want: `{"target": {"type": "pipeline_ref_target", "ref_type": "tag", "ref_name": "master",
			              "commit": {"type": "commit", "hash": "` + testTargetCommitHash + `"},
			              "selector": {"type": "custom", "pattern": "Deploy to production"}}}`,
		},
		{
			name:   "ref target without a ref_type is sent as a branch",
			target: refRunTarget(""),
			want: `{"target": {"type": "pipeline_ref_target", "ref_type": "branch", "ref_name": "master",
			              "commit": {"type": "commit", "hash": "` + testTargetCommitHash + `"},
			              "selector": {"type": "custom", "pattern": "Deploy to production"}}}`,
		},
		{
			name:   "commit target carries nothing but the commit",
			target: commitRunTarget(),
			want: `{"target": {"type": "pipeline_commit_target",
			              "commit": {"type": "commit", "hash": "` + testTargetCommitHash + `"}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, rec := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(t, w, http.StatusCreated, createdPipelineFixture)
			})

			if _, err := client.RunPipeline(t.Context(), testWorkspace, testRepoSlug, tt.target); err != nil {
				t.Fatalf("RunPipeline() error = %v", err)
			}

			requests := rec.snapshot()
			if len(requests) != 1 {
				t.Fatalf("requests = %d, want 1", len(requests))
			}
			testkit.AssertEqual(t, "method", requests[0].Method, testMethodPost)
			testkit.AssertEqual(t, "path", requests[0].Path, testPipelinesPath)
			testkit.AssertEqual(t, "Content-Type", requests[0].Header.Get(contentTypeHeader), contentTypeJSON)
			assertJSONBody(t, requests[0].Body, tt.want)
		})
	}
}

// The endpoint's documentation quotes pullrequest.id as a string. The read model
// carries a number and the server coerces either, so the number is what goes on
// the wire — asserted on the decoded JSON type, which the body comparison alone
// would not pin down if both sides were written the same wrong way.
func TestRunPipeline_SendsThePullRequestIDAsANumber(t *testing.T) {
	t.Parallel()

	client, rec := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusCreated, createdPipelineFixture)
	})

	if _, err := client.RunPipeline(
		t.Context(), testWorkspace, testRepoSlug, pullRequestRunTarget(),
	); err != nil {
		t.Fatalf("RunPipeline() error = %v", err)
	}

	var body struct {
		Target struct {
			PullRequest struct {
				ID json.RawMessage `json:"id"`
			} `json:"pullrequest"`
		} `json:"target"`
	}
	requests := rec.snapshot()
	if len(requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(requests))
	}
	if err := json.Unmarshal(requests[0].Body, &body); err != nil {
		t.Fatalf("unmarshal request body %q: %v", requests[0].Body, err)
	}
	testkit.AssertEqual(t, "pullrequest.id", string(body.Target.PullRequest.ID), "3")
}

func TestRunPipeline_DecodesTheCreatedRun(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		status int
	}{
		{name: "201 is what the endpoint documents", status: http.StatusCreated},
		{name: "200 is tolerated", status: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client, _ := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", "https://api.example"+testPipelinesPath+"/415")
				writeJSON(t, w, tc.status, createdPipelineFixture)
			})

			pipeline, err := client.RunPipeline(
				t.Context(), testWorkspace, testRepoSlug, pullRequestRunTarget())
			if err != nil {
				t.Fatalf("RunPipeline() error = %v", err)
			}

			if pipeline == nil {
				t.Fatal("RunPipeline() = nil, want the created run")
			}
			testkit.AssertEqual(t, "UUID", pipeline.UUID, "{9c2b1f0a-1d3e-4c5b-9a7f-2e8d6c4b1a09}")
			testkit.AssertEqual(t, "BuildNumber", pipeline.BuildNumber, 415)
			testkit.AssertEqual(t, "State", pipeline.State.Name, "PENDING")
			testkit.AssertEqual(t, "IsCompleted", pipeline.State.IsCompleted(), false)
			testkit.AssertEqual(t, "Trigger", pipeline.Trigger.Name, "MANUAL")
		})
	}
}

// A 2xx that carries no run must not become a zero Pipeline the caller then
// reports as a started build.
func TestRunPipeline_RejectsAnAnswerWithoutAPipeline(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "202 is not a status this endpoint answers", status: http.StatusAccepted, body: createdPipelineFixture},
		{name: "201 with an empty object", status: http.StatusCreated, body: "{}"},
		{name: "204 no content", status: http.StatusNoContent, body: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client, _ := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(t, w, tc.status, tc.body)
			})

			pipeline, err := client.RunPipeline(
				t.Context(), testWorkspace, testRepoSlug, pullRequestRunTarget())
			if err == nil {
				t.Fatalf("RunPipeline() = %+v, want an error for a %d without a pipeline", pipeline, tc.status)
			}
			if !strings.Contains(err.Error(), "unexpected status") {
				t.Errorf("error = %q, want it to name the unexpected status", err)
			}
		})
	}
}

// A target the endpoint would reject is refused here, before a socket is
// opened, so the message can name the member that is missing.
func TestRunPipeline_RefusesATargetItCannotRepeat(t *testing.T) {
	t.Parallel()

	withoutRefName := refRunTarget("branch")
	withoutRefName.RefName = ""

	withoutCommit := commitRunTarget()
	withoutCommit.Commit = nil

	emptyCommitHash := commitRunTarget()
	emptyCommitHash.Commit = &CommitRef{Type: "commit", Hash: ""}

	withoutSource := pullRequestRunTarget()
	withoutSource.Source = ""

	withoutDestination := pullRequestRunTarget()
	withoutDestination.Destination = ""

	withoutPullRequestCommit := pullRequestRunTarget()
	withoutPullRequestCommit.Commit = nil

	withoutPullRequestID := pullRequestRunTarget()
	withoutPullRequestID.PullRequest = &PipelinePullRequestRef{
		Type: "pullrequest", ID: 0, Title: "", Draft: false, Queued: false,
	}

	unknownType := pullRequestRunTarget()
	unknownType.Type = "pipeline_release_target"

	tests := []struct {
		name    string
		target  PipelineTarget
		wantMsg string
	}{
		{name: "ref target without a ref name", target: withoutRefName, wantMsg: "pipeline_ref_target carries no ref_name"},
		{name: "commit target without a commit", target: withoutCommit, wantMsg: "pipeline_commit_target carries no commit.hash"},
		{name: "commit target with an empty hash", target: emptyCommitHash, wantMsg: "pipeline_commit_target carries no commit.hash"},
		{name: "pull request target without a source", target: withoutSource, wantMsg: "pipeline_pullrequest_target carries no source"},
		{
			name:    "pull request target without a destination",
			target:  withoutDestination,
			wantMsg: "pipeline_pullrequest_target carries no destination",
		},
		{
			name:    "pull request target without a commit",
			target:  withoutPullRequestCommit,
			wantMsg: "pipeline_pullrequest_target carries no commit.hash",
		},
		{
			name:    "pull request target without a pull request id",
			target:  withoutPullRequestID,
			wantMsg: "pipeline_pullrequest_target carries no pullrequest.id",
		},
		{name: "a target type this package does not know", target: unknownType, wantMsg: `unknown target type "pipeline_release_target"`},
		{name: "a target with no type at all", target: PipelineTarget{}, wantMsg: `unknown target type ""`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, rec := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(t, w, http.StatusCreated, createdPipelineFixture)
			})

			_, err := client.RunPipeline(t.Context(), testWorkspace, testRepoSlug, tt.target)
			if !errors.Is(err, ErrPipelineTarget) {
				t.Fatalf("RunPipeline() error = %v, want ErrPipelineTarget", err)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantMsg)
			}
			if requests := rec.snapshot(); len(requests) != 0 {
				t.Errorf("requests = %d, want none: an unusable target must not reach the server", len(requests))
			}
		})
	}
}

func TestPipelineWrites_ReadOnlyClientMakesNoRequest(t *testing.T) {
	t.Parallel()

	client := readOnlyClient(t)

	_, runErr := client.RunPipeline(t.Context(), testWorkspace, testRepoSlug, pullRequestRunTarget())
	if !IsReadOnly(runErr) {
		t.Fatalf("RunPipeline() error = %v, want ErrReadOnly", runErr)
	}
	if !strings.Contains(runErr.Error(), "run pipeline "+testRepoFull) {
		t.Errorf("error %q does not name the repository", runErr)
	}

	stopErr := client.StopPipeline(t.Context(), testWorkspace, testRepoSlug, testPipelineUUID)
	if !IsReadOnly(stopErr) {
		t.Fatalf("StopPipeline() error = %v, want ErrReadOnly", stopErr)
	}
	if !strings.Contains(stopErr.Error(), "stop pipeline "+testRepoFull+" "+testPipelineUUID) {
		t.Errorf("error %q does not name the run", stopErr)
	}
}

// Pipelines being switched off for the repository is the refusal the TUI has to
// read out, and it arrives as an ordinary error envelope.
func TestRunPipeline_SurfacesTheServersRefusal(t *testing.T) {
	t.Parallel()

	client, _ := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusBadRequest,
			`{"type":"error","error":{"message":"Pipelines is not enabled for this repository."}}`)
	})

	_, err := client.RunPipeline(t.Context(), testWorkspace, testRepoSlug, pullRequestRunTarget())

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v is not an *APIError", err)
	}
	testkit.AssertEqual(t, "Method", apiErr.Method, testMethodPost)
	testkit.AssertEqual(t, "Path", apiErr.Path, testPipelinesPath)
	testkit.AssertEqual(t, "Status", apiErr.Status, http.StatusBadRequest)
	testkit.AssertEqual(t, "UserMessage", apiErr.UserMessage(), "Pipelines is not enabled for this repository.")
}

func TestStopPipeline_PostsNoBodyToTheStopEndpoint(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "204 is what the endpoint documents", status: http.StatusNoContent, body: ""},
		{name: "200 is tolerated", status: http.StatusOK, body: "{}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client, rec := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})

			if err := client.StopPipeline(
				t.Context(), testWorkspace, testRepoSlug, testPipelineUUID,
			); err != nil {
				t.Fatalf("StopPipeline() error = %v", err)
			}

			requests := rec.snapshot()
			if len(requests) != 1 {
				t.Fatalf("requests = %d, want 1", len(requests))
			}
			testkit.AssertEqual(t, "method", requests[0].Method, testMethodPost)
			// The endpoint takes no document at all: a body would want a
			// Content-Type, and there is nothing to describe.
			testkit.AssertEqual(t, "body", string(requests[0].Body), "")
			testkit.AssertEqual(t, "Content-Type", requests[0].Header.Get(contentTypeHeader), "")
			// The braces of the UUID are not legal in a path segment, so the
			// assertion is on the target as it went on the wire.
			testkit.AssertSliceEqual(t, "targets", rec.requestTargets(), []string{testStopPipelinePath})
		})
	}
}

// Stopping a run that has already finished is the refusal the TUI has to read
// out in the server's own words.
func TestStopPipeline_SurfacesAnAlreadyCompletedRun(t *testing.T) {
	t.Parallel()

	client, _ := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusBadRequest,
			`{"type":"error","error":{"message":"The specified pipeline has already completed."}}`)
	})

	err := client.StopPipeline(t.Context(), testWorkspace, testRepoSlug, testPipelineUUID)

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v is not an *APIError", err)
	}
	testkit.AssertEqual(t, "Method", apiErr.Method, testMethodPost)
	testkit.AssertEqual(t, "Status", apiErr.Status, http.StatusBadRequest)
	testkit.AssertEqual(t, "UserMessage", apiErr.UserMessage(), "The specified pipeline has already completed.")
}

func TestStopPipeline_SurfacesAnUnknownRun(t *testing.T) {
	t.Parallel()

	client, _ := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusNotFound, `{"type":"error","error":{"message":"Pipeline not found."}}`)
	})

	err := client.StopPipeline(t.Context(), testWorkspace, testRepoSlug, testPipelineUUID)
	if !IsNotFound(err) {
		t.Fatalf("IsNotFound(%v) = false, want true", err)
	}
}

// Any other 2xx is a contract this package does not recognise, and reporting it
// as success would leave the caller believing a run was signalled.
func TestStopPipeline_RejectsAnUnexpectedStatus(t *testing.T) {
	t.Parallel()

	client, _ := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})

	err := client.StopPipeline(t.Context(), testWorkspace, testRepoSlug, testPipelineUUID)
	if err == nil {
		t.Fatal("StopPipeline() = nil, want an error for a 202")
	}
	if !strings.Contains(err.Error(), "unexpected status 202") {
		t.Errorf("error = %q, want it to name the unexpected status", err)
	}
}

// The read-only refusal is logged for both verbs, so the log panel says why
// nothing happened.
func TestPipelineWrites_ReadOnlyLogsTheRefusal(t *testing.T) {
	t.Parallel()

	var log strings.Builder
	client := readOnlyClient(t)
	client.SetLogger(&log)

	_, _ = client.RunPipeline(t.Context(), testWorkspace, testRepoSlug, pullRequestRunTarget())
	_ = client.StopPipeline(t.Context(), testWorkspace, testRepoSlug, testPipelineUUID)

	testkit.AssertEqual(t, "log", log.String(),
		"  READ-ONLY: refused POST "+testPipelinesPath+"\n"+
			"  READ-ONLY: refused POST "+testStopPipelinePath+"\n")
}
