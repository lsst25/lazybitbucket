package testkit

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
)

type RecordedRequest struct {
	Method string
	Path   string
	Query  url.Values
	Header http.Header
	Body   []byte
}

type StubResponse struct {
	Status int
	Body   string
}

func RecordingServer(t *testing.T, response StubResponse) (*httptest.Server, *RecordedRequest) {
	t.Helper()

	recorded := &RecordedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		recorded.Method = r.Method
		recorded.Path = r.URL.Path
		recorded.Query = r.URL.Query()
		recorded.Header = r.Header.Clone()
		recorded.Body = body

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(response.Status)
		_, _ = io.WriteString(w, response.Body)
	}))
	t.Cleanup(server.Close)

	return server, recorded
}

func RecordingSequenceServer(t *testing.T, responses []StubResponse) (*httptest.Server, *[]RecordedRequest) {
	t.Helper()

	var mutex sync.Mutex
	recorded := &[]RecordedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}

		mutex.Lock()
		index := len(*recorded)
		*recorded = append(*recorded, RecordedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.Query(),
			Header: r.Header.Clone(),
			Body:   body,
		})
		mutex.Unlock()

		response := responses[min(index, len(responses)-1)]
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(response.Status)
		_, _ = io.WriteString(w, response.Body)
	}))
	t.Cleanup(server.Close)

	return server, recorded
}

// NoRequestServer is a server that must never be hit: any request fails the
// test and answers 500. It is how a test proves a guard short-circuited before
// the transport — a read-only client refusing a write, for instance — rather
// than merely proving no assertion tripped.
func NoRequestServer(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s %s: the server should never be reached", r.Method, r.URL.RequestURI())
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	return server
}

func AssertEqual[T comparable](t *testing.T, label string, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
}

func AssertSliceEqual[T comparable](t *testing.T, label string, got, want []T) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
}

func BlankCanvas(width, height int) string {
	row := strings.Repeat(" ", width)
	rows := make([]string, height)
	for i := range rows {
		rows[i] = row
	}
	return strings.Join(rows, "\n")
}
