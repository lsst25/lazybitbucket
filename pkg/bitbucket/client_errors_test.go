package bitbucket

import (
	"context"
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
