package bitbucket

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// Every interface method must wrap the transport error with a stable,
// human-readable prefix and keep the status visible: the TUI prints exactly
// this string in its error banner, and the prefix is the only clue about which
// request failed.
func TestClient_AllMethodsPropagateHTTPErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		invoke  func(ctx context.Context, client *Client) error
		wantErr string
	}{
		{
			name: "GetCurrentUser",
			invoke: func(ctx context.Context, client *Client) error {
				_, err := client.GetCurrentUser(ctx)
				return err
			},
			wantErr: "get current user",
		},
		{
			name: "GetWorkspace",
			invoke: func(ctx context.Context, client *Client) error {
				_, err := client.GetWorkspace(ctx, testWorkspace)
				return err
			},
			wantErr: "get workspace acme",
		},
		{
			name: "ListRepositories",
			invoke: func(ctx context.Context, client *Client) error {
				_, err := client.ListRepositories(ctx, testWorkspace,
					ListRepositoriesOptions{Role: "", Query: "", Sort: "", MaxItems: 0})
				return err
			},
			wantErr: "list repositories acme",
		},
		{
			name: "GetRepository",
			invoke: func(ctx context.Context, client *Client) error {
				_, err := client.GetRepository(ctx, testWorkspace, testRepoSlug)
				return err
			},
			wantErr: "get repository acme/api-gateway",
		},
		{
			name: "ListPullRequests",
			invoke: func(ctx context.Context, client *Client) error {
				_, err := client.ListPullRequests(ctx, testWorkspace, testRepoSlug,
					ListPullRequestsOptions{States: nil, Query: "", Sort: "", MaxItems: 0, Fields: ""})
				return err
			},
			wantErr: "list pull requests acme/api-gateway",
		},
		{
			name: "GetPullRequest",
			invoke: func(ctx context.Context, client *Client) error {
				_, err := client.GetPullRequest(ctx, testWorkspace, testRepoSlug, 101)
				return err
			},
			wantErr: "get pull request acme/api-gateway#101",
		},
		{
			name: "ListPullRequestStatuses",
			invoke: func(ctx context.Context, client *Client) error {
				_, err := client.ListPullRequestStatuses(ctx, testWorkspace, testRepoSlug, 101)
				return err
			},
			wantErr: "list statuses acme/api-gateway#101",
		},
		{
			name: "ListPullRequestCommits",
			invoke: func(ctx context.Context, client *Client) error {
				_, err := client.ListPullRequestCommits(ctx, testWorkspace, testRepoSlug, 101,
					ListOptions{MaxItems: 0})
				return err
			},
			wantErr: "list commits acme/api-gateway#101",
		},
		{
			name: "ListPullRequestComments",
			invoke: func(ctx context.Context, client *Client) error {
				_, err := client.ListPullRequestComments(ctx, testWorkspace, testRepoSlug, 101,
					ListOptions{MaxItems: 0})
				return err
			},
			wantErr: "list comments acme/api-gateway#101",
		},
		{
			name: "ListPullRequestDiffStat",
			invoke: func(ctx context.Context, client *Client) error {
				_, err := client.ListPullRequestDiffStat(ctx, testWorkspace, testRepoSlug, 101)
				return err
			},
			wantErr: "get diffstat acme/api-gateway#101",
		},
		{
			name: "GetPullRequestDiff",
			invoke: func(ctx context.Context, client *Client) error {
				_, err := client.GetPullRequestDiff(ctx, testWorkspace, testRepoSlug, 101,
					DiffOptions{Path: "", Context: 0, DiffURL: ""})
				return err
			},
			wantErr: "get diff acme/api-gateway#101",
		},
		{
			name: "ListPipelines",
			invoke: func(ctx context.Context, client *Client) error {
				_, err := client.ListPipelines(ctx, testWorkspace, testRepoSlug,
					ListPipelinesOptions{TargetBranch: "", Sort: "", MaxItems: 0})
				return err
			},
			wantErr: "list pipelines acme/api-gateway",
		},
		{
			name: "GetPipeline",
			invoke: func(ctx context.Context, client *Client) error {
				_, err := client.GetPipeline(ctx, testWorkspace, testRepoSlug, testPipelineUUID)
				return err
			},
			wantErr: "get pipeline acme/api-gateway " + testPipelineUUID,
		},
		{
			name: "ListPipelineSteps",
			invoke: func(ctx context.Context, client *Client) error {
				_, err := client.ListPipelineSteps(ctx, testWorkspace, testRepoSlug, testPipelineUUID)
				return err
			},
			wantErr: "list pipeline steps acme/api-gateway " + testPipelineUUID,
		},
		{
			name: "GetPipelineStepLog",
			invoke: func(ctx context.Context, client *Client) error {
				_, err := client.GetPipelineStepLog(ctx, testWorkspace, testRepoSlug, testPipelineUUID,
					"{9520b082}", StepLogOptions{TailBytes: 0})
				return err
			},
			wantErr: "get step log acme/api-gateway " + testPipelineUUID + " {9520b082}",
		},
		{
			name: "MergePullRequest",
			invoke: func(ctx context.Context, client *Client) error {
				_, err := client.MergePullRequest(ctx, testWorkspace, testRepoSlug, 101,
					MergeOptions{Strategy: MergeSquash, CloseSourceBranch: nil, Message: ""})
				return err
			},
			wantErr: "merge acme/api-gateway#101",
		},
		{
			name: "GetMergeTaskStatus",
			invoke: func(ctx context.Context, client *Client) error {
				_, err := client.GetMergeTaskStatus(ctx, client.BaseURL()+testTaskPath)
				return err
			},
			wantErr: "get merge task status",
		},
		{
			name: "UpdatePullRequest",
			invoke: func(ctx context.Context, client *Client) error {
				_, err := client.UpdatePullRequest(ctx, testWorkspace, testRepoSlug, 101,
					PullRequestUpdate{Title: stringPtr("t"), Description: nil, Draft: nil, Reviewers: []string{}})
				return err
			},
			wantErr: "update acme/api-gateway#101",
		},
		{
			name: "ListEffectiveDefaultReviewers",
			invoke: func(ctx context.Context, client *Client) error {
				_, err := client.ListEffectiveDefaultReviewers(ctx, testWorkspace, testRepoSlug)
				return err
			},
			wantErr: "list default reviewers acme/api-gateway",
		},
		{
			name: "ListWorkspaceMembers",
			invoke: func(ctx context.Context, client *Client) error {
				_, err := client.ListWorkspaceMembers(ctx, testWorkspace, ListOptions{MaxItems: 0})
				return err
			},
			wantErr: "list workspace members acme",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, _ := newRecordingClient(t, bearerOpts(),
				testkit.StubResponse{Status: http.StatusInternalServerError, Body: "boom"})

			err := tt.invoke(t.Context(), client)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
			if !strings.Contains(err.Error(), "status 500") {
				t.Errorf("error %q does not report status 500", err.Error())
			}
		})
	}
}

// A read-only client must refuse every write before it opens a socket. The
// server here fails the test if it is ever reached, so "no request was made" is
// asserted by construction rather than by counting.
func TestClient_ReadOnlyRefusesEveryWriteWithoutARequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		invoke func(ctx context.Context, client *Client) error
	}{
		{
			name: "MergePullRequest",
			invoke: func(ctx context.Context, client *Client) error {
				_, err := client.MergePullRequest(ctx, testWorkspace, testRepoSlug, testPRID,
					MergeOptions{Strategy: MergeCommit, CloseSourceBranch: truthy(), Message: "no"})
				return err
			},
		},
		{
			name: "UpdatePullRequest",
			invoke: func(ctx context.Context, client *Client) error {
				_, err := client.UpdatePullRequest(ctx, testWorkspace, testRepoSlug, testPRID,
					PullRequestUpdate{
						Title: stringPtr("no"), Description: stringPtr("no"),
						Draft: truthy(), Reviewers: []string{testReviewerUUID},
					})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.invoke(t.Context(), readOnlyClient(t))
			if !IsReadOnly(err) {
				t.Fatalf("error = %v, want ErrReadOnly", err)
			}
			if !errors.Is(err, ErrReadOnly) {
				t.Errorf("errors.Is(err, ErrReadOnly) = false for %v", err)
			}
		})
	}
}

// do is the last line of defence: even a write helper that forgot to check
// read-only mode must not reach the transport.
func TestClient_ReadOnlyGuardSitsInDo(t *testing.T) {
	t.Parallel()

	client := readOnlyClient(t)
	spec := requestSpec{
		method:       http.MethodPost,
		fullURL:      client.BaseURL() + testMergePath,
		relPath:      testMergePath,
		accept:       contentTypeJSON,
		contentType:  contentTypeJSON,
		body:         []byte("{}"),
		extraHeaders: nil,
	}

	resp, err := client.do(t.Context(), client.HTTPClient(), spec)
	if !errors.Is(err, ErrReadOnly) {
		t.Fatalf("do() error = %v, want ErrReadOnly", err)
	}
	testkit.AssertEqual(t, "status", resp.status, 0)
}

// Reads are untouched by read-only mode: it is "browse only", not "do nothing".
func TestClient_ReadOnlyStillAllowsReads(t *testing.T) {
	t.Parallel()

	opts := bearerOpts()
	opts.ReadOnly = true
	client, _ := newRecordingClient(t, opts, testkit.StubResponse{Status: http.StatusOK, Body: accountFixture})

	account, err := client.GetCurrentUser(t.Context())
	if err != nil {
		t.Fatalf("GetCurrentUser() error = %v", err)
	}
	testkit.AssertEqual(t, "DisplayName", account.DisplayName, "Ada Lovelace")
	testkit.AssertEqual(t, "ReadOnly()", client.ReadOnly(), true)
}

// The log panel and every APIError must name the verb that actually went out,
// or a failed merge reads as a failed GET.
func TestClient_RequestLogAndErrorsCarryTheRealVerb(t *testing.T) {
	t.Parallel()

	var logs []RequestLog
	client, _ := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusNotFound, errorEnvelope("Pull request not found"))
	})
	client.SetOnRequest(func(entry RequestLog) { logs = append(logs, entry) })

	_, err := client.UpdatePullRequest(t.Context(), testWorkspace, testRepoSlug, testPRID,
		PullRequestUpdate{Title: stringPtr("x"), Description: nil, Draft: nil, Reviewers: []string{}})

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v is not an *APIError", err)
	}
	testkit.AssertEqual(t, "APIError.Method", apiErr.Method, testMethodPut)

	if len(logs) != 1 {
		t.Fatalf("RequestLog entries = %d, want 1", len(logs))
	}
	testkit.AssertEqual(t, "RequestLog.Method", logs[0].Method, testMethodPut)
	testkit.AssertEqual(t, "RequestLog.Path", logs[0].Path, testPath)
	testkit.AssertEqual(t, "RequestLog.Status", logs[0].Status, http.StatusNotFound)
}

// The default base URL is the real API host, which is why every test in this
// package points its client at an httptest or demo server instead.
func TestNewClient_DefaultsToTheRealAPIHost(t *testing.T) {
	t.Parallel()

	client := NewClient(ClientOpts{
		BaseURL: "", Token: "", Email: "", UserAgent: "", HTTPClient: nil, ReadOnly: false,
	})
	testkit.AssertEqual(t, "BaseURL()", client.BaseURL(), DefaultBaseURL)
}
