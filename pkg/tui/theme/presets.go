package theme

import "strings"

// Preset is a named palette bundled with the binary. Light and dark variants
// of the same theme are modelled as separate Presets so users see and pick
// each one explicitly.
//
// Build is a factory rather than a value so flavors that pull from external
// packages (e.g. Catppuccin) are constructed on demand instead of at init.
type Preset struct {
	Name        string
	Description string
	IsLight     bool
	Build       func() *Theme
}

// DefaultDarkPresetName / DefaultLightPresetName back the "auto" preset
// selector. The choice between them is made by looking at the terminal
// background via HasDarkBackground.
const (
	DefaultDarkPresetName  = "catppuccin-mocha"
	DefaultLightPresetName = "catppuccin-latte"
)

// presetList is the ordered registry of bundled presets. Adding a new theme
// is a single append here — no switch statements to update elsewhere.
var presetList = []Preset{
	{
		Name:        "default",
		Description: "ANSI 16 palette — matches your terminal colors",
		Build:       defaultTheme,
	},
	{
		Name:        "catppuccin-latte",
		Description: "Catppuccin Latte (light pastel)",
		IsLight:     true,
		Build:       catppuccinLatte,
	},
	{
		Name:        "catppuccin-frappe",
		Description: "Catppuccin Frappé (dark pastel, warm)",
		Build:       catppuccinFrappe,
	},
	{
		Name:        "catppuccin-macchiato",
		Description: "Catppuccin Macchiato (dark pastel, medium)",
		Build:       catppuccinMacchiato,
	},
	{
		Name:        "catppuccin-mocha",
		Description: "Catppuccin Mocha (dark pastel, deepest)",
		Build:       catppuccinMocha,
	},
	{
		Name:        "kanagawa-wave",
		Description: "Kanagawa Wave (dark, the default variant)",
		Build:       kanagawaWave,
	},
	{
		Name:        "kanagawa-dragon",
		Description: "Kanagawa Dragon (dark, desaturated)",
		Build:       kanagawaDragon,
	},
	{
		Name:        "kanagawa-lotus",
		Description: "Kanagawa Lotus (light)",
		IsLight:     true,
		Build:       kanagawaLotus,
	},
}

// Family pairs the dark and the light preset of one theme. Selecting a family
// by name picks whichever variant matches the terminal background, the way a
// terminal that follows the system appearance switches its own colours. "auto"
// is the family of the bundled defaults.
type Family struct {
	Name  string
	Dark  string
	Light string
}

// familyList is the registry of theme families, in display order.
var familyList = []Family{
	{Name: "auto", Dark: DefaultDarkPresetName, Light: DefaultLightPresetName},
	{Name: "catppuccin", Dark: "catppuccin-mocha", Light: "catppuccin-latte"},
	{Name: "kanagawa", Dark: "kanagawa-wave", Light: "kanagawa-lotus"},
}

// Presets returns a copy of the bundled preset list in display order.
// Callers are free to mutate the returned slice.
func Presets() []Preset {
	out := make([]Preset, len(presetList))
	copy(out, presetList)
	return out
}

// FindPreset looks up a preset by case-insensitive name. Returns nil if no
// preset matches.
func FindPreset(name string) *Preset {
	for i := range presetList {
		if strings.EqualFold(presetList[i].Name, name) {
			return &presetList[i]
		}
	}
	return nil
}

// Families returns a copy of the theme families in display order.
func Families() []Family {
	out := make([]Family, len(familyList))
	copy(out, familyList)
	return out
}

// FindFamily looks up a family by case-insensitive name. ok is false when no
// family matches.
func FindFamily(name string) (family Family, ok bool) {
	for _, f := range familyList {
		if strings.EqualFold(f.Name, name) {
			return f, true
		}
	}
	return Family{}, false
}
