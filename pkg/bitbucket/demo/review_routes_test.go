package demo

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// These tests drive the four review routes over raw HTTP, so the wire shapes —
// the iterator envelope, the 302 to the revspec, the text/plain diff and the
// 555 — are asserted independently of pkg/bitbucket's decoding.

func TestServer_ListPullRequestCommits(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	body := getOK(t, server, pullRequestPath(repoAPIGateway, 103)+segmentCommits)
	page := decodeJSON[iteratorPage[bitbucket.Commit]](t, body)

	testkit.AssertEqual(t, "len(values)", len(page.Values), 3)
	testkit.AssertEqual(t, "next", page.Next, "")
	// Newest first, and the newest commit is the pull request's own head.
	testkit.AssertEqual(t, "values[0].Hash", page.Values[0].Hash, hashPR103)
	testkit.AssertEqual(t, "values[0].ShortHash()", page.Values[0].ShortHash(), hashPR103[:7])
	testkit.AssertEqual(t, "values[0].Subject()", page.Values[0].Subject(),
		"upstream: derive a fresh deadline per retry attempt")
	testkit.AssertEqual(t, "values[0].AuthorName()", page.Values[0].AuthorName(), "Linus Torvalds")
	testkit.AssertEqual(t, "len(values[0].Parents)", len(page.Values[0].Parents), 1)
	testkit.AssertEqual(t, "values[2].Parents[0].Hash", page.Values[2].Parents[0].Hash, hashMainAPI)

	// An iterator-paginated collection states no size: the server will not count
	// the collection, and the client must not expect it to.
	if _, ok := decodeRaw(t, body)[fieldSize]; ok {
		t.Fatalf("commits envelope carries a %q member: %s", fieldSize, body)
	}
}

// TestServer_ListPullRequestCommitsUsesAnOpaquePageToken pins the pagination
// shape of the commits endpoint: the next link's page parameter is a token, not
// a page number, so a client can only follow it and never rebuild it.
func TestServer_ListPullRequestCommitsUsesAnOpaquePageToken(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	server.SetMaxPageLen(2)

	first := decodeJSON[iteratorPage[bitbucket.Commit]](t,
		getOK(t, server, pullRequestPath(repoWebApp, 40)+segmentCommits))
	testkit.AssertEqual(t, "len(first page)", len(first.Values), 2)
	testkit.AssertEqual(t, "first page pagelen", first.PageLen, 2)
	if first.Next == "" {
		t.Fatalf("first page has no next link: %+v", first)
	}

	next, err := url.Parse(first.Next)
	if err != nil {
		t.Fatalf("parse next %q: %v", first.Next, err)
	}
	token := next.Query().Get(fieldPage)
	if token == "" {
		t.Fatalf("next link %q carries no %q parameter", first.Next, fieldPage)
	}
	if _, err := strconv.Atoi(token); err == nil {
		t.Fatalf("next page token %q is a plain page number, want an opaque token", token)
	}

	body := getOK(t, server, next.RequestURI())
	second := decodeJSON[iteratorPage[bitbucket.Commit]](t, body)
	testkit.AssertEqual(t, "len(second page)", len(second.Values), 2)

	// The page member of an iterator envelope is a string, which is why the
	// client refuses to model it as a number.
	if raw := string(decodeRaw(t, body)[fieldPage]); !strings.HasPrefix(raw, `"`) {
		t.Fatalf("second page %q member = %s, want a quoted token", fieldPage, raw)
	}
	testkit.AssertEqual(t, "second page next", second.Next, "")
	testkit.AssertEqual(t, "no overlap between the pages",
		second.Values[0].Hash != first.Values[0].Hash && second.Values[0].Hash != first.Values[1].Hash,
		true)
}

func TestServer_ListPullRequestCommitsRejectsABadPageToken(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	got := get(t, server.URL+pullRequestPath(repoAPIGateway, 103)+segmentCommits+"?page=not-a-token%21", nil)

	testkit.AssertEqual(t, "status", got.Status, http.StatusBadRequest)
	testkit.AssertEqual(t, "type", decodeJSON[errorBody](t, got.Body).Type, "error")
}

// TestServer_ListPullRequestCommitsEmptyForAClosedPullRequest documents the
// dataset's stance: a merged pull request's source branch is gone, so the
// endpoint answers an empty list rather than an error.
func TestServer_ListPullRequestCommitsEmptyForAClosedPullRequest(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	page := decodeJSON[iteratorPage[bitbucket.Commit]](t,
		getOK(t, server, pullRequestPath(repoAPIGateway, 98)+segmentCommits))

	testkit.AssertEqual(t, "len(values)", len(page.Values), 0)
}

func TestServer_ListPullRequestComments(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	body := getOK(t, server, pullRequestPath(repoAPIGateway, 103)+segmentComments+"?sort=created_on")
	page := decodeJSON[listPage[bitbucket.Comment]](t, body)

	testkit.AssertEqual(t, "size", page.Size, 9)
	testkit.AssertEqual(t, "len(values)", len(page.Values), 9)

	// sort=created_on is ascending: the oldest comment leads.
	for index := 1; index < len(page.Values); index++ {
		if page.Values[index-1].CreatedOn.After(page.Values[index].CreatedOn) {
			t.Fatalf("comment %d is older than its predecessor %d",
				page.Values[index].ID, page.Values[index-1].ID)
		}
	}

	// The paginated collections do state their size, unlike the commits one.
	if _, ok := decodeRaw(t, body)[fieldSize]; !ok {
		t.Fatalf("comments envelope has no %q member: %s", fieldSize, body)
	}
}

func TestServer_ListPullRequestCommentsPagination(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	page := decodeJSON[listPage[bitbucket.Comment]](t,
		getOK(t, server, pullRequestPath(repoAPIGateway, 103)+segmentComments+queryPageLenTwo))

	testkit.AssertEqual(t, "len(values)", len(page.Values), 2)
	testkit.AssertEqual(t, "size", page.Size, 9)
	if !strings.Contains(page.Next, fieldPage+"=2") {
		t.Fatalf("next = %q, want it to ask for page 2", page.Next)
	}
}

func TestServer_ListPullRequestCommentsUnknownPullRequestIsNotFound(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	got := get(t, server.URL+pullRequestPath(repoAPIGateway, 4242)+segmentComments, nil)

	testkit.AssertEqual(t, "status", got.Status, http.StatusNotFound)
}

// TestServer_PullRequestDiffStatRedirectsToTheRevspec pins the 302: the pull
// request endpoint does not serve the document, it points at the
// repository-level resource for the pull request's revspec.
func TestServer_PullRequestDiffStatRedirectsToTheRevspec(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	got := getNoRedirect(t, server.URL+pullRequestPath(repoAPIGateway, 103)+segmentDiffStat)

	testkit.AssertEqual(t, "status", got.Status, http.StatusFound)

	location := got.Header.Get(headerLocation)
	target, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse Location %q: %v", location, err)
	}
	wantPath := pathRepositories + "/" + repoAPIGateway + segmentDiffStat + "/" +
		Workspace + "/" + repoAPIGateway + ":" + shortHash(hashPR103) + "%0D" + shortHash(hashMainAPI)
	testkit.AssertEqual(t, "Location path", target.EscapedPath(), wantPath)
	testkit.AssertEqual(t, "from_pullrequest_id", target.Query().Get(queryFromPullRequest), "103")
	testkit.AssertEqual(t, "topic", target.Query().Get(queryTopic), "true")
}

// TestServer_PullRequestDiffRedirectDropsTheCallerQuery pins the shape the real
// API has: the Location carries the server's own parameters and nothing the
// caller sent. That is precisely why the client cannot let net/http follow the
// hop — the redirected request is built from the Location alone, so path= and
// context= would be lost and the whole diff served — and a fake that copied the
// query on would hide the bug instead of catching it.
func TestServer_PullRequestDiffRedirectDropsTheCallerQuery(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	got := getNoRedirect(t, server.URL+pullRequestPath(repoAPIGateway, 103)+segmentDiff+
		"?path="+url.QueryEscape(pathRetryGo)+"&context=1")

	testkit.AssertEqual(t, "status", got.Status, http.StatusFound)

	target, err := url.Parse(got.Header.Get(headerLocation))
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	if !strings.HasPrefix(target.EscapedPath(), pathRepositories+"/"+repoAPIGateway+segmentDiff+"/") {
		t.Fatalf("Location path = %q, want the repository diff resource", target.EscapedPath())
	}
	testkit.AssertEqual(t, "path", target.Query().Get(queryPath), "")
	testkit.AssertEqual(t, "context", target.Query().Get(queryContext), "")
	testkit.AssertEqual(t, "from_pullrequest_id", target.Query().Get(queryFromPullRequest), "103")
	testkit.AssertEqual(t, "topic", target.Query().Get(queryTopic), "true")
	testkit.AssertEqual(t, "parameters", len(target.Query()), 2)
}

// TestServer_PullRequestLinksPointAtThisServer covers the rewrite NewServer does:
// the client prefers the payload's diff link over the redirect, so the link has
// to be same-origin and has to work.
func TestServer_PullRequestLinksPointAtThisServer(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	pullRequest := demoPullRequest(t, server, repoAPIGateway, 103)

	for label, href := range map[string]string{
		"Links.Diff":     pullRequest.Links.Diff.Href,
		"Links.DiffStat": pullRequest.Links.DiffStat.Href,
	} {
		if !strings.HasPrefix(href, server.URL+"/") {
			t.Fatalf("%s = %q, want it on %s", label, href, server.URL)
		}
		if !strings.Contains(href, "%0D") {
			t.Fatalf("%s = %q, want a %%0D-separated revspec", label, href)
		}
		if !strings.Contains(href, queryFromPullRequest+"=103") {
			t.Fatalf("%s = %q, want from_pullrequest_id=103", label, href)
		}
	}

	// Both links answer on their own, without going through the redirect.
	testkit.AssertEqual(t, "diff status", get(t, pullRequest.Links.Diff.Href, nil).Status, http.StatusOK)
	testkit.AssertEqual(t, "diffstat status",
		get(t, pullRequest.Links.DiffStat.Href, nil).Status, http.StatusOK)
}

func TestServer_RepositoryDiffStat(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	pullRequest := demoPullRequest(t, server, repoAPIGateway, 103)
	body := get(t, pullRequest.Links.DiffStat.Href, nil).Body
	page := decodeJSON[listPage[bitbucket.DiffStatEntry]](t, body)

	testkit.AssertEqual(t, "size", page.Size, 3)
	testkit.AssertSliceEqual(t, "paths", pathsOf(page.Values), []string{
		pathRetryGo, pathRetryTestGo, "docs/rfc-014.md",
	})
	testkit.AssertSliceEqual(t, "statuses", statusesOf(page.Values), []string{
		bitbucket.DiffStatModified, bitbucket.DiffStatAdded, bitbucket.DiffStatModified,
	})

	// The paginated diffstat does state its size.
	if _, ok := decodeRaw(t, body)[fieldSize]; !ok {
		t.Fatalf("diffstat envelope has no %q member: %s", fieldSize, body)
	}
}

func TestServer_RepositoryDiffStatPaginates(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	server.SetMaxPageLen(2)
	pullRequest := demoPullRequest(t, server, repoAPIGateway, 103)

	page := decodeJSON[listPage[bitbucket.DiffStatEntry]](t,
		get(t, pullRequest.Links.DiffStat.Href, nil).Body)

	testkit.AssertEqual(t, "len(values)", len(page.Values), 2)
	testkit.AssertEqual(t, "size", page.Size, 3)
	if !strings.HasPrefix(page.Next, server.URL) {
		t.Fatalf("next = %q, want it on %s", page.Next, server.URL)
	}

	rest := decodeJSON[listPage[bitbucket.DiffStatEntry]](t, get(t, page.Next, nil).Body)
	testkit.AssertEqual(t, "len(second page)", len(rest.Values), 1)
	testkit.AssertEqual(t, "second page next", rest.Next, "")
}

func TestServer_RepositoryDiffServesTheWholeDiffAsText(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	pullRequest := demoPullRequest(t, server, repoAPIGateway, 103)
	got := get(t, pullRequest.Links.Diff.Href, nil)

	testkit.AssertEqual(t, "status", got.Status, http.StatusOK)
	testkit.AssertEqual(t, headerContentType, got.Header.Get(headerContentType), contentTypeText)
	testkit.AssertEqual(t, "body", string(got.Body), server.Data.Diffs[reviewKey(repoAPIGateway, 103)])
}

func TestServer_RepositoryDiffFiltersByRepeatedPath(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	pullRequest := demoPullRequest(t, server, repoAPIGateway, 103)

	oneFile := string(get(t, pullRequest.Links.Diff.Href+"&path="+url.QueryEscape(pathRetryGo), nil).Body)
	testkit.AssertEqual(t, "sections", strings.Count(oneFile, diffSection), 1)
	if !strings.Contains(oneFile, "+++ b/"+pathRetryGo) {
		t.Fatalf("filtered diff does not hold %s:\n%s", pathRetryGo, oneFile)
	}

	twoFiles := string(get(t, pullRequest.Links.Diff.Href+
		"&path="+url.QueryEscape(pathRetryGo)+
		"&path="+url.QueryEscape(pathRetryTestGo), nil).Body)
	testkit.AssertEqual(t, "sections", strings.Count(twoFiles, diffSection), 2)

	none := string(get(t, pullRequest.Links.Diff.Href+"&path=no/such/file.go", nil).Body)
	testkit.AssertEqual(t, "unknown path body", none, "")
}

// TestServer_RepositoryDiffFiltersARemovedFileByItsOldPath covers the one file
// whose path only exists on the old side.
func TestServer_RepositoryDiffFiltersARemovedFileByItsOldPath(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	pullRequest := demoPullRequest(t, server, repoWebApp, 41)

	const removed = "src/legacy/polyfills.ts"
	body := string(get(t, pullRequest.Links.Diff.Href+"&path="+url.QueryEscape(removed), nil).Body)

	testkit.AssertEqual(t, "sections", strings.Count(body, diffSection), 1)
	if !strings.Contains(body, "+++ /dev/null") {
		t.Fatalf("filtered diff is not the removed file:\n%s", body)
	}
}

// TestServer_RepositoryDiffHonoursContext trims the context around the change,
// hunk header included, so context= is visibly honoured rather than accepted
// and ignored.
func TestServer_RepositoryDiffHonoursContext(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	pullRequest := demoPullRequest(t, server, repoAPIGateway, 103)
	href := pullRequest.Links.Diff.Href + "&path=" + url.QueryEscape(pathRetryGo)

	full := string(get(t, href, nil).Body)
	if !strings.Contains(full, "@@ -12,9 +12,12 @@") {
		t.Fatalf("default diff lost its hunk header:\n%s", full)
	}

	clipped := string(get(t, href+"&context=1", nil).Body)
	if !strings.Contains(clipped, "@@ -14,5 +14,8 @@") {
		t.Fatalf("context=1 did not rewrite the hunk header:\n%s", clipped)
	}
	if strings.Contains(clipped, "budget, cancel := context.WithTimeout") {
		t.Fatalf("context=1 kept the third leading context line:\n%s", clipped)
	}
}

func TestServer_RepositoryDiffUnknownRevspecIsNotFound(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	path := pathRepositories + "/" + repoAPIGateway + segmentDiff + "/" +
		Workspace + "/" + repoAPIGateway + ":aaaaaaaaaaaa%0Dbbbbbbbbbbbb"
	got := get(t, server.URL+path, nil)

	testkit.AssertEqual(t, "status", got.Status, http.StatusNotFound)
	testkit.AssertEqual(t, "type", decodeJSON[errorBody](t, got.Body).Type, "error")
}

// TestServer_RepositoryDiffResolvesTheRevspecWithoutTheQuery proves the target
// URL stands on its own: with no from_pullrequest_id the two hashes in the
// revspec identify the pull request.
func TestServer_RepositoryDiffResolvesTheRevspecWithoutTheQuery(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	pullRequest := demoPullRequest(t, server, repoAPIGateway, 103)
	bare, _, _ := strings.Cut(pullRequest.Links.Diff.Href, "?")

	got := get(t, bare, nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusOK)
	testkit.AssertEqual(t, "body", string(got.Body), server.Data.Diffs[reviewKey(repoAPIGateway, 103)])
}

// TestServer_DiffTooLargeAnswers555 is the path the UI has to survive: a pull
// request Bitbucket refuses to diff answers its own nonstandard status, from
// both endpoints, through the redirect.
func TestServer_DiffTooLargeAnswers555(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	for _, segment := range []string{segmentDiff, segmentDiffStat} {
		got := get(t, server.URL+pullRequestPath(repoAPIGateway, 98)+segment, nil)

		testkit.AssertEqual(t, segment+" status", got.Status, bitbucket.StatusDiffTooLarge)
		testkit.AssertEqual(t, segment+" Content-Type",
			got.Header.Get(headerContentType), contentTypeJSON)

		envelope := decodeJSON[errorBody](t, got.Body)
		testkit.AssertEqual(t, segment+" type", envelope.Type, "error")
		if !strings.Contains(envelope.Error.Message, "too large") {
			t.Fatalf("%s error message = %q, want it to say the diff is too large",
				segment, envelope.Error.Message)
		}
	}
}

func pathsOf(entries []bitbucket.DiffStatEntry) []string {
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, entry.Path())
	}
	return paths
}

func statusesOf(entries []bitbucket.DiffStatEntry) []string {
	statuses := make([]string, 0, len(entries))
	for _, entry := range entries {
		statuses = append(statuses, entry.Status)
	}
	return statuses
}
