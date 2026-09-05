package diff

// LineKind classifies one line inside a hunk.
type LineKind int

// The kinds a hunk line can have. LineContext is the zero value: a line that
// is present unchanged on both sides.
const (
	LineContext LineKind = iota
	LineAdded
	LineRemoved
	LineNoNewline
)

// Line is a single line of a hunk. Content carries the text with the leading
// ' ', '+' or '-' marker and the trailing newline removed. OldNo and NewNo are
// the 1-based line numbers on the old and the new side, and are 0 when the
// line does not exist on that side (both are 0 for LineNoNewline).
//
// Source is where the line sat in the document Parse read: the 0-based index
// into the lines the text was split on, counting every header, hunk header and
// file section of the whole document, not just this file's. It is what lets a
// caller line a per-line rendering of the same text — an external diff
// renderer's output — up with the parsed lines.
type Line struct {
	Kind    LineKind
	Content string
	OldNo   int
	NewNo   int
	Source  int
}

// Hunk is one @@ section of a file diff. Header is the text that follows the
// second "@@" on the hunk header line, which git fills with the enclosing
// function signature.
type Hunk struct {
	OldStart int
	OldLines int
	NewStart int
	NewLines int
	Header   string
	Lines    []Line
}

// FileStatus is what happened to a file between the two sides of the diff.
type FileStatus int

// The statuses a file section can have. Modified is the zero value.
const (
	Modified FileStatus = iota
	Added
	Deleted
	Renamed
	Copied
)

// File is one file section of a diff. OldPath is empty for an added file and
// NewPath is empty for a deleted one ("/dev/null" is normalised away). Hunks
// is empty for a binary file, a pure mode change and a rename with no content
// change.
type File struct {
	OldPath  string
	NewPath  string
	Status   FileStatus
	IsBinary bool
	OldMode  string
	NewMode  string
	Hunks    []Hunk
}

// Path is the file's current path: the new one when it has one, else the old.
func (f File) Path() string {
	if f.NewPath != "" {
		return f.NewPath
	}
	return f.OldPath
}

// Added is the number of added lines across every hunk.
func (f File) Added() int { return f.count(LineAdded) }

// Removed is the number of removed lines across every hunk.
func (f File) Removed() int { return f.count(LineRemoved) }

func (f File) count(kind LineKind) int {
	total := 0

	for _, hunk := range f.Hunks {
		for _, line := range hunk.Lines {
			if line.Kind == kind {
				total++
			}
		}
	}
	return total
}
