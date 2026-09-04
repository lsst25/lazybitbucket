package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/tui/theme"
)

// defaultWrapWidth is used when a caller passes a non-positive width.
const defaultWrapWidth = 80

// wrapText word-wraps text to width display columns, paragraph by paragraph.
// It measures display width rather than bytes, so multi-byte and wide runes
// do not overflow the panel.
func wrapText(text string, width int) []string {
	if width <= 0 {
		width = defaultWrapWidth
	}
	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		if lipgloss.Width(paragraph) <= width {
			lines = append(lines, paragraph)
			continue
		}
		runes := []rune(paragraph)
		for len(runes) > 0 {
			w := 0
			cut := 0
			for i, r := range runes {
				rw := lipgloss.Width(string(r))
				if w+rw > width {
					break
				}
				w += rw
				cut = i + 1
			}
			if cut == 0 {
				cut = 1 // at least one rune to avoid an infinite loop
			}
			if cut < len(runes) {
				// Back up to the last space for word-wrap.
				for j := cut - 1; j > 0; j-- {
					if runes[j] == ' ' {
						cut = j
						break
					}
				}
			}
			lines = append(lines, string(runes[:cut]))
			runes = runes[cut:]
			for len(runes) > 0 && runes[0] == ' ' {
				runes = runes[1:]
			}
		}
	}
	return lines
}

// hardWrapLine splits a single line into chunks of at most width display
// columns without looking for word boundaries. Pipeline step logs contain long
// unbroken tokens (paths, hashes, base64) that word-wrapping would leave
// overflowing the panel.
func hardWrapLine(line string, width int) []string {
	if width <= 0 {
		width = defaultWrapWidth
	}
	if lipgloss.Width(line) <= width {
		return []string{line}
	}
	var (
		chunks []string
		cur    []rune
		curW   int
	)
	for _, r := range line {
		rw := lipgloss.Width(string(r))
		if curW+rw > width && len(cur) > 0 {
			chunks = append(chunks, string(cur))
			cur = cur[:0]
			curW = 0
		}
		cur = append(cur, r)
		curW += rw
	}
	if len(cur) > 0 {
		chunks = append(chunks, string(cur))
	}
	return chunks
}

// padRight pads s with spaces to w columns using ANSI-aware display width.
func padRight(s string, w int) string {
	vis := lipgloss.Width(s)
	if vis >= w {
		return s
	}
	return s + strings.Repeat(" ", w-vis)
}

// padLeft pads s with spaces to w columns using ANSI-aware display width,
// which right-aligns it in a fixed column.
func padLeft(s string, w int) string {
	vis := lipgloss.Width(s)
	if vis >= w {
		return s
	}
	return strings.Repeat(" ", w-vis) + s
}

// expandTabs replaces tabs with spaces up to the next multiple-of-width tab
// stop. Diff rows are laid out in fixed columns and hard-truncated, so a
// literal tab would leave the terminal to decide where the code column starts.
func expandTabs(s string, width int) string {
	if !strings.Contains(s, "\t") {
		return s
	}
	if width <= 0 {
		width = 1
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		if r == '\t' {
			pad := width - col%width
			b.WriteString(strings.Repeat(" ", pad))
			col += pad
			continue
		}
		b.WriteRune(r)
		col += lipgloss.Width(string(r))
	}
	return b.String()
}

// urlStyle is the style applied to URLs found in rendered text.
func urlStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.ColorCyan).Underline(true)
}

// colorURLs highlights http/https URLs in a single line. It scans forward
// rather than re-searching the styled result, which would loop forever whenever
// the active colour profile renders styles as a no-op.
func colorURLs(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		offset := -1
		for _, prefix := range []string{"https://", "http://"} {
			if idx := strings.Index(s[i:], prefix); idx >= 0 && (offset == -1 || idx < offset) {
				offset = idx
			}
		}
		if offset == -1 {
			b.WriteString(s[i:])
			break
		}
		start := i + offset
		b.WriteString(s[i:start])
		rest := s[start:]
		end := strings.IndexAny(rest, " \t\n")
		if end == -1 {
			end = len(rest)
		}
		b.WriteString(urlStyle().Render(rest[:end]))
		i = start + end
	}
	return b.String()
}

// colorURLsWrapped highlights URLs across wrapped lines. When wrapText split a
// URL, the continuation on the following line is highlighted too.
func colorURLsWrapped(lines []string) []string {
	result := make([]string, len(lines))
	urlCont := false
	for i, line := range lines {
		if urlCont {
			end := strings.IndexAny(line, " \t")
			if end == -1 {
				result[i] = urlStyle().Render(line)
				urlCont = true
				continue
			}
			result[i] = urlStyle().Render(line[:end]) + colorURLs(line[end:])
		} else {
			result[i] = colorURLs(line)
		}
		urlCont = lineEndsInURL(lines[i])
	}
	return result
}

// lineEndsInURL reports whether the raw line ends inside a URL.
func lineEndsInURL(line string) bool {
	lastURL := strings.LastIndex(line, "https://")
	if idx := strings.LastIndex(line, "http://"); idx > lastURL {
		lastURL = idx
	}
	if lastURL == -1 {
		return false
	}
	return !strings.ContainsAny(line[lastURL:], " \t")
}

// findURLs extracts http/https URLs from free text.
func findURLs(text string) []string {
	var urls []string
	for _, word := range strings.Fields(text) {
		word = strings.TrimLeft(word, "([{<\"'")
		word = strings.TrimRight(word, ".,;:!?)]}>\"'")
		if strings.HasPrefix(word, "http://") || strings.HasPrefix(word, "https://") {
			urls = append(urls, word)
		}
	}
	return urls
}

// timeAgo renders a compact relative age: "3h", "2d", "5mo". It is the short
// form used in list columns; callers that want prose append " ago".
// The zero time renders as "" so an unset timestamp leaves the column blank.
func timeAgo(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	// Clock skew or a timestamp that has not happened yet (the demo dataset is
	// shifted relative to now) must never print a negative age.
	d := max(time.Since(t), 0)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	default:
		return fmt.Sprintf("%dmo", int(d.Hours()/(24*30)))
	}
}

// URLGroup is a named group of URLs for the URL picker.
type URLGroup struct {
	Section string
	URLs    []string
}

// ExtractURLs collects the URLs reachable from a pull request, grouped by where
// they came from. Duplicates are dropped across groups, first occurrence wins.
// commits and threads are the review tabs' data, which is nil until the user
// has opened them; the groups they feed are then simply absent.
func ExtractURLs(
	pr *bitbucket.PullRequest,
	statuses []bitbucket.CommitStatus,
	commits []bitbucket.Commit,
	threads []bitbucket.Thread,
) []URLGroup {
	if pr == nil {
		return nil
	}
	seen := make(map[string]bool)
	add := func(urls *[]string, u string) {
		if u != "" && !seen[u] {
			seen[u] = true
			*urls = append(*urls, u)
		}
	}

	var groups []URLGroup

	var self []string
	add(&self, pr.HTMLURL())
	if len(self) > 0 {
		groups = append(groups, URLGroup{Section: "Pull request", URLs: self})
	}

	var body []string
	for _, u := range findURLs(pr.Description) {
		add(&body, u)
	}
	if len(body) > 0 {
		groups = append(groups, URLGroup{Section: "Description", URLs: body})
	}

	var builds []string
	for _, s := range statuses {
		add(&builds, s.URL)
	}
	if len(builds) > 0 {
		groups = append(groups, URLGroup{Section: "Builds", URLs: builds})
	}

	var commitURLs []string
	for _, commit := range commits {
		add(&commitURLs, commit.HTMLURL())
	}
	if len(commitURLs) > 0 {
		groups = append(groups, URLGroup{Section: "Commits", URLs: commitURLs})
	}

	var threadURLs []string
	for _, thread := range threads {
		add(&threadURLs, thread.Root.HTMLURL())
	}
	if len(threadURLs) > 0 {
		groups = append(groups, URLGroup{Section: "Comments", URLs: threadURLs})
	}

	return groups
}
