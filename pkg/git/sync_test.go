package git

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

const (
	remoteOrigin  = "origin"
	branchMain    = "main"
	branchFeature = "feature"
	// branchDash is a branch name git's ref-format rules allow but its option
	// parser would read as a flag. Only plumbing creates it; see ErrUnsafeName.
	branchDash = "-evil"
	headRev    = "HEAD"

	fileApp     = "app.txt"
	fileFeature = "feature.txt"

	workspaceAcme = "acme"
	slugGateway   = "api-gateway"

	labelIsClean = "IsClean"
	labelHead    = "HEAD"
)

func TestIsClean_CleanWorkingTree(t *testing.T) {
	t.Parallel()
	dir := initRepo(t)

	clean, err := IsClean(dir)
	if err != nil {
		t.Fatalf("IsClean: %v", err)
	}
	testkit.AssertEqual(t, labelIsClean, clean, true)
}

func TestIsClean_ModifiedFile(t *testing.T) {
	t.Parallel()
	dir := initRepo(t)
	writeFile(t, dir, "README", "changed\n")

	clean, err := IsClean(dir)
	if err != nil {
		t.Fatalf("IsClean: %v", err)
	}
	testkit.AssertEqual(t, labelIsClean, clean, false)
}

func TestIsClean_UntrackedFile(t *testing.T) {
	t.Parallel()
	dir := initRepo(t)
	writeFile(t, dir, fileApp, "new\n")

	clean, err := IsClean(dir)
	if err != nil {
		t.Fatalf("IsClean: %v", err)
	}
	testkit.AssertEqual(t, labelIsClean, clean, false)
}

func TestIsClean_StagedChange(t *testing.T) {
	t.Parallel()
	dir := initRepo(t)
	writeFile(t, dir, fileApp, "new\n")
	gitRun(t, dir, "add", fileApp)

	clean, err := IsClean(dir)
	if err != nil {
		t.Fatalf("IsClean: %v", err)
	}
	testkit.AssertEqual(t, labelIsClean, clean, false)
}

func TestIsClean_NotARepoReturnsError(t *testing.T) {
	t.Parallel()

	if _, err := IsClean(t.TempDir()); err == nil {
		t.Error("expected an error outside a repository, got nil")
	}
}

func TestRemoteFor(t *testing.T) {
	t.Parallel()

	type remote struct{ name, url string }

	tests := []struct {
		name    string
		remotes []remote
		want    string
		wantOK  bool
	}{
		{
			name:    "https url",
			remotes: []remote{{remoteOrigin, "https://bitbucket.org/other/repo.git"}, {"bb", "https://bitbucket.org/acme/api-gateway.git"}},
			want:    "bb",
			wantOK:  true,
		},
		{
			name:    "scp-like ssh url",
			remotes: []remote{{"upstream", "git@bitbucket.org:acme/api-gateway.git"}},
			want:    "upstream",
			wantOK:  true,
		},
		{
			name:    "ssh scheme url",
			remotes: []remote{{"upstream", "ssh://git@bitbucket.org/acme/api-gateway.git"}},
			want:    "upstream",
			wantOK:  true,
		},
		{
			name:    "no dot-git suffix",
			remotes: []remote{{"bb", "https://bitbucket.org/acme/api-gateway"}},
			want:    "bb",
			wantOK:  true,
		},
		{
			name:    "case insensitive",
			remotes: []remote{{"bb", "https://bitbucket.org/ACME/API-Gateway.git"}},
			want:    "bb",
			wantOK:  true,
		},
		{
			name:    "url with credentials",
			remotes: []remote{{"bb", "https://someone@bitbucket.org/acme/api-gateway.git"}},
			want:    "bb",
			wantOK:  true,
		},
		{
			name: "a bitbucket remote wins over a mirror with the same path",
			remotes: []remote{
				{"mirror", "https://github.com/acme/api-gateway.git"},
				{"bb", "git@bitbucket.org:acme/api-gateway.git"},
			},
			want:   "bb",
			wantOK: true,
		},
		{
			name:    "a lone match elsewhere still counts",
			remotes: []remote{{"local", "/srv/git/acme/api-gateway.git"}},
			want:    "local",
			wantOK:  true,
		},
		{
			name:    "other repository is not ok — there is no origin fallback",
			remotes: []remote{{remoteOrigin, "https://bitbucket.org/other/api-gateway.git"}},
			want:    "",
			wantOK:  false,
		},
		{
			name:    "slug prefix does not match",
			remotes: []remote{{remoteOrigin, "https://bitbucket.org/acme/api-gateway-v2.git"}},
			want:    "",
			wantOK:  false,
		},
		{
			name:    "no remotes",
			remotes: nil,
			want:    "",
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := initRepo(t)
			for _, r := range tt.remotes {
				gitRun(t, dir, "remote", "add", r.name, r.url)
			}

			got, ok := RemoteFor(dir, workspaceAcme, slugGateway)
			testkit.AssertEqual(t, "RemoteFor", got, tt.want)
			testkit.AssertEqual(t, "ok", ok, tt.wantOK)
		})
	}
}

func TestRemoteFor_NotARepoIsNotOK(t *testing.T) {
	t.Parallel()

	got, ok := RemoteFor(t.TempDir(), workspaceAcme, slugGateway)
	testkit.AssertEqual(t, "RemoteFor", got, "")
	testkit.AssertEqual(t, "ok", ok, false)
}

func TestIsAncestor(t *testing.T) {
	t.Parallel()
	dir := initRepo(t)
	first := revParse(t, dir, "HEAD")
	commitFile(t, dir, "second.txt", "two", "second")
	second := revParse(t, dir, "HEAD")

	got, err := IsAncestor(dir, first, second)
	if err != nil {
		t.Fatalf("IsAncestor: %v", err)
	}
	testkit.AssertEqual(t, "first is an ancestor of second", got, true)

	got, err = IsAncestor(dir, second, first)
	if err != nil {
		t.Fatalf("IsAncestor: %v", err)
	}
	testkit.AssertEqual(t, "second is not an ancestor of first", got, false)

	if _, err := IsAncestor(dir, "no-such-ref", second); err == nil {
		t.Error("an unknown revision must be an error, not false")
	}
}

func TestFetch_UpdatesRemoteState(t *testing.T) {
	t.Parallel()
	remote, work := initRemoteAndClone(t)

	// A second checkout of the same "remote" publishes a commit the first
	// clone has never seen.
	other := cloneWork(t, remote)
	commitFile(t, other, fileApp, "one\n", "add app")
	gitRun(t, other, "push", "-q", remoteOrigin, branchMain)
	published := revParse(t, other, headRev)

	if err := Fetch(work, remoteOrigin, branchMain); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	testkit.AssertEqual(t, "FETCH_HEAD", revParse(t, work, "FETCH_HEAD"), published)
	testkit.AssertEqual(t, "origin/main", revParse(t, work, "origin/"+branchMain), published)
}

func TestFetch_UnknownRemoteReturnsError(t *testing.T) {
	t.Parallel()
	_, work := initRemoteAndClone(t)

	if err := Fetch(work, "ghost", branchMain); err == nil {
		t.Error("expected an error fetching from an unknown remote, got nil")
	}
}

// TestFetch_BranchNameStartingWithDash pins that the option-injection fix does
// not cost a legitimate ref: git's ref-format rules allow refs/heads/-evil, so
// a repository can carry one and syncing against it has to keep working.
func TestFetch_BranchNameStartingWithDash(t *testing.T) {
	t.Parallel()
	remote, work := initRemoteAndClone(t)
	gitRun(t, remote, "update-ref", "refs/heads/"+branchDash, headRev)
	published := revParse(t, remote, "refs/heads/"+branchDash)

	if err := Fetch(work, remoteOrigin, branchDash); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	testkit.AssertEqual(t, "origin/-evil", revParse(t, work, "refs/remotes/"+remoteOrigin+"/"+branchDash), published)
}

// TestFetch_OptionLikeBranchIsNotExecuted covers the argument-injection hole:
// git fetch keeps parsing options past the remote, so a branch name of
// "--upload-pack=<command>" used to make git run that command. The name must
// reach git as a refspec and be rejected as one, leaving no trace behind.
func TestFetch_OptionLikeBranchIsNotExecuted(t *testing.T) {
	t.Parallel()
	_, work := initRemoteAndClone(t)
	sentinel := filepath.Join(t.TempDir(), "executed")

	err := Fetch(work, remoteOrigin, "--upload-pack=touch "+sentinel+"; git-upload-pack")
	if err == nil {
		t.Error("expected an error for an option-like branch name, got nil")
	}
	assertMissing(t, sentinel)
}

func TestMergeBranch_FastForward(t *testing.T) {
	t.Parallel()
	dir := initRepo(t)
	createBranch(t, dir, branchFeature)
	commitFile(t, dir, fileApp, "main work\n", "main work")
	mainTip := revParse(t, dir, headRev)
	checkout(t, dir, branchFeature)

	if err := MergeBranch(dir, branchMain); err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}

	testkit.AssertEqual(t, labelHead, revParse(t, dir, headRev), mainTip)
	testkit.AssertEqual(t, "parents", parentCount(t, dir), 1)
}

func TestMergeBranch_CreatesMergeCommit(t *testing.T) {
	t.Parallel()
	dir := divergedRepo(t, "feature work\n", "main work\n")

	if err := MergeBranch(dir, branchMain); err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}

	testkit.AssertEqual(t, "parents", parentCount(t, dir), 2)
	assertExists(t, filepath.Join(dir, fileApp))
	assertExists(t, filepath.Join(dir, fileFeature))
}

func TestMergeBranch_ConflictAbortsAndWraps(t *testing.T) {
	t.Parallel()
	dir := conflictingRepo(t)
	before := revParse(t, dir, headRev)

	err := MergeBranch(dir, branchMain)
	if err == nil {
		t.Fatal("expected a conflict error, got nil")
	}
	if !errors.Is(err, ErrConflict) {
		t.Errorf("errors.Is(err, ErrConflict) = false, err = %v", err)
	}

	clean, cleanErr := IsClean(dir)
	if cleanErr != nil {
		t.Fatalf("IsClean: %v", cleanErr)
	}
	testkit.AssertEqual(t, labelIsClean, clean, true)
	testkit.AssertEqual(t, labelHead, revParse(t, dir, headRev), before)
	assertMissing(t, filepath.Join(dir, ".git", "MERGE_HEAD"))
}

func TestMergeBranch_UnknownRefIsNotAConflict(t *testing.T) {
	t.Parallel()
	dir := initRepo(t)

	err := MergeBranch(dir, "ghost")
	if err == nil {
		t.Fatal("expected an error merging an unknown ref, got nil")
	}
	if errors.Is(err, ErrConflict) {
		t.Errorf("a missing ref was reported as a conflict: %v", err)
	}
}

// TestMergeBranch_RefStartingWithDash pins the "--" terminator on merge: the
// ref is API-supplied too, and a dash-leading one must merge, not be parsed.
func TestMergeBranch_RefStartingWithDash(t *testing.T) {
	t.Parallel()
	dir := initRepo(t)
	createBranch(t, dir, branchFeature)
	commitFile(t, dir, fileApp, "main work\n", "main work")
	gitRun(t, dir, "update-ref", "refs/heads/"+branchDash, "refs/heads/"+branchMain)
	tip := revParse(t, dir, "refs/heads/"+branchDash)
	checkout(t, dir, branchFeature)

	if err := MergeBranch(dir, branchDash); err != nil {
		t.Fatalf("MergeBranch: %v", err)
	}

	testkit.AssertEqual(t, labelHead, revParse(t, dir, headRev), tip)
}

func TestRebaseOnto_ReplaysCommitsOnTop(t *testing.T) {
	t.Parallel()
	dir := divergedRepo(t, "feature work\n", "main work\n")
	mainTip := revParse(t, dir, branchMain)

	if err := RebaseOnto(dir, branchMain); err != nil {
		t.Fatalf("RebaseOnto: %v", err)
	}

	testkit.AssertEqual(t, "HEAD^", revParse(t, dir, "HEAD^"), mainTip)
	testkit.AssertEqual(t, "commit count", gitOut(t, dir, "rev-list", "--count", headRev), "3")
	testkit.AssertEqual(t, "parents", parentCount(t, dir), 1)
}

func TestRebaseOnto_ConflictAbortsAndWraps(t *testing.T) {
	t.Parallel()
	dir := conflictingRepo(t)
	before := revParse(t, dir, headRev)

	err := RebaseOnto(dir, branchMain)
	if err == nil {
		t.Fatal("expected a conflict error, got nil")
	}
	if !errors.Is(err, ErrConflict) {
		t.Errorf("errors.Is(err, ErrConflict) = false, err = %v", err)
	}

	clean, cleanErr := IsClean(dir)
	if cleanErr != nil {
		t.Fatalf("IsClean: %v", cleanErr)
	}
	testkit.AssertEqual(t, labelIsClean, clean, true)
	testkit.AssertEqual(t, labelHead, revParse(t, dir, headRev), before)

	branch, branchErr := CurrentBranch(dir)
	if branchErr != nil {
		t.Fatalf("CurrentBranch: %v", branchErr)
	}
	testkit.AssertEqual(t, "current branch", branch, branchFeature)
	assertMissing(t, filepath.Join(dir, ".git", "rebase-merge"))
	assertMissing(t, filepath.Join(dir, ".git", "rebase-apply"))
}

func TestPush_MovesRemoteRef(t *testing.T) {
	t.Parallel()
	remote, work := initRemoteAndClone(t)
	commitFile(t, work, fileApp, "one\n", "add app")

	if err := Push(work, remoteOrigin, false); err != nil {
		t.Fatalf("Push: %v", err)
	}

	testkit.AssertEqual(t, "remote main", revParse(t, remote, branchMain), revParse(t, work, headRev))
}

func TestPush_RejectsRewrittenHistoryWithoutForce(t *testing.T) {
	t.Parallel()
	remote, work := rebasedFeature(t)
	published := revParse(t, remote, branchFeature)

	if err := Push(work, remoteOrigin, false); err == nil {
		t.Error("expected a non-fast-forward rejection, got nil")
	}

	testkit.AssertEqual(t, "remote feature", revParse(t, remote, branchFeature), published)
}

func TestPush_ForceWithLeaseAfterRebase(t *testing.T) {
	t.Parallel()
	remote, work := rebasedFeature(t)

	if err := Push(work, remoteOrigin, true); err != nil {
		t.Fatalf("Push: %v", err)
	}

	testkit.AssertEqual(t, "remote feature", revParse(t, remote, branchFeature), revParse(t, work, headRev))
}

func TestPush_UnknownRemoteReturnsError(t *testing.T) {
	t.Parallel()
	_, work := initRemoteAndClone(t)

	if err := Push(work, "ghost", false); err == nil {
		t.Error("expected an error pushing to an unknown remote, got nil")
	}
}

// divergedRepo returns a repository whose feature branch (checked out) and
// main branch each carry one commit touching a different file.
func divergedRepo(t *testing.T, featureContent, mainContent string) string {
	t.Helper()
	dir := initRepo(t)

	checkoutNew(t, dir, branchFeature)
	commitFile(t, dir, fileFeature, featureContent, "feature work")
	checkout(t, dir, branchMain)
	commitFile(t, dir, fileApp, mainContent, "main work")
	checkout(t, dir, branchFeature)
	return dir
}

// conflictingRepo returns a repository whose feature branch (checked out) and
// main branch changed the same line of the same file.
func conflictingRepo(t *testing.T) string {
	t.Helper()
	dir := initRepo(t)
	commitFile(t, dir, fileApp, "base\n", "add app")

	checkoutNew(t, dir, branchFeature)
	commitFile(t, dir, fileApp, "feature\n", "feature edit")
	checkout(t, dir, branchMain)
	commitFile(t, dir, fileApp, "main\n", "main edit")
	checkout(t, dir, branchFeature)
	return dir
}

// rebasedFeature builds the state a sync-with-target leaves behind: a feature
// branch that exists on the remote and has since been replayed on top of a
// moved main, so its history no longer fast-forwards.
func rebasedFeature(t *testing.T) (remote, work string) {
	t.Helper()
	remote, work = initRemoteAndClone(t)

	checkoutNew(t, work, branchFeature)
	commitFile(t, work, fileFeature, "feature work\n", "feature work")
	gitRun(t, work, "push", "-q", remoteOrigin, branchFeature)

	checkout(t, work, branchMain)
	commitFile(t, work, fileApp, "main work\n", "main work")
	gitRun(t, work, "push", "-q", remoteOrigin, branchMain)

	checkout(t, work, branchFeature)
	if err := RebaseOnto(work, branchMain); err != nil {
		t.Fatalf("RebaseOnto: %v", err)
	}
	return remote, work
}

// parentCount reports how many parents HEAD has: 1 after a fast-forward or a
// rebase, 2 after a real merge.
func parentCount(t *testing.T, dir string) int {
	t.Helper()
	fields := strings.Fields(gitOut(t, dir, "rev-list", "--parents", "-n", "1", headRev))
	return len(fields) - 1
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected %s to exist: %v", path, err)
	}
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected %s to be gone, stat error = %v", path, err)
	}
}

func TestFetch_MissingRemoteBranchIsErrNoRemoteBranch(t *testing.T) {
	t.Parallel()
	_, work := initRemoteAndClone(t)

	err := Fetch(work, "origin", "never-pushed")

	if !errors.Is(err, ErrNoRemoteBranch) {
		t.Fatalf("err = %v, want ErrNoRemoteBranch", err)
	}
}

func TestFastForward(t *testing.T) {
	t.Parallel()
	remote, work := initRemoteAndClone(t)

	// Advance the remote's main from another clone.
	other := cloneWork(t, remote)
	commitFile(t, other, "ahead.txt", "ahead", "ahead")
	gitRun(t, other, "push", "-q", "origin", "main")
	gitRun(t, work, "fetch", "-q", "origin")

	if err := FastForward(work, "origin/main"); err != nil {
		t.Fatalf("FastForward: %v", err)
	}
	testkit.AssertEqual(t, "caught up", revParse(t, work, "HEAD"), revParse(t, work, "origin/main"))

	// Diverge: a local commit plus another remote commit — ff-only must refuse.
	commitFile(t, work, "mine.txt", "mine", "mine")
	commitFile(t, other, "theirs.txt", "theirs", "theirs")
	gitRun(t, other, "push", "-q", "origin", "main")
	gitRun(t, work, "fetch", "-q", "origin")
	before := revParse(t, work, "HEAD")

	if err := FastForward(work, "origin/main"); err == nil {
		t.Fatal("a diverged branch must not be fast-forwarded")
	}
	testkit.AssertEqual(t, "HEAD untouched", revParse(t, work, "HEAD"), before)
}

func TestRemoteRepositories(t *testing.T) {
	t.Parallel()
	dir := initRepo(t)
	for name, url := range map[string]string{
		"origin":   "https://bitbucket.org/acme/api-gateway.git",
		"upstream": "git@bitbucket.org:Acme/Web-App.git",
		"scheme":   "ssh://git@bitbucket.org/acme/infra",
		"twin":     "https://someone@bitbucket.org/acme/api-gateway/",
		"odd":      "file:///only-one-segment",
	} {
		gitRun(t, dir, "remote", "add", name, url)
	}

	got := RemoteRepositories(dir)

	slices.Sort(got)
	testkit.AssertSliceEqual(t, "repositories", got, []string{"acme/api-gateway", "acme/infra", "acme/web-app"})
	if RemoteRepositories(t.TempDir()) != nil {
		t.Error("a directory that is not a repository has no remote repositories")
	}
}

func TestRepoPathOf(t *testing.T) {
	t.Parallel()
	for url, want := range map[string]string{
		"https://bitbucket.org/acme/api.git": "acme/api",
		"git@bitbucket.org:acme/api.git":     "acme/api",
		"ssh://git@bitbucket.org/acme/api":   "acme/api",
		"https://x@bitbucket.org/Acme/API/":  "acme/api",
		"/srv/git/acme/api.git":              "acme/api",
		"https://bitbucket.org/api.git":      "",
		"file:///repo":                       "",
	} {
		got, ok := repoPathOf(url)
		testkit.AssertEqual(t, url, got, want)
		testkit.AssertEqual(t, url+" ok", ok, want != "")
	}
}
