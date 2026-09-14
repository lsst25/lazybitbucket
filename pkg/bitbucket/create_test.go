package bitbucket

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// createdPRFixture is what POST .../pullrequests answers with: the pull request
// the server opened, carrying the id the caller could not know.
const createdPRFixture = `{
  "type": "pullrequest",
  "id": 104,
  "title": "Add a token bucket rate limiter",
  "state": "OPEN",
  "draft": false,
  "source": {"branch": {"name": "feature/rate-limiter"}},
  "destination": {"branch": {"name": "main"}},
  "author": {"type": "user", "uuid": "{e5743438-cc31-49a0-96b7-9febd7b946f4}", "display_name": "Yurii Tatar"}
}`

func TestCreatePullRequest_SendsTheBranchesAndDecodesTheCreatedPullRequest(t *testing.T) {
	t.Parallel()

	client, rec := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusCreated, createdPRFixture)
	})

	created, err := client.CreatePullRequest(t.Context(), testWorkspace, testRepoSlug, NewPullRequest{
		Title: "Add a token bucket rate limiter", Description: "## Why\n\nBecause.",
		SourceBranch: "feature/rate-limiter", DestinationBranch: "main",
		Reviewers: []string{testReviewerUUID}, Draft: true, CloseSourceBranch: true,
	})
	if err != nil {
		t.Fatalf("CreatePullRequest() error = %v", err)
	}

	requests := rec.snapshot()
	if len(requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(requests))
	}
	testkit.AssertEqual(t, "method", requests[0].Method, testMethodPost)
	testkit.AssertEqual(t, "path", requests[0].Path, testPullRequestPath)

	var sent map[string]any
	if err := json.Unmarshal(requests[0].Body, &sent); err != nil {
		t.Fatalf("request body is not JSON: %v (%s)", err, requests[0].Body)
	}
	testkit.AssertEqual(t, "title", sent["title"], "Add a token bucket rate limiter")
	testkit.AssertEqual(t, "description", sent["description"], "## Why\n\nBecause.")
	testkit.AssertEqual(t, "draft", sent["draft"], true)
	testkit.AssertEqual(t, "close_source_branch", sent["close_source_branch"], true)
	testkit.AssertEqual(t, "source branch", branchOf(t, sent, "source"), "feature/rate-limiter")
	testkit.AssertEqual(t, "destination branch", branchOf(t, sent, "destination"), "main")

	reviewers, _ := sent["reviewers"].([]any)
	if len(reviewers) != 1 {
		t.Fatalf("reviewers = %v, want one", sent["reviewers"])
	}
	reviewer, _ := reviewers[0].(map[string]any)
	testkit.AssertEqual(t, "reviewer uuid", reviewer["uuid"], testReviewerUUID)

	if created == nil {
		t.Fatal("CreatePullRequest() = nil, want the created pull request")
	}
	testkit.AssertEqual(t, "id", created.ID, 104)
	testkit.AssertEqual(t, "state", created.State, PullRequestOpen)
}

// branchOf digs the branch name out of a source or destination member.
func branchOf(t *testing.T, body map[string]any, member string) string {
	t.Helper()
	endpoint, _ := body[member].(map[string]any)
	branch, _ := endpoint["branch"].(map[string]any)
	name, _ := branch["name"].(string)
	return name
}

func TestCreatePullRequest_OmitsWhatTheServerShouldDecide(t *testing.T) {
	t.Parallel()

	client, rec := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusCreated, createdPRFixture)
	})

	if _, err := client.CreatePullRequest(t.Context(), testWorkspace, testRepoSlug, NewPullRequest{
		Title: "Add a token bucket rate limiter", Description: "",
		SourceBranch: "feature/rate-limiter", DestinationBranch: "",
		Reviewers: nil, Draft: false, CloseSourceBranch: false,
	}); err != nil {
		t.Fatalf("CreatePullRequest() error = %v", err)
	}

	body := string(rec.snapshot()[0].Body)
	// No destination: the repository's own default branch is what the server
	// picks, and a branch called "" is a 400.
	if strings.Contains(body, "destination") {
		t.Errorf("an unset destination must not be sent: %s", body)
	}
	// No reviewers either: an empty array is a statement, and this is a caller
	// that has nothing to say about reviewers.
	if strings.Contains(body, "reviewers") {
		t.Errorf("an unset reviewer list must not be sent: %s", body)
	}
	if strings.Contains(body, `"description"`) {
		t.Errorf("an empty description must not be sent: %s", body)
	}
}

func TestCreatePullRequest_RefusesWhatItCannotSend(t *testing.T) {
	t.Parallel()

	opts := bearerOpts()
	opts.BaseURL = testkit.NoRequestServer(t).URL
	client := NewClient(opts)

	for _, tc := range []struct {
		name   string
		create NewPullRequest
		want   error
	}{
		{
			name:   "no title",
			create: NewPullRequest{Title: "  ", SourceBranch: "feature/x"},
			want:   ErrEmptyTitle,
		},
		{
			name:   "no source branch",
			create: NewPullRequest{Title: "Something", SourceBranch: " "},
			want:   ErrNoSourceBranch,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := client.CreatePullRequest(t.Context(), testWorkspace, testRepoSlug, tc.create)
			if !errors.Is(err, tc.want) {
				t.Errorf("CreatePullRequest() error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestCreatePullRequest_IsRefusedInReadOnlyMode(t *testing.T) {
	t.Parallel()

	client := readOnlyClient(t)

	_, err := client.CreatePullRequest(t.Context(), testWorkspace, testRepoSlug, NewPullRequest{
		Title: "Add a token bucket rate limiter", SourceBranch: "feature/rate-limiter",
	})
	if !errors.Is(err, ErrReadOnly) {
		t.Errorf("CreatePullRequest() error = %v, want ErrReadOnly", err)
	}
}

func TestCreatePullRequest_ReportsTheServersRefusal(t *testing.T) {
	t.Parallel()

	client, _ := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusBadRequest, `{"type": "error", "error": {
			"message": "Bad request",
			"fields": {"source": ["Source branch does not exist in the repository."]}
		}}`)
	})

	_, err := client.CreatePullRequest(t.Context(), testWorkspace, testRepoSlug, NewPullRequest{
		Title: "Add a token bucket rate limiter", SourceBranch: "feature/never-pushed",
	})

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("CreatePullRequest() error = %v, want an *APIError", err)
	}
	// The field detail is the actionable half, and UserMessage appends it.
	if !strings.Contains(apiErr.UserMessage(), "Source branch does not exist") {
		t.Errorf("UserMessage() = %q, want the field detail", apiErr.UserMessage())
	}
}

func TestCreatePullRequest_RejectsAnAnswerWithoutAPullRequest(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "200 instead of 201", status: http.StatusOK, body: createdPRFixture},
		{name: "201 without an id", status: http.StatusCreated, body: `{"type": "pullrequest"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client, _ := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(t, w, tc.status, tc.body)
			})

			created, err := client.CreatePullRequest(t.Context(), testWorkspace, testRepoSlug,
				NewPullRequest{Title: "Something", SourceBranch: "feature/x"})
			if err == nil {
				t.Fatalf("CreatePullRequest() = %+v, want an error", created)
			}
			if !strings.Contains(err.Error(), "unexpected status") {
				t.Errorf("error = %v, want it to name the status", err)
			}
		})
	}
}
