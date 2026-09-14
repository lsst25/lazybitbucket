package demo

import (
	"net/http"
	"testing"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// resolvePath is the resolve endpoint of one comment, which POST and DELETE
// share.
func resolvePath(slug string, id int, commentID int64) string {
	return commentsPath(slug, id) + "/" + itoa64(commentID) + "/resolve"
}

// anUnresolvedComment is the id of a comment of the fixture pull request whose
// thread nobody has resolved.
func anUnresolvedComment(t *testing.T, server *Server, slug string, id int) int64 {
	t.Helper()
	page := decodeJSON[listPage[bitbucket.Comment]](t, getOK(t, server, commentsPath(slug, id)))
	for _, comment := range page.Values {
		if comment.Resolution == nil && comment.Parent == nil && !comment.Deleted {
			return comment.ID
		}
	}
	t.Fatalf("%s#%d has no unresolved thread root", slug, id)
	return 0
}

func TestServer_ResolveCommentRecordsWhoAndWhen(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	commentID := anUnresolvedComment(t, server, repoAPIGateway, 103)
	start := time.Now().UTC()

	got := postJSON(t, server.URL+resolvePath(repoAPIGateway, 103, commentID), nil, nil)

	testkit.AssertEqual(t, "status", got.Status, http.StatusOK)
	resolved := decodeJSON[bitbucket.Comment](t, got.Body)
	if resolved.Resolution == nil {
		t.Fatal("the answer should carry the resolution")
	}
	testkit.AssertEqual(t, "resolved", resolved.Resolution.Resolved(), true)
	testkit.AssertEqual(t, "who", resolved.Resolution.User.UUID, server.Data.User.UUID)
	if resolved.Resolution.CreatedOn.Before(start) {
		t.Errorf("created_on = %v, want the wall clock", resolved.Resolution.CreatedOn)
	}

	// And the list says the same, which is what a refetch would show.
	page := decodeJSON[listPage[bitbucket.Comment]](t, getOK(t, server, commentsPath(repoAPIGateway, 103)))
	for _, comment := range page.Values {
		if comment.ID == commentID && !comment.Resolution.Resolved() {
			t.Error("the stored comment is not resolved")
		}
	}
}

func TestServer_ReopenCommentTakesTheResolutionOff(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	commentID := anUnresolvedComment(t, server, repoAPIGateway, 103)
	postJSON(t, server.URL+resolvePath(repoAPIGateway, 103, commentID), nil, nil)

	got := sendJSON(t, http.MethodDelete, server.URL+resolvePath(repoAPIGateway, 103, commentID), nil, nil)

	testkit.AssertEqual(t, "status", got.Status, http.StatusNoContent)
	testkit.AssertEqual(t, "body", len(got.Body), 0)

	page := decodeJSON[listPage[bitbucket.Comment]](t, getOK(t, server, commentsPath(repoAPIGateway, 103)))
	for _, comment := range page.Values {
		if comment.ID == commentID && comment.Resolution.Resolved() {
			t.Error("the stored comment is still resolved")
		}
	}
}

func TestServer_ResolveRefusesWhatIsAlreadyInThatState(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	commentID := anUnresolvedComment(t, server, repoAPIGateway, 103)

	// Reopening what nobody resolved.
	testkit.AssertEqual(t, "reopen an open thread",
		sendJSON(t, http.MethodDelete, server.URL+resolvePath(repoAPIGateway, 103, commentID), nil, nil).Status,
		http.StatusBadRequest)

	postJSON(t, server.URL+resolvePath(repoAPIGateway, 103, commentID), nil, nil)

	// Resolving it twice.
	testkit.AssertEqual(t, "resolve a resolved thread",
		postJSON(t, server.URL+resolvePath(repoAPIGateway, 103, commentID), nil, nil).Status,
		http.StatusBadRequest)
}

func TestServer_ResolveAnUnknownComment(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	testkit.AssertEqual(t, "an id no comment has",
		postJSON(t, server.URL+resolvePath(repoAPIGateway, 103, 404404), nil, nil).Status,
		http.StatusNotFound)
	testkit.AssertEqual(t, "an unknown pull request",
		postJSON(t, server.URL+resolvePath(repoAPIGateway, 4041, 9001), nil, nil).Status,
		http.StatusNotFound)
}
