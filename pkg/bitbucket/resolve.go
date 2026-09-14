package bitbucket

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// Resolving is a property of a conversation rather than of a comment: Bitbucket
// hangs it off the comment that started the thread, at
// .../comments/{id}/resolve — POST to resolve, DELETE to reopen — and reports it
// back on that comment as its `resolution`.

// ErrNoComment is a resolve with no comment to resolve.
var ErrNoComment = errors.New("resolving needs the id of the comment that starts the thread")

// resolveAnswer is what the two endpoints answer with. Bitbucket has been seen
// to answer both with the resolution object itself and with the comment
// carrying it, so this holds either and ResolveComment reports whichever
// arrived. A caller that gets nothing back — an empty 200, a shape neither of
// these matches — is not left guessing: the method says so, and the TUI
// falls back to what it already knows (who pressed the key, and when).
type resolveAnswer struct {
	Type string `json:"type"`
	// The comment shape: the resolution hangs off it.
	ID         int64              `json:"id"`
	Resolution *CommentResolution `json:"resolution"`
	// The resolution shape: the members are on the document itself.
	User      *Account  `json:"user"`
	CreatedOn time.Time `json:"created_on"`
}

// resolution is the answer read as a resolution, whichever shape it came in,
// and whether there was one at all.
func (a resolveAnswer) resolution() (*CommentResolution, bool) {
	if a.Resolution != nil && a.Resolution.Resolved() {
		return a.Resolution, true
	}
	if a.User != nil {
		return &CommentResolution{Type: a.Type, User: a.User, CreatedOn: a.CreatedOn}, true
	}
	return nil, false
}

// ResolveComment marks the thread that comment_id starts as resolved and
// returns the resolution the server recorded — who resolved it and when.
//
// The resolution is nil, without an error, when the answer carried none: the
// thread is resolved either way, and the caller decides whether to show what it
// knows or to read the comments back.
func (c *Client) ResolveComment(
	ctx context.Context, workspace, repoSlug string, id int, commentID int64,
) (*CommentResolution, error) {
	relPath, err := resolvePath(workspace, repoSlug, id, commentID)
	if err != nil {
		return nil, err
	}

	var answer resolveAnswer
	status, _, err := c.writeJSON(ctx, http.MethodPost, relPath, nil, &answer)
	if err != nil {
		return nil, fmt.Errorf("resolve comment %d on %s/%s#%d: %w", commentID, workspace, repoSlug, id, err)
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return nil, fmt.Errorf("resolve comment %d on %s/%s#%d: unexpected status %d",
			commentID, workspace, repoSlug, id, status)
	}
	resolution, _ := answer.resolution()
	return resolution, nil
}

// ReopenComment reopens a resolved thread. The endpoint answers 204 with no
// document, so there is nothing to return but the failure.
func (c *Client) ReopenComment(
	ctx context.Context, workspace, repoSlug string, id int, commentID int64,
) error {
	relPath, err := resolvePath(workspace, repoSlug, id, commentID)
	if err != nil {
		return err
	}

	status, _, err := c.writeJSON(ctx, http.MethodDelete, relPath, nil, nil)
	if err != nil {
		return fmt.Errorf("reopen comment %d on %s/%s#%d: %w", commentID, workspace, repoSlug, id, err)
	}
	switch status {
	case http.StatusNoContent, http.StatusOK:
		return nil
	default:
		return fmt.Errorf("reopen comment %d on %s/%s#%d: unexpected status %d",
			commentID, workspace, repoSlug, id, status)
	}
}

// resolvePath is the resolve endpoint of one comment, and the local refusal of
// a call that names no comment — which would otherwise be a request against the
// comment collection itself.
func resolvePath(workspace, repoSlug string, id int, commentID int64) (string, error) {
	if commentID == 0 {
		return "", fmt.Errorf("resolve on %s/%s#%d: %w", workspace, repoSlug, id, ErrNoComment)
	}
	return pullRequestPath(workspace, repoSlug, id) +
		"/comments/" + strconv.FormatInt(commentID, 10) + "/resolve", nil
}
