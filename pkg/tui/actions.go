package tui

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/git"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
)

// The six write actions. Each one opens a dialog, and only what the user
// confirms in that dialog ever reaches Bitbucket — there is no key in this file
// that writes on its own.
//
// The verbs are constants because each appears in four places: the refusal
// ("read-only mode: cannot merge"), the in-flight guard, the progress line and
// the failure line. Approving has two of them because the one key does two
// things, and a line reading "approve failed" for a withdrawal would name the
// wrong one.
const (
	verbMerge       = "merge"
	verbSync        = "sync"
	verbEdit        = "edit"
	verbReviewers   = "edit reviewers"
	verbToggleDraft = "toggle draft"
	verbApprove     = "approve"
	verbUnapprove   = "remove approval"
)

// The form ids. FormConfirmedMsg carries one, so a stale confirmation can be
// told apart from the dialog that is actually open.
const (
	formMerge   = "merge"
	formSync    = "sync"
	formDraft   = "draft"
	formApprove = "approve"
)

// cannotApproveOwn is the local refusal that saves a request: Bitbucket answers
// 400 to an author approving their own pull request.
const cannotApproveOwn = "cannot approve your own pull request"

// The form field ids, which is how a confirmed form's values are read back.
const (
	fieldStrategy    = "strategy"
	fieldCloseBranch = "close"
	fieldMessage     = "message"
	fieldMethod      = "method"
	fieldPush        = "push"
)

// The rows of the edit chooser.
const (
	editItemTitle       = "title"
	editItemDescription = "description"
)

// The two ways to bring a branch up to date with its target.
const (
	syncMethodMerge  = "merge"
	syncMethodRebase = "rebase"
)

// mergePollInterval is how long the app waits between polls of a queued merge,
// and mergePollMaxAttempts how many polls it spends before telling the user to
// come back to it — a minute in all, which is far longer than Bitbucket takes.
const (
	mergePollInterval    = 2 * time.Second
	mergePollMaxAttempts = 30
)

// reviewerCandidateLimit bounds the workspace member walk behind the reviewer
// picker. A large workspace has thousands of accounts and the picker shows a
// screenful, so the list is capped rather than paged to the end.
const reviewerCandidateLimit = 500

// The suffixes the reviewer picker labels its rows with, so a row says where it
// came from without a second column.
const (
	groupReviewer        = "reviewer"
	groupDefaultReviewer = "default reviewer"
	groupMember          = "member"
)

// reviewerCandidate is one row of the reviewer picker: an account, the label it
// is shown under and whether it starts checked.
type reviewerCandidate struct {
	uuid     string
	label    string
	group    string
	selected bool
}

// -----------------------------------------------------------------------------
// Guards and bookkeeping
// -----------------------------------------------------------------------------

// overlayVisible reports whether any overlay owns the screen, or has just been
// answered and its result is still on the way back. A write key must not open a
// second dialog behind the one the user is looking at — nor in the window
// between a confirmation and its message, which is not a moment of screen time
// but is a moment of app time: the overlay hides itself and returns the result
// as a command, so a key pressed in between would arm a second callback that
// the first dialog's result then fires.
func (a *App) overlayVisible() bool {
	return a.modal.IsVisible() || a.form.IsVisible() || a.input.IsVisible() || a.overlayArmed()
}

// overlayArmed reports whether an overlay result slot is still waiting for its
// message. Every way out of an overlay — confirm and cancel alike — emits one,
// and the handlers clear all four slots before dispatching, so a slot that is
// still armed means the answer has not been delivered yet.
func (a *App) overlayArmed() bool {
	return a.onSelect != nil || a.onChecklist != nil || a.onForm != nil || a.onInput != nil
}

// writeTarget is the pull request a write key should act on, or false with the
// reason already on the help bar. Every write flow starts here, so the five
// refusals — nothing selected, read-only mode, a pull request that is no longer
// open, another dialog, another write still running — are written once.
func (a *App) writeTarget(verb string) (*bitbucket.PullRequest, bool) {
	pr := a.currentPullRequest()
	if pr == nil {
		return nil, false
	}
	if a.readOnly {
		a.helpBar.SetStatusMsg("read-only mode: cannot " + verb)
		return nil, false
	}
	if pr.State != bitbucket.PullRequestOpen {
		a.helpBar.SetStatusMsg("cannot " + verb + ": pull request is " + string(pr.State))
		return nil, false
	}
	if a.overlayVisible() {
		return nil, false
	}
	if running, busy := a.writesInFlight[pr.Key()]; busy {
		a.helpBar.SetStatusMsg(running + " already in progress")
		return nil, false
	}
	return pr, true
}

// armForm arms the form result slot together with the id of the dialog being
// opened, so a result carrying another id can be told apart from this one's.
func (a *App) armForm(id string, fn onFormFunc) {
	a.onForm = fn
	a.onFormID = id
}

// beginWrite records that a write is running against a pull request, which is
// what stops a second M or D from racing the first.
func (a *App) beginWrite(key, verb string) {
	if a.writesInFlight == nil {
		a.writesInFlight = make(map[string]string)
	}
	a.writesInFlight[key] = verb
	a.logFlag.Store(true)
}

// finishWrite clears the in-flight record, on success and on failure alike.
func (a *App) finishWrite(key string) {
	delete(a.writesInFlight, key)
}

// writeProgressText is the help-bar line while a write is in flight.
func writeProgressText(verb string, id int) string {
	num := " #" + strconv.Itoa(id) + "…"
	switch verb {
	case verbMerge:
		return "merging" + num
	case verbReviewers:
		return "updating reviewers" + num
	case verbEdit, verbToggleDraft:
		return "updating" + num
	case verbApprove:
		return "approving" + num
	case verbUnapprove:
		return "removing approval" + num
	default:
		return verb + num
	}
}

// writeDoneText is the help-bar line a write leaves behind.
func writeDoneText(verb string, id int) string {
	num := " #" + strconv.Itoa(id)
	switch verb {
	case verbMerge:
		return "merged" + num
	case verbReviewers:
		return "reviewers updated" + num
	case verbEdit, verbToggleDraft:
		return "updated" + num
	case verbApprove:
		return "approved" + num
	case verbUnapprove:
		return "approval removed" + num
	default:
		return verb + num
	}
}

// writeErrorText is what the help bar shows when a write fails: the server's own
// sentence, field detail included, rather than a Go error string. Three failures
// have better answers than the server's wording — the local read-only refusal,
// the merge that lost a race, and the merge that ran past the gateway's deadline
// and may well have completed anyway.
func (a *App) writeErrorText(verb string, err error) string {
	if bitbucket.IsReadOnly(err) {
		return "read-only mode"
	}
	var apiErr *bitbucket.APIError
	if !errors.As(err, &apiErr) {
		return err.Error()
	}
	if verb == verbMerge {
		switch apiErr.Status {
		case http.StatusConflict:
			return "refs changed — refresh and retry"
		case bitbucket.StatusDiffTooLarge:
			return "merge timed out on Bitbucket — refresh (" +
				a.keymap.Keys(ActRefresh) + ") to see whether it completed"
		default:
		}
	}
	if message := apiErr.UserMessage(); message != "" {
		return message
	}
	return err.Error()
}

// -----------------------------------------------------------------------------
// The shared update command
// -----------------------------------------------------------------------------

// updatePullRequest is the one PUT path: the title editor, the description
// editor, the reviewer picker and the draft toggle all end here.
//
// It always sends the complete reviewer set, because Bitbucket clears the
// reviewers a PUT omits. When the pull request in hand came off a list payload
// that carried none, the set is unknown rather than empty, so the command reads
// the pull request first and sends what the server says.
func (a *App) updatePullRequest(key, verb string, update bitbucket.PullRequestUpdate) tea.Cmd {
	pr := a.pullRequestByKey(key)
	if pr == nil {
		return nil
	}
	ctx, client := a.context(), a.client
	workspace, slug, id := pr.Workspace(), pr.RepoSlug(), pr.ID
	known := pr.ReviewerUUIDs()

	a.beginWrite(key, verb)
	a.helpBar.SetStatusMsg(writeProgressText(verb, id))

	return func() tea.Msg {
		if update.Reviewers == nil {
			if known == nil {
				detail, err := client.GetPullRequest(ctx, workspace, slug, id)
				if err != nil {
					return pullRequestWrittenMsg{key: key, verb: verb, pr: nil, err: err}
				}
				known = detail.ReviewerUUIDs()
			}
			if known == nil {
				known = []string{}
			}
			update.Reviewers = known
		}
		updated, err := client.UpdatePullRequest(ctx, workspace, slug, id, update)
		return pullRequestWrittenMsg{key: key, verb: verb, pr: updated, err: err}
	}
}

// handlePullRequestWritten applies what a write left behind. The pull request is
// patched wherever it is held, by key, so a write finishing after the user has
// moved on still lands; only the on-screen half — the detail pane, the statuses
// and the review data — is gated on the pull request still being previewed.
//
// A pull request that is no longer open also triggers a list refetch: nothing
// filters by state client side, so a merged pull request sits in the Open tab
// until the server is asked again.
func (a *App) handlePullRequestWritten(msg pullRequestWrittenMsg) (tea.Model, tea.Cmd) {
	a.finishWrite(msg.key)
	a.logFlag.Store(false)

	if msg.err != nil {
		a.helpBar.SetStatusMsg(msg.verb + " failed: " + a.writeErrorText(msg.verb, msg.err))
		return a, nil
	}
	if msg.pr == nil {
		if msg.readErr != nil {
			a.helpBar.SetStatusMsg(a.writeDoneAfterReadFailure(msg))
		}
		return a, nil
	}

	pr := msg.pr
	a.statusPanel.SetOnline(true)
	a.prCache[msg.key] = pr
	a.patchMerged(pr)
	a.prList.PatchPullRequest(pr)
	a.refilterTabs("")
	if msg.readErr != nil {
		a.helpBar.SetStatusMsg(a.writeDoneAfterReadFailure(msg))
	} else {
		a.helpBar.SetStatusMsg(writeDoneText(msg.verb, pr.ID))
	}

	var cmds []tea.Cmd
	if msg.key == a.previewKey {
		a.detailView.UpdatePullRequestData(pr)
		if cmd := a.fetchStatuses(pr, a.previewEpoch); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if cmd := a.refreshReviewData(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if pr.State != bitbucket.PullRequestOpen {
		if cmd := a.refreshCmd(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if len(cmds) == 0 {
		return a, nil
	}
	return a, tea.Batch(cmds...)
}

// writeDoneAfterReadFailure is the help-bar line for a write that landed but
// whose read-back failed: the write is done — saying otherwise would send the
// user to redo it — and the refresh key is the way to see the server's copy.
func (a *App) writeDoneAfterReadFailure(msg pullRequestWrittenMsg) string {
	id := msg.id
	if msg.pr != nil {
		id = msg.pr.ID
	}
	return writeDoneText(msg.verb, id) + " — refresh failed: " + a.writeErrorText(msg.verb, msg.readErr) +
		" (" + a.keymap.Keys(ActRefresh) + " to reload)"
}

// -----------------------------------------------------------------------------
// Merge
// -----------------------------------------------------------------------------

// handleActionMerge opens the merge dialog: a strategy, whether to close the
// source branch, and an optional commit message.
func (a *App) handleActionMerge() tea.Cmd {
	pr, ok := a.writeTarget(verbMerge)
	if !ok {
		return nil
	}

	strategies := bitbucket.AllMergeStrategies()
	labels := make([]string, 0, len(strategies))
	for _, strategy := range strategies {
		labels = append(labels, strategy.Label())
	}

	key := pr.Key()
	a.armForm(formMerge, func(msg components.FormConfirmedMsg) tea.Cmd { return a.mergeFromForm(key, msg) })
	a.form.Show(formMerge,
		"Merge #"+strconv.Itoa(pr.ID)+" into "+pr.DestinationBranch(),
		verbMerge,
		[]components.FormField{
			{ID: fieldStrategy, Label: "Strategy", Kind: components.FormChoice, Options: labels},
			{ID: fieldCloseBranch, Label: "Close source branch", Kind: components.FormToggle, On: pr.CloseSourceBranch},
			// The hint belongs in the label: an empty FormText renders as an
			// empty value column, with nowhere to say what empty means.
			{ID: fieldMessage, Label: "Message (empty = default)", Kind: components.FormText},
		})
	return nil
}

// mergeFromForm turns the confirmed dialog into a merge request.
func (a *App) mergeFromForm(key string, msg components.FormConfirmedMsg) tea.Cmd {
	pr := a.pullRequestByKey(key)
	if pr == nil {
		return nil
	}

	opts := bitbucket.MergeOptions{Strategy: "", CloseSourceBranch: nil, Message: ""}
	strategies := bitbucket.AllMergeStrategies()
	for _, field := range msg.Fields {
		switch field.ID {
		case fieldStrategy:
			if field.Index >= 0 && field.Index < len(strategies) {
				opts.Strategy = strategies[field.Index]
			}
		case fieldCloseBranch:
			closeBranch := field.On
			opts.CloseSourceBranch = &closeBranch
		case fieldMessage:
			opts.Message = strings.TrimSpace(field.Value)
		}
	}

	ctx, client := a.context(), a.client
	workspace, slug, id := pr.Workspace(), pr.RepoSlug(), pr.ID
	a.beginWrite(key, verbMerge)
	a.helpBar.SetStatusMsg(writeProgressText(verbMerge, id))

	return func() tea.Msg {
		result, err := client.MergePullRequest(ctx, workspace, slug, id, opts)
		switch {
		case err != nil:
			return pullRequestWrittenMsg{key: key, verb: verbMerge, pr: nil, err: err}
		case result.Queued():
			return mergeQueuedMsg{key: key, workspace: workspace, slug: slug, id: id, taskURL: result.TaskURL, attempt: 0}
		default:
			return pullRequestWrittenMsg{key: key, verb: verbMerge, pr: result.PullRequest, err: nil}
		}
	}
}

// mergePollDelay is how long to wait before the next poll of a queued merge.
// Tests set debounce to a nanosecond so the whole poll chain drains
// synchronously; production leaves it zero and gets the real interval.
func (a *App) mergePollDelay() time.Duration {
	if a.debounce > 0 {
		return a.debounce
	}
	return mergePollInterval
}

// handleMergeQueued polls a merge Bitbucket queued. PENDING re-arms the tick
// with one more attempt spent; SUCCESS turns into the ordinary write result.
// After mergePollMaxAttempts the app stops asking and says so — the merge is
// Bitbucket's to finish, and a refresh will show whether it did.
func (a *App) handleMergeQueued(msg mergeQueuedMsg) (tea.Model, tea.Cmd) {
	if msg.attempt >= mergePollMaxAttempts {
		a.finishWrite(msg.key)
		a.helpBar.SetStatusMsg("merge still running on Bitbucket — refresh (" +
			a.keymap.Keys(ActRefresh) + ") later")
		return a, nil
	}

	ctx, client := a.context(), a.client
	a.logFlag.Store(true)

	// The first poll goes out at once — a merge that finished instantly is
	// already SUCCESS — and the ones after it wait the interval.
	delay := a.mergePollDelay()
	if msg.attempt == 0 {
		delay = 0
	}
	return a, tea.Tick(delay, func(time.Time) tea.Msg {
		task, err := client.GetMergeTaskStatus(ctx, msg.taskURL)
		switch {
		case err != nil:
			return pullRequestWrittenMsg{key: msg.key, verb: verbMerge, pr: nil, err: err}
		case task.Status != bitbucket.MergeTaskSuccess:
			next := msg
			next.attempt++
			return next
		case task.MergeResult != nil:
			return pullRequestWrittenMsg{key: msg.key, verb: verbMerge, pr: task.MergeResult, err: nil}
		default:
			// SUCCESS without merge_result: the merge is done, so stop polling
			// and read the pull request back rather than spin to exhaustion.
			pr, err := client.GetPullRequest(ctx, msg.workspace, msg.slug, msg.id)
			return pullRequestWrittenMsg{key: msg.key, verb: verbMerge, pr: pr, err: err}
		}
	})
}

// -----------------------------------------------------------------------------
// Sync with the target branch
// -----------------------------------------------------------------------------

// syncParams is everything the sync command needs, snapshotted off the App
// before the goroutine starts.
type syncParams struct {
	key         string
	root        string
	workspace   string
	slug        string
	source      string
	destination string
	rebase      bool
	push        bool
	checkoutKey string
}

// handleActionSync opens the sync dialog after the two preconditions that can
// be checked without shelling out: there has to be a checkout, and it has to be
// on the pull request's source branch. The rest — a clean tree, a remote, the
// fetch itself — is checked inside the command, live, because cached state can
// be minutes old.
func (a *App) handleActionSync() tea.Cmd {
	pr, ok := a.writeTarget(verbSync)
	if !ok {
		return nil
	}
	if a.gitRepoRoot == "" {
		a.helpBar.SetStatusMsg("not inside a git repository")
		return nil
	}
	if a.gitBranch != "" && a.gitBranch != pr.SourceBranch() {
		a.helpBar.SetStatusMsg("check out the source branch first (" + a.keymap.Keys(ActCheckout) + ")")
		return nil
	}
	if isFork(pr) {
		// The source lives in another repository: fetching the target from
		// one remote and pushing the source to another is not something this
		// flow does, and pushing to the wrong one would be worse than nothing.
		a.helpBar.SetStatusMsg("cannot sync: the source branch is in a fork")
		return nil
	}

	key := pr.Key()
	a.armForm(formSync, func(msg components.FormConfirmedMsg) tea.Cmd { return a.syncFromForm(key, msg) })
	a.form.Show(formSync,
		"Sync "+pr.SourceBranch()+" with "+pr.DestinationBranch(),
		verbSync,
		[]components.FormField{
			{
				ID: fieldMethod, Label: "Method", Kind: components.FormChoice,
				Options: []string{syncMethodMerge, syncMethodRebase},
			},
			{ID: fieldPush, Label: "Push", Kind: components.FormToggle, On: true},
		})
	return nil
}

// syncFromForm turns the confirmed dialog into the local git run.
func (a *App) syncFromForm(key string, msg components.FormConfirmedMsg) tea.Cmd {
	pr := a.pullRequestByKey(key)
	if pr == nil {
		return nil
	}

	params := syncParams{
		key:         key,
		root:        a.gitRepoRoot,
		workspace:   pr.Workspace(),
		slug:        pr.RepoSlug(),
		source:      pr.SourceBranch(),
		destination: pr.DestinationBranch(),
		rebase:      false,
		push:        false,
		checkoutKey: a.keymap.Keys(ActCheckout),
	}
	for _, field := range msg.Fields {
		switch field.ID {
		case fieldMethod:
			params.rebase = field.Index == 1
		case fieldPush:
			params.push = field.On
		}
	}

	a.beginWrite(key, verbSync)
	a.helpBar.SetStatusMsg("syncing " + params.source + "…")

	return func() tea.Msg { return runSync(params) }
}

// runSync brings the checkout up to date with the pull request's target branch
// and, when asked, pushes the result. It runs off the event loop: every step
// shells out to git, and a fetch over a slow link would otherwise freeze the UI.
//
// A conflict is not left in the tree — pkg/git aborts before reporting one — so
// the answer is a message rather than a half-finished merge.
func runSync(p syncParams) syncDoneMsg {
	failed := func(err error) syncDoneMsg {
		return syncDoneMsg{key: p.key, summary: "", branch: "", pushed: false, err: err}
	}

	branch, err := git.CurrentBranch(p.root)
	if err != nil {
		return failed(err)
	}
	if branch != p.source {
		return failed(errors.New("check out the source branch first (" + p.checkoutKey + ")"))
	}
	clean, err := git.IsClean(p.root)
	if err != nil {
		return failed(err)
	}
	if !clean {
		return failed(errors.New("working tree has uncommitted changes"))
	}

	remote, ok := git.RemoteFor(p.root, p.workspace, p.slug)
	if !ok {
		return failed(errors.New("no git remote for " + p.workspace + "/" + p.slug + " in this checkout"))
	}
	if err := git.Fetch(p.root, remote, p.destination); err != nil {
		return failed(err)
	}
	if err := refuseIfRemoteAhead(p.root, remote, p.source); err != nil {
		return failed(err)
	}

	ref := remote + "/" + p.destination
	method := syncMethodMerge
	if p.rebase {
		method = syncMethodRebase
		err = git.RebaseOnto(p.root, ref)
	} else {
		err = git.MergeBranch(p.root, ref)
	}
	if errors.Is(err, git.ErrConflict) {
		return failed(errors.New("conflicts with " + p.destination + " — resolve manually (aborted)"))
	}
	if err != nil {
		return failed(err)
	}

	summary := "synced " + p.source + " with " + p.destination + " (" + method + ")"
	if p.push {
		if err := git.Push(p.root, remote, p.rebase); err != nil {
			return failed(err)
		}
		summary += ", pushed"
	}

	// The branch is re-read rather than assumed: a rebase that stopped and was
	// aborted, or a hook that moved HEAD, would leave the app pointing at a
	// branch it is not on any more.
	if current, err := git.CurrentBranch(p.root); err == nil && current != "" {
		branch = current
	}
	return syncDoneMsg{key: p.key, summary: summary, branch: branch, pushed: p.push, err: nil}
}

// refuseIfRemoteAhead refreshes the remote copy of the source branch and
// refuses when it holds commits this checkout does not: a rebase followed by a
// force-with-lease push would otherwise discard them, because the lease only
// checks that the remote is where the checkout last saw it — which the fetch
// just made true. A branch that was never pushed has nothing to compare.
func refuseIfRemoteAhead(root, remote, source string) error {
	err := git.Fetch(root, remote, source)
	if errors.Is(err, git.ErrNoRemoteBranch) {
		return nil
	}
	if err != nil {
		return err
	}
	behind, err := git.IsAncestor(root, remote+"/"+source, "HEAD")
	if err != nil {
		return err
	}
	if !behind {
		return errors.New("remote " + source + " has commits this checkout lacks — pull first")
	}
	return nil
}

// isFork reports whether the pull request's source branch lives in a different
// repository from its destination.
func isFork(pr *bitbucket.PullRequest) bool {
	src, dst := pr.Source.Repository.FullName, pr.Destination.Repository.FullName
	return src != "" && dst != "" && !strings.EqualFold(src, dst)
}

// handleSyncDone reports the run and, when it pushed, invalidates that pull
// request: the source commit has moved, which is what the review caches are
// stamped against. Like every write result the invalidation goes by key, so a
// sync finishing after the user has moved on still takes effect; only the
// refetch is gated on the pull request still being previewed.
func (a *App) handleSyncDone(msg syncDoneMsg) (tea.Model, tea.Cmd) {
	a.finishWrite(msg.key)
	a.logFlag.Store(false)

	if msg.err != nil {
		a.helpBar.SetStatusMsg(verbSync + " failed: " + msg.err.Error())
		return a, nil
	}
	if msg.branch != "" {
		a.gitBranch = msg.branch
	}
	a.helpBar.SetStatusMsg(msg.summary)

	if !msg.pushed {
		return a, nil
	}
	// The head moved for the pull request that was synced, not for whichever one
	// the cursor is on now, so the caches are dropped by key whether or not it
	// is still on screen — the review data is stamped against a source commit
	// that no longer exists, and nothing re-checks it on the way back. The pull
	// request is resolved first because the fetches below only need its
	// workspace, slug and id, all of which survive the stale copy.
	pr := a.pullRequestByKey(msg.key)
	delete(a.prCache, msg.key)
	delete(a.statusCache, msg.key)
	a.dropReviewCaches(msg.key)

	if pr == nil || msg.key != a.previewKey {
		return a, nil
	}
	a.previewEpoch++
	a.logFlag.Store(true)

	cmds := []tea.Cmd{
		a.fetchStatuses(pr, a.previewEpoch),
		a.fetchPullRequestDetail(pr, a.previewEpoch),
	}
	if cmd := a.refreshReviewData(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	return a, tea.Batch(cmds...)
}

// -----------------------------------------------------------------------------
// Edit title and description
// -----------------------------------------------------------------------------

// handleActionEdit opens the chooser: the title is edited inline, the
// description in the configured editor.
func (a *App) handleActionEdit() tea.Cmd {
	pr, ok := a.writeTarget(verbEdit)
	if !ok {
		return nil
	}
	key := pr.Key()
	a.onSelect = func(item components.ModalItem) tea.Cmd {
		switch item.ID {
		case editItemTitle:
			return a.editTitle(key)
		case editItemDescription:
			return a.editDescription(key)
		}
		return nil
	}
	a.modal.Show("Edit #"+strconv.Itoa(pr.ID), []components.ModalItem{
		{ID: editItemTitle, Label: "Title"},
		{ID: editItemDescription, Label: "Description"},
	})
	return nil
}

// editTitle opens the inline input prefilled with the current title.
func (a *App) editTitle(key string) tea.Cmd {
	pr := a.pullRequestByKey(key)
	if pr == nil {
		return nil
	}
	original := pr.Title
	a.onInput = func(text string) tea.Cmd { return a.submitTitle(key, original, text) }
	a.input.SetSize(a.width, a.height)
	a.input.Show("Title", original)
	return nil
}

// submitTitle sends a title the user actually changed. An empty one is refused
// here rather than by the server, which answers the same thing more slowly.
func (a *App) submitTitle(key, original, text string) tea.Cmd {
	title := strings.TrimSpace(text)
	if title == "" {
		a.helpBar.SetStatusMsg("title cannot be empty")
		return nil
	}
	if title == strings.TrimSpace(original) {
		a.helpBar.SetStatusMsg("title unchanged")
		return nil
	}
	return a.updatePullRequest(key, verbEdit,
		bitbucket.PullRequestUpdate{Title: &title, Description: nil, Draft: nil, Reviewers: nil})
}

// editDescription writes the current description to a temp file and suspends
// into the configured editor. The file is read back, compared and deleted when
// the editor exits; see handleDescriptionEdited.
func (a *App) editDescription(key string) tea.Cmd {
	pr := a.pullRequestByKey(key)
	if pr == nil {
		return nil
	}
	original := pr.Description
	path, err := writeDescriptionTempFile(pr.RepoSlug(), pr.ID, original)
	if err != nil {
		a.statusPanel.SetError("description: " + err.Error())
		return nil
	}
	cmd, err := a.buildTempFileEditorCmd(path)
	if err != nil {
		_ = os.Remove(path)
		a.statusPanel.SetError(err.Error())
		return nil
	}

	a.descriptionEditPath = path
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return tea.ExecProcess(cmd, func(execErr error) tea.Msg {
		return descriptionEditedMsg{key: key, path: path, original: original, err: execErr}
	})
}

// writeDescriptionTempFile puts the description somewhere an editor can open
// it. The name carries the repository and the id so a stray file left behind by
// a crash says what it was.
func writeDescriptionTempFile(slug string, id int, body string) (string, error) {
	file, err := os.CreateTemp("", "lazybitbucket-"+slug+"-"+strconv.Itoa(id)+"-*.md")
	if err != nil {
		return "", err
	}
	path := file.Name()
	_, writeErr := file.WriteString(body)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(path)
		return "", errors.Join(writeErr, closeErr)
	}
	return path, nil
}

// handleDescriptionEdited reads the temp file back, deletes it and sends the
// description on when it actually changed. A trailing newline an editor added on
// save is not a change.
func (a *App) handleDescriptionEdited(msg descriptionEditedMsg) (tea.Model, tea.Cmd) {
	// Mouse reporting is off until this returns: the child terminal turned it
	// off and only the app can turn it back on.
	cmds := []tea.Cmd{tea.EnableMouseCellMotion}
	edited, err := a.readEditedDescription()
	a.discardDescriptionTempFile()

	switch {
	case msg.err != nil:
		a.statusPanel.SetError("editor: " + msg.err.Error())
		return a, tea.Batch(cmds...)
	case err != nil:
		a.statusPanel.SetError("description: " + err.Error())
		return a, tea.Batch(cmds...)
	}

	body := strings.TrimRight(edited, "\n")
	if body == strings.TrimRight(msg.original, "\n") {
		a.helpBar.SetStatusMsg("description unchanged")
		return a, tea.Batch(cmds...)
	}
	if cmd := a.updatePullRequest(msg.key, verbEdit,
		bitbucket.PullRequestUpdate{Title: nil, Description: &body, Draft: nil, Reviewers: nil}); cmd != nil {
		cmds = append(cmds, cmd)
	}
	return a, tea.Batch(cmds...)
}

// readEditedDescription reads the description temp file back. The path is taken
// off the App, which is where this process recorded the file it created, rather
// than off anything that travelled through a message.
func (a *App) readEditedDescription() (string, error) {
	path := a.descriptionEditPath
	if path == "" {
		return "", errors.New("no description edit in progress")
	}
	data, err := os.ReadFile(filepath.Clean(path))
	return string(data), err
}

// discardDescriptionTempFile deletes the scratch file and forgets it. A failure
// to delete is not worth reporting: it is a temp file, and the operating system
// clears the directory eventually.
func (a *App) discardDescriptionTempFile() {
	if a.descriptionEditPath != "" {
		_ = os.Remove(a.descriptionEditPath)
		a.descriptionEditPath = ""
	}
}

// -----------------------------------------------------------------------------
// Reviewers
// -----------------------------------------------------------------------------

// handleActionReviewers fetches the candidate accounts and opens the picker once
// they are in. Neither candidate listing is required: a token that cannot see
// the repository's default reviewers or the workspace's members still gets a
// picker over the pull request's own reviewers, with a note saying what is
// missing.
//
// The one thing the picker cannot do without is the current reviewer set. What
// it hands back is the complete desired set, so a seed built from a list payload
// that carried no reviewers would show the real ones unchecked and confirming
// would clear them. A nil reviewer set means unknown rather than empty — the
// same distinction bitbucket.ErrReviewersUnknown draws — so it is read from the
// server first, and a read that fails opens no picker at all.
func (a *App) handleActionReviewers() tea.Cmd {
	pr, ok := a.writeTarget(verbReviewers)
	if !ok {
		return nil
	}

	key := pr.Key()
	author := pr.Author.UUID
	ctx, client := a.context(), a.client
	workspace, slug, id := pr.Workspace(), pr.RepoSlug(), pr.ID
	needDetail := pr.Reviewers == nil
	var seed []reviewerCandidate
	if !needDetail {
		seed = currentReviewerCandidates(pr)
	}
	a.helpBar.SetStatusMsg("loading reviewers…")
	a.logFlag.Store(true)

	return func() tea.Msg {
		var detail *bitbucket.PullRequest
		if needDetail {
			loaded, err := client.GetPullRequest(ctx, workspace, slug, id)
			if err != nil {
				return reviewerCandidatesMsg{key: key, pr: nil, candidates: nil, note: "", err: err}
			}
			detail = loaded
			seed = currentReviewerCandidates(loaded)
			author = loaded.Author.UUID
		}

		var notes []string
		defaults, err := client.ListEffectiveDefaultReviewers(ctx, workspace, slug)
		if note := candidateNote("default reviewers", err); note != "" {
			notes = append(notes, note)
		}
		members, err := client.ListWorkspaceMembers(ctx, workspace,
			bitbucket.ListOptions{MaxItems: reviewerCandidateLimit})
		if note := candidateNote("workspace members", err); note != "" {
			notes = append(notes, note)
		}
		return reviewerCandidatesMsg{
			key:        key,
			pr:         detail,
			candidates: mergeReviewerCandidates(seed, defaults, members, author),
			note:       strings.Join(notes, "; "),
			err:        nil,
		}
	}
}

// currentReviewerCandidates is the pull request's own side of the picker: the
// requested reviewers, then anyone else already participating as one. Both start
// checked. It is called either on the event loop, over the previewed pull
// request, or inside the command over the detail that command just fetched and
// nothing else holds — never over a pull request another goroutine is writing.
func currentReviewerCandidates(pr *bitbucket.PullRequest) []reviewerCandidate {
	out := make([]reviewerCandidate, 0, len(pr.Reviewers)+len(pr.Participants))
	for _, reviewer := range pr.Reviewers {
		out = append(out, reviewerCandidate{
			uuid: reviewer.UUID, label: reviewer.Name(), group: groupReviewer, selected: true,
		})
	}
	for _, participant := range pr.Participants {
		if participant.Role != bitbucket.RoleReviewer {
			continue
		}
		out = append(out, reviewerCandidate{
			uuid: participant.User.UUID, label: participant.User.Name(), group: groupReviewer, selected: true,
		})
	}
	return out
}

// mergeReviewerCandidates puts the four sources in picker order and drops the
// duplicates: current reviewers, default reviewers, then workspace members. The
// author is never a candidate — Bitbucket rejects an update that makes them one.
func mergeReviewerCandidates(
	seed []reviewerCandidate, defaults, members []bitbucket.Account, author string,
) []reviewerCandidate {
	out := make([]reviewerCandidate, 0, len(seed)+len(defaults)+len(members))
	seen := make(map[string]bool, len(seed)+len(defaults)+len(members))
	add := func(candidate reviewerCandidate) {
		if candidate.uuid == "" || candidate.uuid == author || seen[candidate.uuid] {
			return
		}
		seen[candidate.uuid] = true
		out = append(out, candidate)
	}

	for _, candidate := range seed {
		add(candidate)
	}
	for _, account := range defaults {
		add(reviewerCandidate{
			uuid: account.UUID, label: account.Name(), group: groupDefaultReviewer, selected: false,
		})
	}
	for _, account := range members {
		add(reviewerCandidate{
			uuid: account.UUID, label: account.Name(), group: groupMember, selected: false,
		})
	}
	return out
}

// candidateNote is the one-line degradation note for a candidate source that
// could not be listed. A 403 is the expected one — a token without workspace
// scope — and is named as such so the reader knows it is a permission, not an
// outage.
func candidateNote(what string, err error) string {
	switch {
	case err == nil:
		return ""
	case bitbucket.IsForbidden(err):
		return what + " unavailable (403)"
	default:
		return what + " unavailable"
	}
}

// handleReviewerCandidates opens the picker, unless the user has walked off the
// pull request or opened something else while the candidates were loading.
func (a *App) handleReviewerCandidates(msg reviewerCandidatesMsg) (tea.Model, tea.Cmd) {
	a.logFlag.Store(false)

	// A pull request read to establish the reviewer set is kept whatever
	// happens next: it is newer than the row it was read for, and dropping it
	// would cost the same request again.
	if msg.pr != nil {
		a.prCache[msg.key] = msg.pr
		a.patchMerged(msg.pr)
		a.prList.PatchPullRequest(msg.pr)
		a.refilterTabs("")
		if msg.key == a.previewKey {
			a.detailView.UpdatePullRequestData(msg.pr)
		}
	}

	pr := a.currentPullRequest()
	if pr == nil || pr.Key() != msg.key || a.overlayVisible() {
		return a, nil
	}
	if msg.err != nil {
		a.helpBar.SetStatusMsg(verbReviewers + " failed: " + a.writeErrorText(verbReviewers, msg.err))
		return a, nil
	}
	if len(msg.candidates) == 0 {
		a.helpBar.SetStatusMsg("no reviewer candidates available")
		return a, nil
	}

	items := make([]components.ModalItem, 0, len(msg.candidates))
	selected := make(map[string]bool, len(msg.candidates))
	var current []string
	for _, candidate := range msg.candidates {
		items = append(items, components.ModalItem{
			ID: candidate.uuid, Label: candidate.label + "  · " + candidate.group,
		})
		if candidate.selected {
			selected[candidate.uuid] = true
			current = append(current, candidate.uuid)
		}
	}

	key := msg.key
	a.onChecklist = func(picked []components.ModalItem) tea.Cmd { return a.saveReviewers(key, current, picked) }
	// The note goes out before the picker: the modal intercepts keys, so
	// nothing clears the status line until the picker closes again.
	if msg.note != "" {
		a.helpBar.SetStatusMsg(msg.note)
	}
	a.modal.ShowChecklist("Reviewers for #"+strconv.Itoa(pr.ID), items, selected)
	return a, nil
}

// saveReviewers sends the checked set, which is the complete desired reviewer
// list. An unchanged set is not worth a request.
func (a *App) saveReviewers(key string, current []string, picked []components.ModalItem) tea.Cmd {
	uuids := make([]string, 0, len(picked))
	for _, item := range picked {
		if item.ID != "" {
			uuids = append(uuids, item.ID)
		}
	}
	if sameUUIDSet(current, uuids) {
		a.helpBar.SetStatusMsg("reviewers unchanged")
		return nil
	}
	return a.updatePullRequest(key, verbReviewers,
		bitbucket.PullRequestUpdate{Title: nil, Description: nil, Draft: nil, Reviewers: uuids})
}

// sameUUIDSet compares two reviewer sets as sets: the picker sorts checked rows
// first, so the order it hands back says nothing.
func sameUUIDSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	left, right := slices.Clone(a), slices.Clone(b)
	slices.Sort(left)
	slices.Sort(right)
	return slices.Equal(left, right)
}

// -----------------------------------------------------------------------------
// Draft
// -----------------------------------------------------------------------------

// handleActionToggleDraft asks before flipping the draft flag. The dialog is a
// form of one note row, so the only keys are enter and esc.
func (a *App) handleActionToggleDraft() tea.Cmd {
	pr, ok := a.writeTarget(verbToggleDraft)
	if !ok {
		return nil
	}

	draft := !pr.Draft
	title := "Mark #" + strconv.Itoa(pr.ID) + " as draft?"
	if !draft {
		title = "Mark #" + strconv.Itoa(pr.ID) + " ready for review?"
	}

	key := pr.Key()
	a.armForm(formDraft, func(components.FormConfirmedMsg) tea.Cmd {
		return a.updatePullRequest(key, verbToggleDraft,
			bitbucket.PullRequestUpdate{Title: nil, Description: nil, Draft: &draft, Reviewers: nil})
	})
	a.form.Show(formDraft, title, "confirm",
		[]components.FormField{{Kind: components.FormNote, Note: pr.Title}})
	return nil
}

// -----------------------------------------------------------------------------
// Approve
// -----------------------------------------------------------------------------

// handleActionApprove approves the pull request, or withdraws the approval the
// user has already given it — one key, and which of the two it is decided by
// the participant list rather than by a second key.
//
// The direction has to be known before the dialog opens, because the dialog is
// what states it: "Approve #101?" that quietly withdrew an approval would be a
// write the user did not ask for. The pull request is therefore read first and
// the dialog opened on the answer, whatever the cache holds: a list payload
// carries no participants at all, and a cached detail keeps the participants
// it was fetched with while list refreshes move on without it.
func (a *App) handleActionApprove() tea.Cmd {
	pr, ok := a.writeTarget(verbApprove)
	if !ok {
		return nil
	}
	// Bitbucket refuses this with a 400, so there is nothing to gain by asking.
	if a.approvingOwn(pr) {
		a.helpBar.SetStatusMsg(cannotApproveOwn)
		return nil
	}
	// Without an account there is no approval of the user's own to find — an
	// access token is not attached to one — so the key can only ever approve,
	// and there is nothing a read could change about the dialog.
	if a.currentUser != nil {
		return a.fetchApprovalState(pr)
	}

	a.showApprovalDialog(pr)
	return nil
}

// approvingOwn reports whether the pull request is the current user's own. It
// answers false when the account is unknown: an access token has none, and a
// refusal on a guess would take the key away from a session that can use it.
func (a *App) approvingOwn(pr *bitbucket.PullRequest) bool {
	return a.currentUser != nil && pr.IsAuthor(a.currentUser.UUID)
}

// approvedByCurrentUser reports whether the current user's own approval is on
// the pull request. It is false whenever the account is unknown, which makes
// the key an approve-only key for a token that cannot name one.
func (a *App) approvedByCurrentUser(pr *bitbucket.PullRequest) bool {
	if a.currentUser == nil {
		return false
	}
	for _, participant := range pr.Participants {
		if participant.User.UUID != a.currentUser.UUID {
			continue
		}
		return participant.Approved || participant.State == bitbucket.ParticipantApproved
	}
	return false
}

// fetchApprovalState reads the pull request so the dialog can name the right
// direction. It is the reviewer picker's pattern: the read happens before any
// overlay opens, and its answer is worth keeping whatever the user does next.
func (a *App) fetchApprovalState(pr *bitbucket.PullRequest) tea.Cmd {
	key := pr.Key()
	ctx, client := a.context(), a.client
	workspace, slug, id := pr.Workspace(), pr.RepoSlug(), pr.ID

	a.helpBar.SetStatusMsg("checking your approval…")
	a.logFlag.Store(true)

	return func() tea.Msg {
		detail, err := client.GetPullRequest(ctx, workspace, slug, id)
		return approvalStateMsg{key: key, pr: detail, err: err}
	}
}

// handleApprovalState opens the approval dialog over the pull request that was
// just read, unless the user has walked off it or opened something else while
// the read was in flight.
func (a *App) handleApprovalState(msg approvalStateMsg) (tea.Model, tea.Cmd) {
	a.logFlag.Store(false)

	// The detail is newer than the row it was read for, so it is kept whatever
	// happens next — the same bargain the reviewer picker's read strikes.
	if msg.pr != nil {
		a.prCache[msg.key] = msg.pr
		a.patchMerged(msg.pr)
		a.prList.PatchPullRequest(msg.pr)
		a.refilterTabs("")
		if msg.key == a.previewKey {
			a.detailView.UpdatePullRequestData(msg.pr)
		}
	}

	// The guard runs again, because the read took time: the user may have walked
	// off the pull request, opened an overlay, or watched another write land on
	// it while it was in flight.
	pr, ok := a.writeTarget(verbApprove)
	if !ok || pr.Key() != msg.key {
		return a, nil
	}
	if msg.err != nil {
		a.helpBar.SetStatusMsg(verbApprove + " failed: " + a.writeErrorText(verbApprove, msg.err))
		return a, nil
	}
	// The author is re-checked on the fresh copy: a list row can carry a stub
	// account, and the local refusal is only worth anything if it is right.
	if a.approvingOwn(pr) {
		a.helpBar.SetStatusMsg(cannotApproveOwn)
		return a, nil
	}

	a.showApprovalDialog(pr)
	return a, nil
}

// showApprovalDialog asks before either direction. Like the draft dialog it is
// a form of one note row, so the only keys are enter and esc.
func (a *App) showApprovalDialog(pr *bitbucket.PullRequest) {
	remove := a.approvedByCurrentUser(pr)
	title := "Approve #" + strconv.Itoa(pr.ID) + "?"
	if remove {
		title = "Remove your approval from #" + strconv.Itoa(pr.ID) + "?"
	}

	key := pr.Key()
	a.armForm(formApprove, func(components.FormConfirmedMsg) tea.Cmd { return a.setApproval(key, remove) })
	a.form.Show(formApprove, title, "confirm",
		[]components.FormField{{Kind: components.FormNote, Note: pr.Title}})
}

// setApproval sends the confirmed approval and reads the pull request back. The
// approve endpoints answer with a participant rather than a pull request, and
// the list's approval column and the detail pane are drawn from the whole
// participant set, so the fresh pull request is what the write has to hand on.
func (a *App) setApproval(key string, remove bool) tea.Cmd {
	pr := a.pullRequestByKey(key)
	if pr == nil {
		return nil
	}

	verb := verbApprove
	if remove {
		verb = verbUnapprove
	}
	ctx, client := a.context(), a.client
	workspace, slug, id := pr.Workspace(), pr.RepoSlug(), pr.ID
	me := ""
	if a.currentUser != nil {
		me = a.currentUser.UUID
	}

	a.beginWrite(key, verb)
	a.helpBar.SetStatusMsg(writeProgressText(verb, id))

	return func() tea.Msg {
		var (
			participant *bitbucket.Participant
			err         error
		)
		if remove {
			err = client.UnapprovePullRequest(ctx, workspace, slug, id)
		} else {
			participant, err = client.ApprovePullRequest(ctx, workspace, slug, id)
		}
		if err != nil {
			return pullRequestWrittenMsg{key: key, verb: verb, pr: nil, err: err, readErr: nil, id: id}
		}
		updated, err := client.GetPullRequest(ctx, workspace, slug, id)
		if err != nil {
			// The approval is on the server; only the read-back failed. Hand on
			// the cached copy with the participant the server returned, so the
			// list and the detail pane move, and say the refresh is what failed.
			return pullRequestWrittenMsg{key: key, verb: verb,
				pr: approvalPatched(pr, participant, remove, me), err: nil, readErr: err, id: id}
		}
		return pullRequestWrittenMsg{key: key, verb: verb, pr: updated, err: nil, readErr: nil, id: id}
	}
}

// approvalPatched is the cached pull request with the approval applied locally:
// the participant the approve endpoint returned replaces the user's entry (or
// is appended), a withdrawal clears the user's entry. It is nil when the cached
// copy carries no participants — a one-entry set would pose as the whole set —
// or when there is nothing to apply.
func approvalPatched(pr *bitbucket.PullRequest, participant *bitbucket.Participant, remove bool, me string) *bitbucket.PullRequest {
	if pr.Participants == nil {
		return nil
	}
	patched := *pr
	patched.Participants = slices.Clone(pr.Participants)
	if remove {
		for i := range patched.Participants {
			if patched.Participants[i].User.UUID == me {
				patched.Participants[i].Approved = false
				patched.Participants[i].State = ""
				patched.Participants[i].ParticipatedOn = time.Time{}
			}
		}
		return &patched
	}
	if participant == nil {
		return nil
	}
	for i := range patched.Participants {
		if patched.Participants[i].User.UUID == participant.User.UUID {
			patched.Participants[i] = *participant
			return &patched
		}
	}
	patched.Participants = append(patched.Participants, *participant)
	return &patched
}
