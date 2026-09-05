package demo

// Literals used by more than a handful of tests in this package. Repository
// slugs, the workspace and the state/result words are already package constants
// in data.go, so they are not repeated here.
const (
	pathUser         = "/user"
	pathWorkspace    = "/workspaces/" + Workspace
	pathRepositories = "/repositories/" + Workspace

	segmentPullRequests = "/pullrequests"
	segmentStatuses     = "/statuses"
	segmentPipelines    = "/pipelines"
	segmentSteps        = "/steps"
	segmentStopPipeline = "/stopPipeline"

	queryPageLenTwo = "?pagelen=2"

	fieldNext         = "next"
	fieldSize         = "size"
	fieldValues       = "values"
	fieldReviewers    = "reviewers"
	fieldParticipants = "participants"
	fieldCompletedOn  = "completed_on"

	headerAuthorization = "Authorization"
	demoAuthorization   = "Bearer demo-token"
)

// Review route segments and the members their envelopes carry.
const (
	segmentCommits  = "/commits"
	segmentComments = "/comments"
	segmentDiff     = "/diff"
	segmentDiffStat = "/diffstat"

	fieldPage = "page"

	// Write route segments and the members their documents carry.
	segmentMerge      = "/merge"
	segmentTaskStatus = "/task-status"
	segmentApprove    = "/approve"
	pathMembers       = pathWorkspace + "/members"

	fieldTaskStatus  = "task_status"
	fieldMergeResult = "merge_result"
	fieldUser        = "user"

	typeWorkspaceMembership = "workspace_membership"

	updatedTitle = "Rate limit the public routes"

	headerContentType = "Content-Type"
	headerLocation    = "Location"
)

// The pipeline write fixtures, the runs the two pipeline routes are shown on.
// They are all in api-gateway, which is the only repository with a run in every
// state the actions care about.
const (
	// buildRunning is the dataset's only IN_PROGRESS run, and therefore the
	// only one a stop succeeds against.
	buildRunning = 413
	// buildFailed is the FAILED run a rerun is worth doing, and buildRerun is
	// the run that rerunning it creates.
	buildFailed = 414
	buildRerun  = 415

	// triggerManual is the trigger every run started through the API carries,
	// however the run it repeats was triggered.
	triggerManual = "MANUAL"
)
