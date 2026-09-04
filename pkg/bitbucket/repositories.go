package bitbucket

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// ListRepositories returns the repositories of a workspace, newest activity
// first, so the config wizard can offer the repositories worth watching.
//
// A collection endpoint filters rather than rejects: a token that cannot see a
// workspace gets an empty list, not a 403.
func (c *Client) ListRepositories(
	ctx context.Context, workspace string, opts ListRepositoriesOptions,
) ([]Repository, error) {
	maxItems := orDefault(opts.MaxItems, defaultMaxRepositories)

	query := url.Values{}
	query.Set("pagelen", strconv.Itoa(pageLen(maxPageLenDefault, maxItems)))
	query.Set("sort", orString(opts.Sort, "-updated_on"))
	if opts.Role != "" {
		query.Set("role", opts.Role)
	}
	if opts.Query != "" {
		query.Set("q", opts.Query)
	}

	repos, err := getAll[Repository](ctx, c, "/repositories/"+url.PathEscape(workspace), query, maxItems)
	if err != nil {
		return nil, fmt.Errorf("list repositories %s: %w", workspace, err)
	}
	return repos, nil
}

// GetRepository returns one repository. It is the way to tell "no open pull
// requests" from "this token cannot see the repository", which the list
// endpoint reports identically as an empty page.
func (c *Client) GetRepository(ctx context.Context, workspace, repoSlug string) (*Repository, error) {
	var repo Repository
	if err := c.getJSON(ctx, repoPath(workspace, repoSlug), nil, &repo); err != nil {
		return nil, fmt.Errorf("get repository %s/%s: %w", workspace, repoSlug, err)
	}
	return &repo, nil
}
