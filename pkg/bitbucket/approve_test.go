package bitbucket

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// participantFixture is what POST .../approve answers with: a participant
// object, not a pull request.
const participantFixture = `{
  "type": "participant",
  "user": {"type": "user", "uuid": "{b6b5d0ff-32d1-4e3c-816a-9253437180ca}", "display_name": "Grace Hopper"},
  "role": "REVIEWER",
  "approved": true,
  "state": "approved",
  "participated_on": "2026-03-16T09:00:00.000000+00:00"
}`

func TestApprovePullRequest_PostsAnEmptyBodyAndDecodesTheParticipant(t *testing.T) {
	t.Parallel()

	client, rec := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, participantFixture)
	})

	participant, err := client.ApprovePullRequest(t.Context(), testWorkspace, testRepoSlug, testPRID)
	if err != nil {
		t.Fatalf("ApprovePullRequest() error = %v", err)
	}

	requests := rec.snapshot()
	if len(requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(requests))
	}
	testkit.AssertEqual(t, "method", requests[0].Method, testMethodPost)
	testkit.AssertEqual(t, "path", requests[0].Path, testApprovePath)
	// The endpoint takes no document at all: sending "null" would be a body,
	// and a body wants a Content-Type nothing here should have set.
	testkit.AssertEqual(t, "body", string(requests[0].Body), "")
	testkit.AssertEqual(t, "Content-Type", requests[0].Header.Get(contentTypeHeader), "")

	if participant == nil {
		t.Fatal("ApprovePullRequest() = nil, want the participant")
	}
	testkit.AssertEqual(t, "user uuid", participant.User.UUID, testReviewerUUID)
	testkit.AssertEqual(t, "role", participant.Role, RoleReviewer)
	testkit.AssertEqual(t, "approved", participant.Approved, true)
	testkit.AssertEqual(t, "state", participant.State, ParticipantApproved)
	testkit.AssertEqual(t, "participated_on year", participant.ParticipatedOn.Year(), 2026)
}

func TestUnapprovePullRequest_DeletesAndAcceptsANoContentAnswer(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "204 no content", status: http.StatusNoContent, body: ""},
		{name: "200 is tolerated", status: http.StatusOK, body: "{}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client, rec := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})

			if err := client.UnapprovePullRequest(
				t.Context(), testWorkspace, testRepoSlug, testPRID,
			); err != nil {
				t.Fatalf("UnapprovePullRequest() error = %v", err)
			}

			requests := rec.snapshot()
			if len(requests) != 1 {
				t.Fatalf("requests = %d, want 1", len(requests))
			}
			testkit.AssertEqual(t, "method", requests[0].Method, http.MethodDelete)
			testkit.AssertEqual(t, "path", requests[0].Path, testApprovePath)
			testkit.AssertEqual(t, "body", string(requests[0].Body), "")
		})
	}
}

func TestApprovePullRequest_ReadOnlyClientMakesNoRequest(t *testing.T) {
	t.Parallel()

	client := readOnlyClient(t)

	_, approveErr := client.ApprovePullRequest(t.Context(), testWorkspace, testRepoSlug, testPRID)
	if !IsReadOnly(approveErr) {
		t.Fatalf("ApprovePullRequest() error = %v, want ErrReadOnly", approveErr)
	}
	if !strings.Contains(approveErr.Error(), "approve acme/api-gateway#9") {
		t.Errorf("error %q does not name the pull request", approveErr)
	}

	unapproveErr := client.UnapprovePullRequest(t.Context(), testWorkspace, testRepoSlug, testPRID)
	if !IsReadOnly(unapproveErr) {
		t.Fatalf("UnapprovePullRequest() error = %v, want ErrReadOnly", unapproveErr)
	}
	if !strings.Contains(unapproveErr.Error(), "unapprove acme/api-gateway#9") {
		t.Errorf("error %q does not name the pull request", unapproveErr)
	}
}

// Approving one's own pull request is the refusal the TUI has to be able to read
// out, and it arrives as an ordinary error envelope.
func TestApprovePullRequest_SurfacesTheServersRefusal(t *testing.T) {
	t.Parallel()

	client, _ := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusBadRequest,
			`{"type":"error","error":{"message":"You cannot approve your own pull request."}}`)
	})

	_, err := client.ApprovePullRequest(t.Context(), testWorkspace, testRepoSlug, testPRID)

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v is not an *APIError", err)
	}
	testkit.AssertEqual(t, "Method", apiErr.Method, testMethodPost)
	testkit.AssertEqual(t, "Path", apiErr.Path, testApprovePath)
	testkit.AssertEqual(t, "Status", apiErr.Status, http.StatusBadRequest)
	testkit.AssertEqual(t, "UserMessage", apiErr.UserMessage(), "You cannot approve your own pull request.")
}

func TestUnapprovePullRequest_SurfacesTheServersError(t *testing.T) {
	t.Parallel()

	client, _ := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusNotFound,
			`{"type":"error","error":{"message":"Pull request not found."}}`)
	})

	err := client.UnapprovePullRequest(t.Context(), testWorkspace, testRepoSlug, testPRID)

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v is not an *APIError", err)
	}
	testkit.AssertEqual(t, "Method", apiErr.Method, http.MethodDelete)
	testkit.AssertEqual(t, "Status", apiErr.Status, http.StatusNotFound)
	if !IsNotFound(err) {
		t.Errorf("IsNotFound(%v) = false, want true", err)
	}
}

// A 2xx that carries no participant must not become a zero Participant the
// caller then reads as an approval by nobody.
func TestApprovePullRequest_RejectsAnAnswerWithoutAParticipant(t *testing.T) {
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
				w.Header().Set(contentTypeHeader, contentTypeJSON)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})

			participant, err := client.ApprovePullRequest(t.Context(), testWorkspace, testRepoSlug, testPRID)
			if err == nil {
				t.Fatalf("ApprovePullRequest = %+v, want an error for a %d without a participant",
					participant, tc.status)
			}
			if !strings.Contains(err.Error(), "unexpected status") {
				t.Errorf("error = %q, want it to name the unexpected status", err)
			}
		})
	}
}

// The read-only refusal is logged for both verbs, so the log panel says why
// nothing happened.
func TestApprove_ReadOnlyLogsTheRefusal(t *testing.T) {
	t.Parallel()

	var log strings.Builder
	client := readOnlyClient(t)
	client.SetLogger(&log)

	_, _ = client.ApprovePullRequest(t.Context(), testWorkspace, testRepoSlug, testPRID)
	_ = client.UnapprovePullRequest(t.Context(), testWorkspace, testRepoSlug, testPRID)

	testkit.AssertEqual(t, "log", log.String(),
		"  READ-ONLY: refused POST "+testApprovePath+"\n"+
			"  READ-ONLY: refused DELETE "+testApprovePath+"\n")
}
