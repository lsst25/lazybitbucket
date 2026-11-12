package tui

import (
	"context"
	"os"
	"slices"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/git"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
)

// Opening a pull request is the one write that does not start from a pull
// request: it starts from the branch the terminal is on. N reads what it can —
// the last commit's subject for a title, the repository's default branch for a
// target, its default reviewers, and whether the branch is on the remote at all
// — and asks the rest in one form.
//
// Confirming does not necessarily write straight away. The form's answers can
// each add a step: pushing the branch first, writing the description in an
// editor, picking reviewers from a checklist. They run in that order and end in
// the POST, which is why the answers are held in a pendingCreate rather than
// passed along a chain of messages.

// verbCreate is the create write's name on the help bar.
const verbCreate = "open a pull request"

// formCreate is the create dialog's id, which is what tells its confirmation
// from another dialog's.
const formCreate = "create"

// The create form's field ids.
const (
	fieldTitleCreate  = "title"
	fieldDescCreate   = "description"
	fieldTargetCreate = "target"
	fieldReviewers    = "reviewers"
	fieldDraftCreate  = "draft"
	fieldPushCreate   = "push"
)

// The description choices: nothing, or the editor the description editor uses.
const (
	descNone   = "none"
	descEditor = "write in $EDITOR"
)

// The reviewer choices. The default-reviewer option is only offered when the
// repository has some, and its label carries the count.
const (
	reviewersNone    = "none"
	reviewersDefault = "default reviewers"
	reviewersPick    = "pick…"
)

// noRepositoryHere is what N answers where there is no branch to open a pull
// request for: outside a checkout, or in a checkout of a repository the
// configuration does not name.
const (
	noCheckout   = "not a git checkout: nothing to open a pull request from"
	noRepoMatch  = "this checkout is not one of the configured repositories"
	noBranchHere = "no branch checked out (detached HEAD)"
)

// pendingCreate is a pull request being opened: everything the form settled,
// plus the steps still owed before the POST. Each step clears its own flag, so
// advanceCreate can run them in order without a second state machine.
type pendingCreate struct {
	workspace string
	slug      string
	source    string
	target    string
	title     string
	body      string
	draft     bool
	reviewers []string

	// push publishes the branch before the pull request is opened: Bitbucket
	// refuses a source branch its own repository does not have.
	push   bool
	remote string
	// editBody opens the description in the configured editor first.
	editBody bool
	// pickReviewers opens the reviewer checklist first.
	pickReviewers bool
	// defaults are the repository's effective default reviewers, which the
	// form's "default reviewers" choice selects and the picker starts from.
	defaults []bitbucket.Account
}

// createPrefillMsg carries what the create form is filled in from. A failure to
// read any one of them is not fatal: the form opens with what did arrive and
// says what did not.
type createPrefillMsg struct {
	workspace string
	slug      string
	source    string
	target    string
	title     string
	remote    string
	onRemote  bool
	defaults  []bitbucket.Account
	note      string
	err       error
}

// branchPushedMsg is the push a create needed, finished.
type branchPushedMsg struct{ err error }

// createDescriptionEditedMsg is the editor exiting on a new pull request's
// description.
type createDescriptionEditedMsg struct {
	path string
	err  error
}

// pullRequestCreatedMsg is the created pull request, or why it could not be.
type pullRequestCreatedMsg struct {
	pr *bitbucket.PullRequest
	// slug is the repository the create was sent to, which is the name the
	// list holds that repository's rows under.
	slug string
	err  error
}

// handleActionCreate starts the flow: the guards, then one command that reads
// everything the form is prefilled from.
func (a *App) handleActionCreate() tea.Cmd {
	if a.view == viewPipelines {
		a.helpBar.SetStatusMsg(noPullRequestHere)
		return nil
	}
	if a.readOnly {
		a.helpBar.SetStatusMsg("read-only mode: cannot " + verbCreate)
		return nil
	}
	if a.overlayVisible() || a.pendingCreate != nil {
		return nil
	}

	workspace, slug, ok := a.createRepository()
	if !ok {
		return nil
	}
	if a.gitBranch == "" {
		a.helpBar.SetStatusMsg(noBranchHere)
		return nil
	}

	root, branch := a.gitRepoRoot, a.gitBranch
	ctx, client := a.context(), a.client
	a.helpBar.SetStatusMsg("reading " + branch + "…")
	a.logFlag.Store(true)

	return func() tea.Msg {
		return a.readCreatePrefill(ctx, client, workspace, slug, root, branch)
	}
}

// readCreatePrefill is the one round of reads behind the form: git for the
// title and whether the branch is published, the API for the default branch and
// the default reviewers.
func (a *App) readCreatePrefill(
	ctx context.Context, client bitbucket.ClientInterface, workspace, slug, root, branch string,
) tea.Msg {
	prefill := createPrefillMsg{
		workspace: workspace, slug: slug, source: branch,
		target: "", title: "", remote: "", onRemote: false,
		defaults: nil, note: "", err: nil,
	}

	if subject, err := git.LastCommitSubject(root); err == nil {
		prefill.title = subject
	}
	if prefill.title == "" {
		// A branch with no commit of its own still deserves a name to edit.
		prefill.title = branch
	}
	prefill.remote, _ = git.RemoteFor(root, workspace, slug)
	prefill.onRemote = prefill.remote != "" &&
		git.IsRemoteBranch(root, prefill.remote+"/"+branch)

	var notes []string
	repository, err := client.GetRepository(ctx, workspace, slug)
	switch {
	case err != nil:
		notes = append(notes, "default branch unavailable")
	default:
		prefill.target = repository.MainBranchName()
	}

	defaults, err := client.ListEffectiveDefaultReviewers(ctx, workspace, slug)
	if note := candidateNote("default reviewers", err); note != "" {
		notes = append(notes, note)
	}
	prefill.defaults = defaults
	prefill.note = strings.Join(notes, "; ")
	return prefill
}

// createRepository is the repository a pull request would be opened in: the one
// this checkout's remotes name, among the configured ones. It reports the
// refusal on the help bar itself.
func (a *App) createRepository() (workspace, slug string, ok bool) {
	if a.gitRepoRoot == "" {
		a.helpBar.SetStatusMsg(noCheckout)
		return "", "", false
	}
	for _, repo := range a.repos {
		full := strings.ToLower(repo.FullName)
		if full == "" {
			full = strings.ToLower(a.workspace + "/" + repo.Slug)
		}
		if slices.Contains(a.gitRemoteRepos, full) {
			return a.workspace, repo.Slug, true
		}
	}
	a.helpBar.SetStatusMsg(noRepoMatch)
	return "", "", false
}

// handleCreatePrefill opens the form on what the reads found.
func (a *App) handleCreatePrefill(msg createPrefillMsg) (tea.Model, tea.Cmd) {
	a.logFlag.Store(false)
	if a.overlayVisible() || a.pendingCreate != nil {
		return a, nil
	}
	if msg.err != nil {
		a.helpBar.SetStatusMsg(verbCreate + " failed: " + a.writeErrorText(verbCreate, msg.err))
		return a, nil
	}

	a.pendingCreate = &pendingCreate{
		workspace: msg.workspace, slug: msg.slug, source: msg.source, target: msg.target,
		title: msg.title, body: "", draft: false, reviewers: nil,
		push: false, remote: msg.remote, editBody: false, pickReviewers: false,
		defaults: msg.defaults,
	}

	a.armForm(formCreate, func(confirmed components.FormConfirmedMsg) tea.Cmd {
		return a.createFromForm(confirmed)
	})
	a.form.SetSize(a.width, a.height)
	a.form.Show(formCreate, "New pull request in "+msg.slug, "Create",
		createFormFields(msg))
	if msg.note != "" {
		a.helpBar.SetStatusMsg(msg.note)
	}
	return a, nil
}

// createFormFields is the form itself. The push row is only there when the
// branch is not on the remote yet, because a toggle that has nothing to do is a
// question the author should not have to answer.
func createFormFields(msg createPrefillMsg) []components.FormField {
	reviewerOptions := []string{reviewersNone, reviewersPick}
	if len(msg.defaults) > 0 {
		reviewerOptions = append([]string{
			reviewersDefault + " (" + strconv.Itoa(len(msg.defaults)) + ")",
		}, reviewerOptions...)
	}

	fields := []components.FormField{
		{
			ID: fieldTitleCreate, Label: "Title", Kind: components.FormText,
			Value: msg.title, Options: nil, Index: 0, On: false, Note: "",
		},
		{
			ID: fieldDescCreate, Label: "Description", Kind: components.FormChoice,
			Options: []string{descNone, descEditor}, Index: 0, Value: "", On: false, Note: "",
		},
		{
			ID: fieldTargetCreate, Label: "Target", Kind: components.FormText,
			Value: msg.target, Options: nil, Index: 0, On: false, Note: "",
		},
		{
			ID: fieldReviewers, Label: "Reviewers", Kind: components.FormChoice,
			Options: reviewerOptions, Index: 0, Value: "", On: false, Note: "",
		},
		{
			ID: fieldDraftCreate, Label: "Draft", Kind: components.FormToggle,
			On: false, Options: nil, Index: 0, Value: "", Note: "",
		},
	}
	if !msg.onRemote && msg.remote != "" {
		fields = append(fields, components.FormField{
			ID: fieldPushCreate, Label: "Push the branch first", Kind: components.FormToggle,
			On: true, Options: nil, Index: 0, Value: "", Note: "",
		})
	}
	return append(fields, components.FormField{
		ID: "source", Label: "", Kind: components.FormNote, Note: "from " + msg.source,
		Options: nil, Index: 0, On: false, Value: "",
	})
}

// createFromForm reads the confirmed form into the pending create and runs
// whatever it asked for.
func (a *App) createFromForm(msg components.FormConfirmedMsg) tea.Cmd {
	pending := a.pendingCreate
	if pending == nil || msg.ID != formCreate {
		return nil
	}

	for _, field := range msg.Fields {
		switch field.ID {
		case fieldTitleCreate:
			pending.title = strings.TrimSpace(field.Value)
		case fieldTargetCreate:
			pending.target = strings.TrimSpace(field.Value)
		case fieldDescCreate:
			pending.editBody = field.Options[field.Index] == descEditor
		case fieldReviewers:
			a.applyReviewerChoice(pending, field.Options[field.Index])
		case fieldDraftCreate:
			pending.draft = field.On
		case fieldPushCreate:
			pending.push = field.On
		}
	}

	if pending.title == "" {
		a.pendingCreate = nil
		a.helpBar.SetStatusMsg("title cannot be empty")
		return nil
	}
	if pending.target != "" && pending.target == pending.source {
		a.pendingCreate = nil
		a.helpBar.SetStatusMsg("a branch cannot be opened against itself")
		return nil
	}
	return a.advanceCreate()
}

// applyReviewerChoice turns the form's reviewer row into either a set or a step.
func (a *App) applyReviewerChoice(pending *pendingCreate, choice string) {
	switch {
	case choice == reviewersPick:
		pending.pickReviewers = true
	case strings.HasPrefix(choice, reviewersDefault):
		pending.reviewers = accountUUIDs(pending.defaults)
	default:
		pending.reviewers = nil
	}
}

// accountUUIDs is the uuid of every account, which is all a create body carries.
func accountUUIDs(accounts []bitbucket.Account) []string {
	uuids := make([]string, 0, len(accounts))
	for _, account := range accounts {
		if account.UUID != "" {
			uuids = append(uuids, account.UUID)
		}
	}
	return uuids
}

// advanceCreate runs the next step the form asked for, and the POST once there
// are none left.
func (a *App) advanceCreate() tea.Cmd {
	pending := a.pendingCreate
	if pending == nil {
		return nil
	}
	switch {
	case pending.push:
		pending.push = false
		return a.pushCreateBranch(pending)
	case pending.editBody:
		pending.editBody = false
		return a.editCreateDescription(pending)
	case pending.pickReviewers:
		pending.pickReviewers = false
		return a.pickCreateReviewers(pending)
	default:
		return a.submitCreate(pending)
	}
}

// pushCreateBranch publishes the branch, because Bitbucket cannot open a pull
// request for a branch it does not have.
func (a *App) pushCreateBranch(pending *pendingCreate) tea.Cmd {
	root, remote := a.gitRepoRoot, pending.remote
	if root == "" || remote == "" {
		return a.advanceCreate()
	}
	a.helpBar.SetStatusMsg("pushing " + pending.source + " to " + remote + "…")
	a.logFlag.Store(true)
	return func() tea.Msg {
		return branchPushedMsg{err: git.Push(root, remote, false)}
	}
}

// handleBranchPushed carries on, or gives up with the push's own message.
func (a *App) handleBranchPushed(msg branchPushedMsg) (tea.Model, tea.Cmd) {
	a.logFlag.Store(false)
	if msg.err != nil {
		a.pendingCreate = nil
		a.helpBar.SetStatusMsg("push failed: " + msg.err.Error())
		return a, nil
	}
	return a, a.advanceCreate()
}

// editCreateDescription opens an empty temp file in the configured editor, the
// way editing a description does.
func (a *App) editCreateDescription(pending *pendingCreate) tea.Cmd {
	path, err := writeDescriptionTempFile(pending.slug, 0, "")
	if err != nil {
		a.statusPanel.SetError("description: " + err.Error())
		return a.advanceCreate()
	}
	cmd, err := a.buildTempFileEditorCmd(path)
	if err != nil {
		_ = os.Remove(path)
		a.statusPanel.SetError(err.Error())
		return a.advanceCreate()
	}

	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return tea.ExecProcess(cmd, func(execErr error) tea.Msg {
		return createDescriptionEditedMsg{path: path, err: execErr}
	})
}

// handleCreateDescriptionEdited takes what the editor left and carries on. A
// file the editor could not write is not worth abandoning the pull request
// over: the description is the one part that can still be edited afterwards.
func (a *App) handleCreateDescriptionEdited(msg createDescriptionEditedMsg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{tea.EnableMouseCellMotion}
	if a.pendingCreate == nil {
		_ = os.Remove(msg.path)
		return a, tea.Batch(cmds...)
	}

	if msg.err != nil {
		a.statusPanel.SetError("editor: " + msg.err.Error())
	} else if body, err := os.ReadFile(msg.path); err == nil { //nolint:gosec // the app wrote this path
		a.pendingCreate.body = strings.TrimRight(string(body), "\n")
	} else {
		a.statusPanel.SetError("description: " + err.Error())
	}
	_ = os.Remove(msg.path)

	if next := a.advanceCreate(); next != nil {
		cmds = append(cmds, next)
	}
	return a, tea.Batch(cmds...)
}

// pickCreateReviewers opens the checklist over the repository's default
// reviewers and the workspace members, with the defaults checked.
func (a *App) pickCreateReviewers(pending *pendingCreate) tea.Cmd {
	ctx, client := a.context(), a.client
	workspace, defaults := pending.workspace, pending.defaults
	author := ""
	if a.currentUser != nil {
		author = a.currentUser.UUID
	}
	a.helpBar.SetStatusMsg("loading reviewers…")
	a.logFlag.Store(true)

	return func() tea.Msg {
		members, err := client.ListWorkspaceMembers(ctx, workspace,
			bitbucket.ListOptions{MaxItems: reviewerCandidateLimit})
		seed := make([]reviewerCandidate, 0, len(defaults))
		for _, account := range defaults {
			seed = append(seed, reviewerCandidate{
				uuid: account.UUID, label: account.Name(),
				group: groupDefaultReviewer, selected: true,
			})
		}
		return createReviewersMsg{
			candidates: mergeReviewerCandidates(seed, nil, members, author),
			note:       candidateNote("workspace members", err),
		}
	}
}

// createReviewersMsg carries the checklist's rows.
type createReviewersMsg struct {
	candidates []reviewerCandidate
	note       string
}

// handleCreateReviewers shows the picker, and creates without reviewers when
// there is nobody to pick.
func (a *App) handleCreateReviewers(msg createReviewersMsg) (tea.Model, tea.Cmd) {
	a.logFlag.Store(false)
	if a.pendingCreate == nil {
		return a, nil
	}
	if len(msg.candidates) == 0 {
		a.helpBar.SetStatusMsg("no reviewer candidates available")
		return a, a.advanceCreate()
	}

	items := make([]components.ModalItem, 0, len(msg.candidates))
	selected := make(map[string]bool, len(msg.candidates))
	for _, candidate := range msg.candidates {
		items = append(items, components.ModalItem{
			ID: candidate.uuid, Label: candidate.label + "  · " + candidate.group,
		})
		if candidate.selected {
			selected[candidate.uuid] = true
		}
	}

	a.onChecklist = func(picked []components.ModalItem) tea.Cmd {
		if a.pendingCreate == nil {
			return nil
		}
		a.pendingCreate.reviewers = pickedUUIDs(picked)
		return a.advanceCreate()
	}
	if msg.note != "" {
		a.helpBar.SetStatusMsg(msg.note)
	}
	a.modal.ShowChecklist("Reviewers for the new pull request", items, selected)
	return a, nil
}

// pickedUUIDs is the checked rows' ids.
func pickedUUIDs(picked []components.ModalItem) []string {
	uuids := make([]string, 0, len(picked))
	for _, item := range picked {
		if item.ID != "" {
			uuids = append(uuids, item.ID)
		}
	}
	return uuids
}

// submitCreate is the POST, and the end of the flow.
func (a *App) submitCreate(pending *pendingCreate) tea.Cmd {
	ctx, client := a.context(), a.client
	create := bitbucket.NewPullRequest{
		Title: pending.title, Description: pending.body,
		SourceBranch: pending.source, DestinationBranch: pending.target,
		Reviewers: pending.reviewers, Draft: pending.draft, CloseSourceBranch: false,
	}
	workspace, slug := pending.workspace, pending.slug

	a.helpBar.SetStatusMsg("opening a pull request for " + pending.source + "…")
	a.logFlag.Store(true)
	return func() tea.Msg {
		created, err := client.CreatePullRequest(ctx, workspace, slug, create)
		return pullRequestCreatedMsg{pr: created, slug: slug, err: err}
	}
}

// handlePullRequestCreated shows what was created: it goes into the list, the
// cursor onto it and the detail panel into it, because the next thing an author
// does is look at what they just opened.
func (a *App) handlePullRequestCreated(msg pullRequestCreatedMsg) (tea.Model, tea.Cmd) {
	a.pendingCreate = nil
	a.logFlag.Store(false)

	if msg.err != nil || msg.pr == nil {
		a.helpBar.SetStatusMsg(verbCreate + " failed: " + a.writeErrorText(verbCreate, msg.err))
		return a, nil
	}

	created := msg.pr
	a.statusPanel.SetOnline(true)
	a.prCache[created.Key()] = created
	a.addToOpenTabs(msg.slug, created)
	a.helpBar.SetStatusMsg("opened #" + strconv.Itoa(created.ID))

	if !a.prList.SelectByKey(created.Key()) {
		// The list is filtered to another repository, or has not loaded: the
		// pull request is open on Bitbucket either way, and a refresh shows it.
		return a, nil
	}
	a.userTookTheCursor()
	return a, a.previewSelected()
}

// addToOpenTabs puts a new pull request at the top of every loaded list that
// should hold it, so it is on screen without a refetch. A fetch key leads with
// the state it asked for (see fetchKeyOf), and a pull request that has just
// been opened belongs to exactly the lists that asked for open ones; the tabs'
// own Mine and Reviewing filters then decide which of them shows it.
//
// It goes into the store under slug, the repository it was created in, and the
// flat lists are rebuilt from there: a row put only into a flat list would be
// gone at the next rebuild. A list that has not loaded that repository is left
// alone, and gets the pull request with the rest when it does.
func (a *App) addToOpenTabs(slug string, created *bitbucket.PullRequest) {
	open := func(fetchKey string) bool {
		state, _, _ := strings.Cut(fetchKey, "|")
		return strings.EqualFold(state, string(bitbucket.PullRequestOpen))
	}
	for _, fetchKey := range a.fetched.add(slug, created, open) {
		a.rebuildMerged(fetchKey)
	}
	a.refilterTabs("")
}
