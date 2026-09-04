package bitbucket

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// The three 401 hints, pinned verbatim: they are the only place the UI tells a
// user which half of their credentials to fix.
const (
	bearerHint = " (token rejected: check it has scopes and is not expired; for an API token" +
		" you can also run 'lazybitbucket auth' and choose email + token)"
	basicHint = " (the Basic username must be your Atlassian account email, not a Bitbucket" +
		" username, and the token needs scopes)"
	noneHint = " (credentials rejected)"
)

func TestParseAPIError_Envelope(t *testing.T) {
	t.Parallel()

	body := []byte(`{"type":"error","error":{` +
		`"message":"Resource not found",` +
		`"detail":"There is no such pull request in this repository",` +
		`"id":"a1b2c3d4e5f6"}}`)

	apiErr := parseAPIError(testMethod, testPath, http.StatusNotFound, http.Header{}, body)

	testkit.AssertEqual(t, "Method", apiErr.Method, testMethod)
	testkit.AssertEqual(t, "Path", apiErr.Path, testPath)
	testkit.AssertEqual(t, "Status", apiErr.Status, http.StatusNotFound)
	testkit.AssertEqual(t, "Message", apiErr.Message, "Resource not found")
	testkit.AssertEqual(t, "Detail", apiErr.Detail, "There is no such pull request in this repository")
	testkit.AssertEqual(t, "ID", apiErr.ID, "a1b2c3d4e5f6")
	testkit.AssertEqual(t, "AcceptedScopes", apiErr.AcceptedScopes, "")
	testkit.AssertEqual(t, "AuthMethod", apiErr.AuthMethod, AuthNone)
	testkit.AssertEqual(t, "Body", apiErr.Body, string(body))
}

func TestAPIError_ErrorStringForNotFound(t *testing.T) {
	t.Parallel()

	// The exact rendering, pinned. 404 covers a wrong workspace, a missing repo,
	// a missing PR and a no-permission read alike, so the server's own Message is
	// the only thing that disambiguates and it must survive verbatim.
	apiErr := parseAPIError(testMethod, testPath, http.StatusNotFound, http.Header{},
		[]byte(`{"type":"error","error":{"message":"Resource not found"}}`))

	testkit.AssertEqual(t, "Error()", apiErr.Error(),
		"GET /repositories/acme/api-gateway/pullrequests/9: status 404 Not Found: Resource not found")
}

func TestAPIError_ErrorStringWithDetail(t *testing.T) {
	t.Parallel()

	apiErr := parseAPIError(testMethod, testPath, http.StatusBadRequest, http.Header{},
		[]byte(`{"type":"error","error":{"message":"Bad request","detail":"fields is invalid"}}`))

	testkit.AssertEqual(t, "Error()", apiErr.Error(),
		"GET /repositories/acme/api-gateway/pullrequests/9: status 400 Bad Request:"+
			" Bad request — fields is invalid")
}

func TestParseAPIError_EmptyBodyFallsBackToStatusText(t *testing.T) {
	t.Parallel()

	// An unauthenticated GET /user answers 401 with zero bytes.
	apiErr := parseAPIError(testMethod, "/user", http.StatusUnauthorized, http.Header{}, nil)

	testkit.AssertEqual(t, "Message", apiErr.Message, "Unauthorized")
	testkit.AssertEqual(t, "Detail", apiErr.Detail, "")
	testkit.AssertEqual(t, "ID", apiErr.ID, "")
	testkit.AssertEqual(t, "Body", apiErr.Body, "")
	testkit.AssertEqual(t, "Error()", apiErr.Error(), "GET /user: status 401 Unauthorized: Unauthorized"+noneHint)
}

func TestAPIError_UnauthorizedHintPerAuthMethod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method AuthMethod
		want   string
	}{
		{name: "bearer", method: AuthBearer, want: bearerHint},
		{name: "basic", method: AuthBasic, want: basicHint},
		{name: "none", method: AuthNone, want: noneHint},
		{name: "unset", method: "", want: noneHint},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			apiErr := parseAPIError(testMethod, "/user", http.StatusUnauthorized, http.Header{}, nil)
			apiErr.AuthMethod = test.method

			testkit.AssertEqual(t, "Error()", apiErr.Error(),
				"GET /user: status 401 Unauthorized: Unauthorized"+test.want)
		})
	}
}

func TestParseAPIError_NonJSONBody(t *testing.T) {
	t.Parallel()

	// A redirect that lands on a storage host answers HTML, not the envelope.
	body := []byte("<html><body>502 Bad Gateway</body></html>")

	apiErr := parseAPIError(testMethod, testPath, http.StatusBadGateway, http.Header{}, body)

	testkit.AssertEqual(t, "Message", apiErr.Message, "Bad Gateway")
	testkit.AssertEqual(t, "Body", apiErr.Body, string(body))
	testkit.AssertEqual(t, "Error()", apiErr.Error(),
		"GET /repositories/acme/api-gateway/pullrequests/9: status 502 Bad Gateway: Bad Gateway")
}

func TestParseAPIError_JSONWithoutErrorMember(t *testing.T) {
	t.Parallel()

	// Valid JSON that is not the error envelope still leaves Message usable.
	apiErr := parseAPIError(testMethod, testPath, http.StatusInternalServerError, http.Header{},
		[]byte(`{"type":"error"}`))

	testkit.AssertEqual(t, "Message", apiErr.Message, "Internal Server Error")
}

func TestParseAPIError_UnknownStatusText(t *testing.T) {
	t.Parallel()

	apiErr := parseAPIError(testMethod, testPath, 599, http.Header{}, nil)

	testkit.AssertEqual(t, "Message", apiErr.Message, "status 599")
	testkit.AssertEqual(t, "Error()", apiErr.Error(),
		"GET /repositories/acme/api-gateway/pullrequests/9: status 599: status 599")
}

func TestAPIError_ServerErrorMentionsStatus(t *testing.T) {
	t.Parallel()

	// Every per-method wrapper test asserts this substring, so it is pinned here.
	apiErr := parseAPIError(testMethod, testPath, http.StatusInternalServerError, http.Header{},
		[]byte(`{"type":"error","error":{"message":"boom"}}`))

	wrapped := fmt.Errorf("list pull requests acme/api-gateway: %w", apiErr)
	if !strings.Contains(wrapped.Error(), "status 500") {
		t.Errorf("wrapped.Error() = %q, want it to contain %q", wrapped.Error(), "status 500")
	}
	if !strings.Contains(wrapped.Error(), "list pull requests acme/api-gateway: ") {
		t.Errorf("wrapped.Error() = %q, want the wrapper prefix", wrapped.Error())
	}
}

func TestParseAPIError_ForbiddenAcceptedScopes(t *testing.T) {
	t.Parallel()

	header := http.Header{}
	header.Set("X-Accepted-OAuth-Scopes", "repository")

	apiErr := parseAPIError(testMethod, testPath, http.StatusForbidden, http.Header{},
		[]byte(`{"type":"error","error":{"message":"Access denied"}}`))
	withScopes := parseAPIError(testMethod, testPath, http.StatusForbidden, header,
		[]byte(`{"type":"error","error":{"message":"Access denied"}}`))

	testkit.AssertEqual(t, "AcceptedScopes", withScopes.AcceptedScopes, "repository")
	testkit.AssertEqual(t, "Error() with scopes", withScopes.Error(),
		"GET /repositories/acme/api-gateway/pullrequests/9: status 403 Forbidden:"+
			" Access denied (required scope: repository)")
	testkit.AssertEqual(t, "Error() without scopes", apiErr.Error(),
		"GET /repositories/acme/api-gateway/pullrequests/9: status 403 Forbidden: Access denied")
}

func TestParseAPIError_BodyTruncatedTo4KiB(t *testing.T) {
	t.Parallel()

	body := []byte(strings.Repeat("x", maxErrorBodyBytes+1024))

	apiErr := parseAPIError(testMethod, testPath, http.StatusBadGateway, http.Header{}, body)

	testkit.AssertEqual(t, "len(Body)", len(apiErr.Body), maxErrorBodyBytes)

	short := parseAPIError(testMethod, testPath, http.StatusBadGateway, http.Header{}, body[:16])
	testkit.AssertEqual(t, "len(Body) under the cap", len(short.Body), 16)
}

func TestErrorPredicates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status int
		want   [4]bool // notFound, unauthorized, forbidden, rateLimited
	}{
		{name: "404", status: http.StatusNotFound, want: [4]bool{true, false, false, false}},
		{name: "401", status: http.StatusUnauthorized, want: [4]bool{false, true, false, false}},
		{name: "403", status: http.StatusForbidden, want: [4]bool{false, false, true, false}},
		{name: "429", status: http.StatusTooManyRequests, want: [4]bool{false, false, false, true}},
		{name: "500", status: http.StatusInternalServerError, want: [4]bool{false, false, false, false}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// Predicates must see through the wrapper every client method adds.
			err := fmt.Errorf("get pull request acme/api-gateway#9: %w",
				parseAPIError(testMethod, testPath, test.status, http.Header{}, nil))

			got := [4]bool{IsNotFound(err), IsUnauthorized(err), IsForbidden(err), IsRateLimited(err)}
			testkit.AssertEqual(t, "IsNotFound", got[0], test.want[0])
			testkit.AssertEqual(t, "IsUnauthorized", got[1], test.want[1])
			testkit.AssertEqual(t, "IsForbidden", got[2], test.want[2])
			testkit.AssertEqual(t, "IsRateLimited", got[3], test.want[3])
		})
	}
}

func TestErrorPredicatesIgnoreOtherErrors(t *testing.T) {
	t.Parallel()

	plain := errors.New("dial tcp: connection refused")

	testkit.AssertEqual(t, "IsNotFound(plain)", IsNotFound(plain), false)
	testkit.AssertEqual(t, "IsUnauthorized(nil)", IsUnauthorized(nil), false)
	testkit.AssertEqual(t, "IsForbidden(nil)", IsForbidden(nil), false)
	testkit.AssertEqual(t, "IsRateLimited(plain)", IsRateLimited(plain), false)
}

func TestAPIError_ErrorsAsRecoversFields(t *testing.T) {
	t.Parallel()

	wrapped := fmt.Errorf("list statuses acme/api-gateway#9: %w",
		parseAPIError(testMethod, testPath, http.StatusNotFound, http.Header{},
			[]byte(`{"type":"error","error":{"message":"Resource not found","id":"deadbeef"}}`)))

	var apiErr *APIError
	if !errors.As(wrapped, &apiErr) {
		t.Fatalf("errors.As(%v) = false, want the *APIError", wrapped)
	}
	testkit.AssertEqual(t, "Status", apiErr.Status, http.StatusNotFound)
	testkit.AssertEqual(t, "ID", apiErr.ID, "deadbeef")
}

func TestParseAPIError_DiffTooLargeStatus(t *testing.T) {
	t.Parallel()

	// 555 is Bitbucket's own invention: net/http has no text for it, so the
	// APIError has to supply one or the message reads as a bare number.
	apiErr := parseAPIError(testMethod, testPath, StatusDiffTooLarge, http.Header{}, nil)

	testkit.AssertEqual(t, "Status", apiErr.Status, 555)
	testkit.AssertEqual(t, "Message", apiErr.Message, "Diff Too Large")
	testkit.AssertEqual(t, "Error()", apiErr.Error(),
		"GET /repositories/acme/api-gateway/pullrequests/9: status 555 Diff Too Large: Diff Too Large")
}

func TestIsDiffTooLarge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "plain error", err: errors.New("boom"), want: false},
		{
			name: "another status",
			err:  &APIError{Method: testMethod, Path: testPath, Status: http.StatusNotFound},
			want: false,
		},
		{
			name: "555",
			err:  &APIError{Method: testMethod, Path: testPath, Status: StatusDiffTooLarge},
			want: true,
		},
		{
			name: "wrapped 555",
			err: fmt.Errorf("get diff acme/api-gateway#9: %w",
				&APIError{Method: testMethod, Path: testPath, Status: StatusDiffTooLarge}),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testkit.AssertEqual(t, "IsDiffTooLarge", IsDiffTooLarge(tt.err), tt.want)
		})
	}
}
