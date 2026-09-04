package bitbucket

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"strconv"
)

// ListPullRequests returns the pull requests of a repository, newest update
// first, defaulting to the open ones.
//
// The collection endpoint omits reviewers and participants because they are
// expensive, so by default the request asks for them back with
// fields=+values.reviewers,+values.participants — that is what lets the list
// pane show approvals without one detail request per row. Should the server
// reject the projection, the whole listing is retried once without it and the
// review data stays nil (PullRequest.HasReviewData reports that).
func (c *Client) ListPullRequests(
	ctx context.Context, workspace, repoSlug string, opts ListPullRequestsOptions,
) ([]PullRequest, error) {
	maxItems := orDefault(opts.MaxItems, defaultMaxPullRequests)
	path := repoPath(workspace, repoSlug) + pullRequestsSegment

	query := url.Values{}
	query.Set("pagelen", strconv.Itoa(pageLen(maxPageLenPullRequests, maxItems)))
	query.Set("sort", orString(opts.Sort, "-updated_on"))
	states := opts.States
	if len(states) == 0 {
		states = []PullRequestState{PullRequestOpen}
	}
	for _, state := range states {
		query.Add("state", string(state))
	}
	if opts.Query != "" {
		query.Set("q", opts.Query)
	}

	fields := orString(opts.Fields, defaultPullRequestFields)
	projected := query
	if fields != noFields {
		projected = maps.Clone(query)
		projected.Set("fields", fields)
	}

	prs, pages, err := getAllCounted[PullRequest](ctx, c, path, projected, maxItems)
	if err != nil && pages == 0 && fields != noFields && isBadRequest(err) {
		// Only the first request carries fields we built ourselves; a later page
		// comes from an opaque next link that must never be rebuilt.
		prs, _, err = getAllCounted[PullRequest](ctx, c, path, query, maxItems)
	}
	if err != nil {
		return nil, fmt.Errorf("list pull requests %s/%s: %w", workspace, repoSlug, err)
	}
	return prs, nil
}

// GetPullRequest returns one pull request. Unlike a list item it always carries
// reviewers and participants, so it is the fallback when the list projection
// was refused.
func (c *Client) GetPullRequest(ctx context.Context, workspace, repoSlug string, id int) (*PullRequest, error) {
	var pr PullRequest
	if err := c.getJSON(ctx, pullRequestPath(workspace, repoSlug, id), nil, &pr); err != nil {
		return nil, fmt.Errorf("get pull request %s/%s#%d: %w", workspace, repoSlug, id, err)
	}
	return &pr, nil
}

// ListPullRequestStatuses returns the build statuses posted against the pull
// request's head commit. Bitbucket does not order them newest-first and a
// re-run posts a second status under the same key, so reduce them with
// LatestStatuses or RollupStatuses before showing anything.
func (c *Client) ListPullRequestStatuses(
	ctx context.Context, workspace, repoSlug string, id int,
) ([]CommitStatus, error) {
	path := pullRequestPath(workspace, repoSlug, id) + "/statuses"

	query := url.Values{}
	query.Set("pagelen", strconv.Itoa(maxPageLenDefault))

	statuses, err := getAll[CommitStatus](ctx, c, path, query, 0)
	if err != nil {
		return nil, fmt.Errorf("list statuses %s/%s#%d: %w", workspace, repoSlug, id, err)
	}
	return statuses, nil
}

// pullRequestsSegment is the collection path segment shared by the pull request
// endpoints.
const pullRequestsSegment = "/pullrequests"

// pullRequestPath is the API path of one pull request, the prefix every PR
// sub-resource (statuses, commits, comments, diff, diffstat) hangs off.
func pullRequestPath(workspace, repoSlug string, id int) string {
	return repoPath(workspace, repoSlug) + pullRequestsSegment + "/" + strconv.Itoa(id)
}

// isBadRequest reports whether err is a 400 from the API. An unsupported fields
// projection is reported that way, with no detail to distinguish it.
func isBadRequest(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusBadRequest
}
