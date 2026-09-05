package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// The external diff renderer: the `diffRenderer` config key names a program —
// delta is the one this is modelled on, through lazygit's git.paging.pager —
// that the file diff pane pipes a file's whole unified diff through instead of
// colouring it with chroma.
//
// The contract is one output line per input line: the program may colour a
// line but may not add, drop, wrap or reorder one, because the pane keeps its
// own gutters, marker column and cursor and looks each parsed line's cell up
// by diff.Line.Source, which indexes the same split of the same document. A
// program that breaks the contract is caught by the line count and turned off
// for the session rather than drawn wrongly.
//
// Everything in this file runs inside the fetch command, off the event loop:
// the output reaches the view as finished strings.

const (
	// diffRendererTimeout bounds one run. The program is handed a single
	// file's diff, so it is generous; what it is really there for is a
	// misconfigured command that reads stdin and never exits.
	diffRendererTimeout = 15 * time.Second

	// diffRendererWaitDelay bounds the wait for the standard streams once the
	// program itself is gone. See renderDiffLines: the deadline above is worth
	// nothing without it.
	diffRendererWaitDelay = time.Second

	escByte  = 0x1b
	bellByte = 0x07
	escape   = "\x1b"
)

// errDiffRendererMisaligned is the contract failure: the program printed a
// different number of lines than it was given, so no output line can be
// trusted to belong to the parsed line of the same index.
var errDiffRendererMisaligned = errors.New("output does not line up with the diff")

// renderDiffLines pipes a whole unified diff document through the external
// renderer and returns one finished code cell per line of that document,
// indexed the way diff.Line.Source is. A nil slice with a nil error means
// there was nothing to render.
func renderDiffLines(ctx context.Context, name string, args []string, text string) ([]string, error) {
	input := splitDiffDocument(text)
	if len(input) == 0 {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(ctx, diffRendererTimeout)
	defer cancel()

	// A nil Env inherits this process's environment, which is what a renderer
	// configured through its own variables needs.
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(text)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	// The streams are buffers rather than files, so exec pipes them and Wait
	// blocks until every process holding a write end is gone. The deadline
	// above only reaches the program itself, so a renderer that left a child
	// behind — `diffRenderer: my-wrapper.sh`, a shell script that runs delta
	// without exec, is exactly that shape — would otherwise hold the pane on
	// its loading placeholder for as long as the child lives. WaitDelay closes
	// the pipes shortly after the program is done, whichever way it ended.
	cmd.WaitDelay = diffRendererWaitDelay

	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("%s: %w", name, ctxErr)
		}
		// ErrWaitDelay means the program finished on its own and only the
		// pipes outlived it: everything it printed is already in the buffer,
		// so the run counts as the success it was.
		if !errors.Is(err, exec.ErrWaitDelay) {
			return nil, rendererRunError(name, err, stderr.String())
		}
	}

	out := splitDiffDocument(stdout.String())
	if len(out) != len(input) {
		// The remedy leads, because the status panel is narrow and truncates
		// the tail: "add --color-only" has to survive, the line counts can go.
		return nil, fmt.Errorf("%s: add --color-only (or print one line per input line): %w — printed %d lines for %d lines of diff",
			name, errDiffRendererMisaligned, len(out), len(input))
	}

	cells := make([]string, len(out))
	for i, line := range out {
		cells[i] = diffRendererCell(line)
	}
	return cells, nil
}

// rendererRunError is what a failed run says: the program's own first line of
// standard error when it exited non-zero and complained, the exec failure
// otherwise. That failure is wrapped rather than restated, so both shapes of
// "there is no such program" — exec.ErrNotFound for a bare name that is not on
// $PATH, fs.ErrNotExist for a path that names nothing — stay reachable through
// errors.Is: a program that cannot be run turns the renderer off for the
// session.
func rendererRunError(name string, err error, stderr string) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if first := firstLine(stderr); first != "" {
			return errors.New(name + ": " + first)
		}
	}
	return fmt.Errorf("%s: %w", name, err)
}

// firstLine is the first non-empty line of text, trimmed.
func firstLine(text string) string {
	for line := range strings.SplitSeq(text, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// splitDiffDocument splits a diff document into lines exactly as diff.Parse
// does — one trailing newline trimmed, then a split on "\n" — so that an index
// into one split addresses the same line in the other.
func splitDiffDocument(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

// diffRendererCell turns one line of the program's output into the code cell
// the pane draws: the erase-in-line sequence stripped, the leading diff marker
// dropped (the pane draws its own, and a review thread takes that column over)
// and tabs expanded to the same stop the built-in rendering uses.
func diffRendererCell(line string) string {
	// A CRLF document reaches the program with its carriage returns, and one
	// inside a row would send the terminal's cursor back to column zero. The
	// parser drops them from its own lines for the same reason.
	line = strings.ReplaceAll(line, "\r", "")
	return expandTabsANSI(dropDiffMarker(stripEraseInLine(line)), views.DiffTabWidth)
}

// stripEraseInLine removes every erase-in-line sequence (CSI n K) from a line.
// delta closes each row with one so its tint runs to the edge of the terminal;
// in a column drawn next to gutters it would instead wipe the rest of the row.
// The pane repaints the tint itself, out to the width of the code column.
func stripEraseInLine(line string) string {
	if !strings.Contains(line, escape) {
		return line
	}

	var out strings.Builder
	out.Grow(len(line))
	for i := 0; i < len(line); {
		width := ansiSequenceLen(line, i)
		if width == 0 {
			out.WriteByte(line[i])
			i++
			continue
		}
		if !isEraseInLine(line[i : i+width]) {
			out.WriteString(line[i : i+width])
		}
		i += width
	}
	return out.String()
}

// isEraseInLine reports whether an escape sequence is CSI n K.
func isEraseInLine(seq string) bool {
	return len(seq) >= 3 && seq[1] == '[' && seq[len(seq)-1] == 'K'
}

// dropDiffMarker removes the '+', '-', ' ' or '\' a diff line opens with,
// keeping the escape sequences in front of it so the line stays coloured. A
// line whose first visible rune is anything else is handed back untouched.
func dropDiffMarker(line string) string {
	for i := 0; i < len(line); {
		if width := zeroWidthPrefixLen(line, i); width > 0 {
			i += width
			continue
		}
		switch line[i] {
		case '+', '-', ' ', '\\':
			return line[:i] + line[i+1:]
		default:
			return line
		}
	}
	return line
}

// expandTabsANSI expands tabs to a fixed stop counting visible columns only,
// so an escape sequence between two characters does not shift the stops. It is
// views.expandTabs with an eye for ANSI, which the built-in rendering does not
// need because it colours the text after expanding it.
func expandTabsANSI(line string, width int) string {
	if !strings.Contains(line, "\t") {
		return line
	}
	if width <= 0 {
		width = 1
	}

	var out strings.Builder
	out.Grow(len(line) + width)
	col := 0
	for i := 0; i < len(line); {
		if seq := zeroWidthPrefixLen(line, i); seq > 0 {
			out.WriteString(line[i : i+seq])
			i += seq
			continue
		}
		_, size := utf8.DecodeRuneInString(line[i:])
		if line[i] == '\t' {
			pad := width - col%width
			out.WriteString(strings.Repeat(" ", pad))
			col += pad
			i += size
			continue
		}
		out.WriteString(line[i : i+size])
		col += ansi.StringWidth(line[i : i+size])
		i += size
	}
	return out.String()
}

// zeroWidthPrefixLen is the byte length of whatever at text[start] costs the
// terminal no column: an escape sequence, or one of the C0 controls a reset
// can end with — `tput sgr0` under a tmux or screen terminfo closes with SI
// (shift in). Anything visible, the tab included, answers 0. It is what the
// scanning here has to agree on with charmbracelet's ansi, which measures and
// truncates the same cells in the view and counts all of this as zero width.
func zeroWidthPrefixLen(text string, start int) int {
	if seq := ansiSequenceLen(text, start); seq > 0 {
		return seq
	}
	if start < len(text) && text[start] < 0x20 && text[start] != '\t' {
		return 1
	}
	return 0
}

// ansiSequenceLen is the byte length of the escape sequence starting at
// text[start], or 0 when none does. It knows the three shapes a colouring
// program emits — CSI (ESC '[' parameters, then a final byte in 0x40..0x7E),
// OSC (ESC ']' … BEL or ST) and the nF escapes (see nfEscapeLen) — and treats
// an ESC followed by anything else as a two-byte sequence, which is enough to
// keep it out of the column count.
func ansiSequenceLen(text string, start int) int {
	if start >= len(text) || text[start] != escByte {
		return 0
	}
	if start+1 >= len(text) {
		return 1
	}

	switch text[start+1] {
	case '[':
		for i := start + 2; i < len(text); i++ {
			if text[i] >= 0x40 && text[i] <= 0x7E {
				return i - start + 1
			}
		}
	case ']':
		for i := start + 2; i < len(text); i++ {
			if text[i] == bellByte {
				return i - start + 1
			}
			if text[i] == escByte && i+1 < len(text) && text[i+1] == '\\' {
				return i - start + 2
			}
		}
	default:
		return nfEscapeLen(text, start)
	}
	// Unterminated: the rest of the line is the sequence, which at least keeps
	// it out of the visible columns.
	return len(text) - start
}

// nfEscapeLen measures an escape that is neither CSI nor OSC. The nF escapes —
// ESC, one or more intermediate bytes in 0x20..0x2F, then a final byte in
// 0x30..0x7E — are longer than two bytes and do turn up in a renderer's
// output: `tput sgr0` on an xterm terminfo opens the reset with ESC ( B, whose
// final 'B' read as text would be taken for the line's first visible rune and
// leave the diff marker in the cell. Everything else after an ESC is the
// two-byte shape.
func nfEscapeLen(text string, start int) int {
	if b := text[start+1]; b < 0x20 || b > 0x2F {
		return 2
	}
	for i := start + 2; i < len(text); i++ {
		switch {
		case text[i] >= 0x30 && text[i] <= 0x7E:
			return i - start + 1
		case text[i] < 0x20 || text[i] > 0x2F:
			// Not an nF escape after all; stop in front of the stray byte.
			return i - start
		}
	}
	return len(text) - start
}
