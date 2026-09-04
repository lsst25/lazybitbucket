package views

import (
	"strings"
	"testing"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

func TestWrapText_WrapsOnWordBoundaries(t *testing.T) {
	t.Parallel()
	lines := wrapText("the quick brown fox jumps over the lazy dog", 12)
	if len(lines) < 4 {
		t.Fatalf("len(lines) = %d, want the sentence wrapped", len(lines))
	}
	for _, line := range lines {
		if len([]rune(line)) > 12 {
			t.Errorf("line %q is %d columns wide, want at most 12", line, len([]rune(line)))
		}
		if strings.HasPrefix(line, " ") {
			t.Errorf("line %q keeps a leading space", line)
		}
	}
}

func TestWrapText_KeepsParagraphsAndHandlesWideRunes(t *testing.T) {
	t.Parallel()
	lines := wrapText("first\nsecond", 40)
	testkit.AssertSliceEqual(t, "paragraphs", lines, []string{"first", "second"})

	wide := wrapText(strings.Repeat("世", 20), 10)
	for _, line := range wide {
		if len([]rune(line)) > 5 {
			t.Errorf("line %q holds %d wide runes, want at most 5 in 10 columns", line, len([]rune(line)))
		}
	}
}

func TestWrapText_NonPositiveWidthFallsBackTo80(t *testing.T) {
	t.Parallel()
	lines := wrapText(strings.Repeat("x", 200), 0)
	testkit.AssertEqual(t, "first line width", len([]rune(lines[0])), 80)
}

func TestHardWrapLine_SplitsUnbrokenText(t *testing.T) {
	t.Parallel()
	chunks := hardWrapLine(strings.Repeat("x", 25), 10)
	testkit.AssertSliceEqual(t, "chunks", chunks,
		[]string{"xxxxxxxxxx", "xxxxxxxxxx", "xxxxx"})

	testkit.AssertSliceEqual(t, "short line", hardWrapLine("short", 10), []string{"short"})
	testkit.AssertEqual(t, "non-positive width", len(hardWrapLine(strings.Repeat("y", 90), 0)), 2)
}

func TestPadRight_UsesDisplayWidth(t *testing.T) {
	t.Parallel()
	testkit.AssertEqual(t, "padded", padRight("ab", 5), "ab   ")
	testkit.AssertEqual(t, "already wide enough", padRight("abcdef", 3), "abcdef")
	testkit.AssertEqual(t, "wide runes", len([]rune(padRight("世界", 6))), 4)
}

func TestTimeAgo(t *testing.T) {
	t.Parallel()
	now := time.Now()
	cases := []struct {
		name string
		in   time.Time
		want string
	}{
		{"zero", time.Time{}, ""},
		{"minutes", now.Add(-5 * time.Minute), "5m"},
		{"hours", now.Add(-3 * time.Hour), "3h"},
		{"days", now.Add(-48 * time.Hour), "2d"},
		{"months", now.Add(-70 * 24 * time.Hour), "2mo"},
		// A timestamp ahead of the clock (skew, or the shifted demo dataset)
		// reads as "just now", never as a negative age.
		{"future", now.Add(30 * time.Minute), "0m"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			testkit.AssertEqual(t, "timeAgo", timeAgo(tc.in), tc.want)
		})
	}
}

func TestFindURLs_TrimsSurroundingPunctuation(t *testing.T) {
	t.Parallel()
	got := findURLs("see (https://example.com/a), and http://b.test/x. plus ftp://nope")
	testkit.AssertSliceEqual(t, "urls", got,
		[]string{"https://example.com/a", "http://b.test/x"})
}

func TestColorURLs_LeavesTheTextIntact(t *testing.T) {
	t.Parallel()
	cases := []string{
		"go to https://example.com now",
		"two https://a.test and http://b.test links",
		"trailing https://example.com/x",
		"no links at all",
	}
	for _, line := range cases {
		t.Run(line, func(t *testing.T) {
			t.Parallel()
			testkit.AssertEqual(t, "plain text unchanged", stripANSI(colorURLs(line)), line)
		})
	}
}

func TestColorURLsWrapped_KeepsEveryLine(t *testing.T) {
	t.Parallel()
	lines := []string{"see https://example.com/very/long", "path/continues here", "plain"}
	styled := colorURLsWrapped(lines)

	testkit.AssertEqual(t, "line count", len(styled), 3)
	for i, line := range lines {
		testkit.AssertEqual(t, "plain text unchanged", stripANSI(styled[i]), line)
	}
}

func TestLineEndsInURL(t *testing.T) {
	t.Parallel()
	testkit.AssertEqual(t, "ends in a URL", lineEndsInURL("see https://example.com/x"), true)
	testkit.AssertEqual(t, "URL followed by text", lineEndsInURL("https://a.test more"), false)
	testkit.AssertEqual(t, "no URL", lineEndsInURL("plain text"), false)
}

func TestExtractURLs_GroupsBySection(t *testing.T) {
	t.Parallel()
	pr := newPR(101, testRepo, testPRTitle)
	pr.Description = "docs at https://docs.example.com/auth and https://docs.example.com/auth again"
	pr.Links.HTML.Href = "https://bitbucket.org/acme/api-gateway/pull-requests/101"

	groups := ExtractURLs(&pr, testStatuses(), nil, nil)
	if len(groups) != 3 {
		t.Fatalf("len(groups) = %d, want 3", len(groups))
	}

	testkit.AssertEqual(t, "first section", groups[0].Section, "Pull request")
	testkit.AssertSliceEqual(t, "pull request URLs", groups[0].URLs,
		[]string{"https://bitbucket.org/acme/api-gateway/pull-requests/101"})

	testkit.AssertEqual(t, "second section", groups[1].Section, "Description")
	testkit.AssertSliceEqual(t, "description URLs", groups[1].URLs,
		[]string{"https://docs.example.com/auth"})

	testkit.AssertEqual(t, "third section", groups[2].Section, "Builds")
	testkit.AssertSliceEqual(t, "build URLs", groups[2].URLs,
		[]string{"https://bitbucket.org/acme/api-gateway/pipelines/results/42"})
}

func TestExtractURLs_SkipsEmptySectionsAndNilPullRequest(t *testing.T) {
	t.Parallel()
	if got := ExtractURLs(nil, nil, nil, nil); got != nil {
		t.Errorf("ExtractURLs(nil, …) = %v, want nil", got)
	}

	pr := newPR(101, testRepo, testPRTitle)
	groups := ExtractURLs(&pr, []bitbucket.CommitStatus{{Key: "no-url"}}, nil, nil)
	if len(groups) != 1 {
		t.Fatalf("len(groups) = %d, want only the fallback pull request URL", len(groups))
	}
	testkit.AssertEqual(t, "section", groups[0].Section, "Pull request")
	if !strings.Contains(groups[0].URLs[0], "/pull-requests/101") {
		t.Errorf("URL = %q, want the constructed browser URL", groups[0].URLs[0])
	}
}
