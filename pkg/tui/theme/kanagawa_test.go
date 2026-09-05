package theme

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// kanagawaPresetNames lists the three Kanagawa presets by their variant.
var kanagawaPresetNames = map[string]bool{ // name → IsLight
	"kanagawa-wave":   false,
	"kanagawa-dragon": false,
	"kanagawa-lotus":  true,
}

func TestKanagawaPresets_AreRegisteredWithTheirVariant(t *testing.T) {
	t.Parallel()
	for name, light := range kanagawaPresetNames {
		p := FindPreset(name)
		if p == nil {
			t.Fatalf("FindPreset(%q) = nil", name)
		}
		if p.IsLight != light {
			t.Errorf("%s: IsLight = %v, want %v", name, p.IsLight, light)
		}
	}
}

func TestKanagawaPresets_PalettesAreCompleteHexColours(t *testing.T) {
	t.Parallel()
	for name := range kanagawaPresetNames {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			built := FindPreset(name).Build()
			roles := map[string]lipgloss.Color{
				"green": built.Colors.Green, "blue": built.Colors.Blue, "red": built.Colors.Red,
				"yellow": built.Colors.Yellow, "cyan": built.Colors.Cyan, "magenta": built.Colors.Magenta,
				"white": built.Colors.White, "gray": built.Colors.Gray, "orange": built.Colors.Orange,
				"highlight": built.Colors.Highlight,
			}
			for role, c := range roles {
				if len(c) != 7 || c[0] != '#' || !ValidColor(string(c)) {
					t.Errorf("%s: %s = %q, want a #rrggbb colour", name, role, c)
				}
			}
			if built.Colors.None != lipgloss.Color("-1") {
				t.Errorf("%s: None = %q, want the terminal default", name, built.Colors.None)
			}
			if len(built.AuthorPalette) != 12 {
				t.Fatalf("%s: %d author colours, want 12", name, len(built.AuthorPalette))
			}
			seen := map[lipgloss.Color]bool{}
			for _, c := range built.AuthorPalette {
				if !ValidColor(string(c)) {
					t.Errorf("%s: author colour %q is not a colour", name, c)
				}
				if seen[c] {
					t.Errorf("%s: author colour %q repeats", name, c)
				}
				seen[c] = true
			}
		})
	}
}

func TestKanagawaPresets_YellowAndOrangeDiffer(t *testing.T) {
	t.Parallel()
	// An in-progress build (Yellow) and an author name (Orange) must not
	// share a colour; the plugin's "warning" orange would have made them.
	for name := range kanagawaPresetNames {
		built := FindPreset(name).Build()
		if built.Colors.Yellow == built.Colors.Orange {
			t.Errorf("%s: Yellow and Orange are both %q", name, built.Colors.Yellow)
		}
	}
}

func TestKanagawaWave_MatchesThePluginPalette(t *testing.T) {
	t.Parallel()
	built := FindPreset("kanagawa-wave").Build()
	want := map[string][2]lipgloss.Color{
		"Green (springGreen)":     {built.Colors.Green, "#98BB6C"},
		"Blue (crystalBlue)":      {built.Colors.Blue, "#7E9CD8"},
		"White (fujiWhite)":       {built.Colors.White, "#DCD7BA"},
		"Highlight (waveBlue2)":   {built.Colors.Highlight, "#2D4F67"},
		"Gray (fujiGray)":         {built.Colors.Gray, "#727169"},
		"Orange (surimiOrange)":   {built.Colors.Orange, "#FFA066"},
		"Magenta (oniViolet)":     {built.Colors.Magenta, "#957FB8"},
		"Lotus White (lotusInk1)": {FindPreset("kanagawa-lotus").Build().Colors.White, "#545464"},
	}
	for label, pair := range want {
		if pair[0] != pair[1] {
			t.Errorf("%s = %q, want %q", label, pair[0], pair[1])
		}
	}
}
