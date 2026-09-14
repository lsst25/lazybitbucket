package demo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
)

// The routes here are the write half of the fake: PUT .../pullrequests/{id},
// POST .../pullrequests/{id}/merge, the merge task-status poll a queued merge
// needs, POST and DELETE on .../pullrequests/{id}/approve, and the two listings
// the reviewer picker draws its candidates from.
//
// They authenticate nothing, exactly like the read routes: LastAuthorization
// stays the only observable of what the client sent. A fake that rejected an
// unauthenticated write would be asserting an authorization model this package
// does not have.

// Designated fixtures. Merging is otherwise uniform, and a TUI that only ever
// sees 200 cannot be shown to handle the answers that matter, so two pull
// requests are reserved for the interesting ones.
const (
	// prQueuedMerge is the pull request whose merge Bitbucket queues: it answers
	// 202 with a task to poll instead of a merged pull request. It has to be an
	// open non-draft pull request, which is why it is #103 and not the draft
	// #102 — although the demo does not enforce "a draft cannot be merged" at
	// all, and merging #102 succeeds synchronously like any other.
	prQueuedMerge = 103
	// prRefsChanged is the pull request whose merge always loses the race with
	// another push: it answers 409.
	prRefsChanged = 41
)

// Messages the real API answers validation failures with, quoted so the TUI's
// error line can be read against the thing it will show in production.
const (
	msgOnlyOpenCanBeMerged  = "Only open pull requests can be merged."
	msgOnlyOpenCanBeMutated = "Only open pull requests can be mutated."
	msgAuthorCannotReview   = "The pull request author cannot be a reviewer."
	msgTitleRequired        = "This field is required."
	// msgCannotApproveOwn is the refusal both approval verbs answer the author
	// with. Withdrawing is refused with the same sentence rather than a second
	// one: an author never has an approval to withdraw in the first place.
	msgCannotApproveOwn = "You cannot approve your own pull request"
	// msgBadRequest is the envelope's own sentence when the fault is in a named
	// member: the actionable half of the answer is in error.fields, and the
	// message is the generic one the client appends the field detail to.
	msgBadRequest  = "Bad request"
	msgRefsChanged = "Unable to merge because one of the refs involved changed " +
		"while attempting to merge"
)

// fieldTitle is the request member an empty title is reported against.
const fieldTitle = "title"

// mergeTask is a queued merge: the pull request as the merge left it, held back
// until the task has been polled once, plus where it belongs so a task id
// quoted against the wrong pull request can be refused the way the real
// endpoint refuses it.
type mergeTask struct {
	slug   string
	id     int
	polls  int
	merged bitbucket.PullRequest
}

// mergeParameters is the body of POST .../merge. Every member is optional — the
// endpoint accepts an empty document — so they are pointers or zero-valued.
type mergeParameters struct {
	Type              string                  `json:"type"`
	MergeStrategy     bitbucket.MergeStrategy `json:"merge_strategy"`
	CloseSourceBranch *bool                   `json:"close_source_branch"`
	Message           string                  `json:"message"`
}

// pullRequestUpdate is the partial pull request document a PUT carries. A nil
// pointer is a member the request left out; Reviewers is a slice because its
// absence and its emptiness mean different things to the caller and the same
// thing to Bitbucket — see handleUpdatePullRequest.
type pullRequestUpdate struct {
	Title       *string       `json:"title"`
	Description *string       `json:"description"`
	Draft       *bool         `json:"draft"`
	Reviewers   []reviewerRef `json:"reviewers"`
}

// reviewerRef is how a reviewer arrives in an update body: an object carrying
// nothing but the account UUID.
type reviewerRef struct {
	UUID string `json:"uuid"`
}

// selfLink is the links member of the merge task envelope, which carries the
// poll URL and nothing else.
type selfLink struct {
	Self bitbucket.Link `json:"self"`
}

// mergeTaskEnvelope is what the task-status endpoint answers: the status, the
// task's own link, and — once the merge has finished — the merged pull request.
type mergeTaskEnvelope struct {
	Status      string                 `json:"task_status"`
	Links       selfLink               `json:"links"`
	MergeResult *bitbucket.PullRequest `json:"merge_result,omitempty"`
}

// defaultReviewerEntry is one value of the effective-default-reviewers
// collection: the account plus where the rule that adds it came from.
type defaultReviewerEntry struct {
	Type         string            `json:"type"`
	ReviewerType string            `json:"reviewer_type"`
	User         bitbucket.Account `json:"user"`
}

// workspaceMembership is one value of the workspace members collection: an
// account and the workspace it belongs to, never a bare account.
type workspaceMembership struct {
	Type      string              `json:"type"`
	User      bitbucket.Account   `json:"user"`
	Workspace bitbucket.Workspace `json:"workspace"`
}

// -----------------------------------------------------------------------------
// Merge
// -----------------------------------------------------------------------------

// handleMergePullRequest merges a pull request. Which of the four answers it
// gives — 200, the queued 202, the 409 of a lost race, or a 400 — is decided by
// which pull request is being merged and what the body asks for; see the
// designated fixtures above.
func (s *Server) handleMergePullRequest(w http.ResponseWriter, r *http.Request) {
	params, ok := decodeBody[mergeParameters](w, r)
	if !ok {
		return
	}

	s.dataMu.Lock()
	defer s.dataMu.Unlock()

	slug, index, ok := s.pullRequestIndex(r)
	if !ok {
		writePullRequestNotFound(w, r)
		return
	}
	pullRequest := &s.Data.PullRequests[slug][index]

	if !validMergeStrategy(params.MergeStrategy) {
		writeBadRequest(w, "merge_strategy: \""+string(params.MergeStrategy)+"\" is not a valid choice.")
		return
	}
	if pullRequest.State != bitbucket.PullRequestOpen {
		writeBadRequest(w, msgOnlyOpenCanBeMerged)
		return
	}
	if slug == repoWebApp && pullRequest.ID == prRefsChanged {
		writeErrorEnvelope(w, http.StatusConflict, msgRefsChanged)
		return
	}

	merged := mergedPullRequest(*pullRequest, slug, s.Data.User, params)
	if slug == repoAPIGateway && pullRequest.ID == prQueuedMerge {
		s.queueMerge(w, r, slug, merged)
		return
	}

	*pullRequest = merged
	writeJSON(w, merged)
}

// queueMerge answers the 202: the merge is accepted, nothing about the pull
// request has changed yet, and the Location names the task that will report
// when it has. Merging the same pull request again restarts its task.
//
// The response carries no document. The endpoint documents the Location header
// as the whole answer, and a client that decoded a body here would decode it
// into a pull request that is not there yet.
//
// The merged pull request is computed now and kept until the task reports it, so
// an edit made while the merge is queued is lost when it lands — which is what
// merging a moving target does anywhere.
func (s *Server) queueMerge(w http.ResponseWriter, r *http.Request, slug string, merged bitbucket.PullRequest) {
	taskID := mergeTaskID(merged.ID)
	s.mergeTasks[taskID] = &mergeTask{slug: slug, id: merged.ID, polls: 0, merged: merged}

	w.Header().Set("Location", mergeTaskURL(r, slug, merged.ID, taskID))
	w.WriteHeader(http.StatusAccepted)
}

// handleMergeTaskStatus polls a queued merge. The first poll answers PENDING
// and the ones after it answer SUCCESS, which is the shape a client's polling
// loop has to survive; the merge lands on the dataset with the first SUCCESS.
func (s *Server) handleMergeTaskStatus(w http.ResponseWriter, r *http.Request) {
	s.dataMu.Lock()
	defer s.dataMu.Unlock()

	slug, index, ok := s.pullRequestIndex(r)
	if !ok {
		writePullRequestNotFound(w, r)
		return
	}

	pullRequest := &s.Data.PullRequests[slug][index]
	taskID := r.PathValue("task")
	task, known := s.mergeTasks[taskID]
	if !known || task.slug != slug || task.id != pullRequest.ID {
		// The real endpoint answers 400, not 404, for a task id that names no
		// merge of this pull request.
		writeBadRequest(w, "No merge task "+taskID+" for pull request "+
			slug+"#"+strconv.Itoa(pullRequest.ID)+".")
		return
	}

	links := selfLink{Self: bitbucket.Link{Href: mergeTaskURL(r, slug, task.id, taskID)}}
	task.polls++
	if task.polls == 1 {
		writeJSON(w, mergeTaskEnvelope{Status: bitbucket.MergeTaskPending, Links: links, MergeResult: nil})
		return
	}

	*pullRequest = task.merged
	merged := task.merged
	writeJSON(w, mergeTaskEnvelope{Status: bitbucket.MergeTaskSuccess, Links: links, MergeResult: &merged})
}

// mergedPullRequest is the pull request a merge leaves behind: merged, closed by
// the account the demo runs as, and carrying a merge commit.
//
// updated_on is time.Now().UTC() rather than an offset from the dataset's base
// time, deliberately: the merge happens while the user watches, and a list
// ordered by update has to float it to the top the way the real one does.
func mergedPullRequest(
	pullRequest bitbucket.PullRequest, slug string, closedBy bitbucket.Account, params mergeParameters,
) bitbucket.PullRequest {
	merged := pullRequest
	merged.State = bitbucket.PullRequestMerged
	merged.ClosedBy = ptr(closedBy)
	merged.MergeCommit = newCommit(slug, mergeCommitHash(slug, pullRequest, params))
	if params.CloseSourceBranch != nil {
		merged.CloseSourceBranch = *params.CloseSourceBranch
	}
	merged.UpdatedOn = time.Now().UTC()
	return merged
}

// mergeCommitHash is the hash of the commit a merge produces, in the 12-char
// shape a pull request payload carries.
//
// There is no commit graph here to build one from, so it is derived from the
// merge instead — and from the whole merge, strategy and message included,
// which is the only way this fake can show that it read them: the same request
// always yields the same merge commit, and two different requests do not.
func mergeCommitHash(slug string, pullRequest bitbucket.PullRequest, params mergeParameters) string {
	digest := fnv.New64a()
	_, _ = fmt.Fprintf(digest, "merge:%s#%d:%s:%s:%s:%t",
		slug, pullRequest.ID, sourceHash(pullRequest), params.MergeStrategy, params.Message,
		params.CloseSourceBranch != nil && *params.CloseSourceBranch)
	return fmt.Sprintf("%016x", digest.Sum64())[:12]
}

// sourceHash is the pull request's head commit, or "" for a pull request whose
// payload carries none.
func sourceHash(pullRequest bitbucket.PullRequest) string {
	if pullRequest.Source.Commit == nil {
		return ""
	}
	return pullRequest.Source.Commit.Hash
}

// validMergeStrategy reports whether the body named a strategy the API accepts.
// The empty strategy is one of them: it means the member was left out.
func validMergeStrategy(strategy bitbucket.MergeStrategy) bool {
	if strategy == "" {
		return true
	}
	return slices.Contains(bitbucket.AllMergeStrategies(), strategy)
}

// mergeTaskID is the id a queued merge is polled under.
func mergeTaskID(id int) string {
	return "t-" + strconv.Itoa(id)
}

// mergeTaskURL is the absolute, same-origin URL of a merge task. It is built
// from the request's own Host for the same reason pagination links are: the
// client refuses to follow a link that leaves its base URL's origin.
func mergeTaskURL(r *http.Request, slug string, id int, taskID string) string {
	return "http://" + r.Host + "/repositories/" + Workspace + "/" + slug +
		"/pullrequests/" + strconv.Itoa(id) + "/merge/task-status/" + taskID
}

// -----------------------------------------------------------------------------
// Update
// -----------------------------------------------------------------------------

// handleUpdatePullRequest applies a partial pull request document: title,
// description, draft state and reviewers, in whatever combination the body
// carries.
//
// Reviewers are the exception to "a member the body leaves out is left alone".
// A PUT states the complete reviewer set, so a document without one clears it —
// which is exactly the trap bitbucket.PullRequestUpdate refuses to walk into,
// and the reason this fake reproduces it rather than being kind about it.
func (s *Server) handleUpdatePullRequest(w http.ResponseWriter, r *http.Request) {
	update, ok := decodeBody[pullRequestUpdate](w, r)
	if !ok {
		return
	}

	s.dataMu.Lock()
	defer s.dataMu.Unlock()

	slug, index, ok := s.pullRequestIndex(r)
	if !ok {
		writePullRequestNotFound(w, r)
		return
	}
	pullRequest := &s.Data.PullRequests[slug][index]

	if pullRequest.State != bitbucket.PullRequestOpen {
		writeBadRequest(w, msgOnlyOpenCanBeMutated)
		return
	}
	if update.Title != nil && *update.Title == "" {
		writeFieldsError(w, msgBadRequest, map[string][]string{fieldTitle: {msgTitleRequired}})
		return
	}

	reviewers, ok := s.resolveReviewers(w, *pullRequest, update.Reviewers)
	if !ok {
		return
	}

	if update.Title != nil {
		pullRequest.Title = *update.Title
	}
	if update.Description != nil {
		pullRequest.Description = *update.Description
		// The description is served three times — raw, as the summary's raw and
		// as the rendering the mention resolver reads the display names out of
		// — so all three have to move together, exactly as they do in
		// production when Bitbucket re-renders an edited description.
		pullRequest.Summary.Raw = *update.Description
		pullRequest.Summary.HTML = renderHTML(*update.Description, s.Data.Members)
	}
	if update.Draft != nil {
		pullRequest.Draft = *update.Draft
	}
	pullRequest.Reviewers = reviewers
	pullRequest.Participants = rebuildParticipants(pullRequest.Participants, reviewers)
	pullRequest.UpdatedOn = time.Now().UTC()

	writeJSON(w, *pullRequest)
}

// resolveReviewers turns the body's UUIDs into the accounts they name, in the
// order they were given. It writes the 400 itself — for an account the
// workspace does not have and for the author, whom Bitbucket refuses to make a
// reviewer of their own pull request — and reports ok false once it has.
func (s *Server) resolveReviewers(
	w http.ResponseWriter, pullRequest bitbucket.PullRequest, refs []reviewerRef,
) ([]bitbucket.Account, bool) {
	reviewers := make([]bitbucket.Account, 0, len(refs))
	for _, ref := range refs {
		if sameUUID(ref.UUID, pullRequest.Author.UUID) {
			writeBadRequest(w, msgAuthorCannotReview)
			return nil, false
		}
		account, found := s.memberByUUID(ref.UUID)
		if !found {
			writeBadRequest(w, "No account with UUID "+ref.UUID+" in this workspace.")
			return nil, false
		}
		reviewers = append(reviewers, account)
	}
	return reviewers, true
}

// memberByUUID is the workspace account with this UUID. The caller must hold
// dataMu.
func (s *Server) memberByUUID(uuid string) (bitbucket.Account, bool) {
	for _, member := range s.Data.Members {
		if sameUUID(member.UUID, uuid) {
			return member, true
		}
	}
	return bitbucket.Account{}, false
}

// rebuildParticipants derives the participant list from a new reviewer set: one
// REVIEWER row per reviewer, in the order the request named them, then everyone
// who is on the pull request for another reason.
//
// A reviewer who was already there keeps their decision — an approval survives
// having a second reviewer added — and a reviewer who was dropped disappears,
// approval included. The real API demotes an approver to PARTICIPANT instead of
// forgetting them; this is the one place the fake is simpler than production.
func rebuildParticipants(
	existing []bitbucket.Participant, reviewers []bitbucket.Account,
) []bitbucket.Participant {
	rebuilt := make([]bitbucket.Participant, 0, len(existing)+len(reviewers))
	for _, reviewer := range reviewers {
		rebuilt = append(rebuilt, reviewerParticipant(existing, reviewer))
	}
	for _, participant := range existing {
		if participant.Role != bitbucket.RoleReviewer {
			rebuilt = append(rebuilt, participant)
			continue
		}
		if isReviewer(reviewers, participant.User.UUID) || !hasActed(participant) {
			continue // re-derived above, or never acted: nothing to keep
		}
		// Production demotes a reviewer who approved or requested changes to a
		// plain participant rather than forgetting their verdict.
		participant.Role = bitbucket.RoleParticipant
		rebuilt = append(rebuilt, participant)
	}
	return rebuilt
}

// isReviewer reports whether uuid is in the reviewer set.
func isReviewer(reviewers []bitbucket.Account, uuid string) bool {
	for _, reviewer := range reviewers {
		if sameUUID(reviewer.UUID, uuid) {
			return true
		}
	}
	return false
}

// hasActed reports whether the participant approved or requested changes.
func hasActed(participant bitbucket.Participant) bool {
	return participant.Approved || participant.State != ""
}

// reviewerParticipant is the participant row for one reviewer: the one they
// already had, promoted to REVIEWER, or a fresh row that has not acted.
func reviewerParticipant(existing []bitbucket.Participant, reviewer bitbucket.Account) bitbucket.Participant {
	for _, participant := range existing {
		if sameUUID(participant.User.UUID, reviewer.UUID) {
			participant.Role = bitbucket.RoleReviewer
			return participant
		}
	}
	return newParticipant(reviewer, bitbucket.RoleReviewer, "", time.Time{})
}

// -----------------------------------------------------------------------------
// Approve
// -----------------------------------------------------------------------------

// handleApprovePullRequest records the demo user's approval and answers with
// the participant row it left behind — which is what the endpoint returns, a
// participant rather than a pull request.
//
// Who approves is not in the request: there is no body, and the demo runs as
// Data.User the way a real session runs as whoever the token belongs to.
func (s *Server) handleApprovePullRequest(w http.ResponseWriter, r *http.Request) {
	s.dataMu.Lock()
	defer s.dataMu.Unlock()

	pullRequest, ok := s.approvalTarget(w, r)
	if !ok {
		return
	}

	user := s.Data.User
	approved := newParticipant(user, approvalRole(*pullRequest, user),
		bitbucket.ParticipantApproved, time.Now().UTC())

	replaced := false
	for index, participant := range pullRequest.Participants {
		if !sameUUID(participant.User.UUID, user.UUID) {
			continue
		}
		// An existing row keeps the role it was on: someone whose reviewer
		// request was withdrawn after they commented is still a PARTICIPANT.
		approved.Role = participant.Role
		pullRequest.Participants[index] = approved
		replaced = true
		break
	}
	if !replaced {
		pullRequest.Participants = append(pullRequest.Participants, approved)
	}

	writeJSON(w, approved)
}

// handleUnapprovePullRequest withdraws the demo user's approval and answers 204
// with no document, exactly as the real endpoint does.
//
// A row that was only there to carry the approval goes with it; a requested
// reviewer stays on the pull request with nothing decided, because they are
// still expected to review it.
func (s *Server) handleUnapprovePullRequest(w http.ResponseWriter, r *http.Request) {
	s.dataMu.Lock()
	defer s.dataMu.Unlock()

	pullRequest, ok := s.approvalTarget(w, r)
	if !ok {
		return
	}

	user := s.Data.User
	remaining := make([]bitbucket.Participant, 0, len(pullRequest.Participants))
	for _, participant := range pullRequest.Participants {
		if sameUUID(participant.User.UUID, user.UUID) {
			participant.Approved = false
			participant.State = ""
			participant.ParticipatedOn = time.Time{}
			if participant.Role != bitbucket.RoleReviewer {
				continue
			}
		}
		remaining = append(remaining, participant)
	}
	pullRequest.Participants = remaining

	w.WriteHeader(http.StatusNoContent)
}

// approvalTarget is the pull request an approval verb acts on, or false with
// the refusal already written: a 404 for one that is not there, and the 400
// Bitbucket answers an author who tries to approve their own work.
// The caller must hold dataMu for writing.
func (s *Server) approvalTarget(w http.ResponseWriter, r *http.Request) (*bitbucket.PullRequest, bool) {
	slug, index, ok := s.pullRequestIndex(r)
	if !ok {
		writePullRequestNotFound(w, r)
		return nil, false
	}

	pullRequest := &s.Data.PullRequests[slug][index]
	if sameUUID(pullRequest.Author.UUID, s.Data.User.UUID) {
		writeBadRequest(w, msgCannotApproveOwn)
		return nil, false
	}
	return pullRequest, true
}

// approvalRole is the role a fresh participant row is recorded under: REVIEWER
// for someone who was asked to review, PARTICIPANT for anyone else who acts.
func approvalRole(pullRequest bitbucket.PullRequest, user bitbucket.Account) bitbucket.ParticipantRole {
	if isReviewer(pullRequest.Reviewers, user.UUID) {
		return bitbucket.RoleReviewer
	}
	return bitbucket.RoleParticipant
}

// -----------------------------------------------------------------------------
// Reviewer candidates
// -----------------------------------------------------------------------------

// handleEffectiveDefaultReviewers serves the repository's default reviewers,
// the ones inherited from its project included — which is what "effective"
// means and why each entry states where it came from.
func (s *Server) handleEffectiveDefaultReviewers(w http.ResponseWriter, r *http.Request) {
	s.dataMu.RLock()
	defer s.dataMu.RUnlock()

	if _, ok := s.findRepository(r); !ok {
		writeRepositoryNotFound(w, r)
		return
	}

	accounts := s.Data.DefaultReviewers[r.PathValue("slug")]
	entries := make([]defaultReviewerEntry, 0, len(accounts))
	for index, account := range accounts {
		reviewerType := "repository"
		if index == 0 {
			reviewerType = "project"
		}
		entries = append(entries, defaultReviewerEntry{
			Type:         "default_reviewer_and_type",
			ReviewerType: reviewerType,
			User:         account,
		})
	}
	writeList(w, r, entries, s.maxPageLen())
}

// handleWorkspaceMembers serves the workspace's accounts, each wrapped in the
// membership object the collection actually carries.
func (s *Server) handleWorkspaceMembers(w http.ResponseWriter, r *http.Request) {
	s.dataMu.RLock()
	defer s.dataMu.RUnlock()

	if r.PathValue("ws") != Workspace {
		writeNotFound(w, "No workspace with identifier '"+r.PathValue("ws")+"'.")
		return
	}

	memberships := make([]workspaceMembership, 0, len(s.Data.Members))
	for _, member := range s.Data.Members {
		memberships = append(memberships, workspaceMembership{
			Type:      "workspace_membership",
			User:      member,
			Workspace: s.Data.Workspace,
		})
	}
	writeList(w, r, memberships, s.maxPageLen())
}

// -----------------------------------------------------------------------------
// Bodies and validation errors
// -----------------------------------------------------------------------------

// decodeBody reads the request body as a JSON document of type T. An empty body
// is an empty document, which is what the merge endpoint accepts; anything that
// is not JSON is a 400. ok is false once a response has been written.
func decodeBody[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var value T

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeBadRequest(w, "Could not read the request body: "+err.Error())
		return value, false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return value, true
	}
	if err := json.Unmarshal(body, &value); err != nil {
		writeBadRequest(w, "Could not parse the request body as JSON: "+err.Error())
		return value, false
	}
	return value, true
}

// writeFieldsError answers the 400 a write is rejected with when the fault is
// in a named member of the request document. The per-field messages are what
// makes the failure actionable — "This field is required." says more than the
// envelope's own sentence — so they travel in error.fields the way the real API
// sends them.
func writeFieldsError(w http.ResponseWriter, message string, fields map[string][]string) {
	writeJSONStatus(w, http.StatusBadRequest, errorEnvelope{
		Type:  "error",
		Error: errorMessage{Message: message, Fields: fields},
	})
}
