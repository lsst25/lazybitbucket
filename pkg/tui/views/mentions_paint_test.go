package views

import (
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// Painting fixtures: the tags the painter is asked to wrap a mention in, and
// the sequences a colourful Markdown renderer leaves around its text.
const (
	paintOpen  = "<m>"
	paintClose = "</m>"
	sgrGray    = "\x1b[38;5;252m"
	sgrBold    = "\x1b[1m"
)

// marked is one resolved mention as readableMarkdown hands it to the renderer.
func marked(name string) string { return mentionOpen + "@" + name + mentionClose }

// painted is that mention as paintMarked leaves it.
func painted(name string) string { return paintOpen + "@" + name + paintClose }

func TestPaintMarked(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		lines []string
		want  []string
	}{
		{
			name:  "a marked mention is painted and the markers vanish",
			lines: []string{"  Ping " + marked(testAuthor) + " and"},
			want:  []string{"  Ping " + painted(testAuthor) + " and"},
		},
		{
			name:  "the renderer's state is restored after the mention",
			lines: []string{sgrGray + "Ping " + marked(testAuthor) + " and" + sgrReset},
			want:  []string{sgrGray + "Ping " + painted(testAuthor) + sgrGray + " and" + sgrReset},
		},
		{
			name:  "a reset forgets what the renderer set before it",
			lines: []string{sgrBold + "x" + sgrReset + sgrGray + marked(testAuthor)},
			want:  []string{sgrBold + "x" + sgrReset + sgrGray + painted(testAuthor) + sgrGray},
		},
		{
			name:  "a sequence inside the mention is followed by the colour again",
			lines: []string{mentionOpen + "@Tom &" + sgrReset + sgrGray + " Jerry" + mentionClose + "!"},
			want: []string{
				paintOpen + "@Tom &" + sgrReset + paintOpen + sgrGray + paintOpen + " Jerry" + paintClose + sgrGray + "!",
			},
		},
		{
			name: "a mention wrapped across lines is painted on both",
			lines: []string{
				"  " + mentionOpen + "@Grace",
				"  " + sgrGray + "Hopper" + mentionClose + " and" + sgrReset,
			},
			want: []string{
				"  " + paintOpen + "@Grace",
				paintOpen + "  " + sgrGray + paintOpen + "Hopper" + paintClose + sgrGray + " and" + sgrReset,
			},
		},
		{
			name:  "an escape that is not SGR is text",
			lines: []string{"\x1b[2J" + marked(testAuthor)},
			want:  []string{"\x1b[2J" + painted(testAuthor)},
		},
		{
			name:  "a line without markers is copied",
			lines: []string{sgrGray + "plain" + sgrReset},
			want:  []string{sgrGray + "plain" + sgrReset},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := paintMarked(tc.lines, paintOpen, paintClose)
			testkit.AssertEqual(t, "lines", strings.Join(got, "\n"), strings.Join(tc.want, "\n"))
		})
	}
}

func TestPaintMentions(t *testing.T) {
	t.Parallel()

	t.Run("a style with nothing to write only removes the markers", func(t *testing.T) {
		t.Parallel()
		got := paintMentions([]string{"Ping " + marked(testAuthor) + " now"}, lipgloss.NewStyle())
		testkit.AssertEqual(t, "line", got[0], "Ping @"+testAuthor+" now")
	})

	t.Run("a coloured style paints the mention with its own sequences", func(t *testing.T) {
		t.Parallel()
		renderer := lipgloss.NewRenderer(io.Discard)
		renderer.SetColorProfile(termenv.TrueColor)
		style := renderer.NewStyle().Foreground(lipgloss.Color("#7E9CD8"))

		got := paintMentions([]string{"Ping " + marked(testAuthor)}, style)
		testkit.AssertEqual(t, "line", got[0], "Ping "+style.Render("@"+testAuthor))
	})

	t.Run("lines without a marker come back as they are", func(t *testing.T) {
		t.Parallel()
		lines := []string{"plain", "text"}
		got := paintMentions(lines, lipgloss.NewStyle())
		testkit.AssertEqual(t, "same slice", &got[0] == &lines[0], true)
	})
}

func TestRenderBody_GlamourNeverLeaksAMarker(t *testing.T) {
	t.Parallel()

	view := NewDetailView(GlamourRenderer{Style: "dark"}, "")
	raw := "Ping " + mentionRaw(testMentionID) + " now, in a **bold " + mentionRaw(testMentionID) + "** too."
	rendered := mentionHTML("Ping " + mentionSpan(testMentionID, testAuthor) +
		" now, in a <strong>bold " + mentionSpan(testMentionID, testAuthor) + "</strong> too.")

	// 14 columns splits "Grace Hopper" across two lines; 80 keeps it whole.
	for _, width := range []int{14, 80} {
		joined := strings.Join(view.renderBody(raw, rendered, width), "\n")
		if strings.Contains(joined, mentionOpen) || strings.Contains(joined, mentionClose) {
			t.Errorf("width %d leaks a marker:\n%q", width, joined)
		}
		plain := stripANSI(joined)
		for _, want := range []string{"@Grace", "Hopper"} {
			if !strings.Contains(plain, want) {
				t.Errorf("width %d lost %q:\n%s", width, want, plain)
			}
		}
	}
}
