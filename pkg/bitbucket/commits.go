package bitbucket

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// Commit list bounds. The endpoint's pagelen defaults to 10 and caps at 100,
// and a long-lived branch can carry thousands of commits, so the default item
// bound keeps a runaway history from pulling fifty pages.
const (
	maxPageLenCommits = 100
	defaultMaxCommits = 500
)

// ListPullRequestCommits returns the commits a pull request would merge, newest
// first — the endpoint answers in reverse-chronological order and the list is
// passed on as it came, so callers that want oldest-first have to reverse it.
//
// The list is empty — not an error — when the source branch has been deleted,
// which is the normal state of a merged pull request. The collection is
// iterator-paginated: the envelope carries no size and next holds an opaque
// token, so it is walked by following next and never by page number.
func (c *Client) ListPullRequestCommits(
	ctx context.Context, workspace, repoSlug string, id int, opts ListOptions,
) ([]Commit, error) {
	maxItems := orDefault(opts.MaxItems, defaultMaxCommits)

	query := url.Values{}
	query.Set("pagelen", strconv.Itoa(pageLen(maxPageLenCommits, maxItems)))

	path := pullRequestPath(workspace, repoSlug, id) + "/commits"
	commits, err := getAll[Commit](ctx, c, path, query, maxItems)
	if err != nil {
		return nil, fmt.Errorf("list commits %s/%s#%d: %w", workspace, repoSlug, id, err)
	}
	return commits, nil
}
