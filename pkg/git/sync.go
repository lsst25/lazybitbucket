package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"
)

// ErrConflict is wrapped by MergeBranch and RebaseOnto when the operation
// stopped on conflicting changes. Both abort before returning, so a caller
// seeing ErrConflict knows the working tree was left as it was found.
var ErrConflict = errors.New("merge conflict")

// ErrNoRemoteBranch is wrapped by Fetch when the remote has no branch of that
// name — a branch that was never pushed. Callers that only wanted the
// remote-tracking ref refreshed can treat it as "nothing to compare against".
var ErrNoRemoteBranch = errors.New("no such branch on the remote")

// IsClean reports whether the working tree has no staged, unstaged or
// untracked changes.
func IsClean(dir string) (bool, error) {
	cmd := gitCommand(dir, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("git status --porcelain: %w", err)
	}
	return strings.TrimSpace(string(out)) == "", nil
}

// RemoteFor returns the name of the remote whose fetch URL points at
// workspace/slug, matching both the ssh (git@bitbucket.org:acme/api.git) and
// https (https://bitbucket.org/acme/api.git) forms case-insensitively, with or
// without the .git suffix. When several remotes match, one whose host names
// Bitbucket wins over a mirror elsewhere. ok is false when no remote matches,
// when the remotes cannot be listed, or when dir is not a repository: there is
// deliberately no fallback to "origin", because a sync fetches from and pushes
// to this remote, and "origin" in an unrelated checkout would be the wrong
// repository.
func RemoteFor(dir, workspace, slug string) (name string, ok bool) {
	cmd := gitCommand(dir, "remote", "-v")
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}

	repo := strings.ToLower(workspace + "/" + slug)
	first := ""
	for _, line := range parseLines(out) {
		name, url, ok := fetchRemote(line)
		if !ok || !matchesRepo(url, repo) {
			continue
		}
		if strings.Contains(strings.ToLower(url), "bitbucket") {
			return name, true
		}
		if first == "" {
			first = name
		}
	}
	return first, first != ""
}

// IsAncestor reports whether ancestor is reachable from descendant (both are
// revisions git rev-parse understands). A sync uses it to refuse a rebase and
// force-push when the remote branch holds commits this checkout has never seen.
func IsAncestor(dir, ancestor, descendant string) (bool, error) {
	cmd := gitCommand(dir, "merge-base", "--is-ancestor", "--", ancestor, descendant)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("git merge-base --is-ancestor: %s", strings.TrimSpace(string(out)))
}

// RemoteRepositories lists the "owner/name" paths the checkout's remotes fetch
// from, lower-cased, in git remote -v order and without duplicates. Both URL
// forms are understood (https://host/owner/name.git, git@host:owner/name.git),
// and a URL whose path has fewer than two segments is skipped. It is what lets
// the app recognise which Bitbucket repository a checkout belongs to; an empty
// result means the directory is not a repository or has no usable remote.
func RemoteRepositories(dir string) []string {
	out, err := gitCommand(dir, "remote", "-v").Output()
	if err != nil {
		return nil
	}
	var repos []string
	for _, line := range parseLines(out) {
		_, url, ok := fetchRemote(line)
		if !ok {
			continue
		}
		repo, ok := repoPathOf(url)
		if ok && !slices.Contains(repos, repo) {
			repos = append(repos, repo)
		}
	}
	return repos
}

// repoPathOf reduces a remote URL to its lower-cased "owner/name": the last two
// segments of the URL's path, after the scheme and host of an https:// or ssh://
// URL or after the "host:" of the scp-like form. A path with fewer than two
// segments is not a repository path.
func repoPathOf(url string) (string, bool) {
	path := strings.ToLower(url)
	if i := strings.Index(path, "://"); i >= 0 {
		_, after, ok := strings.Cut(path[i+3:], "/")
		if !ok {
			return "", false
		}
		path = after
	} else if hostPart, rest, ok := strings.Cut(path, ":"); ok && !strings.Contains(hostPart, "/") {
		path = rest // user@host:owner/name
	}
	path = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(path, "/"), ".git"), "/")
	segments := strings.Split(strings.Trim(path, "/"), "/")
	if len(segments) < 2 {
		return "", false
	}
	owner, name := segments[len(segments)-2], segments[len(segments)-1]
	if owner == "" || name == "" {
		return "", false
	}
	return owner + "/" + name, true
}

// fetchRemote splits one "name<TAB>url (fetch)" line of git remote -v output.
// Push lines and anything unparseable are reported as not ok.
func fetchRemote(line string) (name, url string, ok bool) {
	name, rest, ok := strings.Cut(line, "\t")
	if !ok {
		return "", "", false
	}
	url, kind, ok := strings.Cut(rest, " ")
	if !ok || kind != "(fetch)" {
		return "", "", false
	}
	return name, url, true
}

// matchesRepo reports whether a remote URL addresses the given lower-cased
// "workspace/slug". The trailing .git and any trailing slash are dropped, so
// the comparison anchors on the path separator and cannot match a repository
// whose name merely starts with slug.
func matchesRepo(url, repo string) bool {
	trimmed := strings.ToLower(strings.TrimSuffix(url, "/"))
	trimmed = strings.TrimSuffix(strings.TrimSuffix(trimmed, ".git"), "/")
	return trimmed == repo ||
		strings.HasSuffix(trimmed, "/"+repo) ||
		strings.HasSuffix(trimmed, ":"+repo)
}

// Fetch updates the remote-tracking state for a single branch. The remote and
// the refspec are passed after "--" because the branch name comes from the
// Bitbucket API: git parses options anywhere on the line, so an argument such
// as "--upload-pack=..." would otherwise be honoured as a flag. See
// ErrUnsafeName.
func Fetch(dir, remote, branch string) error {
	err := runRemote(dir, "fetch", "--", remote, branch)
	if err != nil && strings.Contains(err.Error(), "couldn't find remote ref") {
		return fmt.Errorf("%w: %s", ErrNoRemoteBranch, branch)
	}
	return err
}

// MergeBranch merges ref into the current branch without opening an editor.
// A merge that stops on conflicts is aborted and reported as ErrConflict.
// ref is passed after "--" for the reason given on ErrUnsafeName.
func MergeBranch(dir, ref string) error {
	cmd := gitCommand(dir, "merge", "--no-edit", "--", ref)
	out, err := cmd.CombinedOutput()
	if err != nil {
		abort(dir, "merge")
		return operationError(out)
	}
	return nil
}

// RebaseOnto replays the current branch on top of ref. A rebase that stops on
// conflicts is aborted and reported as ErrConflict. ref is passed after "--"
// for the reason given on ErrUnsafeName.
func RebaseOnto(dir, ref string) error {
	cmd := gitCommand(dir, "rebase", "--", ref)
	out, err := cmd.CombinedOutput()
	if err != nil {
		abort(dir, "rebase")
		return operationError(out)
	}
	return nil
}

// FastForward moves the current branch to ref when ref is a descendant of it:
// git merge --ff-only, which never creates a merge commit and never touches a
// branch that has diverged — that case is an error carrying git's own words.
// ref is passed after "--" for the reason given on ErrUnsafeName.
func FastForward(dir, ref string) error {
	cmd := gitCommand(dir, "merge", "--ff-only", "--", ref)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}

// Push pushes the current branch to remote under its own name. After a rebase
// the branch has been rewritten, so the caller asks for --force-with-lease,
// which still refuses to overwrite commits the checkout has not seen.
func Push(dir, remote string, forceWithLease bool) error {
	args := []string{"push"}
	if forceWithLease {
		args = append(args, "--force-with-lease")
	}
	args = append(args, "--", remote, "HEAD")
	return runRemote(dir, args...)
}

// remoteTimeout bounds a fetch or push. The TUI runs them off the main loop
// but cannot interrupt them, so a remote that never answers (a hung ssh agent,
// a proxy swallowing the connection) must fail on its own.
const remoteTimeout = 2 * time.Minute

// gitCommand builds a git command for dir with a C locale, so the output the
// callers inspect (conflict markers, error lines) is not localized.
func gitCommand(dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(context.Background(), "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	return cmd
}

// runRemote runs a git command that talks to a remote. Terminal prompts are
// disabled because the TUI owns the terminal: a credential prompt would be
// invisible and would hang the sync forever, whereas with prompts off git
// fails immediately with a message the caller can show. The command is also
// bounded by remoteTimeout.
func runRemote(dir string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), remoteTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C", "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("git %s timed out after %s", args[0], remoteTimeout)
		}
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}

// abort undoes a merge or rebase that stopped part-way. Its own failure is
// deliberately ignored: the caller is about to report why the operation
// failed, and "there is no merge to abort" would only mask that.
func abort(dir, operation string) {
	_ = gitCommand(dir, operation, "--abort").Run()
}

// operationError turns the output of a failed merge or rebase into an error,
// wrapping ErrConflict when git reported conflicting changes.
func operationError(out []byte) error {
	text := strings.TrimSpace(string(out))
	if strings.Contains(strings.ToLower(text), "conflict") {
		return fmt.Errorf("%w: %s", ErrConflict, text)
	}
	return fmt.Errorf("%s", text)
}
