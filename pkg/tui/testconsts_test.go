package tui

const (
	testWorkspace = "acme"
	testRepo      = "api-gateway"
	testRepo2     = "web-app"

	testPRKey  = "acme/api-gateway#101"
	testPRKey2 = "acme/api-gateway#102"
	testTitle  = "Add a token bucket rate limiter"

	testUserUUID  = "{00000000-0000-0000-0000-000000000001}"
	testOtherUUID = "{00000000-0000-0000-0000-000000000002}"

	testBranch = "feature/rate-limiter"

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
