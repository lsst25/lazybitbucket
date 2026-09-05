package theme

import "github.com/charmbracelet/lipgloss"

// Kanagawa is the palette of rebelot/kanagawa.nvim (MIT), a theme inspired by
// the colours of Katsushika Hokusai's "The Great Wave off Kanagawa". It comes
// in three variants — Wave (the default dark), Dragon (a darker, desaturated
// dark) and Lotus (light) — modelled here as three presets. The hex values are
// copied from the plugin's lua/kanagawa/colors.lua; the comments name the
// palette entry each value comes from so a future sync is a diff, not a hunt.
//
// Role mapping follows the plugin's own semantics where it has them: Accent is
// the blue the theme is known by — the function colour, and the active border
// and session badge of a Kanagawa tmux — Green the "ok" diagnostic colour, Red
// the vcs "removed" colour, Gray the comment colour, White the default
// foreground and Highlight the popup-menu selection background. Yellow deliberately takes the palette's
// yellow rather than the "warning" diagnostic, which is an orange so close to
// Orange that an in-progress build and an author name would look alike.

// kanagawaWave is the default dark variant: fujiWhite text on sumiInk3.
func kanagawaWave() *Theme {
	return buildTheme(
		ColorPalette{
			Accent:    lipgloss.Color("#7E9CD8"), // crystalBlue — functions, terminal blue
			Green:     lipgloss.Color("#98BB6C"), // springGreen — diag.ok, bright green
			Blue:      lipgloss.Color("#7E9CD8"), // crystalBlue — functions, terminal blue
			Red:       lipgloss.Color("#C34043"), // autumnRed — vcs.removed, terminal red
			Yellow:    lipgloss.Color("#E6C384"), // carpYellow — identifiers, bright yellow
			Cyan:      lipgloss.Color("#7AA89F"), // waveAqua2 — types, bright cyan
			Magenta:   lipgloss.Color("#957FB8"), // oniViolet — keywords, terminal magenta
			White:     lipgloss.Color("#DCD7BA"), // fujiWhite — ui.fg
			Gray:      lipgloss.Color("#727169"), // fujiGray — comments
			Orange:    lipgloss.Color("#FFA066"), // surimiOrange — constants
			None:      lipgloss.Color("-1"),
			Highlight: lipgloss.Color("#2D4F67"), // waveBlue2 — pmenu.bg_sel, bg_search
		},
		[]lipgloss.Color{
			lipgloss.Color("#FFA066"), // surimiOrange
			lipgloss.Color("#D27E99"), // sakuraPink
			lipgloss.Color("#98BB6C"), // springGreen
			lipgloss.Color("#E46876"), // waveRed
			lipgloss.Color("#7FB4CA"), // springBlue
			lipgloss.Color("#E6C384"), // carpYellow
			lipgloss.Color("#957FB8"), // oniViolet
			lipgloss.Color("#7AA89F"), // waveAqua2
			lipgloss.Color("#FF5D62"), // peachRed
			lipgloss.Color("#A3D4D5"), // lightBlue
			lipgloss.Color("#C0A36E"), // boatYellow2
			lipgloss.Color("#9CABCA"), // springViolet2
		},
	)
}

// kanagawaDragon is the darker, desaturated variant: dragonWhite text on
// dragonBlack3. The selection background is shared with Wave, as in the
// plugin.
func kanagawaDragon() *Theme {
	return buildTheme(
		ColorPalette{
			Accent:    lipgloss.Color("#8ba4b0"), // dragonBlue2 — functions, terminal blue
			Green:     lipgloss.Color("#87a987"), // dragonGreen — bright green
			Blue:      lipgloss.Color("#8ba4b0"), // dragonBlue2 — functions, terminal blue
			Red:       lipgloss.Color("#c4746e"), // dragonRed — operators, terminal red
			Yellow:    lipgloss.Color("#c4b28a"), // dragonYellow — identifiers, terminal yellow
			Cyan:      lipgloss.Color("#8ea4a2"), // dragonAqua — types, terminal cyan
			Magenta:   lipgloss.Color("#a292a3"), // dragonPink — numbers, terminal magenta
			White:     lipgloss.Color("#c5c9c5"), // dragonWhite — ui.fg
			Gray:      lipgloss.Color("#737c73"), // dragonAsh — comments
			Orange:    lipgloss.Color("#b6927b"), // dragonOrange — constants
			None:      lipgloss.Color("-1"),
			Highlight: lipgloss.Color("#2D4F67"), // waveBlue2 — pmenu.bg_sel, bg_search
		},
		[]lipgloss.Color{
			lipgloss.Color("#b6927b"), // dragonOrange
			lipgloss.Color("#a292a3"), // dragonPink
			lipgloss.Color("#87a987"), // dragonGreen
			lipgloss.Color("#c4746e"), // dragonRed
			lipgloss.Color("#8ba4b0"), // dragonBlue2
			lipgloss.Color("#c4b28a"), // dragonYellow
			lipgloss.Color("#8992a7"), // dragonViolet
			lipgloss.Color("#8ea4a2"), // dragonAqua
			lipgloss.Color("#b98d7b"), // dragonOrange2
			lipgloss.Color("#949fb5"), // dragonTeal
			lipgloss.Color("#8a9a7b"), // dragonGreen2
			lipgloss.Color("#a6a69c"), // dragonGray
		},
	)
}

// kanagawaLotus is the light variant: lotusInk1 text on lotusWhite3. Its
// Yellow is the vcs "changed" amber — the palette's terminal yellow is an
// olive that would be mistaken for Green on a light background.
func kanagawaLotus() *Theme {
	return buildTheme(
		ColorPalette{
			Accent:    lipgloss.Color("#4d699b"), // lotusBlue4 — functions, terminal blue
			Green:     lipgloss.Color("#6f894e"), // lotusGreen — diag.ok, terminal green
			Blue:      lipgloss.Color("#4d699b"), // lotusBlue4 — functions, terminal blue
			Red:       lipgloss.Color("#c84053"), // lotusRed — terminal red
			Yellow:    lipgloss.Color("#de9800"), // lotusYellow3 — vcs.changed
			Cyan:      lipgloss.Color("#597b75"), // lotusAqua — types, terminal cyan
			Magenta:   lipgloss.Color("#624c83"), // lotusViolet4 — keywords, bright magenta
			White:     lipgloss.Color("#545464"), // lotusInk1 — ui.fg
			Gray:      lipgloss.Color("#8a8980"), // lotusGray3 — comments
			Orange:    lipgloss.Color("#cc6d00"), // lotusOrange — constants
			None:      lipgloss.Color("-1"),
			Highlight: lipgloss.Color("#9fb5c9"), // lotusBlue3 — pmenu.bg_sel
		},
		[]lipgloss.Color{
			lipgloss.Color("#cc6d00"), // lotusOrange
			lipgloss.Color("#b35b79"), // lotusPink
			lipgloss.Color("#6f894e"), // lotusGreen
			lipgloss.Color("#c84053"), // lotusRed
			lipgloss.Color("#4d699b"), // lotusBlue4
			lipgloss.Color("#836f4a"), // lotusYellow2
			lipgloss.Color("#624c83"), // lotusViolet4
			lipgloss.Color("#597b75"), // lotusAqua
			lipgloss.Color("#6693bf"), // lotusTeal2
			lipgloss.Color("#5d57a3"), // lotusBlue5
			lipgloss.Color("#6e915f"), // lotusGreen2
			lipgloss.Color("#4e8ca2"), // lotusTeal1
		},
	)
}
