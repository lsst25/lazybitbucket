package bitbucket

import (
	"strings"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// mentionSpan is the markup Bitbucket wraps a mention it could resolve in.
func mentionSpan(id, name string) string {
	return `<span class="ap-mention" data-atlassian-id="` + id + `">@` + name + `</span>`
}

// mentionRaw is the form Bitbucket stores a mention in.
func mentionRaw(id string) string { return "@{" + id + "}" }

// paragraph wraps a rendering in the <p> Bitbucket's renderer puts around a
// line of prose.
func paragraph(inner string) string { return "<p>" + inner + "</p>" }

// assertMentionNames compares a MentionNames answer with the pairs a test
// expects, one id at a time so a failure names the one that went wrong. No
// pairs at all means the answer has to be nil, not an empty map.
func assertMentionNames(t *testing.T, got, want map[string]string) {
	t.Helper()

	testkit.AssertEqual(t, "len(names)", len(got), len(want))
	if len(want) == 0 {
		testkit.AssertEqual(t, "names is nil", got == nil, true)
		return
	}
	for id, name := range want {
		testkit.AssertEqual(t, "names["+id+"]", got[id], name)
	}
}

func TestResolveMentions_RendersWhatBitbucketRendered(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		raw      string
		rendered string
		want     string
	}{
		{
			// Both of these are shaped like wire samples, the html as served: a fragment,
			// not a document, and not even a closed <p> in the first one.
			name:     "a 24-hex account id",
			raw:      "Hi " + mentionRaw(testMentionID) + " . Thanks",
			rendered: "<p>Hi " + mentionSpan(testMentionID, testMentionName) + " . Thanks",
			want:     "Hi @" + testMentionName + " . Thanks",
		},
		{
			name:     "a digits:uuid account id",
			raw:      mentionRaw(testMentionUUID) + " PR updated",
			rendered: paragraph(mentionSpan(testMentionUUID, testMentionSecondName) + " PR updated"),
			want:     "@" + testMentionSecondName + " PR updated",
		},
		{
			name: "two ids in one body",
			raw:  "cc " + mentionRaw(testMentionID) + " " + mentionRaw(testMentionUUID),
			rendered: paragraph("cc " + mentionSpan(testMentionID, testMentionName) + " " +
				mentionSpan(testMentionUUID, testMentionSecondName)),
			want: "cc @" + testMentionName + " @" + testMentionSecondName,
		},
		{
			name: "an id Bitbucket left literal in its own rendering stays literal",
			raw:  "Hi " + mentionRaw(testMentionID) + ", " + mentionRaw(testMentionGoneID) + " wrote this.",
			rendered: paragraph("Hi " + mentionSpan(testMentionID, testMentionName) + ", " +
				mentionRaw(testMentionGoneID) + " wrote this."),
			want: "Hi @" + testMentionName + ", " + mentionRaw(testMentionGoneID) + " wrote this.",
		},
		{
			name: "adjacent mentions",
			raw:  mentionRaw(testMentionID) + mentionRaw(testMentionUUID),
			rendered: paragraph(mentionSpan(testMentionID, testMentionName) +
				mentionSpan(testMentionUUID, testMentionSecondName)),
			want: "@" + testMentionName + "@" + testMentionSecondName,
		},
		{
			name: "a <br> between the spans",
			raw:  mentionRaw(testMentionID) + "\n" + mentionRaw(testMentionUUID),
			rendered: paragraph(mentionSpan(testMentionID, testMentionName) + "<br>" +
				mentionSpan(testMentionUUID, testMentionSecondName)),
			want: "@" + testMentionName + "\n@" + testMentionSecondName,
		},
		{
			name: "a mention inside link text",
			raw:  "See [" + mentionRaw(testMentionID) + "](https://example.com)",
			rendered: paragraph(`See <a href="https://example.com">` +
				mentionSpan(testMentionID, testMentionName) + "</a>"),
			want: "See [@" + testMentionName + "](https://example.com)",
		},
		{
			name:     "CRLF line endings survive",
			raw:      "Hi " + mentionRaw(testMentionID) + "\r\nThanks",
			rendered: paragraph("Hi " + mentionSpan(testMentionID, testMentionName) + "<br>Thanks"),
			want:     "Hi @" + testMentionName + "\r\nThanks",
		},
		{
			name:     "a zero-width non-joiner right after the mention",
			raw:      mentionRaw(testMentionID) + "\u200c ping",
			rendered: paragraph(mentionSpan(testMentionID, testMentionName) + "\u200c ping"),
			want:     "@" + testMentionName + "\u200c ping",
		},
		{
			// One pass over the raw match positions: the substituted name is
			// never looked at again, so a name that reads like a mention cannot
			// start a second round of substitution.
			name:     "a display name that itself looks like a mention",
			raw:      mentionRaw(testMentionID) + " ping",
			rendered: paragraph(mentionSpan(testMentionID, "Bot @{abc}")),
			want:     "@Bot &#64;&#123;abc&#125; ping",
		},
		{
			name:     "entities in the span text decode into the name and are re-encoded",
			raw:      mentionRaw(testMentionID),
			rendered: paragraph(mentionSpan(testMentionID, "Tom &amp; Jerry")),
			want:     "@Tom &#38; Jerry",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			testkit.AssertEqual(t, "ResolveMentions", ResolveMentions(test.raw, test.rendered), test.want)
		})
	}
}

func TestResolveMentions_OrderedWalkPicksTheRenderedOccurrence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		raw      string
		rendered string
		want     string
	}{
		{
			name: "the second occurrence sits in a code span",
			raw:  "Ping " + mentionRaw(testMentionID) + " and `" + mentionRaw(testMentionID) + "` in code.",
			rendered: paragraph("Ping " + mentionSpan(testMentionID, testMentionName) +
				" and <code>" + mentionRaw(testMentionID) + "</code> in code."),
			want: "Ping @" + testMentionName + " and `" + mentionRaw(testMentionID) + "` in code.",
		},
		{
			name: "the literal one comes first",
			raw:  "`" + mentionRaw(testMentionID) + "` is the id; ask " + mentionRaw(testMentionID) + ".",
			rendered: paragraph("<code>" + mentionRaw(testMentionID) + "</code> is the id; ask " +
				mentionSpan(testMentionID, testMentionName) + "."),
			want: "`" + mentionRaw(testMentionID) + "` is the id; ask @" + testMentionName + ".",
		},
		{
			// Bitbucket renders with Python-Markdown, so a fence comes back
			// Pygments-tokenised and the literal id is split over three spans.
			// Stripping the tags is what puts it back together.
			name: "a fence Bitbucket tokenised, the id split across spans",
			raw: "See " + mentionRaw(testMentionID) + ".\n\n```go\n// " +
				mentionRaw(testMentionID) + "\n```",
			rendered: paragraph("See "+mentionSpan(testMentionID, testMentionName)+".") + "\n" +
				`<div class="codehilite"><pre><code><span class="c1">// @{</span>` +
				`<span class="n">` + testMentionID + `</span><span class="p">}</span>` +
				"\n</code></pre></div>",
			want: "See @" + testMentionName + ".\n\n```go\n// " + mentionRaw(testMentionID) + "\n```",
		},
		{
			// The rendering carries one mention where raw has two, and no
			// literal to pair the second with — nothing to align, so every
			// known id is replaced rather than an arbitrary one of them.
			name:     "a rendering that cannot be aligned replaces every known id",
			raw:      mentionRaw(testMentionID) + " and " + mentionRaw(testMentionID),
			rendered: paragraph(mentionSpan(testMentionID, testMentionName)),
			want:     "@" + testMentionName + " and @" + testMentionName,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			testkit.AssertEqual(t, "ResolveMentions", ResolveMentions(test.raw, test.rendered), test.want)
		})
	}
}

func TestResolveMentions_LeavesRawAlone(t *testing.T) {
	t.Parallel()

	rendered := paragraph("Hi " + mentionSpan(testMentionID, testMentionName))

	tests := []struct {
		name     string
		raw      string
		rendered string
	}{
		{name: "raw carries no mention at all", raw: "Nothing to resolve here", rendered: rendered},
		{name: "no rendering to read the names from", raw: "Hi " + mentionRaw(testMentionID), rendered: ""},
		{
			name:     "a rendering with no mention span",
			raw:      "Hi " + mentionRaw(testMentionID),
			rendered: paragraph("Hi " + mentionRaw(testMentionID)),
		},
		{
			// A stale pairing: the rendering names somebody raw no longer does.
			name:     "an id the rendering never names",
			raw:      "Hi " + mentionRaw(testMentionGoneID),
			rendered: rendered,
		},
		{name: "an unterminated @{", raw: "Hi @{" + testMentionID, rendered: rendered},
		{name: "an empty id", raw: "Hi @{}", rendered: rendered},
		{name: "a blank id", raw: "Hi @{ }", rendered: rendered},
		{name: "an id with a space in it", raw: "Hi @{foo: 1}", rendered: rendered},
		{name: "an id with a character outside the charset", raw: "Hi @{5a73/13df}", rendered: rendered},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			testkit.AssertEqual(t, "ResolveMentions", ResolveMentions(test.raw, test.rendered), test.raw)
		})
	}
}

func TestMentionNames(t *testing.T) {
	t.Parallel()

	resolved := map[string]string{testMentionID: testMentionName}

	tests := []struct {
		name     string
		rendered string
		want     map[string]string
	}{
		{
			name:     "the span Bitbucket emits",
			rendered: paragraph("Hi " + mentionSpan(testMentionID, testMentionName)),
			want:     resolved,
		},
		{
			name: "data-atlassian-id before the class",
			rendered: `<span data-atlassian-id="` + testMentionID + `" class="ap-mention">@` +
				testMentionName + `</span>`,
			want: resolved,
		},
		{
			name: "single quotes",
			rendered: `<span class='ap-mention' data-atlassian-id='` + testMentionID + `'>@` +
				testMentionName + `</span>`,
			want: resolved,
		},
		{
			name: "extra attributes and space around the =",
			rendered: `<span class="ap-mention" data-atlassian-id = "` + testMentionID +
				`" title="mention">@` + testMentionName + `</span>`,
			want: resolved,
		},
		{
			name: "an uppercase tag and attribute",
			rendered: `<SPAN CLASS="ap-mention" DATA-ATLASSIAN-ID="` + testMentionID + `">@` +
				testMentionName + `</SPAN>`,
			want: resolved,
		},
		{
			name: "a newline inside the tag",
			rendered: "<span class=\"ap-mention\"\n      data-atlassian-id=\"" + testMentionID +
				"\">@" + testMentionName + "</span>",
			want: resolved,
		},
		{
			name: "a space before the closing bracket",
			rendered: `<span data-atlassian-id="` + testMentionID + `">@` + testMentionName +
				`</span >`,
			want: resolved,
		},
		{
			name:     "markup inside the span text",
			rendered: `<span data-atlassian-id="` + testMentionID + `">@<b>Margaret</b> Hamilton</span>`,
			want:     resolved,
		},
		{
			name:     "named entities in the name",
			rendered: `<span data-atlassian-id="` + testMentionID + `">@Tom &amp; Jerry &lt;Ops&gt;</span>`,
			want:     map[string]string{testMentionID: "Tom & Jerry <Ops>"},
		},
		{
			name:     "a decimal entity and a named non-ASCII one",
			rendered: `<span data-atlassian-id="` + testMentionID + `">@Andr&#233; M&uuml;ller</span>`,
			want:     map[string]string{testMentionID: "André Müller"},
		},
		{
			name:     "a hexadecimal entity",
			rendered: `<span data-atlassian-id="` + testMentionID + `">@O&#x27;Brien</span>`,
			want:     map[string]string{testMentionID: "O'Brien"},
		},
		{
			name: "two spans",
			rendered: paragraph(mentionSpan(testMentionID, testMentionName) + " and " +
				mentionSpan(testMentionUUID, testMentionSecondName)),
			want: map[string]string{testMentionID: testMentionName, testMentionUUID: testMentionSecondName},
		},
		{
			name: "a repeated id keeps the last name",
			rendered: paragraph(mentionSpan(testMentionID, "Stale Name") + " " +
				mentionSpan(testMentionID, testMentionName)),
			want: resolved,
		},
		{
			name: "an empty span is skipped, its neighbour is not",
			rendered: paragraph(`<span data-atlassian-id="`+testMentionUUID+`">  </span>`) +
				paragraph(mentionSpan(testMentionID, testMentionName)),
			want: resolved,
		},
		{
			name:     "a span carrying nothing but the @",
			rendered: `<span data-atlassian-id="` + testMentionID + `">@</span>`,
			want:     nil,
		},
		{
			name:     "a span with no data-atlassian-id",
			rendered: `<span class="ap-mention">@` + testMentionName + `</span>`,
			want:     nil,
		},
		{
			// A Jira issue link carries a principal uuid, which is not an
			// account id and never names a person.
			name: "a Jira issue link is not a mention",
			rendered: `<a href="https://example.atlassian.net/browse/PROJ-1"` +
				` data-principal-uuid="{557058:d7586a23}" class="ap-connect-dialog">PROJ-1</a>`,
			want: nil,
		},
		{name: "prose with no markup", rendered: "Hi " + mentionRaw(testMentionID), want: nil},
		{name: "nothing at all", rendered: "", want: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assertMentionNames(t, MentionNames(test.rendered), test.want)
		})
	}
}

func TestEscapeMentionName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		display string
		want    string
	}{
		{name: "underscores would italicise", display: "J_Doe_", want: "J&#95;Doe&#95;"},
		{name: "asterisks would emphasise", display: "J*Doe*", want: "J&#42;Doe&#42;"},
		{name: "tildes would strike through", display: "~~Bob~~", want: "&#126;&#126;Bob&#126;&#126;"},
		{name: "an ampersand would open an entity", display: "Tom & Jerry", want: "Tom &#38; Jerry"},
		{name: "angle brackets would be raw HTML", display: "Ann <Ops>", want: "Ann &#60;Ops&#62;"},
		{name: "a pipe would split a table cell", display: "J|Doe", want: "J&#124;Doe"},
		{name: "a dot would be linkified", display: "Grace.Hopper", want: "Grace&#46;Hopper"},
		{name: "colons would be an emoji", display: ":tada:", want: "&#58;tada&#58;"},
		{name: "letters and a space are left alone", display: testMentionSecondName, want: testMentionSecondName},
		{name: "digits are left alone", display: "Agent 007", want: "Agent 007"},
		{name: "non-ASCII letters are left alone", display: "Ünter Müller", want: "Ünter Müller"},
		{name: "non-ASCII scripts are left alone", display: "山田 太郎", want: "山田 太郎"},
		{name: "an em dash is not ASCII punctuation", display: "Ann — Ops", want: "Ann — Ops"},
		{name: "nothing to escape", display: "", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			testkit.AssertEqual(t, "escapeMentionName", escapeMentionName(test.display), test.want)
		})
	}
}

func TestRenderedContent_Readable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content RenderedContent
		want    string
	}{
		{
			name: "the mention the html names",
			content: RenderedContent{
				Raw:  "Hi " + mentionRaw(testMentionID),
				HTML: paragraph("Hi " + mentionSpan(testMentionID, testMentionName)),
			},
			want: "Hi @" + testMentionName,
		},
		{
			name: "no html at all, as a body built client-side carries",
			content: RenderedContent{
				Raw:  "Hi " + mentionRaw(testMentionID),
				HTML: "",
			},
			want: "Hi " + mentionRaw(testMentionID),
		},
		{
			// BuildThreads empties Raw on a deleted comment it keeps as a
			// placeholder, and the html it was served with stays behind.
			name: "a deleted comment's placeholder",
			content: RenderedContent{
				Raw:  "",
				HTML: paragraph(mentionSpan(testMentionID, testMentionName)),
			},
			want: "",
		},
		{name: "the zero value", content: RenderedContent{}, want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			testkit.AssertEqual(t, "Readable", test.content.Readable(), test.want)
		})
	}
}

func TestPullRequest_ReadableDescription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		description string
		summary     RenderedContent
		want        string
	}{
		{
			name:        "the summary names the id",
			description: "Review with " + mentionRaw(testMentionID),
			summary: RenderedContent{
				Raw:  "Review with " + mentionRaw(testMentionID),
				HTML: paragraph("Review with " + mentionSpan(testMentionID, testMentionName)),
			},
			want: "Review with @" + testMentionName,
		},
		{
			// A pull request built client-side carries the description without the summary.
			name:        "a zero-value summary",
			description: "Review with " + mentionRaw(testMentionID),
			summary:     RenderedContent{},
			want:        "Review with " + mentionRaw(testMentionID),
		},
		{
			name:        "no description",
			description: "",
			summary: RenderedContent{
				Raw:  "",
				HTML: paragraph(mentionSpan(testMentionID, testMentionName)),
			},
			want: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			pullRequest := PullRequest{Description: test.description, Summary: test.summary}

			testkit.AssertEqual(t, "ReadableDescription", pullRequest.ReadableDescription(), test.want)
		})
	}
}

func TestResolveMentionsFunc(t *testing.T) {
	t.Parallel()

	wrap := func(mention string) string { return "<" + mention + ">" }
	punctuated := testMentionSecondName + "*"
	raw := "Hi " + mentionRaw(testMentionID) + " and " + mentionRaw(testMentionGoneID) + ", " + mentionRaw(testMentionUUID)
	rendered := paragraph("Hi " + mentionSpan(testMentionID, testMentionName) + " and " +
		mentionRaw(testMentionGoneID) + ", " + mentionSpan(testMentionUUID, punctuated))

	t.Run("write wraps every mention it resolves and sees the escaped name", func(t *testing.T) {
		t.Parallel()
		want := "Hi <@" + testMentionName + "> and " + mentionRaw(testMentionGoneID) + ", <@" + testMentionSecondName + "&#42;>"
		testkit.AssertEqual(t, "resolved", ResolveMentionsFunc(raw, rendered, wrap), want)
	})

	t.Run("a nil write is ResolveMentions", func(t *testing.T) {
		t.Parallel()
		testkit.AssertEqual(t, "resolved", ResolveMentionsFunc(raw, rendered, nil), ResolveMentions(raw, rendered))
	})

	t.Run("the ordered walk hands write only the occurrences Bitbucket rendered", func(t *testing.T) {
		t.Parallel()
		twice := mentionRaw(testMentionID) + " and `" + mentionRaw(testMentionID) + "`"
		once := paragraph(mentionSpan(testMentionID, testMentionName) + " and <code>" + mentionRaw(testMentionID) + "</code>")
		testkit.AssertEqual(t, "resolved", ResolveMentionsFunc(twice, once, wrap),
			"<@"+testMentionName+"> and `"+mentionRaw(testMentionID)+"`")
	})

	t.Run("raw without a mention never reaches write", func(t *testing.T) {
		t.Parallel()
		calls := 0
		counting := func(mention string) string {
			calls++
			return mention
		}
		testkit.AssertEqual(t, "resolved", ResolveMentionsFunc("nobody here", rendered, counting), "nobody here")
		testkit.AssertEqual(t, "calls", calls, 0)
	})
}

func TestMentionSpans_GuardAgainstMalformedRenderings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		raw      string
		rendered string
		want     string
	}{
		{
			name: "a span that lost its closing tag names nobody",
			raw:  mentionRaw(testMentionID) + " and " + mentionRaw(testMentionUUID),
			rendered: paragraph(`<span data-atlassian-id="` + testMentionID + `">@` + testMentionName + " and " +
				mentionSpan(testMentionUUID, testMentionSecondName)),
			want: mentionRaw(testMentionID) + " and " + mentionRaw(testMentionUUID),
		},
		{
			name: "a span nested inside the name leaves the id alone rather than cutting the name short",
			raw:  mentionRaw(testMentionID),
			rendered: paragraph(`<span data-atlassian-id="` + testMentionID +
				`">@<span class="x">Margaret</span> Hamilton</span>`),
			want: mentionRaw(testMentionID),
		},
		{
			name:     "a name longer than any display name is not a name",
			raw:      mentionRaw(testMentionID),
			rendered: paragraph(mentionSpan(testMentionID, strings.Repeat("a", maxMentionNameRunes+1))),
			want:     mentionRaw(testMentionID),
		},
		{
			name:     "whitespace inside a name collapses, so a name cannot end the block it sits in",
			raw:      "# T " + mentionRaw(testMentionID) + " end",
			rendered: paragraph(mentionSpan(testMentionID, "Margaret\n\t Hamilton")),
			want:     "# T @" + testMentionName + " end",
		},
		{
			name: "a stray NUL in the rendering cannot forge a sentinel",
			raw:  "Ping " + mentionRaw(testMentionID) + " and `" + mentionRaw(testMentionID) + "` in code.",
			rendered: "<p>Ping\x00 " + mentionSpan(testMentionID, testMentionName) + " and <code>" +
				mentionRaw(testMentionID) + "</code> in code.</p>",
			want: "Ping @" + testMentionName + " and `" + mentionRaw(testMentionID) + "` in code.",
		},
		{
			name: "a literal the rendering hid in an href still counts, so the link keeps its id",
			raw: "See " + mentionRaw(testMentionID) + " at [profile](https://bitbucket.org/" +
				mentionRaw(testMentionID) + "/)",
			rendered: paragraph("See " + mentionSpan(testMentionID, testMentionName) +
				` at <a href="https://bitbucket.org/` + mentionRaw(testMentionID) + `/">profile</a>`),
			want: "See @" + testMentionName + " at [profile](https://bitbucket.org/" + mentionRaw(testMentionID) + "/)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			testkit.AssertEqual(t, "resolved", ResolveMentions(tc.raw, tc.rendered), tc.want)
		})
	}
}
