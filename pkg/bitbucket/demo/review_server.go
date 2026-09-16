package demo

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
)

// Query parameters the review routes read or add.
const (
	queryPage            = "page"
	queryPath            = "path"
	queryContext         = "context"
	queryFromPullRequest = "from_pullrequest_id"
	queryTopic           = "topic"
	queryFields          = "fields"
)

// Path segments of the repository-level diff resources the pull request
// endpoints redirect to.
const (
	resourceDiff     = "diff"
	resourceDiffStat = "diffstat"
)

// defaultDiffContext is how many lines of context a hunk keeps when the request
// asks for no particular number — the same default git and Bitbucket use.
const defaultDiffContext = 3

// -----------------------------------------------------------------------------
// Pull request routes
// -----------------------------------------------------------------------------

// handlePullRequestCommits serves the commits of a pull request the way the real
// endpoint does: iterator pagination, so the envelope carries no size and its
// next link holds an opaque token rather than a page number.
func (s *Server) handlePullRequestCommits(w http.ResponseWriter, r *http.Request) {
	s.dataMu.RLock()
	defer s.dataMu.RUnlock()

	pullRequest, ok := s.findPullRequest(r)
	if !ok {
		writePullRequestNotFound(w, r)
		return
	}

	key := reviewKey(r.PathValue("slug"), pullRequest.ID)
	writeIteratorList(w, r, s.Data.Commits[key], s.maxPageLen())
}

// handlePullRequestComments serves the flat comment list, oldest first when the
// request asks for sort=created_on as the client does.
func (s *Server) handlePullRequestComments(w http.ResponseWriter, r *http.Request) {
	s.dataMu.RLock()
	defer s.dataMu.RUnlock()

	pullRequest, ok := s.findPullRequest(r)
	if !ok {
		writePullRequestNotFound(w, r)
		return
	}

	key := reviewKey(r.PathValue("slug"), pullRequest.ID)
	comments := make([]bitbucket.Comment, len(s.Data.Comments[key]))
	copy(comments, s.Data.Comments[key])
	stripResolutionMembers(comments, r.URL.Query().Get(queryFields))
	sortByTime(comments, ascending(r), func(comment bitbucket.Comment) time.Time {
		return comment.CreatedOn
	})
	writeList(w, r, comments, s.maxPageLen())
}

// stripResolutionMembers reproduces what the real collection does to a resolved
// comment: it sends `"resolution": {}` — the object, with none of its members —
// unless the request asks for them by name. An unresolved comment carries no
// resolution at all either way.
//
// It matters because the emptiness is all a client sees by default, so a client
// that reads the members to decide whether a thread is resolved sees every
// resolved thread as open. This fake answered with the members filled in and
// therefore could not show that.
func stripResolutionMembers(comments []bitbucket.Comment, fields string) {
	if strings.Contains(fields, "resolution") {
		return
	}
	for i := range comments {
		if comments[i].Resolution == nil {
			continue
		}
		comments[i].Resolution = &bitbucket.CommentResolution{
			Type: "", User: nil, CreatedOn: time.Time{},
		}
	}
}

// handlePullRequestDiffStat and handlePullRequestDiff do not serve a document:
// the real endpoints answer 302 and point at the repository-level resource for
// the pull request's revspec, which is where the parameters that actually shape
// the response live.
func (s *Server) handlePullRequestDiffStat(w http.ResponseWriter, r *http.Request) {
	s.redirectToRevspec(w, r, resourceDiffStat)
}

func (s *Server) handlePullRequestDiff(w http.ResponseWriter, r *http.Request) {
	s.redirectToRevspec(w, r, resourceDiff)
}

// redirectToRevspec answers the 302. The Location carries the server's own
// undocumented parameters and nothing else — the caller's query is dropped,
// exactly as the real API drops it — so a client that let net/http follow the
// hop would lose its path= and context= and be served the whole diff. The
// client resolves this redirect itself and merges its parameters onto the
// Location; the fake must not paper over the shape that forces it to.
func (s *Server) redirectToRevspec(w http.ResponseWriter, r *http.Request, resource string) {
	s.dataMu.RLock()
	defer s.dataMu.RUnlock()

	pullRequest, ok := s.findPullRequest(r)
	if !ok {
		writePullRequestNotFound(w, r)
		return
	}

	slug := r.PathValue("slug")
	query := url.Values{}
	query.Set(queryFromPullRequest, strconv.Itoa(pullRequest.ID))
	query.Set(queryTopic, "true")

	// net/url escapes the revspec's carriage return as %0D on the way out.
	target := url.URL{
		Path:     repositoryResourcePath(slug, resource, revspec(pullRequest, slug)),
		RawQuery: query.Encode(),
	}
	w.Header().Set("Location", target.String())
	w.WriteHeader(http.StatusFound)
}

// repositoryResourcePath is the path of a repository-level diff resource for one
// revspec, with the separator still raw — pass it through net/url to encode it.
func repositoryResourcePath(slug, resource, spec string) string {
	return "/repositories/" + Workspace + "/" + slug + "/" + resource + "/" + spec
}

// -----------------------------------------------------------------------------
// Repository revspec routes (the redirect targets)
// -----------------------------------------------------------------------------

// handleRepositoryDiffStat serves the diffstat of a revspec: one entry per
// changed file, paginated with size present, or 555 for a pull request the
// server refuses to diff.
func (s *Server) handleRepositoryDiffStat(w http.ResponseWriter, r *http.Request) {
	s.dataMu.RLock()
	defer s.dataMu.RUnlock()

	key, ok := s.revspecKey(w, r)
	if !ok {
		return
	}
	writeList(w, r, s.Data.DiffStats[key], s.maxPageLen())
}

// handleRepositoryDiff serves the unified diff of a revspec as text/plain,
// filtered by the repeated path= parameter and clipped to context= lines of
// context around each change.
func (s *Server) handleRepositoryDiff(w http.ResponseWriter, r *http.Request) {
	s.dataMu.RLock()
	defer s.dataMu.RUnlock()

	key, ok := s.revspecKey(w, r)
	if !ok {
		return
	}

	text := filterDiff(s.Data.Diffs[key], s.Data.DiffStats[key], r.URL.Query()[queryPath])
	text = limitDiffContext(text, diffContext(r))

	body := []byte(text)
	w.Header().Set("Content-Type", contentTypeText)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	// The body is this package's own diff fixture, served as text/plain; the
	// only request-derived input is the integer count of context lines.
	_, _ = w.Write(body) //nolint:gosec // G705: no request text reaches the response
}

// revspecKey resolves a revspec request to the pull request key its data is
// stored under, writing the 404 or the 555 itself when it cannot. ok is false
// once anything has been written.
func (s *Server) revspecKey(w http.ResponseWriter, r *http.Request) (string, bool) {
	pullRequest, ok := s.findRevspecPullRequest(r)
	if !ok {
		writeRevspecNotFound(w, r)
		return "", false
	}

	key := reviewKey(r.PathValue("slug"), pullRequest.ID)
	if s.Data.TooLargeDiffs[key] {
		writeDiffTooLarge(w, pullRequest.ID)
		return "", false
	}
	return key, true
}

// findRevspecPullRequest resolves which pull request a revspec request is about.
// from_pullrequest_id is what the redirect carries and is authoritative; without
// it the revspec's own two hashes are matched, so the target URL works on its
// own as well.
func (s *Server) findRevspecPullRequest(r *http.Request) (bitbucket.PullRequest, bool) {
	slug := r.PathValue("slug")
	if _, ok := s.findRepository(r); !ok {
		return bitbucket.PullRequest{}, false
	}

	pullRequests := s.Data.PullRequests[slug]
	if raw := r.URL.Query().Get(queryFromPullRequest); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil {
			return bitbucket.PullRequest{}, false
		}
		for _, pullRequest := range pullRequests {
			if pullRequest.ID == id {
				return pullRequest, true
			}
		}
		return bitbucket.PullRequest{}, false
	}

	spec := r.PathValue("spec")
	for _, pullRequest := range pullRequests {
		if revspec(pullRequest, slug) == spec {
			return pullRequest, true
		}
	}
	return bitbucket.PullRequest{}, false
}

// -----------------------------------------------------------------------------
// Diff filtering
// -----------------------------------------------------------------------------

// diffSection is the marker every file section of a git diff starts with.
const diffSection = "diff --git "

// filterDiff keeps only the sections of a whole-pull-request diff whose file is
// among the requested paths. An empty request keeps everything; a path nothing
// matches yields an empty diff, which is what the real endpoint answers.
//
// The sections line up with the diffstat entries — both come from the same text,
// one per "diff --git" line — so the entries supply the old and new path of each
// section without parsing the header again.
func filterDiff(text string, entries []bitbucket.DiffStatEntry, paths []string) string {
	if text == "" || len(paths) == 0 {
		return text
	}

	sections := splitDiffSections(text)
	if len(sections) != len(entries) {
		// The two disagree only if the dataset was built by hand; serving the
		// whole diff is the harmless answer.
		return text
	}

	wanted := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		wanted[path] = struct{}{}
	}

	var kept strings.Builder
	for index, section := range sections {
		entry := entries[index]
		_, byNew := wanted[entry.Path()]
		_, byOld := wanted[entry.OldPath()]
		if byNew || byOld {
			kept.WriteString(section)
		}
	}
	return kept.String()
}

// splitDiffSections cuts a diff into one string per file section, each keeping
// its trailing newline so concatenating them rebuilds the document.
func splitDiffSections(text string) []string {
	lines := strings.SplitAfter(text, "\n")

	var (
		sections []string
		current  strings.Builder
	)
	for _, line := range lines {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, diffSection) && current.Len() > 0 {
			sections = append(sections, current.String())
			current.Reset()
		}
		current.WriteString(line)
	}
	if current.Len() > 0 {
		sections = append(sections, current.String())
	}
	return sections
}

// diffContext is the number of context lines the request asks for, or the
// default when it asks for nothing.
func diffContext(r *http.Request) int {
	return positiveInt(r.URL.Query().Get(queryContext), defaultDiffContext)
}

// limitDiffContext clips every hunk to at most lines of context before the
// first change and after the last one, rewriting the hunk headers to match.
//
// It is deliberately simpler than git: context between two changes inside one
// hunk is left alone, and no hunk is ever split in two. That is enough to make
// context= visible in the demo without reimplementing a diff generator.
func limitDiffContext(text string, lines int) string {
	if text == "" || lines <= 0 {
		return text
	}

	source := strings.SplitAfter(text, "\n")
	out := make([]string, 0, len(source))

	for index := 0; index < len(source); index++ {
		line := source[index]
		if !strings.HasPrefix(line, prefixHunkHeader) {
			out = append(out, line)
			continue
		}

		body, next := hunkBody(source, index+1)
		header, trimmed := clipHunk(line, body, lines)
		out = append(out, header)
		out = append(out, trimmed...)
		index = next - 1
	}
	return strings.Join(out, "")
}

// prefixHunkHeader starts a hunk header line.
const prefixHunkHeader = "@@"

// hunkBody collects the lines belonging to the hunk that starts at index and
// returns them with the index of the first line after it.
func hunkBody(source []string, index int) ([]string, int) {
	start := index
	for index < len(source) {
		line := source[index]
		if line == "" || strings.HasPrefix(line, prefixHunkHeader) || strings.HasPrefix(line, diffSection) {
			break
		}
		index++
	}
	return source[start:index], index
}

// hunkHeader is the parsed form of "@@ -oldStart,oldLines +newStart,newLines @@ text".
type hunkHeader struct {
	oldStart int
	newStart int
	text     string
}

// clipHunk drops the context beyond keep lines at either end of body and
// rebuilds the hunk header for what is left.
func clipHunk(header string, body []string, keep int) (string, []string) {
	parsed, ok := parseHunkHeaderLine(header)
	if !ok {
		return header, body
	}

	dropLeading := max(0, countContext(body, false)-keep)
	dropTrailing := max(0, countContext(body, true)-keep)
	if dropLeading+dropTrailing >= len(body) {
		return header, body
	}

	clipped := body[dropLeading : len(body)-dropTrailing]
	oldLines, newLines := countSides(clipped)
	parsed.oldStart += dropLeading
	parsed.newStart += dropLeading

	return formatHunkHeader(parsed, oldLines, newLines), clipped
}

// countContext counts the run of context lines at one end of a hunk body. A
// "\ No newline at end of file" line belongs to the change above it, so it ends
// the run rather than extending it.
func countContext(body []string, fromEnd bool) int {
	count := 0
	for index := range body {
		line := body[index]
		if fromEnd {
			line = body[len(body)-1-index]
		}
		if !isContextLine(line) {
			return count
		}
		count++
	}
	return count
}

func isContextLine(line string) bool {
	trimmed := strings.TrimRight(line, "\r\n")
	return trimmed == "" || trimmed[0] == ' '
}

// countSides counts how many lines a hunk body occupies on the old and the new
// side, which is what the hunk header states.
func countSides(body []string) (oldLines, newLines int) {
	for _, line := range body {
		trimmed := strings.TrimRight(line, "\r\n")
		switch {
		case trimmed == "" || trimmed[0] == ' ':
			oldLines++
			newLines++
		case trimmed[0] == '+':
			newLines++
		case trimmed[0] == '-':
			oldLines++
		}
	}
	return oldLines, newLines
}

func parseHunkHeaderLine(line string) (hunkHeader, bool) {
	rest := strings.TrimPrefix(strings.TrimRight(line, "\r\n"), prefixHunkHeader)

	numbers, trailer, found := strings.Cut(rest, prefixHunkHeader)
	if !found {
		return hunkHeader{}, false
	}

	header := hunkHeader{
		oldStart: 0,
		newStart: 0,
		text:     strings.TrimPrefix(trailer, " "),
	}
	for _, field := range strings.Fields(numbers) {
		start, _, _ := strings.Cut(field[1:], ",")
		value, err := strconv.Atoi(start)
		if err != nil {
			return hunkHeader{}, false
		}
		switch field[0] {
		case '-':
			header.oldStart = value
		case '+':
			header.newStart = value
		default:
			return hunkHeader{}, false
		}
	}
	return header, true
}

func formatHunkHeader(header hunkHeader, oldLines, newLines int) string {
	line := "@@ -" + strconv.Itoa(header.oldStart) + "," + strconv.Itoa(oldLines) +
		" +" + strconv.Itoa(header.newStart) + "," + strconv.Itoa(newLines) + " @@"
	if header.text != "" {
		line += " " + header.text
	}
	return line + "\n"
}

// -----------------------------------------------------------------------------
// Iterator pagination
// -----------------------------------------------------------------------------

// iteratorEnvelope is the paginated wrapper of an iterator-paginated
// collection: no size, because the server will not count the collection, and a
// next link whose page parameter is an opaque token.
// The page member is a string here, not a number: that is exactly why the
// client does not model it, and serving it that way keeps the client honest.
type iteratorEnvelope struct {
	PageLen int               `json:"pagelen"`
	Page    string            `json:"page,omitempty"`
	Next    string            `json:"next,omitempty"`
	Values  []json.RawMessage `json:"values"`
}

func writeIteratorList[T any](w http.ResponseWriter, r *http.Request, items []T, ceiling int) {
	values, err := encodeAll(items)
	if err != nil {
		writeServerError(w, err)
		return
	}

	token := r.URL.Query().Get(queryPage)
	start, ok := decodePageToken(token)
	if !ok {
		writeBadRequest(w, "Invalid page token: "+token)
		return
	}

	pageLen := min(positiveInt(r.URL.Query().Get("pagelen"), defaultPageLen), ceiling)
	start = min(start, len(values))
	end := min(start+pageLen, len(values))

	envelope := iteratorEnvelope{PageLen: pageLen, Page: token, Next: "", Values: values[start:end]}
	if end < len(values) {
		envelope.Next = pageURL(r, encodePageToken(end))
	}
	writeJSON(w, envelope)
}

// encodePageToken hides an offset in an opaque token, so a client that tries to
// build the next page itself instead of following the link gets nowhere — which
// is the property the real iterator pagination has.
func encodePageToken(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(offset)))
}

// decodePageToken reverses encodePageToken. An absent token means the first
// page; anything unreadable is a bad request rather than a silent reset.
func decodePageToken(token string) (int, bool) {
	if token == "" {
		return 0, true
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0, false
	}
	offset, err := strconv.Atoi(string(raw))
	if err != nil || offset < 0 {
		return 0, false
	}
	return offset, true
}

// -----------------------------------------------------------------------------
// Responses
// -----------------------------------------------------------------------------

// writeDiffTooLarge answers Bitbucket's nonstandard 555, which means the diff
// was too large to build and retrying will not help.
func writeDiffTooLarge(w http.ResponseWriter, id int) {
	writeErrorEnvelope(w, bitbucket.StatusDiffTooLarge,
		"Diff for pull request "+strconv.Itoa(id)+" is too large to generate.")
}

func writeBadRequest(w http.ResponseWriter, message string) {
	writeErrorEnvelope(w, http.StatusBadRequest, message)
}

func writeRevspecNotFound(w http.ResponseWriter, r *http.Request) {
	writeNotFound(w, "No such revspec in "+r.PathValue("ws")+"/"+r.PathValue("slug")+".")
}
