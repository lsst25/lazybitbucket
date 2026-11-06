package bitbucket

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// maxPageLenDiffStat is the pagelen the diffstat request asks for. The endpoint
// allows up to 5000, but a page that large is a slow response for a pane that
// only ever shows a screenful: 500 covers all but the most sprawling pull
// requests in one round trip.
const maxPageLenDiffStat = 500

// ListPullRequestDiffStat returns one entry per file the pull request changes:
// status and added/removed line counts, with no diff text. It is the file list
// of the diff pane.
//
// The endpoint answers 302 and redirects to the repository diffstat for the
// pull request's revspec, carrying undocumented query parameters
// (from_pullrequest_id, exclude_files) that change the result — which is why
// the redirect is followed rather than the target rebuilt. Both hops are on the
// same host, so the default redirect policy keeps the Authorization header, and
// the paginator follows the target's own next links from there.
//
// Unlike the diff endpoint this one can afford to let net/http follow the hop:
// the only parameter it sends is pagelen, and losing that across the redirect
// costs pages, not entries. GetPullRequestDiff sends parameters it cannot lose.
//
// A pull request too large to diffstat answers 555; IsDiffTooLarge reports that.
func (c *Client) ListPullRequestDiffStat(
	ctx context.Context, workspace, repoSlug string, id int,
) ([]DiffStatEntry, error) {
	query := url.Values{}
	query.Set("pagelen", strconv.Itoa(maxPageLenDiffStat))

	path := pullRequestPath(workspace, repoSlug, id) + "/diffstat"
	entries, err := getAll[DiffStatEntry](ctx, c, path, query, 0)
	if err != nil {
		return nil, fmt.Errorf("get diffstat %s/%s#%d: %w", workspace, repoSlug, id, err)
	}
	return entries, nil
}

// maxDiffStatPaths is the most path= filters CompareDiffStat sends. Past it the
// query string grows long enough to worry a proxy, and the filter is dropped:
// the answer is then every file that changed, which is still correct.
const maxDiffStatPaths = 100

// CompareDiffStat returns one entry per file whose content differs between two
// commits of a repository: GET /repositories/{ws}/{slug}/diffstat/{from}..{to}
// with topic=false, a plain two-dot comparison rather than a pull request's
// three-dot one. paths limits the answer to those files, and nil asks for every
// file. Note the order: Bitbucket reads the spec as "from compared to to",
// which is the opposite of `git diff` — the first commit is the newer one.
//
// It is what tells the viewed-file marks which files a push changed. A commit
// that no longer exists — a force push the server has already collected — is a
// 404, which IsNotFound reports.
func (c *Client) CompareDiffStat(
	ctx context.Context, workspace, repoSlug, from, to string, paths []string,
) ([]DiffStatEntry, error) {
	query := url.Values{}
	query.Set("pagelen", strconv.Itoa(maxPageLenDiffStat))
	query.Set("topic", "false")
	if len(paths) <= maxDiffStatPaths {
		for _, path := range paths {
			query.Add("path", path)
		}
	}

	path := repoPath(workspace, repoSlug) + "/diffstat/" + url.PathEscape(from) + ".." + url.PathEscape(to)
	entries, err := getAll[DiffStatEntry](ctx, c, path, query, 0)
	if err != nil {
		return nil, fmt.Errorf("compare %s/%s %s..%s: %w", workspace, repoSlug, from, to, err)
	}
	return entries, nil
}

// GetPullRequestDiff returns a raw git-style unified diff for the pull request,
// or for the single file named by opts.Path.
//
// The body is returned exactly as the server sent it. Bitbucket does not decode
// it to Unicode — the diff carries whatever encoding the repository's files use
// — so the string may not be valid UTF-8 and callers must tolerate that rather
// than assume it away.
//
// Like the diffstat endpoint this one 302s to the repository diff for the pull
// request's revspec, and answers 555 when the diff was too large to build
// (IsDiffTooLarge). Fetching one file at a time with opts.Path is what keeps a
// big pull request out of that case — which is why the redirect is resolved by
// hand rather than followed: see fetchDiffViaRedirect.
func (c *Client) GetPullRequestDiff(
	ctx context.Context, workspace, repoSlug string, id int, opts DiffOptions,
) (string, error) {
	query := url.Values{}
	if opts.Path != "" {
		query.Set("path", opts.Path)
	}
	if opts.Context > 0 {
		query.Set("context", strconv.Itoa(opts.Context))
	}

	body, err := c.fetchDiff(ctx, pullRequestPath(workspace, repoSlug, id)+"/diff", opts.DiffURL, query)
	if err != nil {
		return "", fmt.Errorf("get diff %s/%s#%d: %w", workspace, repoSlug, id, err)
	}
	return string(body), nil
}

// fetchDiff GETs a raw diff, preferring a pre-resolved same-origin diff link
// over the pull request's own /diff path. The link already carries the
// parameters the redirect would have added, so using it saves the extra hop;
// query is merged into whatever it holds already.
func (c *Client) fetchDiff(ctx context.Context, relPath, diffURL string, query url.Values) ([]byte, error) {
	if target := c.sameOriginURL(diffURL); target != nil {
		return c.getDiffURL(ctx, mergeQuery(target, query))
	}
	return c.fetchDiffViaRedirect(ctx, relPath, query)
}

// fetchDiffViaRedirect resolves the /diff endpoint's 302 itself instead of
// letting net/http follow it.
//
// The redirect cannot be followed, because a followed redirect loses the
// request's query: Go builds the second request from the Location alone, and
// the Location carries only the parameters the server chose for itself
// (from_pullrequest_id, exclude_files). Sending path= and context= to the pull
// request endpoint and letting the hop happen therefore answers with the whole
// pull request diff — the very response opts.Path exists to avoid.
//
// So the first hop is made without following, and the caller's parameters are
// merged onto the Location before the second hop is made. The first hop carries
// them as well: the real API ignores them when building the Location, but a
// deployment that answers the diff directly instead of redirecting (a caching
// proxy) is served by that first hop and must still apply path= and context=.
func (c *Client) fetchDiffViaRedirect(ctx context.Context, relPath string, query url.Values) ([]byte, error) {
	probeURL := c.baseURL + relPath
	if len(query) > 0 {
		probeURL += "?" + query.Encode()
	}
	probe, err := c.do(ctx, c.noRedirectClient(), getSpec(probeURL, relPath, acceptText, nil))
	if err != nil {
		return nil, err
	}
	if !isRedirect(probe.status) {
		// An error status has to keep reading as one: 555 for a diff too large
		// is the whole reason the diff pane fetches per file.
		return probe.body, c.statusError(http.MethodGet, relPath, probe)
	}

	target, err := c.diffRedirectTarget(c.baseURL+relPath, probe.header.Get("Location"))
	if err != nil {
		return nil, err
	}
	return c.getDiffURL(ctx, mergeQuery(target, query))
}

// getDiffURL fetches an absolute diff URL as text, deriving the BaseURL-relative
// path the log and APIError report from it.
func (c *Client) getDiffURL(ctx context.Context, fullURL string) ([]byte, error) {
	body, _, err := c.request(ctx, fullURL, strings.TrimPrefix(fullURL, c.baseURL), acceptText, nil)
	return body, err
}

// noRedirectClient is the client's own HTTP client with the redirect policy
// swapped for "hand me the 3xx". It is a shallow copy, so the transport, the
// cookie jar and the timeout are shared and c.httpClient keeps the default
// policy — which the diffstat walk and the pipeline step log both rely on.
func (c *Client) noRedirectClient() *http.Client {
	noRedirect := *c.httpClient
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &noRedirect
}

// isRedirect reports whether a status is a 3xx, i.e. one whose Location is the
// answer rather than the body.
func isRedirect(status int) bool {
	return status >= http.StatusMultipleChoices && status < http.StatusBadRequest
}

// diffRedirectTarget resolves the Location of the /diff redirect — relative or
// absolute — against the URL that was requested, and refuses a target that is
// not on the API host. The second hop carries the Authorization header, so the
// payload does not get to choose where it goes.
func (c *Client) diffRedirectTarget(requestURL, location string) (*url.URL, error) {
	if location == "" {
		return nil, errDiffRedirectNoLocation
	}

	requested, err := url.Parse(requestURL)
	if err != nil {
		return nil, fmt.Errorf("parse diff URL %q: %w", requestURL, err)
	}
	target, err := requested.Parse(location)
	if err != nil {
		return nil, fmt.Errorf("parse diff redirect %q: %w", location, err)
	}
	if host, ok := c.sameOrigin(target); !ok {
		return nil, fmt.Errorf("refusing to follow diff redirect to %s (expected host %s)", target.Host, host)
	}
	return target, nil
}

// errDiffRedirectNoLocation is the 3xx with nothing to follow.
var errDiffRedirectNoLocation = errors.New("diff redirect carried no Location header")

// sameOriginURL parses an API-supplied absolute URL and returns it only when it
// shares scheme and host with BaseURL. Anything else — a parse failure, another
// host, a downgraded scheme — yields nil, so the Authorization header is never
// sent to a location the payload chose.
func (c *Client) sameOriginURL(absURL string) *url.URL {
	if absURL == "" {
		return nil
	}
	target, err := url.Parse(absURL)
	if err != nil {
		return nil
	}
	if _, ok := c.sameOrigin(target); !ok {
		return nil
	}
	return target
}

// sameOrigin reports whether target shares scheme and host with BaseURL, and
// returns BaseURL's host so a refusal can name the host it expected instead.
func (c *Client) sameOrigin(target *url.URL) (string, bool) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return "", false
	}
	return base.Host, target.Scheme == base.Scheme && target.Host == base.Host
}

// mergeQuery returns target with extra's parameters set on it, replacing any
// same-named parameters the URL already carried and leaving the rest in place.
func mergeQuery(target *url.URL, extra url.Values) string {
	if len(extra) == 0 {
		return target.String()
	}

	merged := *target
	values := merged.Query()
	maps.Copy(values, extra)
	merged.RawQuery = values.Encode()
	return merged.String()
}
