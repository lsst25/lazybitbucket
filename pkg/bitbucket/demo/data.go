// Package demo holds the fixed dataset and the in-process fake REST server that
// back `lazybitbucket --demo` and the integration tests.
//
// Unlike lazyjira's demo server this is an ordinary, untagged package, so it is
// compiled, vetted and tested by `go test ./...` and any test may stand a real
// bitbucket.Client in front of it.
//
// Nothing here is random: every identifier is a literal and every timestamp is
// an offset from a fixed base time, so two datasets built in different runs are
// byte-identical.
package demo

import (
	"fmt"
	"maps"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
)

// Workspace is the only workspace the demo dataset knows about.
const Workspace = "acme"

// Repository slugs in the demo workspace.
const (
	repoAPIGateway = "api-gateway"
	repoWebApp     = "web-app"
	repoInfra      = "infra"
)

const (
	apiBase  = "https://api.bitbucket.org/2.0"
	htmlBase = "https://bitbucket.org"
)

// Account UUIDs. Braces included, exactly as Bitbucket sends them.
const (
	uuidDemo    = "{00000000-0000-0000-0000-000000000001}"
	uuidAda     = "{00000000-0000-0000-0000-000000000002}"
	uuidGrace   = "{00000000-0000-0000-0000-000000000003}"
	uuidLinus   = "{00000000-0000-0000-0000-000000000004}"
	uuidKen     = "{00000000-0000-0000-0000-000000000005}"
	uuidBarbara = "{00000000-0000-0000-0000-000000000006}"
)

// Pipeline and step state/result words, spelled as they appear in state.name.
const (
	stateCompleted  = "COMPLETED"
	stateInProgress = "IN_PROGRESS"
	statePending    = "PENDING"
	stageRunning    = "RUNNING"

	resultSuccessful = "SUCCESSFUL"
	resultFailed     = "FAILED"
	resultStopped    = "STOPPED"
	resultNotRun     = "NOT_RUN"
)

const (
	branchMain    = "main"
	branchDevelop = "develop"
)

// Commit hashes. Bitbucket sends 40 hex characters everywhere except inside a
// pull request payload, where they are abbreviated to 12 — the dataset stores
// the full hash and shortHash derives the abbreviated form, so both shapes stay
// consistent. Written as three groups of 16/16/8 so the length is verifiable by
// eye.
const (
	hashPR101 = "9c1d0a4b7e2f3658" + "1c0d9a8b7e6f5d4c" + "3b2a1908"
	hashPR102 = "5e7a2c9b1d3f4608" + "a7b6c5d4e3f2a1b0" + "c9d8e7f6"
	hashPR103 = "b4d6f8a0c2e41357" + "9bd02468ace13579" + "0f1e2d3c"
	hashPR98  = "77e5c3a1f9d7b5a3" + "918f7e6d5c4b3a29" + "18070605"
	hashPR95  = "0a1b2c3d4e5f6071" + "82930a1b2c3d4e5f" + "60718293"
	hashPR40  = "cafe0babdeadbeef" + "0123456789abcdef" + "fedcba98"
	hashPR41  = "aa11bb22cc33dd44" + "ee55ff6600778899" + "a0b1c2d3"
	hashPR37  = "1234abcd5678ef90" + "1234abcd5678ef90" + "deadc0de"
	hashPR7   = "f0e1d2c3b4a59687" + "78695a4b3c2d1e0f" + "00112233"
	hashPR5   = "5a5b5c5d5e5f6061" + "6263646566676869" + "6a6b6c6d"

	hashMergePR98 = "3e3f404142434445" + "464748494a4b4c4d" + "4e4f5051"
	hashMergePR37 = "6162636465666768" + "696a6b6c6d6e6f70" + "71727374"
	hashMergePR5  = "8182838485868788" + "898a8b8c8d8e8f90" + "91929394"

	// Tip of each repository's main branch, used as the destination commit.
	hashMainAPI    = "1a2b3c4d5e6f7081" + "9293a4b5c6d7e8f9" + "01234567"
	hashDevelopWeb = "3c3d3e3f40414243" + "4445464748494a4b" + "4c4d4e4f"
	hashMainInfra  = "9182736455463728" + "1908a7b6c5d4e3f2" + "a1b0c9d8"
)

// baseTime anchors every timestamp in the dataset.
var baseTime = time.Date(2026, time.March, 16, 9, 0, 0, 0, time.UTC)

const (
	hour = time.Hour
	day  = 24 * time.Hour
)

// ago is a timestamp d before the dataset's fixed base time.
func ago(d time.Duration) time.Time { return baseTime.Add(-d) }

// shortHash is the 12-character abbreviation Bitbucket uses inside pull request
// payloads.
func shortHash(hash string) string {
	const abbreviated = 12
	if len(hash) <= abbreviated {
		return hash
	}
	return hash[:abbreviated]
}

// Dataset is the whole demo world: one workspace, three repositories and the
// pull requests, build statuses, pipeline runs, steps and logs hanging off them.
type Dataset struct {
	User         bitbucket.Account
	Workspace    bitbucket.Workspace
	Repositories []bitbucket.Repository
	PullRequests map[string][]bitbucket.PullRequest  // repo slug → PRs, newest first, all states
	Statuses     map[string][]bitbucket.CommitStatus // bitbucket.PullRequestKey → statuses
	Pipelines    map[string][]bitbucket.Pipeline     // repo slug → runs, newest first
	Steps        map[string][]bitbucket.PipelineStep // pipeline UUID → steps, in run order
	Logs         map[string]string                   // step UUID → log text
}

// NewDataset builds the demo dataset. It allocates a fresh copy every call, so a
// caller may mutate the result without disturbing anyone else.
func NewDataset() *Dataset {
	data := &Dataset{
		User:         demoUser(),
		Workspace:    demoWorkspace(),
		Repositories: demoRepositories(),
		PullRequests: map[string][]bitbucket.PullRequest{},
		Statuses:     map[string][]bitbucket.CommitStatus{},
		Pipelines:    map[string][]bitbucket.Pipeline{},
		Steps:        map[string][]bitbucket.PipelineStep{},
		Logs:         map[string]string{},
	}

	for _, spec := range pullRequestSpecs() {
		data.PullRequests[spec.repo] = append(data.PullRequests[spec.repo], newPullRequest(spec))
	}
	for _, spec := range statusSpecs() {
		key := bitbucket.PullRequestKey(Workspace, spec.repo, spec.pullRequest)
		data.Statuses[key] = append(data.Statuses[key], newCommitStatus(spec))
	}
	for _, spec := range pipelineSpecs() {
		pipeline, steps, logs := newPipeline(spec)
		data.Pipelines[spec.repo] = append(data.Pipelines[spec.repo], pipeline)
		data.Steps[pipeline.UUID] = steps
		maps.Copy(data.Logs, logs)
	}
	return data
}

// Newest is the most recent updated_on across every pull request in the
// dataset, i.e. how old the world looks to the pull request pane. It is the
// zero time for a dataset with no pull requests.
func (d *Dataset) Newest() time.Time {
	var newest time.Time
	for _, pullRequests := range d.PullRequests {
		for index := range pullRequests {
			if pullRequests[index].UpdatedOn.After(newest) {
				newest = pullRequests[index].UpdatedOn
			}
		}
	}
	return newest
}

// Shift moves every timestamp in the dataset by delta, preserving all the
// intervals between them. `--demo` uses it to slide the fixed dataset up to the
// present so relative times ("2h ago") read as if the world were live.
//
// Zero timestamps are left alone: a zero time.Time means "absent" on the wire
// (a running pipeline has no completed_on, a reviewer who has not acted has no
// participated_on), and shifting one would invent a value.
//
// The walk is explicit rather than reflective, so adding a timestamped field to
// a model is a compile-silent omission here — the Shift tests are what catch it.
func (d *Dataset) Shift(delta time.Duration) {
	if delta == 0 {
		return
	}

	shiftTime(&d.Workspace.CreatedOn, delta)

	for index := range d.Repositories {
		repository := &d.Repositories[index]
		shiftTime(&repository.CreatedOn, delta)
		shiftTime(&repository.UpdatedOn, delta)
	}

	for _, pullRequests := range d.PullRequests {
		for index := range pullRequests {
			pullRequest := &pullRequests[index]
			shiftTime(&pullRequest.CreatedOn, delta)
			shiftTime(&pullRequest.UpdatedOn, delta)
			for participant := range pullRequest.Participants {
				shiftTime(&pullRequest.Participants[participant].ParticipatedOn, delta)
			}
		}
	}

	for _, statuses := range d.Statuses {
		for index := range statuses {
			shiftTime(&statuses[index].CreatedOn, delta)
			shiftTime(&statuses[index].UpdatedOn, delta)
		}
	}

	for _, pipelines := range d.Pipelines {
		for index := range pipelines {
			shiftTime(&pipelines[index].CreatedOn, delta)
			shiftTime(&pipelines[index].CompletedOn, delta)
		}
	}

	for _, steps := range d.Steps {
		for index := range steps {
			shiftTime(&steps[index].StartedOn, delta)
			shiftTime(&steps[index].CompletedOn, delta)
		}
	}
}

// shiftTime moves a timestamp by delta unless it is the zero time, which stands
// for "field absent" and must stay that way.
func shiftTime(value *time.Time, delta time.Duration) {
	if value.IsZero() {
		return
	}
	*value = value.Add(delta)
}

// -----------------------------------------------------------------------------
// Accounts
// -----------------------------------------------------------------------------

func newAccount(uuid, displayName, nickname string) bitbucket.Account {
	account := bitbucket.Account{
		Type:        "user",
		UUID:        uuid,
		AccountID:   "712020:" + nickname,
		DisplayName: displayName,
		Nickname:    nickname,
	}
	escaped := url.PathEscape(uuid)
	account.Links.Self = bitbucket.Link{Href: apiBase + "/users/" + escaped}
	account.Links.HTML = bitbucket.Link{Href: htmlBase + "/" + escaped + "/"}
	account.Links.Avatar = bitbucket.Link{Href: htmlBase + "/account/" + nickname + "/avatar/"}
	return account
}

func demoUser() bitbucket.Account { return newAccount(uuidDemo, "Demo User", "demo") }
func ada() bitbucket.Account      { return newAccount(uuidAda, "Ada Lovelace", "ada") }
func grace() bitbucket.Account    { return newAccount(uuidGrace, "Grace Hopper", "grace") }
func linus() bitbucket.Account    { return newAccount(uuidLinus, "Linus Torvalds", "linus") }
func ken() bitbucket.Account      { return newAccount(uuidKen, "Ken Thompson", "ken") }
func barbara() bitbucket.Account  { return newAccount(uuidBarbara, "Barbara Liskov", "barbara") }

func newParticipant(
	user bitbucket.Account,
	role bitbucket.ParticipantRole,
	state bitbucket.ParticipantState,
	on time.Time,
) bitbucket.Participant {
	return bitbucket.Participant{
		Type:           "participant",
		User:           user,
		Role:           role,
		Approved:       state == bitbucket.ParticipantApproved,
		State:          state,
		ParticipatedOn: on,
	}
}

// -----------------------------------------------------------------------------
// Workspace and repositories
// -----------------------------------------------------------------------------

func demoWorkspace() bitbucket.Workspace {
	workspace := bitbucket.Workspace{
		Type:      "workspace",
		UUID:      "{11111111-1111-1111-1111-111111111111}",
		Slug:      Workspace,
		Name:      "Acme Corp",
		IsPrivate: true,
		CreatedOn: ago(900 * day),
	}
	workspace.Links.Self = bitbucket.Link{Href: apiBase + "/workspaces/" + Workspace}
	workspace.Links.HTML = bitbucket.Link{Href: htmlBase + "/" + Workspace + "/"}
	workspace.Links.Avatar = bitbucket.Link{Href: htmlBase + "/workspaces/" + Workspace + "/avatar/"}
	return workspace
}

// repoMeta is the per-repository constant data the builders need.
type repoMeta struct {
	slug        string
	name        string
	uuid        string
	description string
	language    string
	mainBranch  string
	projectKey  string
	projectName string
	projectUUID string
	size        int64
	created     time.Duration // before baseTime
	updated     time.Duration // before baseTime
}

func repoMetas() []repoMeta {
	return []repoMeta{
		{
			slug:        repoAPIGateway,
			name:        "API Gateway",
			uuid:        "{a0000000-0000-0000-0000-000000000001}",
			description: "Edge routing, authentication and rate limiting for the Acme public API.",
			language:    "go",
			mainBranch:  branchMain,
			projectKey:  "PLAT",
			projectName: "Platform",
			projectUUID: "{b0000000-0000-0000-0000-000000000001}",
			size:        18_432_512,
			created:     620 * day,
			updated:     2 * hour,
		},
		{
			slug:        repoWebApp,
			name:        "Web App",
			uuid:        "{a0000000-0000-0000-0000-000000000002}",
			description: "Customer-facing single page application.",
			language:    "typescript",
			mainBranch:  branchDevelop,
			projectKey:  "PLAT",
			projectName: "Platform",
			projectUUID: "{b0000000-0000-0000-0000-000000000001}",
			size:        73_105_920,
			created:     540 * day,
			updated:     3 * hour,
		},
		{
			slug:        repoInfra,
			name:        "Infrastructure",
			uuid:        "{a0000000-0000-0000-0000-000000000003}",
			description: "Terraform modules and cluster manifests.",
			language:    "hcl",
			mainBranch:  branchMain,
			projectKey:  "OPS",
			projectName: "Operations",
			projectUUID: "{b0000000-0000-0000-0000-000000000002}",
			size:        4_194_304,
			created:     700 * day,
			updated:     8 * hour,
		},
	}
}

func repoMetaFor(slug string) repoMeta {
	for _, meta := range repoMetas() {
		if meta.slug == slug {
			return meta
		}
	}
	return repoMeta{
		slug:        slug,
		name:        slug,
		uuid:        "{a0000000-0000-0000-0000-000000000000}",
		description: "",
		language:    "",
		mainBranch:  branchMain,
		projectKey:  "PLAT",
		projectName: "Platform",
		projectUUID: "{b0000000-0000-0000-0000-000000000000}",
		size:        0,
		created:     day,
		updated:     hour,
	}
}

func fullName(slug string) string { return Workspace + "/" + slug }

// repoRef is the abbreviated repository Bitbucket embeds in pull requests,
// statuses and pipelines.
func repoRef(slug string) bitbucket.RepositoryRef {
	meta := repoMetaFor(slug)
	full := fullName(slug)
	ref := bitbucket.RepositoryRef{
		Type:     "repository",
		UUID:     meta.uuid,
		Name:     meta.name,
		FullName: full,
	}
	ref.Links.Self = bitbucket.Link{Href: apiBase + "/repositories/" + full}
	ref.Links.HTML = bitbucket.Link{Href: htmlBase + "/" + full}
	ref.Links.Avatar = bitbucket.Link{
		Href: "https://bytebucket.org/ravatar/" + url.PathEscape(meta.uuid) + "?ts=default",
	}
	return ref
}

func demoRepositories() []bitbucket.Repository {
	metas := repoMetas()
	repositories := make([]bitbucket.Repository, 0, len(metas))
	for _, meta := range metas {
		repositories = append(repositories, newRepository(meta))
	}
	return repositories
}

func newRepository(meta repoMeta) bitbucket.Repository {
	full := fullName(meta.slug)

	project := &bitbucket.ProjectRef{
		Type: "project",
		UUID: meta.projectUUID,
		Key:  meta.projectKey,
		Name: meta.projectName,
	}
	project.Links.Self = bitbucket.Link{Href: apiBase + "/workspaces/" + Workspace + "/projects/" + meta.projectKey}
	project.Links.HTML = bitbucket.Link{Href: htmlBase + "/" + Workspace + "/workspace/projects/" + meta.projectKey}

	workspaceRef := &bitbucket.WorkspaceRef{
		Type: "workspace",
		UUID: "{11111111-1111-1111-1111-111111111111}",
		Slug: Workspace,
		Name: "Acme Corp",
	}
	workspaceRef.Links.Self = bitbucket.Link{Href: apiBase + "/workspaces/" + Workspace}
	workspaceRef.Links.HTML = bitbucket.Link{Href: htmlBase + "/" + Workspace + "/"}

	owner := demoUser()
	repository := bitbucket.Repository{
		Type:        "repository",
		UUID:        meta.uuid,
		Slug:        meta.slug,
		Name:        meta.name,
		FullName:    full,
		Description: meta.description,
		SCM:         "git",
		Language:    meta.language,
		IsPrivate:   true,
		Size:        meta.size,
		MainBranch:  &bitbucket.Branch{Name: meta.mainBranch, Type: "branch"},
		Project:     project,
		Workspace:   workspaceRef,
		Owner:       &owner,
		CreatedOn:   ago(meta.created),
		UpdatedOn:   ago(meta.updated),
	}
	repository.Links = bitbucket.RepositoryLinks{
		Self:         bitbucket.Link{Href: apiBase + "/repositories/" + full},
		HTML:         bitbucket.Link{Href: htmlBase + "/" + full},
		Avatar:       bitbucket.Link{Href: "https://bytebucket.org/ravatar/" + url.PathEscape(meta.uuid) + "?ts=default"},
		PullRequests: bitbucket.Link{Href: apiBase + "/repositories/" + full + "/pullrequests"},
		Commits:      bitbucket.Link{Href: apiBase + "/repositories/" + full + "/commits"},
		Branches:     bitbucket.Link{Href: apiBase + "/repositories/" + full + "/refs/branches"},
		Clone: []bitbucket.Link{
			{Href: "https://bitbucket.org/" + full + ".git", Name: "https"},
			{Href: "git@bitbucket.org:" + full + ".git", Name: "ssh"},
		},
	}
	return repository
}

// newCommit builds the abbreviated commit object embedded in pull requests,
// statuses and pipeline targets. Pass the hash in the length the surrounding
// payload uses — 12 inside a pull request, 40 everywhere else.
func newCommit(slug, hash string) *bitbucket.CommitRef {
	full := fullName(slug)
	commit := &bitbucket.CommitRef{Type: "commit", Hash: hash}
	commit.Links.Self = bitbucket.Link{Href: apiBase + "/repositories/" + full + "/commit/" + hash}
	commit.Links.HTML = bitbucket.Link{Href: htmlBase + "/" + full + "/commits/" + hash}
	return commit
}

// -----------------------------------------------------------------------------
// Pull requests
// -----------------------------------------------------------------------------

// markdown joins description lines and terminates with a newline, so the
// fixtures can hold fenced code blocks that a Go raw string literal cannot.
func markdown(lines ...string) string {
	return strings.Join(lines, "\n") + "\n"
}

type prSpec struct {
	repo         string
	id           int
	title        string
	description  string
	state        bitbucket.PullRequestState
	draft        bool
	author       bitbucket.Account
	reviewers    []bitbucket.Account
	participants []bitbucket.Participant
	source       string
	destination  string
	hash         string // 40-char head commit
	destHash     string // 40-char destination tip
	created      time.Duration
	updated      time.Duration
	closedBy     *bitbucket.Account
	mergeHash    string
	reason       string
	comments     int
	tasks        int
}

func newPullRequestLinks(slug string, id int) bitbucket.PullRequestLinks {
	full := fullName(slug)
	number := strconv.Itoa(id)
	self := apiBase + "/repositories/" + full + "/pullrequests/" + number
	return bitbucket.PullRequestLinks{
		Self:           bitbucket.Link{Href: self},
		HTML:           bitbucket.Link{Href: htmlBase + "/" + full + "/pull-requests/" + number},
		Commits:        bitbucket.Link{Href: self + "/commits"},
		Approve:        bitbucket.Link{Href: self + "/approve"},
		RequestChanges: bitbucket.Link{Href: self + "/request-changes"},
		Diff:           bitbucket.Link{Href: self + "/diff"},
		DiffStat:       bitbucket.Link{Href: self + "/diffstat"},
		Comments:       bitbucket.Link{Href: self + "/comments"},
		Activity:       bitbucket.Link{Href: self + "/activity"},
		Merge:          bitbucket.Link{Href: self + "/merge"},
		Decline:        bitbucket.Link{Href: self + "/decline"},
		Statuses:       bitbucket.Link{Href: self + "/statuses"},
	}
}

func newPullRequest(spec prSpec) bitbucket.PullRequest {
	pullRequest := bitbucket.PullRequest{
		Type:              "pullrequest",
		ID:                spec.id,
		Title:             spec.title,
		Description:       spec.description,
		State:             spec.state,
		Draft:             spec.draft,
		Queued:            false,
		CloseSourceBranch: true,
		Author:            spec.author,
		ClosedBy:          spec.closedBy,
		Reason:            spec.reason,
		CommentCount:      spec.comments,
		TaskCount:         spec.tasks,
		CreatedOn:         ago(spec.created),
		UpdatedOn:         ago(spec.updated),
		Source: bitbucket.PullRequestEndpoint{
			Branch:     bitbucket.Branch{Name: spec.source, Type: "branch"},
			Commit:     newCommit(spec.repo, shortHash(spec.hash)),
			Repository: repoRef(spec.repo),
		},
		Destination: bitbucket.PullRequestEndpoint{
			Branch:     bitbucket.Branch{Name: spec.destination, Type: "branch"},
			Commit:     newCommit(spec.repo, shortHash(spec.destHash)),
			Repository: repoRef(spec.repo),
		},
		Summary: bitbucket.RenderedContent{
			Type:   "rendered",
			Raw:    spec.description,
			Markup: "markdown",
			HTML:   "",
		},
		Reviewers:    spec.reviewers,
		Participants: spec.participants,
		Links:        newPullRequestLinks(spec.repo, spec.id),
	}
	if spec.mergeHash != "" {
		pullRequest.MergeCommit = newCommit(spec.repo, shortHash(spec.mergeHash))
	}
	return pullRequest
}

func pullRequestSpecs() []prSpec {
	return []prSpec{
		{
			repo:        repoAPIGateway,
			id:          103,
			title:       "Fix upstream timeout handling on retried requests",
			description: descriptionPR103(),
			state:       bitbucket.PullRequestOpen,
			author:      linus(),
			reviewers:   []bitbucket.Account{demoUser()},
			participants: []bitbucket.Participant{
				newParticipant(demoUser(), bitbucket.RoleReviewer,
					bitbucket.ParticipantChangesRequested, ago(2*hour)),
				newParticipant(ken(), bitbucket.RoleParticipant, "", time.Time{}),
			},
			source:      "bugfix/upstream-timeout",
			destination: branchMain,
			hash:        hashPR103,
			destHash:    hashMainAPI,
			created:     2 * day,
			updated:     2 * hour,
			comments:    6,
			tasks:       2,
		},
		{
			repo:        repoAPIGateway,
			id:          102,
			title:       "Introduce OAuth token introspection",
			description: descriptionPR102(),
			state:       bitbucket.PullRequestOpen,
			draft:       true,
			author:      grace(),
			reviewers:   []bitbucket.Account{demoUser(), linus()},
			participants: []bitbucket.Participant{
				newParticipant(demoUser(), bitbucket.RoleReviewer, "", time.Time{}),
				newParticipant(linus(), bitbucket.RoleReviewer, "", time.Time{}),
			},
			source:      "feature/oauth-introspection",
			destination: branchMain,
			hash:        hashPR102,
			destHash:    hashMainAPI,
			created:     day,
			updated:     5 * hour,
			comments:    1,
		},
		{
			repo:        repoAPIGateway,
			id:          101,
			title:       "Add a token bucket rate limiter to the public routes",
			description: descriptionPR101(),
			state:       bitbucket.PullRequestOpen,
			author:      demoUser(),
			reviewers:   []bitbucket.Account{ada(), grace()},
			participants: []bitbucket.Participant{
				newParticipant(ada(), bitbucket.RoleReviewer, bitbucket.ParticipantApproved, ago(26*hour)),
				newParticipant(grace(), bitbucket.RoleReviewer, "", time.Time{}),
			},
			source:      "feature/rate-limiter",
			destination: branchMain,
			hash:        hashPR101,
			destHash:    hashMainAPI,
			created:     3 * day,
			updated:     26 * hour,
			comments:    4,
			tasks:       1,
		},
		{
			repo:        repoAPIGateway,
			id:          98,
			title:       "Propagate trace ids through the router",
			description: descriptionPR98(),
			state:       bitbucket.PullRequestMerged,
			author:      ada(),
			reviewers:   []bitbucket.Account{demoUser()},
			participants: []bitbucket.Participant{
				newParticipant(demoUser(), bitbucket.RoleReviewer, bitbucket.ParticipantApproved, ago(4*day)),
			},
			source:      "feature/request-tracing",
			destination: branchMain,
			hash:        hashPR98,
			destHash:    hashMainAPI,
			created:     7 * day,
			updated:     4 * day,
			closedBy:    ptr(demoUser()),
			mergeHash:   hashMergePR98,
			comments:    9,
		},
		{
			repo:        repoAPIGateway,
			id:          95,
			title:       "Spike: replace the gateway with an Envoy sidecar",
			description: descriptionPR95(),
			state:       bitbucket.PullRequestDeclined,
			author:      ken(),
			reviewers:   []bitbucket.Account{grace()},
			participants: []bitbucket.Participant{
				newParticipant(grace(), bitbucket.RoleReviewer,
					bitbucket.ParticipantChangesRequested, ago(10*day)),
			},
			source:      "spike/envoy-sidecar",
			destination: branchMain,
			hash:        hashPR95,
			destHash:    hashMainAPI,
			created:     14 * day,
			updated:     9 * day,
			closedBy:    ptr(grace()),
			reason: "Declined in favour of #101: the sidecar doubles our p99 latency and we do not" +
				" need the extra routing surface yet.",
			comments: 12,
		},
		{
			repo:        repoWebApp,
			id:          41,
			title:       "Bump frontend dependencies",
			description: "",
			state:       bitbucket.PullRequestOpen,
			author:      demoUser(),
			// Loaded, but genuinely empty: nobody has been asked to review yet.
			reviewers:    []bitbucket.Account{},
			participants: []bitbucket.Participant{},
			source:       "chore/bump-deps",
			destination:  branchDevelop,
			hash:         hashPR41,
			destHash:     hashDevelopWeb,
			created:      6 * hour,
			updated:      3 * hour,
		},
		{
			repo:        repoWebApp,
			id:          40,
			title:       "Dark mode",
			description: descriptionPR40(),
			state:       bitbucket.PullRequestOpen,
			author:      ada(),
			reviewers:   []bitbucket.Account{demoUser(), ken()},
			participants: []bitbucket.Participant{
				newParticipant(demoUser(), bitbucket.RoleReviewer, bitbucket.ParticipantApproved, ago(21*hour)),
				newParticipant(ken(), bitbucket.RoleReviewer, "", time.Time{}),
			},
			source:      "feature/dark-mode",
			destination: branchDevelop,
			hash:        hashPR40,
			destHash:    hashDevelopWeb,
			created:     2 * day,
			updated:     20 * hour,
			comments:    7,
			tasks:       3,
		},
		{
			repo:        repoWebApp,
			id:          37,
			title:       "Keyboard navigation for the product grid",
			description: descriptionPR37(),
			state:       bitbucket.PullRequestMerged,
			author:      barbara(),
			reviewers:   []bitbucket.Account{ada()},
			participants: []bitbucket.Participant{
				newParticipant(ada(), bitbucket.RoleReviewer, bitbucket.ParticipantApproved, ago(6*day)),
			},
			source:      "feature/keyboard-nav",
			destination: branchDevelop,
			hash:        hashPR37,
			destHash:    hashDevelopWeb,
			created:     10 * day,
			updated:     6 * day,
			closedBy:    ptr(ada()),
			mergeHash:   hashMergePR37,
			comments:    3,
		},
		{
			repo:        repoInfra,
			id:          7,
			title:       "Upgrade Terraform to 1.9 and pin the AWS provider",
			description: descriptionPR7(),
			state:       bitbucket.PullRequestOpen,
			author:      ken(),
			reviewers:   []bitbucket.Account{barbara()},
			participants: []bitbucket.Participant{
				newParticipant(barbara(), bitbucket.RoleReviewer, "", time.Time{}),
			},
			source:      "feature/terraform-1-9",
			destination: branchMain,
			hash:        hashPR7,
			destHash:    hashMainInfra,
			created:     36 * hour,
			updated:     8 * hour,
			comments:    2,
		},
		{
			repo:        repoInfra,
			id:          5,
			title:       "Rotate the CI deploy secrets",
			description: descriptionPR5(),
			state:       bitbucket.PullRequestMerged,
			author:      barbara(),
			reviewers:   []bitbucket.Account{ken()},
			participants: []bitbucket.Participant{
				newParticipant(ken(), bitbucket.RoleReviewer, bitbucket.ParticipantApproved, ago(11*day)),
			},
			source:      "chore/rotate-secrets",
			destination: branchMain,
			hash:        hashPR5,
			destHash:    hashMainInfra,
			created:     13 * day,
			updated:     11 * day,
			closedBy:    ptr(ken()),
			mergeHash:   hashMergePR5,
			comments:    1,
		},
	}
}

func ptr[T any](value T) *T { return &value }

// -----------------------------------------------------------------------------
// Pull request descriptions (Markdown: headings, lists, fences, tasks, links)
// -----------------------------------------------------------------------------

func descriptionPR101() string {
	return markdown(
		"## Summary",
		"",
		"Puts a token bucket in front of every public route so one noisy client can no longer",
		"starve the rest of the workspace.",
		"",
		"### Why now",
		"",
		"- The upstream provider throttles us at **1000 requests/hour**",
		"- Two incidents last week traced back to a single retry storm",
		"- The limiter is a prerequisite for the OAuth work in #102",
		"",
		"### How",
		"",
		"```go",
		"limiter := ratelimit.New(1000, time.Hour)",
		"router.Use(limiter.Middleware)",
		"```",
		"",
		"### Checklist",
		"",
		"- [x] Unit tests for the bucket refill maths",
		"- [x] Metrics for rejected requests",
		"- [ ] Load test against staging",
		"",
		"See [RFC 014](https://bitbucket.org/acme/api-gateway/src/main/docs/rfc-014.md) for the",
		"design discussion.",
	)
}

func descriptionPR102() string {
	return markdown(
		"## Draft: OAuth token introspection",
		"",
		"Still shaping the cache layer, do not review the middleware yet.",
		"",
		"### Open questions",
		"",
		"1. Do we cache negative introspection results?",
		"2. What is the right TTL when the token carries no `exp`?",
		"",
		"```yaml",
		"introspection:",
		"  endpoint: https://auth.acme.example/oauth2/introspect",
		"  cache_ttl: 60s",
		"```",
		"",
		"### Before this leaves draft",
		"",
		"- [ ] Decide the cache policy",
		"- [ ] Add contract tests against the auth service",
		"",
		"Context in [the auth migration page](https://bitbucket.org/acme/api-gateway/wiki/auth).",
	)
}

func descriptionPR103() string {
	return markdown(
		"## Problem",
		"",
		"A retried request reuses the parent context, so the second attempt inherits an already",
		"expired deadline and fails instantly.",
		"",
		"### Fix",
		"",
		"- Derive a **fresh** timeout per attempt",
		"- Cap the total time with a separate budget context",
		"",
		"```go",
		"attemptCtx, cancel := context.WithTimeout(budgetCtx, perAttempt)",
		"defer cancel()",
		"```",
		"",
		"### Follow-up",
		"",
		"- [x] Regression test for the expired-deadline case",
		"- [ ] Backport to the 2.3 release branch",
		"",
		"Reported in [INC-4471](https://bitbucket.org/acme/api-gateway/issues/4471).",
	)
}

func descriptionPR98() string {
	return markdown(
		"## Request tracing",
		"",
		"Threads the incoming `traceparent` header through the router and onto every upstream call.",
		"",
		"- Adds a `trace` field to the access log",
		"- No change to the public API",
		"",
		"- [x] Verified against the collector in staging",
	)
}

func descriptionPR95() string {
	return markdown(
		"## Spike: Envoy sidecar",
		"",
		"Replaces the bespoke router with an Envoy sidecar to see what we would gain.",
		"",
		"### Findings",
		"",
		"- Configuration is far more expressive",
		"- p99 latency **doubled** under our current traffic shape",
		"- The sidecar adds 180 MB of memory per pod",
		"",
		"- [x] Benchmark against the current gateway",
		"- [ ] Revisit once we need per-route mTLS",
		"",
		"Numbers in [the spike doc](https://bitbucket.org/acme/api-gateway/wiki/envoy-spike).",
	)
}

func descriptionPR40() string {
	return markdown(
		"## Dark mode",
		"",
		"Adds a dark theme driven by `prefers-color-scheme` with a manual override in settings.",
		"",
		"### Scope",
		"",
		"- Design tokens moved to CSS custom properties",
		"- Charts get their own palette",
		"- The override is persisted per account, not per device",
		"",
		"```css",
		":root[data-theme=\"dark\"] { --surface: #12141a; }",
		"```",
		"",
		"### Checklist",
		"",
		"- [x] Contrast audit on the primary flows",
		"- [x] Screenshot tests for both themes",
		"- [ ] Marketing pages",
		"",
		"Mockups in [the design page](https://bitbucket.org/acme/web-app/wiki/dark-mode).",
	)
}

func descriptionPR37() string {
	return markdown(
		"## Keyboard navigation",
		"",
		"Arrow keys move through the product grid, `Enter` opens the focused card.",
		"",
		"- Roving tabindex instead of one tab stop per card",
		"- Announces the focused card to screen readers",
		"",
		"- [x] Tested with VoiceOver and NVDA",
	)
}

func descriptionPR7() string {
	return markdown(
		"## Terraform 1.9",
		"",
		"Upgrades the toolchain and pins the AWS provider so plans stop drifting between machines.",
		"",
		"### Steps",
		"",
		"1. Bump the version in `.tool-versions`",
		"2. Regenerate the lock file",
		"3. Re-plan every workspace",
		"",
		"```hcl",
		"terraform {",
		"  required_version = \"~> 1.9\"",
		"}",
		"```",
		"",
		"- [x] Plans are clean in staging",
		"- [ ] Production plan reviewed",
		"",
		"Runbook: [infra/docs/upgrades](https://bitbucket.org/acme/infra/src/main/docs/upgrades.md).",
	)
}

func descriptionPR5() string {
	return markdown(
		"## Secret rotation",
		"",
		"Rotates the CI deploy keys and moves them into the workspace variable store.",
		"",
		"- Old keys revoked after the merge",
		"- Pipelines read them from repository variables",
		"",
		"- [x] Confirmed the deploy step still runs",
	)
}

// -----------------------------------------------------------------------------
// Build statuses
// -----------------------------------------------------------------------------

type statusSpec struct {
	repo        string
	pullRequest int
	key         string
	name        string
	description string
	refName     string
	buildNumber int
	state       bitbucket.CommitStatusState
	hash        string
	created     time.Duration
	updated     time.Duration
}

func newCommitStatus(spec statusSpec) bitbucket.CommitStatus {
	full := fullName(spec.repo)
	ref := repoRef(spec.repo)
	status := bitbucket.CommitStatus{
		Type:        "build",
		Key:         spec.key,
		Name:        spec.name,
		Description: spec.description,
		RefName:     spec.refName,
		URL:         htmlBase + "/" + full + "/pipelines/results/" + strconv.Itoa(spec.buildNumber),
		State:       spec.state,
		Commit:      newCommit(spec.repo, spec.hash),
		Repository:  &ref,
		CreatedOn:   ago(spec.created),
		UpdatedOn:   ago(spec.updated),
	}
	commitURL := apiBase + "/repositories/" + full + "/commit/" + spec.hash
	status.Links.Self = bitbucket.Link{Href: commitURL + "/statuses/build/" + spec.key}
	status.Links.Commit = bitbucket.Link{Href: commitURL}
	return status
}

func statusSpecs() []statusSpec {
	return []statusSpec{
		{
			repo: repoAPIGateway, pullRequest: 101, key: "prs:**:feature/rate-limiter",
			name: "Pipeline - pullrequests: **", description: "Build 412 passed", refName: "feature/rate-limiter",
			buildNumber: 412, state: bitbucket.StatusSuccessful, hash: hashPR101,
			created: 27 * hour, updated: 26 * hour,
		},
		{
			repo: repoAPIGateway, pullRequest: 102, key: "prs:**:feature/oauth-introspection",
			name: "Pipeline - pullrequests: **", description: "Build 413 is running",
			refName: "feature/oauth-introspection", buildNumber: 413, state: bitbucket.StatusInProgress,
			hash: hashPR102, created: 5 * hour, updated: 5 * hour,
		},
		{
			repo: repoAPIGateway, pullRequest: 103, key: "prs:**:bugfix/upstream-timeout",
			name: "Pipeline - pullrequests: **", description: "Build 414 failed",
			refName: "bugfix/upstream-timeout", buildNumber: 414, state: bitbucket.StatusFailed,
			hash: hashPR103, created: 2*hour + 20*time.Minute, updated: 2 * hour,
		},
		// Two statuses on the same commit with different keys: the older pipeline
		// run was stopped and the rerun succeeded.
		{
			repo: repoWebApp, pullRequest: 40, key: "prs:**:feature/dark-mode",
			name: "Pipeline - pullrequests: **", description: "Build 87 was stopped",
			refName: "feature/dark-mode", buildNumber: 87, state: bitbucket.StatusStopped,
			hash: hashPR40, created: 23 * hour, updated: 22 * hour,
		},
		{
			repo: repoWebApp, pullRequest: 40, key: "e2e:playwright",
			name: "Playwright end-to-end", description: "Build 88 passed",
			refName: "feature/dark-mode", buildNumber: 88, state: bitbucket.StatusSuccessful,
			hash: hashPR40, created: 21 * hour, updated: 20 * hour,
		},
		{
			repo: repoInfra, pullRequest: 7, key: "prs:**:feature/terraform-1-9",
			name: "Pipeline - pullrequests: **", description: "Build 21 was stopped",
			refName: "feature/terraform-1-9", buildNumber: 21, state: bitbucket.StatusStopped,
			hash: hashPR7, created: 9 * hour, updated: 8 * hour,
		},
	}
}

// -----------------------------------------------------------------------------
// Pipelines, steps and logs
// -----------------------------------------------------------------------------

type stepSpec struct {
	name     string
	state    string
	result   string
	errorKey string
	errorMsg string
	image    string
	command  string
	output   []string
	duration int
}

type pipelineSpec struct {
	repo        string
	buildNumber int
	state       string
	stage       string
	result      string
	terminator  *bitbucket.Account
	creator     bitbucket.Account
	trigger     string
	branch      string // ref target branch; used when pullRequest == 0
	destination string
	hash        string
	destHash    string
	pullRequest int
	prTitle     string
	created     time.Duration
	duration    int
	steps       []stepSpec
}

func pipelineUUID(buildNumber int) string {
	return fmt.Sprintf("{c0ffee00-0000-0000-0000-%012d}", buildNumber)
}

func stepUUID(buildNumber, index int) string {
	return fmt.Sprintf("{a1000000-0000-0000-0000-%09d%03d}", buildNumber, index+1)
}

// stepLog renders a short, deterministic log for a step. Steps that never ran
// have no log at all, exactly like the real API, which answers 404 for them.
func stepLog(spec stepSpec) string {
	if spec.state == statePending || spec.result == resultNotRun {
		return ""
	}

	lines := make([]string, 0, len(spec.output)+4)
	lines = append(lines, "+ "+spec.command)
	lines = append(lines, spec.output...)
	switch spec.result {
	case resultFailed:
		lines = append(lines, spec.errorMsg, "Command exited with status 1")
	case resultStopped:
		lines = append(lines, "Step was stopped by a user")
	case resultSuccessful:
		lines = append(lines, "Build teardown: cache 'docker' uploaded")
	default:
	}
	return strings.Join(lines, "\n") + "\n"
}

func newPipelineTarget(spec pipelineSpec) bitbucket.PipelineTarget {
	if spec.pullRequest == 0 {
		return bitbucket.PipelineTarget{
			Type:     "pipeline_ref_target",
			Selector: &bitbucket.PipelineSelector{Type: "branches", Pattern: spec.branch},
			Commit:   newCommit(spec.repo, spec.hash),
			RefType:  "branch",
			RefName:  spec.branch,
		}
	}

	pullRequestRef := &bitbucket.PipelinePullRequestRef{
		Type:  "pullrequest",
		ID:    spec.pullRequest,
		Title: spec.prTitle,
	}
	number := strconv.Itoa(spec.pullRequest)
	full := fullName(spec.repo)
	pullRequestRef.Links.Self = bitbucket.Link{
		Href: apiBase + "/repositories/" + full + "/pullrequests/" + number,
	}
	pullRequestRef.Links.HTML = bitbucket.Link{
		Href: htmlBase + "/" + full + "/pull-requests/" + number,
	}

	return bitbucket.PipelineTarget{
		Type:              "pipeline_pullrequest_target",
		Selector:          &bitbucket.PipelineSelector{Type: "pull-requests", Pattern: "**"},
		Commit:            newCommit(spec.repo, spec.hash),
		Source:            spec.branch,
		Destination:       spec.destination,
		DestinationCommit: newCommit(spec.repo, shortHash(spec.destHash)),
		PullRequest:       pullRequestRef,
	}
}

// newPipeline builds a run together with its steps and their logs.
func newPipeline(spec pipelineSpec) (bitbucket.Pipeline, []bitbucket.PipelineStep, map[string]string) {
	uuid := pipelineUUID(spec.buildNumber)
	ref := repoRef(spec.repo)
	creator := spec.creator
	createdOn := ago(spec.created)

	state := bitbucket.PipelineState{Type: "pipeline_state", Name: spec.state}
	switch spec.state {
	case stateInProgress:
		state.Stage = &bitbucket.PipelineStateStage{
			Type: "pipeline_state_in_progress_" + strings.ToLower(spec.stage),
			Name: spec.stage,
		}
	case stateCompleted:
		state.Result = &bitbucket.PipelineStateResult{
			Type:       "pipeline_state_completed_" + strings.ToLower(spec.result),
			Name:       spec.result,
			Terminator: spec.terminator,
		}
	default:
	}

	pipeline := bitbucket.Pipeline{
		Type:              "pipeline",
		UUID:              uuid,
		BuildNumber:       spec.buildNumber,
		State:             state,
		Creator:           &creator,
		Repository:        &ref,
		Target:            newPipelineTarget(spec),
		Trigger:           bitbucket.PipelineTrigger{Type: "pipeline_trigger_push", Name: spec.trigger},
		CreatedOn:         createdOn,
		RunNumber:         1,
		DurationInSeconds: spec.duration,
		BuildSecondsUsed:  spec.duration,
		FirstSuccessful:   false,
		Expired:           false,
		HasVariables:      false,
	}
	// A run that has not finished carries no completed_on at all; the server
	// drops zero timestamps on the way out, so leaving it zero is enough.
	if spec.state == stateCompleted {
		pipeline.CompletedOn = createdOn.Add(time.Duration(spec.duration) * time.Second)
	}
	pipeline.Links.Self = bitbucket.Link{
		Href: apiBase + "/repositories/" + fullName(spec.repo) + "/pipelines/" + url.PathEscape(uuid),
	}
	pipeline.Links.Steps = bitbucket.Link{Href: pipeline.Links.Self.Href + "/steps"}

	steps, logs := newPipelineSteps(spec, uuid, createdOn)
	return pipeline, steps, logs
}

func newPipelineSteps(
	spec pipelineSpec,
	uuid string,
	createdOn time.Time,
) ([]bitbucket.PipelineStep, map[string]string) {
	steps := make([]bitbucket.PipelineStep, 0, len(spec.steps))
	logs := make(map[string]string, len(spec.steps))
	startedAt := createdOn.Add(4 * time.Second)

	for index, stepDef := range spec.steps {
		id := stepUUID(spec.buildNumber, index)

		state := bitbucket.PipelineStepState{Type: "pipeline_step_state", Name: stepDef.state}
		if stepDef.state == stateCompleted {
			result := &bitbucket.PipelineStepResult{
				Type: "pipeline_step_state_completed_" + strings.ToLower(stepDef.result),
				Name: stepDef.result,
			}
			if stepDef.errorMsg != "" {
				result.Error = &bitbucket.PipelineError{
					Type:    "pipeline_error",
					Key:     stepDef.errorKey,
					Message: stepDef.errorMsg,
				}
			}
			state.Result = result
		}

		step := bitbucket.PipelineStep{
			Type:              "pipeline_step",
			UUID:              id,
			Name:              stepDef.name,
			State:             state,
			DurationInSeconds: stepDef.duration,
			BuildSecondsUsed:  stepDef.duration,
			RunNumber:         1,
			MaxTime:           120,
			Image:             bitbucket.PipelineImage{Name: stepDef.image},
		}
		step.Trigger.Type = "pipeline_step_trigger_automatic"
		step.Pipeline.Type = "pipeline"
		step.Pipeline.UUID = uuid

		// PENDING and NOT_RUN steps never started, so both timestamps stay zero
		// and the server omits them.
		if stepDef.state != statePending && stepDef.result != resultNotRun {
			step.StartedOn = startedAt
			if stepDef.state == stateCompleted {
				step.CompletedOn = startedAt.Add(time.Duration(stepDef.duration) * time.Second)
			}
			startedAt = startedAt.Add(time.Duration(stepDef.duration+2) * time.Second)

			step.SetupCommands = []bitbucket.PipelineCommand{
				{CommandType: "system", Name: "Build setup", Action: "build-setup"},
			}
			step.ScriptCommands = []bitbucket.PipelineCommand{
				{CommandType: "user", Name: stepDef.command, Command: stepDef.command},
			}
			if stepDef.state == stateCompleted {
				step.TeardownCommands = []bitbucket.PipelineCommand{
					{CommandType: "system", Name: "Build teardown", Action: "build-teardown"},
				}
			}
		}

		steps = append(steps, step)
		if text := stepLog(stepDef); text != "" {
			logs[id] = text
		}
	}
	return steps, logs
}

const (
	imageGo   = "golang:1.25"
	imageNode = "node:22"
	imageTF   = "hashicorp/terraform:1.9"
)

func pipelineSpecs() []pipelineSpec {
	return []pipelineSpec{
		// api-gateway, newest first.
		{
			repo: repoAPIGateway, buildNumber: 414, state: stateCompleted, result: resultFailed,
			creator: linus(), trigger: "PUSH", branch: "bugfix/upstream-timeout", destination: branchMain,
			hash: hashPR103, destHash: hashMainAPI, pullRequest: 103,
			prTitle: "Fix upstream timeout handling on retried requests",
			created: 2*hour + 20*time.Minute, duration: 214,
			steps: []stepSpec{
				{
					name: "Build", state: stateCompleted, result: resultSuccessful, image: imageGo,
					command: "go build ./...", duration: 61,
					output: []string{"go: downloading modules", "build finished in 58s"},
				},
				{
					name: "Unit tests", state: stateCompleted, result: resultFailed, image: imageGo,
					command:  "go test ./...",
					duration: 153,
					errorKey: "runtime.step.script.failed",
					errorMsg: "The step exited with a non-zero status: --- FAIL: TestRetryDeadline",
					output: []string{
						"ok  \tgithub.com/acme/api-gateway/internal/router\t0.412s",
						"--- FAIL: TestRetryDeadline (0.00s)",
						"    retry_test.go:88: attempt 2: context deadline exceeded",
						"FAIL\tgithub.com/acme/api-gateway/internal/upstream\t0.106s",
					},
				},
				{
					name: "Integration tests", state: stateCompleted, result: resultNotRun,
					image: imageGo, command: "go test -tags=integration ./...",
				},
				{
					name: "Publish image", state: stateCompleted, result: resultNotRun,
					image: imageGo, command: "docker push acme/api-gateway",
				},
			},
		},
		{
			repo: repoAPIGateway, buildNumber: 413, state: stateInProgress, stage: stageRunning,
			creator: grace(), trigger: "PUSH", branch: "feature/oauth-introspection", destination: branchMain,
			hash: hashPR102, destHash: hashMainAPI, pullRequest: 102,
			prTitle: "Introduce OAuth token introspection",
			created: 5 * hour, duration: 0,
			steps: []stepSpec{
				{
					name: "Build", state: stateCompleted, result: resultSuccessful, image: imageGo,
					command: "go build ./...", duration: 58,
					output: []string{"build finished in 55s"},
				},
				{
					name: "Unit tests", state: stateInProgress, image: imageGo,
					command: "go test ./...", duration: 0,
					output: []string{"ok  \tgithub.com/acme/api-gateway/internal/auth\t0.284s"},
				},
				{
					name: "Integration tests", state: statePending, image: imageGo,
					command: "go test -tags=integration ./...",
				},
				{
					name: "Publish image", state: statePending, image: imageGo,
					command: "docker push acme/api-gateway",
				},
			},
		},
		{
			repo: repoAPIGateway, buildNumber: 412, state: stateCompleted, result: resultSuccessful,
			creator: demoUser(), trigger: "PUSH", branch: "feature/rate-limiter", destination: branchMain,
			hash: hashPR101, destHash: hashMainAPI, pullRequest: 101,
			prTitle: "Add a token bucket rate limiter to the public routes",
			created: 27 * hour, duration: 268,
			steps: []stepSpec{
				{
					name: "Build", state: stateCompleted, result: resultSuccessful, image: imageGo,
					command: "go build ./...", duration: 63,
					output: []string{"go: downloading modules", "build finished in 60s"},
				},
				{
					name: "Unit tests", state: stateCompleted, result: resultSuccessful, image: imageGo,
					command: "go test ./...", duration: 121,
					output: []string{
						"ok  \tgithub.com/acme/api-gateway/internal/ratelimit\t1.204s",
						"ok  \tgithub.com/acme/api-gateway/internal/router\t0.418s",
						"ok  \tgithub.com/acme/api-gateway/internal/upstream\t0.097s",
						"ok  \tgithub.com/acme/api-gateway/cmd/gateway\t0.031s",
					},
				},
				{
					name: "Publish image", state: stateCompleted, result: resultSuccessful, image: imageGo,
					command: "docker push acme/api-gateway", duration: 84,
					output: []string{"The push refers to repository [docker.io/acme/api-gateway]", "412: digest: sha256:9c1d0a4b"},
				},
			},
		},
		{
			repo: repoAPIGateway, buildNumber: 411, state: stateCompleted, result: resultSuccessful,
			creator: ada(), trigger: "PUSH", branch: branchMain,
			hash: hashMainAPI, created: 2 * day, duration: 241,
			steps: []stepSpec{
				{
					name: "Build", state: stateCompleted, result: resultSuccessful, image: imageGo,
					command: "go build ./...", duration: 60, output: []string{"build finished in 57s"},
				},
				{
					name: "Unit tests", state: stateCompleted, result: resultSuccessful, image: imageGo,
					command: "go test ./...", duration: 118,
					output: []string{"ok  \tgithub.com/acme/api-gateway/internal/router\t0.402s"},
				},
				{
					name: "Publish image", state: stateCompleted, result: resultSuccessful, image: imageGo,
					command: "docker push acme/api-gateway", duration: 63,
					output: []string{"411: digest: sha256:1a2b3c4d"},
				},
			},
		},

		// web-app, newest first.
		{
			repo: repoWebApp, buildNumber: 88, state: stateCompleted, result: resultSuccessful,
			creator: ada(), trigger: "PUSH", branch: "feature/dark-mode", destination: branchDevelop,
			hash: hashPR40, destHash: hashDevelopWeb, pullRequest: 40, prTitle: "Dark mode",
			created: 21 * hour, duration: 402,
			steps: []stepSpec{
				{
					name: "Install dependencies", state: stateCompleted, result: resultSuccessful,
					image: imageNode, command: "npm ci", duration: 74,
					output: []string{"added 1284 packages in 71s"},
				},
				{
					name: "Lint", state: stateCompleted, result: resultSuccessful, image: imageNode,
					command: "npm run lint", duration: 39, output: []string{"eslint: 0 problems"},
				},
				{
					name: "Test", state: stateCompleted, result: resultSuccessful, image: imageNode,
					command: "npm test -- --run", duration: 168,
					output: []string{
						"Test Files  62 passed (62)",
						"Tests  914 passed (914)",
					},
				},
				{
					name: "Build bundle", state: stateCompleted, result: resultSuccessful, image: imageNode,
					command: "npm run build", duration: 121,
					output: []string{"dist/assets/index-9c1d0a4b.js   412.08 kB", "built in 118s"},
				},
			},
		},
		{
			repo: repoWebApp, buildNumber: 87, state: stateCompleted, result: resultStopped,
			terminator: ptr(ada()),
			creator:    ada(), trigger: "PUSH", branch: "feature/dark-mode", destination: branchDevelop,
			hash: hashPR40, destHash: hashDevelopWeb, pullRequest: 40, prTitle: "Dark mode",
			created: 23 * hour, duration: 96,
			steps: []stepSpec{
				{
					name: "Install dependencies", state: stateCompleted, result: resultSuccessful,
					image: imageNode, command: "npm ci", duration: 72,
					output: []string{"added 1284 packages in 69s"},
				},
				{
					name: "Lint", state: stateCompleted, result: resultStopped, image: imageNode,
					command: "npm run lint", duration: 22,
				},
				{
					name: "Test", state: stateCompleted, result: resultNotRun, image: imageNode,
					command: "npm test -- --run",
				},
			},
		},

		// infra, newest first.
		{
			repo: repoInfra, buildNumber: 21, state: stateCompleted, result: resultStopped,
			terminator: ptr(ken()),
			creator:    ken(), trigger: "PUSH", branch: "feature/terraform-1-9", destination: branchMain,
			hash: hashPR7, destHash: hashMainInfra, pullRequest: 7,
			prTitle: "Upgrade Terraform to 1.9 and pin the AWS provider",
			created: 9 * hour, duration: 143,
			steps: []stepSpec{
				{
					name: "Terraform plan", state: stateCompleted, result: resultSuccessful, image: imageTF,
					command: "terraform plan -out=tfplan", duration: 118,
					output: []string{
						"Terraform will perform the following actions:",
						"Plan: 3 to add, 1 to change, 0 to destroy.",
					},
				},
				{
					name: "Policy check", state: stateCompleted, result: resultStopped, image: imageTF,
					command: "conftest test tfplan", duration: 25,
				},
				{
					name: "Terraform apply", state: stateCompleted, result: resultNotRun, image: imageTF,
					command: "terraform apply tfplan",
				},
			},
		},
		{
			repo: repoInfra, buildNumber: 20, state: stateCompleted, result: resultSuccessful,
			creator: barbara(), trigger: "SCHEDULE", branch: branchMain,
			hash: hashMainInfra, created: 3 * day, duration: 187,
			steps: []stepSpec{
				{
					name: "Terraform plan", state: stateCompleted, result: resultSuccessful, image: imageTF,
					command: "terraform plan -out=tfplan", duration: 104,
					output: []string{"No changes. Your infrastructure matches the configuration."},
				},
				{
					name: "Policy check", state: stateCompleted, result: resultSuccessful, image: imageTF,
					command: "conftest test tfplan", duration: 31, output: []string{"12 tests, 12 passed"},
				},
				{
					name: "Terraform apply", state: stateCompleted, result: resultSuccessful, image: imageTF,
					command: "terraform apply tfplan", duration: 52,
					output: []string{"Apply complete! Resources: 0 added, 0 changed, 0 destroyed."},
				},
			},
		},
	}
}
