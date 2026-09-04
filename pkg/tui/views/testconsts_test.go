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

	testPipelineUUID = "{11111111-1111-1111-1111-111111111111}"
	testStepUUID     = "{22222222-2222-2222-2222-222222222222}"
	testStepUUID2    = "{33333333-3333-3333-3333-333333333333}"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// stripANSI removes SGR sequences so view assertions can match plain text.
func stripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }
