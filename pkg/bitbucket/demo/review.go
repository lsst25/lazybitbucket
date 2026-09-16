package demo

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/diff"
)

// Wire type words the review payloads use.
const (
	typeCommit     = "commit"
	typeComment    = "pullrequest_comment"
	typeDiffStat   = "diffstat"
	typeCommitFile = "commit_file"
	typeRendered   = "rendered"

	// attributeBinary marks a diffstat side git cannot diff as text.
	attributeBinary = "binary"
)

// reviewKey is the map key the review collections are stored under: the same
// bitbucket.PullRequestKey the statuses use.
func reviewKey(slug string, id int) string {
	return bitbucket.PullRequestKey(Workspace, slug, id)
}

// commitHash is a synthetic 40-character hash for the index'th commit of a pull
// request. The id and the index lead so a hash in a failure message says which
// commit it is, and the fixed body makes it obvious that nothing here is a real
// object.
func commitHash(pullRequest, index int) string {
	return fmt.Sprintf("%04x%04x", pullRequest, index) + strings.Repeat("c0ffee", 5) + "12"
}

// ident is the git author string for a demo account, the "Name <email>" shape
// Commit.Author.Raw carries.
func ident(account bitbucket.Account) string {
	return account.DisplayName + " <" + account.Nickname + "@acme.example>"
}

// -----------------------------------------------------------------------------
// Commits
// -----------------------------------------------------------------------------

type commitSpec struct {
	repo        string
	pullRequest int
	hash        string
	parent      string
	// user is the Bitbucket account behind the ident, nil when the ident matches
	// no account — a bot, or a contributor who never linked that address.
	user    *bitbucket.Account
	ident   string
	message string
	age     time.Duration // before baseTime
}

func newPullRequestCommit(spec commitSpec) bitbucket.Commit {
	full := fullName(spec.repo)
	repository := repoRef(spec.repo)
	apiCommit := apiBase + "/repositories/" + full + "/commit/" + spec.hash

	return bitbucket.Commit{
		Type:    typeCommit,
		Hash:    spec.hash,
		Date:    ago(spec.age),
		Message: spec.message,
		Author: bitbucket.CommitAuthor{
			Type: "author",
			Raw:  spec.ident,
			User: spec.user,
		},
		Summary: bitbucket.RenderedContent{
			Type:   typeRendered,
			Raw:    spec.message,
			Markup: "markdown",
			HTML:   "",
		},
		Parents:    []bitbucket.CommitRef{*newCommit(spec.repo, spec.parent)},
		Repository: &repository,
		Links: bitbucket.CommitLinks{
			Self:     bitbucket.Link{Href: apiCommit},
			HTML:     bitbucket.Link{Href: htmlBase + "/" + full + "/commits/" + spec.hash},
			Diff:     bitbucket.Link{Href: apiBase + "/repositories/" + full + "/diff/" + spec.hash},
			Patch:    bitbucket.Link{Href: apiBase + "/repositories/" + full + "/patch/" + spec.hash},
			Comments: bitbucket.Link{Href: apiCommit + "/comments"},
			Statuses: bitbucket.Link{Href: apiCommit + "/statuses"},
		},
	}
}

// commitSpecs lists the commits of every open pull request, newest first —
// the order `git log` and the commits endpoint both use. Each pull request's
// newest commit is its own source commit, and the oldest one's parent is the
// destination branch tip, so the chain agrees with the pull request payload.
//
// Closed pull requests carry none: their source branch is gone, and an empty
// commit list is what the real endpoint then answers.
func commitSpecs() []commitSpec {
	specs := commitSpecsAPIGateway()
	specs = append(specs, commitSpecsWebApp()...)
	return append(specs, commitSpecsInfra()...)
}

func commitSpecsAPIGateway() []commitSpec {
	return []commitSpec{
		{
			repo: repoAPIGateway, pullRequest: 103,
			hash: hashPR103, parent: commitHash(103, 1),
			user: ptr(linus()), ident: ident(linus()), age: 3 * hour,
			message: markdown(
				"upstream: derive a fresh deadline per retry attempt",
				"",
				"The loop reused the caller's context, so the second attempt inherited an",
				"already expired deadline and failed before it left the process.",
				"",
				"Fixes INC-4471.",
			),
		},
		{
			repo: repoAPIGateway, pullRequest: 103,
			hash: commitHash(103, 1), parent: commitHash(103, 2),
			user: ptr(linus()), ident: ident(linus()), age: 20 * hour,
			message: markdown(
				"upstream: regression test for the expired deadline",
				"",
				"Fails on main, passes with the fix.",
			),
		},
		{
			repo: repoAPIGateway, pullRequest: 103,
			hash: commitHash(103, 2), parent: hashMainAPI,
			user: ptr(linus()), ident: ident(linus()), age: 47 * hour,
			message: markdown("docs: record the retry budget in RFC 014"),
		},
		{
			repo: repoAPIGateway, pullRequest: 102,
			hash: hashPR102, parent: commitHash(102, 1),
			user: ptr(grace()), ident: ident(grace()), age: 6 * hour,
			message: markdown(
				"auth: cache introspection results for a minute",
				"",
				"Still a draft: negative results are cached too, which is probably wrong.",
			),
		},
		{
			repo: repoAPIGateway, pullRequest: 102,
			hash: commitHash(102, 1), parent: hashMainAPI,
			user: ptr(grace()), ident: ident(grace()), age: 23 * hour,
			message: markdown("auth: rename token.go to introspect.go and call the auth service"),
		},
		{
			repo: repoAPIGateway, pullRequest: 101,
			hash: hashPR101, parent: commitHash(101, 1),
			user: ptr(demoUser()), ident: ident(demoUser()), age: 27 * hour,
			message: markdown(
				"ci: run the bucket test under -race and -count=20",
				"",
				"The refill maths is time dependent, so once is not enough.",
			),
		},
		{
			repo: repoAPIGateway, pullRequest: 101,
			hash: commitHash(101, 1), parent: commitHash(101, 2),
			user: ptr(demoUser()), ident: ident(demoUser()), age: 40 * hour,
			message: markdown("router: install the limiter middleware when it is enabled"),
		},
		{
			repo: repoAPIGateway, pullRequest: 101,
			hash: commitHash(101, 2), parent: hashMainAPI,
			user: ptr(demoUser()), ident: ident(demoUser()), age: 70 * hour,
			message: markdown(
				"ratelimit: add a token bucket",
				"",
				"One bucket per client, refilled at a fixed rate, guarded by a mutex.",
			),
		},
	}
}

func commitSpecsWebApp() []commitSpec {
	return []commitSpec{
		{
			repo: repoWebApp, pullRequest: 41,
			hash: hashPR41, parent: commitHash(41, 1),
			user: ptr(demoUser()), ident: ident(demoUser()), age: 4 * hour,
			message: markdown("ci: build on node 22"),
		},
		{
			// The only commit whose ident matches no Bitbucket account, so
			// AuthorName() has to fall back to the raw git ident.
			repo: repoWebApp, pullRequest: 41,
			hash: commitHash(41, 1), parent: hashDevelopWeb,
			user:  nil,
			ident: "dependabot[bot] <49699333+dependabot[bot]@users.noreply.acme.example>",
			age:   5 * hour,
			message: markdown(
				"build(deps): bump react and react-dom from 18.2.0 to 18.3.1",
				"",
				"Drops the legacy polyfills the old versions needed.",
			),
		},
		{
			repo: repoWebApp, pullRequest: 40,
			hash: hashPR40, parent: commitHash(40, 1),
			user: ptr(ada()), ident: ident(ada()), age: 21 * hour,
			message: markdown("theme: screenshot the dark surface for the docs"),
		},
		{
			repo: repoWebApp, pullRequest: 40,
			hash: commitHash(40, 1), parent: commitHash(40, 2),
			user: ptr(ada()), ident: ident(ada()), age: 30 * hour,
			message: markdown(
				"theme: add the toggle button",
				"",
				"Announces the target mode rather than the current one, which is what",
				"the screen reader audit asked for.",
			),
		},
		{
			repo: repoWebApp, pullRequest: 40,
			hash: commitHash(40, 2), parent: commitHash(40, 3),
			user: ptr(ada()), ident: ident(ada()), age: 40 * hour,
			message: markdown("theme: split the tokens per theme"),
		},
		{
			repo: repoWebApp, pullRequest: 40,
			hash: commitHash(40, 3), parent: hashDevelopWeb,
			user: ptr(barbara()), ident: ident(barbara()), age: 46 * hour,
			message: markdown("theme: move the design tokens into custom properties"),
		},
	}
}

func commitSpecsInfra() []commitSpec {
	return []commitSpec{
		{
			repo: repoInfra, pullRequest: 7,
			hash: hashPR7, parent: commitHash(7, 1),
			user: ptr(ken()), ident: ident(ken()), age: 9 * hour,
			message: markdown(
				"k8s: pass TF_IN_AUTOMATION to the gateway deployment",
				"",
				"Keeps the plan output free of interactive prompts.",
			),
		},
		{
			repo: repoInfra, pullRequest: 7,
			hash: commitHash(7, 1), parent: hashMainInfra,
			user: ptr(ken()), ident: ident(ken()), age: 35 * hour,
			message: markdown("tools: pin terraform to 1.9.5"),
		},
	}
}

// -----------------------------------------------------------------------------
// Comments
// -----------------------------------------------------------------------------

type commentSpec struct {
	repo        string
	pullRequest int
	id          int64
	parent      int64 // 0 for a thread root
	user        bitbucket.Account
	body        string
	age         time.Duration // before baseTime
	edited      time.Duration // 0 when the comment was never edited
	deleted     bool
	pending     bool
	inline      *bitbucket.CommentInline
	resolvedBy  *bitbucket.Account
	resolvedAge time.Duration
}

func newComment(spec commentSpec) bitbucket.Comment {
	full := fullName(spec.repo)
	number := strconv.Itoa(spec.pullRequest)
	apiComments := apiBase + "/repositories/" + full + "/pullrequests/" + number + "/comments/"
	htmlPullRequest := htmlBase + "/" + full + "/pull-requests/" + number

	comment := bitbucket.Comment{
		Type:      typeComment,
		ID:        spec.id,
		CreatedOn: ago(spec.age),
		UpdatedOn: ago(orDuration(spec.edited, spec.age)),
		Content: bitbucket.RenderedContent{
			Type:   typeRendered,
			Raw:    spec.body,
			Markup: "markdown",
			HTML:   renderHTML(spec.body, workspaceMembers()),
		},
		User:       spec.user,
		Deleted:    spec.deleted,
		Pending:    spec.pending,
		Parent:     nil,
		Inline:     spec.inline,
		Resolution: nil,
		Links: bitbucket.CommentLinks{
			Self: bitbucket.Link{Href: apiComments + strconv.FormatInt(spec.id, 10)},
			HTML: bitbucket.Link{
				Href: htmlPullRequest + "?_comment=" + strconv.FormatInt(spec.id, 10),
			},
			Code: bitbucket.Link{Href: ""},
		},
	}

	if spec.parent != 0 {
		parent := &bitbucket.CommentRef{ID: spec.parent}
		parent.Links.Self = bitbucket.Link{Href: apiComments + strconv.FormatInt(spec.parent, 10)}
		parent.Links.HTML = bitbucket.Link{
			Href: htmlPullRequest + "?_comment=" + strconv.FormatInt(spec.parent, 10),
		}
		comment.Parent = parent
	}

	// links.code is the diff anchor, and only an inline comment has one.
	if spec.inline != nil {
		comment.Links.Code = bitbucket.Link{
			Href: htmlPullRequest + "/diff#chg-" + spec.inline.Path,
		}
	}

	// An unresolved comment carries no resolution here. The real API sometimes
	// sends an empty object instead, which CommentResolution.Resolved() reads as
	// unresolved either way.
	if spec.resolvedBy != nil {
		comment.Resolution = &bitbucket.CommentResolution{
			Type:      "comment_resolution",
			User:      spec.resolvedBy,
			CreatedOn: ago(spec.resolvedAge),
		}
	}
	return comment
}

// orDuration returns value when it is positive, else fallback.
func orDuration(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}

// Comment ids. Bitbucket's are large sequential integers; these keep that shape
// so nothing downstream assumes a comment id fits a small counter.
const commentIDBase int64 = 458_724_000

// Files the demo comments are anchored to. The line numbers below are new-side
// (and old-side) numbers of the hunks in diffs.go, so the diff view really does
// find a thread on the line it marks — TestDataset_CommentAnchorsExistInDiffs
// keeps them honest.
const (
	pathRetryGo     = "internal/upstream/retry.go"
	pathRetryTestGo = "internal/upstream/retry_test.go"
	pathThemeTS     = "src/theme/tokens.ts"
)

// commentSpecs is the review conversation of the dataset: the noisy pull
// request #103 carries every thread shape the view has to render, and #40 has
// the file-level anchor (a comment on a file, on no line at all).
func commentSpecs() []commentSpec {
	specs := commentSpecsInline103()
	specs = append(specs, commentSpecsGeneral103()...)
	return append(specs, commentSpecs40()...)
}

func commentSpecsInline103() []commentSpec {
	return []commentSpec{
		// An inline thread on an added line, with one reply and no resolution.
		{
			repo: repoAPIGateway, pullRequest: 103, id: commentIDBase + 1,
			user: demoUser(), age: 2*hour + 10*time.Minute,
			inline: &bitbucket.CommentInline{Path: pathRetryGo, From: nil, To: ptr(16), StartFrom: nil, StartTo: nil},
			body: markdown(
				"`cancelAttempt()` runs after the response body is handed back — won't that",
				"cancel the body read for a streaming upstream?",
			),
		},
		{
			repo: repoAPIGateway, pullRequest: 103, id: commentIDBase + 2,
			parent: commentIDBase + 1,
			user:   linus(), age: 2 * hour,
			inline: &bitbucket.CommentInline{Path: pathRetryGo, From: nil, To: ptr(16), StartFrom: nil, StartTo: nil},
			body: markdown(
				"Only on the error path — the success path returns before the deferred",
				"cancel. I'll add a test for the streaming case, @{"+demoUser().AccountID+"}.",
			),
		},
		// A multi-line thread on context lines, resolved by the reviewer who
		// opened it. Anchored on both sides, as Bitbucket anchors a context line.
		{
			repo: repoAPIGateway, pullRequest: 103, id: commentIDBase + 3,
			user: ken(), age: 26 * hour, edited: 25 * hour,
			inline: &bitbucket.CommentInline{
				Path: pathRetryGo, From: ptr(13), To: ptr(13), StartFrom: ptr(12), StartTo: ptr(12),
			},
			resolvedBy: ptr(ken()), resolvedAge: 24 * hour,
			body: markdown(
				"These two lines are the whole budget: worth a comment saying the budget",
				"is *total* and the per-attempt timeout is derived from it.",
				"",
				"Never mind — the RFC says it. Resolving.",
			),
		},
		// A draft the author has not published yet: nobody else can see it, and
		// the view marks it "(pending)".
		{
			repo: repoAPIGateway, pullRequest: 103, id: commentIDBase + 9,
			user: demoUser(), age: 30 * time.Minute, pending: true,
			inline: &bitbucket.CommentInline{
				Path: pathRetryTestGo, From: nil, To: ptr(11), StartFrom: nil, StartTo: nil,
			},
			body: markdown("`perAttempt: 20ms` will be flaky on a loaded CI box."),
		},
	}
}

func commentSpecsGeneral103() []commentSpec {
	return []commentSpec{
		// A general thread: root plus two replies, the second one a reply to the
		// first, which BuildThreads flattens into the same thread. It is also
		// where the two mention shapes live — the root names a colleague who
		// has left, whose id Bitbucket cannot resolve and therefore renders as
		// literal text, and the reply names two people, one of them twice.
		{
			repo: repoAPIGateway, pullRequest: 103, id: commentIDBase + 4,
			user: grace(), age: 30 * hour,
			body: markdown(
				"## Before this merges",
				"",
				"- [ ] backport to `release/2.3`",
				"- [x] regression test",
				"",
				"The backport is the risky half — the 2.3 branch has no budget context.",
				"",
				"@{"+AccountIDGone+"} wrote the 2.3 budget code but has since left.",
			),
		},
		{
			repo: repoAPIGateway, pullRequest: 103, id: commentIDBase + 5,
			parent: commentIDBase + 4,
			user:   linus(), age: 29 * hour,
			body: markdown(
				"Backport is drafted, waiting for this one to land first.",
				"",
				"@{"+demoUser().AccountID+"} @{"+ken().AccountID+"} - @{"+ken().AccountID+"} owns",
				"the 2.3 branch, so the second review is his.",
			),
		},
		{
			repo: repoAPIGateway, pullRequest: 103, id: commentIDBase + 6,
			parent: commentIDBase + 5,
			user:   demoUser(), age: 28 * hour,
			body: markdown("Link it here when it is up and I'll review both together."),
		},
		// A deleted root that still has a reply, so the thread survives as a
		// "(deleted)" placeholder rather than losing the answer with it.
		{
			repo: repoAPIGateway, pullRequest: 103, id: commentIDBase + 7,
			user: ada(), age: 40 * hour, deleted: true,
			body: "",
		},
		{
			repo: repoAPIGateway, pullRequest: 103, id: commentIDBase + 8,
			parent: commentIDBase + 7,
			user:   grace(), age: 39 * hour,
			body: markdown("Answered above: the budget context is created in `Do`, not per attempt."),
		},
	}
}

func commentSpecs40() []commentSpec {
	return []commentSpec{
		// A file-level inline comment: anchored to a path with every line field
		// null, which is what Bitbucket sends for a comment on a file rather
		// than on one of its lines.
		{
			repo: repoWebApp, pullRequest: 40, id: commentIDBase + 20,
			user: ken(), age: 22 * hour,
			inline: &bitbucket.CommentInline{
				Path: pathThemeTS, From: nil, To: nil, StartFrom: nil, StartTo: nil,
			},
			body: markdown(
				"Whole-file note: once these are per theme, the CSS custom properties in",
				"`base.css` duplicate them. Worth a follow-up.",
			),
		},
	}
}

// -----------------------------------------------------------------------------
// Diffs and diffstats
// -----------------------------------------------------------------------------

type diffSpec struct {
	repo        string
	pullRequest int
	// oldHash and newHash are the two sides of the comparison: the destination
	// tip and the pull request's head, both 40 characters. They appear in the
	// diffstat's per-file source links and in the diff revspec.
	oldHash string
	newHash string
	text    string
	// tooLarge makes both diff endpoints answer 555 instead of a document, the
	// way Bitbucket gives up on a pull request it cannot diff in time.
	tooLarge bool
}

func diffSpecs() []diffSpec {
	return []diffSpec{
		{
			repo: repoAPIGateway, pullRequest: 103,
			oldHash: hashMainAPI, newHash: hashPR103, text: diffPR103(), tooLarge: false,
		},
		{
			repo: repoAPIGateway, pullRequest: 102,
			oldHash: hashMainAPI, newHash: hashPR102, text: diffPR102(), tooLarge: false,
		},
		{
			repo: repoAPIGateway, pullRequest: 101,
			oldHash: hashMainAPI, newHash: hashPR101, text: diffPR101(), tooLarge: false,
		},
		{
			// The merged pull request nobody can diff any more: 555 from both
			// endpoints, which is the too-large path the UI has to survive.
			repo: repoAPIGateway, pullRequest: 98,
			oldHash: hashMainAPI, newHash: hashPR98, text: "", tooLarge: true,
		},
		{
			repo: repoWebApp, pullRequest: 41,
			oldHash: hashDevelopWeb, newHash: hashPR41, text: diffPR41(), tooLarge: false,
		},
		{
			repo: repoWebApp, pullRequest: 40,
			oldHash: hashDevelopWeb, newHash: hashPR40, text: diffPR40(), tooLarge: false,
		},
		{
			repo: repoInfra, pullRequest: 7,
			oldHash: hashMainInfra, newHash: hashPR7, text: diffPR7(), tooLarge: false,
		},
	}
}

// newDiffStat derives the diffstat of a pull request from its diff text, so the
// file list, the statuses and the +/- counts can never disagree with the diff
// the other endpoint serves.
//
// The fixtures are literals in this package: a parse failure means diffs.go is
// broken, which is a programming error rather than a runtime condition.
func newDiffStat(spec diffSpec) []bitbucket.DiffStatEntry {
	files, err := diff.Parse(spec.text)
	if err != nil {
		panic(fmt.Sprintf("demo: the %s#%d diff fixture is not a diff: %v",
			spec.repo, spec.pullRequest, err))
	}

	entries := make([]bitbucket.DiffStatEntry, 0, len(files))
	for _, file := range files {
		entries = append(entries, newDiffStatEntry(spec, file))
	}
	return entries
}

func newDiffStatEntry(spec diffSpec, file diff.File) bitbucket.DiffStatEntry {
	entry := bitbucket.DiffStatEntry{
		Type:         typeDiffStat,
		Status:       diffStatStatus(file.Status),
		LinesAdded:   file.Added(),
		LinesRemoved: file.Removed(),
		Old:          nil,
		New:          nil,
	}
	if file.OldPath != "" {
		entry.Old = newCommitFile(spec.repo, spec.oldHash, file.OldPath, file.IsBinary)
	}
	if file.NewPath != "" {
		entry.New = newCommitFile(spec.repo, spec.newHash, file.NewPath, file.IsBinary)
	}
	return entry
}

// diffStatStatus maps a parsed file status onto the word the diffstat endpoint
// uses. Bitbucket has no "copied": a copy shows up as an added file.
func diffStatStatus(status diff.FileStatus) string {
	switch status {
	case diff.Added:
		return bitbucket.DiffStatAdded
	case diff.Deleted:
		return bitbucket.DiffStatRemoved
	case diff.Renamed:
		return bitbucket.DiffStatRenamed
	case diff.Copied:
		return bitbucket.DiffStatAdded
	case diff.Modified:
		return bitbucket.DiffStatModified
	default:
		return bitbucket.DiffStatModified
	}
}

func newCommitFile(slug, hash, path string, binary bool) *bitbucket.CommitFile {
	file := &bitbucket.CommitFile{
		Type:        typeCommitFile,
		Path:        path,
		EscapedPath: path,
		Attributes:  nil,
	}
	if binary {
		file.Attributes = []string{attributeBinary}
	}
	file.Links.Self = bitbucket.Link{
		Href: apiBase + "/repositories/" + fullName(slug) + "/src/" + hash + "/" + path,
	}
	return file
}

// -----------------------------------------------------------------------------
// Revspecs
// -----------------------------------------------------------------------------

// specSeparator is what Bitbucket puts between the two sides of a revspec: a
// carriage return, which every URL carrying a revspec spells "%0D". Handlers
// see it in this decoded form, because net/http unescapes the request path
// before matching it.
const specSeparator = "\r"

// revspec is the two-dot comparison the diff and diffstat endpoints work on:
// "<workspace>/<slug>:<source>\r<destination>", with the two 12-character
// hashes the pull request payload carries. Build a URL from it with net/url,
// which encodes the separator as %0D.
func revspec(pullRequest bitbucket.PullRequest, slug string) string {
	return fullName(slug) + ":" + endpointHash(pullRequest.Source) +
		specSeparator + endpointHash(pullRequest.Destination)
}

func endpointHash(endpoint bitbucket.PullRequestEndpoint) string {
	if endpoint.Commit == nil {
		return ""
	}
	return endpoint.Commit.Hash
}
