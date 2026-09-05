package bitbucket

import (
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// truthy is an addressable true, for MergeOptions.CloseSourceBranch.
func truthy() *bool { yes := true; return &yes }

// falsy is an addressable false: it has to reach the wire as
// "close_source_branch": false, not be omitted as a zero value.
func falsy() *bool { no := false; return &no }

func TestMergeStrategy_LabelsEveryDocumentedValue(t *testing.T) {
	t.Parallel()

	strategies := AllMergeStrategies()
	testkit.AssertSliceEqual(t, "AllMergeStrategies", strategies, []MergeStrategy{
		MergeCommit,
		MergeSquash,
		MergeFastForward,
		MergeSquashFastForward,
		MergeRebaseFastForward,
		MergeRebaseMerge,
	})

	labels := make([]string, 0, len(strategies))
	for _, strategy := range strategies {
		labels = append(labels, strategy.Label())
	}
	testkit.AssertSliceEqual(t, "labels", labels, []string{
		"merge commit", "squash", "fast-forward", "squash fast-forward", "rebase fast-forward", "rebase merge",
	})

	// An open enum: a value the API adds later is shown, not swallowed.
	testkit.AssertEqual(t, "unknown strategy", MergeStrategy("cherry_pick").Label(), "cherry_pick")
	testkit.AssertEqual(t, "empty strategy", MergeStrategy("").Label(), "")
}

func TestMergePullRequest_SendsTheMergeBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts MergeOptions
		want string
	}{
		{
			name: "defaults omit every optional member",
			opts: MergeOptions{Strategy: "", CloseSourceBranch: nil, Message: ""},
			want: `{"type":"pullrequest_merge_parameters"}`,
		},
		{
			name: "strategy only",
			opts: MergeOptions{Strategy: MergeSquash, CloseSourceBranch: nil, Message: ""},
			want: `{"type":"pullrequest_merge_parameters","merge_strategy":"squash"}`,
		},
		{
			name: "close source branch false is sent, not omitted",
			opts: MergeOptions{Strategy: "", CloseSourceBranch: falsy(), Message: ""},
			want: `{"type":"pullrequest_merge_parameters","close_source_branch":false}`,
		},
		{
			name: "everything",
			opts: MergeOptions{Strategy: MergeRebaseMerge, CloseSourceBranch: truthy(), Message: "ship it"},
			want: `{"type":"pullrequest_merge_parameters","merge_strategy":"rebase_merge",` +
				`"close_source_branch":true,"message":"ship it"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, rec := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(t, w, http.StatusOK, writePRFixture)
			})

			if _, err := client.MergePullRequest(t.Context(), testWorkspace, testRepoSlug, testPRID, tt.opts); err != nil {
				t.Fatalf("MergePullRequest() error = %v", err)
			}

			requests := rec.snapshot()
			if len(requests) != 1 {
				t.Fatalf("requests = %d, want 1", len(requests))
			}
			testkit.AssertEqual(t, "method", requests[0].Method, testMethodPost)
			testkit.AssertEqual(t, "path", requests[0].Path, testMergePath)
			testkit.AssertEqual(t, "Content-Type", requests[0].Header.Get(contentTypeHeader), contentTypeJSON)
			testkit.AssertEqual(t, "Accept", requests[0].Header.Get("Accept"), contentTypeJSON)
			testkit.AssertEqual(t, "body", string(requests[0].Body), tt.want)
		})
	}
}

func TestMergePullRequest_ReturnsTheMergedPullRequest(t *testing.T) {
	t.Parallel()

	client, _ := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, writePRFixture)
	})

	result, err := client.MergePullRequest(t.Context(), testWorkspace, testRepoSlug, testPRID,
		MergeOptions{Strategy: MergeCommit, CloseSourceBranch: nil, Message: ""})
	if err != nil {
		t.Fatalf("MergePullRequest() error = %v", err)
	}

	testkit.AssertEqual(t, "Queued()", result.Queued(), false)
	testkit.AssertEqual(t, "TaskURL", result.TaskURL, "")
	if result.PullRequest == nil {
		t.Fatal("PullRequest = nil, want the merged pull request")
	}
	testkit.AssertEqual(t, "State", result.PullRequest.State, PullRequestMerged)
	testkit.AssertEqual(t, "ID", result.PullRequest.ID, testPRID)
}

func TestMergePullRequest_QueuedReturnsTheTaskURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		location string
	}{
		{name: "relative Location", location: testTaskPath},
		{name: "absolute same-origin Location", location: ""}, // filled in with the server's own URL
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, rec := writeServer(t, func(w http.ResponseWriter, r *http.Request) {
				location := tt.location
				if location == "" {
					location = "http://" + r.Host + testTaskPath
				}
				w.Header().Set("Location", location)
				w.WriteHeader(http.StatusAccepted)
			})

			result, err := client.MergePullRequest(t.Context(), testWorkspace, testRepoSlug, testPRID,
				MergeOptions{Strategy: MergeSquash, CloseSourceBranch: nil, Message: ""})
			if err != nil {
				t.Fatalf("MergePullRequest() error = %v", err)
			}

			testkit.AssertEqual(t, "Queued()", result.Queued(), true)
			if result.PullRequest != nil {
				t.Errorf("PullRequest = %+v, want nil for a queued merge", result.PullRequest)
			}
			testkit.AssertEqual(t, "TaskURL", result.TaskURL, client.BaseURL()+testTaskPath)

			// The empty 202 body must not decode into a zero PullRequest.
			testkit.AssertEqual(t, "requests", len(rec.snapshot()), 1)
		})
	}
}

func TestMergePullRequest_QueuedWithoutLocationIsAnError(t *testing.T) {
	t.Parallel()

	client, _ := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})

	_, err := client.MergePullRequest(t.Context(), testWorkspace, testRepoSlug, testPRID,
		MergeOptions{Strategy: "", CloseSourceBranch: nil, Message: ""})
	if !errors.Is(err, errMergeTaskNoLocation) {
		t.Fatalf("error = %v, want errMergeTaskNoLocation", err)
	}
	if !strings.Contains(err.Error(), "merge acme/api-gateway#9") {
		t.Errorf("error %q does not name the pull request", err)
	}
}

func TestMergePullRequest_RefusesAForeignTaskLocation(t *testing.T) {
	t.Parallel()

	client, _ := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "https://evil.example/steal"+testTaskPath)
		w.WriteHeader(http.StatusAccepted)
	})

	_, err := client.MergePullRequest(t.Context(), testWorkspace, testRepoSlug, testPRID,
		MergeOptions{Strategy: "", CloseSourceBranch: nil, Message: ""})
	if err == nil {
		t.Fatal("expected an error for a foreign task link")
	}
	if !strings.Contains(err.Error(), "evil.example") {
		t.Errorf("error %q does not name the refused host", err)
	}
}

func TestMergePullRequest_ReportsAPIErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		status   int
		body     string
		wantText string
	}{
		{
			name:     "409 refs changed",
			status:   http.StatusConflict,
			body:     errorEnvelope("Unable to merge because one of the refs involved changed"),
			wantText: "status 409 Conflict: Unable to merge because one of the refs involved changed",
		},
		{
			name:     "555 is a timeout for a write, never Diff Too Large",
			status:   StatusDiffTooLarge,
			body:     errorEnvelope("The merge took too long"),
			wantText: "status 555 Timed Out: The merge took too long",
		},
		{
			name:     "400 names the offending field",
			status:   http.StatusBadRequest,
			body:     `{"type":"error","error":{"message":"Bad request","fields":{"merge_strategy":["\"x\" is not a valid choice."]}}}`,
			wantText: `Bad request: merge_strategy: "x" is not a valid choice.`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, _ := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(t, w, tt.status, tt.body)
			})

			_, err := client.MergePullRequest(t.Context(), testWorkspace, testRepoSlug, testPRID,
				MergeOptions{Strategy: "", CloseSourceBranch: nil, Message: ""})
			if err == nil {
				t.Fatal("expected an error")
			}

			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("error %v is not an *APIError", err)
			}
			testkit.AssertEqual(t, "Method", apiErr.Method, testMethodPost)
			testkit.AssertEqual(t, "Path", apiErr.Path, testMergePath)
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Errorf("error %q does not contain %q", err, tt.wantText)
			}
		})
	}
}

// A redirected POST must never be replayed or downgraded to a GET: net/http
// would do one or the other, so the write path uses the no-redirect client and
// the 3xx becomes an APIError instead.
func TestMergePullRequest_DoesNotFollowRedirects(t *testing.T) {
	t.Parallel()

	client, rec := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", testMergePath+"?followed=1")
		w.WriteHeader(http.StatusMovedPermanently)
	})

	_, err := client.MergePullRequest(t.Context(), testWorkspace, testRepoSlug, testPRID,
		MergeOptions{Strategy: MergeSquash, CloseSourceBranch: nil, Message: ""})

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v is not an *APIError", err)
	}
	testkit.AssertEqual(t, "Status", apiErr.Status, http.StatusMovedPermanently)
	testkit.AssertEqual(t, "Method", apiErr.Method, testMethodPost)

	requests := rec.snapshot()
	if len(requests) != 1 {
		t.Fatalf("requests = %d, want exactly 1 (the redirect must not be followed)", len(requests))
	}
	testkit.AssertEqual(t, "method", requests[0].Method, testMethodPost)
}

func TestMergePullRequest_ReadOnlyClientMakesNoRequest(t *testing.T) {
	t.Parallel()

	client := readOnlyClient(t)

	_, err := client.MergePullRequest(t.Context(), testWorkspace, testRepoSlug, testPRID,
		MergeOptions{Strategy: MergeSquash, CloseSourceBranch: truthy(), Message: "nope"})
	if !IsReadOnly(err) {
		t.Fatalf("error = %v, want ErrReadOnly", err)
	}
	if !strings.Contains(err.Error(), "merge acme/api-gateway#9") {
		t.Errorf("error %q does not name the pull request", err)
	}
}

func TestGetMergeTaskStatus_PendingThenSuccess(t *testing.T) {
	t.Parallel()

	pending := `{"task_status":"PENDING","links":{"self":{"href":"https://api.example/t"}}}`
	success := `{"task_status":"SUCCESS","merge_result":` + writePRFixture + `}`

	var polls atomic.Int32
	client, rec := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		body := pending
		if polls.Add(1) > 1 {
			body = success
		}
		writeJSON(t, w, http.StatusOK, body)
	})

	taskURL := client.BaseURL() + testTaskPath

	first, err := client.GetMergeTaskStatus(t.Context(), taskURL)
	if err != nil {
		t.Fatalf("GetMergeTaskStatus() error = %v", err)
	}
	testkit.AssertEqual(t, "first status", first.Status, MergeTaskPending)
	if first.MergeResult != nil {
		t.Errorf("MergeResult = %+v, want nil while pending", first.MergeResult)
	}

	second, err := client.GetMergeTaskStatus(t.Context(), taskURL)
	if err != nil {
		t.Fatalf("GetMergeTaskStatus() error = %v", err)
	}
	testkit.AssertEqual(t, "second status", second.Status, MergeTaskSuccess)
	if second.MergeResult == nil {
		t.Fatal("MergeResult = nil, want the merged pull request")
	}
	testkit.AssertEqual(t, "merged state", second.MergeResult.State, PullRequestMerged)

	requests := rec.snapshot()
	testkit.AssertEqual(t, "requests", len(requests), 2)
	testkit.AssertEqual(t, "method", requests[0].Method, testMethod)
	testkit.AssertEqual(t, "path", requests[0].Path, testTaskPath)
}

// A failed merge is reported as a 200 carrying the error envelope, so the status
// alone cannot be trusted here.
func TestGetMergeTaskStatus_ErrorEnvelopeAtStatus200(t *testing.T) {
	t.Parallel()

	client, _ := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, errorEnvelope("merge failed: the branch was deleted"))
	})

	_, err := client.GetMergeTaskStatus(t.Context(), client.BaseURL()+testTaskPath)
	if err == nil {
		t.Fatal("expected an error for a 200 carrying the error envelope")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v is not an *APIError", err)
	}
	testkit.AssertEqual(t, "Status", apiErr.Status, http.StatusOK)
	testkit.AssertEqual(t, "Message", apiErr.Message, "merge failed: the branch was deleted")
	testkit.AssertEqual(t, "AuthMethod", apiErr.AuthMethod, AuthBearer)
}

func TestGetMergeTaskStatus_RefusesAForeignHost(t *testing.T) {
	t.Parallel()

	client := newHandlerClient(t, func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request to %s", r.URL)
	})

	_, err := client.GetMergeTaskStatus(t.Context(), "https://evil.example"+testTaskPath)
	if err == nil {
		t.Fatal("expected an error for a foreign task URL")
	}
	if !strings.Contains(err.Error(), "evil.example") {
		t.Errorf("error %q does not name the refused host", err)
	}
}

// The task-status poll is a GET, so read-only mode leaves it alone: a queued
// merge still has to be observable.
func TestGetMergeTaskStatus_IsAllowedInReadOnlyMode(t *testing.T) {
	t.Parallel()

	client, _ := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, `{"task_status":"PENDING"}`)
	})
	client.readOnly = true

	task, err := client.GetMergeTaskStatus(t.Context(), client.BaseURL()+testTaskPath)
	if err != nil {
		t.Fatalf("GetMergeTaskStatus() error = %v", err)
	}
	testkit.AssertEqual(t, "status", task.Status, MergeTaskPending)
}

// The merge is always requested asynchronously: a synchronous merge that
// outlives Bitbucket's gateway answers 555 and leaves the caller guessing
// whether it landed, whereas async=true answers 202 with a task to poll.
func TestMergePullRequest_AsksForAnAsynchronousMerge(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	client := newHandlerClient(t, func(w http.ResponseWriter, r *http.Request) {
		rec.add(r)
		w.Header().Set("Content-Type", contentTypeJSON)
		_, _ = w.Write([]byte(`{"id":101,"state":"MERGED","type":"pullrequest"}`))
	})

	if _, err := client.MergePullRequest(t.Context(), testWorkspace, testRepoSlug, testPRID, MergeOptions{}); err != nil {
		t.Fatalf("MergePullRequest: %v", err)
	}
	requests := rec.snapshot()
	testkit.AssertEqual(t, "requests", len(requests), 1)
	testkit.AssertEqual(t, "async", requests[0].Query.Get("async"), "true")
}
