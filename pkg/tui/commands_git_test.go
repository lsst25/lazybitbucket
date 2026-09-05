package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/bitbuckettest"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", dir},
		{"-C", dir, "config", "user.email", "test@test.com"},
		{"-C", dir, "config", "user.name", "Test"},
	} {
		if err := runGit(t, args); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("init"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	for _, args := range [][]string{
		{"-C", dir, "add", "."},
		{"-C", dir, "commit", "-m", "init"},
	} {
		if err := runGit(t, args); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	return dir
}

func runGit(t *testing.T, args []string) error {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("git %v output: %s", args, out)
	}
	return err
}

func gitApp(t *testing.T, repoDir string) *App {
	t.Helper()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	app.gitRepoPath = repoDir
	return app
}

func TestGitCheckoutBranch_ChecksOutALocalBranch(t *testing.T) {
	t.Parallel()
	repoDir := initGitRepo(t)
	if err := runGit(t, []string{"-C", repoDir, "branch", "existing-branch"}); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	app := gitApp(t, repoDir)

	msg := app.gitCheckoutBranch(testWorkspace, testRepo, "existing-branch")()

	done, ok := msg.(gitCheckoutDoneMsg)
	if !ok {
		t.Fatalf("msg = %T (%v), want gitCheckoutDoneMsg", msg, msg)
	}
	testkit.AssertEqual(t, "branch name", done.name, "existing-branch")
}

func TestGitCheckoutBranch_TracksARemoteBranchAndStripsThePrefix(t *testing.T) {
	t.Parallel()
	remotePath := initGitRepo(t)
	if err := runGit(t, []string{"-C", remotePath, "branch", "tracked-feature"}); err != nil {
		t.Fatalf("create remote branch: %v", err)
	}
	repoDir := initGitRepo(t)
	if err := runGit(t, []string{"-C", repoDir, "remote", "add", "origin", remotePath}); err != nil {
		t.Fatalf("add remote: %v", err)
	}
	if err := runGit(t, []string{"-C", repoDir, "fetch", "origin"}); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	app := gitApp(t, repoDir)

	msg := app.gitCheckoutBranch(testWorkspace, testRepo, "origin/tracked-feature")()

	done, ok := msg.(gitCheckoutDoneMsg)
	if !ok {
		t.Fatalf("msg = %T (%v), want gitCheckoutDoneMsg", msg, msg)
	}
	testkit.AssertEqual(t, "stripped name", done.name, "tracked-feature")
}

func TestGitCheckoutBranch_NeverCreatesABranch(t *testing.T) {
	t.Parallel()
	app := gitApp(t, initGitRepo(t))

	msg := app.gitCheckoutBranch(testWorkspace, testRepo, "no-such-branch")()

	failure, ok := msg.(gitErrorMsg)
	if !ok {
		t.Fatalf("msg = %T (%v), want gitErrorMsg", msg, msg)
	}
	if failure.err == nil || !containsAll(failure.err.Error(), "no-such-branch", "no remote of this checkout points at "+testWorkspace+"/"+testRepo) {
		t.Errorf("err = %v, want an explanation naming the branch", failure.err)
	}
}

func TestHandleActionCheckout(t *testing.T) {
	t.Parallel()

	t.Run("outside a git repository it is an error, not a crash", func(t *testing.T) {
		t.Parallel()
		app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
		pr := newPR(testRepo, 101, testTitle)
		app.prList.SetPullRequests([]bitbucket.PullRequest{pr})

		_, cmd, handled := app.handleActionCheckout()

		if !handled || cmd != nil {
			t.Error("checkout outside a repository should be handled without running anything")
		}
		testkit.AssertEqual(t, "status error", app.statusPanel.ErrorMessage(), "not inside a git repository")
	})

	t.Run("with no selection it does nothing", func(t *testing.T) {
		t.Parallel()
		app := gitApp(t, initGitRepo(t))

		_, cmd, handled := app.handleActionCheckout()

		if !handled || cmd != nil {
			t.Error("checkout without a pull request should be a no-op")
		}
	})

	t.Run("with a selection it checks out the source branch", func(t *testing.T) {
		t.Parallel()
		repoDir := initGitRepo(t)
		if err := runGit(t, []string{"-C", repoDir, "branch", testBranch}); err != nil {
			t.Fatalf("create branch: %v", err)
		}
		app := gitApp(t, repoDir)
		app.prList.SetPullRequests([]bitbucket.PullRequest{newPR(testRepo, 101, testTitle)})

		_, cmd, handled := app.handleActionCheckout()

		if !handled || cmd == nil {
			t.Fatal("checkout should have produced a command")
		}
		done, ok := cmd().(gitCheckoutDoneMsg)
		if !ok {
			t.Fatalf("checkout did not complete: %v", cmd())
		}
		testkit.AssertEqual(t, "branch", done.name, testBranch)
	})
}

func TestHandleGitBranchSwitch_QuitsWhenConfigured(t *testing.T) {
	t.Parallel()
	app := sizedApp(t, &bitbuckettest.FakeClient{T: t})
	app.cfg.Git.CloseOnCheckout = true

	_, cmd := app.handleGitBranchSwitch(testBranch, "")

	if cmd == nil {
		t.Error("closeOnCheckout should quit after a checkout")
	}
	testkit.AssertEqual(t, "git branch", app.gitBranch, testBranch)
}

func containsAll(s string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(s, part) {
			return false
		}
	}
	return true
}

// checkoutRepo builds a bare "remote" whose path ends in <workspace>/<slug>.git —
// which is what RemoteFor matches — and a clone of it on main.
func checkoutRepo(t *testing.T) (remote, work string) {
	t.Helper()
	remote = filepath.Join(t.TempDir(), testWorkspace, testRepo+".git")
	if err := os.MkdirAll(remote, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := runGit(t, []string{"-C", remote, "init", "--bare", "-q", "-b", branchMain}); err != nil {
		t.Fatalf("init bare: %v", err)
	}
	seed := initGitRepo(t)
	for _, args := range [][]string{
		{"-C", seed, "branch", "-M", branchMain},
		{"-C", seed, "remote", "add", "origin", remote},
		{"-C", seed, "push", "-q", "origin", branchMain},
	} {
		if err := runGit(t, args); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	work = t.TempDir()
	for _, args := range [][]string{
		{"clone", "-q", remote, work},
		{"-C", work, "config", "user.email", "test@test.com"},
		{"-C", work, "config", "user.name", "Test"},
	} {
		if err := runGit(t, args); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	return remote, work
}

// pushBranchFrom creates branch on the remote through a throwaway clone, adding
// one commit with the given file, so the clone under test has never seen it.
func pushBranchFrom(t *testing.T, remote, branch, file string) {
	t.Helper()
	other := t.TempDir()
	for _, args := range [][]string{
		{"clone", "-q", remote, other},
		{"-C", other, "config", "user.email", "test@test.com"},
		{"-C", other, "config", "user.name", "Test"},
	} {
		if err := runGit(t, args); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	// Continue the branch where the remote has it, else start it from main.
	start := []string{"-C", other, "checkout", "-q", "-B", branch}
	if runGit(t, []string{"-C", other, "rev-parse", "--verify", "-q", "origin/" + branch}) == nil {
		start = append(start, "origin/"+branch)
	}
	if err := runGit(t, start); err != nil {
		t.Fatalf("git %v: %v", start, err)
	}
	if err := os.WriteFile(filepath.Join(other, file), []byte(file), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"-C", other, "add", "."},
		{"-C", other, "commit", "-q", "-m", file},
		{"-C", other, "push", "-q", "origin", branch},
	} {
		if err := runGit(t, args); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
}

func headOf(t *testing.T, dir, rev string) string {
	t.Helper()
	out, err := exec.CommandContext(t.Context(), "git", "-C", dir, "rev-parse", rev).Output()
	if err != nil {
		t.Fatalf("rev-parse %s: %v", rev, err)
	}
	return strings.TrimSpace(string(out))
}

func TestGitCheckoutBranch_FetchesTheBranchItHasNeverSeen(t *testing.T) {
	t.Parallel()
	remote, work := checkoutRepo(t)
	pushBranchFrom(t, remote, testBranch, "feature.txt")
	app := gitApp(t, work)

	msg := app.gitCheckoutBranch(testWorkspace, testRepo, testBranch)()

	done, ok := msg.(gitCheckoutDoneMsg)
	if !ok {
		t.Fatalf("msg = %T (%v), want gitCheckoutDoneMsg", msg, msg)
	}
	testkit.AssertEqual(t, "branch", done.name, testBranch)
	testkit.AssertEqual(t, "note", done.note, "")
	testkit.AssertEqual(t, "HEAD matches the remote branch", headOf(t, work, "HEAD"), headOf(t, remote, testBranch))
}

func TestGitCheckoutBranch_FastForwardsAStaleLocalBranch(t *testing.T) {
	t.Parallel()
	remote, work := checkoutRepo(t)
	// The clone has the branch at the remote's first commit…
	pushBranchFrom(t, remote, testBranch, "one.txt")
	if err := runGit(t, []string{"-C", work, "fetch", "-q", "origin"}); err != nil {
		t.Fatal(err)
	}
	if err := runGit(t, []string{"-C", work, "branch", "-q", testBranch, "origin/" + testBranch}); err != nil {
		t.Fatal(err)
	}
	// …and the remote moves on.
	pushBranchFrom(t, remote, testBranch, "two.txt")
	app := gitApp(t, work)

	msg := app.gitCheckoutBranch(testWorkspace, testRepo, testBranch)()

	done, ok := msg.(gitCheckoutDoneMsg)
	if !ok {
		t.Fatalf("msg = %T (%v), want gitCheckoutDoneMsg", msg, msg)
	}
	testkit.AssertEqual(t, "note", done.note, "fast-forwarded to origin/"+testBranch)
	testkit.AssertEqual(t, "HEAD caught up", headOf(t, work, "HEAD"), headOf(t, remote, testBranch))
}

func TestGitCheckoutBranch_LeavesADivergedBranchAlone(t *testing.T) {
	t.Parallel()
	remote, work := checkoutRepo(t)
	pushBranchFrom(t, remote, testBranch, "one.txt")
	if err := runGit(t, []string{"-C", work, "fetch", "-q", "origin"}); err != nil {
		t.Fatal(err)
	}
	// A local commit the remote does not have…
	for _, args := range [][]string{
		{"-C", work, "checkout", "-q", "-b", testBranch, "origin/" + testBranch},
		{"-C", work, "commit", "-q", "--allow-empty", "-m", "local work"},
		{"-C", work, "checkout", "-q", branchMain},
	} {
		if err := runGit(t, args); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	local := headOf(t, work, testBranch)
	// …and a remote commit the checkout does not have.
	pushBranchFrom(t, remote, testBranch, "two.txt")
	app := gitApp(t, work)

	msg := app.gitCheckoutBranch(testWorkspace, testRepo, testBranch)()

	done, ok := msg.(gitCheckoutDoneMsg)
	if !ok {
		t.Fatalf("msg = %T (%v), want gitCheckoutDoneMsg", msg, msg)
	}
	if !strings.Contains(done.note, "diverged") {
		t.Errorf("note = %q, want the divergence note", done.note)
	}
	testkit.AssertEqual(t, "local commits kept", headOf(t, work, "HEAD"), local)
}

func TestGitCheckoutBranch_ReportsAMissingRemoteBranchByName(t *testing.T) {
	t.Parallel()
	_, work := checkoutRepo(t)
	app := gitApp(t, work)

	msg := app.gitCheckoutBranch(testWorkspace, testRepo, "never-pushed")()

	failure, ok := msg.(gitErrorMsg)
	if !ok {
		t.Fatalf("msg = %T (%v), want gitErrorMsg", msg, msg)
	}
	if failure.err == nil || !containsAll(failure.err.Error(), "never-pushed", "not found locally or on origin") {
		t.Errorf("err = %v", failure.err)
	}
}
