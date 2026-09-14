package tui

const (
	testWorkspace = "acme"
	testRepo      = "api-gateway"
	testRepo2     = "web-app"

	testPRKey  = "acme/api-gateway#101"
	testPRKey2 = "acme/api-gateway#102"
	testTitle  = "Add a token bucket rate limiter"

	testUserUUID   = "{00000000-0000-0000-0000-000000000001}"
	testOtherUUID  = "{00000000-0000-0000-0000-000000000002}"
	testThirdUUID  = "{00000000-0000-0000-0000-000000000003}"
	testFourthUUID = "{00000000-0000-0000-0000-000000000004}"

	testBranch   = "feature/rate-limiter"
	branchMain   = "main"
	remoteOrigin = "origin"

	// testDescription is the pull request body the description editor edits.
	testDescription = "See https://example.test/spec for the design."

	// The two pipeline runs the rerun and stop flows act on: the build the
	// fixtures put under the Builds panel cursor, and the run a rerun of it
	// creates.
	testBuild    = 412
	testNewBuild = 413

	// testOtherBuild is the second repository's run in the cross-repository
	// pipeline list, and testRunKey and testOtherRunKey are the two runs'
	// keys — the "ws/slug#build" shape the pipelines view addresses a run by.
	testOtherBuild  = 77
	testStepUUID    = "{step-1}"
	testRunKey      = "acme/api-gateway#412"
	testOtherRunKey = "acme/web-app#77"

	// testRunCreator is who the fixture runs say started them, which is the
	// third thing the pipeline dialogs name a run by.
	testRunCreator = "Rita Runner"

	// testTaskURL is the queued-merge task a 202 would point at.
	testTaskURL = "https://api.example.test/2.0/repositories/acme/api-gateway/pullrequests/101/merge/task-status/t-1"

	testOpenState = "OPEN"
	testFetchKey  = "OPEN|"

	testBoom = "boom"

	// The two files of the review fixture diff (see review_tabs_test.go).
	testFileGo = "internal/rate/bucket.go"
	testFileMD = "docs/rate.md"

	// testSourceHash is the source commit the review fixtures describe; a
	// second hash stands in for a push.
	testSourceHash  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testPushedHash  = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	testThreadRoot  = int64(9001)
	testCommitHash  = "cccccccccccccccccccccccccccccccccccccccc"
	testCommitShort = "ccccccc"

	// testStepLog is the body the stubbed step-log fetch answers with.
	testStepLog = "+ go build ./..."

	// testEditor is the editor a description edit suspends into: /usr/bin/true
	// exits straight away and leaves the temp file alone, so the test writes
	// the "edited" body itself.
	testEditor = "true"

	// testMentionID is an account id in the 24-hex shape and testMentionRaw is
	// the mention Bitbucket stores against it, which is what a body still shows
	// when no rendering came with it.
	testMentionID  = "5d4a1c7e2b9f3a0001c8e6d2"
	testMentionRaw = "@{" + testMentionID + "}"

	// The two demo accounts the mention fixtures name, as a reader sees them.
	// The colleague who has left is demo.AccountIDGone: it belongs to no
	// account, so Bitbucket renders it as literal text and so does the detail
	// panel.
	mentionDemoUser = "@Demo User"
	mentionKen      = "@Ken Thompson"
)
