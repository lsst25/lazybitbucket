package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	gitRun(t, dir, "init", "-q", "-b", "main")
	configureRepo(t, dir)

	commitFile(t, dir, "README", "x\n", "init")
	return dir
}

func addRemoteRef(t *testing.T, dir, ref string) {
	t.Helper()
	gitRun(t, dir, "update-ref", "refs/remotes/"+ref, "HEAD")
}

func createBranch(t *testing.T, dir, name string) {
	t.Helper()
	gitRun(t, dir, "branch", name)
}

// gitOut runs git in dir and returns its trimmed standard output.
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v failed: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

// configureRepo gives a repository the identity every commit needs, without
// depending on the machine's global git config.
func configureRepo(t *testing.T, dir string) {
	t.Helper()
	gitRun(t, dir, "config", "user.email", "test@example.com")
	gitRun(t, dir, "config", "user.name", "test")
	gitRun(t, dir, "config", "commit.gpgsign", "false")
}

// writeFile writes a file into a repository without staging it.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// commitFile writes a file in dir and commits it.
func commitFile(t *testing.T, dir, name, content, message string) {
	t.Helper()
	writeFile(t, dir, name, content)
	gitRun(t, dir, "add", name)
	gitRun(t, dir, "commit", "-q", "-m", message)
}

// revParse resolves a revision to its commit hash.
func revParse(t *testing.T, dir, rev string) string {
	t.Helper()
	return gitOut(t, dir, "rev-parse", rev)
}

// checkout switches to an existing branch.
func checkout(t *testing.T, dir, name string) {
	t.Helper()
	gitRun(t, dir, "checkout", "-q", name)
}

// checkoutNew creates a branch at HEAD and switches to it.
func checkoutNew(t *testing.T, dir, name string) {
	t.Helper()
	gitRun(t, dir, "checkout", "-q", "-b", name)
}

// cloneWork clones a local bare repository into a fresh working tree. The
// clone inherits no identity, so it gets its own.
func cloneWork(t *testing.T, remote string) string {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "clone", "-q", remote, dir)
	configureRepo(t, dir)
	return dir
}

// initRemoteAndClone builds a bare repository holding one commit on main plus
// a working clone of it, the shape every fetch/push test needs. No network is
// involved: the "remote" is a local directory.
func initRemoteAndClone(t *testing.T) (remote, work string) {
	t.Helper()

	remote = t.TempDir()
	gitRun(t, remote, "init", "--bare", "-q", "-b", "main")

	seed := initRepo(t)
	gitRun(t, seed, "remote", "add", "origin", remote)
	gitRun(t, seed, "push", "-q", "origin", "main")

	return remote, cloneWork(t, remote)
}
