package bitbucket

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// diffStatEntryJSON is a modified file, the shape observed live in
// phaseA/api-errors-users-misc.md §8.
const diffStatEntryJSON = `{
  "type": "diffstat",
  "status": "modified",
  "lines_added": 12,
  "lines_removed": 3,
  "old": {
    "type": "commit_file",
    "path": "pkg/tui/app.go",
    "escaped_path": "pkg/tui/app.go",
    "links": {"self": {"href": "https://api.bitbucket.org/2.0/x/src/3c39f5b55d7f/pkg/tui/app.go"}}
  },
  "new": {
    "type": "commit_file",
    "path": "pkg/tui/app.go",
    "escaped_path": "pkg/tui/app.go",
    "links": {"self": {"href": "https://api.bitbucket.org/2.0/x/src/06fd23caff6b/pkg/tui/app.go"}}
  }
}`

// addedFileJSON has no old side, which is how an added file surfaces.
const addedFileJSON = `{
  "type": "diffstat",
  "status": "added",
  "lines_added": 40,
  "lines_removed": 0,
  "old": null,
  "new": {"type": "commit_file", "path": "pkg/diff/parse.go", "escaped_path": "pkg/diff/parse.go"}
}`

// removedFileJSON has no new side.
const removedFileJSON = `{
  "type": "diffstat",
  "status": "removed",
  "lines_added": 0,
  "lines_removed": 18,
  "old": {"type": "commit_file", "path": "pkg/jira/client.go", "escaped_path": "pkg/jira/client.go"},
  "new": null
}`

// renamedBinaryJSON is a rename whose file is binary, so both the old path and
// the binary attribute matter.
const renamedBinaryJSON = `{
  "type": "diffstat",
  "status": "renamed",
  "lines_added": 0,
  "lines_removed": 0,
  "old": {"type": "commit_file", "path": "docs/old logo.png", "escaped_path": "docs/old%20logo.png"},
  "new": {
    "type": "commit_file",
    "path": "docs/logo.png",
    "escaped_path": "docs/logo.png",
    "attributes": ["binary"]
  }
}`

// diffText is a two-file unified diff, as the followed /diff endpoint returns
// it (phaseA/api-rest-docs.md §6.7).
const diffText = `diff --git a/pkg/tui/app.go b/pkg/tui/app.go
index 492d9ba7970..6ddf9ae6786 100644
--- a/pkg/tui/app.go
+++ b/pkg/tui/app.go
@@ -119,7 +119,7 @@ func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
-	old line
+	new line
`

func TestDiffStatEntry_Unmarshal(t *testing.T) {
	t.Parallel()

	var entry DiffStatEntry
	if err := json.Unmarshal([]byte(diffStatEntryJSON), &entry); err != nil {
		t.Fatalf("unmarshal diffstat entry: %v", err)
	}

	testkit.AssertEqual(t, "Type", entry.Type, "diffstat")
	testkit.AssertEqual(t, "Status", entry.Status, DiffStatModified)
	testkit.AssertEqual(t, "LinesAdded", entry.LinesAdded, 12)
	testkit.AssertEqual(t, "LinesRemoved", entry.LinesRemoved, 3)
	testkit.AssertEqual(t, "Path", entry.Path(), testDiffFilePath)
	testkit.AssertEqual(t, "OldPath", entry.OldPath(), testDiffFilePath)
	testkit.AssertEqual(t, "EscapedPath", entry.EscapedPath(), testDiffFilePath)
	testkit.AssertEqual(t, "IsBinary", entry.IsBinary(), false)
	if entry.New == nil {
		t.Fatal("New is nil")
	}
	testkit.AssertEqual(t, "New self link", entry.New.Links.Self.Href,
		"https://api.bitbucket.org/2.0/x/src/06fd23caff6b/pkg/tui/app.go")
}

func TestDiffStatEntry_Sides(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		raw         string
		wantStatus  string
		wantPath    string
		wantOldPath string
		wantEscaped string
		wantBinary  bool
	}{
		{
			name:        "added file has no old side",
			raw:         addedFileJSON,
			wantStatus:  DiffStatAdded,
			wantPath:    "pkg/diff/parse.go",
			wantOldPath: "",
			wantEscaped: "pkg/diff/parse.go",
			wantBinary:  false,
		},
		{
			name:        "removed file has no new side",
			raw:         removedFileJSON,
			wantStatus:  DiffStatRemoved,
			wantPath:    "pkg/jira/client.go",
			wantOldPath: "pkg/jira/client.go",
			wantEscaped: "pkg/jira/client.go",
			wantBinary:  false,
		},
		{
			name:        "renamed binary file",
			raw:         renamedBinaryJSON,
			wantStatus:  DiffStatRenamed,
			wantPath:    "docs/logo.png",
			wantOldPath: "docs/old logo.png",
			wantEscaped: "docs/logo.png",
			wantBinary:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var entry DiffStatEntry
			if err := json.Unmarshal([]byte(tt.raw), &entry); err != nil {
				t.Fatalf("unmarshal diffstat entry: %v", err)
			}

			testkit.AssertEqual(t, "Status", entry.Status, tt.wantStatus)
			testkit.AssertEqual(t, "Path", entry.Path(), tt.wantPath)
			testkit.AssertEqual(t, "OldPath", entry.OldPath(), tt.wantOldPath)
			testkit.AssertEqual(t, "EscapedPath", entry.EscapedPath(), tt.wantEscaped)
			testkit.AssertEqual(t, "IsBinary", entry.IsBinary(), tt.wantBinary)
		})
	}
}

func TestDiffStatEntry_EmptyEntryHasNoPath(t *testing.T) {
	t.Parallel()

	var entry DiffStatEntry

	testkit.AssertEqual(t, "Path", entry.Path(), "")
	testkit.AssertEqual(t, "OldPath", entry.OldPath(), "")
	testkit.AssertEqual(t, "EscapedPath", entry.EscapedPath(), "")
	testkit.AssertEqual(t, "IsBinary", entry.IsBinary(), false)
}

func TestClient_ListPullRequestDiffStat_FollowsTheRedirectAndPaginatesTheTarget(t *testing.T) {
	t.Parallel()

	// The endpoint 302s to the repository diffstat for the PR's revspec, and the
	// target paginates on its own. Both hops must carry the credentials, and the
	// next link points at the target, not at the pull request path.
	rec := &recorder{}
	client := newHandlerClient(t, func(w http.ResponseWriter, r *http.Request) {
		rec.add(r)

		switch {
		case r.URL.Path == testDiffStatPath:
			w.Header().Set("Location", testDiffStatTarget+"?from_pullrequest_id=9&topic=true")
			w.WriteHeader(http.StatusFound)
		case r.URL.Query().Get("page") == "2":
			writeJSON(t, w, http.StatusOK, `{"pagelen": 500, "size": 3, "values": [`+removedFileJSON+`]}`)
		default:
			next, err := json.Marshal("http://" + r.Host + testDiffStatTarget + "?page=2")
			if err != nil {
				t.Errorf("marshal next link: %v", err)
				return
			}
			writeJSON(t, w, http.StatusOK, `{"pagelen": 500, "size": 3, "next": `+string(next)+
				`, "values": [`+diffStatEntryJSON+`,`+addedFileJSON+`]}`)
		}
	})

	entries, err := client.ListPullRequestDiffStat(t.Context(), testWorkspace, testRepoSlug, testPRID)
	if err != nil {
		t.Fatalf("ListPullRequestDiffStat: %v", err)
	}

	testkit.AssertEqual(t, "entries", len(entries), 3)
	testkit.AssertEqual(t, "third path", entries[2].Path(), "pkg/jira/client.go")

	requests := rec.snapshot()
	testkit.AssertEqual(t, "requests", len(requests), 3)
	testkit.AssertEqual(t, "pull request path", requests[0].Path, testDiffStatPath)
	testkit.AssertEqual(t, "pagelen", requests[0].Query.Get(paramPageLen), "500")
	testkit.AssertEqual(t, "redirect target", requests[1].Path, testDiffStatTarget)
	testkit.AssertEqual(t, "redirect query", requests[1].Query.Get("from_pullrequest_id"), "9")
	// The Location carries the server's own parameters and not ours, so the
	// second hop loses the requested pagelen. That costs pages, not entries:
	// the paginator follows next until the collection is exhausted.
	testkit.AssertEqual(t, "redirect drops pagelen", requests[1].Query.Get(paramPageLen), "")
	testkit.AssertEqual(t, "next target", requests[2].Path, testDiffStatTarget)

	for index, request := range requests {
		if request.Header.Get(authorizationHeader) != "Bearer "+testToken {
			t.Errorf("request %d lost the Authorization header: %q",
				index, request.Header.Get(authorizationHeader))
		}
	}
}

func TestClient_ListPullRequestDiffStat_DiffTooLarge(t *testing.T) {
	t.Parallel()

	client := newHandlerClient(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, StatusDiffTooLarge, errorEnvelope("Diff too large to generate"))
	})

	_, err := client.ListPullRequestDiffStat(t.Context(), testWorkspace, testRepoSlug, testPRID)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !IsDiffTooLarge(err) {
		t.Errorf("IsDiffTooLarge(%v) = false, want true", err)
	}
	if !strings.Contains(err.Error(), "get diffstat acme/api-gateway#9") {
		t.Errorf("error %q does not carry the wrapper prefix", err.Error())
	}
	if !strings.Contains(err.Error(), "status 555 Diff Too Large") {
		t.Errorf("error %q does not render the 555 status text", err.Error())
	}
}

func TestClient_GetPullRequestDiff_FollowsTheRedirectWithCredentials(t *testing.T) {
	t.Parallel()

	// The client resolves the 302 itself, and the target is on the same API
	// host and still needs authenticating: the second hop must carry the
	// credentials and the text/plain Accept just as the first one did.
	rec := &recorder{}
	client := newHandlerClient(t, func(w http.ResponseWriter, r *http.Request) {
		rec.add(r)

		if r.URL.Path == testDiffPath {
			w.Header().Set("Location", testDiffTarget+"?from_pullrequest_id=9")
			w.WriteHeader(http.StatusFound)
			return
		}
		writeText(w, http.StatusOK, diffText)
	})

	diff, err := client.GetPullRequestDiff(t.Context(), testWorkspace, testRepoSlug, testPRID,
		DiffOptions{Path: "", Context: 0, DiffURL: ""})
	if err != nil {
		t.Fatalf("GetPullRequestDiff: %v", err)
	}

	testkit.AssertEqual(t, "diff", diff, diffText)

	requests := rec.snapshot()
	testkit.AssertEqual(t, "requests", len(requests), 2)
	testkit.AssertEqual(t, "first path", requests[0].Path, testDiffPath)
	testkit.AssertEqual(t, "accept", requests[0].Header.Get("Accept"), contentTypeText)
	testkit.AssertEqual(t, "redirect target", requests[1].Path, testDiffTarget)
	testkit.AssertEqual(t, "redirected authorization",
		requests[1].Header.Get(authorizationHeader), "Bearer "+testToken)
	testkit.AssertEqual(t, "redirect keeps accept", requests[1].Header.Get("Accept"), contentTypeText)
}

func TestClient_GetPullRequestDiff_PathAndContextParameters(t *testing.T) {
	t.Parallel()

	// This is the regression: the real API's Location carries only its own
	// query, and Go builds the redirected request from the Location alone, so a
	// followed redirect drops path= and context= and answers with the whole
	// pull request diff. The parameters have to land on the second hop, on top
	// of the two the server chose, and nothing may be lost either way.
	rec := &recorder{}
	client := newHandlerClient(t, func(w http.ResponseWriter, r *http.Request) {
		rec.add(r)

		if r.URL.Path == testDiffPath {
			w.Header().Set("Location", testDiffTarget+"?from_pullrequest_id=9&exclude_files=93c7645b")
			w.WriteHeader(http.StatusFound)
			return
		}
		writeText(w, http.StatusOK, diffText)
	})

	_, err := client.GetPullRequestDiff(t.Context(), testWorkspace, testRepoSlug, testPRID,
		DiffOptions{Path: "docs/old logo.png", Context: 8, DiffURL: ""})
	if err != nil {
		t.Fatalf("GetPullRequestDiff: %v", err)
	}

	requests := rec.snapshot()
	testkit.AssertEqual(t, "requests", len(requests), 2)
	targets := rec.requestTargets()
	// The probe carries the parameters too, for a deployment that answers
	// directly; the real API ignores them when it builds the Location.
	testkit.AssertEqual(t, "probe path", requests[0].Path, testDiffPath)
	testkit.AssertEqual(t, "probe path param", requests[0].Query.Get("path"), "docs/old logo.png")

	testkit.AssertEqual(t, "target path", requests[1].Path, testDiffTarget)
	testkit.AssertEqual(t, "path param", requests[1].Query.Get("path"), "docs/old logo.png")
	testkit.AssertEqual(t, "context param", requests[1].Query.Get("context"), "8")
	// The escaped form is what goes on the wire: a bare space would not.
	for _, want := range []string{"path=docs%2Fold+logo.png", "context=8"} {
		if !strings.Contains(targets[1], want) {
			t.Errorf("second hop %q does not carry %q", targets[1], want)
		}
	}
	// The server's own parameters survive the merge.
	testkit.AssertEqual(t, "kept from_pullrequest_id", requests[1].Query.Get("from_pullrequest_id"), "9")
	testkit.AssertEqual(t, "kept exclude_files", requests[1].Query.Get("exclude_files"), "93c7645b")
	// merge= was removed from the API on 2026-09-04 and must never be sent.
	testkit.AssertEqual(t, "merge param", requests[1].Query.Get("merge"), "")
}

func TestClient_GetPullRequestDiff_RefusesAForeignRedirect(t *testing.T) {
	t.Parallel()

	// A Location on another host is where the token would leak. The hop is
	// refused outright rather than falling back to an unfiltered diff, and
	// nothing is requested after the probe.
	rec := &recorder{}
	client := newHandlerClient(t, func(w http.ResponseWriter, r *http.Request) {
		rec.add(r)
		w.Header().Set("Location", "https://evil.example.com"+testDiffTarget+"?from_pullrequest_id=9")
		w.WriteHeader(http.StatusFound)
	})

	_, err := client.GetPullRequestDiff(t.Context(), testWorkspace, testRepoSlug, testPRID,
		DiffOptions{Path: testDiffFilePath, Context: 0, DiffURL: ""})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "refusing to follow diff redirect to evil.example.com") {
		t.Errorf("error %q does not refuse the foreign host", err.Error())
	}
	if !strings.Contains(err.Error(), "get diff acme/api-gateway#9") {
		t.Errorf("error %q does not carry the wrapper prefix", err.Error())
	}
	testkit.AssertEqual(t, "requests", len(rec.snapshot()), 1)
}

func TestClient_GetPullRequestDiff_AcceptsADirectAnswer(t *testing.T) {
	t.Parallel()

	// Not every deployment redirects — a caching proxy in front of the API may
	// answer the diff itself — so a 200 on the first hop is the body.
	rec := &recorder{}
	client := newHandlerClient(t, func(w http.ResponseWriter, r *http.Request) {
		rec.add(r)
		writeText(w, http.StatusOK, diffText)
	})

	diff, err := client.GetPullRequestDiff(t.Context(), testWorkspace, testRepoSlug, testPRID,
		DiffOptions{Path: "internal/rate/bucket.go", Context: 3, DiffURL: ""})
	if err != nil {
		t.Fatalf("GetPullRequestDiff: %v", err)
	}

	testkit.AssertEqual(t, "diff", diff, diffText)
	requests := rec.snapshot()
	testkit.AssertEqual(t, "requests", len(requests), 1)
	testkit.AssertEqual(t, "path", requests[0].Path, testDiffPath)
	// A direct answer is the only hop there is, so the file filter has to be on
	// it or the whole pull request diff would come back.
	testkit.AssertEqual(t, "path param", requests[0].Query.Get("path"), "internal/rate/bucket.go")
	testkit.AssertEqual(t, "context param", requests[0].Query.Get("context"), "3")
}

func TestClient_GetPullRequestDiff_RejectsARedirectWithNoLocation(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	client := newHandlerClient(t, func(w http.ResponseWriter, r *http.Request) {
		rec.add(r)
		w.WriteHeader(http.StatusFound)
	})

	_, err := client.GetPullRequestDiff(t.Context(), testWorkspace, testRepoSlug, testPRID,
		DiffOptions{Path: "", Context: 0, DiffURL: ""})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "no Location header") {
		t.Errorf("error %q does not name the missing Location", err.Error())
	}
	testkit.AssertEqual(t, "requests", len(rec.snapshot()), 1)
}

func TestClient_GetPullRequestDiff_UsesASameOriginDiffURL(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	client := newHandlerClient(t, func(w http.ResponseWriter, r *http.Request) {
		rec.add(r)
		writeText(w, http.StatusOK, diffText)
	})

	// The href the API handed out already carries the parameters the redirect
	// would have added, so it is fetched directly — with the caller's path
	// merged in and the existing query preserved.
	diffURL := client.BaseURL() + testDiffTarget + "?from_pullrequest_id=9&exclude_files=93c7645b"
	_, err := client.GetPullRequestDiff(t.Context(), testWorkspace, testRepoSlug, testPRID,
		DiffOptions{Path: testDiffFilePath, Context: 0, DiffURL: diffURL})
	if err != nil {
		t.Fatalf("GetPullRequestDiff: %v", err)
	}

	requests := rec.snapshot()
	testkit.AssertEqual(t, "requests", len(requests), 1)
	testkit.AssertEqual(t, "target path", requests[0].Path, testDiffTarget)
	testkit.AssertEqual(t, "path param", requests[0].Query.Get("path"), testDiffFilePath)
	testkit.AssertEqual(t, "kept from_pullrequest_id", requests[0].Query.Get("from_pullrequest_id"), "9")
	testkit.AssertEqual(t, "kept exclude_files", requests[0].Query.Get("exclude_files"), "93c7645b")
	testkit.AssertEqual(t, "authorization", requests[0].Header.Get(authorizationHeader), "Bearer "+testToken)
}

func TestClient_GetPullRequestDiff_IgnoresAForeignDiffURL(t *testing.T) {
	t.Parallel()

	// A diff link on another host must not receive the Authorization header, so
	// it is ignored and the pull request's own /diff path is used instead.
	rec := &recorder{}
	client := newHandlerClient(t, func(w http.ResponseWriter, r *http.Request) {
		rec.add(r)
		writeText(w, http.StatusOK, diffText)
	})

	for _, diffURL := range []string{
		"https://evil.example.com/2.0/repositories/acme/api-gateway/diff/spec",
		"http://127.0.0.1:1/2.0/diff",
		"::not a url::",
	} {
		_, err := client.GetPullRequestDiff(t.Context(), testWorkspace, testRepoSlug, testPRID,
			DiffOptions{Path: "", Context: 0, DiffURL: diffURL})
		if err != nil {
			t.Fatalf("GetPullRequestDiff(%q): %v", diffURL, err)
		}
	}

	requests := rec.snapshot()
	testkit.AssertEqual(t, "requests", len(requests), 3)
	for index, request := range requests {
		testkit.AssertEqual(t, "path of request "+strconv.Itoa(index), request.Path, testDiffPath)
	}
}

func TestClient_GetPullRequestDiff_DiffTooLarge(t *testing.T) {
	t.Parallel()

	client := newHandlerClient(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, StatusDiffTooLarge, errorEnvelope("Diff too large to generate"))
	})

	_, err := client.GetPullRequestDiff(t.Context(), testWorkspace, testRepoSlug, testPRID,
		DiffOptions{Path: "", Context: 0, DiffURL: ""})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !IsDiffTooLarge(err) {
		t.Errorf("IsDiffTooLarge(%v) = false, want true", err)
	}
	if IsNotFound(err) || IsRateLimited(err) {
		t.Errorf("555 must not read as another status: %v", err)
	}
	if !strings.Contains(err.Error(), "get diff acme/api-gateway#9") {
		t.Errorf("error %q does not carry the wrapper prefix", err.Error())
	}
	if !strings.Contains(err.Error(), "Diff too large to generate") {
		t.Errorf("error %q drops the server's own message", err.Error())
	}
}

func TestClient_GetPullRequestDiff_PassesANonUTF8BodyThrough(t *testing.T) {
	t.Parallel()

	// Bitbucket returns the diff in whatever encoding the repository's files
	// use and does not decode it to Unicode, so the bytes must survive intact.
	latin1 := "--- a/README\n+++ b/README\n@@ -1 +1 @@\n-caf\xe9\n+caf\xe9s\n"
	client := newHandlerClient(t, func(w http.ResponseWriter, _ *http.Request) {
		writeText(w, http.StatusOK, latin1)
	})

	diff, err := client.GetPullRequestDiff(t.Context(), testWorkspace, testRepoSlug, testPRID,
		DiffOptions{Path: "", Context: 0, DiffURL: ""})
	if err != nil {
		t.Fatalf("GetPullRequestDiff: %v", err)
	}

	testkit.AssertEqual(t, "diff", diff, latin1)
	if utf8.ValidString(diff) {
		t.Error("the fixture is not the non-UTF-8 case it claims to be")
	}
}

func TestCompareDiffStat_RequestShape(t *testing.T) {
	t.Parallel()

	client, recorded := newRecordingClient(t, bearerOpts(), testkit.StubResponse{
		Status: http.StatusOK,
		Body:   `{"pagelen": 500, "values": [` + diffStatEntryJSON + `]}`,
	})
	entries, err := client.CompareDiffStat(t.Context(), testWorkspace, testRepoSlug,
		"06fd23caff6b", "3c39f5b55d7f", []string{testDiffFilePath, "README.md"})
	if err != nil {
		t.Fatalf("CompareDiffStat() error = %v", err)
	}

	testkit.AssertEqual(t, "method", recorded.Method, http.MethodGet)
	testkit.AssertEqual(t, "path", recorded.Path,
		"/repositories/acme/api-gateway/diffstat/06fd23caff6b..3c39f5b55d7f")
	testkit.AssertEqual(t, "topic", recorded.Query.Get("topic"), "false")
	testkit.AssertSliceEqual(t, "paths", recorded.Query["path"], []string{testDiffFilePath, "README.md"})
	testkit.AssertEqual(t, "entries", len(entries), 1)
	testkit.AssertEqual(t, "entry path", entries[0].Path(), testDiffFilePath)
}

func TestCompareDiffStat_DropsAnOversizedPathFilter(t *testing.T) {
	t.Parallel()

	client, recorded := newRecordingClient(t, bearerOpts(), testkit.StubResponse{
		Status: http.StatusOK, Body: `{"values": []}`,
	})
	paths := make([]string, maxDiffStatPaths+1)
	for i := range paths {
		paths[i] = "file" + strconv.Itoa(i) + ".go"
	}
	if _, err := client.CompareDiffStat(t.Context(), testWorkspace, testRepoSlug, "aaaaaaa", "bbbbbbb", paths); err != nil {
		t.Fatalf("CompareDiffStat() error = %v", err)
	}
	testkit.AssertEqual(t, "no path filter", len(recorded.Query["path"]), 0)
}

func TestCompareDiffStat_MissingCommitIsNotFound(t *testing.T) {
	t.Parallel()

	client, _ := newRecordingClient(t, bearerOpts(), testkit.StubResponse{
		Status: http.StatusNotFound,
		Body:   `{"type": "error", "error": {"message": "Commit not found"}}`,
	})
	_, err := client.CompareDiffStat(t.Context(), testWorkspace, testRepoSlug, "aaaaaaa", "bbbbbbb", nil)
	if !IsNotFound(err) {
		t.Errorf("CompareDiffStat() error = %v, want a 404", err)
	}
}
