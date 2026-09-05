package demo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
)

const (
	// listenAddr keeps the demo server on the loopback interface only.
	listenAddr = "127.0.0.1:0"
	// readHeaderTimeout guards the server against a client that opens a
	// connection and never finishes the request line (gosec G112).
	readHeaderTimeout = 10 * time.Second

	// apiPrefix is accepted but not required: BaseURL is the server URL, so the
	// client appends "/repositories/…" directly. A caller that points BaseURL at
	// "<url>/2.0" still works.
	apiPrefix = "/2.0"

	defaultPageLen = 10
	maxPageLen     = 100

	contentTypeJSON = "application/json"
	contentTypeText = "text/plain; charset=utf-8"

	// zeroTimestamp is how encoding/json renders a zero time.Time. Bitbucket
	// omits such fields entirely (completed_on on a running pipeline,
	// participated_on for a reviewer who has not acted), so the encoder strips
	// every field carrying this value.
	zeroTimestamp = "0001-01-01T00:00:00Z"
)

// Server is an in-process fake of the slice of the Bitbucket Cloud REST API
// that lazybitbucket uses. Point a bitbucket.Client at Server.URL and every
// call in ClientInterface resolves against Data.
//
// It performs no authentication, on the write routes no more than on the read
// ones: any Authorization header it sees is recorded (LastAuthorization) so
// tests can assert the client sent the right one, and nothing is ever rejected
// for the lack of one.
type Server struct {
	// URL is the origin to use as the client's BaseURL, e.g. "http://127.0.0.1:53412".
	URL string
	// Data is the dataset the routes serve. The write routes change it in
	// place, so a caller that reads it while requests are in flight has to hold
	// dataMu — mutating it between requests, with nothing in flight, stays
	// allowed.
	Data *Dataset

	server *http.Server

	// dataMu guards Data and mergeTasks: the merge and update routes write what
	// every other route reads. It is separate from mu, which guards the two
	// pieces of server state that have nothing to do with the dataset, so the
	// two locks never nest.
	dataMu     sync.RWMutex
	mergeTasks map[string]*mergeTask

	mu         sync.Mutex
	lastAuth   string
	pageLenCap int
}

// NewServer starts a demo server on a random loopback port, serving a fresh
// NewDataset. Call Close when done.
func NewServer() (*Server, error) {
	return NewServerWithDataset(NewDataset())
}

// NewServerWithDataset starts a demo server on a random loopback port serving
// the given dataset, so a caller can adjust the world first — shifting its
// timestamps towards now, for instance. Call Close when done.
func NewServerWithDataset(data *Dataset) (*Server, error) {
	var config net.ListenConfig
	listener, err := config.Listen(context.Background(), "tcp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", listenAddr, err)
	}

	server := &Server{
		URL:        "http://" + listener.Addr().String(),
		Data:       data,
		server:     nil,
		dataMu:     sync.RWMutex{},
		mergeTasks: map[string]*mergeTask{},
		mu:         sync.Mutex{},
		lastAuth:   "",
		pageLenCap: 0,
	}
	server.server = &http.Server{Handler: server.handler(), ReadHeaderTimeout: readHeaderTimeout}

	if err := rewriteDiffLinks(data, server.URL); err != nil {
		_ = listener.Close()
		return nil, err
	}

	go func() { _ = server.server.Serve(listener) }()

	return server, nil
}

// rewriteDiffLinks points every pull request's diff and diffstat links at this
// server, at exactly the redirect targets its own pull request endpoints send
// callers to.
//
// The client prefers PullRequest.Links.Diff over the /pullrequests/{id}/diff
// path whenever the link is same-origin, so leaving the dataset's
// api.bitbucket.org links in place would mean that shortcut — the one every
// real diff request takes — is never exercised in demo mode or in these tests.
func rewriteDiffLinks(data *Dataset, serverURL string) error {
	base, err := url.Parse(serverURL)
	if err != nil {
		return fmt.Errorf("parse demo server URL %q: %w", serverURL, err)
	}

	for slug, pullRequests := range data.PullRequests {
		for index := range pullRequests {
			pullRequest := &pullRequests[index]
			pullRequest.Links.Diff = revspecLink(base, slug, resourceDiff, *pullRequest)
			pullRequest.Links.DiffStat = revspecLink(base, slug, resourceDiffStat, *pullRequest)
		}
	}
	return nil
}

// revspecLink is the absolute URL of one repository-level diff resource on this
// server, with the parameters the redirect would have added already in place.
func revspecLink(base *url.URL, slug, resource string, pullRequest bitbucket.PullRequest) bitbucket.Link {
	query := url.Values{}
	query.Set(queryFromPullRequest, strconv.Itoa(pullRequest.ID))
	query.Set(queryTopic, "true")

	target := url.URL{
		Scheme:   base.Scheme,
		Host:     base.Host,
		Path:     repositoryResourcePath(slug, resource, revspec(pullRequest, slug)),
		RawQuery: query.Encode(),
	}
	return bitbucket.Link{Href: target.String()}
}

// SetMaxPageLen lowers the number of items the server is willing to put in one
// page below the API's own ceiling, so a test can make a listing span several
// pages and exercise the client's "next" following. A value of zero or less
// restores the default ceiling.
func (s *Server) SetMaxPageLen(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pageLenCap = n
}

// maxPageLen is the current per-page ceiling.
func (s *Server) maxPageLen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pageLenCap > 0 {
		return s.pageLenCap
	}
	return maxPageLen
}

// Close shuts the server down and releases its port.
func (s *Server) Close() error {
	if err := s.server.Close(); err != nil {
		return fmt.Errorf("close demo server: %w", err)
	}
	return nil
}

// LastAuthorization is the Authorization header of the most recent request that
// carried one, or "" when no request has. Safe to call while requests are in
// flight.
func (s *Server) LastAuthorization() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastAuth
}

func (s *Server) recordAuthorization(r *http.Request) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return
	}
	s.mu.Lock()
	s.lastAuth = header
	s.mu.Unlock()
}

// handler wires the routes and wraps them with the Authorization recorder and
// the optional "/2.0" prefix strip.
func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /user", s.handleCurrentUser)
	mux.HandleFunc("GET /workspaces/{ws}", s.handleWorkspace)
	mux.HandleFunc("GET /repositories/{ws}", s.handleRepositories)
	mux.HandleFunc("GET /repositories/{ws}/{slug}", s.handleRepository)
	mux.HandleFunc("GET /repositories/{ws}/{slug}/pullrequests", s.handlePullRequests)
	mux.HandleFunc("GET /repositories/{ws}/{slug}/pullrequests/{id}", s.handlePullRequest)
	mux.HandleFunc("GET /repositories/{ws}/{slug}/pullrequests/{id}/statuses", s.handleStatuses)
	mux.HandleFunc("GET /repositories/{ws}/{slug}/pullrequests/{id}/commits", s.handlePullRequestCommits)
	mux.HandleFunc("GET /repositories/{ws}/{slug}/pullrequests/{id}/comments", s.handlePullRequestComments)
	mux.HandleFunc("GET /repositories/{ws}/{slug}/pullrequests/{id}/diffstat", s.handlePullRequestDiffStat)
	mux.HandleFunc("GET /repositories/{ws}/{slug}/pullrequests/{id}/diff", s.handlePullRequestDiff)
	// The write routes, plus the two listings the reviewer picker feeds on.
	mux.HandleFunc("PUT /repositories/{ws}/{slug}/pullrequests/{id}", s.handleUpdatePullRequest)
	mux.HandleFunc("POST /repositories/{ws}/{slug}/pullrequests/{id}/merge", s.handleMergePullRequest)
	mux.HandleFunc("GET /repositories/{ws}/{slug}/pullrequests/{id}/merge/task-status/{task}",
		s.handleMergeTaskStatus)
	mux.HandleFunc("GET /repositories/{ws}/{slug}/effective-default-reviewers",
		s.handleEffectiveDefaultReviewers)
	mux.HandleFunc("GET /workspaces/{ws}/members", s.handleWorkspaceMembers)
	// The redirect targets. A revspec holds a slash and a %0D, so the wildcard
	// has to swallow the rest of the path.
	mux.HandleFunc("GET /repositories/{ws}/{slug}/diffstat/{spec...}", s.handleRepositoryDiffStat)
	mux.HandleFunc("GET /repositories/{ws}/{slug}/diff/{spec...}", s.handleRepositoryDiff)
	mux.HandleFunc("GET /repositories/{ws}/{slug}/pipelines", s.handlePipelines)
	mux.HandleFunc("GET /repositories/{ws}/{slug}/pipelines/{ref}", s.handlePipeline)
	mux.HandleFunc("GET /repositories/{ws}/{slug}/pipelines/{ref}/steps", s.handlePipelineSteps)
	mux.HandleFunc("GET /repositories/{ws}/{slug}/pipelines/{ref}/steps/{step}/log", s.handleStepLog)
	mux.HandleFunc("/", handleUnknown)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.recordAuthorization(r)
		mux.ServeHTTP(w, stripAPIPrefix(r))
	})
}

// stripAPIPrefix removes a leading "/2.0" so the same mux answers both
// "<url>/user" and "<url>/2.0/user".
func stripAPIPrefix(r *http.Request) *http.Request {
	path := r.URL.Path
	if path != apiPrefix && !strings.HasPrefix(path, apiPrefix+"/") {
		return r
	}

	stripped := r.Clone(r.Context())
	stripped.URL.Path = strings.TrimPrefix(path, apiPrefix)
	if stripped.URL.Path == "" {
		stripped.URL.Path = "/"
	}
	if stripped.URL.RawPath != "" {
		stripped.URL.RawPath = strings.TrimPrefix(stripped.URL.RawPath, apiPrefix)
	}
	return stripped
}

// -----------------------------------------------------------------------------
// Handlers
// -----------------------------------------------------------------------------

// Every handler that touches Data holds dataMu for as long as it needs it,
// writing included: a pull request's reviewers and participants are slices the
// dataset and its copies share, so a read that only copied the struct would
// still race the merge and update routes.

func (s *Server) handleCurrentUser(w http.ResponseWriter, _ *http.Request) {
	s.dataMu.RLock()
	defer s.dataMu.RUnlock()

	writeJSON(w, s.Data.User)
}

func (s *Server) handleWorkspace(w http.ResponseWriter, r *http.Request) {
	s.dataMu.RLock()
	defer s.dataMu.RUnlock()

	if r.PathValue("ws") != Workspace {
		writeNotFound(w, "No workspace with identifier '"+r.PathValue("ws")+"'.")
		return
	}
	writeJSON(w, s.Data.Workspace)
}

func (s *Server) handleRepositories(w http.ResponseWriter, r *http.Request) {
	s.dataMu.RLock()
	defer s.dataMu.RUnlock()

	if r.PathValue("ws") != Workspace {
		writeNotFound(w, "No workspace with identifier '"+r.PathValue("ws")+"'.")
		return
	}

	repositories := make([]bitbucket.Repository, len(s.Data.Repositories))
	copy(repositories, s.Data.Repositories)
	sortByTime(repositories, ascending(r), func(repository bitbucket.Repository) time.Time {
		return repository.UpdatedOn
	})
	writeList(w, r, repositories, s.maxPageLen())
}

func (s *Server) handleRepository(w http.ResponseWriter, r *http.Request) {
	s.dataMu.RLock()
	defer s.dataMu.RUnlock()

	repository, ok := s.findRepository(r)
	if !ok {
		writeRepositoryNotFound(w, r)
		return
	}
	writeJSON(w, repository)
}

func (s *Server) handlePullRequests(w http.ResponseWriter, r *http.Request) {
	s.dataMu.RLock()
	defer s.dataMu.RUnlock()

	if _, ok := s.findRepository(r); !ok {
		writeRepositoryNotFound(w, r)
		return
	}

	wanted := requestedStates(r)
	all := s.Data.PullRequests[r.PathValue("slug")]
	matching := make([]bitbucket.PullRequest, 0, len(all))
	for _, pullRequest := range all {
		if _, ok := wanted[pullRequest.State]; ok {
			matching = append(matching, pullRequest)
		}
	}
	sortByTime(matching, ascending(r), func(pullRequest bitbucket.PullRequest) time.Time {
		return pullRequest.UpdatedOn
	})
	writeList(w, r, matching, s.maxPageLen())
}

func (s *Server) handlePullRequest(w http.ResponseWriter, r *http.Request) {
	s.dataMu.RLock()
	defer s.dataMu.RUnlock()

	pullRequest, ok := s.findPullRequest(r)
	if !ok {
		writePullRequestNotFound(w, r)
		return
	}
	writeJSON(w, pullRequest)
}

func (s *Server) handleStatuses(w http.ResponseWriter, r *http.Request) {
	s.dataMu.RLock()
	defer s.dataMu.RUnlock()

	pullRequest, ok := s.findPullRequest(r)
	if !ok {
		writePullRequestNotFound(w, r)
		return
	}

	key := bitbucket.PullRequestKey(Workspace, r.PathValue("slug"), pullRequest.ID)
	statuses := s.Data.Statuses[key]
	writeList(w, r, statuses, s.maxPageLen())
}

func (s *Server) handlePipelines(w http.ResponseWriter, r *http.Request) {
	s.dataMu.RLock()
	defer s.dataMu.RUnlock()

	if _, ok := s.findRepository(r); !ok {
		writeRepositoryNotFound(w, r)
		return
	}

	branch := r.URL.Query().Get("target.branch")
	all := s.Data.Pipelines[r.PathValue("slug")]
	matching := make([]bitbucket.Pipeline, 0, len(all))
	for _, pipeline := range all {
		if branch != "" && pipeline.Target.BranchName() != branch {
			continue
		}
		matching = append(matching, pipeline)
	}
	sortByTime(matching, ascending(r), func(pipeline bitbucket.Pipeline) time.Time {
		return pipeline.CreatedOn
	})
	writeList(w, r, matching, s.maxPageLen())
}

func (s *Server) handlePipeline(w http.ResponseWriter, r *http.Request) {
	s.dataMu.RLock()
	defer s.dataMu.RUnlock()

	pipeline, ok := s.findPipeline(r)
	if !ok {
		writePipelineNotFound(w, r)
		return
	}
	writeJSON(w, pipeline)
}

func (s *Server) handlePipelineSteps(w http.ResponseWriter, r *http.Request) {
	s.dataMu.RLock()
	defer s.dataMu.RUnlock()

	pipeline, ok := s.findPipeline(r)
	if !ok {
		writePipelineNotFound(w, r)
		return
	}
	writeList(w, r, s.Data.Steps[pipeline.UUID], s.maxPageLen())
}

func (s *Server) handleStepLog(w http.ResponseWriter, r *http.Request) {
	s.dataMu.RLock()
	defer s.dataMu.RUnlock()

	pipeline, ok := s.findPipeline(r)
	if !ok {
		writePipelineNotFound(w, r)
		return
	}

	step := r.PathValue("step")
	found := false
	for _, candidate := range s.Data.Steps[pipeline.UUID] {
		if sameUUID(candidate.UUID, step) {
			step = candidate.UUID
			found = true
			break
		}
	}
	log, hasLog := s.Data.Logs[step]
	if !found || !hasLog {
		// A step that never ran has no log, and the real API answers 404 for it.
		writeNotFound(w, "No log available for step "+r.PathValue("step")+".")
		return
	}

	writeLog(w, r, log)
}

func handleUnknown(w http.ResponseWriter, r *http.Request) {
	writeNotFound(w, "Resource not found: "+r.Method+" "+r.URL.Path)
}

// -----------------------------------------------------------------------------
// Lookups
// -----------------------------------------------------------------------------

func (s *Server) findRepository(r *http.Request) (bitbucket.Repository, bool) {
	if r.PathValue("ws") != Workspace {
		return bitbucket.Repository{}, false
	}
	slug := r.PathValue("slug")
	for _, repository := range s.Data.Repositories {
		if repository.Slug == slug {
			return repository, true
		}
	}
	return bitbucket.Repository{}, false
}

func (s *Server) findPullRequest(r *http.Request) (bitbucket.PullRequest, bool) {
	if _, ok := s.findRepository(r); !ok {
		return bitbucket.PullRequest{}, false
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		return bitbucket.PullRequest{}, false
	}
	for _, pullRequest := range s.Data.PullRequests[r.PathValue("slug")] {
		if pullRequest.ID == id {
			return pullRequest, true
		}
	}
	return bitbucket.PullRequest{}, false
}

// pullRequestIndex resolves the request's pull request to where it lives in the
// dataset, so a write route can change it in place instead of a copy of it.
// findPullRequest stays the answer for a read. The caller must hold dataMu, and
// must not append to the slice while it holds the index.
func (s *Server) pullRequestIndex(r *http.Request) (string, int, bool) {
	if _, ok := s.findRepository(r); !ok {
		return "", 0, false
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		return "", 0, false
	}

	slug := r.PathValue("slug")
	for index, pullRequest := range s.Data.PullRequests[slug] {
		if pullRequest.ID == id {
			return slug, index, true
		}
	}
	return "", 0, false
}

// findPipeline resolves the {ref} path segment, which Bitbucket accepts as
// either a braced UUID or a build number. ServeMux has already URL-decoded it,
// so "%7Bc0ffee00-…%7D" arrives with its braces.
func (s *Server) findPipeline(r *http.Request) (bitbucket.Pipeline, bool) {
	if _, ok := s.findRepository(r); !ok {
		return bitbucket.Pipeline{}, false
	}

	ref := r.PathValue("ref")
	pipelines := s.Data.Pipelines[r.PathValue("slug")]
	if buildNumber, err := strconv.Atoi(ref); err == nil {
		for _, pipeline := range pipelines {
			if pipeline.BuildNumber == buildNumber {
				return pipeline, true
			}
		}
		return bitbucket.Pipeline{}, false
	}

	for _, pipeline := range pipelines {
		if sameUUID(pipeline.UUID, ref) {
			return pipeline, true
		}
	}
	return bitbucket.Pipeline{}, false
}

// sameUUID compares two UUIDs ignoring braces and case, so a caller may pass
// either "{c0ffee00-…}" or "c0ffee00-…".
func sameUUID(a, b string) bool {
	return strings.EqualFold(strings.Trim(a, "{}"), strings.Trim(b, "{}"))
}

// -----------------------------------------------------------------------------
// Query parameters
// -----------------------------------------------------------------------------

// requestedStates is the set from the repeated state= parameter. The API
// defaults to OPEN when none is given.
func requestedStates(r *http.Request) map[bitbucket.PullRequestState]struct{} {
	raw := r.URL.Query()["state"]
	if len(raw) == 0 {
		return map[bitbucket.PullRequestState]struct{}{bitbucket.PullRequestOpen: {}}
	}
	states := make(map[bitbucket.PullRequestState]struct{}, len(raw))
	for _, value := range raw {
		states[bitbucket.PullRequestState(strings.ToUpper(value))] = struct{}{}
	}
	return states
}

// ascending reports whether the sort parameter asks for ascending order.
// Bitbucket spells descending with a leading "-", and every caller here defaults
// to descending (newest first).
func ascending(r *http.Request) bool {
	sort := r.URL.Query().Get("sort")
	return sort != "" && !strings.HasPrefix(sort, "-")
}

func sortByTime[T any](items []T, asc bool, key func(T) time.Time) {
	slices.SortStableFunc(items, func(a, b T) int {
		if asc {
			return key(a).Compare(key(b))
		}
		return key(b).Compare(key(a))
	})
}

func positiveInt(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

// -----------------------------------------------------------------------------
// Responses
// -----------------------------------------------------------------------------

// listEnvelope is Bitbucket's paginated collection wrapper. size is present on
// every list this server serves, which the real API only guarantees for cheap
// collections.
type listEnvelope struct {
	PageLen  int               `json:"pagelen"`
	Page     int               `json:"page"`
	Size     int               `json:"size"`
	Next     string            `json:"next,omitempty"`
	Previous string            `json:"previous,omitempty"`
	Values   []json.RawMessage `json:"values"`
}

// errorMessage is the error member of Bitbucket's envelope. Fields carries the
// per-input validation messages a write can be rejected with — one entry per
// offending member of the request document — and is absent everywhere else.
type errorMessage struct {
	Message string              `json:"message"`
	Fields  map[string][]string `json:"fields,omitempty"`
}

type errorEnvelope struct {
	Type  string       `json:"type"`
	Error errorMessage `json:"error"`
}

func writeList[T any](w http.ResponseWriter, r *http.Request, items []T, ceiling int) {
	values, err := encodeAll(items)
	if err != nil {
		writeServerError(w, err)
		return
	}

	pageLen := min(positiveInt(r.URL.Query().Get("pagelen"), defaultPageLen), ceiling)
	page := positiveInt(r.URL.Query().Get("page"), 1)

	size := len(values)
	start := min((page-1)*pageLen, size)
	end := min(start+pageLen, size)

	envelope := listEnvelope{
		PageLen:  pageLen,
		Page:     page,
		Size:     size,
		Next:     "",
		Previous: "",
		Values:   values[start:end],
	}
	if end < size {
		envelope.Next = pageURL(r, strconv.Itoa(page+1))
	}
	if page > 1 && start > 0 {
		envelope.Previous = pageURL(r, strconv.Itoa(page-1))
	}

	writeJSON(w, envelope)
}

// pageURL builds an absolute, same-origin pagination link. The client refuses to
// follow a next link whose host differs from its base URL, so the link must be
// built from the request's own Host. page is a number for the page-based
// collections and an opaque token for the iterator-paginated ones.
func pageURL(r *http.Request, page string) string {
	query := r.URL.Query()
	query.Set("page", page)
	return "http://" + r.Host + r.URL.EscapedPath() + "?" + query.Encode()
}

// writeLog serves a step log as text/plain, honouring a suffix Range header
// ("bytes=-N") with a 206 the way the real log endpoint does.
func writeLog(w http.ResponseWriter, r *http.Request, log string) {
	body := []byte(log)

	start, ok := suffixRange(r.Header.Get("Range"), len(body))
	w.Header().Set("Content-Type", contentTypeText)
	if !ok {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
		return
	}

	tail := body[start:]
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(body)-1, len(body)))
	w.Header().Set("Content-Length", strconv.Itoa(len(tail)))
	w.WriteHeader(http.StatusPartialContent)
	_, _ = w.Write(tail)
}

// suffixRange parses "bytes=-N" and returns the offset the tail starts at. Any
// other range spelling is ignored, and so is a request for zero bytes.
func suffixRange(header string, size int) (int, bool) {
	const prefix = "bytes=-"
	if size == 0 || !strings.HasPrefix(header, prefix) {
		return 0, false
	}
	count, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(header, prefix)))
	if err != nil || count <= 0 {
		return 0, false
	}
	if count >= size {
		return 0, true
	}
	return size - count, true
}

// writeJSON answers 200 with value as the body.
func writeJSON(w http.ResponseWriter, value any) {
	writeJSONStatus(w, http.StatusOK, value)
}

// writeJSONStatus answers with value as the body at the given status, which is
// what a route answering anything but a plain 200 needs — the error envelopes
// and, were it to carry a document, the merge 202.
func writeJSONStatus(w http.ResponseWriter, status int, value any) {
	body, err := encodeValue(value)
	if err != nil {
		writeServerError(w, err)
		return
	}
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func writeNotFound(w http.ResponseWriter, message string) {
	writeErrorEnvelope(w, http.StatusNotFound, message)
}

// writeErrorEnvelope answers with Bitbucket's error body at the given status.
func writeErrorEnvelope(w http.ResponseWriter, status int, message string) {
	writeJSONStatus(w, status, errorEnvelope{
		Type:  "error",
		Error: errorMessage{Message: message, Fields: nil},
	})
}

func writeRepositoryNotFound(w http.ResponseWriter, r *http.Request) {
	writeNotFound(w, "Repository "+r.PathValue("ws")+"/"+r.PathValue("slug")+" not found.")
}

func writePullRequestNotFound(w http.ResponseWriter, r *http.Request) {
	writeNotFound(w, "Pull request not found: "+
		r.PathValue("ws")+"/"+r.PathValue("slug")+"#"+r.PathValue("id"))
}

func writePipelineNotFound(w http.ResponseWriter, r *http.Request) {
	writeNotFound(w, "No pipeline "+r.PathValue("ref")+" in "+
		r.PathValue("ws")+"/"+r.PathValue("slug")+".")
}

func writeServerError(w http.ResponseWriter, cause error) {
	body, err := json.Marshal(errorEnvelope{
		Type:  "error",
		Error: errorMessage{Message: cause.Error(), Fields: nil},
	})
	if err != nil {
		body = []byte(`{"type":"error","error":{"message":"internal demo server error"}}`)
	}
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write(body)
}

// -----------------------------------------------------------------------------
// Encoding
// -----------------------------------------------------------------------------

func encodeAll[T any](items []T) ([]json.RawMessage, error) {
	values := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		raw, err := encodeValue(item)
		if err != nil {
			return nil, err
		}
		values = append(values, raw)
	}
	return values, nil
}

// encodeValue marshals a model struct and then fixes the one place where
// encoding/json cannot produce Bitbucket's wire shape on its own: absent
// timestamps. See pruneZeroTimestamps.
func encodeValue(value any) (json.RawMessage, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal %T: %w", value, err)
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber() // keep integers exact instead of round-tripping via float64
	var generic any
	if err := decoder.Decode(&generic); err != nil {
		return nil, fmt.Errorf("decode %T: %w", value, err)
	}

	pruned, err := json.Marshal(pruneZeroTimestamps(generic))
	if err != nil {
		return nil, fmt.Errorf("re-marshal %T: %w", value, err)
	}
	return pruned, nil
}

// pruneZeroTimestamps drops every field holding a zero timestamp, because a
// time.Time cannot express "absent": a running pipeline carries no completed_on,
// a step that never started carries neither started_on nor completed_on, and a
// reviewer who has not acted carries no participated_on.
//
// Key names need no fixing up: every field of every model struct carries a
// lowercase json tag, the anonymous link structs included, so encoding/json
// never falls back to a Go field name here (TestWireKeysAreLowercase guards it).
func pruneZeroTimestamps(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		pruned := make(map[string]any, len(typed))
		for key, member := range typed {
			if text, ok := member.(string); ok && text == zeroTimestamp {
				continue
			}
			pruned[key] = pruneZeroTimestamps(member)
		}
		return pruned
	case []any:
		for index, member := range typed {
			typed[index] = pruneZeroTimestamps(member)
		}
		return typed
	default:
		return value
	}
}
