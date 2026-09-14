package theme

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestSetThemeDefault(t *testing.T) {
	if err := SetTheme("default"); err != nil {
		t.Fatalf("SetTheme(default): %v", err)
	}
	if ColorGreen != lipgloss.Color("2") {
		t.Errorf("ColorGreen = %q, want %q", ColorGreen, "2")
	}
	if ColorBlue != lipgloss.Color("4") {
		t.Errorf("ColorBlue = %q, want %q", ColorBlue, "4")
	}
}

func TestSetThemeEmpty(t *testing.T) {
	// Empty preset must resolve to the legacy ANSI 16 default so that
	// upgrading users who never set gui.theme see no visual change.
	if err := SetTheme(""); err != nil {
		t.Fatalf("SetTheme(''): %v", err)
	}
	if ColorGreen != lipgloss.Color("2") {
		t.Errorf("ColorGreen = %q, want ANSI 2", ColorGreen)
	}
	if ColorBlue != lipgloss.Color("4") {
		t.Errorf("ColorBlue = %q, want ANSI 4", ColorBlue)
	}
	_ = SetTheme("default")
}

func TestSetThemeAuto(t *testing.T) {
	// "auto" picks a Catppuccin default based on terminal background. We
	// can't predict which lands, but it must populate the palette and the
	// chosen preset must be one of the registered defaults.
	if err := SetTheme("auto"); err != nil {
		t.Fatalf("SetTheme(auto): %v", err)
	}
	if Default.Colors.Green == "" {
		t.Error("auto-detected palette has empty Green")
	}
	got := string(Default.Colors.Green)
	if got != "#a6e3a1" && got != "#40a02b" {
		t.Errorf("auto Green = %q, want mocha or latte default", got)
	}
	_ = SetTheme("default")
}

func TestSetThemeCatppuccinMocha(t *testing.T) {
	if err := SetTheme("catppuccin-mocha"); err != nil {
		t.Fatalf("SetTheme(catppuccin-mocha): %v", err)
	}
	if ColorGreen != lipgloss.Color("#a6e3a1") {
		t.Errorf("ColorGreen = %q, want %q", ColorGreen, "#a6e3a1")
	}
	if ColorBlue != lipgloss.Color("#89b4fa") {
		t.Errorf("ColorBlue = %q, want %q", ColorBlue, "#89b4fa")
	}
	if Default.Colors.Red != lipgloss.Color("#f38ba8") {
		t.Errorf("Default.Colors.Red = %q, want %q", Default.Colors.Red, "#f38ba8")
	}

	_ = SetTheme("default")
}

func TestSetThemeAllFlavors(t *testing.T) {
	flavors := []string{
		"catppuccin-latte",
		"catppuccin-frappe",
		"catppuccin-macchiato",
		"catppuccin-mocha",
		"kanagawa-wave",
		"kanagawa-dragon",
		"kanagawa-lotus",
	}
	for _, name := range flavors {
		t.Run(name, func(t *testing.T) {
			if err := SetTheme(name); err != nil {
				t.Fatalf("SetTheme(%s): %v", name, err)
			}
			if Default.Colors.Green == "" {
				t.Error("Colors.Green is empty")
			}
			if len(Default.AuthorPalette) != 12 {
				t.Errorf("AuthorPalette has %d entries, want 12", len(Default.AuthorPalette))
			}
		})
	}
	_ = SetTheme("default")
}

func TestSetThemeKanagawaWave(t *testing.T) {
	if err := SetTheme("kanagawa-wave"); err != nil {
		t.Fatalf("SetTheme(kanagawa-wave): %v", err)
	}
	if ColorGreen != lipgloss.Color("#98BB6C") {
		t.Errorf("ColorGreen = %q, want springGreen #98BB6C", ColorGreen)
	}
	if ColorBlue != lipgloss.Color("#7E9CD8") {
		t.Errorf("ColorBlue = %q, want crystalBlue #7E9CD8", ColorBlue)
	}
	if ColorHighlight != lipgloss.Color("#2D4F67") {
		t.Errorf("ColorHighlight = %q, want waveBlue2 #2D4F67", ColorHighlight)
	}
	if ColorAccent != lipgloss.Color("#7E9CD8") {
		t.Errorf("ColorAccent = %q, want crystalBlue #7E9CD8", ColorAccent)
	}
	if Default.Title.GetForeground() != ColorAccent || Default.SuccessText.GetForeground() != ColorGreen {
		t.Error("Title must paint with the accent and SuccessText with green")
	}
	_ = SetTheme("default")
}

func TestAccentIsGreenOutsideKanagawa(t *testing.T) {
	// The accent role was split out of Green for Kanagawa; every other preset
	// keeps the two equal so the look users have does not change.
	for _, p := range Presets() {
		built := p.Build()
		isKanagawa := strings.HasPrefix(p.Name, "kanagawa-")
		if !isKanagawa && built.Colors.Accent != built.Colors.Green {
			t.Errorf("%s: Accent %q != Green %q", p.Name, built.Colors.Accent, built.Colors.Green)
		}
		if isKanagawa && built.Colors.Accent == built.Colors.Green {
			t.Errorf("%s: Accent should be the theme's blue, got its green %q", p.Name, built.Colors.Accent)
		}
	}
}

func TestInitOverridesAccent(t *testing.T) {
	err := Init(Options{Preset: "kanagawa-wave", Colors: map[string]string{"accent": "#98BB6C"}})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if ColorAccent != lipgloss.Color("#98BB6C") {
		t.Errorf("accent override not applied: %q", ColorAccent)
	}
	if ColorBlue != lipgloss.Color("#7E9CD8") {
		t.Errorf("overriding accent must leave Blue alone: %q", ColorBlue)
	}
	_ = SetTheme("default")
}

func TestSetThemeFamilyPicksAVariantOfThatFamily(t *testing.T) {
	// A family name resolves to its dark or its light preset depending on
	// the terminal background, which a test cannot predict — but the result
	// must be one of the two, and never a preset of another family.
	for _, family := range Families() {
		t.Run(family.Name, func(t *testing.T) {
			if err := SetTheme(family.Name); err != nil {
				t.Fatalf("SetTheme(%s): %v", family.Name, err)
			}
			dark := FindPreset(family.Dark).Build().Colors
			light := FindPreset(family.Light).Build().Colors
			if Default.Colors != dark && Default.Colors != light {
				t.Errorf("%s resolved to a palette that is neither %s nor %s: %+v",
					family.Name, family.Dark, family.Light, Default.Colors)
			}
		})
	}
	_ = SetTheme("default")
}

func TestSetThemeFamilyIsCaseInsensitive(t *testing.T) {
	if err := SetTheme(" Kanagawa "); err != nil {
		t.Fatalf("SetTheme(' Kanagawa '): %v", err)
	}
	wave := FindPreset("kanagawa-wave").Build().Colors
	lotus := FindPreset("kanagawa-lotus").Build().Colors
	if Default.Colors != wave && Default.Colors != lotus {
		t.Errorf("' Kanagawa ' did not resolve to a Kanagawa palette: %+v", Default.Colors)
	}
	_ = SetTheme("default")
}

func TestFindFamily(t *testing.T) {
	family, ok := FindFamily("KANAGAWA")
	if !ok {
		t.Fatal("FindFamily should be case-insensitive")
	}
	if family.Dark != "kanagawa-wave" || family.Light != "kanagawa-lotus" {
		t.Errorf("kanagawa family = %+v, want wave/lotus", family)
	}
	for _, f := range Families() {
		if FindPreset(f.Dark) == nil || FindPreset(f.Light) == nil {
			t.Errorf("family %s names an unregistered preset: dark %q, light %q", f.Name, f.Dark, f.Light)
		}
		if FindPreset(f.Dark).IsLight || !FindPreset(f.Light).IsLight {
			t.Errorf("family %s: dark/light variants have the wrong IsLight flag", f.Name)
		}
	}
	if _, ok := FindFamily("kanagawa-wave"); ok {
		t.Error("a preset name must not be found as a family")
	}
	if _, ok := FindFamily("nord"); ok {
		t.Error("an unknown name must not be found as a family")
	}
}

func TestSetThemeUnknown(t *testing.T) {
	err := SetTheme("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown theme")
	}
}

func TestSetThemeSyncsColors(t *testing.T) {
	_ = SetTheme("catppuccin-mocha")
	if ColorGreen != Default.Colors.Green {
		t.Errorf("ColorGreen not synced: %q != %q", ColorGreen, Default.Colors.Green)
	}
	if ColorBlue != Default.Colors.Blue {
		t.Errorf("ColorBlue not synced: %q != %q", ColorBlue, Default.Colors.Blue)
	}
	if ColorOrange != Default.Colors.Orange {
		t.Errorf("ColorOrange not synced: %q != %q", ColorOrange, Default.Colors.Orange)
	}
	if ColorMagenta != Default.Colors.Magenta {
		t.Errorf("ColorMagenta not synced: %q != %q", ColorMagenta, Default.Colors.Magenta)
	}
	_ = SetTheme("default")
}

func TestSetThemeSyncsAuthorPalette(t *testing.T) {
	_ = SetTheme("default")
	defaultFirst := authorPalette[0]

	_ = SetTheme("catppuccin-mocha")
	if authorPalette[0] == defaultFirst {
		t.Error("authorPalette did not switch when theme changed")
	}
	if len(authorPalette) != len(Default.AuthorPalette) {
		t.Errorf("authorPalette length %d != Default.AuthorPalette length %d",
			len(authorPalette), len(Default.AuthorPalette))
	}
	_ = SetTheme("default")
}

func TestSetThemeResetsAuthorCache(t *testing.T) {
	_ = SetTheme("default")
	_ = AuthorStyle("Alice")
	if len(authorCache) == 0 {
		t.Fatal("author cache should have an entry")
	}

	_ = SetTheme("catppuccin-mocha")
	if len(authorCache) != 0 {
		t.Error("author cache should be empty after theme switch")
	}
	_ = SetTheme("default")
}

func TestInitAppliesSharedOverrides(t *testing.T) {
	err := Init(Options{
		Preset: "default",
		Colors: map[string]string{
			"green":     "#abcdef",
			"highlight": "#123456",
			"bogus":     "ignored",
			"red":       "", // empty value must be skipped
		},
	})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if Default.Colors.Green != lipgloss.Color("#abcdef") {
		t.Errorf("Green = %q, want #abcdef", Default.Colors.Green)
	}
	if ColorGreen != lipgloss.Color("#abcdef") {
		t.Errorf("ColorGreen not synced: %q", ColorGreen)
	}
	if Default.Colors.Highlight != lipgloss.Color("#123456") {
		t.Errorf("Highlight = %q, want #123456", Default.Colors.Highlight)
	}
	// Empty value must leave Red at the preset's value.
	if Default.Colors.Red != lipgloss.Color("1") {
		t.Errorf("Red = %q, want preset default 1", Default.Colors.Red)
	}
	_ = SetTheme("default")
}

func TestInitAppliesDarkOverridesOnlyOnDarkPreset(t *testing.T) {
	// Mocha is a dark preset → ColorsDark applies, ColorsLight is ignored.
	err := Init(Options{
		Preset:      "catppuccin-mocha",
		ColorsDark:  map[string]string{"green": "#111111"},
		ColorsLight: map[string]string{"green": "#999999"},
	})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if Default.Colors.Green != lipgloss.Color("#111111") {
		t.Errorf("dark override not applied: Green = %q", Default.Colors.Green)
	}

	// Latte is a light preset → ColorsLight applies instead.
	err = Init(Options{
		Preset:      "catppuccin-latte",
		ColorsDark:  map[string]string{"green": "#111111"},
		ColorsLight: map[string]string{"green": "#999999"},
	})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if Default.Colors.Green != lipgloss.Color("#999999") {
		t.Errorf("light override not applied: Green = %q", Default.Colors.Green)
	}
	_ = SetTheme("default")
}

func TestFindPresetCaseInsensitive(t *testing.T) {
	if FindPreset("Catppuccin-Mocha") == nil {
		t.Error("FindPreset should be case-insensitive")
	}
	if FindPreset("does-not-exist") != nil {
		t.Error("FindPreset should return nil for unknown names")
	}
}

func TestPresetsListed(t *testing.T) {
	got := Presets()
	if len(got) < 5 {
		t.Errorf("expected at least 5 presets, got %d", len(got))
	}
}

func TestInitFallsBackOnInvalidColorValues(t *testing.T) {
	err := Init(Options{
		Preset: "default",
		Colors: map[string]string{
			"green": "not-a-color",
			"blue":  "#zzzzzz",
			"red":   "totally bogus value",
		},
	})
	if err != nil {
		t.Fatalf("Init must accept malformed color values: %v", err)
	}

	// Each invalid value falls back to lipgloss.Color("-1"), the terminal
	// default sentinel used elsewhere in the package (theme.go:39, 82).
	fallback := lipgloss.Color("-1")
	if Default.Colors.Green != fallback {
		t.Errorf("Green = %q, want %q (terminal default)", Default.Colors.Green, fallback)
	}
	if Default.Colors.Blue != fallback {
		t.Errorf("Blue = %q, want %q (terminal default)", Default.Colors.Blue, fallback)
	}
	if Default.Colors.Red != fallback {
		t.Errorf("Red = %q, want %q (terminal default)", Default.Colors.Red, fallback)
	}

	// Package-level color vars must be synced to the fallback too.
	if ColorGreen != fallback {
		t.Errorf("ColorGreen not synced: %q, want %q", ColorGreen, fallback)
	}
	if ColorBlue != fallback {
		t.Errorf("ColorBlue not synced: %q, want %q", ColorBlue, fallback)
	}
	if ColorRed != fallback {
		t.Errorf("ColorRed not synced: %q, want %q", ColorRed, fallback)
	}

	// Other keys must keep their preset defaults.
	if Default.Colors.Yellow != lipgloss.Color("3") {
		t.Errorf("Yellow = %q, want preset default 3", Default.Colors.Yellow)
	}

	// Rendering with the fallback color does not panic.
	_ = Default.Title.Render("smoke test")
	_ = SetTheme("default")
}

func TestValidColor(t *testing.T) {
	cases := []struct {
		val  string
		want bool
	}{
		// Valid hex (case-insensitive).
		{"#abc", true},
		{"#abcdef", true},
		{"#ABCDEF", true},
		{"#abcdef12", true},
		// Valid ANSI decimal and terminal-default sentinel.
		{"-1", true},
		{"0", true},
		{"15", true},
		{"208", true},
		{"255", true},
		// Invalid.
		{"", false},
		{"#", false},
		{"#xyz", false},
		{"#abcd", false},
		{"#abcde", false},
		{"#zzzzzz", false},
		{"-2", false},
		{"256", false},
		{"not-a-color", false},
		{"#abcdef ", false}, // trailing space — we do not trim
	}
	for _, c := range cases {
		t.Run(c.val, func(t *testing.T) {
			if got := ValidColor(c.val); got != c.want {
				t.Errorf("ValidColor(%q) = %v, want %v", c.val, got, c.want)
			}
		})
	}
}

func TestPresetsReturnsCopy(t *testing.T) {
	got := Presets()
	if len(got) == 0 {
		t.Fatal("Presets returned empty slice")
	}
	original := got[0].Name
	got[0].Name = "mutated"
	again := Presets()
	if again[0].Name != original {
		t.Errorf("Presets backing slice was mutated: got %q, want %q", again[0].Name, original)
	}
}

func TestBuildTheme_MentionUsesTheBlueRole(t *testing.T) {
	built := buildTheme(defaultPalette(), defaultAuthorPalette())
	if got := built.Mention.GetForeground(); got != built.Colors.Blue {
		t.Errorf("Mention foreground = %v, want the palette's blue %v", got, built.Colors.Blue)
	}
}
