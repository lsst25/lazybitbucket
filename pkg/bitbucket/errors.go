package bitbucket

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
)

// maxErrorBodyBytes caps how much of a non-2xx body an APIError retains.
const maxErrorBodyBytes = 4 << 10 // 4 KiB

// acceptedScopesHeader names the scope Bitbucket wanted for a 403.
const acceptedScopesHeader = "X-Accepted-OAuth-Scopes"

// credentialsRejectedHint is the 401 hint used when the credential shape behind
// the request is unknown.
const credentialsRejectedHint = " (credentials rejected)"

// StatusDiffTooLarge is Bitbucket's nonstandard status for a diff or diffstat
// that timed out because it was too large. Neither endpoint paginates the
// document it builds, so a big enough pull request answers this instead of a
// diff — and net/http knows no text for it.
const StatusDiffTooLarge = 555

// statusTextDiffTooLarge is the status text APIError renders for a 555 answering
// a GET.
const statusTextDiffTooLarge = "Diff Too Large"

// statusTextTimedOut is the status text APIError renders for a 555 answering a
// write. The merge and decline endpoints answer 555 when the operation took too
// long, which has nothing to do with a diff being too large: the operation may
// even have completed, so the fix is to refresh rather than to fetch less.
const statusTextTimedOut = "Timed Out"

// ErrReadOnly is what every mutating method returns when the client was built
// with ClientOpts.ReadOnly. It is reported before a request is built, so no
// socket is opened and nothing reaches Bitbucket.
var ErrReadOnly = errors.New("client is read-only: refusing to modify Bitbucket")

// IsReadOnly reports whether err is the read-only refusal, wrapped or not. The
// TUI uses it to answer "read-only mode" instead of an API message.
func IsReadOnly(err error) bool { return errors.Is(err, ErrReadOnly) }

// APIError is a non-2xx response from the Bitbucket Cloud REST API. Bitbucket
// returns 404 for a wrong workspace, a missing repository, a missing pull
// request and a no-permission read alike, so Message is shown verbatim in the
// UI — it is the only place the server's own hint survives.
type APIError struct {
	Method string
	Path   string // path relative to the base URL, without query
	Status int
	// Message is error.message from the envelope, else the HTTP status text.
	Message string
	// Detail is error.detail from the envelope, when present.
	Detail string
	// ID is error.id from the envelope: Bitbucket's log correlation id, worth
	// quoting in a bug report.
	ID string
	// Fields is error.fields from the envelope: per-input validation messages a
	// write can fail with, e.g. {"title": ["This field is required."]}. It is nil
	// for the read endpoints, which never send it.
	Fields map[string][]string
	// AcceptedScopes is the X-Accepted-OAuth-Scopes response header, which tells
	// which scope a 403 wanted.
	AcceptedScopes string
	// AuthMethod is the credential shape the failing request used. The client
	// stamps it after parseAPIError returns, so a 401 can name the right fix;
	// it is AuthNone (the zero value is "", handled the same way) when unknown.
	AuthMethod AuthMethod
	// Body is the raw response body, truncated to 4 KiB.
	Body string
}

// Error renders the failure the way the UI shows it: request, status, the
// server's own message, then whatever hint the status and credential shape
// allow.
func (e *APIError) Error() string {
	var out strings.Builder
	out.WriteString(e.Method)
	out.WriteString(" ")
	out.WriteString(e.Path)
	out.WriteString(": status ")
	out.WriteString(strconv.Itoa(e.Status))
	if text := statusText(e.Status, e.Method); text != "" {
		out.WriteString(" ")
		out.WriteString(text)
	}
	if message := e.UserMessage(); message != "" {
		out.WriteString(": ")
		out.WriteString(message)
	}
	if e.Detail != "" {
		out.WriteString(" — ")
		out.WriteString(e.Detail)
	}

	switch e.Status {
	case http.StatusUnauthorized:
		out.WriteString(unauthorizedHint(e.AuthMethod))
	case http.StatusForbidden:
		if e.AcceptedScopes != "" {
			out.WriteString(" (required scope: ")
			out.WriteString(e.AcceptedScopes)
			out.WriteString(")")
		}
	default:
	}
	return out.String()
}

// UserMessage is the single line the UI shows: the server's own message, plus
// the first per-field validation detail when the response carried any. A write
// rejected for an empty title reads "title: This field is required." rather
// than the generic "Bad request" the envelope's message often is.
//
// Fields are visited in sorted key order so the sentence is stable across runs
// — Go map iteration is not.
func (e *APIError) UserMessage() string {
	for _, name := range slices.Sorted(maps.Keys(e.Fields)) {
		messages := e.Fields[name]
		if len(messages) == 0 || messages[0] == "" {
			continue
		}
		if e.Message == "" {
			return name + ": " + messages[0]
		}
		if strings.Contains(strings.ToLower(e.Message), strings.ToLower(messages[0])) {
			// Some envelopes repeat the field sentence in the generic message;
			// appending it again would read "title: required: title: required".
			return e.Message
		}
		return e.Message + ": " + name + ": " + messages[0]
	}
	return e.Message
}

// unauthorizedHint is the actionable half of a 401 message. Which credential
// shape failed decides what the user has to change, so the hint is written per
// AuthMethod rather than as one catch-all sentence.
func unauthorizedHint(method AuthMethod) string {
	switch method {
	case AuthBearer:
		return " (token rejected: check it has scopes and is not expired; for an API token" +
			" you can also run 'lazybitbucket auth' and choose email + token)"
	case AuthBasic:
		return " (the Basic username must be your Atlassian account email, not a Bitbucket" +
			" username, and the token needs scopes)"
	case AuthNone:
		return credentialsRejectedHint
	default:
		return credentialsRejectedHint
	}
}

// IsNotFound reports whether err is a 404. Bitbucket does not distinguish
// "does not exist" from "you cannot see it".
func IsNotFound(err error) bool { return hasStatus(err, http.StatusNotFound) }

// IsUnauthorized reports whether err is a 401: missing, malformed or expired
// credentials.
func IsUnauthorized(err error) bool { return hasStatus(err, http.StatusUnauthorized) }

// IsForbidden reports whether err is a 403: authenticated but not permitted,
// usually a missing token scope.
func IsForbidden(err error) bool { return hasStatus(err, http.StatusForbidden) }

// IsRateLimited reports whether err is a 429. The client does not retry.
func IsRateLimited(err error) bool { return hasStatus(err, http.StatusTooManyRequests) }

// IsDiffTooLarge reports whether err is Bitbucket's 555: the diff or diffstat
// was too large and the server gave up building it. Retrying is pointless — the
// only way out is a per-file diff or the browser — so this must be told apart
// from the 5xx statuses a retry could fix.
func IsDiffTooLarge(err error) bool { return hasStatus(err, StatusDiffTooLarge) }

func hasStatus(err error, status int) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == status
}

// parseAPIError builds an APIError from a non-2xx response. It parses
// Bitbucket's standard envelope
//
//	{"type":"error","error":{"message":"…","detail":"…","id":"…"}}
//
// and tolerates both an empty body (an unauthenticated GET /user returns zero
// bytes) and a non-JSON body (the diff redirect answers text/html), falling back
// to the HTTP status text for Message.
//
// It deliberately does not know how the request authenticated: the caller sets
// AuthMethod on the returned error afterwards, so a 401 can name the right fix.
func parseAPIError(method, path string, status int, header http.Header, body []byte) *APIError {
	apiErr := &APIError{
		Method:         method,
		Path:           path,
		Status:         status,
		Message:        "",
		Detail:         "",
		ID:             "",
		Fields:         nil,
		AcceptedScopes: header.Get(acceptedScopesHeader),
		AuthMethod:     AuthNone,
		Body:           truncateBody(body),
	}

	if len(bytes.TrimSpace(body)) > 0 {
		var envelope struct {
			Type  string `json:"type"`
			Error struct {
				Message string                     `json:"message"`
				Detail  string                     `json:"detail"`
				ID      string                     `json:"id"`
				Fields  map[string]json.RawMessage `json:"fields"`
			} `json:"error"`
		}
		if err := json.Unmarshal(body, &envelope); err == nil {
			apiErr.Message = envelope.Error.Message
			apiErr.Detail = envelope.Error.Detail
			apiErr.ID = envelope.Error.ID
			apiErr.Fields = parseErrorFields(envelope.Error.Fields)
		}
	}

	if apiErr.Message == "" {
		apiErr.Message = statusMessage(status, method)
	}
	return apiErr
}

// parseErrorFields decodes error.fields leniently. Bitbucket documents the
// values as arrays of strings, but sends a bare string for some inputs, so both
// shapes are accepted and anything else for a key is dropped rather than
// failing the whole error.
func parseErrorFields(raw map[string]json.RawMessage) map[string][]string {
	if len(raw) == 0 {
		return nil
	}

	fields := make(map[string][]string, len(raw))
	for name, value := range raw {
		var messages []string
		if err := json.Unmarshal(value, &messages); err == nil {
			fields[name] = messages
			continue
		}
		var single string
		if err := json.Unmarshal(value, &single); err == nil {
			fields[name] = []string{single}
		}
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}

// errorEnvelopeError reports the *APIError hiding in an otherwise successful
// body. The merge task-status endpoint answers 200 with Bitbucket's error
// envelope when the merge itself failed, so the status alone cannot be trusted
// there. A body that is not an error envelope yields nil.
func errorEnvelopeError(method, path string, status int, body []byte) *APIError {
	var envelope struct {
		Type string `json:"type"`
	}
	// A body that is not JSON leaves Type empty, which is answer enough: only a
	// decoded "type":"error" makes this an envelope worth reporting.
	_ = json.Unmarshal(body, &envelope)
	if envelope.Type != errorEnvelopeType {
		return nil
	}
	return parseAPIError(method, path, status, http.Header{}, body)
}

// errorEnvelopeType is the "type" member of Bitbucket's error envelope.
const errorEnvelopeType = "error"

// statusMessage is the fallback Message for a body that carried none.
func statusMessage(status int, method string) string {
	if text := statusText(status, method); text != "" {
		return text
	}
	return fmt.Sprintf("status %d", status)
}

// statusText is http.StatusText plus the one status Bitbucket invented. 555
// means two different things depending on the verb: a GET could not build a
// diff that large, while a write (merge, decline) ran past the gateway's
// deadline and may or may not have completed.
func statusText(status int, method string) string {
	if status == StatusDiffTooLarge {
		if method == http.MethodGet {
			return statusTextDiffTooLarge
		}
		return statusTextTimedOut
	}
	return http.StatusText(status)
}

func truncateBody(body []byte) string {
	if len(body) > maxErrorBodyBytes {
		body = body[:maxErrorBodyBytes]
	}
	return strings.ToValidUTF8(string(body), "")
}
