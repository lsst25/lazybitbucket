package bitbucket

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// defaultMaxWorkspaceMembers bounds the member walk when the caller names no
// limit. A large workspace has thousands of accounts and the reviewer picker
// shows a screenful, so the walk stops at five pages rather than paging until
// the workspace runs out.
const defaultMaxWorkspaceMembers = 500

// defaultReviewerEntry is one value of the effective-default-reviewers
// collection: the account plus where the rule came from, of which only the
// account is used.
type defaultReviewerEntry struct {
	Type         string  `json:"type"`
	ReviewerType string  `json:"reviewer_type"`
	User         Account `json:"user"`
}

// workspaceMembership is one value of the workspace members collection: the
// account plus the workspace it belongs to.
type workspaceMembership struct {
	Type string  `json:"type"`
	User Account `json:"user"`
}

// ListEffectiveDefaultReviewers returns the accounts Bitbucket adds to every new
// pull request in the repository, both the repository's own default reviewers
// and those inherited from its project.
//
// They are the first candidates the reviewer picker offers after the pull
// request's current reviewers. A token without permission to see them answers
// 403, which is not fatal: the picker degrades to the reviewers it already knows.
func (c *Client) ListEffectiveDefaultReviewers(
	ctx context.Context, workspace, repoSlug string,
) ([]Account, error) {
	query := url.Values{}
	query.Set("pagelen", strconv.Itoa(maxPageLenDefault))

	path := repoPath(workspace, repoSlug) + "/effective-default-reviewers"
	entries, err := getAll[defaultReviewerEntry](ctx, c, path, query, 0)
	if err != nil {
		return nil, fmt.Errorf("list default reviewers %s/%s: %w", workspace, repoSlug, err)
	}

	accounts := make([]Account, 0, len(entries))
	for _, entry := range entries {
		accounts = append(accounts, entry.User)
	}
	return accounts, nil
}

// ListWorkspaceMembers returns the accounts belonging to the workspace, which is
// the pool the reviewer picker offers beyond the pull request's own
// participants. opts.MaxItems bounds the walk; 0 means 500.
func (c *Client) ListWorkspaceMembers(
	ctx context.Context, workspace string, opts ListOptions,
) ([]Account, error) {
	maxItems := orDefault(opts.MaxItems, defaultMaxWorkspaceMembers)

	query := url.Values{}
	query.Set("pagelen", strconv.Itoa(pageLen(maxPageLenDefault, maxItems)))

	path := "/workspaces/" + url.PathEscape(workspace) + "/members"
	memberships, err := getAll[workspaceMembership](ctx, c, path, query, maxItems)
	if err != nil {
		return nil, fmt.Errorf("list workspace members %s: %w", workspace, err)
	}

	accounts := make([]Account, 0, len(memberships))
	for _, membership := range memberships {
		accounts = append(accounts, membership.User)
	}
	return accounts, nil
}
