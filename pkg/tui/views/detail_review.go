package views

import (
	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
)

// This file holds the review tabs' data plumbing: the setters the app feeds
// them, the getters it reads back, and the cursor accessors. The rendering of
// each list lives beside it in detail_files.go, detail_commits.go,
// detail_threads.go and detail_filediff.go.
//
// Every setter is guarded on the pull request key. The app fires the four
// review fetches independently and a user can walk off a pull request while
// they are in flight, so a response that names another pull request is dropped
// rather than shown against the wrong one.

// SetDiffStat feeds the Files tab. A response for another pull request is
// dropped.
func (d *DetailView) SetDiffStat(prKey string, entries []bitbucket.DiffStatEntry) {
	if !d.keyMatches(prKey) {
		return
	}
	d.files.set(entries)
	d.invalidateBlocksFor(listFiles)
}

// SetDiffStatLoading marks the Files tab's fetch in flight.
func (d *DetailView) SetDiffStatLoading(prKey string) {
	if !d.keyMatches(prKey) {
		return
	}
	d.files.setLoading()
	d.invalidateBlocksFor(listFiles)
}

// SetDiffStatError shows why the changed-file list could not be fetched.
func (d *DetailView) SetDiffStatError(prKey, msg string) {
	if !d.keyMatches(prKey) {
		return
	}
	d.files.setError(msg)
	d.invalidateBlocksFor(listFiles)
}

// SetCommits feeds the Commits tab. A response for another pull request is
// dropped.
func (d *DetailView) SetCommits(prKey string, commits []bitbucket.Commit) {
	if !d.keyMatches(prKey) {
		return
	}
	d.commits.set(commits)
	d.invalidateBlocksFor(listCommits)
}

// SetCommitsLoading marks the Commits tab's fetch in flight.
func (d *DetailView) SetCommitsLoading(prKey string) {
	if !d.keyMatches(prKey) {
		return
	}
	d.commits.setLoading()
	d.invalidateBlocksFor(listCommits)
}

// SetCommitsError shows why the commit list could not be fetched.
func (d *DetailView) SetCommitsError(prKey, msg string) {
	if !d.keyMatches(prKey) {
		return
	}
	d.commits.setError(msg)
	d.invalidateBlocksFor(listCommits)
}

// SetThreads feeds the Comments tab. A response for another pull request is
// dropped.
func (d *DetailView) SetThreads(prKey string, threads []bitbucket.Thread) {
	if !d.keyMatches(prKey) {
		return
	}
	d.threads.set(threads)
	d.invalidateBlocksFor(listThreads)
}

// SetThreadsLoading marks the Comments tab's fetch in flight.
func (d *DetailView) SetThreadsLoading(prKey string) {
	if !d.keyMatches(prKey) {
		return
	}
	d.threads.setLoading()
	d.invalidateBlocksFor(listThreads)
}

// SetThreadsError shows why the review threads could not be fetched.
func (d *DetailView) SetThreadsError(prKey, msg string) {
	if !d.keyMatches(prKey) {
		return
	}
	d.threads.setError(msg)
	d.invalidateBlocksFor(listThreads)
}

// keyMatches reports whether a response naming prKey is still for the pull
// request on screen.
func (d *DetailView) keyMatches(prKey string) bool {
	return d.pr != nil && d.pr.Key() == prKey
}

// DiffStat returns the changed files of the pull request in view.
func (d *DetailView) DiffStat() []bitbucket.DiffStatEntry { return d.files.items }

// Commits returns the commits of the pull request in view.
func (d *DetailView) Commits() []bitbucket.Commit { return d.commits.items }

// Threads returns the review threads of the pull request in view.
func (d *DetailView) Threads() []bitbucket.Thread { return d.threads.items }

// SelectedFile returns the changed file under the cursor, nil outside the Files
// tab and while the tab is showing a placeholder instead of a list. The
// placeholder check is what keeps Enter, the editor key and the copy keys off
// a row the reader cannot see: the placeholder is the only block then, so the
// cursor sits on it rather than on any file.
func (d *DetailView) SelectedFile() *bitbucket.DiffStatEntry {
	if _, ok := d.filesPlaceholder(); ok {
		return nil
	}
	if d.listKind() != listFiles || d.listCursor >= len(d.files.items) || d.listCursor < 0 {
		return nil
	}
	return &d.files.items[d.listCursor]
}

// SelectedCommit returns the commit under the cursor, nil outside the Commits
// tab and while the tab is showing a placeholder.
func (d *DetailView) SelectedCommit() *bitbucket.Commit {
	if _, ok := d.commitsPlaceholder(); ok {
		return nil
	}
	if d.listKind() != listCommits || d.listCursor >= len(d.commits.items) || d.listCursor < 0 {
		return nil
	}
	return &d.commits.items[d.listCursor]
}

// SelectedThread returns the review thread under the cursor, nil outside the
// Comments tab and while the tab is showing a placeholder.
func (d *DetailView) SelectedThread() *bitbucket.Thread {
	if _, ok := d.threadsPlaceholder(); ok {
		return nil
	}
	// The cursor indexes what is on the list, which is not every thread when
	// the resolved ones are hidden.
	listed := d.listedThreads()
	if d.listKind() != listThreads || d.listCursor >= len(listed) || d.listCursor < 0 {
		return nil
	}
	return &listed[d.listCursor]
}

// FocusThread puts the Comments tab on screen with the cursor on the thread
// rooted at rootID, which is how Enter on a commented diff line lands on the
// conversation. It reports false when no such thread is loaded, so the caller
// can say the comments are still on their way.
func (d *DetailView) FocusThread(rootID int64) bool {
	if d.threadIndex(rootID) < 0 {
		return false
	}
	d.clearFileDiff()
	d.clearThread()
	d.mode = ModePullRequest
	d.SetActiveTab(TabComments)
	if d.listedThreadIndex(rootID) < 0 {
		// The thread is resolved and the resolved ones are hidden. Being sent
		// to a conversation and shown a list without it in it would be worse
		// than the filter turning itself off, which the tab label then says.
		d.hideResolved = false
		d.invalidateBlocksFor(listThreads)
	}
	d.listCursor = max(d.listedThreadIndex(rootID), 0)
	return true
}

// NextFile moves to the following changed file, PrevFile to the preceding one,
// and both report the path they landed on. On the Files tab they move the
// cursor; in ModeFileDiff they only name the neighbour, leaving it to the
// caller to open it. At either end of the list, and anywhere else, they report
// false.
func (d *DetailView) NextFile() (string, bool) { return d.stepFile(1) }

// PrevFile is NextFile in the other direction.
func (d *DetailView) PrevFile() (string, bool) { return d.stepFile(-1) }

func (d *DetailView) stepFile(delta int) (string, bool) {
	if len(d.files.items) == 0 {
		return "", false
	}

	current := -1
	switch {
	case d.mode == ModeFileDiff:
		current = d.fileIndex(d.fileDiff.path)
	case d.listKind() == listFiles:
		current = clampIndex(d.listCursor, len(d.files.items))
	}
	if current < 0 {
		return "", false
	}

	next := current + delta
	if next < 0 || next >= len(d.files.items) {
		return "", false
	}
	if d.mode != ModeFileDiff {
		d.listCursor = next
	}
	return d.files.items[next].Path(), true
}

// fileIndex is the position of path in the changed-file list, -1 when the list
// does not have it (a diff opened before the diffstat arrived, say).
func (d *DetailView) fileIndex(path string) int {
	for i := range d.files.items {
		if d.files.items[i].Path() == path {
			return i
		}
	}
	return -1
}
