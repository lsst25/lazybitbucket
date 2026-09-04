package git

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// GitAvailable returns true if git is found in PATH
func GitAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// IsRepo returns true if dir is inside a git repository
func IsRepo(dir string) bool {
	cmd := exec.CommandContext(context.Background(), "git", "-C", dir, "rev-parse", "--git-dir")
	return cmd.Run() == nil
}

// CurrentBranch returns the current branch name, or an empty string when in detached HEAD state
func CurrentBranch(dir string) (string, error) {
	cmd := exec.CommandContext(context.Background(), "git", "-C", dir, "symbolic-ref", "--short", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// TopLevel returns the absolute path of the working tree dir sits in. It is
// what a path taken from a pull request's diff has to be resolved against:
// diff paths are repository-relative, and the tool may well have been started
// from a subdirectory of the checkout.
func TopLevel(dir string) (string, error) {
	cmd := exec.CommandContext(context.Background(), "git", "-C", dir, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --show-toplevel: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
