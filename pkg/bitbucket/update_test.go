package bitbucket

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// noReviewerChange is the reviewer set every update has to carry even when it is
// not what is being edited: Bitbucket clears the reviewers a PUT omits.
func noReviewerChange() []string { return []string{testReviewerUUID} }

func stringPtr(value string) *string { return &value }

func TestUpdatePullRequest_SendsOnlyTheChangedFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		update PullRequestUpdate
		want   string
	}{
		{
			name: "title",
			update: PullRequestUpdate{
				Title: stringPtr("Rate limit the gateway"), Description: nil, Draft: nil,
				Reviewers: noReviewerChange(),
			},
			want: `{"reviewers":[{"uuid":"{b6b5d0ff-32d1-4e3c-816a-9253437180ca}"}],"title":"Rate limit the gateway"}`,
		},
		{
			name: "description, including clearing it",
			update: PullRequestUpdate{
				Title: nil, Description: stringPtr(""), Draft: nil, Reviewers: noReviewerChange(),
			},
			want: `{"description":"","reviewers":[{"uuid":"{b6b5d0ff-32d1-4e3c-816a-9253437180ca}"}]}`,
		},
		{
			name: "draft false is sent, not omitted",
			update: PullRequestUpdate{
				Title: nil, Description: nil, Draft: falsy(), Reviewers: noReviewerChange(),
			},
			want: `{"draft":false,"reviewers":[{"uuid":"{b6b5d0ff-32d1-4e3c-816a-9253437180ca}"}]}`,
		},
		{
			name: "reviewers only",
			update: PullRequestUpdate{
				Title: nil, Description: nil, Draft: nil,
				Reviewers: []string{testReviewerUUID, testOtherUUID},
			},
			want: `{"reviewers":[{"uuid":"{b6b5d0ff-32d1-4e3c-816a-9253437180ca}"},` +
				`{"uuid":"{781a664d-34db-4bc7-9c0d-bd3716d0d1de}"}]}`,
		},
		{
			name: "an empty non-nil reviewer set removes everyone",
			update: PullRequestUpdate{
				Title: nil, Description: nil, Draft: nil, Reviewers: []string{},
			},
			want: `{"reviewers":[]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, rec := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(t, w, http.StatusOK, writePRFixture)
			})

			if _, err := client.UpdatePullRequest(
				t.Context(), testWorkspace, testRepoSlug, testPRID, tt.update,
			); err != nil {
				t.Fatalf("UpdatePullRequest() error = %v", err)
			}

			requests := rec.snapshot()
			if len(requests) != 1 {
				t.Fatalf("requests = %d, want 1", len(requests))
			}
			testkit.AssertEqual(t, "method", requests[0].Method, testMethodPut)
			testkit.AssertEqual(t, "path", requests[0].Path, testPath)
			testkit.AssertEqual(t, "Content-Type", requests[0].Header.Get(contentTypeHeader), contentTypeJSON)
			testkit.AssertEqual(t, "body", string(requests[0].Body), tt.want)
		})
	}
}

func TestUpdatePullRequest_ReturnsTheServersPullRequest(t *testing.T) {
	t.Parallel()

	client, _ := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, writePRFixture)
	})

	updated, err := client.UpdatePullRequest(t.Context(), testWorkspace, testRepoSlug, testPRID,
		PullRequestUpdate{Title: stringPtr("Add rate limiting"), Description: nil, Draft: nil, Reviewers: []string{}})
	if err != nil {
		t.Fatalf("UpdatePullRequest() error = %v", err)
	}
	if updated == nil {
		t.Fatal("UpdatePullRequest() = nil, want the updated pull request")
	}
	testkit.AssertEqual(t, "Title", updated.Title, "Add rate limiting")
	testkit.AssertEqual(t, "Key", updated.Key(), "acme/api-gateway#9")
}

func TestUpdatePullRequest_RefusesUpdatesItCannotSendSafely(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		update  PullRequestUpdate
		wantErr error
	}{
		{
			name:    "nothing to change",
			update:  PullRequestUpdate{Title: nil, Description: nil, Draft: nil, Reviewers: nil},
			wantErr: ErrEmptyUpdate,
		},
		{
			name: "a nil reviewer set would clear the reviewers",
			update: PullRequestUpdate{
				Title: stringPtr("New title"), Description: nil, Draft: nil, Reviewers: nil,
			},
			wantErr: ErrReviewersUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := bearerOpts()
			opts.BaseURL = testkit.NoRequestServer(t).URL
			client := NewClient(opts)

			_, err := client.UpdatePullRequest(t.Context(), testWorkspace, testRepoSlug, testPRID, tt.update)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if !strings.Contains(err.Error(), "update acme/api-gateway#9") {
				t.Errorf("error %q does not name the pull request", err)
			}
		})
	}
}

func TestUpdatePullRequest_SurfacesFieldValidationErrors(t *testing.T) {
	t.Parallel()

	client, _ := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusBadRequest,
			`{"type":"error","error":{"message":"Bad request","fields":{"title":["This field is required."]}}}`)
	})

	_, err := client.UpdatePullRequest(t.Context(), testWorkspace, testRepoSlug, testPRID,
		PullRequestUpdate{Title: stringPtr(""), Description: nil, Draft: nil, Reviewers: []string{}})

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v is not an *APIError", err)
	}
	testkit.AssertEqual(t, "Method", apiErr.Method, testMethodPut)
	testkit.AssertEqual(t, "Status", apiErr.Status, http.StatusBadRequest)
	testkit.AssertEqual(t, "UserMessage", apiErr.UserMessage(), "Bad request: title: This field is required.")
	testkit.AssertSliceEqual(t, "Fields[title]", apiErr.Fields["title"], []string{"This field is required."})
}

func TestUpdatePullRequest_ReadOnlyClientMakesNoRequest(t *testing.T) {
	t.Parallel()

	client := readOnlyClient(t)

	_, err := client.UpdatePullRequest(t.Context(), testWorkspace, testRepoSlug, testPRID,
		PullRequestUpdate{Title: stringPtr("nope"), Description: nil, Draft: truthy(), Reviewers: []string{}})
	if !IsReadOnly(err) {
		t.Fatalf("error = %v, want ErrReadOnly", err)
	}
}

// The read-only refusal is logged so the log panel shows why nothing happened.
func TestWriteJSON_ReadOnlyLogsTheRefusal(t *testing.T) {
	t.Parallel()

	var log strings.Builder
	client := readOnlyClient(t)
	client.SetLogger(&log)

	if _, err := client.UpdatePullRequest(t.Context(), testWorkspace, testRepoSlug, testPRID,
		PullRequestUpdate{Title: stringPtr("x"), Description: nil, Draft: nil, Reviewers: []string{}}); err == nil {
		t.Fatal("expected ErrReadOnly")
	}

	testkit.AssertEqual(t, "log", log.String(), "  READ-ONLY: refused PUT "+testPath+"\n")
}

func TestPullRequest_ReviewerUUIDs(t *testing.T) {
	t.Parallel()

	loaded := PullRequest{Reviewers: []Account{
		{UUID: testReviewerUUID, DisplayName: "Grace"},
		{UUID: "", DisplayName: "a deleted account with no uuid"},
		{UUID: testOtherUUID, DisplayName: "Linus"},
	}}
	testkit.AssertSliceEqual(t, "ReviewerUUIDs", loaded.ReviewerUUIDs(), []string{testReviewerUUID, testOtherUUID})

	empty := PullRequest{Reviewers: []Account{}}
	got := empty.ReviewerUUIDs()
	if got == nil {
		t.Error("ReviewerUUIDs() = nil for an empty-but-loaded reviewer list, want a non-nil empty slice")
	}
	testkit.AssertEqual(t, "len(empty)", len(got), 0)

	// nil means "the payload carried no reviewers", which UpdatePullRequest must
	// refuse rather than read as "remove everyone".
	notLoaded := PullRequest{Reviewers: nil}
	if notLoaded.ReviewerUUIDs() != nil {
		t.Errorf("ReviewerUUIDs() = %v for an unloaded reviewer list, want nil", notLoaded.ReviewerUUIDs())
	}
}

// A body-less 2xx must not become a zero pull request the caller then caches
// over the real one: writeJSON only decodes a 200, so the update has to check
// what it was handed.
func TestUpdatePullRequest_RejectsAnAnswerWithoutAPullRequest(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "204 no content", status: http.StatusNoContent, body: ""},
		{name: "200 with an empty object", status: http.StatusOK, body: "{}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client := newHandlerClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", contentTypeJSON)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})

			title := "renamed"
			pr, err := client.UpdatePullRequest(t.Context(), testWorkspace, testRepoSlug, testPRID,
				PullRequestUpdate{Title: &title, Reviewers: []string{}})
			if err == nil {
				t.Fatalf("UpdatePullRequest = %+v, want an error for a %d without a pull request", pr, tc.status)
			}
			if !strings.Contains(err.Error(), "unexpected status") {
				t.Errorf("error = %q, want it to name the unexpected status", err)
			}
		})
	}
}
