package demo

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
)

// listPage mirrors Bitbucket's paginated envelope. The tests decode into it
// rather than into the client's own page type, because pkg/bitbucket.Client is
// written by another wave and this package must not depend on it.
type listPage[T any] struct {
	PageLen  int    `json:"pagelen"`
	Page     int    `json:"page"`
	Size     int    `json:"size"`
	Next     string `json:"next"`
	Previous string `json:"previous"`
	Values   []T    `json:"values"`
}

// errorBody is the envelope every error from the demo server uses. Fields is
// empty everywhere except on a write rejected for a named member of its
// document.
type errorBody struct {
	Type  string `json:"type"`
	Error struct {
		Message string              `json:"message"`
		Fields  map[string][]string `json:"fields"`
	} `json:"error"`
}

// newTestServer starts a demo server that is closed when the test ends.
func newTestServer(t *testing.T) *Server {
	t.Helper()

	server, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := server.Close(); closeErr != nil {
			t.Errorf("Close: %v", closeErr)
		}
	})
	return server
}

// result is a finished HTTP exchange. The helper reads and closes the body
// before returning, so nothing downstream owns an open response.
type result struct {
	Status int
	Header http.Header
	Body   []byte
}

// get issues a GET against an absolute URL and returns the whole exchange.
func get(t *testing.T, url string, header http.Header) result {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request %s: %v", url, err)
	}
	for name, values := range header {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body of %s: %v", url, err)
	}
	return result{Status: response.StatusCode, Header: response.Header.Clone(), Body: body}
}

// putJSON issues a PUT carrying body as its JSON document.
func putJSON(t *testing.T, url string, body any, header http.Header) result {
	t.Helper()
	return sendJSON(t, http.MethodPut, url, body, header)
}

// postJSON issues a POST carrying body as its JSON document.
func postJSON(t *testing.T, url string, body any, header http.Header) result {
	t.Helper()
	return sendJSON(t, http.MethodPost, url, body, header)
}

// sendJSON issues a request with a JSON body and returns the whole exchange. A
// nil body sends none at all, and a json.RawMessage is sent verbatim, so a test
// can hand the server a document that does not parse.
func sendJSON(t *testing.T, method, url string, body any, header http.Header) result {
	t.Helper()

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(encodeTestBody(t, body))
	}

	request, err := http.NewRequestWithContext(t.Context(), method, url, reader)
	if err != nil {
		t.Fatalf("build request %s %s: %v", method, url, err)
	}
	request.Header.Set("Content-Type", "application/json")
	for name, values := range header {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer func() { _ = response.Body.Close() }()

	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body of %s %s: %v", method, url, err)
	}
	return result{Status: response.StatusCode, Header: response.Header.Clone(), Body: payload}
}

func encodeTestBody(t *testing.T, body any) []byte {
	t.Helper()

	if raw, ok := body.(json.RawMessage); ok {
		return raw
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encode request body %T: %v", body, err)
	}
	return encoded
}

// getOK fetches a path relative to the server and fails unless it answered 200.
func getOK(t *testing.T, server *Server, path string) []byte {
	t.Helper()

	got := get(t, server.URL+path, nil)
	if got.Status != http.StatusOK {
		t.Fatalf("GET %s = status %d, want 200 (body %s)", path, got.Status, got.Body)
	}
	return got.Body
}

func decodeJSON[T any](t *testing.T, body []byte) T {
	t.Helper()

	var value T
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatalf("decode %T: %v (body %s)", value, err, body)
	}
	return value
}

// decodeRaw decodes an object into its raw members, so a test can assert that a
// key is present rather than merely non-zero.
func decodeRaw(t *testing.T, body []byte) map[string]json.RawMessage {
	t.Helper()
	return decodeJSON[map[string]json.RawMessage](t, body)
}

func pullRequestsPath(slug string) string {
	return pathRepositories + "/" + slug + segmentPullRequests
}

func pullRequestPath(slug string, id int) string {
	return pullRequestsPath(slug) + "/" + strconv.Itoa(id)
}

// mergePath is the merge endpoint of one pull request.
func mergePath(slug string, id int) string {
	return pullRequestPath(slug, id) + segmentMerge
}

// approvePath is the approval endpoint of one pull request, which POST and
// DELETE share.
func approvePath(slug string, id int) string {
	return pullRequestPath(slug, id) + segmentApprove
}

// mergeTaskPath is where a queued merge is polled.
func mergeTaskPath(slug string, id int, task string) string {
	return mergePath(slug, id) + segmentTaskStatus + "/" + task
}

// defaultReviewersPath is the repository's effective default reviewers.
func defaultReviewersPath(slug string) string {
	return pathRepositories + "/" + slug + "/effective-default-reviewers"
}

func pipelinesPath(slug string) string {
	return pathRepositories + "/" + slug + segmentPipelines
}

func pipelinePath(slug string, buildNumber int) string {
	return pipelinesPath(slug) + "/" + strconv.Itoa(buildNumber)
}

// iteratorPage mirrors the envelope of an iterator-paginated collection: no
// size member at all, and an opaque token in the next link.
type iteratorPage[T any] struct {
	PageLen int    `json:"pagelen"`
	Next    string `json:"next"`
	Values  []T    `json:"values"`
}

// getNoRedirect issues a GET that stops at a redirect instead of following it,
// so a test can assert what the Location header actually says.
func getNoRedirect(t *testing.T, url string) result {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request %s: %v", url, err)
	}

	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body of %s: %v", url, err)
	}
	return result{Status: response.StatusCode, Header: response.Header.Clone(), Body: body}
}

// demoPullRequest is the dataset's pull request, which a test needs for the
// links the server rewrote to point at itself.
func demoPullRequest(t *testing.T, server *Server, slug string, id int) bitbucket.PullRequest {
	t.Helper()

	for _, pullRequest := range server.Data.PullRequests[slug] {
		if pullRequest.ID == id {
			return pullRequest
		}
	}
	t.Fatalf("no pull request %s#%d in the dataset", slug, id)
	return bitbucket.PullRequest{}
}
