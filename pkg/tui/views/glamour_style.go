package views

import (
	"github.com/lsst25/lazybitbucket/pkg/config"
	"github.com/lsst25/lazybitbucket/pkg/tui/theme"
)

const (
	glamourStyleDark  = "dark"
	glamourStyleLight = "light"
	glamourStyleNoTTY = "notty"
)

// ResolveGlamourStyle maps the configured rendererStyle to a glamour style
// name. "auto" and an unknown value follow the terminal background, detected
// the same way the theme presets are (see theme.HasDarkBackground).
func ResolveGlamourStyle(style string) string {
	return resolveGlamourStyle(style, theme.HasDarkBackground)
}

func resolveGlamourStyle(style string, hasDarkBackground func() bool) string {
	switch style {
	case config.RendererStyleDark:
		return glamourStyleDark
	case config.RendererStyleLight:
		return glamourStyleLight
	case config.RendererStyleNoTTY:
		return glamourStyleNoTTY
	}
	if hasDarkBackground() {
		return glamourStyleDark
	}
	return glamourStyleLight
}
