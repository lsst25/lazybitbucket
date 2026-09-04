package demo

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"testing"
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

// errorBody is the envelope every 404 from the demo server uses.
type errorBody struct {
	Type  string `json:"type"`
	Error struct {
		Message string `json:"message"`
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

func pipelinesPath(slug string) string {
	return pathRepositories + "/" + slug + segmentPipelines
}

func pipelinePath(slug string, buildNumber int) string {
	return pipelinesPath(slug) + "/" + strconv.Itoa(buildNumber)
}
