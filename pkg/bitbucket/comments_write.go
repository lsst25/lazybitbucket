package bitbucket

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// CommentCreate is what POST .../pullrequests/{id}/comments writes: a body, and
// either the comment it answers or the place in the diff it hangs off.
type CommentCreate struct {
	// Raw is the comment's Markdown, as the author typed it. Bitbucket renders
	// it and answers with both forms, so the created comment comes back with
	// its mentions already resolvable.
	Raw string
	// ParentID is the comment being replied to, 0 for a new conversation.
	// Bitbucket flattens a reply to a reply onto the same thread, so this may
	// be any comment of the thread, not only its root.
	ParentID int64
	// Anchor hangs a new conversation off a file, and off one line of it when
	// it names one. nil is a comment on the pull request itself. A reply takes
	// its anchor from the comment it answers, so setting both is refused with
	// ErrCommentAnchor rather than sent.
	Anchor *CommentAnchor
}

// CommentAnchor is where an inline comment sits: a file of the pull request,
// and one line of that file when Line names one.
//
// The line number is the diff's own: a line on the new side is numbered as the
// file is after the change, one on the old side as it was before. Which of the
// two Line means is OldSide, because the same number means a different line on
// each side — that is the whole reason Bitbucket has two members for it.
type CommentAnchor struct {
	// Path is the file, as the diffstat and the diff name it.
	Path string
	// Line is the anchored line, 0 to hang the comment off the file itself —
	// which is what Bitbucket shows above the first hunk.
	Line int
	// OldSide anchors to the old side of the diff, which is where a removed
	// line lives. An added or context line is on the new side.
	OldSide bool
}

// inline is the anchor as it goes on the wire: a path, and at most one of the
// two line members. Sending both, or a zero one, anchors a comment somewhere
// other than where the reader put it.
func (a CommentAnchor) inline() commentInline {
	body := commentInline{Path: a.Path, To: nil, From: nil}
	switch {
	case a.Line <= 0:
	case a.OldSide:
		line := a.Line
		body.From = &line
	default:
		line := a.Line
		body.To = &line
	}
	return body
}

// ErrEmptyComment is a comment with nothing in it. It is refused before any
// request is built: Bitbucket answers 400 to an empty body, and an accidental
// Enter on an empty composer is not worth a round trip to be told so.
var ErrEmptyComment = errors.New("comment is empty")

// ErrCommentAnchor is an anchor that names no file, or one on a reply. A reply
// hangs where the comment it answers hangs; an anchor of its own would be a
// request to put it somewhere else, which the endpoint does not offer.
var ErrCommentAnchor = errors.New("comment anchor must name a file, and a reply cannot carry one")

// commentBody is the request document. The parent and the anchor are omitted
// entirely rather than sent empty — a parent of id 0 is a 404 rather than a
// root, and an inline with no path is a 400.
type commentBody struct {
	Content commentContent `json:"content"`
	Parent  *commentParent `json:"parent,omitempty"`
	Inline  *commentInline `json:"inline,omitempty"`
}

// commentInline is CommentAnchor on the wire. The line members are pointers so
// the one that does not apply is left out; sending "to": null anchors nothing
// but does say something, and the API is happier without the noise.
type commentInline struct {
	Path string `json:"path"`
	To   *int   `json:"to,omitempty"`
	From *int   `json:"from,omitempty"`
}

type commentContent struct {
	Raw string `json:"raw"`
}

type commentParent struct {
	ID int64 `json:"id"`
}

// CreateComment posts a comment on a pull request and returns the comment the
// server created — rendered html included, which is what makes the new comment
// readable (mentions and all) without a refetch.
//
// The comment is a reply when ParentID names one, an inline comment when Anchor
// names a file, and a comment on the pull request itself when it carries
// neither.
//
// A pull request that is merged or declined still takes comments; unlike the
// other writes, this one is not refused for a state the reader can no longer
// change. An anchor the diff has moved past is Bitbucket's to refuse, and it
// answers 400 for it.
func (c *Client) CreateComment(
	ctx context.Context, workspace, repoSlug string, id int, create CommentCreate,
) (*Comment, error) {
	if strings.TrimSpace(create.Raw) == "" {
		return nil, fmt.Errorf("comment on %s/%s#%d: %w", workspace, repoSlug, id, ErrEmptyComment)
	}
	if create.Anchor != nil && (create.Anchor.Path == "" || create.ParentID != 0) {
		return nil, fmt.Errorf("comment on %s/%s#%d: %w", workspace, repoSlug, id, ErrCommentAnchor)
	}

	body := commentBody{Content: commentContent{Raw: create.Raw}, Parent: nil, Inline: nil}
	if create.ParentID != 0 {
		body.Parent = &commentParent{ID: create.ParentID}
	}
	if create.Anchor != nil {
		inline := create.Anchor.inline()
		body.Inline = &inline
	}

	var created Comment
	relPath := pullRequestPath(workspace, repoSlug, id) + "/comments"
	status, _, err := c.writeJSON(ctx, http.MethodPost, relPath, body, &created)
	if err != nil {
		return nil, fmt.Errorf("comment on %s/%s#%d: %w", workspace, repoSlug, id, err)
	}
	// The endpoint answers 201 with the created comment. A different status, or
	// a body without an id, would otherwise hand back a zero Comment that the
	// caller then inserts into the thread as a comment from nobody.
	if status != http.StatusCreated || created.ID == 0 {
		return nil, fmt.Errorf("comment on %s/%s#%d: unexpected status %d without a comment body",
			workspace, repoSlug, id, status)
	}
	return &created, nil
}
