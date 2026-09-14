package demo

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
)

// POST .../pullrequests: opening one. It mirrors the production validation the
// TUI has to survive — a missing title, a source branch the repository does not
// have, the author as their own reviewer, and a pull request already open for
// the same pair of branches — and answers 201 with the pull request it created,
// which is where its id comes from.

// The messages the real endpoint answers these faults with.
const (
	msgSourceBranchNotFound = "Source branch does not exist in the repository."
	msgSameBranch           = "The source and destination branches must be different."
	msgDuplicatePR          = "A pull request already exists for this source and destination."
)

// The request members a refusal is reported against.
const (
	fieldSource      = "source"
	fieldTitleCreate = "title"
)

// demoBranches is the set of branch names the fake repositories have: the
// repository's own main branch plus the source of every pull request it
// already carries. A create names one of these or is refused, because a branch
// that is only in someone's checkout is a 400 in production too.
func (s *Server) demoBranches(slug string) []string {
	branches := []string{}
	for _, meta := range repoMetas() {
		if meta.slug == slug {
			branches = append(branches, meta.mainBranch)
		}
	}
	for _, pullRequest := range s.Data.PullRequests[slug] {
		branches = append(branches,
			pullRequest.Source.Branch.Name, pullRequest.Destination.Branch.Name)
	}
	return branches
}

// newPullRequestRequest is the create document, in the shape the client sends.
type newPullRequestRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Source      struct {
		Branch struct {
			Name string `json:"name"`
		} `json:"branch"`
	} `json:"source"`
	Destination *struct {
		Branch struct {
			Name string `json:"name"`
		} `json:"branch"`
	} `json:"destination"`
	Reviewers []struct {
		UUID string `json:"uuid"`
	} `json:"reviewers"`
	Draft             bool `json:"draft"`
	CloseSourceBranch bool `json:"close_source_branch"`
}

// handleCreatePullRequest opens a pull request authored by the demo user.
func (s *Server) handleCreatePullRequest(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeBody[newPullRequestRequest](w, r)
	if !ok {
		return
	}

	s.dataMu.Lock()
	defer s.dataMu.Unlock()

	repository, ok := s.findRepository(r)
	if !ok {
		writeRepositoryNotFound(w, r)
		return
	}
	slug := repository.Slug

	created, ok := s.validateNewPullRequest(w, slug, request)
	if !ok {
		return
	}

	s.Data.PullRequests[slug] = append([]bitbucket.PullRequest{created}, s.Data.PullRequests[slug]...)
	writeJSONStatus(w, http.StatusCreated, created)
}

// validateNewPullRequest answers the refusals and otherwise builds the pull
// request the request asked for. The caller holds dataMu.
func (s *Server) validateNewPullRequest(
	w http.ResponseWriter, slug string, request newPullRequestRequest,
) (bitbucket.PullRequest, bool) {
	if strings.TrimSpace(request.Title) == "" {
		writeFieldsError(w, msgBadRequest, map[string][]string{fieldTitleCreate: {msgTitleRequired}})
		return bitbucket.PullRequest{}, false
	}

	source := request.Source.Branch.Name
	destination := mainBranchOf(slug)
	if request.Destination != nil && request.Destination.Branch.Name != "" {
		destination = request.Destination.Branch.Name
	}

	branches := s.demoBranches(slug)
	switch {
	case source == "" || !slices.Contains(branches, source):
		writeFieldsError(w, msgBadRequest, map[string][]string{fieldSource: {msgSourceBranchNotFound}})
		return bitbucket.PullRequest{}, false
	case source == destination:
		writeBadRequest(w, msgSameBranch)
		return bitbucket.PullRequest{}, false
	}

	for _, existing := range s.Data.PullRequests[slug] {
		if existing.State == bitbucket.PullRequestOpen &&
			existing.Source.Branch.Name == source &&
			existing.Destination.Branch.Name == destination {
			writeBadRequest(w, msgDuplicatePR)
			return bitbucket.PullRequest{}, false
		}
	}

	reviewers, ok := s.resolveNewReviewers(w, request)
	if !ok {
		return bitbucket.PullRequest{}, false
	}
	return s.buildNewPullRequest(slug, source, destination, request, reviewers), true
}

// resolveNewReviewers turns the requested uuids into accounts, refusing the
// author among them the way the update route does.
func (s *Server) resolveNewReviewers(
	w http.ResponseWriter, request newPullRequestRequest,
) ([]bitbucket.Account, bool) {
	reviewers := make([]bitbucket.Account, 0, len(request.Reviewers))
	for _, ref := range request.Reviewers {
		if sameUUID(ref.UUID, s.Data.User.UUID) {
			// The demo user authors everything created here.
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

// buildNewPullRequest is the pull request the create leaves behind: open,
// authored by the demo user, timestamped now and numbered one past the highest
// id in the whole dataset — ids are per repository in Bitbucket, but a demo
// that reuses one makes two pull requests indistinguishable in a key.
func (s *Server) buildNewPullRequest(
	slug, source, destination string, request newPullRequestRequest, reviewers []bitbucket.Account,
) bitbucket.PullRequest {
	id := s.nextPullRequestID()
	participants := make([]bitbucket.Participant, 0, len(reviewers)+1)
	participants = append(participants,
		newParticipant(s.Data.User, bitbucket.RoleParticipant, "", time.Time{}))
	for _, reviewer := range reviewers {
		participants = append(participants,
			newParticipant(reviewer, bitbucket.RoleReviewer, "", time.Time{}))
	}

	created := newPullRequest(prSpec{
		repo: slug, id: id, title: strings.TrimSpace(request.Title),
		description: request.Description, state: bitbucket.PullRequestOpen,
		draft: request.Draft, author: s.Data.User,
		reviewers: reviewers, participants: participants,
		source: source, destination: destination,
		hash: commitHash(id, 1), destHash: commitHash(id, 0),
		created: 0, updated: 0, closedBy: nil, mergeHash: "", reason: "",
		comments: 0, tasks: 0,
	})
	created.CloseSourceBranch = request.CloseSourceBranch
	// A pull request opened now, not one the dataset was written around.
	now := time.Now().UTC()
	created.CreatedOn, created.UpdatedOn = now, now
	return created
}

// mainBranchOf is a repository's default branch, which is what a create with no
// destination lands on.
func mainBranchOf(slug string) string {
	for _, meta := range repoMetas() {
		if meta.slug == slug {
			return meta.mainBranch
		}
	}
	return branchMain
}

// nextPullRequestID is one past the highest pull request id in the dataset. The
// caller holds dataMu.
func (s *Server) nextPullRequestID() int {
	highest := 0
	for _, pullRequests := range s.Data.PullRequests {
		for i := range pullRequests {
			if pullRequests[i].ID > highest {
				highest = pullRequests[i].ID
			}
		}
	}
	return highest + 1
}
