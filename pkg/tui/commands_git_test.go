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

	msg := app.gitCheckoutBranch("existing-branch")()

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

	msg := app.gitCheckoutBranch("origin/tracked-feature")()

	done, ok := msg.(gitCheckoutDoneMsg)
	if !ok {
		t.Fatalf("msg = %T (%v), want gitCheckoutDoneMsg", msg, msg)
	}
	testkit.AssertEqual(t, "stripped name", done.name, "tracked-feature")
}

func TestGitCheckoutBranch_NeverCreatesABranch(t *testing.T) {
	t.Parallel()
	app := gitApp(t, initGitRepo(t))

	msg := app.gitCheckoutBranch("no-such-branch")()

	failure, ok := msg.(gitErrorMsg)
	if !ok {
		t.Fatalf("msg = %T (%v), want gitErrorMsg", msg, msg)
	}
	if failure.err == nil || !containsAll(failure.err.Error(), "no-such-branch", "fetch first") {
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

	_, cmd := app.handleGitBranchSwitch(testBranch)

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
