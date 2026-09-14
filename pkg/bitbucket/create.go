package bitbucket

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// NewPullRequest is what POST .../pullrequests opens: a branch, a title, and
// whatever else the author wants to say about it up front.
//
// Everything but the title and the source branch is optional. An omitted
// destination is the repository's own default branch, which is what the web UI
// offers first, and an omitted reviewer set is nobody — Bitbucket does not add
// the repository's default reviewers to a pull request created through the API,
// so a caller that wants them has to send them (ListEffectiveDefaultReviewers).
type NewPullRequest struct {
	Title       string
	Description string
	// SourceBranch is the branch being proposed. It has to exist on the
	// remote: a branch that lives only in a local checkout is a 400.
	SourceBranch string
	// DestinationBranch is what it is proposed into, "" for the repository's
	// default branch.
	DestinationBranch string
	// Reviewers are braced account UUIDs. The author may not be among them.
	Reviewers []string
	// Draft opens the pull request as a draft rather than ready for review.
	Draft bool
	// CloseSourceBranch deletes the source branch when the pull request merges.
	CloseSourceBranch bool
}

// ErrEmptyTitle is a pull request with no title. Bitbucket answers 400 for it,
// against the title field; the refusal is local because an empty title is not
// worth a round trip.
var ErrEmptyTitle = errors.New("pull request title is empty")

// ErrNoSourceBranch is a pull request with nothing to propose.
var ErrNoSourceBranch = errors.New("pull request needs a source branch")

// newPullRequestBody is the request document. The destination is omitted
// entirely when the caller did not name one, which is how the server is asked
// for its own default rather than for a branch called "".
type newPullRequestBody struct {
	Title             string        `json:"title"`
	Description       string        `json:"description,omitempty"`
	Source            branchRef     `json:"source"`
	Destination       *branchRef    `json:"destination,omitempty"`
	Reviewers         []reviewerRef `json:"reviewers,omitempty"`
	Draft             bool          `json:"draft"`
	CloseSourceBranch bool          `json:"close_source_branch"`
}

// branchRef is how a branch goes on the wire in a create body: an object
// holding an object holding a name.
type branchRef struct {
	Branch branchName `json:"branch"`
}

type branchName struct {
	Name string `json:"name"`
}

// CreatePullRequest opens a pull request and returns it as the server created
// it — with its id, which is the one thing the caller cannot know in advance.
//
// The usual refusals arrive as an *APIError whose UserMessage names the field:
// a source branch the remote does not have, a destination that does not exist,
// an author among the reviewers, or a pull request that is already open for the
// same pair of branches.
func (c *Client) CreatePullRequest(
	ctx context.Context, workspace, repoSlug string, create NewPullRequest,
) (*PullRequest, error) {
	body, err := create.body()
	if err != nil {
		return nil, fmt.Errorf("create pull request in %s/%s: %w", workspace, repoSlug, err)
	}

	var created PullRequest
	relPath := "/repositories/" + workspace + "/" + repoSlug + "/pullrequests"
	status, _, err := c.writeJSON(ctx, http.MethodPost, relPath, body, &created)
	if err != nil {
		return nil, fmt.Errorf("create pull request in %s/%s: %w", workspace, repoSlug, err)
	}
	// The endpoint answers 201 with the pull request. A different status, or a
	// body without an id, would hand back a zero PullRequest the caller then
	// shows as "#0".
	if status != http.StatusCreated || created.ID == 0 {
		return nil, fmt.Errorf("create pull request in %s/%s: unexpected status %d without a pull request body",
			workspace, repoSlug, status)
	}
	return &created, nil
}

// body validates the request and builds the document.
func (n NewPullRequest) body() (newPullRequestBody, error) {
	title := strings.TrimSpace(n.Title)
	if title == "" {
		return newPullRequestBody{}, ErrEmptyTitle
	}
	source := strings.TrimSpace(n.SourceBranch)
	if source == "" {
		return newPullRequestBody{}, ErrNoSourceBranch
	}

	body := newPullRequestBody{
		Title:             title,
		Description:       n.Description,
		Source:            branchRef{Branch: branchName{Name: source}},
		Destination:       nil,
		Reviewers:         nil,
		Draft:             n.Draft,
		CloseSourceBranch: n.CloseSourceBranch,
	}
	if destination := strings.TrimSpace(n.DestinationBranch); destination != "" {
		body.Destination = &branchRef{Branch: branchName{Name: destination}}
	}
	for _, uuid := range n.Reviewers {
		if uuid != "" {
			body.Reviewers = append(body.Reviewers, reviewerRef{UUID: uuid})
		}
	}
	return body, nil
}
