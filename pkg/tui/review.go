package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/diff"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// The review tabs — Files, Commits, Comments and the per-file diff behind the
// Files tab — are each backed by their own request, and none of them is fetched
// when a pull request is selected. They are warmed on tab entry instead
// (onDetailTabChanged), so a user who only ever reads the overview pays no
// extra requests, and the answers are cached per pull request so re-entering a
// tab is free.
//
// Staleness is by identity rather than by epoch: a response is correct for the
// pull request it names, so the handler caches it unconditionally and lets the
// view setter drop it when the user has moved on. What does go stale is a
// pushed branch — every cache entry is stamped with the source commit it
// describes and dropped when that commit moves (see reviewFresh).

// fileDiffKeySep joins a pull request key and a file path into the file diff
// cache's key. A NUL can appear in neither half, so the join is unambiguous.
const fileDiffKeySep = "\x00"

// reviewEntry is one cached review response. err is carried rather than
// discarded so a failure — Bitbucket's "diff too large" in particular — is not
// retried on every tab entry.
type reviewEntry[T any] struct {
	value      T
	sourceHash string
	err        error
}

// fileDiffKey addresses one file's diff within one pull request.
func fileDiffKey(prKey, path string) string { return prKey + fileDiffKeySep + path }

// sourceHash is the commit the pull request's source branch points at, "" when
// the list row did not carry one.
func sourceHash(pr *bitbucket.PullRequest) string {
	if pr == nil || pr.Source.Commit == nil {
		return ""
	}
	return pr.Source.Commit.Hash
}

// sameSource compares two source-commit stamps. Two unknown hashes count as
// the same commit: a list row without a source commit must still be cacheable,
// and bitbucket.SameCommit deliberately refuses to match short hashes.
func sameSource(a, b string) bool {
	if a == "" || b == "" {
		return a == b
	}
	return bitbucket.SameCommit(a, b)
}

// reviewFresh looks a cached review response up and reports whether it still
// describes the pull request's current head. A push moves Source.Commit, which
// is exactly what makes a diff, a diffstat or a commit list wrong.
func reviewFresh[T any](cache map[string]reviewEntry[T], key string, pr *bitbucket.PullRequest) (reviewEntry[T], bool) {
	entry, ok := cache[key]
	if !ok || !sameSource(entry.sourceHash, sourceHash(pr)) {
		var zero reviewEntry[T]
		return zero, false
	}
	return entry, true
}

// ensureReviewCaches creates the four caches on first use. The hand-built App
// values tests construct do not initialise every map, and a nil map is a panic
// on write rather than a compile error.
func (a *App) ensureReviewCaches() {
	if a.diffStatCache == nil {
		a.diffStatCache = make(map[string]reviewEntry[[]bitbucket.DiffStatEntry])
	}
	if a.commitsCache == nil {
		a.commitsCache = make(map[string]reviewEntry[[]bitbucket.Commit])
	}
	if a.threadsCache == nil {
		a.threadsCache = make(map[string]reviewEntry[[]bitbucket.Thread])
	}
	if a.fileDiffCache == nil {
		a.fileDiffCache = make(map[string]reviewEntry[*diff.File])
	}
}

// dropReviewCaches forgets everything cached for one pull request, including
// every file diff under it. It is what `r` does, and what a new push does.
func (a *App) dropReviewCaches(key string) {
	delete(a.diffStatCache, key)
	delete(a.commitsCache, key)
	delete(a.threadsCache, key)
	prefix := key + fileDiffKeySep
	for cached := range a.fileDiffCache {
		if strings.HasPrefix(cached, prefix) {
			delete(a.fileDiffCache, cached)
		}
	}
}

// dropStaleReviewCaches drops a pull request's review data when the pull
// request in hand describes a different head than the data does. It runs on
// every selection and every background refresh, so a push made while the app
// is open never shows a stale diff.
func (a *App) dropStaleReviewCaches(pr *bitbucket.PullRequest) {
	key, hash := pr.Key(), sourceHash(pr)
	stale := false
	if entry, ok := a.diffStatCache[key]; ok && !sameSource(entry.sourceHash, hash) {
		stale = true
	}
	if entry, ok := a.commitsCache[key]; ok && !sameSource(entry.sourceHash, hash) {
		stale = true
	}
	if entry, ok := a.threadsCache[key]; ok && !sameSource(entry.sourceHash, hash) {
		stale = true
	}
	prefix := key + fileDiffKeySep
	for cached, entry := range a.fileDiffCache {
		if strings.HasPrefix(cached, prefix) && !sameSource(entry.sourceHash, hash) {
			stale = true
			break
		}
	}
	if stale {
		a.dropReviewCaches(key)
	}
}

// -----------------------------------------------------------------------------
// Tab entry
// -----------------------------------------------------------------------------

// onDetailTabChanged warms the tab the detail panel just landed on. Every path
// that switches a detail tab — the `[`/`]` cycle, the four jump keys and a
// click on the tab bar — goes through it, and it is the only thing that ever
// issues these four requests.
func (a *App) onDetailTabChanged() tea.Cmd {
	pr := a.currentPullRequest()
	if pr == nil || a.detailView.Mode() != views.ModePullRequest {
		return nil
	}
	switch a.detailView.ActiveTab() {
	case views.TabFiles:
		return a.ensureDiffStat(pr)
	case views.TabCommits:
		return a.ensureCommits(pr)
	case views.TabComments:
		return a.ensureThreads(pr)
	case views.TabOverview:
		// The overview is rendered from the pull request itself.
		return nil
	}
	return nil
}

// ensureDiffStat feeds the Files tab from the cache while the cache still
// describes the pull request's head, and fetches it otherwise.
func (a *App) ensureDiffStat(pr *bitbucket.PullRequest) tea.Cmd {
	key := pr.Key()
	if entry, ok := reviewFresh(a.diffStatCache, key, pr); ok {
		if entry.err != nil {
			a.detailView.SetDiffStatError(key, a.reviewErrorText(entry.err, "diff stat"))
		} else {
			a.detailView.SetDiffStat(key, entry.value)
		}
		return nil
	}
	a.detailView.SetDiffStatLoading(key)
	return a.fetchDiffStat(pr)
}

// ensureCommits is ensureDiffStat for the Commits tab.
func (a *App) ensureCommits(pr *bitbucket.PullRequest) tea.Cmd {
	key := pr.Key()
	if entry, ok := reviewFresh(a.commitsCache, key, pr); ok {
		if entry.err != nil {
			a.detailView.SetCommitsError(key, entry.err.Error())
		} else {
			a.detailView.SetCommits(key, entry.value)
		}
		return nil
	}
	a.detailView.SetCommitsLoading(key)
	return a.fetchCommits(pr)
}

// ensureThreads is ensureDiffStat for the Comments tab. Opening a file diff
// warms it too, because the diff's comment markers come from the threads.
func (a *App) ensureThreads(pr *bitbucket.PullRequest) tea.Cmd {
	key := pr.Key()
	if entry, ok := reviewFresh(a.threadsCache, key, pr); ok {
		if entry.err != nil {
			a.detailView.SetThreadsError(key, entry.err.Error())
		} else {
			a.detailView.SetThreads(key, entry.value)
		}
		return nil
	}
	a.detailView.SetThreadsLoading(key)
	return a.fetchThreads(pr)
}

// reviewErrorText is what a failed review fetch says on screen. Bitbucket's
// 555 means the server gave up building the document, so retrying is pointless
// and the message names the way out instead of the status code.
func (a *App) reviewErrorText(err error, what string) string {
	if bitbucket.IsDiffTooLarge(err) {
		return what + " too large — open in browser (" + a.keymap.Keys(ActBrowser) + ")"
	}
	return err.Error()
}

// -----------------------------------------------------------------------------
// Handlers
// -----------------------------------------------------------------------------

// handleDiffStatLoaded applies the changed-file list to the Files tab.
func (a *App) handleDiffStatLoaded(msg diffStatLoadedMsg) (tea.Model, tea.Cmd) {
	a.logFlag.Store(false)
	a.ensureReviewCaches()
	a.diffStatCache[msg.key] = reviewEntry[[]bitbucket.DiffStatEntry]{
		value: msg.entries, sourceHash: msg.hash, err: msg.err,
	}
	if msg.err != nil {
		text := a.reviewErrorText(msg.err, "diff stat")
		a.detailView.SetDiffStatError(msg.key, text)
		a.noteReviewFailure(msg.err, text)
		return a, nil
	}
	a.statusPanel.SetOnline(true)
	a.detailView.SetDiffStat(msg.key, msg.entries)
	return a, nil
}

// handleCommitsLoaded applies the commit list to the Commits tab.
func (a *App) handleCommitsLoaded(msg commitsLoadedMsg) (tea.Model, tea.Cmd) {
	a.logFlag.Store(false)
	a.ensureReviewCaches()
	a.commitsCache[msg.key] = reviewEntry[[]bitbucket.Commit]{
		value: msg.commits, sourceHash: msg.hash, err: msg.err,
	}
	if msg.err != nil {
		a.detailView.SetCommitsError(msg.key, msg.err.Error())
		a.noteReviewFailure(msg.err, msg.err.Error())
		return a, nil
	}
	a.statusPanel.SetOnline(true)
	a.detailView.SetCommits(msg.key, msg.commits)
	return a, nil
}

// handleThreadsLoaded applies the review threads to the Comments tab, and lights
// up the markers of a file diff that is already on screen.
func (a *App) handleThreadsLoaded(msg threadsLoadedMsg) (tea.Model, tea.Cmd) {
	a.logFlag.Store(false)
	a.ensureReviewCaches()
	a.threadsCache[msg.key] = reviewEntry[[]bitbucket.Thread]{
		value: msg.threads, sourceHash: msg.hash, err: msg.err,
	}
	if msg.err != nil {
		a.detailView.SetThreadsError(msg.key, msg.err.Error())
		a.noteReviewFailure(msg.err, msg.err.Error())
		return a, nil
	}
	a.statusPanel.SetOnline(true)
	a.detailView.SetThreads(msg.key, msg.threads)
	a.applyFileDiffMarkers(msg.key)
	return a, nil
}

// handleFileDiffLoaded shows one file's parsed diff.
func (a *App) handleFileDiffLoaded(msg fileDiffLoadedMsg) (tea.Model, tea.Cmd) {
	a.logFlag.Store(false)
	a.ensureReviewCaches()
	a.fileDiffCache[fileDiffKey(msg.key, msg.path)] = reviewEntry[*diff.File]{
		value: msg.file, sourceHash: msg.hash, err: msg.err,
	}
	if msg.err != nil {
		text := a.reviewErrorText(msg.err, "diff")
		a.detailView.SetFileDiffError(msg.key, msg.path, text)
		a.noteReviewFailure(msg.err, text)
		return a, nil
	}
	a.statusPanel.SetOnline(true)
	a.detailView.SetFileDiff(msg.key, msg.path, msg.file, a.commentMarkers(msg.key, msg.path))
	return a, nil
}

// noteReviewFailure records a review fetch failure outside the tab it belongs
// to. A 555 is data — the server refusing a document it cannot build — and
// belongs only in the tab; anything else is a connection or permission problem
// worth the status panel.
func (a *App) noteReviewFailure(err error, text string) {
	if bitbucket.IsDiffTooLarge(err) {
		a.helpBar.SetStatusMsg(text)
		return
	}
	a.statusPanel.SetError(text)
}

// handleFileSelected opens the diff of the file the user activated on the Files
// tab.
func (a *App) handleFileSelected(msg views.FileSelectedMsg) (tea.Model, tea.Cmd) {
	if a.detailView.PullRequestKey() != msg.PullRequestKey {
		return a, nil
	}
	return a, a.openFileDiff(msg.Path)
}

// handleThreadSelected lands on the conversation anchored to a commented diff
// line. The threads can still be in flight, in which case there is nothing to
// jump to yet and the help bar says so.
func (a *App) handleThreadSelected(msg views.ThreadSelectedMsg) (tea.Model, tea.Cmd) {
	if a.detailView.PullRequestKey() != msg.PullRequestKey {
		return a, nil
	}
	if !a.detailView.FocusThread(msg.RootID) {
		a.helpBar.SetStatusMsg("comments still loading")
	}
	return a, nil
}

// -----------------------------------------------------------------------------
// The per-file diff
// -----------------------------------------------------------------------------

// openFileDiff shows one changed file's diff, from the cache when it is warm.
// The review threads go through ensureThreads alongside it — fetched when they
// are cold, pushed into the view from the cache when they are warm — because
// that is where the diff's comment markers come from, and a marker the view has
// no thread list behind is a dead marker.
func (a *App) openFileDiff(path string) tea.Cmd {
	pr := a.currentPullRequest()
	if pr == nil || path == "" {
		return nil
	}
	a.ensureReviewCaches()
	key := pr.Key()
	a.detailView.SetFileDiffLoading(key, path)

	var cmds []tea.Cmd
	if cmd := a.ensureThreads(pr); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if entry, ok := reviewFresh(a.fileDiffCache, fileDiffKey(key, path), pr); ok {
		if entry.err != nil {
			a.detailView.SetFileDiffError(key, path, a.reviewErrorText(entry.err, "diff"))
		} else {
			a.detailView.SetFileDiff(key, path, entry.value, a.commentMarkers(key, path))
		}
	} else {
		cmds = append(cmds, a.fetchFileDiff(pr, path, a.escapedPathFor(path)))
	}
	return tea.Batch(cmds...)
}

// applyFileDiffMarkers re-feeds the file diff on screen now that its comment
// markers are known. SetFileDiff leaves the cursor alone, so a reader who has
// already scrolled stays where they were and the markers simply appear.
func (a *App) applyFileDiffMarkers(key string) {
	path := a.detailView.FileDiffPath()
	if path == "" || a.detailView.PullRequestKey() != key {
		return
	}
	entry, ok := a.fileDiffCache[fileDiffKey(key, path)]
	if !ok || entry.err != nil {
		return
	}
	a.detailView.SetFileDiff(key, path, entry.value, a.commentMarkers(key, path))
}

// commentMarkers is the line → thread-count map the diff view marks lines from,
// nil while the threads are still cold.
func (a *App) commentMarkers(key, path string) map[int]int {
	entry, ok := a.threadsCache[key]
	if !ok || entry.err != nil {
		return nil
	}
	return bitbucket.CommentedLines(entry.value)[path]
}

// escapedPathFor is the path= parameter the diff endpoint wants: the diffstat's
// escaped form when the file is listed there, else the display path.
func (a *App) escapedPathFor(path string) string {
	for _, entry := range a.detailView.DiffStat() {
		if entry.Path() == path {
			return entry.EscapedPath()
		}
	}
	return path
}

// cachedFileDiff is the parsed diff of one file of the pull request in view,
// nil when it is not cached or failed.
func (a *App) cachedFileDiff(path string) *diff.File {
	entry, ok := a.fileDiffCache[fileDiffKey(a.detailView.PullRequestKey(), path)]
	if !ok || entry.err != nil {
		return nil
	}
	return entry.value
}

// refreshReviewData refetches whatever review data is on screen: the file diff
// when one is open, the active tab's list otherwise. It is the review half of
// the `r` key.
func (a *App) refreshReviewData() tea.Cmd {
	if path := a.detailView.FileDiffPath(); path != "" {
		return a.openFileDiff(path)
	}
	return a.onDetailTabChanged()
}
