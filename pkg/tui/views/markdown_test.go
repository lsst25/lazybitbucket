package views

import (
	"strings"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

const testMarkdown = "# Heading\n\nSome prose.\n\n- first\n- second\n\n```go\nfmt.Println(\"hi\")\n```\n"

// Both renderers satisfy the interface the detail view is constructed with.
var (
	_ MarkdownRenderer = GlamourRenderer{}
	_ MarkdownRenderer = PlainRenderer{}
)

func TestGlamourRenderer_RendersHeadingListAndCodeFence(t *testing.T) {
	t.Parallel()
	lines := GlamourRenderer{Style: glamourStyleNoTTY}.Render(testMarkdown, 60)
	if len(lines) == 0 {
		t.Fatal("Render() returned no lines")
	}

	out := stripANSI(strings.Join(lines, "\n"))
	for _, want := range []string{"Heading", "Some prose.", "first", "second", "fmt.Println"} {
		if !strings.Contains(out, want) {
			t.Errorf("Render() = %q, want it to contain %q", out, want)
		}
	}
	if strings.Contains(out, "```") {
		t.Errorf("Render() = %q, want the code fence markers consumed", out)
	}
}

func TestGlamourRenderer_TrimsTrailingBlankLines(t *testing.T) {
	t.Parallel()
	lines := GlamourRenderer{Style: glamourStyleNoTTY}.Render("just one line\n\n\n", 40)
	if len(lines) == 0 {
		t.Fatal("Render() returned no lines")
	}
	last := stripANSI(lines[len(lines)-1])
	if strings.TrimSpace(last) == "" {
		t.Errorf("last line = %q, want trailing blanks trimmed", last)
	}
}

func TestGlamourRenderer_EmptyInputRendersNothing(t *testing.T) {
	t.Parallel()
	if lines := (GlamourRenderer{Style: glamourStyleNoTTY}).Render("", 40); lines != nil {
		t.Errorf("Render(\"\") = %v, want nil", lines)
	}
}

func TestGlamourRenderer_NonPositiveWidthStillRenders(t *testing.T) {
	t.Parallel()
	lines := GlamourRenderer{Style: glamourStyleNoTTY}.Render("hello", 0)
	if len(lines) == 0 {
		t.Fatal("Render() with width 0 returned no lines")
	}
	if !strings.Contains(stripANSI(strings.Join(lines, "\n")), "hello") {
		t.Error("Render() with width 0 dropped the content")
	}
}

func TestGlamourRenderer_UnknownStyleReportsInline(t *testing.T) {
	t.Parallel()
	lines := GlamourRenderer{Style: "no-such-style"}.Render("hello", 40)
	if len(lines) == 0 {
		t.Fatal("Render() with a bad style returned no lines")
	}
	joined := stripANSI(strings.Join(lines, "\n"))
	if !strings.Contains(joined, "hello") && !strings.Contains(joined, "glamour:") {
		t.Errorf("Render() = %q, want either the content or an inline error", joined)
	}
}

func TestPlainRenderer_WrapsAtTheGivenWidth(t *testing.T) {
	t.Parallel()
	lines := PlainRenderer{}.Render(strings.Repeat("word ", 30), 20)
	if len(lines) < 2 {
		t.Fatalf("len(lines) = %d, want the text wrapped", len(lines))
	}
	for _, line := range lines {
		if len([]rune(line)) > 20 {
			t.Errorf("line %q is %d columns wide, want at most 20", line, len([]rune(line)))
		}
	}
}

func TestPlainRenderer_KeepsMarkupAndDropsEmptyInput(t *testing.T) {
	t.Parallel()
	lines := PlainRenderer{}.Render("## Heading", 40)
	testkit.AssertEqual(t, "len(lines)", len(lines), 1)
	testkit.AssertEqual(t, "line", lines[0], "## Heading")

	if got := (PlainRenderer{}).Render("", 40); got != nil {
		t.Errorf("Render(\"\") = %v, want nil", got)
	}
}
