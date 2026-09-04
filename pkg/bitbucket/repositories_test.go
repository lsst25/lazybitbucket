package bitbucket

import (
	"net/http"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

const repositoryFixture = `{
  "type": "repository",
  "uuid": "{85dc8e4b-1111-2222-3333-444455556666}",
  "slug": "sales-erp",
  "name": "Sales ERP",
  "full_name": "acme/sales-erp",
  "description": "Order intake",
  "scm": "git",
  "language": "php",
  "is_private": true,
  "size": 421337,
  "mainbranch": {"type": "branch", "name": "develop"},
  "project": {"type": "project", "uuid": "{p}", "key": "PLAT", "name": "Platform",
              "links": {"self": {"href": "s"}, "html": {"href": "h"}, "avatar": {"href": "a"}}},
  "workspace": {"type": "workspace", "uuid": "{w}", "slug": "acme", "name": "Acme Corp",
                "links": {"self": {"href": "s"}, "html": {"href": "h"}, "avatar": {"href": "a"}}},
  "created_on": "2017-10-16T08:32:04.844941+00:00",
  "updated_on": "2026-09-03T17:02:02.828459+00:00",
  "links": {
    "self":   {"href": "https://api.bitbucket.org/2.0/repositories/acme/sales-erp"},
    "html":   {"href": "https://bitbucket.org/acme/sales-erp"},
    "avatar": {"href": "https://bytebucket.org/ravatar/x"},
    "pullrequests": {"href": "https://api.example/pullrequests"},
    "commits": {"href": "https://api.example/commits"},
    "branches": {"href": "https://api.example/branches"},
    "clone": [
      {"name": "https", "href": "https://bitbucket.org/acme/sales-erp.git"},
      {"name": "ssh", "href": "git@bitbucket.org:acme/sales-erp.git"}
    ]
  }
}`

func TestListRepositories_RequestShape(t *testing.T) {
	t.Parallel()

	client, recorded := newRecordingClient(t, bearerOpts(),
		testkit.StubResponse{Status: http.StatusOK, Body: emptyPage})

	opts := ListRepositoriesOptions{Role: "contributor", Query: `project.key="PLAT"`, Sort: "", MaxItems: 0}
	if _, err := client.ListRepositories(t.Context(), testWorkspace, opts); err != nil {
		t.Fatalf("ListRepositories() error = %v", err)
	}

	testkit.AssertEqual(t, "method", recorded.Method, http.MethodGet)
	testkit.AssertEqual(t, "path", recorded.Path, "/repositories/acme")
	testkit.AssertEqual(t, paramPageLen, recorded.Query.Get(paramPageLen), "100")
	testkit.AssertEqual(t, paramSort, recorded.Query.Get(paramSort), sortUpdatedDesc)
	testkit.AssertEqual(t, "role", recorded.Query.Get("role"), "contributor")
	testkit.AssertEqual(t, "q", recorded.Query.Get("q"), `project.key="PLAT"`)
}

func TestListRepositories_OmitsEmptyFilters(t *testing.T) {
	t.Parallel()

	client, recorded := newRecordingClient(t, bearerOpts(),
		testkit.StubResponse{Status: http.StatusOK, Body: emptyPage})

	if _, err := client.ListRepositories(t.Context(), testWorkspace,
		ListRepositoriesOptions{Role: "", Query: "", Sort: "-name", MaxItems: 0}); err != nil {
		t.Fatalf("ListRepositories() error = %v", err)
	}

	testkit.AssertEqual(t, paramSort, recorded.Query.Get(paramSort), "-name")
	if _, ok := recorded.Query["role"]; ok {
		t.Error("role parameter present, want absent")
	}
	if _, ok := recorded.Query["q"]; ok {
		t.Error("q parameter present, want absent")
	}
}

func TestListRepositories_ParsesValues(t *testing.T) {
	t.Parallel()

	client, _ := newRecordingClient(t, bearerOpts(),
		testkit.StubResponse{Status: http.StatusOK, Body: `{"pagelen":100,"values":[` + repositoryFixture + `]}`})

	repos, err := client.ListRepositories(t.Context(), testWorkspace,
		ListRepositoriesOptions{Role: "", Query: "", Sort: "", MaxItems: 0})
	if err != nil {
		t.Fatalf("ListRepositories() error = %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("repository count = %d, want 1", len(repos))
	}

	// name is not the slug: "Sales ERP" vs "sales-erp".
	testkit.AssertEqual(t, "Slug", repos[0].Slug, "sales-erp")
	testkit.AssertEqual(t, "Name", repos[0].Name, "Sales ERP")
	testkit.AssertEqual(t, "MainBranchName()", repos[0].MainBranchName(), "develop")
}

func TestGetRepository_RequestShapeAndParsing(t *testing.T) {
	t.Parallel()

	client, recorded := newRecordingClient(t, bearerOpts(),
		testkit.StubResponse{Status: http.StatusOK, Body: repositoryFixture})

	repo, err := client.GetRepository(t.Context(), testWorkspace, "sales-erp")
	if err != nil {
		t.Fatalf("GetRepository() error = %v", err)
	}

	testkit.AssertEqual(t, "path", recorded.Path, "/repositories/acme/sales-erp")
	if len(recorded.Query) != 0 {
		t.Errorf("query = %v, want empty", recorded.Query)
	}
	testkit.AssertEqual(t, "FullName", repo.FullName, "acme/sales-erp")
	testkit.AssertEqual(t, "WorkspaceSlug()", repo.WorkspaceSlug(), testWorkspace)
	testkit.AssertEqual(t, "HTMLURL()", repo.HTMLURL(), "https://bitbucket.org/acme/sales-erp")
	testkit.AssertEqual(t, "clone links", len(repo.Links.Clone), 2)
}

func TestGetRepository_NotFound(t *testing.T) {
	t.Parallel()

	client, _ := newRecordingClient(t, bearerOpts(), testkit.StubResponse{
		Status: http.StatusNotFound,
		Body:   `{"type":"error","error":{"message":"Repository acme/nope not found"}}`,
	})

	_, err := client.GetRepository(t.Context(), testWorkspace, "nope")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !IsNotFound(err) {
		t.Errorf("IsNotFound(%v) = false, want true", err)
	}
}

func TestGetWorkspace_RequestShapeAndParsing(t *testing.T) {
	t.Parallel()

	body := `{"type":"workspace","uuid":"{w}","slug":"acme","name":"Acme Corp","is_private":true,
	          "created_on":"2016-01-02T03:04:05.000006+00:00",
	          "links":{"self":{"href":"s"},"html":{"href":"https://bitbucket.org/acme/"},"avatar":{"href":"a"}}}`

	client, recorded := newRecordingClient(t, bearerOpts(), testkit.StubResponse{Status: http.StatusOK, Body: body})

	workspace, err := client.GetWorkspace(t.Context(), testWorkspace)
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}

	testkit.AssertEqual(t, "path", recorded.Path, "/workspaces/acme")
	testkit.AssertEqual(t, "Slug", workspace.Slug, testWorkspace)
	testkit.AssertEqual(t, "Name", workspace.Name, "Acme Corp")
	testkit.AssertEqual(t, "IsPrivate", workspace.IsPrivate, true)
}
