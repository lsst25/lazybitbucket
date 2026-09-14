package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

func TestGitAvailable_FindsGitInPath(t *testing.T) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git binary not found; cannot construct test PATH")
	}
	t.Setenv("PATH", filepath.Dir(gitPath))
	testkit.AssertEqual(t, "GitAvailable with git dir in PATH", GitAvailable(), true)
}

func TestIsRepo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		insideRepo bool
		want       bool
	}{
		{name: "inside a repo", insideRepo: true, want: true},
		{name: "outside a repo", insideRepo: false, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if tt.insideRepo {
				dir = initRepo(t)
			}
			testkit.AssertEqual(t, "IsRepo", IsRepo(dir), tt.want)
		})
	}
}

func TestCurrentBranch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		nonRepo bool
		detach  bool
		want    string
	}{
		{name: "returns checked out branch", want: "main"},
		{name: "detached head returns empty", detach: true, want: ""},
		{name: "non repo returns empty", nonRepo: true, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if !tt.nonRepo {
				dir = initRepo(t)
				if tt.detach {
					gitRun(t, dir, "checkout", "-q", "--detach", "HEAD")
				}
			}

			got, err := CurrentBranch(dir)
			if err != nil {
				t.Fatalf("CurrentBranch: %v", err)
			}
			testkit.AssertEqual(t, "branch", got, tt.want)
		})
	}
}

func TestCurrentBranch_GitMissingFromPathReturnsError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", "")

	branch, err := CurrentBranch(dir)
	if err == nil {
		t.Fatal("CurrentBranch without git in PATH should error")
	}
	if branch != "" {
		t.Errorf("branch = %q, want empty on error", branch)
	}
}

func TestTopLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		nonRepo   bool
		subdir    string
		wantError bool
	}{
		{name: "repository root", nonRepo: false, subdir: "", wantError: false},
		{name: "subdirectory reports the root", nonRepo: false, subdir: "pkg/deep", wantError: false},
		{name: "non repo errors", nonRepo: true, subdir: "", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			if !tt.nonRepo {
				root = initRepo(t)
			}
			dir := root
			if tt.subdir != "" {
				dir = filepath.Join(root, tt.subdir)
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			}

			got, err := TopLevel(dir)
			if tt.wantError {
				if err == nil {
					t.Fatalf("TopLevel(%q) = %q, want an error outside a repository", dir, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("TopLevel: %v", err)
			}
			// macOS hands out /var symlinks for temporary directories, so
			// compare the resolved paths rather than the literals.
			wantRoot, resolveErr := filepath.EvalSymlinks(root)
			if resolveErr != nil {
				t.Fatal(resolveErr)
			}
			testkit.AssertEqual(t, "top level", got, wantRoot)
		})
	}
}

func TestLastCommitSubject(t *testing.T) {
	t.Parallel()

	// An unborn HEAD has no subject, and that is not a failure.
	empty := t.TempDir()
	gitRun(t, empty, "init", "-q", "-b", "main")
	configureRepo(t, empty)
	subject, err := LastCommitSubject(empty)
	if err != nil {
		t.Fatalf("LastCommitSubject() on an empty repository error = %v", err)
	}
	testkit.AssertEqual(t, "subject", subject, "")

	dir := initRepo(t)
	commitFile(t, dir, "a.txt", "hello\n", "feat: teach the parser about renames\n\nbody text")

	subject, err = LastCommitSubject(dir)
	if err != nil {
		t.Fatalf("LastCommitSubject() error = %v", err)
	}
	// The subject alone: the body of the message is not a pull request title.
	testkit.AssertEqual(t, "subject", subject, "feat: teach the parser about renames")
}
