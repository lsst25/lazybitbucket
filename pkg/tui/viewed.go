package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
	"github.com/lsst25/lazybitbucket/pkg/viewed"
)

// Viewed files. `v` marks the changed file in hand as viewed — the one under
// the Files tab's cursor, or the one whose diff is open — and pressing it again
// clears the mark. Marking moves on to the next file not yet viewed, so a review
// is a pass of `v` presses down the pull request.
//
// Bitbucket's public API has no viewed-file state, so the marks live in a
// pkg/viewed store on this machine. Each mark carries the source commit it was
// made at and counts only at that commit. When a push moves the branch, one
// diffstat between the old and the new head (CompareDiffStat, limited to the
// marked paths) says which marked files changed; their marks are dropped and the
// rest are carried over to the new head. Until that answer arrives the marks of
// the old head are simply not shown, so a file is never claimed viewed when it
// may have changed.

// reviewKindViewed is the reviewFetchKey kind of a viewed-mark comparison.
const reviewKindViewed = "viewed"

// viewedComparedMsg answers one comparison of the marked files between the
// commit they were marked at (from) and the pull request's head (to).
type viewedComparedMsg struct {
	key     string
	from    string
	to      string
	paths   []string
	entries []bitbucket.DiffStatEntry
	err     error
}

// SetViewedStore gives the app the store the viewed marks are kept in, and is
// shaped to take viewed.Open's answer as it is. A store that could not be
// opened is replaced by one in memory, and the status panel says why: the marks
// still work for the session, but nothing is written over a file the app could
// not read. Without a call the app keeps the marks in memory, which is what
// the demo and the tests get.
func (a *App) SetViewedStore(store *viewed.Store, err error) {
	if err != nil || store == nil {
		a.viewed = viewed.New()
		if err != nil {
			// A warning rather than an error: the first pull request load
			// clears the error line, and this has to outlive it.
			a.setWarning("viewed files not loaded: " + err.Error())
		}
		return
	}
	a.viewed = store
}

// viewedStore is the store in use, created in memory on first use.
func (a *App) viewedStore() *viewed.Store {
	if a.viewed == nil {
		a.viewed = viewed.New()
	}
	return a.viewed
}

// syncViewed pushes the pull request's current viewed set to the detail view.
func (a *App) syncViewed(pr *bitbucket.PullRequest) {
	if pr == nil {
		return
	}
	key := pr.Key()
	a.detailView.SetViewedFiles(key, a.viewedStore().ViewedPaths(key, sourceHash(pr)))
}

// saveViewed writes the marks, and says so on the status line when it cannot:
// the mark itself still holds for this session.
func (a *App) saveViewed() {
	if err := a.viewedStore().Save(); err != nil {
		a.helpBar.SetStatusMsg("viewed files not saved: " + err.Error())
	}
}

// reconcileViewed asks which marked files changed since they were marked, once
// per commit the marks were made at, and carries the rest over to the head.
func (a *App) reconcileViewed(pr *bitbucket.PullRequest) tea.Cmd {
	if pr == nil {
		return nil
	}
	head := sourceHash(pr)
	if head == "" {
		return nil
	}
	key := pr.Key()
	store := a.viewedStore()
	stale := store.Stale(key, head)
	if len(stale) == 0 {
		return nil
	}

	var cmds []tea.Cmd
	for from, paths := range stale {
		if from == "" {
			// Marked when the row carried no commit: there is nothing to
			// compare with, so the marks cannot be trusted at a real head.
			if store.Reconcile(key, from, head, paths) {
				a.saveViewed()
			}
			continue
		}
		if !a.claimReviewFetch(reviewKindViewed, key, from+".."+head) {
			continue
		}
		cmds = append(cmds, a.compareViewed(pr, from, head, paths))
	}
	return tea.Batch(cmds...)
}

// compareViewed lists which of paths differ between from and to.
func (a *App) compareViewed(pr *bitbucket.PullRequest, from, to string, paths []string) tea.Cmd {
	ctx, client := a.context(), a.client
	workspace, slug, key := pr.Workspace(), pr.RepoSlug(), pr.Key()
	a.logFlag.Store(true)
	return func() tea.Msg {
		// Bitbucket reads the spec as "first compared to second", the newer
		// commit first.
		entries, err := client.CompareDiffStat(ctx, workspace, slug, to, from, paths)
		return viewedComparedMsg{key: key, from: from, to: to, paths: paths, entries: entries, err: err}
	}
}

// handleViewedCompared applies a comparison: the marks on files that changed
// are dropped, the others move to the new head. A commit the server no longer
// has — a force push it has already collected — leaves nothing to compare with,
// so every mark made at it is dropped. Any other failure leaves the marks where
// they are: they stay hidden, and the next visit to the Files tab asks again.
func (a *App) handleViewedCompared(msg viewedComparedMsg) (tea.Model, tea.Cmd) {
	delete(a.reviewInFlight, reviewFetchKey(reviewKindViewed, msg.key, msg.from+".."+msg.to))
	a.logFlag.Store(false)

	var changed []string
	switch {
	case msg.err == nil:
		changed = make([]string, 0, 2*len(msg.entries))
		for _, entry := range msg.entries {
			changed = append(changed, entry.Path())
			if old := entry.OldPath(); old != "" {
				changed = append(changed, old)
			}
		}
	case bitbucket.IsNotFound(msg.err):
		changed = msg.paths
	default:
		return a, nil
	}

	if a.viewedStore().Reconcile(msg.key, msg.from, msg.to, changed) {
		a.saveViewed()
	}
	if pr := a.pullRequestByKey(msg.key); pr != nil {
		a.syncViewed(pr)
	}
	return a, nil
}

// viewedTarget is the changed file `v` acts on: the open diff's file, or the
// one under the Files tab's cursor. ok is false anywhere else.
func (a *App) viewedTarget() (string, bool) {
	switch a.detailView.Mode() {
	case views.ModeFileDiff:
		path := a.detailView.FileDiffPath()
		return path, path != ""
	case views.ModePullRequest:
		if a.detailView.ActiveTab() != views.TabFiles {
			return "", false
		}
		if entry := a.detailView.SelectedFile(); entry != nil {
			return entry.Path(), true
		}
		return "", false
	case views.ModeThread, views.ModePipeline, views.ModeStepLog, views.ModeRepo, views.ModeSplash:
		return "", false
	default:
		return "", false
	}
}

// handleActionToggleViewed marks the file in hand as viewed, or clears its
// mark. Marking moves on to the next file not yet viewed — the cursor on the
// Files tab, the diff itself in a file diff — and a diff with nothing left to
// view goes back to the list, which now shows every file ticked.
func (a *App) handleActionToggleViewed() tea.Cmd {
	path, ok := a.viewedTarget()
	if !ok {
		return nil
	}
	pr := a.currentPullRequest()
	if pr == nil || pr.Key() != a.detailView.PullRequestKey() {
		return nil
	}
	key, head := pr.Key(), sourceHash(pr)
	store := a.viewedStore()

	if store.Viewed(key, path, head) {
		store.Unmark(key, path)
		a.saveViewed()
		a.syncViewed(pr)
		a.helpBar.SetStatusMsg("not viewed: " + path)
		return nil
	}

	store.Mark(key, path, head)
	a.saveViewed()
	a.syncViewed(pr)

	next, more := a.detailView.NextUnviewedFile(path)
	inDiff := a.detailView.Mode() == views.ModeFileDiff
	switch {
	case !more:
		if inDiff {
			a.detailView.Back()
		}
		a.helpBar.SetStatusMsg("every file viewed")
		return nil
	case inDiff:
		a.helpBar.SetStatusMsg("viewed: " + path)
		return a.openFileDiff(next)
	default:
		a.detailView.SelectFile(next)
		a.helpBar.SetStatusMsg("viewed: " + path)
		return nil
	}
}
