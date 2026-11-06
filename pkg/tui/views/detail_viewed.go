package views

// viewedFiles is the set of files the reader has marked as viewed, for one
// pull request. The app owns the marks (pkg/viewed) and pushes the current set
// in; the view only draws it. It is keyed by pull request, so a set pushed for
// one pull request is never drawn over another's file list.
type viewedFiles struct {
	prKey string
	paths map[string]bool
}

// SetViewedFiles replaces the viewed set. Like the review setters it is
// dropped for a pull request that is not on screen, so a fetch for another
// pull request cannot wipe the marks the reader is looking at.
func (d *DetailView) SetViewedFiles(prKey string, paths map[string]bool) {
	if !d.keyMatches(prKey) {
		return
	}
	d.viewed = viewedFiles{prKey: prKey, paths: paths}
	d.invalidateBlocksFor(listFiles, listDiffLines)
}

// FileViewed reports whether path of the pull request on screen is marked as
// viewed.
func (d *DetailView) FileViewed(path string) bool {
	if d.pr == nil || d.viewed.prKey != d.pr.Key() {
		return false
	}
	return d.viewed.paths[path]
}

// ViewedFileCount is how many of the listed changed files are marked as
// viewed. A mark on a path the list does not hold — a file a later push took
// out of the pull request — is not counted.
func (d *DetailView) ViewedFileCount() int {
	count := 0
	for i := range d.files.items {
		if d.FileViewed(d.files.items[i].Path()) {
			count++
		}
	}
	return count
}

// NextUnviewedFile is the first changed file after path that is not marked as
// viewed, wrapping round to the top of the list, and false when every other
// file is viewed. An empty or unlisted path starts the search at the top.
func (d *DetailView) NextUnviewedFile(path string) (string, bool) {
	n := len(d.files.items)
	start := d.fileIndex(path)
	for step := 1; step <= n; step++ {
		i := (start + step) % n
		if start < 0 {
			i = step - 1
		}
		candidate := d.files.items[i].Path()
		if candidate != path && !d.FileViewed(candidate) {
			return candidate, true
		}
	}
	return "", false
}

// SelectFile puts the Files tab's cursor on path. It reports false, and moves
// nothing, outside the Files tab or for a path the list does not hold.
func (d *DetailView) SelectFile(path string) bool {
	i := d.fileIndex(path)
	if i < 0 || d.listKind() != listFiles {
		return false
	}
	d.listCursor = i
	return true
}
