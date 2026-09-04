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
