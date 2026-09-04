// Package diff parses git-style unified diffs into files, hunks and lines.
//
// The input is the raw text of a diff as git writes it and as Bitbucket's
// /diff endpoints return it. Parse walks the text once and hands back a
// [File] per file section, each carrying its [Hunk] list with per-line old
// and new line numbers already assigned, which is what a diff viewer needs
// to draw a gutter.
//
// Parsing is deliberately lenient, because the text comes off the wire and
// may be clipped by a timeout or a size limit: unrecognised header lines are
// skipped, a truncated final hunk is kept with the lines that did arrive,
// a lone trailing carriage return is dropped, and bytes are never validated
// as UTF-8 — invalid sequences pass through untouched. The only input Parse
// rejects is a hunk header that appears before any file header, since there
// is nothing to attach it to.
//
// The package is pure: it depends on the standard library only and knows
// nothing about Bitbucket, the TUI or the configuration.
package diff
