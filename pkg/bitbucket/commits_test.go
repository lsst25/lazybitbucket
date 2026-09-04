package bitbucket

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// commitJSON is one entry of GET .../pullrequests/{id}/commits, from
// phaseA/api-errors-users-misc.md §3: the hash is the full 40 chars, the date
// carries no fractional seconds, and the message keeps its trailing newline.
const commitJSON = `{
  "type": "commit",
  "hash": "06fd23caff6b0a76ce2f42c983988394dca1f17f",
  "date": "2026-08-21T10:39:02+00:00",
  "message": "RND-14515 add media tags\n\nTags are stored on the theme, not the asset.\n",
  "summary": {
    "type": "rendered",
    "raw": "RND-14515 add media tags\n\nTags are stored on the theme, not the asset.\n",
    "markup": "markdown",
    "html": "<p>RND-14515 add media tags</p>"
  },
  "author": {
    "type": "author",
    "raw": "Yurii Tatar <yuri@expivi.com>",
    "user": {
      "type": "user",
      "uuid": "{e5743438-cc31-49a0-96b7-9febd7b946f4}",
      "display_name": "Yurii Tatar",
      "nickname": "yurii",
      "account_id": "712020:e6359374-4156-4475-9f14-003f4b09cf83",
      "links": {"self": {"href": "https://api.bitbucket.org/2.0/users/x"}}
    }
  },
  "parents": [
    {
      "type": "commit",
      "hash": "3c39f5b55d7f61e4a9b2d1783a207b16482bf8e5",
      "links": {"self": {"href": "https://api.bitbucket.org/2.0/p"}, "html": {"href": "https://bitbucket.org/p"}}
    }
  ],
  "repository": {
    "type": "repository",
    "full_name": "acme/api-gateway",
    "name": "API Gateway",
    "uuid": "{85dc8e4b-811d-48af-b2f9-6468270ae2fb}",
    "links": {"self": {"href": "https://api.bitbucket.org/2.0/repositories/acme/api-gateway"}}
  },
  "links": {
    "self": {"href": "https://api.bitbucket.org/2.0/repositories/acme/api-gateway/commit/06fd23caff6b"},
    "html": {"href": "https://bitbucket.org/acme/api-gateway/commits/06fd23caff6b"},
    "diff": {"href": "https://api.bitbucket.org/2.0/repositories/acme/api-gateway/diff/06fd23caff6b"},
    "patch": {"href": "https://api.bitbucket.org/2.0/repositories/acme/api-gateway/patch/06fd23caff6b"},
    "comments": {"href": "https://api.bitbucket.org/2.0/x/comments"},
    "statuses": {"href": "https://api.bitbucket.org/2.0/x/statuses"}
  }
}`

// unmatchedAuthorCommitJSON is a commit whose git ident matches no Bitbucket
// account — the API sends author.user as null, which is the common case on an
// imported or bot-authored commit.
const unmatchedAuthorCommitJSON = `{
  "type": "commit",
  "hash": "9f2b1c4d",
  "date": "2026-08-20T09:00:00+00:00",
  "message": "chore: bump deps",
  "author": {"type": "author", "raw": "Release Bot <bot@example.com>", "user": null},
  "parents": [],
  "links": {"html": {"href": "https://bitbucket.org/acme/api-gateway/commits/9f2b1c4d"}}
}`

func TestCommit_Unmarshal(t *testing.T) {
	t.Parallel()

	var commit Commit
	if err := json.Unmarshal([]byte(commitJSON), &commit); err != nil {
		t.Fatalf("unmarshal commit: %v", err)
	}

	testkit.AssertEqual(t, "Type", commit.Type, "commit")
	testkit.AssertEqual(t, "Hash", commit.Hash, "06fd23caff6b0a76ce2f42c983988394dca1f17f")
	testkit.AssertEqual(t, "Date", commit.Date.UTC().Format(timestampLayout), "2026-08-21T10:39:02.000000000")
	testkit.AssertEqual(t, "ShortHash", commit.ShortHash(), "06fd23c")
	testkit.AssertEqual(t, "Subject", commit.Subject(), "RND-14515 add media tags")
	testkit.AssertEqual(t, "AuthorName", commit.AuthorName(), "Yurii Tatar")
	testkit.AssertEqual(t, "Author.Raw", commit.Author.Raw, "Yurii Tatar <yuri@expivi.com>")
	testkit.AssertEqual(t, "Summary.Markup", commit.Summary.Markup, "markdown")
	testkit.AssertEqual(t, "HTMLURL", commit.HTMLURL(),
		"https://bitbucket.org/acme/api-gateway/commits/06fd23caff6b")
	testkit.AssertEqual(t, "parents", len(commit.Parents), 1)
	testkit.AssertEqual(t, "parent hash", commit.Parents[0].Hash, "3c39f5b55d7f61e4a9b2d1783a207b16482bf8e5")
	testkit.AssertEqual(t, "Links.Statuses", commit.Links.Statuses.Href, "https://api.bitbucket.org/2.0/x/statuses")

	if commit.Repository == nil {
		t.Fatal("Repository is nil")
	}
	testkit.AssertEqual(t, "Repository.Slug", commit.Repository.Slug(), testRepoSlug)

	// The message keeps its trailing newline: the subject must not.
	if !SameCommit(commit.Hash, "06fd23caff6b") {
		t.Error("SameCommit does not match the 12-char abbreviation of the same commit")
	}
}

func TestCommit_AuthorNameFallsBackToTheGitIdent(t *testing.T) {
	t.Parallel()

	var commit Commit
	if err := json.Unmarshal([]byte(unmatchedAuthorCommitJSON), &commit); err != nil {
		t.Fatalf("unmarshal commit: %v", err)
	}

	if commit.Author.User != nil {
		t.Fatal("Author.User should be nil for an unmatched ident")
	}
	testkit.AssertEqual(t, "AuthorName", commit.AuthorName(), "Release Bot")
	testkit.AssertEqual(t, "ShortHash", commit.ShortHash(), "9f2b1c4")
	testkit.AssertEqual(t, "Subject", commit.Subject(), "chore: bump deps")
}

func TestCommit_HelpersOnDegenerateValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		commit     Commit
		wantShort  string
		wantSubj   string
		wantAuthor string
	}{
		{
			name:       "empty commit",
			commit:     Commit{},
			wantShort:  "",
			wantSubj:   "",
			wantAuthor: "",
		},
		{
			name:       "hash shorter than the abbreviation",
			commit:     Commit{Hash: "abc12"},
			wantShort:  "abc12",
			wantSubj:   "",
			wantAuthor: "",
		},
		{
			name:       "ident without an email",
			commit:     Commit{Author: CommitAuthor{Raw: "buildbot"}},
			wantShort:  "",
			wantSubj:   "",
			wantAuthor: "buildbot",
		},
		{
			name:       "ident that is only an email",
			commit:     Commit{Author: CommitAuthor{Raw: "<bot@example.com>"}},
			wantShort:  "",
			wantSubj:   "",
			wantAuthor: "<bot@example.com>",
		},
		{
			name:       "account without a display name",
			commit:     Commit{Author: CommitAuthor{Raw: "x <x@y>", User: &Account{UUID: testOtherUUID}}},
			wantShort:  "",
			wantSubj:   "",
			wantAuthor: testOtherUUID,
		},
		{
			name:       "subject padded with whitespace",
			commit:     Commit{Message: "  fix the thing  \n\nbody\n"},
			wantShort:  "",
			wantSubj:   "fix the thing",
			wantAuthor: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			testkit.AssertEqual(t, "ShortHash", tt.commit.ShortHash(), tt.wantShort)
			testkit.AssertEqual(t, "Subject", tt.commit.Subject(), tt.wantSubj)
			testkit.AssertEqual(t, "AuthorName", tt.commit.AuthorName(), tt.wantAuthor)
		})
	}
}

func TestClient_ListPullRequestCommits_FollowsTheOpaqueIteratorToken(t *testing.T) {
	t.Parallel()

	// The commits collection is iterator-paginated: no size, and next carries
	// an opaque token instead of a page number. Nothing may be rebuilt from it.
	rec := &recorder{}
	client := newHandlerClient(t, func(w http.ResponseWriter, r *http.Request) {
		rec.add(r)

		if r.URL.Query().Get("page") == "E8Ux" {
			writeJSON(t, w, http.StatusOK, `{"pagelen": 100, "values": [`+unmatchedAuthorCommitJSON+`]}`)
			return
		}

		next, err := json.Marshal("http://" + r.Host + r.URL.Path + "?pagelen=100&page=E8Ux")
		if err != nil {
			t.Errorf("marshal next link: %v", err)
			return
		}
		writeJSON(t, w, http.StatusOK,
			`{"pagelen": 100, "next": `+string(next)+`, "values": [`+commitJSON+`]}`)
	})

	commits, err := client.ListPullRequestCommits(t.Context(), testWorkspace, testRepoSlug, testPRID,
		ListOptions{MaxItems: 0})
	if err != nil {
		t.Fatalf("ListPullRequestCommits: %v", err)
	}

	testkit.AssertEqual(t, "commits", len(commits), 2)
	testkit.AssertEqual(t, "first subject", commits[0].Subject(), "RND-14515 add media tags")
	testkit.AssertEqual(t, "second author", commits[1].AuthorName(), "Release Bot")

	requests := rec.snapshot()
	testkit.AssertEqual(t, "requests", len(requests), 2)
	testkit.AssertEqual(t, "path", requests[0].Path, testCommitsPath)
	testkit.AssertEqual(t, "pagelen", requests[0].Query.Get(paramPageLen), "100")
	testkit.AssertEqual(t, "first page token", requests[0].Query.Get("page"), "")
	testkit.AssertEqual(t, "second page token", requests[1].Query.Get("page"), "E8Ux")
	testkit.AssertEqual(t, "accept", requests[0].Header.Get("Accept"), contentTypeJSON)
}

func TestClient_ListPullRequestCommits_MaxItemsBoundsPageLenAndResult(t *testing.T) {
	t.Parallel()

	client, recorded := newRecordingClient(t, bearerOpts(), testkit.StubResponse{
		Status: http.StatusOK,
		Body:   `{"pagelen": 2, "values": [` + commitJSON + `,` + unmatchedAuthorCommitJSON + `]}`,
	})

	commits, err := client.ListPullRequestCommits(t.Context(), testWorkspace, testRepoSlug, testPRID,
		ListOptions{MaxItems: 1})
	if err != nil {
		t.Fatalf("ListPullRequestCommits: %v", err)
	}

	testkit.AssertEqual(t, "commits", len(commits), 1)
	testkit.AssertEqual(t, "pagelen", recorded.Query.Get(paramPageLen), "1")
}

func TestClient_ListPullRequestCommits_EmptyWhenTheSourceBranchIsGone(t *testing.T) {
	t.Parallel()

	// A merged pull request whose source branch was deleted answers 200 with an
	// empty collection, not an error.
	client, _ := newRecordingClient(t, bearerOpts(),
		testkit.StubResponse{Status: http.StatusOK, Body: emptyPage})

	commits, err := client.ListPullRequestCommits(t.Context(), testWorkspace, testRepoSlug, testPRID,
		ListOptions{MaxItems: 0})
	if err != nil {
		t.Fatalf("ListPullRequestCommits: %v", err)
	}
	testkit.AssertEqual(t, "commits", len(commits), 0)
}
