package demo

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// These drive POST .../pullrequests over raw HTTP: the 201 and the pull request
// it answers with, the four validation failures, and the fact that what was
// created is really in the dataset afterwards.

// newPullRequestDocument is a create body, written as a raw document so a test
// can leave a member out.
func newPullRequestDocument(title, source, destination string, draft bool, reviewers ...string) json.RawMessage {
	body := `{"title":` + mustQuote(title) +
		`,"source":{"branch":{"name":` + mustQuote(source) + `}}`
	if destination != "" {
		body += `,"destination":{"branch":{"name":` + mustQuote(destination) + `}}`
	}
	if len(reviewers) > 0 {
		refs := make([]string, 0, len(reviewers))
		for _, uuid := range reviewers {
			refs = append(refs, `{"uuid":`+mustQuote(uuid)+`}`)
		}
		body += `,"reviewers":[` + strings.Join(refs, ",") + `]`
	}
	if draft {
		body += `,"draft":true`
	}
	return json.RawMessage(body + "}")
}

func TestServer_CreatePullRequestAnswers201WithThePullRequest(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	start := time.Now().UTC()
	before := len(decodeJSON[listPage[bitbucket.PullRequest]](t,
		getOK(t, server, pullRequestsPath(repoAPIGateway))).Values)

	// bugfix/upstream-timeout is a branch the fixtures already use, so the
	// repository has it; main is its default target, so this one goes to a
	// different destination to stay out of the duplicate rule.
	got := postJSON(t, server.URL+pullRequestsPath(repoAPIGateway),
		newPullRequestDocument("Fix the retry budget", "bugfix/upstream-timeout",
			"feature/oauth-introspection", false), nil)

	testkit.AssertEqual(t, "status", got.Status, http.StatusCreated)
	created := decodeJSON[bitbucket.PullRequest](t, got.Body)
	testkit.AssertEqual(t, "title", created.Title, "Fix the retry budget")
	testkit.AssertEqual(t, "state", created.State, bitbucket.PullRequestOpen)
	testkit.AssertEqual(t, "author", created.Author.UUID, server.Data.User.UUID)
	testkit.AssertEqual(t, "source", created.Source.Branch.Name, "bugfix/upstream-timeout")
	testkit.AssertEqual(t, "destination", created.Destination.Branch.Name, "feature/oauth-introspection")
	if created.ID == 0 {
		t.Error("the created pull request should carry an id")
	}
	if created.CreatedOn.Before(start) {
		t.Errorf("created_on = %v, want the wall clock", created.CreatedOn)
	}

	// And it is in the list, at the top, because it is the newest.
	page := decodeJSON[listPage[bitbucket.PullRequest]](t,
		getOK(t, server, pullRequestsPath(repoAPIGateway)))
	testkit.AssertEqual(t, "list length", len(page.Values), before+1)
	testkit.AssertEqual(t, "first row", page.Values[0].ID, created.ID)
}

func TestServer_CreatePullRequestFallsBackToTheDefaultBranch(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	// web-app's default branch is develop, not main: a create with no
	// destination lands on whatever the repository says. feature/keyboard-nav
	// is a branch whose pull request has already merged, so nothing is open for
	// the pair.
	got := postJSON(t, server.URL+pullRequestsPath(repoWebApp),
		newPullRequestDocument("Bump the design system", "feature/keyboard-nav", "", true), nil)

	testkit.AssertEqual(t, "status", got.Status, http.StatusCreated)
	created := decodeJSON[bitbucket.PullRequest](t, got.Body)
	testkit.AssertEqual(t, "destination", created.Destination.Branch.Name, branchDevelop)
	testkit.AssertEqual(t, "draft", created.Draft, true)
}

func TestServer_CreatePullRequestCarriesItsReviewers(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	reviewer := ""
	for _, member := range server.Data.Members {
		if !sameUUID(member.UUID, server.Data.User.UUID) {
			reviewer = member.UUID
			break
		}
	}
	if reviewer == "" {
		t.Skip("the dataset has no member other than the demo user")
	}

	got := postJSON(t, server.URL+pullRequestsPath(repoInfra),
		newPullRequestDocument("Pin terraform", "chore/rotate-secrets", "", false, reviewer), nil)

	testkit.AssertEqual(t, "status", got.Status, http.StatusCreated)
	created := decodeJSON[bitbucket.PullRequest](t, got.Body)
	if len(created.Reviewers) != 1 {
		t.Fatalf("reviewers = %+v, want one", created.Reviewers)
	}
	testkit.AssertEqual(t, "reviewer", created.Reviewers[0].UUID, reviewer)
	// The participant row comes with it, which is what the approvals column
	// and the reviewer line are drawn from.
	found := false
	for _, participant := range created.Participants {
		if sameUUID(participant.User.UUID, reviewer) &&
			participant.Role == bitbucket.RoleReviewer {
			found = true
		}
	}
	if !found {
		t.Errorf("participants = %+v, want the reviewer among them", created.Participants)
	}
}

func TestServer_CreatePullRequestRefusals(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		slug   string
		body   json.RawMessage
		status int
		field  string
	}{
		{
			name: "no title", slug: repoAPIGateway,
			body:   newPullRequestDocument("  ", "bugfix/upstream-timeout", "", false),
			status: http.StatusBadRequest, field: fieldTitleCreate,
		},
		{
			name: "a branch the repository does not have", slug: repoAPIGateway,
			body:   newPullRequestDocument("Something", "feature/never-pushed", "", false),
			status: http.StatusBadRequest, field: fieldSource,
		},
		{
			name: "source and destination are the same", slug: repoAPIGateway,
			body: newPullRequestDocument("Something", "bugfix/upstream-timeout",
				"bugfix/upstream-timeout", false),
			status: http.StatusBadRequest,
		},
		{
			name: "a pull request is already open for the pair", slug: repoAPIGateway,
			// #103 is open for exactly this pair.
			body:   newPullRequestDocument("Again", "bugfix/upstream-timeout", branchMain, false),
			status: http.StatusBadRequest,
		},
		{
			name: "the author as their own reviewer", slug: repoInfra,
			body: newPullRequestDocument("Something", "chore/rotate-secrets", "", false,
				NewDataset().User.UUID),
			status: http.StatusBadRequest,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := newTestServer(t)
			got := postJSON(t, server.URL+pullRequestsPath(tc.slug), tc.body, nil)

			testkit.AssertEqual(t, "status", got.Status, tc.status)
			if tc.field == "" {
				return
			}
			envelope := decodeJSON[errorBody](t, got.Body)
			if messages := envelope.Error.Fields[tc.field]; len(messages) == 0 {
				t.Errorf("the refusal should name %q: %s", tc.field, got.Body)
			}
		})
	}
}

func TestServer_CreatePullRequestInAnUnknownRepository(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	got := postJSON(t, server.URL+pullRequestsPath("no-such-repo"),
		newPullRequestDocument("Something", "main", "", false), nil)

	testkit.AssertEqual(t, "status", got.Status, http.StatusNotFound)
}
