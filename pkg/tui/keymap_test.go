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

func TestDefaultKeymap_NoKeyBoundTwice(t *testing.T) {
	t.Parallel()

	owner := make(map[string]Action)
	for action, keys := range DefaultKeymap() {
		for _, key := range keys {
			if first, clash := owner[key]; clash {
				t.Errorf("key %q is bound to both %q and %q", key, first, action)
				continue
			}
			owner[key] = action
		}
	}
}

func TestDefaultKeymap_EveryActionHasAKey(t *testing.T) {
	t.Parallel()

	keymap := DefaultKeymap()
	for _, action := range []Action{
		ActDetailOverview, ActDetailFiles, ActDetailCommits, ActDetailComments,
		ActNextFile, ActPrevFile, ActOpenEditor,
	} {
		if keymap.Keys(action) == "" {
			t.Errorf("%q has no default key, so its help row would be blank", action)
		}
	}
}

func TestKeymapFromConfig_RemapsTheDetailKeys(t *testing.T) {
	t.Parallel()

	var keybindingConfig config.KeybindingConfig
	// Keys nothing else claims, so Match cannot answer with another action.
	keybindingConfig.Detail.OverviewTab = "I"
	keybindingConfig.Detail.FilesTab = "D"
	keybindingConfig.Detail.CommitsTab = "M"
	keybindingConfig.Detail.CommentsTab = "T"
	keybindingConfig.Detail.NextFile = "N"
	keybindingConfig.Detail.PrevFile = "P"
	keybindingConfig.Detail.OpenEditor = "E"

	keymap := KeymapFromConfig(keybindingConfig)

	for key, want := range map[string]Action{
		"I": ActDetailOverview, "D": ActDetailFiles, "M": ActDetailCommits,
		"T": ActDetailComments, "N": ActNextFile, "P": ActPrevFile, "E": ActOpenEditor,
	} {
		testkit.AssertEqual(t, "remapped "+key, keymap.Match(key), want)
	}
}
