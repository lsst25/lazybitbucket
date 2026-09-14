package demo

import (
	"strconv"
	"strings"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/diff"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// These tests are the dataset's own consistency proof. The diff text, the
// diffstat and the comment anchors are three views of the same change, and a
// demo world where they disagree teaches the UI the wrong lesson — a file list
// whose counts do not match its diff, or a marker on a line the diff has not
// got.

func TestDataset_ReviewShape(t *testing.T) {
	t.Parallel()

	data := NewDataset()

	for _, want := range []struct {
		slug        string
		pullRequest int
		commits     int
		comments    int
		files       int
	}{
		{slug: repoAPIGateway, pullRequest: 103, commits: 3, comments: 9, files: 3},
		{slug: repoAPIGateway, pullRequest: 102, commits: 2, comments: 0, files: 2},
		{slug: repoAPIGateway, pullRequest: 101, commits: 3, comments: 0, files: 3},
		{slug: repoWebApp, pullRequest: 41, commits: 2, comments: 0, files: 3},
		{slug: repoWebApp, pullRequest: 40, commits: 4, comments: 1, files: 3},
		{slug: repoInfra, pullRequest: 7, commits: 2, comments: 0, files: 3},
	} {
		key := reviewKey(want.slug, want.pullRequest)
		testkit.AssertEqual(t, key+" commits", len(data.Commits[key]), want.commits)
		testkit.AssertEqual(t, key+" comments", len(data.Comments[key]), want.comments)
		testkit.AssertEqual(t, key+" diffstat entries", len(data.DiffStats[key]), want.files)
		if data.Diffs[key] == "" {
			t.Errorf("%s has no diff text", key)
		}
	}

	// The merged pull request is the too-large case, and closed pull requests
	// carry no commits: their source branch is gone.
	tooLarge := reviewKey(repoAPIGateway, 98)
	testkit.AssertEqual(t, "TooLargeDiffs[#98]", data.TooLargeDiffs[tooLarge], true)
	testkit.AssertEqual(t, "Diffs[#98]", data.Diffs[tooLarge], "")
	testkit.AssertEqual(t, "DiffStats[#98]", len(data.DiffStats[tooLarge]), 0)
	testkit.AssertEqual(t, "Commits[#98]", len(data.Commits[tooLarge]), 0)
}

// TestDataset_CommitsChainToTheirPullRequest keeps the commit list and the pull
// request payload telling the same story: the newest commit is the head the
// payload names, and the oldest one's parent is the destination tip.
func TestDataset_CommitsChainToTheirPullRequest(t *testing.T) {
	t.Parallel()

	data := NewDataset()
	for slug, pullRequests := range data.PullRequests {
		for _, pullRequest := range pullRequests {
			key := reviewKey(slug, pullRequest.ID)
			commits := data.Commits[key]
			if len(commits) == 0 {
				continue
			}

			testkit.AssertEqual(t, key+" head commit",
				shortHash(commits[0].Hash), pullRequest.Source.Commit.Hash)
			testkit.AssertEqual(t, key+" head hash length", len(commits[0].Hash), 40)

			for index := 1; index < len(commits); index++ {
				testkit.AssertEqual(t, key+" parent of commit "+strconv.Itoa(index-1),
					commits[index-1].Parents[0].Hash, commits[index].Hash)
				if !commits[index-1].Date.After(commits[index].Date) {
					t.Errorf("%s: commit %s is not newer than its parent", key, commits[index-1].Hash)
				}
			}

			oldest := commits[len(commits)-1]
			testkit.AssertEqual(t, key+" root parent",
				shortHash(oldest.Parents[0].Hash), pullRequest.Destination.Commit.Hash)
		}
	}
}

// TestDataset_OneCommitHasNoBitbucketAccount covers Commit.AuthorName()'s
// fallback: a bot's git ident matches no account, so the name comes out of the
// raw ident.
func TestDataset_OneCommitHasNoBitbucketAccount(t *testing.T) {
	t.Parallel()

	data := NewDataset()

	var unlinked []bitbucket.Commit
	for _, commits := range data.Commits {
		for _, commit := range commits {
			if commit.Author.User == nil {
				unlinked = append(unlinked, commit)
			}
		}
	}

	testkit.AssertEqual(t, "commits with no account", len(unlinked), 1)
	testkit.AssertEqual(t, "AuthorName()", unlinked[0].AuthorName(), "dependabot[bot]")
	if !strings.Contains(unlinked[0].Author.Raw, "<") {
		t.Fatalf("Author.Raw = %q, want a git ident", unlinked[0].Author.Raw)
	}
}

// TestDataset_DiffStatAgreesWithTheDiff is the important one: every diffstat
// entry is re-derived here from the diff text with pkg/diff, so a hand-edited
// fixture cannot leave the file list claiming counts the diff does not have.
func TestDataset_DiffStatAgreesWithTheDiff(t *testing.T) {
	t.Parallel()

	data := NewDataset()
	for key, text := range data.Diffs {
		files, err := diff.Parse(text)
		if err != nil {
			t.Fatalf("%s: parse the diff: %v", key, err)
		}

		entries := data.DiffStats[key]
		testkit.AssertEqual(t, key+" entry count", len(entries), len(files))
		if len(entries) != len(files) {
			continue
		}

		for index, file := range files {
			entry := entries[index]
			label := key + " " + file.Path()
			testkit.AssertEqual(t, label+" path", entry.Path(), file.Path())
			testkit.AssertEqual(t, label+" lines_added", entry.LinesAdded, file.Added())
			testkit.AssertEqual(t, label+" lines_removed", entry.LinesRemoved, file.Removed())
			testkit.AssertEqual(t, label+" status", entry.Status, diffStatStatus(file.Status))
			testkit.AssertEqual(t, label+" is binary", entry.IsBinary(), file.IsBinary)
			testkit.AssertEqual(t, label+" old path", entry.OldPath(), file.OldPath)
			testkit.AssertEqual(t, label+" escaped path", entry.EscapedPath(), file.Path())
		}
	}
}

// TestDataset_DiffHunkHeadersCountTheirLines catches the mistake hand-written
// fixtures actually make: a hunk header whose counts disagree with the lines
// below it, which makes the parser swallow the next file's header.
func TestDataset_DiffHunkHeadersCountTheirLines(t *testing.T) {
	t.Parallel()

	data := NewDataset()
	for key, text := range data.Diffs {
		files, err := diff.Parse(text)
		if err != nil {
			t.Fatalf("%s: parse the diff: %v", key, err)
		}

		for _, file := range files {
			for index, hunk := range file.Hunks {
				var oldLines, newLines int
				for _, line := range hunk.Lines {
					switch line.Kind {
					case diff.LineContext:
						oldLines++
						newLines++
					case diff.LineAdded:
						newLines++
					case diff.LineRemoved:
						oldLines++
					case diff.LineNoNewline:
					default:
					}
				}

				label := key + " " + file.Path() + " hunk " + strconv.Itoa(index)
				testkit.AssertEqual(t, label+" old lines", oldLines, hunk.OldLines)
				testkit.AssertEqual(t, label+" new lines", newLines, hunk.NewLines)
			}
		}
	}
}

// TestDataset_DiffsCoverEveryFileShape states what the fixtures are for: the
// diff pane has a rename, an added, a removed and a binary file to render, and
// a hunk that ends without a trailing newline.
func TestDataset_DiffsCoverEveryFileShape(t *testing.T) {
	t.Parallel()

	data := NewDataset()

	shapes := map[diff.FileStatus]int{}
	binary, noNewline := 0, 0
	for key, text := range data.Diffs {
		files, err := diff.Parse(text)
		if err != nil {
			t.Fatalf("%s: parse the diff: %v", key, err)
		}
		for _, file := range files {
			shapes[file.Status]++
			if file.IsBinary {
				binary++
			}
			for _, hunk := range file.Hunks {
				for _, line := range hunk.Lines {
					if line.Kind == diff.LineNoNewline {
						noNewline++
					}
				}
			}
		}
	}

	for status, label := range map[diff.FileStatus]string{
		diff.Modified: "modified",
		diff.Added:    "added",
		diff.Deleted:  "deleted",
		diff.Renamed:  "renamed",
	} {
		if shapes[status] == 0 {
			t.Errorf("no %s file anywhere in the demo diffs", label)
		}
	}
	if binary == 0 {
		t.Error("no binary file in the demo diffs")
	}
	if noNewline == 0 {
		t.Error(`no "\ No newline at end of file" in the demo diffs`)
	}
}

// TestDataset_CommentAnchorsExistInDiffs is what makes the diff markers real: an
// inline comment names a file and a line, and both have to exist in that pull
// request's diff or the marker would sit on nothing.
func TestDataset_CommentAnchorsExistInDiffs(t *testing.T) {
	t.Parallel()

	data := NewDataset()
	for key, comments := range data.Comments {
		files, err := diff.Parse(data.Diffs[key])
		if err != nil {
			t.Fatalf("%s: parse the diff: %v", key, err)
		}

		for _, comment := range comments {
			if !comment.IsInline() {
				continue
			}

			label := key + " comment " + strconv.FormatInt(comment.ID, 10)
			file, ok := fileByPath(files, comment.Inline.Path)
			if !ok {
				t.Errorf("%s is anchored to %s, which the diff does not change",
					label, comment.Inline.Path)
				continue
			}

			assertAnchorLine(t, label+" from", file, comment.Inline.From, false)
			assertAnchorLine(t, label+" to", file, comment.Inline.To, true)
			assertAnchorLine(t, label+" start_from", file, comment.Inline.StartFrom, false)
			assertAnchorLine(t, label+" start_to", file, comment.Inline.StartTo, true)
		}
	}
}

// TestDataset_CommentsCoverEveryThreadShape checks the conversation through
// BuildThreads, the way the view will see it: inline threads first, a resolved
// one, a deleted root kept for its reply, a pending draft, and a general thread
// whose nested reply has been flattened.
func TestDataset_CommentsCoverEveryThreadShape(t *testing.T) {
	t.Parallel()

	data := NewDataset()
	threads := bitbucket.BuildThreads(data.Comments[reviewKey(repoAPIGateway, 103)])

	testkit.AssertEqual(t, "len(threads)", len(threads), 5)

	// Inline threads lead, ordered by path then line: retry.go:13, retry.go:16,
	// retry_test.go:11, then the two general ones by age.
	inline := 0
	for _, thread := range threads {
		if thread.IsInline() {
			inline++
		}
	}
	testkit.AssertEqual(t, "inline threads", inline, 3)
	testkit.AssertEqual(t, "threads[0].Path()", threads[0].Path(), pathRetryGo)
	testkit.AssertEqual(t, "threads[0].Resolved()", threads[0].Resolved(), true)
	testkit.AssertEqual(t, "threads[1].Path()", threads[1].Path(), pathRetryGo)
	testkit.AssertEqual(t, "threads[1].Resolved()", threads[1].Resolved(), false)
	testkit.AssertEqual(t, "threads[1] replies", len(threads[1].Replies), 1)
	testkit.AssertEqual(t, "threads[2].Path()", threads[2].Path(), pathRetryTestGo)
	testkit.AssertEqual(t, "threads[2].Root.Pending", threads[2].Root.Pending, true)

	general := threads[3:]
	testkit.AssertEqual(t, "general threads", len(general), 2)
	testkit.AssertEqual(t, "deleted root", general[0].Root.Deleted, true)
	testkit.AssertEqual(t, "deleted root body", general[0].Root.Content.Raw, "")
	testkit.AssertEqual(t, "deleted root replies", len(general[0].Replies), 1)
	// The second reply of the general thread replies to the first one, and
	// BuildThreads flattens it into the same thread.
	testkit.AssertEqual(t, "general thread replies", len(general[1].Replies), 2)

	lines := bitbucket.CommentedLines(threads)
	// Three marked lines, not two: the resolved thread spans 12-13 and a
	// multi-line thread marks every line of its range.
	testkit.AssertEqual(t, "marked lines in retry.go", len(lines[pathRetryGo]), 3)
	testkit.AssertEqual(t, "threads on retry.go:16", lines[pathRetryGo][16], 1)
	testkit.AssertEqual(t, "threads on retry.go:13", lines[pathRetryGo][13], 1)
	testkit.AssertEqual(t, "threads on retry.go:12", lines[pathRetryGo][12], 1)
}

// TestDataset_FileLevelCommentHasNoLine is the anchor shape the view has to
// treat separately: a comment on a file, with every line member null.
func TestDataset_FileLevelCommentHasNoLine(t *testing.T) {
	t.Parallel()

	data := NewDataset()
	comments := data.Comments[reviewKey(repoWebApp, 40)]
	testkit.AssertEqual(t, "len(comments)", len(comments), 1)

	inline := comments[0].Inline
	testkit.AssertEqual(t, "path", inline.Path, pathThemeTS)
	if inline.From != nil || inline.To != nil || inline.StartFrom != nil || inline.StartTo != nil {
		t.Fatalf("file-level anchor carries a line: %+v", inline)
	}

	_, _, ok := inline.Line()
	testkit.AssertEqual(t, "Line() ok", ok, false)

	threads := bitbucket.BuildThreads(comments)
	testkit.AssertEqual(t, "len(threads)", len(threads), 1)
	testkit.AssertEqual(t, "file-level marker key",
		bitbucket.CommentedLines(threads)[pathThemeTS][0], 1)
}

// TestDataset_ResolvedThreadCarriesItsResolver keeps the resolution shape
// honest: an unresolved comment carries none at all, and a resolved one names
// who resolved it and when.
func TestDataset_ResolvedThreadCarriesItsResolver(t *testing.T) {
	t.Parallel()

	data := NewDataset()

	resolved := 0
	for _, comment := range data.Comments[reviewKey(repoAPIGateway, 103)] {
		if !comment.Resolution.Resolved() {
			continue
		}
		resolved++
		testkit.AssertEqual(t, "resolver", comment.Resolution.User.Nickname, "ken")
		if comment.Resolution.CreatedOn.Before(comment.CreatedOn) {
			t.Errorf("comment %d was resolved before it was written", comment.ID)
		}
	}
	testkit.AssertEqual(t, "resolved comments", resolved, 1)
}

func fileByPath(files []diff.File, path string) (diff.File, bool) {
	for _, file := range files {
		if file.Path() == path || file.OldPath == path {
			return file, true
		}
	}
	return diff.File{}, false
}

// assertAnchorLine fails unless the given line number appears on the requested
// side of the file's hunks. A nil anchor is not an anchor and passes.
func assertAnchorLine(t *testing.T, label string, file diff.File, line *int, newSide bool) {
	t.Helper()

	if line == nil {
		return
	}
	for _, hunk := range file.Hunks {
		for _, candidate := range hunk.Lines {
			number := candidate.OldNo
			if newSide {
				number = candidate.NewNo
			}
			if number == *line {
				return
			}
		}
	}
	t.Errorf("%s = %d, which is not a line of %s", label, *line, file.Path())
}

// TestDataset_CommentCountsMatchTheCommentsServed keeps every pull request's
// comment_count equal to the comments the dataset actually serves for it —
// roots and replies, the deleted one excluded — because the Comments tab shows
// the former before the tab loads and the latter afterwards, and a demo whose
// number changed on load would look broken.
func TestDataset_CommentCountsMatchTheCommentsServed(t *testing.T) {
	t.Parallel()

	data := NewDataset()
	for slug, pullRequests := range data.PullRequests {
		for _, pullRequest := range pullRequests {
			key := reviewKey(slug, pullRequest.ID)
			served := bitbucket.CommentTotal(bitbucket.BuildThreads(data.Comments[key]))
			testkit.AssertEqual(t, key+" comment_count", pullRequest.CommentCount, served)
		}
	}
}
