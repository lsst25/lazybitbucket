package views

import (
	"html"
	"strings"
	"testing"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
)

// mentionRaw is a mention as Bitbucket stores it in the Markdown.
func mentionRaw(id string) string { return "@{" + id + "}" }

// mentionSpan is a mention as Bitbucket renders it: the account id in
// data-atlassian-id and the display name as the span's text, html-escaped the
// way the wire carries it.
func mentionSpan(id, name string) string {
	return `<span class="ap-mention" data-atlassian-id="` + id + `">@` + html.EscapeString(name) + `</span>`
}

// mentionHTML wraps rendered spans and text in the one paragraph Bitbucket
// serves next to the raw Markdown.
func mentionHTML(inner string) string { return "<p>" + inner + "</p>" }

// mentionBody is the body every mention fixture uses, short enough to survive
// the detail panel's wrapping on one line whether it reads as an id or a name.
func mentionBody() string { return "Ping " + mentionRaw(testMentionID) + " on this." }

func TestDetailView_Comments_MentionReadsAsTheRenderedName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		rendered string
		want     string
		notWant  string
	}{
		{
			name:     "the rendering names the id",
			rendered: mentionHTML("Ping " + mentionSpan(testMentionID, testReviewer) + " on this."),
			want:     "@" + testReviewer,
			notWant:  mentionRaw(testMentionID),
		},
		{
			name:     "no rendering keeps the stored id",
			rendered: "",
			want:     mentionRaw(testMentionID),
			notWant:  "@" + testReviewer,
		},
		{
			name:     "a rendering that names somebody else keeps the stored id",
			rendered: mentionHTML("cc " + mentionSpan(testMentionID2, testAuthor)),
			want:     mentionRaw(testMentionID),
			notWant:  "@" + testAuthor,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := threadComment(testGeneralRootID, testReviewer, mentionBody(), time.Hour)
			root.Content.HTML = tc.rendered

			view := makeFocusedDetail()
			view.SetPullRequest(detailPR())
			view.SetThreads(testPRKey, bitbucket.BuildThreads([]bitbucket.Comment{root}))
			view.SetActiveTab(TabComments)
			view.SetSize(80, 60)

			out := stripANSI(view.View())
			if !strings.Contains(out, tc.want) {
				t.Errorf("comment does not contain %q:\n%s", tc.want, out)
			}
			if strings.Contains(out, tc.notWant) {
				t.Errorf("comment should not contain %q:\n%s", tc.notWant, out)
			}
		})
	}
}

func TestDetailView_Overview_MentionReadsAsTheRenderedName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		rendered string
		want     string
		notWant  string
	}{
		{
			name:     "summary.html names the id",
			rendered: mentionHTML("Ping " + mentionSpan(testMentionID, testReviewer) + " on this."),
			want:     "@" + testReviewer,
			notWant:  mentionRaw(testMentionID),
		},
		{
			name:     "a zero-value summary keeps the stored id",
			rendered: "",
			want:     mentionRaw(testMentionID),
			notWant:  "@" + testReviewer,
		},
		{
			name:     "a summary that names somebody else keeps the stored id",
			rendered: mentionHTML("cc " + mentionSpan(testMentionID2, testAuthor)),
			want:     mentionRaw(testMentionID),
			notWant:  "@" + testAuthor,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pr := detailPR()
			pr.Description = mentionBody()
			pr.Summary = bitbucket.RenderedContent{Raw: pr.Description, Markup: "markdown", HTML: tc.rendered}

			view := makeFocusedDetail()
			view.SetPullRequest(pr)
			view.SetActiveTab(TabOverview)
			view.SetSize(80, 40)

			out := stripANSI(view.View())
			if !strings.Contains(out, tc.want) {
				t.Errorf("description does not contain %q:\n%s", tc.want, out)
			}
			if strings.Contains(out, tc.notWant) {
				t.Errorf("description should not contain %q:\n%s", tc.notWant, out)
			}
		})
	}
}

// TestDetailView_Mentions_GlamourRendersPunctuatedNamesLiterally is the reason
// a resolved name is entity-escaped rather than left as it came: production
// renders through glamour, which reads "_", "*", "<" and ":" in a display name
// as markup. It runs on GlamourRenderer, not on the PlainRenderer the other
// view tests inject, because the escaping is only undone by a Markdown pass.
func TestDetailView_Mentions_GlamourRendersPunctuatedNamesLiterally(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		who  string
		raw  string
		want string
	}{
		{"a leading underscore is not emphasis", "_Doe_", mentionBody(), "@_Doe_"},
		{"asterisks are not emphasis", "J*Doe*", mentionBody(), "@J*Doe*"},
		{"angle brackets are not html", "Ann <Ops>", mentionBody(), "@Ann <Ops>"},
		{"an ampersand survives the entity round trip", "Tom & Jerry", mentionBody(), "@Tom & Jerry"},
		{"a colon pair is not an emoji", "Ann :tada: Lee", mentionBody(), "@Ann :tada: Lee"},
		{"a dotted name after cc is not an address", "Grace.Hopper", "cc" + mentionRaw(testMentionID), "cc@Grace.Hopper"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pr := newPR(101, testRepo, testPRTitle)
			pr.Description = tc.raw
			pr.Summary = bitbucket.RenderedContent{
				Raw:    tc.raw,
				Markup: "markdown",
				HTML:   mentionHTML(mentionSpan(testMentionID, tc.who)),
			}

			// The dark style is the one that tells: under it a bare "J*Doe*"
			// comes out as an italic "JDoe" and "_Doe_" as an italic "Doe",
			// whereas notty prints the asterisks and underscores either way.
			view := NewDetailView(GlamourRenderer{Style: "dark"}, "")
			view.SetSize(80, 24)
			view.SetPullRequest(&pr)

			out := stripANSI(strings.Join(view.renderDescription(76), "\n"))
			if !strings.Contains(out, tc.want) {
				t.Errorf("glamour does not render %q literally:\n%s", tc.want, out)
			}
			for _, leak := range []string{"&#", `\`, "mailto:", "🎉"} {
				if strings.Contains(out, leak) {
					t.Errorf("rendered description leaks %q:\n%s", leak, out)
				}
			}
		})
	}
}
