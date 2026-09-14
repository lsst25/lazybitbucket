package tui

import (
	"errors"
	"io/fs"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/diff"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// The review tabs — Files, Commits, Comments and the per-file diff behind the
// Files tab — are each backed by their own request, and none of them is fetched
// when a pull request is selected. They are warmed when the reader moves in
// instead: all three together the moment the detail panel takes focus
// (prefetchDetailTabs, gui.prefetchDetailTabs), and one at a time on tab entry
// (onDetailTabChanged) when that setting is off or the panel was focused on
// something other than a pull request. A user who only skims the list pays no
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

// fileDiffData is one file's diff as the view needs it. rendered is the
// external diff renderer's output for the whole document the file was parsed
// out of, one finished code cell per line, indexed by diff.Line.Source; it is
// nil whenever the built-in colouring is in charge.
type fileDiffData struct {
	file     *diff.File
	rendered []string
}

// fileDiffKey addresses one file's diff within one pull request.
func fileDiffKey(prKey, path string) string { return prKey + fileDiffKeySep + path }

// The three review fetches, as reviewFetchKey names them.
const (
	reviewKindDiffStat = "diffstat"
	reviewKindCommits  = "commits"
	reviewKindThreads  = "threads"
)

// reviewFetchKey addresses one review fetch: its kind, the pull request it is
// for and the head it describes. The head is part of the key because a fetch
// issued for an older head is not the answer a pushed branch is waiting for.
func reviewFetchKey(kind, key, hash string) string {
	return kind + fileDiffKeySep + key + fileDiffKeySep + hash
}

// claimReviewFetch reserves one review fetch, answering false when the same
// fetch is already in flight. It is what keeps a tab entered in the middle of a
// prefetch from asking a second time: the answer already on its way is the one
// that tab wants.
func (a *App) claimReviewFetch(kind, key, hash string) bool {
	a.ensureReviewCaches()
	fetchKey := reviewFetchKey(kind, key, hash)
	if a.reviewInFlight[fetchKey] {
		return false
	}
	a.reviewInFlight[fetchKey] = true
	return true
}

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
		a.fileDiffCache = make(map[string]reviewEntry[fileDiffData])
	}
	if a.reviewInFlight == nil {
		a.reviewInFlight = make(map[string]bool)
	}
}

// dropReviewCaches forgets everything cached for one pull request, including
// every file diff under it, and releases the fetches still in flight for it, so
// the next entry asks again rather than waiting for an answer that was asked
// for before the drop. It is what `r` does, and what a new push does.
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
	inFlight := fileDiffKeySep + key + fileDiffKeySep
	for fetch := range a.reviewInFlight {
		if strings.Contains(fetch, inFlight) {
			delete(a.reviewInFlight, fetch)
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

// prefetchDetailTabs warms the Files, Commits and Comments tabs of the pull
// request on screen in one go. It is what focusing the detail panel means with
// gui.prefetchDetailTabs on: a reader who moves in is about to walk the tabs,
// and three requests now beat a pause on each tab later. It does nothing when
// the setting is off, when no pull request is on screen, or when the panel is
// showing a pipeline, a step log or a file diff; the tab a reader lands on
// still warms itself through onDetailTabChanged.
func (a *App) prefetchDetailTabs() tea.Cmd {
	if a.cfg == nil || !a.cfg.GUI.PrefetchesDetailTabs() || a.view != viewPullRequests {
		return nil
	}
	pr := a.currentPullRequest()
	if pr == nil || a.detailView.Mode() != views.ModePullRequest {
		return nil
	}
	cmds := make([]tea.Cmd, 0, 3)
	if cmd := a.ensureDiffStat(pr); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if cmd := a.ensureCommits(pr); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if cmd := a.ensureThreads(pr); cmd != nil {
		cmds = append(cmds, cmd)
	}
	switch len(cmds) {
	case 0:
		return nil
	case 1:
		return cmds[0]
	default:
		return tea.Batch(cmds...)
	}
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
	// The loading flag is set even when the fetch is already in flight: the
	// view drops it whenever the reader walks to another pull request, so the
	// tab they come back to has to be told again that an answer is on its way.
	a.detailView.SetDiffStatLoading(key)
	if !a.claimReviewFetch(reviewKindDiffStat, key, sourceHash(pr)) {
		return nil
	}
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
	if !a.claimReviewFetch(reviewKindCommits, key, sourceHash(pr)) {
		return nil
	}
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
	if !a.claimReviewFetch(reviewKindThreads, key, sourceHash(pr)) {
		return nil
	}
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
	delete(a.reviewInFlight, reviewFetchKey(reviewKindDiffStat, msg.key, msg.hash))
	a.logFlag.Store(false)
	a.ensureReviewCaches()
	if outdatedReviewAnswer(a.diffStatCache, msg.key, msg.hash, a.pullRequestByKey(msg.key)) {
		return a, nil
	}
	a.diffStatCache[msg.key] = reviewEntry[[]bitbucket.DiffStatEntry]{
		value: msg.entries, sourceHash: msg.hash, err: msg.err,
	}
	if msg.err != nil {
		text := a.reviewErrorText(msg.err, "diff stat")
		a.detailView.SetDiffStatError(msg.key, text)
		if a.reviewTabOnScreen(msg.key, views.TabFiles) {
			a.noteReviewFailure(msg.err, text)
		}
		if a.pendingCmd != nil && a.pendingCmd.key == msg.key {
			a.statusPanel.SetError(a.pendingCmd.cmd.Name + ": " + text)
			a.pendingCmd = nil
		}
		return a, nil
	}
	a.statusPanel.SetOnline(true)
	a.detailView.SetDiffStat(msg.key, msg.entries)
	return a, a.resumeCustomCommand(msg.key)
}

// handleCommitsLoaded applies the commit list to the Commits tab.
func (a *App) handleCommitsLoaded(msg commitsLoadedMsg) (tea.Model, tea.Cmd) {
	delete(a.reviewInFlight, reviewFetchKey(reviewKindCommits, msg.key, msg.hash))
	a.logFlag.Store(false)
	a.ensureReviewCaches()
	if outdatedReviewAnswer(a.commitsCache, msg.key, msg.hash, a.pullRequestByKey(msg.key)) {
		return a, nil
	}
	a.commitsCache[msg.key] = reviewEntry[[]bitbucket.Commit]{
		value: msg.commits, sourceHash: msg.hash, err: msg.err,
	}
	if msg.err != nil {
		a.detailView.SetCommitsError(msg.key, msg.err.Error())
		if a.reviewTabOnScreen(msg.key, views.TabCommits) {
			a.noteReviewFailure(msg.err, msg.err.Error())
		}
		return a, nil
	}
	a.statusPanel.SetOnline(true)
	a.detailView.SetCommits(msg.key, msg.commits)
	return a, nil
}

// handleThreadsLoaded applies the review threads to the Comments tab, and lights
// up the markers of a file diff that is already on screen.
func (a *App) handleThreadsLoaded(msg threadsLoadedMsg) (tea.Model, tea.Cmd) {
	delete(a.reviewInFlight, reviewFetchKey(reviewKindThreads, msg.key, msg.hash))
	a.logFlag.Store(false)
	a.ensureReviewCaches()
	if outdatedReviewAnswer(a.threadsCache, msg.key, msg.hash, a.pullRequestByKey(msg.key)) {
		return a, nil
	}
	a.threadsCache[msg.key] = reviewEntry[[]bitbucket.Thread]{
		value: msg.threads, sourceHash: msg.hash, err: msg.err,
	}
	if msg.err != nil {
		a.detailView.SetThreadsError(msg.key, msg.err.Error())
		if a.reviewTabOnScreen(msg.key, views.TabComments) {
			a.noteReviewFailure(msg.err, msg.err.Error())
		}
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
	a.noteRendererFailure(msg.rendererErr)
	data := fileDiffData{file: msg.file, rendered: msg.rendered}
	a.fileDiffCache[fileDiffKey(msg.key, msg.path)] = reviewEntry[fileDiffData]{
		value: data, sourceHash: msg.hash, err: msg.err,
	}
	if msg.err != nil {
		text := a.reviewErrorText(msg.err, "diff")
		a.detailView.SetFileDiffError(msg.key, msg.path, text)
		a.noteReviewFailure(msg.err, text)
		return a, nil
	}
	a.statusPanel.SetOnline(true)
	a.detailView.SetFileDiff(msg.key, msg.path, data.file, data.rendered, a.commentMarkers(msg.key, msg.path))
	return a, nil
}

// noteRendererFailure reports a `diffRenderer` that did not work out. The diff
// itself is fine — it is on screen with the built-in colouring — so the
// complaint goes to the status panel and only once per distinct message: a
// broken renderer would otherwise repaint it on every file opened.
//
// A program that cannot be run at all, and one whose output does not line up
// with the diff, are conditions no later file improves, so they turn the
// renderer off for the rest of the session. A timeout or a non-zero exit does
// not: a program can perfectly well choke on one file and handle the next.
//
// "Cannot be run at all" has two shapes, because exec only searches $PATH for
// a bare name: `delta` that is not installed fails with exec.ErrNotFound,
// while `~/bin/delta` — a path, and nothing expands the tilde — fails at
// fork/exec with an fs.PathError. Both are the same misconfiguration.
func (a *App) noteRendererFailure(err error) {
	if err == nil {
		return
	}
	if rendererIsHopeless(err) {
		a.diffRendererDisabled = true
	}
	text := "diffRenderer: " + err.Error()
	if text == a.lastRendererError {
		return
	}
	a.lastRendererError = text
	a.statusPanel.SetError(text)
	// The status panel is a narrow column that truncates; the hint bar spans
	// the window, so the whole sentence is readable there until the next key.
	a.helpBar.SetStatusMsg(text)
}

// rendererIsHopeless reports whether a renderer failure condemns the program
// for the rest of the session rather than costing only the file it happened on.
func rendererIsHopeless(err error) bool {
	return errors.Is(err, exec.ErrNotFound) ||
		errors.Is(err, fs.ErrNotExist) ||
		errors.Is(err, errDiffRendererMisaligned)
}

// outdatedReviewAnswer reports whether an answer for key would overwrite a
// fresher one: the cache already holds the pull request's current head and the
// answer describes another. Two fetches issued for two heads land in whichever
// order the network picks, so the order says nothing about which one is right.
func outdatedReviewAnswer[T any](
	cache map[string]reviewEntry[T], key, hash string, pr *bitbucket.PullRequest,
) bool {
	entry, ok := cache[key]
	if !ok || pr == nil || hash == "" || entry.sourceHash == "" {
		return false
	}
	return sameSource(entry.sourceHash, sourceHash(pr)) && !sameSource(entry.sourceHash, hash)
}

// reviewTabOnScreen reports whether the reader is looking at tab for the pull
// request key — the one case a review fetch failure is worth announcing outside
// the tab. A prefetch that fails behind the overview says nothing until the
// reader opens the tab, which shows the failure itself; inside a file diff the
// Files and Comments answers are the ones in use, so those two still speak.
func (a *App) reviewTabOnScreen(key string, tab views.DetailTab) bool {
	pr := a.currentPullRequest()
	if pr == nil || pr.Key() != key {
		return false
	}
	switch a.detailView.Mode() {
	case views.ModePullRequest:
		return a.detailView.ActiveTab() == tab
	case views.ModeFileDiff:
		return tab == views.TabFiles || tab == views.TabComments
	case views.ModeThread:
		return tab == views.TabComments
	default:
		return false
	}
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
			a.detailView.SetFileDiff(key, path, entry.value.file, entry.value.rendered,
				a.commentMarkers(key, path))
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
	a.detailView.SetFileDiff(key, path, entry.value.file, entry.value.rendered,
		a.commentMarkers(key, path))
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
	return entry.value.file
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
