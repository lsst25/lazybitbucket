package diff

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Prefixes of the structural lines of a git-style diff.
const (
	prefixGitHeader   = "diff --git "
	prefixHunk        = "@@"
	prefixOldPath     = "--- "
	prefixNewPath     = "+++ "
	prefixNoNewline   = `\`
	prefixIndex       = "index "
	prefixNewFile     = "new file mode "
	prefixDeletedFile = "deleted file mode "
	prefixOldMode     = "old mode "
	prefixNewMode     = "new mode "
	prefixRenameFrom  = "rename from "
	prefixRenameTo    = "rename to "
	prefixCopyFrom    = "copy from "
	prefixCopyTo      = "copy to "
	prefixBinaryFiles = "Binary files "
	suffixBinaryFiles = " differ"
	lineGitBinary     = "GIT binary patch"
)

// Path prefixes git puts on the two sides of a diff, and the placeholder it
// uses for the side on which the file does not exist.
const (
	oldPathPrefix = "a/"
	newPathPrefix = "b/"
	devNull       = "/dev/null"
)

// errHunkBeforeFile is the only thing Parse refuses to guess about: a hunk
// header with no file section to attach it to.
var errHunkBeforeFile = errors.New("hunk header before any file header")

// Parse parses a git-style unified diff.
//
// It is lenient by design — see the package comment for exactly how — and
// returns an error only when the text has a hunk header before any file
// header, which means it is not a diff at all.
func Parse(text string) ([]File, error) {
	if text == "" {
		return nil, nil
	}

	p := &parser{lines: strings.Split(strings.TrimSuffix(text, "\n"), "\n")}
	for p.index = 0; p.index < len(p.lines); p.index++ {
		line := strings.TrimSuffix(p.lines[p.index], "\r")
		if err := p.step(line); err != nil {
			return nil, fmt.Errorf("diff line %d: %w", p.index+1, err)
		}
	}
	p.flushHunk()

	files := make([]File, 0, len(p.files))
	for _, file := range p.files {
		normalise(file)
		files = append(files, *file)
	}
	return files, nil
}

// normalise drops the path of the side a file does not have, so that an added
// file always has an empty OldPath and a deleted one an empty NewPath even
// though "diff --git a/x b/x" names both sides of either.
func normalise(file *File) {
	switch file.Status {
	case Added:
		file.OldPath = ""
	case Deleted:
		file.NewPath = ""
	case Modified, Renamed, Copied:
	default:
	}
}

// parser holds the walk state: the file being filled in, the hunk being
// filled in, and how many lines each side of that hunk still owes.
type parser struct {
	lines []string
	index int

	files    []*File
	current  *File
	pathSeen bool

	hunk   Hunk
	inHunk bool
	oldNo  int
	newNo  int
	remOld int
	remNew int
}

func (p *parser) step(line string) error {
	switch {
	case strings.HasPrefix(line, prefixGitHeader):
		p.startFile(line[len(prefixGitHeader):])
	case strings.HasPrefix(line, prefixHunk):
		return p.startHunk(line)
	case p.isPathPair(line):
		p.takePathPair(line)
	case p.inHunk && p.hunkLine(line):
		// Consumed as hunk content.
	default:
		p.meta(line)
	}
	return nil
}

// isPathPair reports whether line is the "--- old" half of a file header
// rather than a removed line. The pair of markers alone is not proof: a diff
// of a diff can hold a removed "--- x" followed by an added "+++ y", so the
// current hunk must also have all the lines it asked for.
func (p *parser) isPathPair(line string) bool {
	if p.inHunk && !p.satisfied() {
		return false
	}
	return strings.HasPrefix(line, prefixOldPath) && strings.HasPrefix(p.peek(1), prefixNewPath)
}

func (p *parser) peek(offset int) string {
	if p.index+offset >= len(p.lines) {
		return ""
	}
	return strings.TrimSuffix(p.lines[p.index+offset], "\r")
}

// takePathPair consumes the "--- old" / "+++ new" pair. A plain unified diff
// has no "diff --git" line, so the pair itself opens a new file section
// whenever the current one already has its paths or its hunks.
func (p *parser) takePathPair(line string) {
	p.flushHunk()
	if p.current == nil || p.pathSeen || len(p.current.Hunks) > 0 {
		p.newFile()
	}
	p.pathSeen = true

	p.setOldPath(strings.TrimPrefix(line, prefixOldPath))
	p.index++
	p.setNewPath(strings.TrimPrefix(p.peek(0), prefixNewPath))
}

func (p *parser) startFile(paths string) {
	p.flushHunk()
	p.newFile()

	oldRaw, newRaw := splitGitPaths(paths)
	p.setOldPath(oldRaw)
	if newRaw != "" {
		p.setNewPath(newRaw)
	}
	// "diff --git a/x b/x" names both sides even for a file that is only on
	// one of them; the ---/+++ lines that follow carry /dev/null and settle it.
	p.current.Status = Modified
}

func (p *parser) newFile() {
	p.current = &File{Status: Modified}
	p.files = append(p.files, p.current)
	p.pathSeen = false
}

func (p *parser) setOldPath(raw string) {
	path := cleanPath(raw, oldPathPrefix)
	p.current.OldPath = path
	if path == "" && p.current.Status == Modified {
		p.current.Status = Added
	}
}

func (p *parser) setNewPath(raw string) {
	path := cleanPath(raw, newPathPrefix)
	p.current.NewPath = path
	if path == "" && p.current.Status == Modified {
		p.current.Status = Deleted
	}
}

// meta handles the header lines that describe the file rather than its
// content. Anything it does not recognise is skipped, and anything at all is
// skipped before the first file section starts.
func (p *parser) meta(line string) {
	if p.current == nil {
		return
	}

	switch {
	case strings.HasPrefix(line, prefixOldPath):
		p.setOldPath(strings.TrimPrefix(line, prefixOldPath))
	case strings.HasPrefix(line, prefixNewPath):
		p.setNewPath(strings.TrimPrefix(line, prefixNewPath))
	case strings.HasPrefix(line, prefixNewFile):
		p.current.Status = Added
		p.current.NewMode = strings.TrimPrefix(line, prefixNewFile)
	case strings.HasPrefix(line, prefixDeletedFile):
		p.current.Status = Deleted
		p.current.OldMode = strings.TrimPrefix(line, prefixDeletedFile)
	case strings.HasPrefix(line, prefixOldMode):
		p.current.OldMode = strings.TrimPrefix(line, prefixOldMode)
	case strings.HasPrefix(line, prefixNewMode):
		p.current.NewMode = strings.TrimPrefix(line, prefixNewMode)
	case strings.HasPrefix(line, prefixRenameFrom):
		p.current.Status = Renamed
		p.current.OldPath = cleanPath(strings.TrimPrefix(line, prefixRenameFrom), "")
	case strings.HasPrefix(line, prefixRenameTo):
		p.current.Status = Renamed
		p.current.NewPath = cleanPath(strings.TrimPrefix(line, prefixRenameTo), "")
	case strings.HasPrefix(line, prefixCopyFrom):
		p.current.Status = Copied
		p.current.OldPath = cleanPath(strings.TrimPrefix(line, prefixCopyFrom), "")
	case strings.HasPrefix(line, prefixCopyTo):
		p.current.Status = Copied
		p.current.NewPath = cleanPath(strings.TrimPrefix(line, prefixCopyTo), "")
	case strings.HasPrefix(line, prefixIndex):
		p.indexLine(strings.TrimPrefix(line, prefixIndex))
	case strings.HasPrefix(line, prefixBinaryFiles) && strings.HasSuffix(line, suffixBinaryFiles):
		p.binaryFiles(line)
	case line == lineGitBinary:
		p.current.IsBinary = true
	}
}

// indexLine reads the trailing mode of "index <old>..<new> <mode>", which is
// the only place an unchanged mode is stated.
func (p *parser) indexLine(rest string) {
	fields := strings.Fields(rest)
	if len(fields) < 2 {
		return
	}
	if p.current.OldMode == "" {
		p.current.OldMode = fields[1]
	}
	if p.current.NewMode == "" {
		p.current.NewMode = fields[1]
	}
}

// binaryFiles marks the file binary and reads the paths out of "Binary files
// a/x and b/y differ", which for a binary file may be the only line that names
// them. A side already named is left alone unless this line says /dev/null.
func (p *parser) binaryFiles(line string) {
	p.current.IsBinary = true

	inner := strings.TrimSuffix(strings.TrimPrefix(line, prefixBinaryFiles), suffixBinaryFiles)
	oldRaw, newRaw, found := strings.Cut(inner, " and ")
	if !found {
		return
	}

	if p.current.OldPath == "" || cleanPath(oldRaw, oldPathPrefix) == "" {
		p.setOldPath(oldRaw)
	}
	if p.current.NewPath == "" || cleanPath(newRaw, newPathPrefix) == "" {
		p.setNewPath(newRaw)
	}
}

func (p *parser) startHunk(line string) error {
	if p.current == nil {
		return errHunkBeforeFile
	}

	p.flushHunk()
	p.hunk = parseHunkHeader(line)
	p.inHunk = true
	p.oldNo = p.hunk.OldStart
	p.newNo = p.hunk.NewStart
	p.remOld = p.hunk.OldLines
	p.remNew = p.hunk.NewLines
	return nil
}

func (p *parser) flushHunk() {
	if !p.inHunk {
		return
	}
	p.current.Hunks = append(p.current.Hunks, p.hunk)
	p.hunk = Hunk{}
	p.inHunk = false
}

// satisfied reports whether the current hunk has received every line its
// header promised. Once it has, content markers stop being content: what
// follows belongs to the next hunk, the next file, or the trailing noise.
func (p *parser) satisfied() bool { return p.remOld <= 0 && p.remNew <= 0 }

// hunkLine consumes one line of hunk content and reports whether it did.
func (p *parser) hunkLine(line string) bool {
	if message, ok := strings.CutPrefix(line, prefixNoNewline); ok {
		p.addLine(LineNoNewline, strings.TrimPrefix(message, " "))
		return true
	}
	if p.satisfied() {
		return false
	}

	switch {
	case line == "":
		// A transport that strips trailing whitespace turns the marker of an
		// empty context line into nothing at all.
		p.addLine(LineContext, "")
	case line[0] == ' ':
		p.addLine(LineContext, line[1:])
	case line[0] == '+':
		p.addLine(LineAdded, line[1:])
	case line[0] == '-':
		p.addLine(LineRemoved, line[1:])
	default:
		return false
	}
	return true
}

func (p *parser) addLine(kind LineKind, content string) {
	line := Line{Kind: kind, Content: content}

	switch kind {
	case LineContext:
		line.OldNo, line.NewNo = p.oldNo, p.newNo
		p.oldNo++
		p.newNo++
		p.remOld--
		p.remNew--
	case LineAdded:
		line.NewNo = p.newNo
		p.newNo++
		p.remNew--
	case LineRemoved:
		line.OldNo = p.oldNo
		p.oldNo++
		p.remOld--
	case LineNoNewline:
	default:
	}

	p.hunk.Lines = append(p.hunk.Lines, line)
}

// parseHunkHeader reads "@@ -oldStart[,oldLines] +newStart[,newLines] @@ header".
// The counts are optional and default to 1, as the unified diff format says.
func parseHunkHeader(line string) Hunk {
	var hunk Hunk

	rest := strings.TrimPrefix(line, prefixHunk)
	if end := strings.Index(rest, prefixHunk); end >= 0 {
		hunk.Header = strings.TrimPrefix(rest[end+len(prefixHunk):], " ")
		rest = rest[:end]
	}

	for _, field := range strings.Fields(rest) {
		switch {
		case strings.HasPrefix(field, "-"):
			hunk.OldStart, hunk.OldLines = parseRange(field[1:])
		case strings.HasPrefix(field, "+"):
			hunk.NewStart, hunk.NewLines = parseRange(field[1:])
		}
	}
	return hunk
}

func parseRange(field string) (start, count int) {
	count = 1
	if comma := strings.IndexByte(field, ','); comma >= 0 {
		count = atoi(field[comma+1:])
		field = field[:comma]
	}
	return atoi(field), count
}

func atoi(text string) int {
	value, err := strconv.Atoi(text)
	if err != nil {
		return 0
	}
	return value
}

// splitGitPaths splits the text after "diff --git " into its two path words.
// The form is ambiguous whenever a path contains a space and git did not quote
// it, so the split that leaves the same tail on both sides wins; the ---/+++
// lines settle whatever is left over.
func splitGitPaths(paths string) (oldRaw, newRaw string) {
	if strings.HasPrefix(paths, `"`) {
		if end := endOfQuoted(paths); end > 0 {
			return paths[:end], strings.TrimLeft(paths[end:], " ")
		}
	}

	fallback := -1
	for position := range len(paths) {
		if paths[position] != ' ' {
			continue
		}
		left, right := paths[:position], paths[position+1:]
		if !strings.HasPrefix(left, oldPathPrefix) || !strings.HasPrefix(right, newPathPrefix) {
			continue
		}
		if left[len(oldPathPrefix):] == right[len(newPathPrefix):] {
			return left, right
		}
		if fallback < 0 {
			fallback = position
		}
	}
	if fallback < 0 {
		fallback = strings.IndexByte(paths, ' ')
	}
	if fallback < 0 {
		// Nothing to split: the line is clipped, so only the old side is known.
		return paths, ""
	}
	return paths[:fallback], paths[fallback+1:]
}

// cleanPath turns one raw path word from a header line into a repository path:
// it drops a "--- path\ttimestamp" trailer, unquotes git's C-style quoting,
// strips the given side prefix and normalises /dev/null to the empty string.
func cleanPath(raw, sidePrefix string) string {
	if tab := strings.IndexByte(raw, '\t'); tab >= 0 {
		raw = raw[:tab]
	}
	raw = strings.TrimRight(raw, " ")
	if strings.HasPrefix(raw, `"`) && endOfQuoted(raw) == len(raw) {
		raw = unquoteC(raw)
	}
	if raw == devNull {
		return ""
	}
	return strings.TrimPrefix(raw, sidePrefix)
}

// endOfQuoted returns the index just past the closing quote of the C-style
// quoted string at the start of text, or 0 when it is not closed.
func endOfQuoted(text string) int {
	for position := 1; position < len(text); position++ {
		switch text[position] {
		case '\\':
			position++
		case '"':
			return position + 1
		}
	}
	return 0
}

// unquoteC reverses git's quote_c_style for the quoted string text, which
// includes its surrounding quotes. Octal escapes are byte escapes: a
// non-ASCII path comes back as the original bytes, valid UTF-8 or not.
func unquoteC(text string) string {
	inner := text[1 : len(text)-1]

	var out strings.Builder
	out.Grow(len(inner))

	for position := 0; position < len(inner); {
		if inner[position] != '\\' || position+1 == len(inner) {
			out.WriteByte(inner[position])
			position++
			continue
		}

		position++
		escape := inner[position]
		if octal, width := octalEscape(inner[position:]); width > 0 {
			out.WriteByte(octal)
			position += width
			continue
		}
		out.WriteByte(unescape(escape))
		position++
	}
	return out.String()
}

// octalEscape reads up to three octal digits and returns the byte they encode.
func octalEscape(text string) (value byte, width int) {
	for width < 3 && width < len(text) && text[width] >= '0' && text[width] <= '7' {
		value = value*8 + (text[width] - '0')
		width++
	}
	return value, width
}

func unescape(escape byte) byte {
	switch escape {
	case 'a':
		return '\a'
	case 'b':
		return '\b'
	case 'f':
		return '\f'
	case 'n':
		return '\n'
	case 'r':
		return '\r'
	case 't':
		return '\t'
	case 'v':
		return '\v'
	default:
		return escape
	}
}
