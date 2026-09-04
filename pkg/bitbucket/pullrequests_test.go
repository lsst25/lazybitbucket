package bitbucket

import (
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// prListEnvelope is a pull request collection page as returned with
// fields=+values.reviewers,+values.participants — the projection that gives the
// list pane its approval column.
const prListEnvelope = `{
  "pagelen": 50,
  "page": 1,
  "size": 1,
  "values": [
    {
      "type": "pullrequest",
      "id": 101,
      "title": "Add rate limiting to the gateway",
      "description": "## Why\n\nProtect the upstreams.\n",
      "state": "OPEN",
      "draft": false,
      "queued": false,
      "close_source_branch": true,
      "comment_count": 3,
      "task_count": 0,
      "reason": "",
      "merge_commit": null,
      "closed_by": null,
      "created_on": "2026-09-01T09:12:33.123456+00:00",
      "updated_on": "2026-09-03T16:23:55.597480+00:00",
      "author": {
        "type": "user", "uuid": "{e5743438-cc31-49a0-96b7-9febd7b946f4}",
        "account_id": "5f1e9a4c0e1b2c0068ab1234", "display_name": "Ada Lovelace", "nickname": "ada",
        "links": {"self": {"href": "https://api.bitbucket.org/2.0/users/x"},
                  "html": {"href": "https://bitbucket.org/x/"},
                  "avatar": {"href": "https://avatar.example/128"}}
      },
      "source": {
        "branch": {"name": "feature/rate-limit"},
        "commit": {"type": "commit", "hash": "d8e3c5fdfc16",
                   "links": {"self": {"href": "https://api.example/c"}, "html": {"href": "https://bitbucket.org/c"}}},
        "repository": {"type": "repository", "uuid": "{85dc8e4b-1111-2222-3333-444455556666}",
                       "name": "api-gateway", "full_name": "acme/api-gateway",
                       "links": {"self": {"href": "https://api.example/r"},
                                 "html": {"href": "https://bitbucket.org/acme/api-gateway"},
                                 "avatar": {"href": "https://bytebucket.org/ravatar/x"}}}
      },
      "destination": {
        "branch": {"name": "main"},
        "commit": {"type": "commit", "hash": "ef5690e8743c",
                   "links": {"self": {"href": "https://api.example/c"}, "html": {"href": "https://bitbucket.org/c"}}},
        "repository": {"type": "repository", "uuid": "{85dc8e4b-1111-2222-3333-444455556666}",
                       "name": "api-gateway", "full_name": "acme/api-gateway",
                       "links": {"self": {"href": "https://api.example/r"},
                                 "html": {"href": "https://bitbucket.org/acme/api-gateway"},
                                 "avatar": {"href": "https://bytebucket.org/ravatar/x"}}}
      },
      "summary": {"type": "rendered", "raw": "## Why\n\nProtect the upstreams.\n",
                  "markup": "markdown", "html": "<h2>Why</h2>"},
      "reviewers": [
        {"type": "user", "uuid": "{b6b5d0ff-32d1-4e3c-816a-9253437180ca}",
         "display_name": "Grace Hopper", "nickname": "grace",
         "links": {"self": {"href": "s"}, "html": {"href": "h"}, "avatar": {"href": "a"}}},
        {"type": "user", "uuid": "{781a664d-34db-4bc7-9c0d-bd3716d0d1de}",
         "display_name": "Linus Torvalds", "nickname": "linus",
         "links": {"self": {"href": "s"}, "html": {"href": "h"}, "avatar": {"href": "a"}}}
      ],
      "participants": [
        {"type": "participant", "role": "REVIEWER", "approved": true, "state": "approved",
         "participated_on": "2026-09-02T13:14:59.892545+00:00",
         "user": {"type": "user", "uuid": "{b6b5d0ff-32d1-4e3c-816a-9253437180ca}",
                  "display_name": "Grace Hopper", "nickname": "grace",
                  "links": {"self": {"href": "s"}, "html": {"href": "h"}, "avatar": {"href": "a"}}}},
        {"type": "participant", "role": "REVIEWER", "approved": false, "state": null,
         "participated_on": null,
         "user": {"type": "user", "uuid": "{781a664d-34db-4bc7-9c0d-bd3716d0d1de}",
                  "display_name": "Linus Torvalds", "nickname": "linus",
                  "links": {"self": {"href": "s"}, "html": {"href": "h"}, "avatar": {"href": "a"}}}}
      ],
      "links": {
        "self": {"href": "https://api.bitbucket.org/2.0/repositories/acme/api-gateway/pullrequests/101"},
        "html": {"href": "https://bitbucket.org/acme/api-gateway/pull-requests/101"},
        "commits": {"href": "https://api.example/commits"},
        "approve": {"href": "https://api.example/approve"},
        "request-changes": {"href": "https://api.example/request-changes"},
        "diff": {"href": "https://api.example/diff"},
        "diffstat": {"href": "https://api.example/diffstat"},
        "comments": {"href": "https://api.example/comments"},
        "activity": {"href": "https://api.example/activity"},
        "merge": {"href": "https://api.example/merge"},
        "decline": {"href": "https://api.example/decline"},
        "statuses": {"href": "https://api.example/statuses"}
      }
    }
  ]
}`

// prDetail is a declined pull request, the state that carries closed_by and a
// non-empty reason.
const prDetail = `{
  "type": "pullrequest",
  "id": 95,
  "title": "Drop the legacy shim",
  "description": "Superseded.",
  "state": "DECLINED",
  "draft": false,
  "close_source_branch": false,
  "reason": "Merged with another PR",
  "merge_commit": null,
  "closed_by": {"type": "user", "uuid": "{781a664d-34db-4bc7-9c0d-bd3716d0d1de}",
                "display_name": "Linus Torvalds", "nickname": "linus",
                "links": {"self": {"href": "s"}, "html": {"href": "h"}, "avatar": {"href": "a"}}},
  "created_on": "2026-08-20T07:27:00.000001+00:00",
  "updated_on": "2026-09-03T14:37:36.396218+00:00",
  "author": {"type": "user", "uuid": "{e5743438-cc31-49a0-96b7-9febd7b946f4}",
             "display_name": "Ada Lovelace", "nickname": "ada",
             "links": {"self": {"href": "s"}, "html": {"href": "h"}, "avatar": {"href": "a"}}},
  "source": {"branch": {"name": "chore/drop-shim"}, "commit": {"type": "commit", "hash": "aaaaaaaaaaaa"},
             "repository": {"type": "repository", "name": "api-gateway", "full_name": "acme/api-gateway"}},
  "destination": {"branch": {"name": "main"}, "commit": {"type": "commit", "hash": "bbbbbbbbbbbb"},
                  "repository": {"type": "repository", "name": "api-gateway", "full_name": "acme/api-gateway"}},
  "summary": {"type": "rendered", "raw": "Superseded.", "markup": "markdown", "html": "<p>Superseded.</p>"},
  "reviewers": [],
  "participants": [],
  "links": {"html": {"href": "https://bitbucket.org/acme/api-gateway/pull-requests/95"}}
}`

const invalidPageLenError = `{"type":"error","error":{"message":"Invalid pagelen"}}`

func TestListPullRequests_DefaultRequestShape(t *testing.T) {
	t.Parallel()

	client, recorded := newRecordingClient(t, bearerOpts(),
		testkit.StubResponse{Status: http.StatusOK, Body: prListEnvelope})

	if _, err := client.ListPullRequests(t.Context(), testWorkspace, testRepoSlug,
		ListPullRequestsOptions{States: nil, Query: "", Sort: "", MaxItems: 0, Fields: ""}); err != nil {
		t.Fatalf("ListPullRequests() error = %v", err)
	}

	testkit.AssertEqual(t, "method", recorded.Method, http.MethodGet)
	testkit.AssertEqual(t, "path", recorded.Path, testPullRequestPath)
	testkit.AssertEqual(t, paramPageLen, recorded.Query.Get(paramPageLen), "50")
	testkit.AssertEqual(t, paramSort, recorded.Query.Get(paramSort), sortUpdatedDesc)
	testkit.AssertSliceEqual(t, paramState, recorded.Query[paramState], []string{"OPEN"})
	// A literal "+" in the query would have decoded to a space here, so this
	// also pins the mandatory %2B encoding.
	testkit.AssertEqual(t, paramFields, recorded.Query.Get(paramFields), defaultPullRequestFields)
}

func TestListPullRequests_QueryOptions(t *testing.T) {
	t.Parallel()

	client, recorded := newRecordingClient(t, bearerOpts(),
		testkit.StubResponse{Status: http.StatusOK, Body: prListEnvelope})

	opts := ListPullRequestsOptions{
		States:   []PullRequestState{PullRequestOpen, PullRequestMerged},
		Query:    `author.uuid="{e5743438}"`,
		Sort:     "-created_on",
		MaxItems: 20,
		Fields:   "",
	}
	if _, err := client.ListPullRequests(t.Context(), testWorkspace, testRepoSlug, opts); err != nil {
		t.Fatalf("ListPullRequests() error = %v", err)
	}

	testkit.AssertSliceEqual(t, paramState, recorded.Query[paramState], []string{"OPEN", "MERGED"})
	testkit.AssertEqual(t, paramSort, recorded.Query.Get(paramSort), "-created_on")
	testkit.AssertEqual(t, paramPageLen, recorded.Query.Get(paramPageLen), "20")
	testkit.AssertEqual(t, "q", recorded.Query.Get("q"), `author.uuid="{e5743438}"`)
}

func TestListPullRequests_FieldsSentinelSuppressesProjection(t *testing.T) {
	t.Parallel()

	client, recorded := newRecordingClient(t, bearerOpts(),
		testkit.StubResponse{Status: http.StatusOK, Body: prListEnvelope})

	if _, err := client.ListPullRequests(t.Context(), testWorkspace, testRepoSlug,
		ListPullRequestsOptions{States: nil, Query: "", Sort: "", MaxItems: 0, Fields: noFields}); err != nil {
		t.Fatalf("ListPullRequests() error = %v", err)
	}

	if _, ok := recorded.Query[paramFields]; ok {
		t.Errorf("fields parameter present = %q, want absent", recorded.Query.Get(paramFields))
	}
}

func TestListPullRequests_ParsesListItem(t *testing.T) {
	t.Parallel()

	client, _ := newRecordingClient(t, bearerOpts(),
		testkit.StubResponse{Status: http.StatusOK, Body: prListEnvelope})

	prs, err := client.ListPullRequests(t.Context(), testWorkspace, testRepoSlug,
		ListPullRequestsOptions{States: nil, Query: "", Sort: "", MaxItems: 0, Fields: ""})
	if err != nil {
		t.Fatalf("ListPullRequests() error = %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("pull request count = %d, want 1", len(prs))
	}

	pr := prs[0]
	testkit.AssertEqual(t, "ID", pr.ID, 101)
	testkit.AssertEqual(t, "Title", pr.Title, "Add rate limiting to the gateway")
	testkit.AssertEqual(t, "State", pr.State, PullRequestOpen)
	testkit.AssertEqual(t, "Draft", pr.Draft, false)
	testkit.AssertEqual(t, "CommentCount", pr.CommentCount, 3)
	testkit.AssertEqual(t, "Author", pr.Author.Name(), "Ada Lovelace")
	testkit.AssertEqual(t, "SourceBranch()", pr.SourceBranch(), "feature/rate-limit")
	testkit.AssertEqual(t, "DestinationBranch()", pr.DestinationBranch(), "main")
	testkit.AssertEqual(t, "RepoFullName()", pr.RepoFullName(), testRepoFull)
	testkit.AssertEqual(t, "RepoSlug()", pr.RepoSlug(), testRepoSlug)
	testkit.AssertEqual(t, "Workspace()", pr.Workspace(), testWorkspace)
	testkit.AssertEqual(t, "Key()", pr.Key(), "acme/api-gateway#101")
	testkit.AssertEqual(t, "HTMLURL()", pr.HTMLURL(), "https://bitbucket.org/acme/api-gateway/pull-requests/101")
	testkit.AssertEqual(t, "HasReviewData()", pr.HasReviewData(), true)
	testkit.AssertEqual(t, "ClosedBy", pr.ClosedBy, (*Account)(nil))
	testkit.AssertEqual(t, "MergeCommit", pr.MergeCommit, (*CommitRef)(nil))

	summary := pr.ReviewSummary()
	testkit.AssertEqual(t, "Reviewers", summary.Reviewers, 2)
	testkit.AssertEqual(t, "Approved", summary.Approved, 1)
	testkit.AssertEqual(t, "Pending", summary.Pending, 1)
	testkit.AssertEqual(t, "IsReviewer(grace)", pr.IsReviewer(testReviewerUUID), true)
	testkit.AssertEqual(t, "IsAuthor(ada)", pr.IsAuthor(testAuthorUUID), true)
}

func TestListPullRequests_RetriesWithoutFieldsAfterBadRequest(t *testing.T) {
	t.Parallel()

	client, recorded := newSequenceClient(t, bearerOpts(),
		testkit.StubResponse{Status: http.StatusBadRequest, Body: invalidPageLenError},
		testkit.StubResponse{Status: http.StatusOK, Body: prListEnvelope},
	)

	prs, err := client.ListPullRequests(t.Context(), testWorkspace, testRepoSlug,
		ListPullRequestsOptions{States: nil, Query: "", Sort: "", MaxItems: 0, Fields: ""})
	if err != nil {
		t.Fatalf("ListPullRequests() error = %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("pull request count = %d, want 1", len(prs))
	}

	requests := *recorded
	if len(requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(requests))
	}
	testkit.AssertEqual(t, "first attempt fields", requests[0].Query.Get(paramFields), defaultPullRequestFields)
	if _, ok := requests[1].Query[paramFields]; ok {
		t.Errorf("retry still sent fields = %q, want absent", requests[1].Query.Get(paramFields))
	}
	testkit.AssertSliceEqual(t, "retry state", requests[1].Query[paramState], []string{"OPEN"})
}

func TestListPullRequests_DoesNotRetryWhenFieldsWereSuppressed(t *testing.T) {
	t.Parallel()

	client, recorded := newSequenceClient(t, bearerOpts(),
		testkit.StubResponse{Status: http.StatusBadRequest, Body: invalidPageLenError})

	_, err := client.ListPullRequests(t.Context(), testWorkspace, testRepoSlug,
		ListPullRequestsOptions{States: nil, Query: "", Sort: "", MaxItems: 0, Fields: noFields})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if len(*recorded) != 1 {
		t.Fatalf("request count = %d, want 1", len(*recorded))
	}
}

func TestListPullRequests_DoesNotRetryOtherStatuses(t *testing.T) {
	t.Parallel()

	client, recorded := newSequenceClient(t, bearerOpts(),
		testkit.StubResponse{Status: http.StatusForbidden, Body: `{"type":"error","error":{"message":"Denied"}}`})

	_, err := client.ListPullRequests(t.Context(), testWorkspace, testRepoSlug,
		ListPullRequestsOptions{States: nil, Query: "", Sort: "", MaxItems: 0, Fields: ""})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !IsForbidden(err) {
		t.Errorf("IsForbidden(%v) = false, want true", err)
	}
	if len(*recorded) != 1 {
		t.Fatalf("request count = %d, want 1", len(*recorded))
	}
}

func TestListPullRequests_DoesNotRetryBadRequestOnLaterPage(t *testing.T) {
	t.Parallel()

	var requests atomic.Int64
	client := newHandlerClient(t, func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.Header().Set("Content-Type", contentTypeJSON)
			_, _ = w.Write(selfNextPage(t, r, "http", testPullRequestPath, nil))
			return
		}
		writeJSON(t, w, http.StatusBadRequest, invalidPageLenError)
	})

	_, err := client.ListPullRequests(t.Context(), testWorkspace, testRepoSlug,
		ListPullRequestsOptions{States: nil, Query: "", Sort: "", MaxItems: 0, Fields: ""})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Invalid pagelen") {
		t.Errorf("error %q does not carry the server message", err.Error())
	}
	testkit.AssertEqual(t, "requests made", requests.Load(), int64(2))
}

func TestGetPullRequest_RequestShapeAndParsing(t *testing.T) {
	t.Parallel()

	client, recorded := newRecordingClient(t, bearerOpts(),
		testkit.StubResponse{Status: http.StatusOK, Body: prDetail})

	pr, err := client.GetPullRequest(t.Context(), testWorkspace, testRepoSlug, 95)
	if err != nil {
		t.Fatalf("GetPullRequest() error = %v", err)
	}

	testkit.AssertEqual(t, "path", recorded.Path, testPullRequestPath+"/95")
	if len(recorded.Query) != 0 {
		t.Errorf("query = %v, want empty", recorded.Query)
	}
	testkit.AssertEqual(t, "State", pr.State, PullRequestDeclined)
	testkit.AssertEqual(t, "Reason", pr.Reason, "Merged with another PR")
	testkit.AssertEqual(t, "Key()", pr.Key(), "acme/api-gateway#95")
	testkit.AssertEqual(t, "HasReviewData()", pr.HasReviewData(), true)
	if pr.ClosedBy == nil {
		t.Fatal("ClosedBy = nil, want the decliner")
	}
	testkit.AssertEqual(t, "ClosedBy", pr.ClosedBy.Name(), "Linus Torvalds")
}
