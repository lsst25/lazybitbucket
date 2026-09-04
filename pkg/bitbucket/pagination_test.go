package bitbucket

import (
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// Three single-item pages. pagedServer injects the "next" link; "size" is
// present here and absent from page two on purpose — only "values" and "next"
// are guaranteed by the API.
const (
	repoPageOne = `{
  "pagelen": 100,
  "page": 1,
  "size": 3,
  "values": [
    {"type": "repository", "uuid": "{aaa}", "slug": "api-gateway", "name": "api-gateway",
     "full_name": "acme/api-gateway", "updated_on": "2026-09-03T17:02:02.828459+00:00"}
  ]
}`
	repoPageTwo = `{
  "pagelen": 100,
  "values": [
    {"type": "repository", "uuid": "{bbb}", "slug": "web-app", "name": "web-app",
     "full_name": "acme/web-app", "updated_on": "2026-09-02T17:02:02.828459+00:00"}
  ]
}`
	repoPageThree = `{
  "pagelen": 100,
  "values": [
    {"type": "repository", "uuid": "{ccc}", "slug": "infra", "name": "infra",
     "full_name": "acme/infra", "updated_on": "2026-09-01T17:02:02.828459+00:00"}
  ]
}`
)

// Two-item pages, for the MaxItems bound.
const (
	repoPairOne = `{
  "pagelen": 3,
  "values": [
    {"type": "repository", "slug": "api-gateway", "full_name": "acme/api-gateway"},
    {"type": "repository", "slug": "web-app", "full_name": "acme/web-app"}
  ]
}`
	repoPairTwo = `{
  "pagelen": 3,
  "values": [
    {"type": "repository", "slug": "infra", "full_name": "acme/infra"},
    {"type": "repository", "slug": "docs", "full_name": "acme/docs"}
  ]
}`
	repoPairThree = `{
  "pagelen": 3,
  "values": [
    {"type": "repository", "slug": "tools", "full_name": "acme/tools"}
  ]
}`
)

func TestGetAll_FollowsNextAcrossPages(t *testing.T) {
	t.Parallel()

	client, recorded := pagedServer(t, []string{repoPageOne, repoPageTwo, repoPageThree})

	repos, err := client.ListRepositories(t.Context(), testWorkspace,
		ListRepositoriesOptions{Role: "", Query: "", Sort: "", MaxItems: 0})
	if err != nil {
		t.Fatalf("ListRepositories() error = %v", err)
	}

	got := make([]string, 0, len(repos))
	for _, repo := range repos {
		got = append(got, repo.Slug)
	}
	testkit.AssertSliceEqual(t, "slugs", got, []string{testRepoSlug, "web-app", "infra"})

	requests := *recorded
	if len(requests) != 3 {
		t.Fatalf("request count = %d, want 3", len(requests))
	}
	testkit.AssertEqual(t, "page 1 pagelen", requests[0].Query.Get(paramPageLen), "100")
	testkit.AssertEqual(t, "page 2 page param", requests[1].Query.Get("page"), "2")
	testkit.AssertEqual(t, "page 3 page param", requests[2].Query.Get("page"), "3")
	testkit.AssertEqual(t, "page 3 path", requests[2].Path, "/repositories/acme")
}

func TestGetAll_NextIsUsedVerbatim(t *testing.T) {
	t.Parallel()

	client, recorded := pagedServer(t, []string{repoPageOne, repoPageTwo})

	if _, err := client.ListRepositories(t.Context(), testWorkspace,
		ListRepositoriesOptions{Role: "member", Query: "", Sort: "", MaxItems: 0}); err != nil {
		t.Fatalf("ListRepositories() error = %v", err)
	}

	requests := *recorded
	if len(requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(requests))
	}
	// The injected link carries only "page"; a client that rebuilt the query
	// from its own options would have re-sent role/sort/pagelen as well.
	testkit.AssertEqual(t, "page 1 role", requests[0].Query.Get("role"), "member")
	testkit.AssertEqual(t, "page 2 role", requests[1].Query.Get("role"), "")
	testkit.AssertEqual(t, "page 2 pagelen", requests[1].Query.Get(paramPageLen), "")
}

func TestGetAll_StopsAndTruncatesAtMaxItems(t *testing.T) {
	t.Parallel()

	client, recorded := pagedServer(t, []string{repoPairOne, repoPairTwo, repoPairThree})

	repos, err := client.ListRepositories(t.Context(), testWorkspace,
		ListRepositoriesOptions{Role: "", Query: "", Sort: "", MaxItems: 3})
	if err != nil {
		t.Fatalf("ListRepositories() error = %v", err)
	}

	got := make([]string, 0, len(repos))
	for _, repo := range repos {
		got = append(got, repo.Slug)
	}
	testkit.AssertSliceEqual(t, "slugs", got, []string{testRepoSlug, "web-app", "infra"})

	requests := *recorded
	if len(requests) != 2 {
		t.Fatalf("request count = %d, want 2 (the third page must not be fetched)", len(requests))
	}
	testkit.AssertEqual(t, "pagelen clamped to MaxItems", requests[0].Query.Get(paramPageLen), "3")
}

func TestGetAll_PageLenClampedOnlyBelowTheEndpointCeiling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		maxItems int
		want     string
	}{
		{name: "default uses the endpoint ceiling", maxItems: 0, want: "100"},
		{name: "smaller bound shrinks the page", maxItems: 25, want: "25"},
		{name: "larger bound keeps the ceiling", maxItems: 500, want: "100"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, recorded := newRecordingClient(t, bearerOpts(),
				testkit.StubResponse{Status: http.StatusOK, Body: emptyPage})

			if _, err := client.ListRepositories(t.Context(), testWorkspace,
				ListRepositoriesOptions{Role: "", Query: "", Sort: "", MaxItems: tt.maxItems}); err != nil {
				t.Fatalf("ListRepositories() error = %v", err)
			}

			testkit.AssertEqual(t, paramPageLen, recorded.Query.Get(paramPageLen), tt.want)
		})
	}
}

func TestGetAll_RefusesForeignHostNextLink(t *testing.T) {
	t.Parallel()

	var requests atomic.Int64
	client := newHandlerClient(t, func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writeJSON(t, w, http.StatusOK,
			`{"pagelen":100,"values":[{"type":"repository","slug":"api-gateway"}],`+
				`"next":"https://evil.example.com/2.0/repositories/acme?page=2"}`)
	})

	_, err := client.ListRepositories(t.Context(), testWorkspace,
		ListRepositoriesOptions{Role: "", Query: "", Sort: "", MaxItems: 0})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "refusing to follow pagination link to https://evil.example.com") {
		t.Errorf("error %q does not refuse the foreign host", err.Error())
	}
	if !strings.Contains(err.Error(), "list repositories acme") {
		t.Errorf("error %q lacks the method prefix", err.Error())
	}
	testkit.AssertEqual(t, "requests made", requests.Load(), int64(1))
}

func TestGetAll_RefusesSchemeChangeOnNextLink(t *testing.T) {
	t.Parallel()

	client := newHandlerClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentTypeJSON)
		_, _ = w.Write(selfNextPage(t, r, "https", "/repositories/acme", nil))
	})

	_, err := client.ListRepositories(t.Context(), testWorkspace,
		ListRepositoriesOptions{Role: "", Query: "", Sort: "", MaxItems: 0})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "refusing to follow pagination link") {
		t.Errorf("error %q does not refuse the scheme change", err.Error())
	}
}

func TestGetAll_PropagatesLaterPageFailure(t *testing.T) {
	t.Parallel()

	var requests atomic.Int64
	client := newHandlerClient(t, func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.Header().Set("Content-Type", contentTypeJSON)
			_, _ = w.Write(selfNextPage(t, r, "http", "/repositories/acme",
				[]any{map[string]string{"type": "repository", "slug": testRepoSlug}}))
			return
		}
		writeJSON(t, w, http.StatusInternalServerError, `{"type":"error","error":{"message":"Boom"}}`)
	})

	_, err := client.ListRepositories(t.Context(), testWorkspace,
		ListRepositoriesOptions{Role: "", Query: "", Sort: "", MaxItems: 0})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Errorf("error %q does not report status 500", err.Error())
	}
	testkit.AssertEqual(t, "requests made", requests.Load(), int64(2))
}
