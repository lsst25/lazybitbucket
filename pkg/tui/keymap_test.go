package tui

import (
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/config"
	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
)

func TestKeymapFromConfig_OverridesAndMatches(t *testing.T) {
	t.Parallel()

	var keybindingConfig config.KeybindingConfig
	keybindingConfig.Universal.Quit = "Q"
	keybindingConfig.Navigation.Down = "n"

	keymap := KeymapFromConfig(keybindingConfig)

	testkit.AssertSliceEqual(t, "quit binding overridden", keymap[ActQuit], []string{"Q"})
	testkit.AssertEqual(t, "Match resolves override", keymap.Match("Q"), ActQuit)
	testkit.AssertEqual(t, "MatchNav resolves override", keymap.MatchNav("n"), components.NavDown)
}

func TestKeymapFromConfig_EmptyKeepsDefaults(t *testing.T) {
	t.Parallel()

	defaults := DefaultKeymap()
	keymap := KeymapFromConfig(config.KeybindingConfig{})

	testkit.AssertSliceEqual(t, "quit default preserved", keymap[ActQuit], defaults[ActQuit])
}

func TestKeymap_MatchUnknownReturnsEmpty(t *testing.T) {
	t.Parallel()

	keymap := DefaultKeymap()

	testkit.AssertEqual(t, "unknown key", keymap.Match("this-key-is-unbound"), Action(""))
	testkit.AssertEqual(t, "unknown nav key", keymap.MatchNav("this-key-is-unbound"), components.NavNone)
}
