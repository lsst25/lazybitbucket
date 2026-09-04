package bitbucket

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// Pipeline and step state names, as they appear in state.name on the wire.
// Both enums are open: Bitbucket adds values (PAUSED, HALTED) that were never
// observed during the API probe, so nothing switches exhaustively on them.
const (
	pipelineStateInProgress = "IN_PROGRESS"
	pipelineStateCompleted  = "COMPLETED"
)

// minCommitPrefix is the shortest abbreviated hash SameCommit will compare.
// PR payloads carry 12-char hashes while commit and pipeline payloads carry 40,
// so any comparison is a prefix match; below 7 chars it stops being meaningful.
const minCommitPrefix = 7

// Link is a single href entry under a links object.
type Link struct {
	Href string `json:"href"`
	Name string `json:"name,omitempty"`
}

// AccountLinks are the links carried by an Account.
type AccountLinks struct {
	Self   Link `json:"self"`
	HTML   Link `json:"html"`
	Avatar Link `json:"avatar"`
}

// Account is the abbreviated user object Bitbucket embeds everywhere: PR author,
// closed_by, reviewers, participants[].user, pipeline creator and terminator.
// Legacy or deleted accounts can degrade to a bare {type, uuid} stub, so every
// field except Type and UUID may be empty.
type Account struct {
	Type        string       `json:"type"`         // observed "user"; also "team", "app_user"
	UUID        string       `json:"uuid"`         // "{e5743438-…}" — braces included
	AccountID   string       `json:"account_id"`   // opaque; two formats in the wild; may be ""
	DisplayName string       `json:"display_name"` // may be "" on legacy/deleted accounts
	Nickname    string       `json:"nickname"`
	Links       AccountLinks `json:"links"`
}

// Name is the best available human label for the account, never empty as long as
// the account carries a UUID.
func (a Account) Name() string {
	if a.DisplayName != "" {
		return a.DisplayName
	}
	if a.Nickname != "" {
		return a.Nickname
	}
	return a.UUID
}

// RepositoryRefLinks are the links carried by an abbreviated repository.
type RepositoryRefLinks struct {
	Self   Link `json:"self"`
	HTML   Link `json:"html"`
	Avatar Link `json:"avatar"`
}

// RepositoryRef is the abbreviated repository embedded in PRs, statuses and
// pipelines. It has no slug field — derive the slug from FullName.
type RepositoryRef struct {
	Type     string             `json:"type"`
	UUID     string             `json:"uuid"`
	Name     string             `json:"name"`      // NOT the slug ("Sales ERP" vs "sales-erp")
	FullName string             `json:"full_name"` // "workspace/slug"
	Links    RepositoryRefLinks `json:"links"`
}

// Workspace is the workspace slug taken from FullName, or "" when FullName
// carries no slash.
func (r RepositoryRef) Workspace() string {
	workspace, _, found := strings.Cut(r.FullName, "/")
	if !found {
		return ""
	}
	return workspace
}

// Slug is the repository slug taken from FullName, falling back to Name.
func (r RepositoryRef) Slug() string {
	_, slug, found := strings.Cut(r.FullName, "/")
	if !found || slug == "" {
		return r.Name
	}
	return slug
}

// Branch is a named branch reference.
type Branch struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// ProjectRef is the project a repository belongs to.
type ProjectRef struct {
	Type  string `json:"type"`
	UUID  string `json:"uuid"`
	Key   string `json:"key"`
	Name  string `json:"name"`
	Links struct {
		Self   Link `json:"self"`
		HTML   Link `json:"html"`
		Avatar Link `json:"avatar"`
	} `json:"links"`
}

// WorkspaceRef is the abbreviated workspace embedded in a repository.
type WorkspaceRef struct {
	Type  string `json:"type"`
	UUID  string `json:"uuid"`
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Links struct {
		Self   Link `json:"self"`
		HTML   Link `json:"html"`
		Avatar Link `json:"avatar"`
	} `json:"links"`
}

// Workspace is the full workspace object from GET /workspaces/{workspace}.
type Workspace struct {
	Type      string    `json:"type"`
	UUID      string    `json:"uuid"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	IsPrivate bool      `json:"is_private"`
	CreatedOn time.Time `json:"created_on"`
	Links     struct {
		Self   Link `json:"self"`
		HTML   Link `json:"html"`
		Avatar Link `json:"avatar"`
	} `json:"links"`
}

// RepositoryLinks are the links on a full repository object. Clone is an array,
// unlike every other link member.
type RepositoryLinks struct {
	Self         Link   `json:"self"`
	HTML         Link   `json:"html"`
	Avatar       Link   `json:"avatar"`
	PullRequests Link   `json:"pullrequests"`
	Commits      Link   `json:"commits"`
	Branches     Link   `json:"branches"`
	Clone        []Link `json:"clone"`
}

// Repository is the full repository object, identical in shape between
// GET /repositories/{ws} list items and GET /repositories/{ws}/{slug}.
type Repository struct {
	Type        string          `json:"type"`
	UUID        string          `json:"uuid"`
	Slug        string          `json:"slug"`
	Name        string          `json:"name"`
	FullName    string          `json:"full_name"`
	Description string          `json:"description"`
	SCM         string          `json:"scm"`
	Language    string          `json:"language"`
	IsPrivate   bool            `json:"is_private"`
	Size        int64           `json:"size"`
	MainBranch  *Branch         `json:"mainbranch"`
	Project     *ProjectRef     `json:"project"`
	Workspace   *WorkspaceRef   `json:"workspace"`
	Owner       *Account        `json:"owner"`
	CreatedOn   time.Time       `json:"created_on"`
	UpdatedOn   time.Time       `json:"updated_on"`
	Links       RepositoryLinks `json:"links"`
}

// HTMLURL is the browser URL for the repository.
func (r Repository) HTMLURL() string {
	if r.Links.HTML.Href != "" {
		return r.Links.HTML.Href
	}
	return "https://bitbucket.org/" + r.FullName
}

// WorkspaceSlug is the slug of the workspace owning the repository.
func (r Repository) WorkspaceSlug() string {
	if r.Workspace != nil && r.Workspace.Slug != "" {
		return r.Workspace.Slug
	}
	workspace, _, found := strings.Cut(r.FullName, "/")
	if !found {
		return ""
	}
	return workspace
}

// MainBranchName is the repository's main branch, or "" when it has none.
func (r Repository) MainBranchName() string {
	if r.MainBranch == nil {
		return ""
	}
	return r.MainBranch.Name
}

// CommitRef is an abbreviated commit. Hash is 12 chars inside a PR payload and
// 40 chars in commit, status and pipeline payloads — compare with SameCommit.
type CommitRef struct {
	Type  string `json:"type"`
	Hash  string `json:"hash"`
	Links struct {
		Self Link `json:"self"`
		HTML Link `json:"html"`
	} `json:"links"`
}

// PullRequestEndpoint is the source or destination side of a pull request.
type PullRequestEndpoint struct {
	Branch     Branch        `json:"branch"`
	Commit     *CommitRef    `json:"commit"`
	Repository RepositoryRef `json:"repository"`
}

// RenderedContent is Bitbucket's {type, raw, markup, html} shape, used by
// summary, rendered.title and rendered.description.
type RenderedContent struct {
	Type   string `json:"type"`
	Raw    string `json:"raw"`
	Markup string `json:"markup"` // "markdown" | "creole" | "plaintext"
	HTML   string `json:"html"`
}

// PullRequestState is the lifecycle state of a pull request. Open enum.
type PullRequestState string

// Observed pull request states.
const (
	PullRequestOpen       PullRequestState = "OPEN"
	PullRequestMerged     PullRequestState = "MERGED"
	PullRequestDeclined   PullRequestState = "DECLINED"
	PullRequestSuperseded PullRequestState = "SUPERSEDED"
)

// ParticipantRole is a participant's role on a pull request. Uppercase on the
// wire, unlike ParticipantState. Open enum.
type ParticipantRole string

// Observed participant roles.
const (
	RoleReviewer    ParticipantRole = "REVIEWER"
	RoleParticipant ParticipantRole = "PARTICIPANT"
)

// ParticipantState is a participant's review decision. Lowercase on the wire,
// and JSON null (no decision yet) decodes to "". Open enum.
type ParticipantState string

// Observed participant states. The zero value means "has not acted".
const (
	ParticipantApproved         ParticipantState = "approved"
	ParticipantChangesRequested ParticipantState = "changes_requested"
)

// Participant is one reviewer or actor on a pull request.
type Participant struct {
	Type           string           `json:"type"`
	User           Account          `json:"user"`
	Role           ParticipantRole  `json:"role"`
	Approved       bool             `json:"approved"`
	State          ParticipantState `json:"state"`           // JSON null → ""
	ParticipatedOn time.Time        `json:"participated_on"` // JSON null → zero
}

// PullRequestLinks are the links on a pull request.
type PullRequestLinks struct {
	Self           Link `json:"self"`
	HTML           Link `json:"html"`
	Commits        Link `json:"commits"`
	Approve        Link `json:"approve"`
	RequestChanges Link `json:"request-changes"`
	Diff           Link `json:"diff"`
	DiffStat       Link `json:"diffstat"`
	Comments       Link `json:"comments"`
	Activity       Link `json:"activity"`
	Merge          Link `json:"merge"`
	Decline        Link `json:"decline"`
	Statuses       Link `json:"statuses"`
}

// PullRequest is a Bitbucket Cloud pull request. Reviewers and Participants are
// nil when the payload did not carry them (a list response without a fields=
// projection), which HasReviewData reports.
type PullRequest struct {
	Type              string              `json:"type"`
	ID                int                 `json:"id"` // unique per repository only
	Title             string              `json:"title"`
	Description       string              `json:"description"` // raw Markdown; may be ""
	State             PullRequestState    `json:"state"`
	Draft             bool                `json:"draft"`
	Queued            bool                `json:"queued"`
	CloseSourceBranch bool                `json:"close_source_branch"`
	Author            Account             `json:"author"`
	ClosedBy          *Account            `json:"closed_by"`    // nil while OPEN
	MergeCommit       *CommitRef          `json:"merge_commit"` // nil unless MERGED
	Reason            string              `json:"reason"`       // decline reason, "" otherwise
	CommentCount      int                 `json:"comment_count"`
	TaskCount         int                 `json:"task_count"`
	CreatedOn         time.Time           `json:"created_on"`
	UpdatedOn         time.Time           `json:"updated_on"`
	Source            PullRequestEndpoint `json:"source"`
	Destination       PullRequestEndpoint `json:"destination"`
	Summary           RenderedContent     `json:"summary"`
	Reviewers         []Account           `json:"reviewers"`    // nil = not loaded
	Participants      []Participant       `json:"participants"` // nil = not loaded
	Links             PullRequestLinks    `json:"links"`
}

// Key is the composite identity used by the TUI caches: "workspace/slug#id".
// A bare id collides across repositories.
func (pr *PullRequest) Key() string {
	return PullRequestKey(pr.Workspace(), pr.RepoSlug(), pr.ID)
}

// PullRequestKey builds the same key as PullRequest.Key from its parts.
func PullRequestKey(workspace, slug string, id int) string {
	return fmt.Sprintf("%s/%s#%d", workspace, slug, id)
}

// Workspace is the workspace slug of the destination repository.
func (pr *PullRequest) Workspace() string {
	return pr.Destination.Repository.Workspace()
}

// RepoSlug is the slug of the destination repository.
func (pr *PullRequest) RepoSlug() string {
	return pr.Destination.Repository.Slug()
}

// RepoFullName is "workspace/slug" for the destination repository.
func (pr *PullRequest) RepoFullName() string {
	if full := pr.Destination.Repository.FullName; full != "" {
		return full
	}
	return pr.Destination.Repository.Name
}

// SourceBranch is the branch the pull request merges from.
func (pr *PullRequest) SourceBranch() string {
	return pr.Source.Branch.Name
}

// DestinationBranch is the branch the pull request merges into.
func (pr *PullRequest) DestinationBranch() string {
	return pr.Destination.Branch.Name
}

// HTMLURL is the browser URL for the pull request. Note the hyphen: the web URL
// uses "pull-requests" where the API uses "pullrequests".
func (pr *PullRequest) HTMLURL() string {
	if pr.Links.HTML.Href != "" {
		return pr.Links.HTML.Href
	}
	return fmt.Sprintf("https://bitbucket.org/%s/%s/pull-requests/%d",
		pr.Workspace(), pr.RepoSlug(), pr.ID)
}

// HasReviewData reports whether the payload carried participants, i.e. whether
// ReviewSummary can say anything meaningful.
func (pr *PullRequest) HasReviewData() bool {
	return pr.Participants != nil
}

// IsAuthor reports whether the account with this braced UUID opened the PR.
func (pr *PullRequest) IsAuthor(uuid string) bool {
	return uuid != "" && pr.Author.UUID == uuid
}

// IsReviewer reports whether the account with this braced UUID is a requested
// reviewer, either in reviewers or as a participant with the REVIEWER role.
func (pr *PullRequest) IsReviewer(uuid string) bool {
	if uuid == "" {
		return false
	}
	for _, reviewer := range pr.Reviewers {
		if reviewer.UUID == uuid {
			return true
		}
	}
	for _, participant := range pr.Participants {
		if participant.Role == RoleReviewer && participant.User.UUID == uuid {
			return true
		}
	}
	return false
}

// ReviewSummary is the approval rollup shown in the PR list and detail panes.
type ReviewSummary struct {
	Reviewers        int // distinct reviewers = Reviewers ∪ participants who reviewed
	Approved         int
	ChangesRequested int
	Pending          int // Reviewers - Approved - ChangesRequested, floored at 0
}

// ReviewSummary counts distinct reviewers by account UUID. A participant with
// Approved=true counts as approved even when State is "". A participant who
// acted (approved or requested changes) counts as a reviewer regardless of role,
// because they reviewed de facto. All zero when the PR carries no review data.
func (pr *PullRequest) ReviewSummary() ReviewSummary {
	reviewers := make(map[string]struct{}, len(pr.Reviewers)+len(pr.Participants))
	for _, reviewer := range pr.Reviewers {
		reviewers[reviewer.UUID] = struct{}{}
	}

	approved := make(map[string]struct{}, len(pr.Participants))
	changesRequested := make(map[string]struct{}, len(pr.Participants))
	for _, participant := range pr.Participants {
		uuid := participant.User.UUID
		didApprove := participant.Approved || participant.State == ParticipantApproved
		didReject := participant.State == ParticipantChangesRequested

		if participant.Role == RoleReviewer || didApprove || didReject {
			reviewers[uuid] = struct{}{}
		}
		switch {
		case didApprove:
			approved[uuid] = struct{}{}
		case didReject:
			changesRequested[uuid] = struct{}{}
		}
	}

	summary := ReviewSummary{
		Reviewers:        len(reviewers),
		Approved:         len(approved),
		ChangesRequested: len(changesRequested),
		Pending:          0,
	}
	if pending := summary.Reviewers - summary.Approved - summary.ChangesRequested; pending > 0 {
		summary.Pending = pending
	}
	return summary
}

// CommitStatusState is a build status state. Open enum: third-party CI can post
// values Bitbucket Pipelines never uses.
type CommitStatusState string

// Observed build status states.
const (
	StatusSuccessful CommitStatusState = "SUCCESSFUL"
	StatusFailed     CommitStatusState = "FAILED"
	StatusInProgress CommitStatusState = "INPROGRESS"
	StatusStopped    CommitStatusState = "STOPPED"
)

// CommitStatus is a build status posted against a commit in a pull request.
type CommitStatus struct {
	Type        string            `json:"type"` // "build"
	Key         string            `json:"key"`  // NOT a run identifier ("prs:**:develop")
	Name        string            `json:"name"`
	Description string            `json:"description"`
	RefName     string            `json:"refname"`
	URL         string            `json:"url"` // .../pipelines/results/{build_number}
	State       CommitStatusState `json:"state"`
	Commit      *CommitRef        `json:"commit"`
	Repository  *RepositoryRef    `json:"repository"`
	CreatedOn   time.Time         `json:"created_on"`
	UpdatedOn   time.Time         `json:"updated_on"`
	Links       struct {
		Self   Link `json:"self"`
		Commit Link `json:"commit"`
	} `json:"links"`
}

// BuildNumber parses the trailing integer of URL when it has the shape
// ".../pipelines/results/{n}". It reports false for third-party status URLs.
func (s CommitStatus) BuildNumber() (int, bool) {
	const marker = "/pipelines/results/"

	index := strings.LastIndex(s.URL, marker)
	if index < 0 {
		return 0, false
	}
	digits := strings.TrimSuffix(s.URL[index+len(marker):], "/")
	if digits == "" {
		return 0, false
	}

	number := 0
	for _, char := range digits {
		if char < '0' || char > '9' {
			return 0, false
		}
		number = number*10 + int(char-'0')
	}
	if number == 0 {
		return 0, false
	}
	return number, true
}

// PipelineStateStage is the sub-state of an IN_PROGRESS pipeline.
type PipelineStateStage struct {
	Type string `json:"type"`
	Name string `json:"name"` // RUNNING | PAUSED
}

// PipelineError is the error attached to a failed pipeline or step.
type PipelineError struct {
	Type    string `json:"type"`
	Key     string `json:"key"`
	Message string `json:"message"`
}

// PipelineStateResult is the outcome of a COMPLETED pipeline.
type PipelineStateResult struct {
	Type       string         `json:"type"`
	Name       string         `json:"name"`       // SUCCESSFUL | FAILED | ERROR | STOPPED | EXPIRED
	Terminator *Account       `json:"terminator"` // STOPPED, optional even then
	Error      *PipelineError `json:"error"`      // ERROR
}

// PipelineState is the state of a pipeline run.
type PipelineState struct {
	Type   string               `json:"type"`
	Name   string               `json:"name"`   // PENDING | IN_PROGRESS | COMPLETED
	Stage  *PipelineStateStage  `json:"stage"`  // IN_PROGRESS only: RUNNING | PAUSED
	Result *PipelineStateResult `json:"result"` // COMPLETED only
}

// Summary is the single word the UI shows for the run.
func (s PipelineState) Summary() string {
	switch s.Name {
	case pipelineStateCompleted:
		if s.Result != nil && s.Result.Name != "" {
			return s.Result.Name
		}
	case pipelineStateInProgress:
		if s.Stage != nil && s.Stage.Name != "" {
			return s.Stage.Name
		}
	}
	return s.Name
}

// IsCompleted reports whether the run has finished.
func (s PipelineState) IsCompleted() bool {
	return s.Name == pipelineStateCompleted
}

// PipelineSelector is the bitbucket-pipelines.yml selector that matched the run.
type PipelineSelector struct {
	Type    string `json:"type"` // branches | tags | bookmarks | default | custom | pull-requests
	Pattern string `json:"pattern"`
}

// PipelinePullRequestRef is the pull request a PR-target pipeline ran for.
type PipelinePullRequestRef struct {
	Type   string `json:"type"`
	ID     int    `json:"id"`
	Title  string `json:"title"`
	Draft  bool   `json:"draft"`
	Queued bool   `json:"queued"`
	Links  struct {
		Self Link `json:"self"`
		HTML Link `json:"html"`
	} `json:"links"`
}

// PipelineTarget flattens the pipeline_ref_target / pipeline_commit_target /
// pipeline_pullrequest_target union. Discriminate on Type.
type PipelineTarget struct {
	Type              string                  `json:"type"`
	Selector          *PipelineSelector       `json:"selector"`
	Commit            *CommitRef              `json:"commit"`             // 40 chars
	RefType           string                  `json:"ref_type"`           // ref target
	RefName           string                  `json:"ref_name"`           // ref target
	Source            string                  `json:"source"`             // PR target: source branch
	Destination       string                  `json:"destination"`        // PR target: destination branch
	DestinationCommit *CommitRef              `json:"destination_commit"` // PR target, 12 chars
	PullRequest       *PipelinePullRequestRef `json:"pullrequest"`        // PR target
}

// BranchName is the branch the run was for: ref_name on a ref target, the source
// branch on a pull request target.
func (t PipelineTarget) BranchName() string {
	if t.RefName != "" {
		return t.RefName
	}
	return t.Source
}

// PipelineTrigger is what started a pipeline run.
type PipelineTrigger struct {
	Type string `json:"type"`
	Name string `json:"name"` // PUSH | MANUAL | SCHEDULE
}

// Pipeline is a Bitbucket Pipelines run. There is no html link on the wire; use
// HTMLURL to build the browser URL from the build number.
type Pipeline struct {
	Type              string          `json:"type"`
	UUID              string          `json:"uuid"` // braced
	BuildNumber       int             `json:"build_number"`
	State             PipelineState   `json:"state"`
	Creator           *Account        `json:"creator"` // may be a {type,uuid} stub
	Repository        *RepositoryRef  `json:"repository"`
	Target            PipelineTarget  `json:"target"`
	Trigger           PipelineTrigger `json:"trigger"`
	CreatedOn         time.Time       `json:"created_on"`
	CompletedOn       time.Time       `json:"completed_on"` // zero while running
	RunNumber         int             `json:"run_number"`
	DurationInSeconds int             `json:"duration_in_seconds"`
	BuildSecondsUsed  int             `json:"build_seconds_used"`
	FirstSuccessful   bool            `json:"first_successful"`
	Expired           bool            `json:"expired"`
	HasVariables      bool            `json:"has_variables"`
	Links             struct {
		Self  Link `json:"self"`
		Steps Link `json:"steps"`
	} `json:"links"`
}

// Duration is how long the run took. BuildSecondsUsed is a billing figure and
// can exceed it when steps run in parallel.
func (p Pipeline) Duration() time.Duration {
	return time.Duration(p.DurationInSeconds) * time.Second
}

// HTMLURL is the browser URL for the run, built from the embedded repository's
// full name because a pipeline payload carries no html link (its links.self uses
// UUID paths). It is "" when the run carries no repository, or one without a
// full_name — the caller then has nothing to open.
func (p Pipeline) HTMLURL() string {
	if p.Repository == nil || p.Repository.FullName == "" {
		return ""
	}
	return fmt.Sprintf("https://bitbucket.org/%s/pipelines/results/%d", p.Repository.FullName, p.BuildNumber)
}

// PipelineStepResult is the outcome of a COMPLETED step.
type PipelineStepResult struct {
	Type  string         `json:"type"`
	Name  string         `json:"name"` // SUCCESSFUL | FAILED | ERROR | STOPPED | NOT_RUN | EXPIRED
	Error *PipelineError `json:"error"`
}

// PipelineStepState is the state of one step in a run.
type PipelineStepState struct {
	Type   string              `json:"type"`
	Name   string              `json:"name"`   // PENDING | READY | IN_PROGRESS | COMPLETED
	Result *PipelineStepResult `json:"result"` // COMPLETED only
}

// Summary is the single word the UI shows for the step.
func (s PipelineStepState) Summary() string {
	if s.Name == pipelineStateCompleted && s.Result != nil && s.Result.Name != "" {
		return s.Result.Name
	}
	return s.Name
}

// PipelineImage is the container image a step ran in.
type PipelineImage struct {
	Name string `json:"name"`
}

// PipelineCommand is one command in a step's setup, script or teardown phase.
// Note the camelCase key in an otherwise snake_case payload.
type PipelineCommand struct {
	CommandType string `json:"commandType"` // user | system
	Name        string `json:"name"`
	Command     string `json:"command"`
	Action      string `json:"action"`
}

// PipelineStep is one step of a pipeline run. Legacy runs omit name and
// teardown_commands, and no step carries links or a build number.
type PipelineStep struct {
	Type              string            `json:"type"`
	UUID              string            `json:"uuid"`
	Name              string            `json:"name"` // may be absent on legacy steps
	State             PipelineStepState `json:"state"`
	StartedOn         time.Time         `json:"started_on"`
	CompletedOn       time.Time         `json:"completed_on"`
	DurationInSeconds int               `json:"duration_in_seconds"`
	BuildSecondsUsed  int               `json:"build_seconds_used"`
	RunNumber         int               `json:"run_number"`
	MaxTime           int               `json:"maxTime"` // minutes, camelCase on the wire
	Image             PipelineImage     `json:"image"`
	Trigger           struct {
		Type string `json:"type"`
	} `json:"trigger"`
	Pipeline struct {
		Type string `json:"type"`
		UUID string `json:"uuid"`
	} `json:"pipeline"`
	SetupCommands    []PipelineCommand `json:"setup_commands"`
	ScriptCommands   []PipelineCommand `json:"script_commands"`
	TeardownCommands []PipelineCommand `json:"teardown_commands"`
}

// DisplayName is the step's name, or "Step N" (1-based) for legacy steps that
// carry none. index is the step's position in the run's step list.
func (s PipelineStep) DisplayName(index int) string {
	if s.Name != "" {
		return s.Name
	}
	return fmt.Sprintf("Step %d", index+1)
}

// Duration is how long the step took.
func (s PipelineStep) Duration() time.Duration {
	return time.Duration(s.DurationInSeconds) * time.Second
}

// SameCommit reports whether two commit hashes refer to the same commit. Hashes
// arrive abbreviated to 12 chars in PR payloads and full at 40 chars elsewhere,
// so the comparison is a case-insensitive prefix match on the shorter hash.
// Hashes shorter than 7 characters never match.
func SameCommit(a, b string) bool {
	short, long := a, b
	if len(short) > len(long) {
		short, long = long, short
	}
	if len(short) < minCommitPrefix {
		return false
	}
	return strings.EqualFold(long[:len(short)], short)
}

// LatestStatuses keeps the newest status per key and returns them sorted by
// UpdatedOn descending. Bitbucket's own ordering is not newest-first.
func LatestStatuses(statuses []CommitStatus) []CommitStatus {
	if len(statuses) == 0 {
		return nil
	}

	positions := make(map[string]int, len(statuses))
	latest := make([]CommitStatus, 0, len(statuses))
	for _, status := range statuses {
		position, seen := positions[status.Key]
		if !seen {
			positions[status.Key] = len(latest)
			latest = append(latest, status)
			continue
		}
		if status.UpdatedOn.After(latest[position].UpdatedOn) {
			latest[position] = status
		}
	}

	slices.SortStableFunc(latest, func(a, b CommitStatus) int {
		return b.UpdatedOn.Compare(a.UpdatedOn)
	})
	return latest
}

// RollupStatuses reduces the latest status per key to the single state shown in
// the PR list: "" when there are none, then FAILED, INPROGRESS and STOPPED in
// that priority order, else SUCCESSFUL.
func RollupStatuses(statuses []CommitStatus) CommitStatusState {
	latest := LatestStatuses(statuses)
	if len(latest) == 0 {
		return ""
	}

	var inProgress, stopped bool
	for _, status := range latest {
		switch status.State {
		case StatusFailed:
			return StatusFailed
		case StatusInProgress:
			inProgress = true
		case StatusStopped:
			stopped = true
		default:
		}
	}
	switch {
	case inProgress:
		return StatusInProgress
	case stopped:
		return StatusStopped
	default:
		return StatusSuccessful
	}
}
