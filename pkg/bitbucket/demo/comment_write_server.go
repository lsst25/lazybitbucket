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

// The request document. Every member is the same shape the client sends: a
// general comment carries no parent at all rather than a parent of id 0, and no
// inline rather than an empty one.
type commentRequest struct {
	Content struct {
		Raw string `json:"raw"`
	} `json:"content"`
	Parent *struct {
		ID int64 `json:"id"`
	} `json:"parent"`
	Inline *struct {
		Path string `json:"path"`
		To   *int   `json:"to"`
		From *int   `json:"from"`
	} `json:"inline"`
}

// The messages the real endpoint answers these faults with.
const (
	msgContentRequired = "This field is required."
	msgParentNotFound  = "No such comment on this pull request."
	msgPathRequired    = "This field is required."
	msgInlineOnReply   = "A reply cannot be anchored to a line."
)

// The request members a refusal is reported against.
const (
	fieldContent = "content"
	fieldPath    = "inline.path"
)

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

	var inline *bitbucket.CommentInline
	if request.Inline != nil {
		if request.Inline.Path == "" {
			writeFieldsError(w, msgBadRequest, map[string][]string{fieldPath: {msgPathRequired}})
			return
		}
		if parentID != 0 {
			// A reply hangs where its parent hangs. The real endpoint has no
			// way to say otherwise, so neither does this.
			writeBadRequest(w, msgInlineOnReply)
			return
		}
		inline = &bitbucket.CommentInline{
			Path: request.Inline.Path, To: request.Inline.To, From: request.Inline.From,
			StartFrom: nil, StartTo: nil,
		}
	}

	created := newComment(commentSpec{
		repo: slug, pullRequest: pullRequest.ID, id: s.nextCommentID(),
		parent: parentID, user: s.Data.User, body: request.Content.Raw,
		age: 0, edited: 0, deleted: false, pending: false,
		inline: inline, resolvedBy: nil, resolvedAge: 0,
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
