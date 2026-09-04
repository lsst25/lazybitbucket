package bitbucket

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
	if text := http.StatusText(e.Status); text != "" {
		out.WriteString(" ")
		out.WriteString(text)
	}
	if e.Message != "" {
		out.WriteString(": ")
		out.WriteString(e.Message)
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
		AcceptedScopes: header.Get(acceptedScopesHeader),
		AuthMethod:     AuthNone,
		Body:           truncateBody(body),
	}

	if len(bytes.TrimSpace(body)) > 0 {
		var envelope struct {
			Type  string `json:"type"`
			Error struct {
				Message string `json:"message"`
				Detail  string `json:"detail"`
				ID      string `json:"id"`
			} `json:"error"`
		}
		if err := json.Unmarshal(body, &envelope); err == nil {
			apiErr.Message = envelope.Error.Message
			apiErr.Detail = envelope.Error.Detail
			apiErr.ID = envelope.Error.ID
		}
	}

	if apiErr.Message == "" {
		apiErr.Message = statusMessage(status)
	}
	return apiErr
}

// statusMessage is the fallback Message for a body that carried none.
func statusMessage(status int) string {
	if text := http.StatusText(status); text != "" {
		return text
	}
	return fmt.Sprintf("status %d", status)
}

func truncateBody(body []byte) string {
	if len(body) > maxErrorBodyBytes {
		body = body[:maxErrorBodyBytes]
	}
	return strings.ToValidUTF8(string(body), "")
}
