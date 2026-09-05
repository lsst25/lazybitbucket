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
)
