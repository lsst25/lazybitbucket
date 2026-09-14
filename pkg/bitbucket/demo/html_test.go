package demo

import (
	"strings"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// These tests cover the half of the dataset the mention resolver reads: the
// rendering served next to every raw body, and the invariant that the two
// agree — every mention of somebody in the workspace resolves to their display
// name, and the one mention of the colleague who left does not.

func TestRenderHTML(t *testing.T) {
	t.Parallel()

	accounts := []bitbucket.Account{demoUser(), ken()}

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "an empty body renders to nothing",
			raw:  "",
			want: "",
		},
		{
			name: "a body with no mention is a paragraph",
			raw:  "Looks good to me.",
			want: "<p>Looks good to me.</p>",
		},
		{
			name: "a known id becomes a span",
			raw:  "ping @{" + accountIDKen + "}",
			want: `<p>ping <span class="ap-mention" data-atlassian-id="` + accountIDKen +
				`">@Ken Thompson</span></p>`,
		},
		{
			name: "an id nobody has stays literal",
			raw:  "@{" + AccountIDGone + "} wrote it",
			want: "<p>@{" + AccountIDGone + "} wrote it</p>",
		},
		{
			name: "the same id twice becomes two spans",
			raw:  "@{" + accountIDDemo + "} and @{" + accountIDDemo + "}",
			want: `<p><span class="ap-mention" data-atlassian-id="` + accountIDDemo +
				`">@Demo User</span> and <span class="ap-mention" data-atlassian-id="` +
				accountIDDemo + `">@Demo User</span></p>`,
		},
		{
			name: "markup in the body is escaped",
			raw:  "Tom & Jerry <b>bold</b> \"quoted\"",
			want: "<p>Tom &amp; Jerry &lt;b&gt;bold&lt;/b&gt; &#34;quoted&#34;</p>",
		},
		{
			name: "escaping runs before the spans are spliced in",
			raw:  "a & b @{" + accountIDKen + "}",
			want: `<p>a &amp; b <span class="ap-mention" data-atlassian-id="` + accountIDKen +
				`">@Ken Thompson</span></p>`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			testkit.AssertEqual(t, "renderHTML", renderHTML(test.raw, accounts), test.want)
		})
	}
}

// TestRenderHTML_RoundTripsThroughResolveMentions is the contract the fixtures
// live by: what renderHTML writes is what bitbucket.ResolveMentions reads.
func TestRenderHTML_RoundTripsThroughResolveMentions(t *testing.T) {
	t.Parallel()

	raw := "@{" + accountIDDemo + "} and @{" + AccountIDGone + "} reviewed it"
	resolved := bitbucket.ResolveMentions(raw, renderHTML(raw, workspaceMembers()))

	testkit.AssertEqual(t, "resolved",
		resolved, "@Demo User and @{"+AccountIDGone+"} reviewed it")
}

// TestDataset_EveryBodyCarriesItsRendering keeps raw and rendered together.
// Bitbucket never serves one without the other, and a fixture that did would
// let a resolver bug through unnoticed.
func TestDataset_EveryBodyCarriesItsRendering(t *testing.T) {
	t.Parallel()

	data := NewDataset()

	for _, pullRequests := range data.PullRequests {
		for _, pullRequest := range pullRequests {
			if pullRequest.Description == "" {
				testkit.AssertEqual(t, "no description means no rendering on "+pullRequest.Key(),
					pullRequest.Summary.HTML, "")
				continue
			}
			if pullRequest.Summary.HTML == "" {
				t.Errorf("%s has a description but no summary.html", pullRequest.Key())
			}
		}
	}

	for key, comments := range data.Comments {
		for _, comment := range comments {
			if comment.Content.Raw == "" {
				testkit.AssertEqual(t, "the deleted comment has no rendering either",
					comment.Content.HTML, "")
				continue
			}
			if comment.Content.HTML == "" {
				t.Errorf("comment %d on %s has a body but no content.html", comment.ID, key)
			}
		}
	}
}

// TestDataset_MentionsResolveToDisplayNames walks every fixture body: no
// mention of anyone in the workspace may survive as an id, and the one mention
// of the departed colleague must survive as one.
func TestDataset_MentionsResolveToDisplayNames(t *testing.T) {
	t.Parallel()

	data := NewDataset()

	for _, pullRequests := range data.PullRequests {
		for index := range pullRequests {
			pullRequest := &pullRequests[index]
			assertNoKnownMention(t, data.Members, pullRequest.Key()+" description",
				pullRequest.ReadableDescription())
		}
	}
	for key, comments := range data.Comments {
		for _, comment := range comments {
			assertNoKnownMention(t, data.Members, key+" comment "+comment.Links.Self.Href,
				comment.Content.Readable())
		}
	}
}

// assertNoKnownMention fails if resolved still writes any workspace member as
// an "@{account_id}" — the form the reader is meant never to see.
func assertNoKnownMention(t *testing.T, members []bitbucket.Account, label, resolved string) {
	t.Helper()

	for _, member := range members {
		if strings.Contains(resolved, "@{"+member.AccountID+"}") {
			t.Errorf("%s still mentions %s by account id:\n%s", label, member.Nickname, resolved)
		}
	}
}

// TestDataset_MentionFixtures pins the bodies the review surface is shown on:
// #103's description and the two threads that carry a mention, including the
// departed colleague whose id stays literal because Bitbucket cannot resolve it.
func TestDataset_MentionFixtures(t *testing.T) {
	t.Parallel()

	data := NewDataset()

	description := findPullRequest(t, data, repoAPIGateway, 103).ReadableDescription()
	if !strings.Contains(description, "@Demo User the budget maths") {
		t.Errorf("#103 description does not name the reviewer:\n%s", description)
	}

	byID := map[int64]bitbucket.Comment{}
	for _, comment := range data.Comments[reviewKey(repoAPIGateway, 103)] {
		byID[comment.ID] = comment
	}

	reply := byID[commentIDBase+2].Content.Readable()
	if !strings.HasSuffix(reply, "streaming case, @Demo User.\n") {
		t.Errorf("the inline reply does not end in a resolved mention:\n%s", reply)
	}

	root := byID[commentIDBase+4].Content.Readable()
	if !strings.Contains(root, "@{"+AccountIDGone+"} wrote the 2.3 budget code") {
		t.Errorf("the departed colleague's mention was resolved away:\n%s", root)
	}

	repeated := byID[commentIDBase+5].Content.Readable()
	if !strings.Contains(repeated, "@Demo User @Ken Thompson - @Ken Thompson owns") {
		t.Errorf("the repeated mention did not resolve twice:\n%s", repeated)
	}

	// The names come out of the rendering, not out of a lookup table: the
	// rendering alone knows both of them.
	names := bitbucket.MentionNames(byID[commentIDBase+5].Content.HTML)
	testkit.AssertEqual(t, "len(names)", len(names), 2)
	testkit.AssertEqual(t, "demo user", names[accountIDDemo], "Demo User")
	testkit.AssertEqual(t, "ken", names[accountIDKen], "Ken Thompson")
}
