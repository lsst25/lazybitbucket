package views

import (
	"strings"
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

func TestDetailView_Viewed_TicksTheFileAndCountsItInTheLabel(t *testing.T) {
	t.Parallel()
	view := filesDetail(t)

	view.SetViewedFiles(testPRKey, map[string]bool{testDiffPath: true, "b.go": true})

	out := stripANSI(view.View())
	if !strings.Contains(out, "✔ M "+testDiffPath) {
		t.Errorf("the viewed file has no tick:\n%s", out)
	}
	if strings.Contains(out, "✔ A pkg/new/thing.go") {
		t.Errorf("a file not viewed has a tick:\n%s", out)
	}
	testkit.AssertEqual(t, "label", tabLabelFor(t, view, TabFiles), "Files (5 · 2 viewed)")
	testkit.AssertEqual(t, "count", view.ViewedFileCount(), 2)
}

func TestDetailView_Viewed_AMarkOutsideTheListIsNotCounted(t *testing.T) {
	t.Parallel()
	view := filesDetail(t)

	view.SetViewedFiles(testPRKey, map[string]bool{"removed/by/a/push.go": true})

	testkit.AssertEqual(t, "count", view.ViewedFileCount(), 0)
	testkit.AssertEqual(t, "label", tabLabelFor(t, view, TabFiles), "Files (5)")
}

func TestDetailView_Viewed_ASetForAnotherPullRequestIsDropped(t *testing.T) {
	t.Parallel()
	view := filesDetail(t)
	view.SetViewedFiles(testPRKey, map[string]bool{testDiffPath: true})

	view.SetViewedFiles(testPRKey2, map[string]bool{"b.go": true})

	testkit.AssertEqual(t, "own mark kept", view.FileViewed(testDiffPath), true)
	testkit.AssertEqual(t, "foreign mark ignored", view.FileViewed("b.go"), false)
}

func TestDetailView_NextUnviewedFile(t *testing.T) {
	t.Parallel()
	view := filesDetail(t)
	view.SetViewedFiles(testPRKey, map[string]bool{"pkg/new/thing.go": true, "pkg/old/gone.go": true})

	cases := []struct {
		name, from, want string
		ok               bool
	}{
		{name: "skips viewed files", from: testDiffPath, want: "b.go", ok: true},
		{name: "wraps round to the top", from: "docs/logo.png", want: testDiffPath, ok: true},
		{name: "an unlisted path starts at the top", from: "nowhere.go", want: testDiffPath, ok: true},
	}
	for _, c := range cases {
		got, ok := view.NextUnviewedFile(c.from)
		testkit.AssertEqual(t, c.name+": ok", ok, c.ok)
		testkit.AssertEqual(t, c.name+": path", got, c.want)
	}

	view.SetViewedFiles(testPRKey, map[string]bool{
		"pkg/new/thing.go": true, "pkg/old/gone.go": true, "b.go": true, "docs/logo.png": true,
	})
	if got, ok := view.NextUnviewedFile(testDiffPath); ok {
		t.Errorf("NextUnviewedFile() = %q with every other file viewed, want false", got)
	}
}

func TestDetailView_SelectFile(t *testing.T) {
	t.Parallel()
	view := filesDetail(t)

	testkit.AssertEqual(t, "listed path", view.SelectFile("b.go"), true)
	testkit.AssertEqual(t, "cursor", view.SelectedFile().Path(), "b.go")
	testkit.AssertEqual(t, "unlisted path", view.SelectFile("nowhere.go"), false)
	testkit.AssertEqual(t, "cursor unchanged", view.SelectedFile().Path(), "b.go")

	view.SetActiveTab(TabCommits)
	testkit.AssertEqual(t, "off the Files tab", view.SelectFile(testDiffPath), false)
}
