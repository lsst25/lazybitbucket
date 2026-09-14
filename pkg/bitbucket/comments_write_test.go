package bitbucket

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// createdCommentFixture is what POST .../comments answers with: the comment the
// server built, carrying the html it rendered the Markdown into.
const createdCommentFixture = `{
  "type": "pullrequest_comment",
  "id": 9101,
  "created_on": "2026-03-16T09:00:00.000000+00:00",
  "updated_on": "2026-03-16T09:00:00.000000+00:00",
  "content": {
    "type": "rendered",
    "raw": "Rotating weekly still leaves a window.",
    "markup": "markdown",
    "html": "<p>Rotating weekly still leaves a window.</p>"
  },
  "user": {"type": "user", "uuid": "{b6b5d0ff-32d1-4e3c-816a-9253437180ca}", "display_name": "Grace Hopper"},
  "parent": {"id": 9001},
  "links": {"self": {"href": "https://api.bitbucket.org/2.0/x/comments/9101"}}
}`

func TestCreateComment_PostsTheReplyAndDecodesTheCreatedComment(t *testing.T) {
	t.Parallel()

	client, rec := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusCreated, createdCommentFixture)
	})

	comment, err := client.CreateComment(t.Context(), testWorkspace, testRepoSlug, testPRID,
		CommentCreate{Raw: "Rotating weekly still leaves a window.", ParentID: 9001})
	if err != nil {
		t.Fatalf("CreateComment() error = %v", err)
	}

	requests := rec.snapshot()
	if len(requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(requests))
	}
	testkit.AssertEqual(t, "method", requests[0].Method, testMethodPost)
	testkit.AssertEqual(t, "path", requests[0].Path, testCommentsPath)
	testkit.AssertEqual(t, "Content-Type", requests[0].Header.Get(contentTypeHeader), contentTypeJSON)

	var sent map[string]any
	if err := json.Unmarshal(requests[0].Body, &sent); err != nil {
		t.Fatalf("request body is not JSON: %v (%s)", err, requests[0].Body)
	}
	content, _ := sent["content"].(map[string]any)
	testkit.AssertEqual(t, "content.raw", content["raw"], "Rotating weekly still leaves a window.")
	parent, _ := sent["parent"].(map[string]any)
	parentID, _ := parent["id"].(float64)
	testkit.AssertEqual(t, "parent.id", int64(parentID), int64(9001))

	if comment == nil {
		t.Fatal("CreateComment() = nil, want the created comment")
	}
	testkit.AssertEqual(t, "id", comment.ID, int64(9101))
	// The rendering is the point of decoding the answer: it is the oracle the
	// mention resolver reads display names out of.
	testkit.AssertEqual(t, "html", comment.Content.HTML, "<p>Rotating weekly still leaves a window.</p>")
	if comment.Parent == nil {
		t.Fatal("the created reply should carry its parent")
	}
	testkit.AssertEqual(t, "parent id", comment.Parent.ID, int64(9001))
}

func TestCreateComment_OmitsTheParentOfAGeneralComment(t *testing.T) {
	t.Parallel()

	client, rec := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusCreated, createdCommentFixture)
	})

	if _, err := client.CreateComment(t.Context(), testWorkspace, testRepoSlug, testPRID,
		CommentCreate{Raw: "Ready from my side.", ParentID: 0}); err != nil {
		t.Fatalf("CreateComment() error = %v", err)
	}

	body := string(rec.snapshot()[0].Body)
	// A parent of id 0 is not a root, it is a comment that does not exist, so
	// the member has to be absent rather than zero.
	if strings.Contains(body, "parent") {
		t.Errorf("a general comment must not carry a parent: %s", body)
	}
}

func TestCreateComment_RefusesAnEmptyBodyWithoutARequest(t *testing.T) {
	t.Parallel()

	opts := bearerOpts()
	opts.BaseURL = testkit.NoRequestServer(t).URL
	client := NewClient(opts)

	for _, raw := range []string{"", "   \n\t "} {
		_, err := client.CreateComment(t.Context(), testWorkspace, testRepoSlug, testPRID,
			CommentCreate{Raw: raw, ParentID: 0})
		if !errors.Is(err, ErrEmptyComment) {
			t.Errorf("CreateComment(%q) error = %v, want ErrEmptyComment", raw, err)
		}
	}
}

func TestCreateComment_IsRefusedInReadOnlyMode(t *testing.T) {
	t.Parallel()

	client := readOnlyClient(t)

	_, err := client.CreateComment(t.Context(), testWorkspace, testRepoSlug, testPRID,
		CommentCreate{Raw: "Looks good.", ParentID: 0})
	if !errors.Is(err, ErrReadOnly) {
		t.Errorf("CreateComment() error = %v, want ErrReadOnly", err)
	}
}

func TestCreateComment_RejectsAnAnswerWithoutAComment(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "200 instead of 201", status: http.StatusOK, body: createdCommentFixture},
		{name: "201 without an id", status: http.StatusCreated, body: `{"type": "pullrequest_comment"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client, _ := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(t, w, tc.status, tc.body)
			})

			comment, err := client.CreateComment(t.Context(), testWorkspace, testRepoSlug, testPRID,
				CommentCreate{Raw: "Looks good.", ParentID: 0})
			if err == nil {
				t.Fatalf("CreateComment() = %+v, want an error", comment)
			}
			if !strings.Contains(err.Error(), "unexpected status") {
				t.Errorf("error = %v, want it to name the status", err)
			}
		})
	}
}

func TestCreateComment_ReportsTheServersRefusal(t *testing.T) {
	t.Parallel()

	client, _ := writeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusNotFound,
			`{"type": "error", "error": {"message": "No such comment on this pull request."}}`)
	})

	_, err := client.CreateComment(t.Context(), testWorkspace, testRepoSlug, testPRID,
		CommentCreate{Raw: "Agreed.", ParentID: 404404})

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("CreateComment() error = %v, want an *APIError", err)
	}
	testkit.AssertEqual(t, "status", apiErr.Status, http.StatusNotFound)
	testkit.AssertEqual(t, "message", apiErr.UserMessage(), "No such comment on this pull request.")
}
