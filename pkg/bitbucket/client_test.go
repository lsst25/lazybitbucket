package bitbucket

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

const accountFixture = `{
  "type": "user",
  "uuid": "{e5743438-cc31-49a0-96b7-9febd7b946f4}",
  "account_id": "5f1e9a4c0e1b2c0068ab1234",
  "display_name": "Ada Lovelace",
  "nickname": "ada",
  "links": {
    "self":   {"href": "https://api.bitbucket.org/2.0/users/%7Be5743438%7D"},
    "html":   {"href": "https://bitbucket.org/%7Be5743438%7D/"},
    "avatar": {"href": "https://avatar.example/128"}
  }
}`

func TestNewClient_Authorization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		opts       ClientOpts
		wantHeader string
		wantMethod AuthMethod
	}{
		{
			name:       "token only is bearer",
			opts:       bearerOpts(),
			wantHeader: "Bearer " + testToken,
			wantMethod: AuthBearer,
		},
		{
			name:       "email and token is basic",
			opts:       basicOpts(),
			wantHeader: "Basic " + base64.StdEncoding.EncodeToString([]byte(testEmail+":"+testToken)),
			wantMethod: AuthBasic,
		},
		{
			name:       "no credentials is anonymous",
			opts:       ClientOpts{BaseURL: "", Token: "", Email: "", UserAgent: "", HTTPClient: nil},
			wantHeader: "",
			wantMethod: AuthNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := NewClient(tt.opts)
			testkit.AssertEqual(t, "AuthHeader()", client.AuthHeader(), tt.wantHeader)
			testkit.AssertEqual(t, "AuthMethod()", client.AuthMethod(), tt.wantMethod)
		})
	}
}

func TestNewClient_BaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty falls back to the API root", in: "", want: DefaultBaseURL},
		{name: "trailing slash is trimmed", in: "https://api.bitbucket.org/2.0/", want: DefaultBaseURL},
		{name: "repeated slashes are trimmed", in: "http://127.0.0.1:8080//", want: "http://127.0.0.1:8080"},
		{name: "custom base is kept", in: "http://127.0.0.1:8080", want: "http://127.0.0.1:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := bearerOpts()
			opts.BaseURL = tt.in
			testkit.AssertEqual(t, "BaseURL()", NewClient(opts).BaseURL(), tt.want)
		})
	}
}

func TestNewClient_DefaultHTTPClient(t *testing.T) {
	t.Parallel()

	client := NewClient(bearerOpts())
	httpClient := client.HTTPClient()
	if httpClient == nil {
		t.Fatal("HTTPClient() = nil, want a default client")
	}
	testkit.AssertEqual(t, "timeout", httpClient.Timeout, 30*time.Second)
}

func TestNewClient_KeepsInjectedHTTPClient(t *testing.T) {
	t.Parallel()

	injected := &http.Client{Timeout: time.Second}
	opts := bearerOpts()
	opts.HTTPClient = injected

	testkit.AssertEqual(t, "HTTPClient()", NewClient(opts).HTTPClient(), injected)
}

// countingTransport fails every request and counts the attempts.
type countingTransport struct{ calls atomic.Int64 }

func (c *countingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	c.calls.Add(1)
	return nil, errors.New("transport must not be used")
}

func TestNewClient_IssuesNoRequests(t *testing.T) {
	t.Parallel()

	transport := &countingTransport{}
	opts := basicOpts()
	opts.HTTPClient = &http.Client{Transport: transport}

	NewClient(opts)

	testkit.AssertEqual(t, "requests during construction", transport.calls.Load(), int64(0))
}

func TestClient_RequestHeaders(t *testing.T) {
	t.Parallel()

	opts := basicOpts()
	opts.UserAgent = "lazybitbucket/test"
	client, recorded := newRecordingClient(t, opts, testkit.StubResponse{Status: http.StatusOK, Body: accountFixture})

	if _, err := client.GetCurrentUser(t.Context()); err != nil {
		t.Fatalf("GetCurrentUser() error = %v", err)
	}

	testkit.AssertEqual(t, "method", recorded.Method, http.MethodGet)
	testkit.AssertEqual(t, "path", recorded.Path, "/user")
	testkit.AssertEqual(t, "Accept", recorded.Header.Get("Accept"), contentTypeJSON)
	testkit.AssertEqual(t, "User-Agent", recorded.Header.Get("User-Agent"), "lazybitbucket/test")
	testkit.AssertEqual(t, "Authorization", recorded.Header.Get("Authorization"), client.AuthHeader())
}

func TestClient_DefaultUserAgent(t *testing.T) {
	t.Parallel()

	client, recorded := newRecordingClient(t, bearerOpts(),
		testkit.StubResponse{Status: http.StatusOK, Body: accountFixture})

	if _, err := client.GetCurrentUser(t.Context()); err != nil {
		t.Fatalf("GetCurrentUser() error = %v", err)
	}

	testkit.AssertEqual(t, "User-Agent", recorded.Header.Get("User-Agent"), "lazybitbucket")
}

func TestClient_SendsNoAuthorizationWithoutCredentials(t *testing.T) {
	t.Parallel()

	opts := ClientOpts{BaseURL: "", Token: "", Email: "", UserAgent: "", HTTPClient: nil}
	client, recorded := newRecordingClient(t, opts, testkit.StubResponse{Status: http.StatusOK, Body: accountFixture})

	if _, err := client.GetCurrentUser(t.Context()); err != nil {
		t.Fatalf("GetCurrentUser() error = %v", err)
	}

	if _, ok := recorded.Header["Authorization"]; ok {
		t.Errorf("Authorization header present = %q, want absent", recorded.Header.Get("Authorization"))
	}
}

func TestClient_GetCurrentUserParsesAccount(t *testing.T) {
	t.Parallel()

	client, _ := newRecordingClient(t, bearerOpts(), testkit.StubResponse{Status: http.StatusOK, Body: accountFixture})

	account, err := client.GetCurrentUser(t.Context())
	if err != nil {
		t.Fatalf("GetCurrentUser() error = %v", err)
	}

	testkit.AssertEqual(t, "UUID", account.UUID, testAuthorUUID)
	testkit.AssertEqual(t, "Name()", account.Name(), "Ada Lovelace")
}

func TestClient_SetLoggerWritesRequestAndResponseLines(t *testing.T) {
	t.Parallel()

	client, _ := newRecordingClient(t, bearerOpts(), testkit.StubResponse{Status: http.StatusOK, Body: accountFixture})

	var logged strings.Builder
	client.SetLogger(&logged)

	if _, err := client.GetCurrentUser(t.Context()); err != nil {
		t.Fatalf("GetCurrentUser() error = %v", err)
	}

	out := logged.String()
	for _, want := range []string{http.MethodGet, client.BaseURL() + "/user", "-> 200", "bytes"} {
		if !strings.Contains(out, want) {
			t.Errorf("log %q does not contain %q", out, want)
		}
	}
}

func TestClient_SetLoggerRecordsErrorBody(t *testing.T) {
	t.Parallel()

	client, _ := newRecordingClient(t, bearerOpts(),
		testkit.StubResponse{Status: http.StatusNotFound, Body: `{"type":"error","error":{"message":"Not found"}}`})

	var logged strings.Builder
	client.SetLogger(&logged)

	if _, err := client.GetCurrentUser(t.Context()); err == nil {
		t.Fatal("expected error, got nil")
	}

	out := logged.String()
	if !strings.Contains(out, "-> 404") || !strings.Contains(out, "BODY:") {
		t.Errorf("log %q does not report the failing response", out)
	}
}

func TestClient_SetOnRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		status        int
		rateLimit     string
		wantStatus    int
		wantRemaining int
	}{
		{name: "success with rate limit header", status: http.StatusOK, rateLimit: "873", wantStatus: 200, wantRemaining: 873},
		{name: "success without rate limit header", status: http.StatusOK, rateLimit: "", wantStatus: 200, wantRemaining: -1},
		{name: "unparsable rate limit header", status: http.StatusOK, rateLimit: "many", wantStatus: 200, wantRemaining: -1},
		{
			name:          "server error still reports",
			status:        http.StatusInternalServerError,
			rateLimit:     "0",
			wantStatus:    500,
			wantRemaining: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newHandlerClient(t, func(w http.ResponseWriter, _ *http.Request) {
				if tt.rateLimit != "" {
					w.Header().Set("X-RateLimit-Remaining", tt.rateLimit)
				}
				writeJSON(t, w, tt.status, accountFixture)
			})

			var logs []RequestLog
			client.SetOnRequest(func(entry RequestLog) { logs = append(logs, entry) })

			_, _ = client.GetCurrentUser(t.Context())

			if len(logs) != 1 {
				t.Fatalf("RequestLog count = %d, want 1", len(logs))
			}
			testkit.AssertEqual(t, "Method", logs[0].Method, http.MethodGet)
			testkit.AssertEqual(t, "Path", logs[0].Path, "/user")
			testkit.AssertEqual(t, "Status", logs[0].Status, tt.wantStatus)
			testkit.AssertEqual(t, "RateLimitRemaining", logs[0].RateLimitRemaining, tt.wantRemaining)
			if logs[0].Elapsed < 0 {
				t.Errorf("Elapsed = %v, want a non-negative duration", logs[0].Elapsed)
			}
		})
	}
}

func TestClient_RequestLogPathCarriesQuery(t *testing.T) {
	t.Parallel()

	client, _ := newRecordingClient(t, bearerOpts(), testkit.StubResponse{Status: http.StatusOK, Body: emptyPage})

	var logs []RequestLog
	client.SetOnRequest(func(entry RequestLog) { logs = append(logs, entry) })

	if _, err := client.ListPullRequests(t.Context(), testWorkspace, testRepoSlug,
		ListPullRequestsOptions{States: nil, Query: "", Sort: "", MaxItems: 0, Fields: noFields}); err != nil {
		t.Fatalf("ListPullRequests() error = %v", err)
	}

	if len(logs) != 1 {
		t.Fatalf("RequestLog count = %d, want 1", len(logs))
	}
	testkit.AssertEqual(t, "Path", logs[0].Path,
		testPullRequestPath+"?pagelen=50&sort=-updated_on&state=OPEN")
}

func TestClient_UnauthorizedWithEmptyBody(t *testing.T) {
	t.Parallel()

	client, _ := newRecordingClient(t, basicOpts(), testkit.StubResponse{Status: http.StatusUnauthorized, Body: ""})

	_, err := client.GetCurrentUser(t.Context())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !IsUnauthorized(err) {
		t.Fatalf("IsUnauthorized(%v) = false, want true", err)
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v is not an *APIError", err)
	}
	testkit.AssertEqual(t, "Status", apiErr.Status, http.StatusUnauthorized)
	testkit.AssertEqual(t, "Message", apiErr.Message, "Unauthorized")
	testkit.AssertEqual(t, "Path", apiErr.Path, "/user")
	testkit.AssertEqual(t, "AuthMethod", apiErr.AuthMethod, AuthBasic)

	if !strings.Contains(err.Error(), "Atlassian account email") {
		t.Errorf("error %q does not carry the Basic-auth hint", err.Error())
	}
}

func TestClient_UnauthorizedStampsBearerAuthMethod(t *testing.T) {
	t.Parallel()

	client, _ := newRecordingClient(t, bearerOpts(), testkit.StubResponse{Status: http.StatusUnauthorized, Body: ""})

	_, err := client.GetCurrentUser(t.Context())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v is not an *APIError", err)
	}
	testkit.AssertEqual(t, "AuthMethod", apiErr.AuthMethod, AuthBearer)
	if !strings.Contains(err.Error(), "token rejected") {
		t.Errorf("error %q does not carry the bearer hint", err.Error())
	}
}

func TestClient_APIErrorPathDropsQuery(t *testing.T) {
	t.Parallel()

	client, _ := newRecordingClient(t, bearerOpts(), testkit.StubResponse{Status: http.StatusBadRequest,
		Body: `{"type":"error","error":{"message":"Invalid pagelen"}}`})

	_, err := client.ListRepositories(t.Context(), testWorkspace,
		ListRepositoriesOptions{Role: "", Query: "", Sort: "", MaxItems: 0})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v is not an *APIError", err)
	}
	testkit.AssertEqual(t, "Path", apiErr.Path, "/repositories/acme")
	testkit.AssertEqual(t, "Message", apiErr.Message, "Invalid pagelen")
}

func TestClient_TransportFailureIsWrapped(t *testing.T) {
	t.Parallel()

	opts := bearerOpts()
	opts.BaseURL = "http://127.0.0.1:1"
	opts.HTTPClient = &http.Client{Transport: &countingTransport{}}
	client := NewClient(opts)

	_, err := client.GetWorkspace(t.Context(), testWorkspace)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "execute request GET /workspaces/acme") {
		t.Errorf("error %q does not wrap the transport failure", err.Error())
	}
	if !strings.Contains(err.Error(), "get workspace acme") {
		t.Errorf("error %q lacks the method prefix", err.Error())
	}
}

func TestClient_MalformedJSONIsWrapped(t *testing.T) {
	t.Parallel()

	client, _ := newRecordingClient(t, bearerOpts(), testkit.StubResponse{Status: http.StatusOK, Body: "{not json"})

	_, err := client.GetCurrentUser(t.Context())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "decode response for GET /user") {
		t.Errorf("error %q does not name the decode failure", err.Error())
	}
}
