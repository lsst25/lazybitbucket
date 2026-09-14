package demo

import (
	"net/http"
	"strconv"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
)

// POST and DELETE on .../comments/{cid}/resolve: resolving a conversation and
// reopening it. The resolution is recorded on the comment that starts the
// thread, which is what the list serves back and what the TUI reads.
//
// The POST answers with the comment carrying its resolution. The real endpoint
// has been seen to answer with the resolution object alone; the client accepts
// either, and the richer answer is the one worth serving — a fake that answers
// less than production may would let a bug through.

// msgAlreadyResolved is the refusal a second resolve of the same thread gets.
const msgAlreadyResolved = "This comment thread is already resolved."

// msgNotResolved is the refusal a reopen of an unresolved thread gets.
const msgNotResolved = "This comment thread is not resolved."

// handleResolveComment marks a thread resolved by the demo user.
func (s *Server) handleResolveComment(w http.ResponseWriter, r *http.Request) {
	s.dataMu.Lock()
	defer s.dataMu.Unlock()

	comment, key, index, ok := s.commentToResolve(w, r)
	if !ok {
		return
	}
	if comment.Resolution.Resolved() {
		writeBadRequest(w, msgAlreadyResolved)
		return
	}

	user := s.Data.User
	s.Data.Comments[key][index].Resolution = &bitbucket.CommentResolution{
		Type:      "pullrequest_comment_resolution",
		User:      &user,
		CreatedOn: time.Now().UTC(),
	}
	writeJSON(w, s.Data.Comments[key][index])
}

// handleReopenComment takes the resolution off again, and answers 204 with no
// document exactly as the real endpoint does.
func (s *Server) handleReopenComment(w http.ResponseWriter, r *http.Request) {
	s.dataMu.Lock()
	defer s.dataMu.Unlock()

	comment, key, index, ok := s.commentToResolve(w, r)
	if !ok {
		return
	}
	if !comment.Resolution.Resolved() {
		writeBadRequest(w, msgNotResolved)
		return
	}

	s.Data.Comments[key][index].Resolution = nil
	w.WriteHeader(http.StatusNoContent)
}

// commentToResolve finds the comment the request names, in the pull request it
// names, and reports the 404s. The caller holds dataMu.
func (s *Server) commentToResolve(
	w http.ResponseWriter, r *http.Request,
) (bitbucket.Comment, string, int, bool) {
	pullRequest, ok := s.findPullRequest(r)
	if !ok {
		writePullRequestNotFound(w, r)
		return bitbucket.Comment{}, "", 0, false
	}

	commentID, err := strconv.ParseInt(r.PathValue("cid"), 10, 64)
	if err != nil {
		writeNotFound(w, msgParentNotFound)
		return bitbucket.Comment{}, "", 0, false
	}

	key := reviewKey(r.PathValue("slug"), pullRequest.ID)
	for index := range s.Data.Comments[key] {
		if s.Data.Comments[key][index].ID == commentID {
			return s.Data.Comments[key][index], key, index, true
		}
	}
	writeNotFound(w, msgParentNotFound)
	return bitbucket.Comment{}, "", 0, false
}
