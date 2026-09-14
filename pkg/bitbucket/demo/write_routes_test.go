package demo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// These tests drive the write routes and the two reviewer-candidate listings
// over raw HTTP, so the wire shapes — the merged pull request, the empty 202
// with its Location, the PENDING-then-SUCCESS task, the 409, and the validation
// envelopes with their error.fields — are asserted independently of the
// decoding in pkg/bitbucket.

// currentReviewers is the reviewer set a pull request already has, in the shape
// an update body carries it. A PUT states the complete set, so a test that
// means to leave the reviewers alone has to send them back.
func currentReviewers(t *testing.T, server *Server, slug string, id int) []reviewerRef {
	t.Helper()

	pullRequest := demoPullRequest(t, server, slug, id)
	refs := make([]reviewerRef, 0, len(pullRequest.Reviewers))
	for _, reviewer := range pullRequest.Reviewers {
		refs = append(refs, reviewerRef{UUID: reviewer.UUID})
	}
	return refs
}

// participantUUIDs is the participant list reduced to who is on it and in which
// role, which is what the reviewer rebuild has to get right.
func participantUUIDs(participants []bitbucket.Participant) []string {
	roles := make([]string, 0, len(participants))
	for _, participant := range participants {
		roles = append(roles, participant.User.UUID+":"+string(participant.Role))
	}
	return roles
}

// -----------------------------------------------------------------------------
// PUT .../pullrequests/{id}
// -----------------------------------------------------------------------------

func TestServer_UpdatePullRequestTitleAndDescription(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	before := demoPullRequest(t, server, repoAPIGateway, 101)
	start := time.Now().UTC()

	got := putJSON(t, server.URL+pullRequestPath(repoAPIGateway, 101), pullRequestUpdate{
		Title:       ptr(updatedTitle),
		Description: ptr("Now with a token bucket.\n"),
		Draft:       nil,
		Reviewers:   currentReviewers(t, server, repoAPIGateway, 101),
	}, nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusOK)

	updated := decodeJSON[bitbucket.PullRequest](t, got.Body)
	testkit.AssertEqual(t, "title", updated.Title, updatedTitle)
	testkit.AssertEqual(t, "description", updated.Description, "Now with a token bucket.\n")
	// The overview pane reads the raw description, but it resolves the mentions
	// in it through the rendered summary, so an edit has to move all three
	// members — as it does in production, where Bitbucket re-renders.
	testkit.AssertEqual(t, "summary.raw", updated.Summary.Raw, "Now with a token bucket.\n")
	testkit.AssertEqual(t, "summary.html", updated.Summary.HTML,
		"<p>Now with a token bucket.\n</p>")
	testkit.AssertEqual(t, "state is untouched", string(updated.State), string(bitbucket.PullRequestOpen))
	testkit.AssertSliceEqual(t, "reviewers survive", uuidsOf(updated.Reviewers),
		[]string{uuidAda, uuidGrace})
	if updated.UpdatedOn.Before(start) {
		t.Fatalf("updated_on = %s, want a stamp at or after %s", updated.UpdatedOn, start)
	}
	if !updated.UpdatedOn.After(before.UpdatedOn) {
		t.Fatalf("updated_on = %s, want later than the dataset's %s", updated.UpdatedOn, before.UpdatedOn)
	}

	// The change is in the dataset, not only in the response.
	fetched := decodeJSON[bitbucket.PullRequest](t, getOK(t, server, pullRequestPath(repoAPIGateway, 101)))
	testkit.AssertEqual(t, "title after GET", fetched.Title, updatedTitle)
	testkit.AssertEqual(t, "description after GET", fetched.Description, "Now with a token bucket.\n")
}

// TestServer_UpdatePullRequestRendersMentions is what makes an edited
// description behave like a fetched one: the server renders the new body the
// way Bitbucket would, so the mention the user just typed reads as a name the
// moment the overview redraws, rather than as an account id until the next
// refresh.
func TestServer_UpdatePullRequestRendersMentions(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	edited := "Rebased onto main. @{" + linus().AccountID + "} please take another look.\n"

	got := putJSON(t, server.URL+pullRequestPath(repoAPIGateway, 101), pullRequestUpdate{
		Title:       nil,
		Description: ptr(edited),
		Draft:       nil,
		Reviewers:   currentReviewers(t, server, repoAPIGateway, 101),
	}, nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusOK)

	updated := decodeJSON[bitbucket.PullRequest](t, got.Body)
	if !strings.Contains(updated.Summary.HTML, `data-atlassian-id="`+linus().AccountID+`">@Linus Torvalds`) {
		t.Fatalf("summary.html does not carry the mention span:\n%s", updated.Summary.HTML)
	}
	testkit.AssertEqual(t, "the description reads as a name",
		updated.ReadableDescription(), "Rebased onto main. @Linus Torvalds please take another look.\n")

	// And the rendering is in the dataset, not only in the response.
	fetched := decodeJSON[bitbucket.PullRequest](t, getOK(t, server, pullRequestPath(repoAPIGateway, 101)))
	testkit.AssertEqual(t, "summary.html after GET", fetched.Summary.HTML, updated.Summary.HTML)
}

func TestServer_UpdatePullRequestTogglesDraft(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	testkit.AssertEqual(t, "the fixture starts as a draft",
		demoPullRequest(t, server, repoAPIGateway, 102).Draft, true)

	got := putJSON(t, server.URL+pullRequestPath(repoAPIGateway, 102), pullRequestUpdate{
		Title:       nil,
		Description: nil,
		Draft:       ptr(false),
		Reviewers:   currentReviewers(t, server, repoAPIGateway, 102),
	}, nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusOK)
	testkit.AssertEqual(t, "draft", decodeJSON[bitbucket.PullRequest](t, got.Body).Draft, false)

	fetched := decodeJSON[bitbucket.PullRequest](t, getOK(t, server, pullRequestPath(repoAPIGateway, 102)))
	testkit.AssertEqual(t, "draft after GET", fetched.Draft, false)
}

// TestServer_UpdatePullRequestReplacesReviewers pins the whole reviewer
// transition: the set is replaced rather than merged, a reviewer who stays
// keeps the decision they had already made, and one who is dropped leaves.
func TestServer_UpdatePullRequestReplacesReviewers(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	before := demoPullRequest(t, server, repoAPIGateway, 101)
	testkit.AssertEqual(t, "the fixture starts with an approval",
		before.Participants[0].State, bitbucket.ParticipantApproved)

	got := putJSON(t, server.URL+pullRequestPath(repoAPIGateway, 101), pullRequestUpdate{
		Title:       nil,
		Description: nil,
		Draft:       nil,
		Reviewers:   []reviewerRef{{UUID: uuidAda}, {UUID: uuidLinus}},
	}, nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusOK)

	updated := decodeJSON[bitbucket.PullRequest](t, got.Body)
	testkit.AssertSliceEqual(t, "reviewers", uuidsOf(updated.Reviewers), []string{uuidAda, uuidLinus})
	testkit.AssertEqual(t, "reviewer names survive the round trip",
		updated.Reviewers[1].DisplayName, "Linus Torvalds")
	testkit.AssertSliceEqual(t, "participants", participantUUIDs(updated.Participants),
		[]string{uuidAda + ":REVIEWER", uuidLinus + ":REVIEWER"})
	// Ada had approved and stays, so her approval stays with her.
	testkit.AssertEqual(t, "kept reviewer keeps the approval", updated.Participants[0].Approved, true)
	testkit.AssertEqual(t, "kept reviewer keeps the state",
		updated.Participants[0].State, bitbucket.ParticipantApproved)
	// Linus is new, so he has not acted.
	testkit.AssertEqual(t, "new reviewer has not approved", updated.Participants[1].Approved, false)
	testkit.AssertEqual(t, "new reviewer has no state", string(updated.Participants[1].State), "")
	testkit.AssertEqual(t, "new reviewer has no participated_on",
		updated.Participants[1].ParticipatedOn.IsZero(), true)
}

// TestServer_UpdatePullRequestKeepsNonReviewerParticipants covers the other half
// of the rebuild: only the REVIEWER rows are derived from the reviewer set,
// someone who is on the pull request for commenting on it stays regardless, and
// a reviewer who already approved is demoted to a plain participant rather than
// forgotten — which is what production does.
func TestServer_UpdatePullRequestKeepsNonReviewerParticipants(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	testkit.AssertSliceEqual(t, "the fixture has a plain participant",
		participantUUIDs(demoPullRequest(t, server, repoAPIGateway, 103).Participants),
		[]string{uuidDemo + ":REVIEWER", uuidKen + ":PARTICIPANT"})

	got := putJSON(t, server.URL+pullRequestPath(repoAPIGateway, 103), pullRequestUpdate{
		Title:       nil,
		Description: nil,
		Draft:       nil,
		Reviewers:   []reviewerRef{{UUID: uuidAda}},
	}, nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusOK)

	updated := decodeJSON[bitbucket.PullRequest](t, got.Body)
	testkit.AssertSliceEqual(t, "participants", participantUUIDs(updated.Participants),
		[]string{uuidAda + ":REVIEWER", uuidDemo + ":PARTICIPANT", uuidKen + ":PARTICIPANT"})
	for _, participant := range updated.Participants {
		if participant.User.UUID == uuidDemo && !participant.Approved && participant.State == "" {
			t.Error("the demoted reviewer must keep their verdict (approval or requested changes)")
		}
	}
}

// TestServer_UpdatePullRequestWithoutReviewersClearsThem reproduces the trap the
// client's PullRequestUpdate.Reviewers exists to keep callers out of: a PUT
// states the complete reviewer set, so a document that omits it asks for none.
func TestServer_UpdatePullRequestWithoutReviewersClearsThem(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	got := putJSON(t, server.URL+pullRequestPath(repoAPIGateway, 101),
		map[string]any{"title": updatedTitle}, nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusOK)

	updated := decodeJSON[bitbucket.PullRequest](t, got.Body)
	testkit.AssertEqual(t, "len(reviewers)", len(updated.Reviewers), 0)
	for _, participant := range updated.Participants {
		// Whoever had acted stays as a plain participant; nobody is a reviewer.
		testkit.AssertEqual(t, "role of "+participant.User.UUID, participant.Role, bitbucket.RoleParticipant)
	}
}

func TestServer_UpdatePullRequestEmptyTitleIsAFieldError(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	got := putJSON(t, server.URL+pullRequestPath(repoAPIGateway, 101), pullRequestUpdate{
		Title:       ptr(""),
		Description: nil,
		Draft:       nil,
		Reviewers:   currentReviewers(t, server, repoAPIGateway, 101),
	}, nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusBadRequest)

	envelope := decodeJSON[errorBody](t, got.Body)
	testkit.AssertEqual(t, "error.type", envelope.Type, "error")
	testkit.AssertEqual(t, "error.message", envelope.Error.Message, msgBadRequest)
	testkit.AssertSliceEqual(t, "error.fields.title", envelope.Error.Fields[fieldTitle],
		[]string{msgTitleRequired})

	// Nothing was applied.
	testkit.AssertEqual(t, "title is untouched",
		demoPullRequest(t, server, repoAPIGateway, 101).Title,
		"Add a token bucket rate limiter to the public routes")
}

func TestServer_UpdatePullRequestAuthorCannotBeAReviewer(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	testkit.AssertEqual(t, "the fixture is authored by the demo user",
		demoPullRequest(t, server, repoAPIGateway, 101).Author.UUID, uuidDemo)

	got := putJSON(t, server.URL+pullRequestPath(repoAPIGateway, 101), pullRequestUpdate{
		Title:       nil,
		Description: nil,
		Draft:       nil,
		Reviewers:   []reviewerRef{{UUID: uuidAda}, {UUID: uuidDemo}},
	}, nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusBadRequest)
	testkit.AssertEqual(t, "error.message",
		decodeJSON[errorBody](t, got.Body).Error.Message, msgAuthorCannotReview)
	testkit.AssertSliceEqual(t, "reviewers are untouched",
		uuidsOf(demoPullRequest(t, server, repoAPIGateway, 101).Reviewers),
		[]string{uuidAda, uuidGrace})
}

func TestServer_UpdatePullRequestUnknownReviewerIsBadRequest(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	const stranger = "{00000000-0000-0000-0000-0000000000ff}"

	got := putJSON(t, server.URL+pullRequestPath(repoAPIGateway, 101), pullRequestUpdate{
		Title:       nil,
		Description: nil,
		Draft:       nil,
		Reviewers:   []reviewerRef{{UUID: stranger}},
	}, nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusBadRequest)
	if message := decodeJSON[errorBody](t, got.Body).Error.Message; !strings.Contains(message, stranger) {
		t.Fatalf("error.message = %q, want it to name %s", message, stranger)
	}
}

func TestServer_UpdatePullRequestNonOpenIsBadRequest(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	testkit.AssertEqual(t, "the fixture is merged",
		string(demoPullRequest(t, server, repoAPIGateway, 98).State),
		string(bitbucket.PullRequestMerged))

	got := putJSON(t, server.URL+pullRequestPath(repoAPIGateway, 98), pullRequestUpdate{
		Title:       ptr(updatedTitle),
		Description: nil,
		Draft:       nil,
		Reviewers:   currentReviewers(t, server, repoAPIGateway, 98),
	}, nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusBadRequest)
	testkit.AssertEqual(t, "error.message",
		decodeJSON[errorBody](t, got.Body).Error.Message, msgOnlyOpenCanBeMutated)
}

func TestServer_UpdatePullRequestMalformedBodyIsBadRequest(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	got := putJSON(t, server.URL+pullRequestPath(repoAPIGateway, 101),
		json.RawMessage(`{"title": `), nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusBadRequest)
	if message := decodeJSON[errorBody](t, got.Body).Error.Message; !strings.Contains(message, "JSON") {
		t.Fatalf("error.message = %q, want it to blame the JSON", message)
	}
}

func TestServer_UpdatePullRequestUnknownIsNotFound(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	got := putJSON(t, server.URL+pullRequestPath(repoAPIGateway, 999), pullRequestUpdate{
		Title:       ptr(updatedTitle),
		Description: nil,
		Draft:       nil,
		Reviewers:   nil,
	}, nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusNotFound)
}

// TestServer_WriteRoutesAuthenticateNothing states the rule the read routes
// already follow: the fake records the Authorization header and never judges it,
// on a write no less than on a read.
func TestServer_WriteRoutesAuthenticateNothing(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	got := putJSON(t, server.URL+pullRequestPath(repoAPIGateway, 101), pullRequestUpdate{
		Title:       ptr(updatedTitle),
		Description: nil,
		Draft:       nil,
		Reviewers:   currentReviewers(t, server, repoAPIGateway, 101),
	}, nil)
	testkit.AssertEqual(t, "status without credentials", got.Status, http.StatusOK)
	testkit.AssertEqual(t, "LastAuthorization()", server.LastAuthorization(), "")
}

// -----------------------------------------------------------------------------
// POST .../pullrequests/{id}/merge
// -----------------------------------------------------------------------------

func TestServer_MergePullRequest(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	start := time.Now().UTC()

	got := postJSON(t, server.URL+mergePath(repoAPIGateway, 101), mergeParameters{
		Type:              "pullrequest_merge_parameters",
		MergeStrategy:     bitbucket.MergeSquash,
		CloseSourceBranch: ptr(false),
		Message:           "Merge the rate limiter",
	}, nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusOK)

	merged := decodeJSON[bitbucket.PullRequest](t, got.Body)
	testkit.AssertEqual(t, "state", string(merged.State), string(bitbucket.PullRequestMerged))
	if merged.ClosedBy == nil {
		t.Fatalf("merged pull request has no closed_by: %+v", merged)
	}
	testkit.AssertEqual(t, "closed_by.uuid", merged.ClosedBy.UUID, uuidDemo)
	if merged.MergeCommit == nil {
		t.Fatalf("merged pull request has no merge_commit: %+v", merged)
	}
	// A hash inside a pull request payload is the 12-character abbreviation.
	testkit.AssertEqual(t, "len(merge_commit.hash)", len(merged.MergeCommit.Hash), 12)
	testkit.AssertEqual(t, "close_source_branch follows the body", merged.CloseSourceBranch, false)
	if merged.UpdatedOn.Before(start) {
		t.Fatalf("updated_on = %s, want a stamp at or after %s", merged.UpdatedOn, start)
	}

	// The dataset moved with it: the pull request reads MERGED and has left the
	// open listing.
	fetched := decodeJSON[bitbucket.PullRequest](t, getOK(t, server, pullRequestPath(repoAPIGateway, 101)))
	testkit.AssertEqual(t, "state after GET", string(fetched.State), string(bitbucket.PullRequestMerged))

	open := decodeJSON[listPage[bitbucket.PullRequest]](t,
		getOK(t, server, pullRequestsPath(repoAPIGateway)))
	for _, pullRequest := range open.Values {
		if pullRequest.ID == 101 {
			t.Fatalf("the open listing still carries #101: %+v", open.Values)
		}
	}
}

// TestServer_MergeCommitHashFollowsTheParameters shows that the fake reads the
// merge body it is sent: there is no commit graph here, so the merge commit is
// derived from the request, which makes it both reproducible and sensitive to
// what was asked for.
func TestServer_MergeCommitHashFollowsTheParameters(t *testing.T) {
	t.Parallel()

	squash := mergeParameters{
		Type:              "pullrequest_merge_parameters",
		MergeStrategy:     bitbucket.MergeSquash,
		CloseSourceBranch: nil,
		Message:           "",
	}
	fastForward := mergeParameters{
		Type:              "pullrequest_merge_parameters",
		MergeStrategy:     bitbucket.MergeFastForward,
		CloseSourceBranch: nil,
		Message:           "",
	}

	first := mergeHashOf(t, squash)
	second := mergeHashOf(t, squash)
	other := mergeHashOf(t, fastForward)

	testkit.AssertEqual(t, "the same merge yields the same commit", first, second)
	if first == other {
		t.Fatalf("squash and fast_forward both merged as %s, want different commits", first)
	}
}

// mergeHashOf merges #101 on a fresh server and reports the merge commit.
func mergeHashOf(t *testing.T, params mergeParameters) string {
	t.Helper()

	server := newTestServer(t)
	got := postJSON(t, server.URL+mergePath(repoAPIGateway, 101), params, nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusOK)

	merged := decodeJSON[bitbucket.PullRequest](t, got.Body)
	if merged.MergeCommit == nil {
		t.Fatalf("merged pull request has no merge_commit: %+v", merged)
	}
	return merged.MergeCommit.Hash
}

func TestServer_MergePullRequestAcceptsAnEmptyBody(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	got := postJSON(t, server.URL+mergePath(repoInfra, 7), nil, nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusOK)

	merged := decodeJSON[bitbucket.PullRequest](t, got.Body)
	testkit.AssertEqual(t, "state", string(merged.State), string(bitbucket.PullRequestMerged))
	// Without close_source_branch in the body the pull request's own flag stands.
	testkit.AssertEqual(t, "close_source_branch", merged.CloseSourceBranch, true)
}

func TestServer_MergePullRequestUnknownStrategyIsBadRequest(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	got := postJSON(t, server.URL+mergePath(repoAPIGateway, 101),
		map[string]any{"merge_strategy": "octopus"}, nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusBadRequest)
	testkit.AssertEqual(t, "error.message", decodeJSON[errorBody](t, got.Body).Error.Message,
		`merge_strategy: "octopus" is not a valid choice.`)
	testkit.AssertEqual(t, "the pull request is untouched",
		string(demoPullRequest(t, server, repoAPIGateway, 101).State),
		string(bitbucket.PullRequestOpen))
}

func TestServer_MergePullRequestNonOpenIsBadRequest(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	got := postJSON(t, server.URL+mergePath(repoAPIGateway, 98), nil, nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusBadRequest)
	testkit.AssertEqual(t, "error.message",
		decodeJSON[errorBody](t, got.Body).Error.Message, msgOnlyOpenCanBeMerged)
}

// TestServer_MergePullRequestRefsChanged is the designated 409: the fixture that
// always loses the race with another push, so the TUI's "refresh and retry"
// path has something to answer.
func TestServer_MergePullRequestRefsChanged(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	got := postJSON(t, server.URL+mergePath(repoWebApp, prRefsChanged), nil, nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusConflict)
	testkit.AssertEqual(t, "error.message",
		decodeJSON[errorBody](t, got.Body).Error.Message, msgRefsChanged)
	testkit.AssertEqual(t, "the pull request is untouched",
		string(demoPullRequest(t, server, repoWebApp, prRefsChanged).State),
		string(bitbucket.PullRequestOpen))
}

func TestServer_MergePullRequestUnknownIsNotFound(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	got := postJSON(t, server.URL+mergePath(repoAPIGateway, 999), nil, nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusNotFound)
}

// -----------------------------------------------------------------------------
// The queued merge and its task
// -----------------------------------------------------------------------------

// TestServer_MergePullRequestQueued walks the whole asynchronous path: the 202
// with a Location and no document, a first poll that answers PENDING while the
// pull request is still open, and a second that answers SUCCESS and lands the
// merge on the dataset.
func TestServer_MergePullRequestQueued(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	queued := postJSON(t, server.URL+mergePath(repoAPIGateway, prQueuedMerge), mergeParameters{
		Type:              "pullrequest_merge_parameters",
		MergeStrategy:     bitbucket.MergeCommit,
		CloseSourceBranch: nil,
		Message:           "",
	}, nil)
	testkit.AssertEqual(t, "status", queued.Status, http.StatusAccepted)
	testkit.AssertEqual(t, "a queued merge answers no document", len(queued.Body), 0)

	location := queued.Header.Get(headerLocation)
	target, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse Location %q: %v", location, err)
	}
	// The client refuses to follow a link off its own origin, so the task URL is
	// absolute and points back here.
	testkit.AssertEqual(t, "Location scheme", target.Scheme, "http")
	testkit.AssertEqual(t, "Location host", "http://"+target.Host, server.URL)
	testkit.AssertEqual(t, "Location path", target.Path,
		mergeTaskPath(repoAPIGateway, prQueuedMerge, mergeTaskID(prQueuedMerge)))

	// Nothing has happened to the pull request yet.
	testkit.AssertEqual(t, "state while the merge is queued",
		string(demoPullRequest(t, server, repoAPIGateway, prQueuedMerge).State),
		string(bitbucket.PullRequestOpen))

	pending := decodeRaw(t, getOK(t, server, target.Path))
	testkit.AssertEqual(t, "first poll task_status", string(pending[fieldTaskStatus]),
		`"`+bitbucket.MergeTaskPending+`"`)
	if _, ok := pending[fieldMergeResult]; ok {
		t.Fatalf("a pending task carries a %q member: %s", fieldMergeResult, pending)
	}
	testkit.AssertEqual(t, "state after the first poll",
		string(demoPullRequest(t, server, repoAPIGateway, prQueuedMerge).State),
		string(bitbucket.PullRequestOpen))

	body := getOK(t, server, target.Path)
	success := decodeJSON[mergeTaskEnvelope](t, body)
	testkit.AssertEqual(t, "second poll task_status", success.Status, bitbucket.MergeTaskSuccess)
	if success.MergeResult == nil {
		t.Fatalf("a successful task carries no merge_result: %s", body)
	}
	testkit.AssertEqual(t, "merge_result.state",
		string(success.MergeResult.State), string(bitbucket.PullRequestMerged))
	testkit.AssertEqual(t, "the task links to itself", success.Links.Self.Href, location)

	// The merge landed with the SUCCESS.
	fetched := decodeJSON[bitbucket.PullRequest](t,
		getOK(t, server, pullRequestPath(repoAPIGateway, prQueuedMerge)))
	testkit.AssertEqual(t, "state after SUCCESS", string(fetched.State), string(bitbucket.PullRequestMerged))
	testkit.AssertEqual(t, "merge commits agree",
		fetched.MergeCommit.Hash, success.MergeResult.MergeCommit.Hash)
}

func TestServer_MergeTaskStatusUnknownTaskIsBadRequest(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	got := get(t, server.URL+mergeTaskPath(repoAPIGateway, prQueuedMerge, "t-nope"), nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusBadRequest)
}

// TestServer_MergeTaskStatusRejectsAnotherPullRequestsTask is the second half of
// the endpoint's own 400: a task id is only valid against the pull request it
// was issued for.
func TestServer_MergeTaskStatusRejectsAnotherPullRequestsTask(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	queued := postJSON(t, server.URL+mergePath(repoAPIGateway, prQueuedMerge), nil, nil)
	testkit.AssertEqual(t, "status", queued.Status, http.StatusAccepted)

	got := get(t, server.URL+mergeTaskPath(repoAPIGateway, 101, mergeTaskID(prQueuedMerge)), nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusBadRequest)
	if message := decodeJSON[errorBody](t, got.Body).Error.Message; !strings.Contains(message, "101") {
		t.Fatalf("error.message = %q, want it to name the pull request it was asked about", message)
	}
}

func TestServer_MergeTaskStatusUnknownPullRequestIsNotFound(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	got := get(t, server.URL+mergeTaskPath(repoAPIGateway, 999, mergeTaskID(prQueuedMerge)), nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusNotFound)
}

// TestServer_ConcurrentReadsAndWrites is why the dataset is behind a lock at
// all: a merge or an update rewrites pull requests another request is in the
// middle of serialising. Under -race, an unguarded dataset fails this.
func TestServer_ConcurrentReadsAndWrites(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	ctx := t.Context()

	// Enough concurrency that an unguarded dataset actually loses the race: with
	// a handful of workers the read and the write rarely overlap and -race sees
	// nothing.
	const workers = 60
	failures := make(chan error, 3*workers)
	var group sync.WaitGroup

	for worker := range workers {
		group.Add(3)
		go func() {
			defer group.Done()
			failures <- exchange(ctx, http.MethodGet, server.URL+pullRequestsPath(repoAPIGateway), nil)
		}()
		go func() {
			defer group.Done()
			failures <- exchange(ctx, http.MethodGet, server.URL+pullRequestPath(repoAPIGateway, 101), nil)
		}()
		go func() {
			defer group.Done()
			body := []byte(`{"title":"Concurrent ` + strconv.Itoa(worker) +
				`","reviewers":[{"uuid":"` + uuidAda + `"}]}`)
			failures <- exchange(ctx, http.MethodPut, server.URL+pullRequestPath(repoAPIGateway, 101), body)
		}()
	}

	group.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatalf("concurrent request: %v", err)
		}
	}

	// Whichever write won, the pull request is one of the documents that were
	// sent and not a mixture of them.
	final := demoPullRequest(t, server, repoAPIGateway, 101)
	if !strings.HasPrefix(final.Title, "Concurrent ") {
		t.Fatalf("title = %q, want one of the concurrent writes", final.Title)
	}
	testkit.AssertSliceEqual(t, "reviewers", uuidsOf(final.Reviewers), []string{uuidAda})
}

// exchange issues one request and reports what went wrong, if anything, so a
// goroutine can fail a test without touching *testing.T from off the test's own
// goroutine.
func exchange(ctx context.Context, method, url string, body []byte) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	request, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return fmt.Errorf("build request %s %s: %w", method, url, err)
	}
	request.Header.Set(headerContentType, "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, url, err)
	}
	defer func() { _ = response.Body.Close() }()

	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		return fmt.Errorf("read body of %s %s: %w", method, url, err)
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s %s: status %d", method, url, response.StatusCode)
	}
	return nil
}

// -----------------------------------------------------------------------------
// POST and DELETE .../pullrequests/{id}/approve
// -----------------------------------------------------------------------------

// participantOf is the demo user's participant row on a pull request, and
// whether they are on it at all.
func participantOf(participants []bitbucket.Participant, uuid string) (bitbucket.Participant, bool) {
	for _, participant := range participants {
		if sameUUID(participant.User.UUID, uuid) {
			return participant, true
		}
	}
	return bitbucket.Participant{}, false
}

func TestServer_ApprovePullRequestAsAReviewer(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	before, ok := participantOf(demoPullRequest(t, server, repoAPIGateway, 102).Participants, uuidDemo)
	testkit.AssertEqual(t, "the demo user is a reviewer on the fixture", ok && before.Role ==
		bitbucket.RoleReviewer, true)
	testkit.AssertEqual(t, "and has not acted yet", before.Approved, false)
	start := time.Now().UTC()

	got := postJSON(t, server.URL+approvePath(repoAPIGateway, 102), nil, nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusOK)

	// The endpoint answers with a participant, not with a pull request.
	approved := decodeJSON[bitbucket.Participant](t, got.Body)
	testkit.AssertEqual(t, "type", approved.Type, "participant")
	testkit.AssertEqual(t, "user.uuid", approved.User.UUID, uuidDemo)
	testkit.AssertEqual(t, "role", approved.Role, bitbucket.RoleReviewer)
	testkit.AssertEqual(t, "approved", approved.Approved, true)
	testkit.AssertEqual(t, "state", approved.State, bitbucket.ParticipantApproved)
	if approved.ParticipatedOn.Before(start) {
		t.Fatalf("participated_on = %s, want a stamp at or after %s", approved.ParticipatedOn, start)
	}

	// The dataset moved with it, so the TUI's re-fetch sees the approval.
	fetched := decodeJSON[bitbucket.PullRequest](t, getOK(t, server, pullRequestPath(repoAPIGateway, 102)))
	stored, found := participantOf(fetched.Participants, uuidDemo)
	testkit.AssertEqual(t, "still one row for the approver", found, true)
	testkit.AssertEqual(t, "approved after GET", stored.Approved, true)
	testkit.AssertEqual(t, "role after GET", stored.Role, bitbucket.RoleReviewer)
	testkit.AssertEqual(t, "the reviewer set is untouched", len(fetched.Reviewers), 2)
	testkit.AssertEqual(t, "the approval is counted", fetched.ReviewSummary().Approved, 1)
}

// Someone who was never asked to review can still approve; the row they get is
// a PARTICIPANT rather than a REVIEWER.
func TestServer_ApprovePullRequestAsANonReviewer(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	_, ok := participantOf(demoPullRequest(t, server, repoInfra, 7).Participants, uuidDemo)
	testkit.AssertEqual(t, "the demo user is not on the fixture", ok, false)

	got := postJSON(t, server.URL+approvePath(repoInfra, 7), nil, nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusOK)
	testkit.AssertEqual(t, "role", decodeJSON[bitbucket.Participant](t, got.Body).Role,
		bitbucket.RoleParticipant)

	stored, found := participantOf(demoPullRequest(t, server, repoInfra, 7).Participants, uuidDemo)
	testkit.AssertEqual(t, "the approver was added", found, true)
	testkit.AssertEqual(t, "role in the dataset", stored.Role, bitbucket.RoleParticipant)
}

// Approving twice is not an error and does not add a second row.
func TestServer_ApprovePullRequestIsIdempotent(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	for range 2 {
		testkit.AssertEqual(t, "status",
			postJSON(t, server.URL+approvePath(repoAPIGateway, 102), nil, nil).Status, http.StatusOK)
	}

	participants := demoPullRequest(t, server, repoAPIGateway, 102).Participants
	testkit.AssertSliceEqual(t, "participants", participantUUIDs(participants),
		[]string{uuidDemo + ":REVIEWER", uuidLinus + ":REVIEWER"})
}

// A reviewer who withdraws stays on the pull request with nothing decided: they
// are still expected to review it.
func TestServer_UnapprovePullRequestKeepsTheReviewerRow(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	before, _ := participantOf(demoPullRequest(t, server, repoWebApp, 40).Participants, uuidDemo)
	testkit.AssertEqual(t, "the fixture starts approved", before.Approved, true)

	got := sendJSON(t, http.MethodDelete, server.URL+approvePath(repoWebApp, 40), nil, nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusNoContent)
	testkit.AssertEqual(t, "a 204 carries no document", len(got.Body), 0)

	fetched := decodeJSON[bitbucket.PullRequest](t, getOK(t, server, pullRequestPath(repoWebApp, 40)))
	stored, found := participantOf(fetched.Participants, uuidDemo)
	testkit.AssertEqual(t, "the reviewer row survives", found, true)
	testkit.AssertEqual(t, "approved", stored.Approved, false)
	testkit.AssertEqual(t, "state", string(stored.State), "")
	testkit.AssertEqual(t, "participated_on is cleared", stored.ParticipatedOn.IsZero(), true)
	testkit.AssertEqual(t, "the approval is gone from the rollup", fetched.ReviewSummary().Approved, 0)
}

// A row that existed only to carry the approval goes with it.
func TestServer_UnapprovePullRequestDropsAPlainParticipant(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	testkit.AssertEqual(t, "approve first",
		postJSON(t, server.URL+approvePath(repoInfra, 7), nil, nil).Status, http.StatusOK)

	got := sendJSON(t, http.MethodDelete, server.URL+approvePath(repoInfra, 7), nil, nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusNoContent)

	participants := demoPullRequest(t, server, repoInfra, 7).Participants
	_, found := participantOf(participants, uuidDemo)
	testkit.AssertEqual(t, "the participant row is gone", found, false)
	testkit.AssertSliceEqual(t, "the rest are untouched", participantUUIDs(participants),
		[]string{uuidBarbara + ":REVIEWER"})
}

func TestServer_ApprovingYourOwnPullRequestIsBadRequest(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	testkit.AssertEqual(t, "the fixture is authored by the demo user",
		demoPullRequest(t, server, repoWebApp, prRefsChanged).Author.UUID, uuidDemo)

	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		got := sendJSON(t, method, server.URL+approvePath(repoWebApp, prRefsChanged), nil, nil)
		testkit.AssertEqual(t, method+" status", got.Status, http.StatusBadRequest)
		testkit.AssertEqual(t, method+" error.message",
			decodeJSON[errorBody](t, got.Body).Error.Message, msgCannotApproveOwn)
	}

	testkit.AssertEqual(t, "nobody was added",
		len(demoPullRequest(t, server, repoWebApp, prRefsChanged).Participants), 0)
}

func TestServer_ApproveUnknownPullRequestIsNotFound(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		testkit.AssertEqual(t, method+" on an unknown pull request",
			sendJSON(t, method, server.URL+approvePath(repoAPIGateway, 999), nil, nil).Status,
			http.StatusNotFound)
		testkit.AssertEqual(t, method+" on an unknown repository",
			sendJSON(t, method, server.URL+approvePath("no-such-repository", 101), nil, nil).Status,
			http.StatusNotFound)
	}
}

// -----------------------------------------------------------------------------
// Reviewer candidates
// -----------------------------------------------------------------------------

func TestServer_EffectiveDefaultReviewers(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	body := getOK(t, server, defaultReviewersPath(repoAPIGateway))
	page := decodeJSON[listPage[defaultReviewerEntry]](t, body)

	testkit.AssertEqual(t, "len(values)", len(page.Values), 2)
	testkit.AssertEqual(t, "values[0].type", page.Values[0].Type, "default_reviewer_and_type")
	// "Effective" is the point of the endpoint: it answers with the reviewers
	// inherited from the project as well as the repository's own.
	testkit.AssertEqual(t, "values[0].reviewer_type", page.Values[0].ReviewerType, "project")
	testkit.AssertEqual(t, "values[1].reviewer_type", page.Values[1].ReviewerType, "repository")
	testkit.AssertEqual(t, "values[0].user.uuid", page.Values[0].User.UUID, uuidAda)
	testkit.AssertEqual(t, "values[1].user.uuid", page.Values[1].User.UUID, uuidGrace)

	// The account is nested under user, never spread over the entry itself.
	values := decodeJSON[[]map[string]json.RawMessage](t, decodeRaw(t, body)[fieldValues])
	if _, ok := values[0][fieldUser]; !ok {
		t.Fatalf("entry carries no %q member: %s", fieldUser, body)
	}
}

func TestServer_EffectiveDefaultReviewersUnknownRepositoryIsNotFound(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	got := get(t, server.URL+defaultReviewersPath("no-such-repository"), nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusNotFound)
}

func TestServer_WorkspaceMembers(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	body := getOK(t, server, pathMembers)
	page := decodeJSON[listPage[workspaceMembership]](t, body)

	testkit.AssertEqual(t, "len(values)", len(page.Values), 6)
	testkit.AssertEqual(t, "size", page.Size, 6)
	testkit.AssertEqual(t, "values[0].type", page.Values[0].Type, typeWorkspaceMembership)
	testkit.AssertEqual(t, "values[0].user.uuid", page.Values[0].User.UUID, uuidDemo)
	// A membership is an account plus the workspace it belongs to, not a bare
	// account: a client that decoded values[] as accounts would get nothing.
	testkit.AssertEqual(t, "values[0].workspace.slug", page.Values[0].Workspace.Slug, Workspace)
}

func TestServer_WorkspaceMembersPagination(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)
	first := decodeJSON[listPage[workspaceMembership]](t, getOK(t, server, pathMembers+queryPageLenTwo))

	testkit.AssertEqual(t, "len(first page)", len(first.Values), 2)
	testkit.AssertEqual(t, "first page size", first.Size, 6)
	if first.Next == "" {
		t.Fatalf("first page has no next link: %+v", first)
	}

	next, err := url.Parse(first.Next)
	if err != nil {
		t.Fatalf("parse next %q: %v", first.Next, err)
	}
	testkit.AssertEqual(t, "the next link stays on this origin", "http://"+next.Host, server.URL)
	testkit.AssertEqual(t, "the next link names page 2", next.Query().Get(fieldPage), strconv.Itoa(2))

	second := decodeJSON[listPage[workspaceMembership]](t, getOK(t, server, next.RequestURI()))
	testkit.AssertEqual(t, "len(second page)", len(second.Values), 2)
	testkit.AssertEqual(t, "the pages do not overlap",
		second.Values[0].User.UUID != first.Values[0].User.UUID, true)
}

func TestServer_WorkspaceMembersUnknownWorkspaceIsNotFound(t *testing.T) {
	t.Parallel()

	server := newTestServer(t)

	got := get(t, server.URL+"/workspaces/not-acme/members", nil)
	testkit.AssertEqual(t, "status", got.Status, http.StatusNotFound)
}
