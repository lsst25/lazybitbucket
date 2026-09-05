package bitbucket

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// defaultReviewersFixture is the effective-default-reviewers envelope: the
// account is nested under "user", next to where the rule came from.
const defaultReviewersFixture = `{
  "pagelen": 100,
  "page": 1,
  "size": 2,
  "values": [
    {
      "type": "default_reviewer_and_type",
      "reviewer_type": "repository",
      "user": {"type": "user", "uuid": "{b6b5d0ff-32d1-4e3c-816a-9253437180ca}",
               "display_name": "Grace Hopper", "nickname": "grace"}
    },
    {
      "type": "default_reviewer_and_type",
      "reviewer_type": "project",
      "user": {"type": "user", "uuid": "{781a664d-34db-4bc7-9c0d-bd3716d0d1de}",
               "display_name": "Linus Torvalds", "nickname": "linus"}
    }
  ]
}`

// membersFixture is one page of the workspace membership collection.
const membersFixture = `{
  "pagelen": 100,
  "page": 1,
  "size": 1,
  "values": [
    {
      "type": "workspace_membership",
      "user": {"type": "user", "uuid": "{e5743438-cc31-49a0-96b7-9febd7b946f4}",
               "display_name": "Ada Lovelace", "nickname": "ada"},
      "workspace": {"type": "workspace", "slug": "acme", "name": "Acme"}
    }
  ]
}`

func TestListEffectiveDefaultReviewers(t *testing.T) {
	t.Parallel()

	client, recorded := newRecordingClient(t, bearerOpts(),
		testkit.StubResponse{Status: http.StatusOK, Body: defaultReviewersFixture})

	accounts, err := client.ListEffectiveDefaultReviewers(t.Context(), testWorkspace, testRepoSlug)
	if err != nil {
		t.Fatalf("ListEffectiveDefaultReviewers() error = %v", err)
	}

	testkit.AssertEqual(t, "method", recorded.Method, testMethod)
	testkit.AssertEqual(t, "path", recorded.Path, testDefaultReviewersPath)
	testkit.AssertEqual(t, paramPageLen, recorded.Query.Get(paramPageLen), strconv.Itoa(maxPageLenDefault))

	names := make([]string, 0, len(accounts))
	uuids := make([]string, 0, len(accounts))
	for _, account := range accounts {
		names = append(names, account.DisplayName)
		uuids = append(uuids, account.UUID)
	}
	testkit.AssertSliceEqual(t, "names", names, []string{"Grace Hopper", "Linus Torvalds"})
	testkit.AssertSliceEqual(t, "uuids", uuids, []string{testReviewerUUID, testOtherUUID})
}

// A 403 here is expected on repositories the token cannot inspect; the caller
// degrades to the reviewers it already knows, so the error has to be legible.
func TestListEffectiveDefaultReviewers_ForbiddenIsAnAPIError(t *testing.T) {
	t.Parallel()

	client, _ := newRecordingClient(t, bearerOpts(), testkit.StubResponse{
		Status: http.StatusForbidden,
		Body:   errorEnvelope("You do not have access to view the default reviewers"),
	})

	_, err := client.ListEffectiveDefaultReviewers(t.Context(), testWorkspace, testRepoSlug)
	if !IsForbidden(err) {
		t.Fatalf("error = %v, want a 403", err)
	}
	if !strings.Contains(err.Error(), "list default reviewers acme/api-gateway") {
		t.Errorf("error %q does not name the repository", err)
	}
}

func TestListWorkspaceMembers_UnwrapsTheMemberships(t *testing.T) {
	t.Parallel()

	client, recorded := newRecordingClient(t, bearerOpts(),
		testkit.StubResponse{Status: http.StatusOK, Body: membersFixture})

	accounts, err := client.ListWorkspaceMembers(t.Context(), testWorkspace, ListOptions{MaxItems: 0})
	if err != nil {
		t.Fatalf("ListWorkspaceMembers() error = %v", err)
	}

	testkit.AssertEqual(t, "path", recorded.Path, testMembersPath)
	testkit.AssertEqual(t, paramPageLen, recorded.Query.Get(paramPageLen), strconv.Itoa(maxPageLenDefault))

	if len(accounts) != 1 {
		t.Fatalf("accounts = %d, want 1", len(accounts))
	}
	testkit.AssertEqual(t, "DisplayName", accounts[0].DisplayName, "Ada Lovelace")
	testkit.AssertEqual(t, "UUID", accounts[0].UUID, testAuthorUUID)
}

func TestListWorkspaceMembers_FollowsNextLinks(t *testing.T) {
	t.Parallel()

	page := func(uuid, name string) string {
		return `{"pagelen": 1, "values": [{"type": "workspace_membership",` +
			`"user": {"type": "user", "uuid": "` + uuid + `", "display_name": "` + name + `"}}]}`
	}

	client, recorded := pagedServer(t, []string{
		page(testAuthorUUID, "Ada Lovelace"),
		page(testReviewerUUID, "Grace Hopper"),
		page(testOtherUUID, "Linus Torvalds"),
	})

	accounts, err := client.ListWorkspaceMembers(t.Context(), testWorkspace, ListOptions{MaxItems: 0})
	if err != nil {
		t.Fatalf("ListWorkspaceMembers() error = %v", err)
	}

	testkit.AssertEqual(t, "requests", len(*recorded), 3)

	names := make([]string, 0, len(accounts))
	for _, account := range accounts {
		names = append(names, account.DisplayName)
	}
	testkit.AssertSliceEqual(t, "names", names, []string{"Ada Lovelace", "Grace Hopper", "Linus Torvalds"})
}

// MaxItems bounds both the walk and the page size, so a picker that wants a
// handful of candidates does not pull a hundred.
func TestListWorkspaceMembers_MaxItemsBoundsTheWalk(t *testing.T) {
	t.Parallel()

	page := func(uuid string) string {
		return `{"pagelen": 1, "values": [{"type": "workspace_membership",` +
			`"user": {"type": "user", "uuid": "` + uuid + `", "display_name": "member"}}]}`
	}

	client, recorded := pagedServer(t, []string{
		page(testAuthorUUID), page(testReviewerUUID), page(testOtherUUID),
	})

	accounts, err := client.ListWorkspaceMembers(t.Context(), testWorkspace, ListOptions{MaxItems: 2})
	if err != nil {
		t.Fatalf("ListWorkspaceMembers() error = %v", err)
	}

	testkit.AssertEqual(t, "accounts", len(accounts), 2)
	testkit.AssertEqual(t, "requests", len(*recorded), 2)
	testkit.AssertEqual(t, paramPageLen, (*recorded)[0].Query.Get(paramPageLen), "2")
}
