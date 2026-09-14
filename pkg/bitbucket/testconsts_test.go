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

	testMethod     = "GET"
	testMethodPost = "POST"
	testMethodPut  = "PUT"
	testPath       = "/repositories/acme/api-gateway/pullrequests/9"
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

// Write endpoint paths, all on the same pull request the read fixtures use.
const (
	testMergePath            = testPath + "/merge"
	testApprovePath          = testPath + "/approve"
	testTaskPath             = testMergePath + "/task-status/t-9"
	testDefaultReviewersPath = testRepoPath + "/effective-default-reviewers"
	testMembersPath          = "/workspaces/acme/members"
)

// testStopPipelinePath is the stop endpoint of the run the pipeline fixtures
// use, with the braced UUID percent-encoded as it goes on the wire.
const testStopPipelinePath = testPipelinesPath +
	"/%7B210f1f44-67d9-4b63-b755-7f46d0af56d4%7D/stopPipeline"

// contentTypeHeader is the header a JSON body must carry.
const contentTypeHeader = "Content-Type"

// writePRFixture is the smallest pull request body a write endpoint answers
// with: enough to prove the response was decoded, not the whole document.
const writePRFixture = `{
  "type": "pullrequest",
  "id": 9,
  "title": "Add rate limiting",
  "state": "MERGED",
  "draft": false,
  "destination": {
    "branch": {"name": "main"},
    "repository": {"type": "repository", "name": "api-gateway", "full_name": "acme/api-gateway"}
  }
}`

// authorizationHeader is the header the same-host redirect must preserve.
const authorizationHeader = "Authorization"

// testDiffFilePath is the file the inline comment and diff fixtures anchor to.
const testDiffFilePath = "pkg/tui/app.go"

// Mention fixtures. The two ids follow the two account id shapes seen on the
// wire — a 24-hex Atlassian id and "<digits>:<uuid>" — without being anyone's
// real id, and are examples of the documented charset, not the whole of it.
// testMentionGoneID is the departed colleague no rendering ever names, the id
// Bitbucket itself leaves literal.
const (
	testMentionID     = "5f3a9c1e7b2d4a6f8e0c1b3d"
	testMentionUUID   = "557058:1f7c3a9e-2b4d-4c6e-8a0f-3d5b7e9c1a2f"
	testMentionGoneID = "712020:9b1d3f5a-7c2e-4a8b-b0d6-4e6f8a1c2d3e"

	testMentionName       = "Margaret Hamilton"
	testMentionSecondName = "Grace Hopper"
)
