package demo

import (
	"net/http"
	"strings"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
)

// POST .../pullrequests/{id}/comments: the one comment write. It mirrors the
// production validation the TUI has to survive — an empty body is a 400 against
// the content member, a parent that is not a comment of this pull request a 404
// — and, like the real endpoint, it takes a comment on a pull request whatever
// state it is in: a merged pull request is still worth discussing.

// The request document. Both members are the same shape the client sends, and
// a general comment carries no parent at all rather than a parent of id 0.
type commentRequest struct {
	Content struct {
		Raw string `json:"raw"`
	} `json:"content"`
	Parent *struct {
		ID int64 `json:"id"`
	} `json:"parent"`
}

// The messages the real endpoint answers these two faults with.
const (
	msgContentRequired = "This field is required."
	msgParentNotFound  = "No such comment on this pull request."
)

// fieldContent is the request member an empty comment is reported against.
const fieldContent = "content"

// handleCreateComment appends a comment written by the demo user and answers
// 201 with it, rendered html included — which is what lets a client show a new
// comment, mentions resolved, without refetching the list.
func (s *Server) handleCreateComment(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeBody[commentRequest](w, r)
	if !ok {
		return
	}

	s.dataMu.Lock()
	defer s.dataMu.Unlock()

	pullRequest, ok := s.findPullRequest(r)
	if !ok {
		writePullRequestNotFound(w, r)
		return
	}
	if strings.TrimSpace(request.Content.Raw) == "" {
		writeFieldsError(w, msgBadRequest, map[string][]string{fieldContent: {msgContentRequired}})
		return
	}

	slug := r.PathValue("slug")
	key := reviewKey(slug, pullRequest.ID)

	var parentID int64
	if request.Parent != nil {
		parentID = request.Parent.ID
		if !hasComment(s.Data.Comments[key], parentID) {
			writeNotFound(w, msgParentNotFound)
			return
		}
	}

	created := newComment(commentSpec{
		repo: slug, pullRequest: pullRequest.ID, id: s.nextCommentID(),
		parent: parentID, user: s.Data.User, body: request.Content.Raw,
		age: 0, edited: 0, deleted: false, pending: false,
		inline: nil, resolvedBy: nil, resolvedAge: 0,
	})
	// newComment timestamps against the dataset's own base time, which is what
	// every fixture is written relative to. A comment written now is not a
	// fixture: it happened at the wall clock the reader is looking at, the way
	// the real server stamps one.
	now := time.Now().UTC()
	created.CreatedOn, created.UpdatedOn = now, now

	s.Data.Comments[key] = append(s.Data.Comments[key], created)
	writeJSONStatus(w, http.StatusCreated, created)
}

// hasComment reports whether id names a comment of this pull request. Bitbucket
// answers 404 for a parent from another pull request, so the check is scoped to
// the one being commented on rather than to the whole dataset.
func hasComment(comments []bitbucket.Comment, id int64) bool {
	for i := range comments {
		if comments[i].ID == id {
			return true
		}
	}
	return false
}

// nextCommentID is one past the highest comment id in the dataset. Ids are
// unique across the whole of Bitbucket, not per pull request, and the TUI keys
// a conversation by the root's id — so a second comment must not be able to
// take an id another pull request already used. The caller holds dataMu.
func (s *Server) nextCommentID() int64 {
	var highest int64
	for _, comments := range s.Data.Comments {
		for i := range comments {
			if comments[i].ID > highest {
				highest = comments[i].ID
			}
		}
	}
	return highest + 1
}
