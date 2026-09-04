package bitbucket

// Literals shared by more than a handful of tests in this package. Values match
// the inline JSON fixtures in models_test.go.
const (
	testWorkspace = "acme"
	testRepoSlug  = "api-gateway"
	testRepoFull  = "acme/api-gateway"

	testAuthorUUID   = "{e5743438-cc31-49a0-96b7-9febd7b946f4}"
	testReviewerUUID = "{b6b5d0ff-32d1-4e3c-816a-9253437180ca}"
	testOtherUUID    = "{781a664d-34db-4bc7-9c0d-bd3716d0d1de}"

	testMethod = "GET"
	testPath   = "/repositories/acme/api-gateway/pullrequests/9"
)

// Credentials the client tests build their ClientOpts from.
const (
	testToken = "secret-token"
	testEmail = "ci@example.com"
)

// Request paths the client tests assert on.
const (
	testRepoPath        = "/repositories/acme/api-gateway"
	testPullRequestPath = "/repositories/acme/api-gateway/pullrequests"
	testPipelinesPath   = "/repositories/acme/api-gateway/pipelines"
	testPipelineUUID    = "{210f1f44-67d9-4b63-b755-7f46d0af56d4}"
)

// Query parameter names and values the client sends.
const (
	paramPageLen    = "pagelen"
	paramSort       = "sort"
	paramState      = "state"
	paramFields     = "fields"
	sortUpdatedDesc = "-updated_on"
)

const contentTypeJSON = "application/json"

// emptyPage is a last-page envelope with no values.
const emptyPage = `{"pagelen": 10, "page": 1, "size": 0, "values": []}`

// Review sub-resource paths, all on pull request 9 — the same pull request
// testPath names, so an APIError path and a request path stay comparable.
const (
	testPRID           = 9
	testCommitsPath    = testPath + "/commits"
	testCommentsPath   = testPath + "/comments"
	testDiffPath       = testPath + "/diff"
	testDiffStatPath   = testPath + "/diffstat"
	testDiffTarget     = testRepoPath + "/diff/spec"
	testDiffStatTarget = testRepoPath + "/diffstat/spec"
)

const contentTypeText = "text/plain"

// authorizationHeader is the header the same-host redirect must preserve.
const authorizationHeader = "Authorization"

// testDiffFilePath is the file the inline comment and diff fixtures anchor to.
const testDiffFilePath = "pkg/tui/app.go"
