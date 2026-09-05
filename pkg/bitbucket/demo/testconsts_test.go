package demo

// Literals used by more than a handful of tests in this package. Repository
// slugs, the workspace and the state/result words are already package constants
// in data.go, so they are not repeated here.
const (
	pathUser         = "/user"
	pathWorkspace    = "/workspaces/" + Workspace
	pathRepositories = "/repositories/" + Workspace

	segmentPullRequests = "/pullrequests"
	segmentPipelines    = "/pipelines"
	segmentSteps        = "/steps"

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
	pathMembers       = pathWorkspace + "/members"

	fieldTaskStatus  = "task_status"
	fieldMergeResult = "merge_result"
	fieldUser        = "user"

	typeWorkspaceMembership = "workspace_membership"

	updatedTitle = "Rate limit the public routes"

	headerContentType = "Content-Type"
	headerLocation    = "Location"
)
