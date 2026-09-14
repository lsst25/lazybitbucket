package demo

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// These drive POST .../pullrequests/{id}/comments over raw HTTP: the 201 and
// the document it answers with, the two validation failures, and the fact that
// the comment is really in the dataset afterwards rather than only in the
// response.

// commentsPath is the comment collection of one pull request, which GET and
// POST share.
func commentsPath(slug string, id int) string {
	return pullRequestPath(slug, id) + segmentComments
}

// commentDocument is a request body for the comment route, written as a raw
// document so a test can send one that is missing a member.
func commentDocument(raw string, parentID int64) json.RawMessage {
	body := `{"content":{"raw":` + mustQuote(raw) + `}`
	if parentID != 0 {
		body += `,"parent":{"id":` + itoa64(parentID) + `}`
	}
	return json.RawMessage(body + "}")
}

func mustQuote(s string) string {
	quoted, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(quoted)
}

func itoa64(n int64) string {
	encoded, err := json.Marshal(n)
	if err != nil {
		return "0"
	}
	return string(encoded)
}

func TestServer_CreateCommentAnswers201WithTheCreatedComment(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	start := time.Now().UTC()

	got := postJSON(t, server.URL+commentsPath(repoAPIGateway, 103),
		commentDocument("Rebased and green now.", 0), nil)

	testkit.AssertEqual(t, "status", got.Status, http.StatusCreated)
	created := decodeJSON[bitbucket.Comment](t, got.Body)
	testkit.AssertEqual(t, "raw", created.Content.Raw, "Rebased and green now.")
	// The rendering travels with the answer, which is what lets a client show
	// the new comment without asking for the list again.
	testkit.AssertEqual(t, "html", created.Content.HTML, "<p>Rebased and green now.</p>")
	testkit.AssertEqual(t, "author", created.User.UUID, server.Data.User.UUID)
	if created.ID == 0 {
		t.Error("the created comment should carry an id")
	}
	if created.Parent != nil {
		t.Errorf("a general comment should carry no parent, got %+v", created.Parent)
	}
	if created.CreatedOn.Before(start) {
		t.Errorf("created_on = %v, want the wall clock and not the dataset's base time", created.CreatedOn)
	}

	// And it is in the dataset, not only in the answer.
	page := decodeJSON[listPage[bitbucket.Comment]](t, getOK(t, server, commentsPath(repoAPIGateway, 103)))
	if !hasComment(page.Values, created.ID) {
		t.Errorf("the comment list does not carry the new comment %d", created.ID)
	}
}

func TestServer_CreateCommentRepliesToAnExistingComment(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	existing := decodeJSON[listPage[bitbucket.Comment]](t,
		getOK(t, server, commentsPath(repoAPIGateway, 103)))
	parentID := existing.Values[0].ID

	got := postJSON(t, server.URL+commentsPath(repoAPIGateway, 103),
		commentDocument("Agreed, doing that.", parentID), nil)

	testkit.AssertEqual(t, "status", got.Status, http.StatusCreated)
	created := decodeJSON[bitbucket.Comment](t, got.Body)
	if created.Parent == nil {
		t.Fatal("a reply should carry its parent")
	}
	testkit.AssertEqual(t, "parent id", created.Parent.ID, parentID)
	// A new id, not the parent's and not one the dataset already used.
	if created.ID == parentID {
		t.Error("the reply took its parent's id")
	}
}

func TestServer_CreateCommentGivesEveryCommentItsOwnID(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	first := decodeJSON[bitbucket.Comment](t, postJSON(t,
		server.URL+commentsPath(repoAPIGateway, 103), commentDocument("one", 0), nil).Body)
	// The second one is written on another pull request: ids are unique across
	// Bitbucket, not per pull request, and the TUI keys a conversation by one.
	second := decodeJSON[bitbucket.Comment](t, postJSON(t,
		server.URL+commentsPath(repoWebApp, 41), commentDocument("two", 0), nil).Body)

	if first.ID == second.ID {
		t.Errorf("both comments got id %d", first.ID)
	}
}

func TestServer_CreateCommentRefusesAnEmptyBody(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	got := postJSON(t, server.URL+commentsPath(repoAPIGateway, 103), commentDocument("   ", 0), nil)

	testkit.AssertEqual(t, "status", got.Status, http.StatusBadRequest)
	envelope := decodeJSON[errorBody](t, got.Body)
	testkit.AssertEqual(t, "message", envelope.Error.Message, msgBadRequest)
	if messages := envelope.Error.Fields[fieldContent]; len(messages) == 0 {
		t.Errorf("the refusal should name the content field: %s", got.Body)
	}
}

func TestServer_CreateCommentRefusesAParentFromAnotherPullRequest(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	elsewhere := decodeJSON[listPage[bitbucket.Comment]](t,
		getOK(t, server, commentsPath(repoWebApp, 41)))
	if len(elsewhere.Values) == 0 {
		t.Skip("the fixture pull request has no comments to borrow a parent from")
	}

	got := postJSON(t, server.URL+commentsPath(repoAPIGateway, 103),
		commentDocument("Agreed.", elsewhere.Values[0].ID), nil)

	testkit.AssertEqual(t, "status", got.Status, http.StatusNotFound)
}

func TestServer_CreateCommentOnAPullRequestThatIsNotOpen(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	// #98 is merged. Unlike every other write, commenting is not refused for a
	// pull request the reader can no longer change.
	got := postJSON(t, server.URL+commentsPath(repoAPIGateway, 98),
		commentDocument("Noting this for the follow-up.", 0), nil)

	testkit.AssertEqual(t, "status", got.Status, http.StatusCreated)
}

func TestServer_CreateCommentOnAnUnknownPullRequest(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	got := postJSON(t, server.URL+commentsPath(repoAPIGateway, 4041),
		commentDocument("Hello?", 0), nil)

	testkit.AssertEqual(t, "status", got.Status, http.StatusNotFound)
}

// inlineCommentDocument is a request body anchoring a comment to a file, and to
// a line of it when line is not zero.
func inlineCommentDocument(raw, path string, line int, oldSide bool) json.RawMessage {
	body := `{"content":{"raw":` + mustQuote(raw) + `},"inline":{"path":` + mustQuote(path)
	switch {
	case line == 0:
	case oldSide:
		body += `,"from":` + itoa64(int64(line))
	default:
		body += `,"to":` + itoa64(int64(line))
	}
	return json.RawMessage(body + "}}")
}

func TestServer_CreateCommentAnchorsToALine(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	got := postJSON(t, server.URL+commentsPath(repoAPIGateway, 103),
		inlineCommentDocument("is this still needed?", "internal/upstream/retry.go", 42, false), nil)

	testkit.AssertEqual(t, "status", got.Status, http.StatusCreated)
	created := decodeJSON[bitbucket.Comment](t, got.Body)
	if created.Inline == nil {
		t.Fatal("the created comment should carry its anchor")
	}
	testkit.AssertEqual(t, "path", created.Inline.Path, "internal/upstream/retry.go")
	line, newSide, ok := created.Inline.Line()
	testkit.AssertEqual(t, "anchored", ok, true)
	testkit.AssertEqual(t, "line", line, 42)
	testkit.AssertEqual(t, "new side", newSide, true)
	// links.code is the diff anchor, and only an inline comment has one.
	if created.Links.Code.Href == "" {
		t.Error("an inline comment should carry its diff link")
	}

	// And it is in the list, anchored, so a client that refetches sees the same.
	page := decodeJSON[listPage[bitbucket.Comment]](t, getOK(t, server, commentsPath(repoAPIGateway, 103)))
	for _, comment := range page.Values {
		if comment.ID == created.ID && comment.Inline == nil {
			t.Error("the stored comment lost its anchor")
		}
	}
}

func TestServer_CreateCommentAnchorsToTheOldSideAndToAWholeFile(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	removed := decodeJSON[bitbucket.Comment](t, postJSON(t,
		server.URL+commentsPath(repoAPIGateway, 103),
		inlineCommentDocument("why did this go?", "internal/upstream/retry.go", 17, true), nil).Body)
	line, newSide, ok := removed.Inline.Line()
	testkit.AssertEqual(t, "anchored", ok, true)
	testkit.AssertEqual(t, "line", line, 17)
	testkit.AssertEqual(t, "old side", newSide, false)

	whole := decodeJSON[bitbucket.Comment](t, postJSON(t,
		server.URL+commentsPath(repoAPIGateway, 103),
		inlineCommentDocument("this file wants a test", "internal/upstream/retry.go", 0, false), nil).Body)
	if whole.Inline == nil {
		t.Fatal("a file-level comment is still an inline comment")
	}
	if _, _, anchored := whole.Inline.Line(); anchored {
		t.Error("a file-level comment should carry no line")
	}
}

func TestServer_CreateCommentRefusesAnAnchorItCannotHonour(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	existing := decodeJSON[listPage[bitbucket.Comment]](t,
		getOK(t, server, commentsPath(repoAPIGateway, 103)))

	t.Run("no path", func(t *testing.T) {
		t.Parallel()
		got := postJSON(t, server.URL+commentsPath(repoAPIGateway, 103),
			json.RawMessage(`{"content":{"raw":"here"},"inline":{"to":3}}`), nil)

		testkit.AssertEqual(t, "status", got.Status, http.StatusBadRequest)
		envelope := decodeJSON[errorBody](t, got.Body)
		if messages := envelope.Error.Fields[fieldPath]; len(messages) == 0 {
			t.Errorf("the refusal should name the path field: %s", got.Body)
		}
	})

	t.Run("an anchored reply", func(t *testing.T) {
		t.Parallel()
		body := `{"content":{"raw":"here"},"parent":{"id":` +
			itoa64(existing.Values[0].ID) + `},"inline":{"path":"a.go","to":3}}`
		got := postJSON(t, server.URL+commentsPath(repoAPIGateway, 103), json.RawMessage(body), nil)

		testkit.AssertEqual(t, "status", got.Status, http.StatusBadRequest)
	})
}
