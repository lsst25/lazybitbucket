package bitbucket

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Defaults applied by NewClient when ClientOpts leaves a field empty.
const (
	defaultUserAgent = "lazybitbucket"
	defaultTimeout   = 30 * time.Second
)

// Media types the client asks for. JSON everywhere except the pipeline step
// log, which is a raw text blob.
const (
	acceptJSON = "application/json"
	acceptText = "text/plain"
)

// rateLimitRemainingHeader carries how many requests are left in the current
// hour. Bitbucket does not send it on every response.
const rateLimitRemainingHeader = "X-RateLimit-Remaining"

// rateLimitUnknown is RequestLog.RateLimitRemaining when the header was absent
// or unparsable.
const rateLimitUnknown = -1

// Per-endpoint pagelen ceilings. The pull request list caps at 50 and answers
// 400 "Invalid pagelen" above it, which the generic 100 would trip; the
// pipelines list declares a default of 10 and states no maximum, so the
// documented default is used as the ceiling.
const (
	maxPageLenDefault      = 100
	maxPageLenPullRequests = 50
	maxPageLenPipelines    = 10
)

// Default MaxItems per list option struct, applied when the caller leaves it at
// zero. They bound a first render: a repository with 6,000 pull requests must
// not stall the TUI behind hundreds of pages.
const (
	defaultMaxRepositories = 100
	defaultMaxPullRequests = 50
	defaultMaxPipelines    = 10
)

// defaultPullRequestFields pulls the review data into list items that the
// collection endpoint omits for performance. The "+" prefixes are additive, so
// every default field is still returned; url.Values.Encode escapes them as
// %2B, which is mandatory (a bare "+" decodes as a space and matches nothing).
const defaultPullRequestFields = "+values.reviewers,+values.participants"

// noFields is the ListPullRequestsOptions.Fields sentinel that suppresses the
// fields parameter entirely.
const noFields = "-"

// Client is a read-only Bitbucket Cloud REST API v2 client. It issues nothing
// but GET requests, holds no workspace binding, and is safe for concurrent use
// once constructed — SetLogger and SetOnRequest must be called before the first
// request.
type Client struct {
	baseURL    string
	authHeader string
	authMethod AuthMethod
	userAgent  string
	httpClient *http.Client
	logger     io.Writer
	onRequest  func(RequestLog)
}

// ClientOpts configures a Client. The two supported credential shapes are a
// bare token (Bearer — API tokens and access tokens) and an Atlassian account
// email plus API token (Basic).
type ClientOpts struct {
	// BaseURL overrides the API root; "" means DefaultBaseURL. A trailing "/"
	// is trimmed. Tests inject an httptest URL here.
	BaseURL string
	// Token is the secret: an API token or an access token.
	Token string
	// Email switches the client to Basic auth when non-empty. It must be the
	// Atlassian account email, not a Bitbucket username.
	Email string
	// UserAgent overrides the User-Agent header; "" means "lazybitbucket".
	UserAgent string
	// HTTPClient overrides the transport; nil means a 30 second timeout.
	HTTPClient *http.Client
}

var _ ClientInterface = (*Client)(nil)

// NewClient builds a Client from opts. It performs no I/O: a bad token is only
// discovered on the first request.
func NewClient(opts ClientOpts) *Client {
	baseURL := strings.TrimRight(opts.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	authHeader, authMethod := authorization(opts.Email, opts.Token)

	userAgent := opts.UserAgent
	if userAgent == "" {
		userAgent = defaultUserAgent
	}

	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}

	return &Client{
		baseURL:    baseURL,
		authHeader: authHeader,
		authMethod: authMethod,
		userAgent:  userAgent,
		httpClient: httpClient,
		logger:     nil,
		onRequest:  nil,
	}
}

// authorization picks the credential shape: an email means Basic (API tokens
// only), a bare token means Bearer, and neither means anonymous access to
// public repositories.
func authorization(email, token string) (string, AuthMethod) {
	switch {
	case email != "":
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(email+":"+token)), AuthBasic
	case token != "":
		return "Bearer " + token, AuthBearer
	default:
		return "", AuthNone
	}
}

// BaseURL returns the API root every request is built against.
func (c *Client) BaseURL() string { return c.baseURL }

// AuthMethod returns the credential shape in use. APIError values carry the
// same value so a 401 can name the right fix.
func (c *Client) AuthMethod() AuthMethod { return c.authMethod }

// AuthHeader returns the Authorization header value, or "" when unauthenticated.
func (c *Client) AuthHeader() string { return c.authHeader }

// HTTPClient returns the underlying HTTP client.
func (c *Client) HTTPClient() *http.Client { return c.httpClient }

// SetLogger sends one request line and one response line per call to w.
func (c *Client) SetLogger(w io.Writer) { c.logger = w }

// SetOnRequest installs a hook called after every completed request, error
// statuses included. It is how the TUI's log panel and rate-limit indicator are
// fed. The hook runs on whichever goroutine issued the request and, because
// FetchPullRequests issues requests concurrently, it may be invoked from
// several goroutines at once: it must be goroutine-safe.
func (c *Client) SetOnRequest(fn func(RequestLog)) { c.onRequest = fn }

func (c *Client) log(format string, args ...any) {
	if c.logger != nil {
		fmt.Fprintf(c.logger, format, args...)
	}
}

func (c *Client) notify(entry RequestLog) {
	if c.onRequest != nil {
		c.onRequest(entry)
	}
}

// request performs one GET and returns the body and status. fullURL is what
// goes on the wire; relPath is the same location relative to BaseURL, used for
// logging, RequestLog.Path and APIError.Path (which drops the query).
//
// Non-2xx responses return the body, the status and an *APIError stamped with
// the client's AuthMethod.
func (c *Client) request(
	ctx context.Context, fullURL, relPath, accept string, extraHeaders http.Header,
) ([]byte, int, error) {
	start := time.Now()
	c.log("%s %s %s\n", start.Format(time.TimeOnly), http.MethodGet, fullURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("create request %s %s: %w", http.MethodGet, relPath, err)
	}
	if c.authHeader != "" {
		req.Header.Set("Authorization", c.authHeader)
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", c.userAgent)
	for name, values := range extraHeaders {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log("  ERROR: %v\n", err)
		return nil, 0, fmt.Errorf("execute request %s %s: %w", http.MethodGet, relPath, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response body for %s %s: %w", http.MethodGet, relPath, err)
	}

	elapsed := time.Since(start)
	c.log("  -> %d (%s) %d bytes\n", resp.StatusCode, elapsed.Round(time.Millisecond), len(body))
	c.notify(RequestLog{
		Method:             http.MethodGet,
		Path:               relPath,
		Status:             resp.StatusCode,
		Elapsed:            elapsed,
		RateLimitRemaining: rateLimitRemaining(resp.Header),
	})

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		c.log("  BODY: %s\n", string(body))
		apiErr := parseAPIError(http.MethodGet, pathWithoutQuery(relPath), resp.StatusCode, resp.Header, body)
		apiErr.AuthMethod = c.authMethod
		return body, resp.StatusCode, apiErr
	}
	return body, resp.StatusCode, nil
}

// getJSON fetches path (with query appended) and unmarshals the body into out.
func (c *Client) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	relPath := joinQuery(path, query)
	body, _, err := c.request(ctx, c.baseURL+relPath, relPath, acceptJSON, nil)
	if err != nil {
		return err
	}
	return unmarshalBody(relPath, body, out)
}

// getJSONURL follows an absolute URL handed back by the API — a pagination
// "next" link, which is opaque and must be used verbatim rather than rebuilt.
//
// It refuses a link whose scheme or host differ from BaseURL, so a compromised
// or misconfigured API cannot redirect the Authorization header to a host of
// its choosing.
func (c *Client) getJSONURL(ctx context.Context, absURL string, out any) error {
	target, err := url.Parse(absURL)
	if err != nil {
		return fmt.Errorf("parse pagination link %q: %w", absURL, err)
	}
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return fmt.Errorf("parse base URL %q: %w", c.baseURL, err)
	}
	if target.Scheme != base.Scheme || target.Host != base.Host {
		return fmt.Errorf("refusing to follow pagination link to %s (expected host %s)", absURL, base.Host)
	}

	relPath := strings.TrimPrefix(absURL, c.baseURL)
	body, _, err := c.request(ctx, absURL, relPath, acceptJSON, nil)
	if err != nil {
		return err
	}
	return unmarshalBody(relPath, body, out)
}

// getRaw fetches a non-JSON resource — the pipeline step log, which is a text
// blob served with an optional Range. query is a pre-encoded query string, "" for
// none. Redirects use the default http.Client policy on purpose: a completed
// step's log 307s to long-term storage on another host, and Go drops the
// Authorization header across hosts, which is exactly right for a presigned URL.
func (c *Client) getRaw(
	ctx context.Context, path, query, accept string, extraHeaders http.Header,
) ([]byte, int, error) {
	relPath := path
	if query != "" {
		relPath = path + "?" + query
	}
	return c.request(ctx, c.baseURL+relPath, relPath, accept, extraHeaders)
}

func unmarshalBody(relPath string, body []byte, out any) error {
	if out == nil || len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode response for %s %s: %w", http.MethodGet, pathWithoutQuery(relPath), err)
	}
	return nil
}

func joinQuery(path string, query url.Values) string {
	encoded := query.Encode()
	if encoded == "" {
		return path
	}
	return path + "?" + encoded
}

func pathWithoutQuery(relPath string) string {
	path, _, _ := strings.Cut(relPath, "?")
	return path
}

func rateLimitRemaining(header http.Header) int {
	raw := header.Get(rateLimitRemainingHeader)
	if raw == "" {
		return rateLimitUnknown
	}
	remaining, err := strconv.Atoi(raw)
	if err != nil {
		return rateLimitUnknown
	}
	return remaining
}

// page is the pagination envelope every collection endpoint wraps its results
// in. Only values and next are guaranteed: size and previous are dropped where
// computing them is expensive, so Size is a pointer to keep "absent" apart from
// zero.
//
// The "page" member is deliberately not modelled. Iterator-based collections
// put an opaque token there ("E8Ux"), so an int field would fail to unmarshal
// on the second page — and following next makes it redundant anyway.
type page[T any] struct {
	Size     *int   `json:"size"`
	PageLen  int    `json:"pagelen"`
	Next     string `json:"next"`
	Previous string `json:"previous"`
	Values   []T    `json:"values"`
}

// getAll fetches path?query and then follows next until the collection is
// exhausted or maxItems values have been collected, truncating a final page
// that overshoots. maxItems <= 0 means "no bound".
func getAll[T any](ctx context.Context, client *Client, path string, query url.Values, maxItems int) ([]T, error) {
	values, _, err := getAllCounted[T](ctx, client, path, query, maxItems)
	return values, err
}

// getAllCounted is getAll plus the number of pages it fetched successfully.
// ListPullRequests needs that count to tell a first-request rejection (the one
// carrying fields, which it can retry without them) from a failure while
// following an opaque next link (which it must not rebuild).
func getAllCounted[T any](
	ctx context.Context, client *Client, path string, query url.Values, maxItems int,
) ([]T, int, error) {
	var (
		all      []T
		pages    int
		envelope page[T]
	)

	if err := client.getJSON(ctx, path, query, &envelope); err != nil {
		return nil, pages, err
	}

	for {
		pages++
		all = append(all, envelope.Values...)
		if maxItems > 0 && len(all) >= maxItems {
			return all[:maxItems], pages, nil
		}
		if envelope.Next == "" {
			return all, pages, nil
		}

		next := envelope.Next
		envelope = page[T]{Size: nil, PageLen: 0, Next: "", Previous: "", Values: nil}
		if err := client.getJSONURL(ctx, next, &envelope); err != nil {
			return nil, pages, err
		}
	}
}

// pageLen is the pagelen to request: the endpoint ceiling, lowered to maxItems
// when the caller wants fewer than a full page, so a small discovery costs one
// small response instead of one large one.
func pageLen(endpointMax, maxItems int) int {
	if maxItems > 0 && maxItems < endpointMax {
		return maxItems
	}
	return endpointMax
}

// orDefault returns value when it is positive, else fallback.
func orDefault(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

// orString returns value when it is non-empty, else fallback.
func orString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

// repoPath is the API path prefix for a repository.
func repoPath(workspace, repoSlug string) string {
	return "/repositories/" + url.PathEscape(workspace) + "/" + url.PathEscape(repoSlug)
}
