package views

import (
	"strconv"
	"strings"
)

// sgrReset closes a background the diff pane opened to pad a cell out.
const sgrReset = "\x1b[0m"

// csiPrefix opens every sequence this file cares about.
const csiPrefix = "\x1b["

// The SGR parameters that set a background: the eight ANSI colours, the eight
// bright ones, and the extended form whose arguments follow it.
const (
	sgrBackgroundFirst  = 40
	sgrBackgroundLast   = 47
	sgrBrightBackground = 100
	sgrBrightBgLast     = 107
	sgrExtendedBg       = 48
	sgrExtendedFg       = 38
)

// leadingSGR is the SGR sequence a line opens with — ESC '[' parameters 'm' at
// offset zero — and whether it has one. Only the opening sequence is looked at:
// it is the one that decides what a line's padding would be drawn under.
func leadingSGR(line string) (string, bool) {
	if !strings.HasPrefix(line, csiPrefix) {
		return "", false
	}
	end := strings.IndexByte(line, 'm')
	if end < 0 {
		return "", false
	}
	params := line[len(csiPrefix):end]
	if strings.Trim(params, "0123456789;") != "" {
		return "", false
	}
	return line[:end+1], true
}

// sgrSetsBackground reports whether an SGR sequence sets a background colour.
// The parameters have to be walked rather than searched, because an extended
// foreground carries arguments of its own — the 41 of "38;5;41" is a shade of
// green, not a red background.
func sgrSetsBackground(seq string) bool {
	params := strings.Split(strings.TrimSuffix(strings.TrimPrefix(seq, csiPrefix), "m"), ";")
	for i := 0; i < len(params); {
		code, err := strconv.Atoi(params[i])
		if err != nil {
			// An empty parameter is a zero, which is the reset.
			i++
			continue
		}
		switch {
		case code == sgrExtendedBg:
			return true
		case code == sgrExtendedFg:
			i += 1 + extendedColorArgs(params[i+1:])
		case code >= sgrBackgroundFirst && code <= sgrBackgroundLast,
			code >= sgrBrightBackground && code <= sgrBrightBgLast:
			return true
		default:
			i++
		}
	}
	return false
}

// extendedColorArgs is how many parameters an extended colour introducer eats
// after itself: "5;n" is two and "2;r;g;b" is four. Anything else is treated as
// eating none, so a malformed sequence cannot skip past the end.
func extendedColorArgs(rest []string) int {
	if len(rest) == 0 {
		return 0
	}
	switch rest[0] {
	case "5": // 5;n — the 256-colour palette.
		return min(2, len(rest))
	case "2": // 2;r;g;b — a direct colour.
		return min(4, len(rest))
	default:
		return 0
	}
}
