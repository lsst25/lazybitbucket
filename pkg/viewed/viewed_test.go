package viewed

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

const (
	prKey    = "acme/api#7"
	headOld  = "aaaaaaaaaaaa"
	headNew  = "bbbbbbbbbbbb"
	pathMain = "main.go"
	pathUtil = "util.go"
)

func TestMarkCountsOnlyAtItsCommit(t *testing.T) {
	t.Parallel()
	s := New()
	s.Mark(prKey, pathMain, headOld)

	testkit.AssertEqual(t, "same head", s.Viewed(prKey, pathMain, headOld), true)
	testkit.AssertEqual(t, "full hash of the same head", s.Viewed(prKey, pathMain, headOld+"0123456789abcdef0123456789"), true)
	testkit.AssertEqual(t, "other head", s.Viewed(prKey, pathMain, headNew), false)
	testkit.AssertEqual(t, "other file", s.Viewed(prKey, pathUtil, headOld), false)
	testkit.AssertEqual(t, "other pull request", s.Viewed("acme/api#8", pathMain, headOld), false)
}

func TestUnmarkRemovesTheMark(t *testing.T) {
	t.Parallel()
	s := New()
	s.Mark(prKey, pathMain, headOld)
	s.Mark(prKey, pathUtil, headOld)
	s.Unmark(prKey, pathMain)

	testkit.AssertEqual(t, "unmarked", s.Viewed(prKey, pathMain, headOld), false)
	testkit.AssertEqual(t, "other mark kept", s.Viewed(prKey, pathUtil, headOld), true)

	s.Unmark(prKey, pathUtil)
	testkit.AssertEqual(t, "pull request dropped", len(s.data.PullRequests), 0)
}

func TestViewedPathsIsTheCurrentSet(t *testing.T) {
	t.Parallel()
	s := New()
	s.Mark(prKey, pathMain, headNew)
	s.Mark(prKey, pathUtil, headOld)

	got := s.ViewedPaths(prKey, headNew)
	testkit.AssertEqual(t, "count", len(got), 1)
	testkit.AssertEqual(t, "current mark", got[pathMain], true)
}

func TestStaleGroupsByCommit(t *testing.T) {
	t.Parallel()
	s := New()
	s.Mark(prKey, pathUtil, headOld)
	s.Mark(prKey, pathMain, headOld)
	s.Mark(prKey, "new.go", headNew)

	stale := s.Stale(prKey, headNew)
	testkit.AssertEqual(t, "groups", len(stale), 1)
	testkit.AssertSliceEqual(t, "old head paths", stale[headOld], []string{pathMain, pathUtil})

	if got := s.Stale(prKey, headOld); len(got[headNew]) != 1 {
		t.Errorf("Stale at the old head = %v, want the new-head mark", got)
	}
	if got := New().Stale(prKey, headNew); got != nil {
		t.Errorf("Stale on an empty store = %v, want nil", got)
	}
}

func TestReconcileKeepsUnchangedAndDropsChanged(t *testing.T) {
	t.Parallel()
	s := New()
	s.Mark(prKey, pathMain, headOld)
	s.Mark(prKey, pathUtil, headOld)

	testkit.AssertEqual(t, "changed something", s.Reconcile(prKey, headOld, headNew, []string{pathMain}), true)
	testkit.AssertEqual(t, "changed file dropped", s.Viewed(prKey, pathMain, headNew), false)
	testkit.AssertEqual(t, "unchanged file carried over", s.Viewed(prKey, pathUtil, headNew), true)
	testkit.AssertEqual(t, "nothing stale left", len(s.Stale(prKey, headNew)), 0)

	testkit.AssertEqual(t, "second run is a no-op", s.Reconcile(prKey, headOld, headNew, nil), false)
}

func TestReconcileLeavesAMarkMadeAtTheNewHead(t *testing.T) {
	t.Parallel()
	s := New()
	s.Mark(prKey, pathMain, headNew)

	s.Reconcile(prKey, headOld, headNew, []string{pathMain})
	testkit.AssertEqual(t, "fresh mark kept", s.Viewed(prKey, pathMain, headNew), true)
}

func TestSaveAndOpenRoundTrip(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "nested", "viewed.json")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open(missing) error = %v", err)
	}
	s.Mark(prKey, pathMain, headOld)
	if err := s.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	again, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	testkit.AssertEqual(t, "mark survives", again.Viewed(prKey, pathMain, headOld), true)

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	testkit.AssertEqual(t, "no temp file left behind", len(entries), 1)
}

func TestSavePrunesOldPullRequests(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "viewed.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := start
	s.SetClock(func() time.Time { return clock })
	s.Mark("acme/api#1", pathMain, headOld)
	clock = start.Add(Retention + time.Hour)
	s.Mark("acme/api#2", pathMain, headOld)
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	again, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	testkit.AssertEqual(t, "old one pruned", again.Viewed("acme/api#1", pathMain, headOld), false)
	testkit.AssertEqual(t, "recent one kept", again.Viewed("acme/api#2", pathMain, headOld), true)
}

func TestOpenRefusesANewerFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "viewed.json")
	if err := os.WriteFile(path, []byte(`{"version": 99, "pullRequests": {}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); !errors.Is(err, ErrNewerVersion) {
		t.Errorf("Open() error = %v, want ErrNewerVersion", err)
	}
}

func TestOpenRefusesBrokenJSON(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "viewed.json")
	if err := os.WriteFile(path, []byte(`{`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Error("Open() on broken JSON returned no error")
	}
}

func TestNewStoreSaveIsANoOp(t *testing.T) {
	t.Parallel()
	s := New()
	s.Mark(prKey, pathMain, headOld)
	if err := s.Save(); err != nil {
		t.Errorf("Save() on a memory store error = %v", err)
	}
}

func TestSameCommit(t *testing.T) {
	t.Parallel()
	cases := []struct {
		a, b string
		want bool
	}{
		{"", "", true},
		{"", headOld, false},
		{headOld, headOld + "cafe", true},
		{"AAAAAAAAAAAA", headOld, true},
		{"aaaa", "aaaabbbb", false},
		{"aaaa", "aaaa", true},
		{headOld, headNew, false},
	}
	for _, c := range cases {
		testkit.AssertEqual(t, c.a+" vs "+c.b, SameCommit(c.a, c.b), c.want)
	}
}
