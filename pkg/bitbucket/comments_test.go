package bitbucket

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// generalCommentJSON is a pull-request-level comment: no inline block, no
// parent, and no code link. From phaseA/api-rest-docs.md §6.8.
const generalCommentJSON = `{
  "id": 835275766,
  "created_on": "2026-08-04T12:13:27.150106+00:00",
  "updated_on": "2026-08-04T12:20:01.000000+00:00",
  "content": {"type": "rendered", "raw": "Global comment", "markup": "markdown", "html": "<p>Global comment</p>"},
  "user": {
    "type": "user",
    "uuid": "{e5743438-cc31-49a0-96b7-9febd7b946f4}",
    "display_name": "Yurii Tatar",
    "links": {"self": {"href": "https://api.bitbucket.org/2.0/users/x"}}
  },
  "deleted": false,
  "pending": false,
  "type": "pullrequest_comment",
  "links": {
    "self": {"href": "https://api.bitbucket.org/2.0/x/comments/835275766"},
    "html": {"href": "https://bitbucket.org/acme/api-gateway/pull-requests/9/_/diff#comment-835275766"}
  },
  "pullrequest": {"type": "pullrequest", "id": 9, "title": "Add media tags", "draft": false}
}`

// inlineCommentJSON is an inline comment on an added line: to is set, from is
// null, and links gains a code rel. resolution is the empty object Bitbucket
// emits on plenty of unresolved comments.
const inlineCommentJSON = `{
  "id": 822302793,
  "created_on": "2026-08-04T12:00:00.000000+00:00",
  "updated_on": "2026-08-04T12:05:00.000000+00:00",
  "content": {"type": "rendered", "raw": "This allocates on every call.", "markup": "markdown", "html": "<p>x</p>"},
  "user": {"type": "user", "uuid": "{b6b5d0ff-32d1-4e3c-816a-9253437180ca}", "display_name": "Reviewer"},
  "deleted": false,
  "pending": false,
  "type": "pullrequest_comment",
  "inline": {"from": null, "to": 383, "path": "pkg/tui/app.go", "start_from": null, "start_to": null},
  "resolution": {},
  "links": {
    "self": {"href": "https://api.bitbucket.org/2.0/x/comments/822302793"},
    "html": {"href": "https://bitbucket.org/acme/api-gateway/pull-requests/9/_/diff#comment-822302793"},
    "code": {"href": "https://bitbucket.org/acme/api-gateway/pull-requests/9/diff#chg-pkg/tui/app.go"}
  }
}`

// replyCommentJSON is a reply. It repeats the parent's inline block verbatim,
// so only parent tells a reply from a root.
const replyCommentJSON = `{
  "id": 822617705,
  "created_on": "2026-08-04T13:00:00.000000+00:00",
  "updated_on": "2026-08-04T13:00:00.000000+00:00",
  "content": {"type": "rendered", "raw": "Fixed in the next push.", "markup": "markdown", "html": "<p>y</p>"},
  "user": {"type": "user", "uuid": "{e5743438-cc31-49a0-96b7-9febd7b946f4}", "display_name": "Yurii Tatar"},
  "deleted": false,
  "pending": false,
  "type": "pullrequest_comment",
  "parent": {
    "id": 822302793,
    "links": {
      "self": {"href": "https://api.bitbucket.org/2.0/x/comments/822302793"},
      "html": {"href": "https://bitbucket.org/x#comment-822302793"}
    }
  },
  "inline": {"from": null, "to": 383, "path": "pkg/tui/app.go", "start_from": null, "start_to": null},
  "links": {"self": {"href": "https://api.bitbucket.org/2.0/x/comments/822617705"}}
}`

// resolvedCommentJSON carries a populated resolution — the shape a resolved
// thread has.
const resolvedCommentJSON = `{
  "id": 900000001,
  "created_on": "2026-08-05T08:00:00.000000+00:00",
  "updated_on": "2026-08-05T09:00:00.000000+00:00",
  "content": {"type": "rendered", "raw": "Rename this.", "markup": "markdown", "html": "<p>z</p>"},
  "user": {"type": "user", "uuid": "{b6b5d0ff-32d1-4e3c-816a-9253437180ca}", "display_name": "Reviewer"},
  "deleted": false,
  "pending": false,
  "type": "pullrequest_comment",
  "inline": {"from": 12, "to": null, "path": "pkg/tui/app.go", "start_from": 10, "start_to": null},
  "resolution": {
    "type": "pullrequest_comment_resolution",
    "created_on": "2026-08-05T10:00:00.000000+00:00",
    "user": {"type": "user", "uuid": "{e5743438-cc31-49a0-96b7-9febd7b946f4}", "display_name": "Yurii Tatar"}
  },
  "links": {"self": {"href": "https://api.bitbucket.org/2.0/x/comments/900000001"}}
}`

// fileLevelCommentJSON is anchored to a file and no line: every anchor field is
// null, which is the shape observed live.
const fileLevelCommentJSON = `{
  "id": 900000002,
  "created_on": "2026-08-05T11:00:00.000000+00:00",
  "updated_on": "2026-08-05T11:00:00.000000+00:00",
  "content": {"type": "rendered", "raw": "Whole file needs a doc comment.", "markup": "markdown"},
  "user": {"type": "user", "uuid": "{781a664d-34db-4bc7-9c0d-bd3716d0d1de}", "display_name": "Other"},
  "deleted": false,
  "pending": true,
  "type": "pullrequest_comment",
  "inline": {"from": null, "to": null, "path": "README.md", "start_from": null, "start_to": null},
  "links": {"self": {"href": "https://api.bitbucket.org/2.0/x/comments/900000002"}}
}`

func mustUnmarshalComment(t *testing.T, raw string) Comment {
	t.Helper()

	var comment Comment
	if err := json.Unmarshal([]byte(raw), &comment); err != nil {
		t.Fatalf("unmarshal comment: %v", err)
	}
	return comment
}

func TestComment_UnmarshalGeneral(t *testing.T) {
	t.Parallel()

	comment := mustUnmarshalComment(t, generalCommentJSON)

	testkit.AssertEqual(t, "Type", comment.Type, "pullrequest_comment")
	testkit.AssertEqual(t, "ID", comment.ID, int64(835275766))
	testkit.AssertEqual(t, "CreatedOn", comment.CreatedOn.UTC().Format(timestampLayout),
		"2026-08-04T12:13:27.150106000")
	testkit.AssertEqual(t, "Content.Raw", comment.Content.Raw, "Global comment")
	testkit.AssertEqual(t, "user", comment.User.Name(), "Yurii Tatar")
	testkit.AssertEqual(t, "IsInline", comment.IsInline(), false)
	testkit.AssertEqual(t, "Deleted", comment.Deleted, false)
	testkit.AssertEqual(t, "Pending", comment.Pending, false)
	testkit.AssertEqual(t, "HTMLURL", comment.HTMLURL(),
		"https://bitbucket.org/acme/api-gateway/pull-requests/9/_/diff#comment-835275766")
	if comment.Parent != nil {
		t.Error("Parent should be nil on a root comment")
	}
	if comment.Inline != nil {
		t.Error("Inline should be nil on a general comment")
	}
	testkit.AssertEqual(t, "Resolved", comment.Resolution.Resolved(), false)
	testkit.AssertEqual(t, "code link", comment.Links.Code.Href, "")
}

func TestComment_UnmarshalInline(t *testing.T) {
	t.Parallel()

	comment := mustUnmarshalComment(t, inlineCommentJSON)

	testkit.AssertEqual(t, "IsInline", comment.IsInline(), true)
	testkit.AssertEqual(t, "path", comment.Inline.Path, testDiffFilePath)
	if comment.Inline.From != nil {
		t.Error("From should be nil for a comment on an added line")
	}

	line, newSide, ok := comment.Inline.Line()
	testkit.AssertEqual(t, "line", line, 383)
	testkit.AssertEqual(t, "newSide", newSide, true)
	testkit.AssertEqual(t, "ok", ok, true)

	start, end, newSide, ok := comment.Inline.Range()
	testkit.AssertEqual(t, "range start", start, 383)
	testkit.AssertEqual(t, "range end", end, 383)
	testkit.AssertEqual(t, "range newSide", newSide, true)
	testkit.AssertEqual(t, "range ok", ok, true)

	// An empty resolution object is NOT a resolution.
	if comment.Resolution == nil {
		t.Fatal("Resolution should be a non-nil empty object")
	}
	testkit.AssertEqual(t, "Resolved", comment.Resolution.Resolved(), false)
	testkit.AssertEqual(t, "code link", comment.Links.Code.Href,
		"https://bitbucket.org/acme/api-gateway/pull-requests/9/diff#chg-pkg/tui/app.go")
}

func TestComment_UnmarshalReplyRepeatsTheAnchor(t *testing.T) {
	t.Parallel()

	comment := mustUnmarshalComment(t, replyCommentJSON)

	if comment.Parent == nil {
		t.Fatal("Parent should be set on a reply")
	}
	testkit.AssertEqual(t, "Parent.ID", comment.Parent.ID, int64(822302793))
	testkit.AssertEqual(t, "Parent self link", comment.Parent.Links.Self.Href,
		"https://api.bitbucket.org/2.0/x/comments/822302793")
	testkit.AssertEqual(t, "Parent html link", comment.Parent.Links.HTML.Href,
		"https://bitbucket.org/x#comment-822302793")
	// The reply repeats its parent's anchor, so IsInline alone cannot tell a
	// root from a reply.
	testkit.AssertEqual(t, "IsInline", comment.IsInline(), true)
}

func TestComment_UnmarshalResolvedMultiLineOldSide(t *testing.T) {
	t.Parallel()

	comment := mustUnmarshalComment(t, resolvedCommentJSON)

	line, newSide, ok := comment.Inline.Line()
	testkit.AssertEqual(t, "line", line, 12)
	testkit.AssertEqual(t, "newSide", newSide, false)
	testkit.AssertEqual(t, "ok", ok, true)

	start, end, newSide, ok := comment.Inline.Range()
	testkit.AssertEqual(t, "range start", start, 10)
	testkit.AssertEqual(t, "range end", end, 12)
	testkit.AssertEqual(t, "range newSide", newSide, false)
	testkit.AssertEqual(t, "range ok", ok, true)

	testkit.AssertEqual(t, "Resolved", comment.Resolution.Resolved(), true)
	testkit.AssertEqual(t, "resolver", comment.Resolution.User.Name(), "Yurii Tatar")
	testkit.AssertEqual(t, "resolved at", comment.Resolution.CreatedOn.UTC().Format(timestampLayout),
		"2026-08-05T10:00:00.000000000")
}

func TestComment_UnmarshalFileLevelAnchor(t *testing.T) {
	t.Parallel()

	comment := mustUnmarshalComment(t, fileLevelCommentJSON)

	testkit.AssertEqual(t, "IsInline", comment.IsInline(), true)
	testkit.AssertEqual(t, "Pending", comment.Pending, true)

	_, _, ok := comment.Inline.Line()
	testkit.AssertEqual(t, "Line ok", ok, false)

	_, _, _, rangeOK := comment.Inline.Range()
	testkit.AssertEqual(t, "Range ok", rangeOK, false)
}

func TestCommentInline_LineAndRange(t *testing.T) {
	t.Parallel()

	line := func(n int) *int { return &n }

	tests := []struct {
		name      string
		inline    CommentInline
		wantLine  int
		wantNew   bool
		wantOK    bool
		wantStart int
		wantEnd   int
	}{
		{
			name:     "new side only",
			inline:   CommentInline{Path: "a.go", To: line(20)},
			wantLine: 20, wantNew: true, wantOK: true, wantStart: 20, wantEnd: 20,
		},
		{
			name:     "old side only",
			inline:   CommentInline{Path: "a.go", From: line(7)},
			wantLine: 7, wantNew: false, wantOK: true, wantStart: 7, wantEnd: 7,
		},
		{
			name:     "both sides prefers the new one",
			inline:   CommentInline{Path: "a.go", From: line(7), To: line(9)},
			wantLine: 9, wantNew: true, wantOK: true, wantStart: 9, wantEnd: 9,
		},
		{
			name:     "multi-line on the new side",
			inline:   CommentInline{Path: "a.go", To: line(30), StartTo: line(25)},
			wantLine: 30, wantNew: true, wantOK: true, wantStart: 25, wantEnd: 30,
		},
		{
			name:     "start beyond the end is normalised",
			inline:   CommentInline{Path: "a.go", To: line(25), StartTo: line(30)},
			wantLine: 25, wantNew: true, wantOK: true, wantStart: 25, wantEnd: 30,
		},
		{
			name:     "start on the wrong side is ignored",
			inline:   CommentInline{Path: "a.go", To: line(40), StartFrom: line(2)},
			wantLine: 40, wantNew: true, wantOK: true, wantStart: 40, wantEnd: 40,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotLine, gotNew, gotOK := tt.inline.Line()
			testkit.AssertEqual(t, "line", gotLine, tt.wantLine)
			testkit.AssertEqual(t, "newSide", gotNew, tt.wantNew)
			testkit.AssertEqual(t, "ok", gotOK, tt.wantOK)

			start, end, rangeNew, rangeOK := tt.inline.Range()
			testkit.AssertEqual(t, "start", start, tt.wantStart)
			testkit.AssertEqual(t, "end", end, tt.wantEnd)
			testkit.AssertEqual(t, "range newSide", rangeNew, tt.wantNew)
			testkit.AssertEqual(t, "range ok", rangeOK, tt.wantOK)
		})
	}
}

func TestCommentResolution_Resolved(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		resolution *CommentResolution
		want       bool
	}{
		{name: "absent", resolution: nil, want: false},
		{name: "empty object", resolution: &CommentResolution{}, want: false},
		{name: "type only", resolution: &CommentResolution{Type: "resolution"}, want: true},
		{name: "user only", resolution: &CommentResolution{User: &Account{UUID: testAuthorUUID}}, want: true},
		{
			name:       "timestamp only",
			resolution: &CommentResolution{CreatedOn: time.Unix(1780000000, 0)},
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testkit.AssertEqual(t, "Resolved", tt.resolution.Resolved(), tt.want)
		})
	}
}

func TestClient_ListPullRequestComments_RequestShape(t *testing.T) {
	t.Parallel()

	client, recorded := newRecordingClient(t, bearerOpts(), testkit.StubResponse{
		Status: http.StatusOK,
		Body: `{"pagelen": 100, "values": [` +
			generalCommentJSON + `,` + inlineCommentJSON + `,` + replyCommentJSON + `]}`,
	})

	comments, err := client.ListPullRequestComments(t.Context(), testWorkspace, testRepoSlug, testPRID,
		ListOptions{MaxItems: 0})
	if err != nil {
		t.Fatalf("ListPullRequestComments: %v", err)
	}

	testkit.AssertEqual(t, "comments", len(comments), 3)
	testkit.AssertEqual(t, "path", recorded.Path, testCommentsPath)
	testkit.AssertEqual(t, "pagelen", recorded.Query.Get(paramPageLen), "100")
	testkit.AssertEqual(t, "sort", recorded.Query.Get(paramSort), "created_on")
}

func TestClient_ListPullRequestComments_MaxItemsBoundsPageLen(t *testing.T) {
	t.Parallel()

	client, recorded := newRecordingClient(t, bearerOpts(), testkit.StubResponse{
		Status: http.StatusOK,
		Body:   `{"pagelen": 2, "values": [` + generalCommentJSON + `,` + inlineCommentJSON + `]}`,
	})

	comments, err := client.ListPullRequestComments(t.Context(), testWorkspace, testRepoSlug, testPRID,
		ListOptions{MaxItems: 2})
	if err != nil {
		t.Fatalf("ListPullRequestComments: %v", err)
	}

	testkit.AssertEqual(t, "comments", len(comments), 2)
	testkit.AssertEqual(t, "pagelen", recorded.Query.Get(paramPageLen), "2")
}
