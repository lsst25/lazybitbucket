package bitbucket

import (
	"context"
	"fmt"
	"net/http"
)

// approveSegment is the path segment both approval verbs share: POST records
// the caller's approval and DELETE withdraws it. There is no request body
// either way — who is approving is whoever the credentials belong to.
const approveSegment = "/approve"

// ApprovePullRequest records the caller's approval and returns the participant
// row the server built for it: the account, the role it was recorded under
// (REVIEWER for a requested reviewer, PARTICIPANT for anyone else) and when.
//
// Bitbucket refuses an approval of one's own pull request with a 400, and a
// pull request the token cannot see with a 404; both arrive as an *APIError
// whose UserMessage is the sentence to show.
func (c *Client) ApprovePullRequest(
	ctx context.Context, workspace, repoSlug string, id int,
) (*Participant, error) {
	relPath := pullRequestPath(workspace, repoSlug, id) + approveSegment

	var participant Participant
	status, _, err := c.writeJSON(ctx, http.MethodPost, relPath, nil, &participant)
	if err != nil {
		return nil, fmt.Errorf("approve %s/%s#%d: %w", workspace, repoSlug, id, err)
	}
	// writeJSON decodes the body only on a 200. Any other 2xx — or a 200 whose
	// body is not a participant — would otherwise hand back a zero Participant
	// that the caller then reads as "approved by nobody".
	if status != http.StatusOK || participant.User.UUID == "" {
		return nil, fmt.Errorf("approve %s/%s#%d: unexpected status %d without a participant body",
			workspace, repoSlug, id, status)
	}
	return &participant, nil
}

// UnapprovePullRequest withdraws the caller's approval. The endpoint answers
// 204 with no document, so there is nothing to return but the failure.
//
// A 200 is accepted too: the answer carries no information either way, and the
// point of the call is the state it leaves behind, which the caller reads back
// with GetPullRequest.
func (c *Client) UnapprovePullRequest(ctx context.Context, workspace, repoSlug string, id int) error {
	relPath := pullRequestPath(workspace, repoSlug, id) + approveSegment

	status, _, err := c.writeJSON(ctx, http.MethodDelete, relPath, nil, nil)
	if err != nil {
		return fmt.Errorf("unapprove %s/%s#%d: %w", workspace, repoSlug, id, err)
	}
	switch status {
	case http.StatusNoContent, http.StatusOK:
		return nil
	default:
		return fmt.Errorf("unapprove %s/%s#%d: unexpected status %d", workspace, repoSlug, id, status)
	}
}
