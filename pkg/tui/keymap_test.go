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
		ActMerge, ActSync, ActEdit, ActReviewers, ActToggleDraft, ActApprove,
		ActRerunPipeline, ActStopPipeline,
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
	// D, M and E are the write keys now and P and X the pipeline ones, so the
	// detail keys move off all five.
	keybindingConfig.Detail.OverviewTab = "I"
	keybindingConfig.Detail.FilesTab = "F"
	keybindingConfig.Detail.CommitsTab = "W"
	keybindingConfig.Detail.CommentsTab = "T"
	keybindingConfig.Detail.NextFile = "N"
	keybindingConfig.Detail.PrevFile = "B"
	keybindingConfig.Detail.OpenEditor = "C"

	keymap := KeymapFromConfig(keybindingConfig)

	for key, want := range map[string]Action{
		"I": ActDetailOverview, "F": ActDetailFiles, "W": ActDetailCommits,
		"T": ActDetailComments, "N": ActNextFile, "B": ActPrevFile, "C": ActOpenEditor,
	} {
		testkit.AssertEqual(t, "remapped "+key, keymap.Match(key), want)
	}
}

func TestKeymapFromConfig_RemapsTheWriteKeys(t *testing.T) {
	t.Parallel()

	var keybindingConfig config.KeybindingConfig
	keybindingConfig.PullRequests.Merge = "F"
	keybindingConfig.PullRequests.Sync = "W"
	keybindingConfig.PullRequests.Edit = "Y"
	keybindingConfig.PullRequests.Reviewers = "I"
	keybindingConfig.PullRequests.ToggleDraft = "N"
	keybindingConfig.PullRequests.Approve = "Z"

	keymap := KeymapFromConfig(keybindingConfig)

	for key, want := range map[string]Action{
		"F": ActMerge, "W": ActSync, "Y": ActEdit, "I": ActReviewers, "N": ActToggleDraft,
		"Z": ActApprove,
	} {
		testkit.AssertEqual(t, "remapped "+key, keymap.Match(key), want)
	}
	// The defaults they replaced are free again.
	for _, key := range []string{"M", "S", "E", "V", "D", "A"} {
		if action := keymap.Match(key); action == ActMerge || action == ActSync ||
			action == ActEdit || action == ActReviewers || action == ActToggleDraft ||
			action == ActApprove {
			t.Errorf("key %q still resolves to %q after the remap", key, action)
		}
	}
}

func TestKeymapFromConfig_RemapsThePipelineKeys(t *testing.T) {
	t.Parallel()

	km := DefaultKeymap()
	testkit.AssertEqual(t, "default rerun", km.Match("P"), ActRerunPipeline)
	testkit.AssertEqual(t, "default stop", km.Match("X"), ActStopPipeline)

	var keybindingConfig config.KeybindingConfig
	keybindingConfig.Builds.Rerun = "B"
	keybindingConfig.Builds.Stop = "C"

	km = KeymapFromConfig(keybindingConfig)

	testkit.AssertEqual(t, "remapped rerun", km.Match("B"), ActRerunPipeline)
	testkit.AssertEqual(t, "remapped stop", km.Match("C"), ActStopPipeline)
	// The defaults they replaced are free again, and nothing else claimed them.
	testkit.AssertEqual(t, "old rerun key released", km.Match("P"), Action(""))
	testkit.AssertEqual(t, "old stop key released", km.Match("X"), Action(""))
}

func TestKeymap_ScreenModeKeys(t *testing.T) {
	t.Parallel()
	km := DefaultKeymap()
	testkit.AssertEqual(t, "default next", km.Match("+"), ActNextScreenMode)
	testkit.AssertEqual(t, "default prev", km.Match("_"), ActPrevScreenMode)

	var kcfg config.KeybindingConfig
	kcfg.Universal.NextScreenMode = "="
	kcfg.Universal.PrevScreenMode = "-"
	km = KeymapFromConfig(kcfg)
	testkit.AssertEqual(t, "remapped next", km.Match("="), ActNextScreenMode)
	testkit.AssertEqual(t, "remapped prev", km.Match("-"), ActPrevScreenMode)
	testkit.AssertEqual(t, "old key released", km.Match("+"), Action(""))
}
