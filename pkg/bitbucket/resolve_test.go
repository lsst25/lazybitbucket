package bitbucket

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// testResolvePath is the resolve endpoint of the fixture comment.
const testResolvePath = testCommentsPath + "/9001/resolve"

func TestResolveComment_PostsAndReadsTheResolution(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{
			// The answer as the comment carrying its resolution.
			name: "the comment shape", status: http.StatusOK, body: `{
			  "type": "pullrequest_comment", "id": 9001,
			  "resolution": {
			    "type": "comment_resolution",
			    "user": {"type": "user", "uuid": "{b6b5d0ff-32d1-4e3c-816a-9253437180ca}", "display_name": "Grace Hopper"},
			    "created_on": "2026-03-16T09:00:00.000000+00:00"
			  }
			}`,
		},
		{
			// The answer as the resolution alone.
			name: "the resolution shape", status: http.StatusCreated, body: `{
			  "type": "comment_resolution",
			  "user": {"type": "user", "uuid": "{b6b5d0ff-32d1-4e3c-816a-9253437180ca}", "display_name": "Grace Hopper"},
			  "created_on": "2026-03-16T09:00:00.000000+00:00"
			}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client, rec := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(t, w, tc.status, tc.body)
			})

			resolution, err := client.ResolveComment(t.Context(), testWorkspace, testRepoSlug, testPRID, 9001)
			if err != nil {
				t.Fatalf("ResolveComment() error = %v", err)
			}

			requests := rec.snapshot()
			testkit.AssertEqual(t, "method", requests[0].Method, testMethodPost)
			testkit.AssertEqual(t, "path", requests[0].Path, testResolvePath)
			// The endpoint takes no document at all.
			testkit.AssertEqual(t, "body", string(requests[0].Body), "")

			if resolution == nil {
				t.Fatal("ResolveComment() = nil, want the resolution")
			}
			testkit.AssertEqual(t, "resolved", resolution.Resolved(), true)
			testkit.AssertEqual(t, "who", resolution.User.Name(), "Grace Hopper")
			testkit.AssertEqual(t, "when", resolution.CreatedOn.Year(), 2026)
		})
	}
}

func TestResolveComment_AcceptsAnAnswerWithoutAResolution(t *testing.T) {
	t.Parallel()

	client, _ := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, `{}`)
	})

	// The thread is resolved either way; the caller decides what to show for
	// the detail the server did not send.
	resolution, err := client.ResolveComment(t.Context(), testWorkspace, testRepoSlug, testPRID, 9001)
	if err != nil {
		t.Fatalf("ResolveComment() error = %v", err)
	}
	if resolution != nil {
		t.Errorf("ResolveComment() = %+v, want nil for an answer that carried none", resolution)
	}
}

func TestReopenComment_DeletesAndAcceptsANoContentAnswer(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusNoContent, http.StatusOK} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()

			client, rec := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
			})

			if err := client.ReopenComment(t.Context(), testWorkspace, testRepoSlug, testPRID, 9001); err != nil {
				t.Fatalf("ReopenComment() error = %v", err)
			}
			testkit.AssertEqual(t, "method", rec.snapshot()[0].Method, http.MethodDelete)
			testkit.AssertEqual(t, "path", rec.snapshot()[0].Path, testResolvePath)
		})
	}
}

func TestResolveComment_RefusesACallWithNoComment(t *testing.T) {
	t.Parallel()

	opts := bearerOpts()
	opts.BaseURL = testkit.NoRequestServer(t).URL
	client := NewClient(opts)

	_, err := client.ResolveComment(t.Context(), testWorkspace, testRepoSlug, testPRID, 0)
	if !errors.Is(err, ErrNoComment) {
		t.Errorf("ResolveComment() error = %v, want ErrNoComment", err)
	}
	if err := client.ReopenComment(t.Context(), testWorkspace, testRepoSlug, testPRID, 0); !errors.Is(err, ErrNoComment) {
		t.Errorf("ReopenComment() error = %v, want ErrNoComment", err)
	}
}

func TestResolveComment_IsRefusedInReadOnlyMode(t *testing.T) {
	t.Parallel()

	client := readOnlyClient(t)

	if _, err := client.ResolveComment(t.Context(), testWorkspace, testRepoSlug, testPRID, 9001); !errors.Is(err, ErrReadOnly) {
		t.Errorf("ResolveComment() error = %v, want ErrReadOnly", err)
	}
	if err := client.ReopenComment(t.Context(), testWorkspace, testRepoSlug, testPRID, 9001); !errors.Is(err, ErrReadOnly) {
		t.Errorf("ReopenComment() error = %v, want ErrReadOnly", err)
	}
}

func TestResolveComment_ReportsTheServersRefusal(t *testing.T) {
	t.Parallel()

	client, _ := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusNotFound,
			`{"type": "error", "error": {"message": "No such comment on this pull request."}}`)
	})

	_, err := client.ResolveComment(t.Context(), testWorkspace, testRepoSlug, testPRID, 9001)

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("ResolveComment() error = %v, want an *APIError", err)
	}
	if !strings.Contains(apiErr.UserMessage(), "No such comment") {
		t.Errorf("UserMessage() = %q, want the server's own words", apiErr.UserMessage())
	}
}
