package bitbucket

import (
	"html"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// mentionOpen is the two bytes every stored mention starts with, and the cheap
// test that answers "this content has no mention in it" without a regex.
const mentionOpen = "@{"

// mentionSentinel brackets an id while the rendering is walked in order. A NUL
// has no business in html, and renderedSequence scrubs any the rendering
// happens to carry before it writes its own, so nothing else can look like one.
const mentionSentinel = "\x00"

// mentionRE matches the form Bitbucket stores a mention in: @{account_id}.
// Atlassian documents an account id as ^[a-zA-Z0-9_|\-:]{1,128}$, so the two
// shapes seen on the wire — a 24-hex id and "<digits>:<uuid>" — are examples
// rather than the grammar, and the whole documented charset is matched.
var mentionRE = regexp.MustCompile(`@\{([A-Za-z0-9_|:-]{1,128})\}`)

// mentionSpanRE matches the form Bitbucket renders a mention in. It keys on
// data-atlassian-id and not on the ap-mention class, which is decoration:
// either quote, extra attributes in any order, whitespace around the "=" and an
// uppercase tag all pass. Go's RE2 has no backreferences, so the closing tag is
// spelled out rather than matched to the opening one — every mention on the
// wire is a span, so there is nothing else it could close.
var mentionSpanRE = regexp.MustCompile(
	`(?is)<span\b[^>]*?\bdata-atlassian-id\s*=\s*["']([^"']+)["'][^>]*>(.*?)</span\s*>`)

// mentionSentinelRE finds the sentinels renderedSequence left behind.
var mentionSentinelRE = regexp.MustCompile(
	mentionSentinel + "([^" + mentionSentinel + "]*)" + mentionSentinel)

// tagRE matches one HTML tag, which is all the stripping a mention needs:
// Bitbucket serves a fragment, and the text between the tags is the part a
// display name is made of.
var tagRE = regexp.MustCompile(`(?s)<[^>]*>`)

// MentionNames maps account id to display name for every mention span in
// rendered — the html Bitbucket serves next to the raw Markdown, and the only
// place the names appear at all. The span text is stripped of tags (a name can
// carry markup of its own), entity-unescaped, trimmed and relieved of its
// leading "@"; an id whose text is empty after that is skipped, since it names
// nobody. A repeated id keeps the last value, which on the wire is the same
// value. It answers nil when the rendering carries no mention.
func MentionNames(rendered string) map[string]string {
	names, _ := mentionSpans(rendered)
	return names
}

// maxMentionNameRunes bounds a display name. Atlassian caps a profile name far
// below it; anything longer is a span that ran past its own end.
const maxMentionNameRunes = 255

// mentionSpans is one pass over the rendering's mention spans: the display name
// per id, as MentionNames documents it, and how many spans each id has, which
// is what decides whether every raw occurrence was rendered. Both are nil when
// the rendering carries no mention span at all.
func mentionSpans(rendered string) (map[string]string, map[string]int) {
	matches := mentionSpanRE.FindAllStringSubmatch(rendered, -1)
	if len(matches) == 0 {
		return nil, nil
	}

	names := make(map[string]string, len(matches))
	counts := make(map[string]int, len(matches))
	for _, match := range matches {
		counts[match[1]]++
		name, ok := mentionSpanName(match[2])
		if !ok {
			continue
		}
		names[match[1]] = name
	}
	if len(names) == 0 {
		return nil, counts
	}
	return names, counts
}

// mentionSpanName reduces the text of a mention span to the display name, and
// reports whether there is one worth using. There is not when the text is
// empty, when it runs into another span — a mention span that lost its closing
// tag would otherwise swallow the document up to the next one, and a span
// nested inside the name would cut it short — or when it is longer than any
// display name Atlassian allows. Whitespace runs, line breaks included,
// collapse to one space, so a name can never end the Markdown block it sits in.
func mentionSpanName(text string) (string, bool) {
	if strings.Contains(strings.ToLower(text), "<span") {
		return "", false
	}
	plain := html.UnescapeString(tagRE.ReplaceAllString(text, ""))
	name := strings.TrimSpace(strings.TrimPrefix(strings.Join(strings.Fields(plain), " "), "@"))
	if name == "" || utf8.RuneCountInString(name) > maxMentionNameRunes {
		return "", false
	}
	return name, true
}

// ResolveMentions returns raw with the mentions Bitbucket rendered replaced by
// "@" and the display name, using rendered as the oracle for both the names and
// for which occurrences were rendered at all. An id Bitbucket could not resolve
// — a departed colleague — and an occurrence it left literal, inside a code
// span or a fence, stay exactly as written, which is what the web UI shows too.
//
// The names are escaped for a Markdown renderer, not for a text round trip: see
// escapeMentionName. Anything that has to round-trip the text — the description
// editor, a custom-command template — keeps the raw form.
func ResolveMentions(raw, rendered string) string {
	return ResolveMentionsFunc(raw, rendered, nil)
}

// ResolveMentionsFunc is ResolveMentions with write deciding what each resolved
// mention becomes. write receives the text ResolveMentions would have written —
// "@" and the escaped display name — and returns the text to write in its
// place, which is how a renderer wraps a mention in markers it can find again
// once the Markdown has been rendered and style it on its own terms. A nil
// write is ResolveMentions.
func ResolveMentionsFunc(raw, rendered string, write func(mention string) string) string {
	if rendered == "" || !strings.Contains(raw, mentionOpen) {
		return raw
	}

	names, spans := mentionSpans(rendered)
	if len(names) == 0 {
		return raw
	}

	rawMatches := mentionRE.FindAllStringSubmatchIndex(raw, -1)
	if len(rawMatches) == 0 {
		return raw
	}

	if mentionsAllRendered(raw, rawMatches, names, spans) {
		return replaceMentions(raw, rawMatches, names, nil, write)
	}

	// Some known id is literal somewhere in the rendering — the same person
	// named once in prose and once in a code span. Only the order the two forms
	// appear in can say which raw occurrence is which.
	if resolved, ok := alignMentions(renderedSequence(rendered), raw, rawMatches); ok {
		return replaceMentions(raw, rawMatches, names, resolved, write)
	}
	return replaceMentions(raw, rawMatches, names, nil, write)
}

// mentionsAllRendered reports whether every id raw names and names knows is
// rendered as a span (spans counts them per id) at least as often as raw
// writes it — the overwhelmingly common case, where nothing was left literal
// and no ordered walk is needed.
func mentionsAllRendered(raw string, rawMatches [][]int, names map[string]string, spans map[string]int) bool {
	inRaw := make(map[string]int, len(rawMatches))
	for _, match := range rawMatches {
		id := submatch(raw, match)
		if _, known := names[id]; known {
			inRaw[id]++
		}
	}

	for id, count := range inRaw {
		if spans[id] < count {
			return false
		}
	}
	return true
}

// mentionEvent is one mention in the order the rendering walks them: the id it
// names, and whether Bitbucket turned it into a span or left it as plain text.
type mentionEvent struct {
	id       string
	rendered bool
}

// renderedSequence is every mention in the rendering, in order, rendered ones
// and literal ones alike. Each span becomes a sentinel around its id, the
// remaining tags are stripped and the entities decoded, which is what pulls a
// literal @{id} back together after Bitbucket's syntax highlighter split it
// across three spans inside a fence.
func renderedSequence(rendered string) []mentionEvent {
	// The sentinel has to be ours alone: a NUL the rendering happened to carry
	// would pair up with a real one and scramble the whole sequence.
	clean := strings.ReplaceAll(rendered, mentionSentinel, "�")
	marked := mentionSpanRE.ReplaceAllString(clean, mentionSentinel+"${1}"+mentionSentinel)
	// A tag goes, but a literal mention inside one — an @{id} Bitbucket kept in
	// a link's href — stays where it was, or the walk would be one event short
	// and the alignment would fail for the wrong reason.
	text := html.UnescapeString(tagRE.ReplaceAllStringFunc(marked, literalMentionsIn))

	spans := mentionSentinelRE.FindAllStringSubmatchIndex(text, -1)
	literals := mentionRE.FindAllStringSubmatchIndex(text, -1)

	events := make([]mentionEvent, 0, len(spans)+len(literals))
	spanAt, literalAt := 0, 0
	for spanAt < len(spans) || literalAt < len(literals) {
		takeSpan := literalAt >= len(literals)
		if !takeSpan && spanAt < len(spans) {
			takeSpan = spans[spanAt][0] < literals[literalAt][0]
		}
		if takeSpan {
			events = append(events, mentionEvent{id: submatch(text, spans[spanAt]), rendered: true})
			spanAt++
			continue
		}
		events = append(events, mentionEvent{id: submatch(text, literals[literalAt]), rendered: false})
		literalAt++
	}
	return events
}

// literalMentionsIn is what a stripped tag leaves behind: nothing, unless its
// attributes carry literal mentions, which stay as text in the tag's place.
func literalMentionsIn(tag string) string {
	return strings.Join(mentionRE.FindAllString(tag, -1), " ")
}

// alignMentions pairs the rendering's mentions with raw's, one for one, and
// answers which raw occurrences Bitbucket rendered. It refuses the pairing
// unless the two sequences are the same length and name the same ids in the
// same order: anything else means the rendering is not the one this raw was
// rendered from, and guessing would resolve the wrong occurrence.
func alignMentions(events []mentionEvent, raw string, rawMatches [][]int) ([]bool, bool) {
	if len(events) != len(rawMatches) {
		return nil, false
	}

	resolved := make([]bool, len(events))
	for index, event := range events {
		if event.id != submatch(raw, rawMatches[index]) {
			return nil, false
		}
		resolved[index] = event.rendered
	}
	return resolved, true
}

// replaceMentions rewrites the matched occurrences of raw in one pass, so a
// display name that itself looks like a mention is never re-matched. resolved
// says, position by position, which occurrences to rewrite; nil rewrites every
// occurrence whose id names knows. An id names does not know is left alone.
// write, when not nil, has the last word on each mention's text.
func replaceMentions(
	raw string, rawMatches [][]int, names map[string]string, resolved []bool, write func(string) string,
) string {
	var builder strings.Builder
	builder.Grow(len(raw))

	written := 0
	for index, match := range rawMatches {
		if resolved != nil && !resolved[index] {
			continue
		}
		name, known := names[submatch(raw, match)]
		if !known {
			continue
		}
		mention := "@" + escapeMentionName(name)
		if write != nil {
			mention = write(mention)
		}
		builder.WriteString(raw[written:match[0]])
		builder.WriteString(mention)
		written = match[1]
	}

	if written == 0 {
		return raw
	}
	builder.WriteString(raw[written:])
	return builder.String()
}

// submatch is the first capture group of a FindAllStringSubmatchIndex match.
func submatch(text string, match []int) string { return text[match[2]:match[3]] }

// escapeMentionName encodes every ASCII punctuation and symbol rune of a
// display name as a numeric HTML entity, leaving letters, digits, spaces and
// every non-ASCII rune alone.
//
// The output is for a Markdown renderer and nothing else. glamour honours
// backslash escapes for only a fixed set of characters but decodes numeric
// entities everywhere it matters — paragraphs, headings, list items, table
// cells, link text, inside emphasis — so entities are the one mechanism that
// renders "J_Doe_", "Tom & Jerry" or ":tada:" as the name its owner chose
// rather than as emphasis, as raw HTML or as an emoji. The one place glamour
// prints the source instead, entities included, is an image's alt text; a
// mention inside an image label is not worth an exception.
func escapeMentionName(name string) string {
	if !strings.ContainsFunc(name, needsMentionEscape) {
		return name
	}

	var builder strings.Builder
	builder.Grow(len(name) + len(name)/2)
	for _, char := range name {
		if !needsMentionEscape(char) {
			builder.WriteRune(char)
			continue
		}
		builder.WriteString("&#")
		builder.WriteString(strconv.Itoa(int(char)))
		builder.WriteString(";")
	}
	return builder.String()
}

// needsMentionEscape reports whether a rune is ASCII punctuation or an ASCII
// symbol — every rune in 0x21-0x2F, 0x3A-0x40, 0x5B-0x60 and 0x7B-0x7E, which
// is the whole of what a Markdown renderer can read as markup.
func needsMentionEscape(char rune) bool {
	return char < utf8.RuneSelf && (unicode.IsPunct(char) || unicode.IsSymbol(char))
}

// Readable is Raw with the mentions Bitbucket rendered made readable — see
// ResolveMentions. It is the plain readable form; the detail pane goes through
// ResolveMentionsFunc instead so it can colour the mentions once rendered, and
// anything that round-trips the text keeps Raw.
func (c RenderedContent) Readable() string { return ResolveMentions(c.Raw, c.HTML) }

// ReadableDescription is Description with its mentions resolved from
// Summary.HTML, the rendering Bitbucket serves beside it. It reads Description
// rather than Summary.Raw because a caller may carry the one without the other.
func (pr *PullRequest) ReadableDescription() string {
	return ResolveMentions(pr.Description, pr.Summary.HTML)
}
