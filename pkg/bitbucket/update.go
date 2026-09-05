package bitbucket

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// PullRequestUpdate is the set of changes PUT .../pullrequests/{id} applies. A
// nil pointer means "leave this alone" and is omitted from the body, so the
// request is a partial document carrying only what changed.
//
// Reviewers is the exception: see the field's own note.
type PullRequestUpdate struct {
	// Title replaces the pull request's title. Bitbucket rejects an empty one.
	Title *string
	// Description replaces the raw Markdown description. An empty string is a
	// valid value — it clears the description.
	Description *string
	// Draft moves the pull request between draft and ready for review.
	Draft *bool
	// Reviewers is the COMPLETE desired reviewer set as braced account UUIDs,
	// and it is always included in the body: Bitbucket clears the reviewers a
	// PUT omits, so leaving it out of a title edit would silently unassign
	// everyone. Pass PullRequest.ReviewerUUIDs() when the set is unchanged; a
	// non-nil empty slice removes every reviewer; nil is refused with
	// ErrReviewersUnknown rather than guessed at.
	Reviewers []string
}

// ErrEmptyUpdate is an update that would change nothing. It is reported before
// any request is built, because a PUT with an empty body is a request to clear
// the reviewer set.
var ErrEmptyUpdate = errors.New("pull request update carries no changes")

// ErrReviewersUnknown is an update whose reviewer set is nil while other fields
// are being changed. Sending it would clear the pull request's reviewers, so the
// caller has to say what the set should be — PullRequest.ReviewerUUIDs() when it
// is not what is being edited, which itself returns nil for a pull request
// fetched without review data (fetch it with GetPullRequest first).
var ErrReviewersUnknown = errors.New("pull request update must carry the complete reviewer set")

// reviewerRef is how a reviewer goes on the wire in an update body: an object
// with nothing but the account UUID.
type reviewerRef struct {
	UUID string `json:"uuid"`
}

// UpdatePullRequest changes the pull request's title, description, draft state
// and reviewers, and returns the pull request as the server left it — which is
// the authoritative version, not the one the caller assembled.
//
// Only open pull requests can be mutated; a merged or declined one answers 400,
// as does an empty title or an attempt to make the author a reviewer. Those
// arrive as an *APIError whose UserMessage names the offending field.
func (c *Client) UpdatePullRequest(
	ctx context.Context, workspace, repoSlug string, id int, update PullRequestUpdate,
) (*PullRequest, error) {
	body, err := update.body()
	if err != nil {
		return nil, fmt.Errorf("update %s/%s#%d: %w", workspace, repoSlug, id, err)
	}

	var updated PullRequest
	relPath := pullRequestPath(workspace, repoSlug, id)
	status, _, err := c.writeJSON(ctx, http.MethodPut, relPath, body, &updated)
	if err != nil {
		return nil, fmt.Errorf("update %s/%s#%d: %w", workspace, repoSlug, id, err)
	}
	// writeJSON decodes the body only on a 200. Any other 2xx — or a 200 whose
	// body is not a pull request — would otherwise hand back a zero PullRequest
	// that the caller then treats as the server's truth.
	if status != http.StatusOK || updated.ID == 0 {
		return nil, fmt.Errorf("update %s/%s#%d: unexpected status %d without a pull request body",
			workspace, repoSlug, id, status)
	}
	return &updated, nil
}

// body is the partial pull request document to PUT, or the error that says why
// there is nothing safe to send.
func (u PullRequestUpdate) body() (map[string]any, error) {
	if u.Title == nil && u.Description == nil && u.Draft == nil && u.Reviewers == nil {
		return nil, ErrEmptyUpdate
	}
	if u.Reviewers == nil {
		return nil, ErrReviewersUnknown
	}

	reviewers := make([]reviewerRef, 0, len(u.Reviewers))
	for _, uuid := range u.Reviewers {
		reviewers = append(reviewers, reviewerRef{UUID: uuid})
	}

	body := map[string]any{"reviewers": reviewers}
	if u.Title != nil {
		body["title"] = *u.Title
	}
	if u.Description != nil {
		body["description"] = *u.Description
	}
	if u.Draft != nil {
		body["draft"] = *u.Draft
	}
	return body, nil
}
