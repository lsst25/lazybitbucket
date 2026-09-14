package bitbucket

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// CommentCreate is what POST .../pullrequests/{id}/comments writes: a body, and
// optionally the comment it answers.
//
// Anchoring a new comment to a file and a line is a third field Bitbucket takes
// and this does not have yet, so every comment written here is either a reply
// or a general one on the pull request.
type CommentCreate struct {
	// Raw is the comment's Markdown, as the author typed it. Bitbucket renders
	// it and answers with both forms, so the created comment comes back with
	// its mentions already resolvable.
	Raw string
	// ParentID is the comment being replied to, 0 for a new conversation.
	// Bitbucket flattens a reply to a reply onto the same thread, so this may
	// be any comment of the thread, not only its root.
	ParentID int64
}

// ErrEmptyComment is a comment with nothing in it. It is refused before any
// request is built: Bitbucket answers 400 to an empty body, and an accidental
// Enter on an empty composer is not worth a round trip to be told so.
var ErrEmptyComment = errors.New("comment is empty")

// commentBody is the request document. The parent is omitted entirely for a
// general comment — a parent of id 0 is a 404, not a root.
type commentBody struct {
	Content commentContent `json:"content"`
	Parent  *commentParent `json:"parent,omitempty"`
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
// A pull request that is merged or declined still takes comments; unlike the
// other writes, this one is not refused for a state the reader can no longer
// change.
func (c *Client) CreateComment(
	ctx context.Context, workspace, repoSlug string, id int, create CommentCreate,
) (*Comment, error) {
	if strings.TrimSpace(create.Raw) == "" {
		return nil, fmt.Errorf("comment on %s/%s#%d: %w", workspace, repoSlug, id, ErrEmptyComment)
	}

	body := commentBody{Content: commentContent{Raw: create.Raw}, Parent: nil}
	if create.ParentID != 0 {
		body.Parent = &commentParent{ID: create.ParentID}
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
