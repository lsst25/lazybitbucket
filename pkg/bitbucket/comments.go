package bitbucket

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// Comment list bounds. pagelen caps at 100; the item bound is generous because
// the collection holds every general comment, every inline comment and every
// reply of the pull request, and BuildThreads needs the whole set to attach
// replies to their roots.
const (
	maxPageLenComments = 100
	defaultMaxComments = 1000
)

// commentResolutionFields adds the members of a comment's resolution to what
// the collection sends. Every entry is a "+" addition, so the default document
// is untouched and only the three stripped members come back.
const commentResolutionFields = "+values.resolution.user," +
	"+values.resolution.created_on,+values.resolution.type"

// ListPullRequestComments returns every comment on a pull request — general,
// inline and replies alike — oldest first.
//
// The collection is flat: threading lives in each reply's parent link, so pass
// the result to BuildThreads to get root-and-replies threads. Sorting is
// requested explicitly rather than relying on the documented default, so the
// order the threads are built from does not depend on the server's mood.
//
// The resolution's members are asked for by name. A collection sends a resolved
// comment's resolution as an empty object — enough to know the thread is
// resolved, not enough to say who resolved it or when — and these three fields
// bring the rest back in the same request.
func (c *Client) ListPullRequestComments(
	ctx context.Context, workspace, repoSlug string, id int, opts ListOptions,
) ([]Comment, error) {
	maxItems := orDefault(opts.MaxItems, defaultMaxComments)

	query := url.Values{}
	query.Set("pagelen", strconv.Itoa(pageLen(maxPageLenComments, maxItems)))
	query.Set("sort", "created_on")
	query.Set("fields", commentResolutionFields)

	path := pullRequestPath(workspace, repoSlug, id) + "/comments"
	comments, err := getAll[Comment](ctx, c, path, query, maxItems)
	if err != nil {
		return nil, fmt.Errorf("list comments %s/%s#%d: %w", workspace, repoSlug, id, err)
	}
	return comments, nil
}
