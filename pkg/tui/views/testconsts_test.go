package views

import "regexp"

// Shared identities for the PR-shaped fixtures. Anything used four or more
// times in the package lives here so goconst stays quiet.
const (
	testWorkspace = "acme"
	testRepo      = "api-gateway"
	testRepo2     = "web-app"
	testPRKey     = "acme/api-gateway#101"
	testPRKey2    = "acme/api-gateway#102"
	testPRTitle   = "Add gateway auth"
	testAuthor    = "Grace Hopper"
	testReviewer  = "Ada Lovelace"

	// Review-tab fixtures: one changed file, one commit and one anchor line
	// shared by the Files, Commits, Comments and file-diff tests.
	testDiffPath   = "pkg/tui/app.go"
	testCommitHash = "abc1234def5678901234567890123456789abcde"

	// Pipelines-panel fixtures: the source branch of the pull request run and
	// the owner token a standalone run is shown under.
	testRunBranch = "bugfix/upstream-timeout"
	testRunOwner  = "acme/api-gateway#415"

	testPipelineUUID = "{11111111-1111-1111-1111-111111111111}"
	testStepUUID     = "{22222222-2222-2222-2222-222222222222}"
	testStepUUID2    = "{33333333-3333-3333-3333-333333333333}"

	// Mention fixtures: the two account-id shapes Bitbucket writes a mention
	// under, so a rendered span and a raw @{id} can name the same person.
	testMentionID  = "5d4a1c7e2b9f3a0001c8e6d2"
	testMentionID2 = "557058:8f2c1a4e-3b7d-4c9e-a1f0-2d6b8e4c7a19"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// stripANSI removes SGR sequences so view assertions can match plain text.
func stripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }
