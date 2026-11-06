// Package viewed remembers which files of a pull request the reader has marked
// as viewed.
//
// Bitbucket's public API has no viewed-file state — the web UI keeps its own
// behind an internal endpoint — so the marks live on this machine only, in one
// small JSON file. Each mark records the source commit the file was viewed at.
// A mark counts only at that commit: once the branch moves, the caller asks
// Bitbucket which of the marked files changed between the two commits (Stale,
// Reconcile), keeps the marks of the files that did not, and drops the rest —
// which is how the web UI behaves too.
//
// The package is pure apart from Open and Save, and imports nothing from the
// rest of the module.
package viewed

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// fileVersion is the schema version written into the file. A file written by a
// newer version is refused rather than overwritten with less than it held.
const fileVersion = 1

// Retention is how long a pull request's marks are kept after the last time
// any of them changed. A merged or declined pull request is never looked at
// again, and nothing else would ever remove its marks.
const Retention = 90 * 24 * time.Hour

// minCommitPrefix is the shortest hash two commits are compared on. Bitbucket
// sends 12-character hashes on a pull request's source commit and full ones
// elsewhere, so a prefix match is the comparison, but not on a stub.
const minCommitPrefix = 7

// ErrNewerVersion is Open's answer for a file a newer lazybitbucket wrote.
var ErrNewerVersion = errors.New("viewed files: written by a newer version")

// Store is the set of viewed marks. It is safe for concurrent use. The zero
// value is not usable; build one with New or Open.
type Store struct {
	mu   sync.Mutex
	path string // "" for a store that is never written
	now  func() time.Time
	data document
}

// document is the file's shape.
type document struct {
	Version      int                 `json:"version"`
	PullRequests map[string]*prMarks `json:"pullRequests"`
}

// prMarks is one pull request's marks: path → the source commit the file was
// viewed at.
type prMarks struct {
	Files   map[string]string `json:"files"`
	Updated time.Time         `json:"updated"`
}

// New is a store that lives in memory only: Save does nothing. It is what the
// demo and the tests use.
func New() *Store {
	return &Store{path: "", now: time.Now, data: emptyDocument()}
}

// Open reads the marks from path. A missing file is an empty store, not an
// error: nothing has been marked yet.
func Open(path string) (*Store, error) {
	s := &Store{path: path, now: time.Now, data: emptyDocument()}
	raw, err := os.ReadFile(path) //nolint:gosec // G304: the path is the app's own state file
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read viewed files: %w", err)
	}
	var doc document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse viewed files %s: %w", path, err)
	}
	if doc.Version > fileVersion {
		return nil, fmt.Errorf("%s: %w", path, ErrNewerVersion)
	}
	if doc.PullRequests == nil {
		doc.PullRequests = map[string]*prMarks{}
	}
	for key, marks := range doc.PullRequests {
		if marks == nil || len(marks.Files) == 0 {
			delete(doc.PullRequests, key)
		}
	}
	doc.Version = fileVersion
	s.data = doc
	return s, nil
}

func emptyDocument() document {
	return document{Version: fileVersion, PullRequests: map[string]*prMarks{}}
}

// SetClock replaces the clock the Updated stamps and the pruning read. Tests
// use it; nil restores time.Now.
func (s *Store) SetClock(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now == nil {
		now = time.Now
	}
	s.now = now
}

// Viewed reports whether path of the pull request is marked as viewed at head.
// A mark made at another commit does not count until Reconcile has carried it
// over.
func (s *Store) Viewed(prKey, path, head string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	marks := s.data.PullRequests[prKey]
	if marks == nil {
		return false
	}
	at, ok := marks.Files[path]
	return ok && SameCommit(at, head)
}

// ViewedPaths is every path of the pull request marked as viewed at head.
func (s *Store) ViewedPaths(prKey, head string) map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	marks := s.data.PullRequests[prKey]
	if marks == nil {
		return nil
	}
	paths := make(map[string]bool, len(marks.Files))
	for path, at := range marks.Files {
		if SameCommit(at, head) {
			paths[path] = true
		}
	}
	return paths
}

// Mark records path as viewed at head.
func (s *Store) Mark(prKey, path, head string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	marks := s.data.PullRequests[prKey]
	if marks == nil {
		marks = &prMarks{Files: map[string]string{}, Updated: time.Time{}}
		s.data.PullRequests[prKey] = marks
	}
	marks.Files[path] = head
	marks.Updated = s.now()
}

// Unmark removes path's mark, at whatever commit it was made.
func (s *Store) Unmark(prKey, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	marks := s.data.PullRequests[prKey]
	if marks == nil {
		return
	}
	delete(marks.Files, path)
	marks.Updated = s.now()
	if len(marks.Files) == 0 {
		delete(s.data.PullRequests, prKey)
	}
}

// Stale groups the pull request's marks that were made at a commit other than
// head by that commit, each group's paths sorted. It is nil when every mark is
// current.
func (s *Store) Stale(prKey, head string) map[string][]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	marks := s.data.PullRequests[prKey]
	if marks == nil {
		return nil
	}
	var stale map[string][]string
	for path, at := range marks.Files {
		if SameCommit(at, head) {
			continue
		}
		if stale == nil {
			stale = map[string][]string{}
		}
		stale[at] = append(stale[at], path)
	}
	for _, paths := range stale {
		slices.Sort(paths)
	}
	return stale
}

// Reconcile carries the marks made at from over to to, except for the paths in
// changed — the files whose content differs between the two commits — whose
// marks are dropped. A mark made at any other commit is left alone, and so is a
// mark on the same path that was made at to in the meantime. It reports
// whether anything changed.
func (s *Store) Reconcile(prKey, from, to string, changed []string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	marks := s.data.PullRequests[prKey]
	if marks == nil {
		return false
	}
	drop := make(map[string]bool, len(changed))
	for _, path := range changed {
		drop[path] = true
	}
	touched := false
	for path, at := range marks.Files {
		if at != from {
			continue
		}
		touched = true
		if drop[path] {
			delete(marks.Files, path)
			continue
		}
		marks.Files[path] = to
	}
	if !touched {
		return false
	}
	marks.Updated = s.now()
	if len(marks.Files) == 0 {
		delete(s.data.PullRequests, prKey)
	}
	return true
}

// Save writes the marks to the store's file, first dropping every pull request
// whose marks have not changed within Retention. The write goes to a temporary
// file in the same directory and is renamed over the old one, so a crash cannot
// leave half a file behind. A store from New has no file and Save does nothing.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == "" {
		return nil
	}
	cutoff := s.now().Add(-Retention)
	maps.DeleteFunc(s.data.PullRequests, func(_ string, marks *prMarks) bool {
		return marks.Updated.Before(cutoff)
	})

	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("encode viewed files: %w", err)
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // G301: the config dir, as config.Save makes it
		return fmt.Errorf("save viewed files: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".viewed-*.json")
	if err != nil {
		return fmt.Errorf("save viewed files: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("save viewed files: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("save viewed files: %w", err)
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		return fmt.Errorf("save viewed files: %w", err)
	}
	return nil
}

// SameCommit reports whether two commit hashes name the same commit: equal, or
// one a prefix of the other of at least minCommitPrefix characters. Two empty
// hashes are the same — a pull request row without a source commit must still
// be markable — but an empty one never matches a real one.
func SameCommit(a, b string) bool {
	if a == "" || b == "" {
		return a == b
	}
	short, long := a, b
	if len(short) > len(long) {
		short, long = long, short
	}
	if len(short) < minCommitPrefix {
		return strings.EqualFold(a, b)
	}
	return strings.EqualFold(long[:len(short)], short)
}
