package bitbucket

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"sync"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// bearerOpts is the token-only credential shape: Authorization: Bearer <token>.
func bearerOpts() ClientOpts {
	return ClientOpts{BaseURL: "", Token: testToken, Email: "", UserAgent: "", HTTPClient: nil}
}

// basicOpts is the email+token credential shape: Basic base64(email:token).
func basicOpts() ClientOpts {
	return ClientOpts{BaseURL: "", Token: testToken, Email: testEmail, UserAgent: "", HTTPClient: nil}
}

func newRecordingClient(
	t *testing.T, opts ClientOpts, response testkit.StubResponse,
) (*Client, *testkit.RecordedRequest) {
	t.Helper()

	server, recorded := testkit.RecordingServer(t, response)
	opts.BaseURL = server.URL
	return NewClient(opts), recorded
}

func newSequenceClient(
	t *testing.T, opts ClientOpts, responses ...testkit.StubResponse,
) (*Client, *[]testkit.RecordedRequest) {
	t.Helper()

	server, recorded := testkit.RecordingSequenceServer(t, responses)
	opts.BaseURL = server.URL
	return NewClient(opts), recorded
}

// newHandlerClient points a bearer client at an arbitrary handler, for the
// cases testkit's stub servers cannot express: response headers, a body that
// depends on the request, or a deliberately hostile pagination link.
func newHandlerClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	opts := bearerOpts()
	opts.BaseURL = server.URL
	return NewClient(opts)
}

// pagedServer serves pages in order, injecting a "next" link into every page
// but the last. The link is built from the incoming request's own host, so it
// is absolute and same-origin — which the client insists on, and which
// testkit.RecordingSequenceServer cannot produce because a stub body has to be
// written before the server knows its own URL.
func pagedServer(t *testing.T, pages []string) (*Client, *[]testkit.RecordedRequest) {
	t.Helper()

	envelopes := make([]map[string]json.RawMessage, 0, len(pages))
	for _, body := range pages {
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal([]byte(body), &envelope); err != nil {
			t.Fatalf("unmarshal page fixture: %v", err)
		}
		envelopes = append(envelopes, envelope)
	}

	var mutex sync.Mutex
	recorded := &[]testkit.RecordedRequest{}

	client := newHandlerClient(t, func(w http.ResponseWriter, r *http.Request) {
		mutex.Lock()
		index := len(*recorded)
		*recorded = append(*recorded, testkit.RecordedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.Query(),
			Header: r.Header.Clone(),
			Body:   nil,
		})
		mutex.Unlock()

		if index >= len(envelopes) {
			t.Errorf("unexpected request %d for %s", index+1, r.URL)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		envelope := maps.Clone(envelopes[index])
		if index < len(envelopes)-1 {
			next := "http://" + r.Host + r.URL.Path + "?page=" + strconv.Itoa(index+2)
			link, err := json.Marshal(next)
			if err != nil {
				t.Errorf("marshal next link: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			envelope["next"] = link
		}

		body, err := json.Marshal(envelope)
		if err != nil {
			t.Errorf("marshal envelope: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", contentTypeJSON)
		_, _ = w.Write(body)
	})

	return client, recorded
}

// writeJSON answers a handler-built request with a fixed body.
func writeJSON(t *testing.T, w http.ResponseWriter, status int, body string) {
	t.Helper()

	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// selfNextPage marshals a pagination envelope whose next link points back at
// the requesting server: with scheme "http" it is the same-origin link the
// client will follow, with "https" the scheme change it must refuse.
//
// The link is marshalled rather than concatenated into the response so the
// request-derived host never reaches the writer as raw bytes.
func selfNextPage(t *testing.T, r *http.Request, scheme, path string, values []any) []byte {
	t.Helper()

	// nil means one anonymous value rather than none: a page with no values
	// ends the walk before its next link is ever followed, and every caller of
	// this helper is testing what happens when the link is followed.
	if values == nil {
		values = []any{map[string]any{}}
	}
	body, err := json.Marshal(map[string]any{
		"pagelen": 100,
		"values":  values,
		"next":    scheme + "://" + r.Host + path + "?page=2",
	})
	if err != nil {
		t.Errorf("marshal envelope: %v", err)
		return []byte(`{"values":[]}`)
	}
	return body
}

// recorder collects the requests a handler-built client issued. The review
// endpoint tests cannot use testkit's stub servers — they need request-derived
// pagination links, redirects and non-JSON bodies — so they record here and
// assert on the snapshot once the client has returned.
type recorder struct {
	mu       sync.Mutex
	requests []testkit.RecordedRequest
	targets  []string
}

func (rec *recorder) add(r *http.Request) {
	rec.mu.Lock()
	defer rec.mu.Unlock()

	rec.requests = append(rec.requests, testkit.RecordedRequest{
		Method: r.Method,
		Path:   r.URL.Path,
		Query:  r.URL.Query(),
		Header: r.Header.Clone(),
		Body:   nil,
	})
	rec.targets = append(rec.targets, r.URL.RequestURI())
}

// snapshot is the requests recorded so far, safe to read while the server is
// still running.
func (rec *recorder) snapshot() []testkit.RecordedRequest {
	rec.mu.Lock()
	defer rec.mu.Unlock()

	return slices.Clone(rec.requests)
}

// requestTargets is the path-plus-query of every recorded request, still
// escaped exactly as it went on the wire — testkit.RecordedRequest keeps the
// query decoded, and the diff redirect asserts on the escaping itself.
func (rec *recorder) requestTargets() []string {
	rec.mu.Lock()
	defer rec.mu.Unlock()

	return slices.Clone(rec.targets)
}

// writeText answers with a text/plain body, the way the diff endpoints do.
func writeText(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", contentTypeText)
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// errorEnvelope is Bitbucket's standard error body, as the 555 responses carry
// it.
func errorEnvelope(message string) string {
	return `{"type": "error", "error": {"message": ` + strconv.Quote(message) + `}}`
}
