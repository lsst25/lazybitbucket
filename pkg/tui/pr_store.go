package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
)

// repoRows is what one repository last answered for one fetch key.
type repoRows struct {
	prs       []bitbucket.PullRequest
	fetchedAt time.Time
}

// prStore holds every list answer in hand: per fetch key, per repository. It is
// the source the flat merged list is built from.
//
// A repository's rows sit under the slug that was asked for, never under a slug
// read back from a pull request. Replacing one repository is then a plain
// overwrite: it cannot leave rows behind, or double them, when the two are
// spelled differently. The zero value is ready to use.
type prStore struct {
	byKey map[string]map[string]repoRows
}

// put records a repository's answer, replacing the one before it.
func (s *prStore) put(fetchKey, slug string, prs []bitbucket.PullRequest, at time.Time) {
	if s.byKey == nil {
		s.byKey = make(map[string]map[string]repoRows)
	}
	if s.byKey[fetchKey] == nil {
		s.byKey[fetchKey] = make(map[string]repoRows)
	}
	s.byKey[fetchKey][slug] = repoRows{prs: prs, fetchedAt: at}
}

// drop forgets a repository, which makes it unloaded again.
func (s *prStore) drop(fetchKey, slug string) {
	delete(s.byKey[fetchKey], slug)
}

// get answers a repository's rows and whether it is loaded at all. A loaded
// repository with no pull requests is not the same as one never asked for.
func (s *prStore) get(fetchKey, slug string) (repoRows, bool) {
	rows, ok := s.byKey[fetchKey][slug]
	return rows, ok
}

// results is a fetch key's rows in the shape bitbucket.MergePullRequests takes.
func (s *prStore) results(fetchKey string) []bitbucket.RepoPullRequests {
	out := make([]bitbucket.RepoPullRequests, 0, len(s.byKey[fetchKey]))
	for slug, rows := range s.byKey[fetchKey] {
		out = append(out, bitbucket.RepoPullRequests{Repo: slug, PullRequests: rows.prs, Err: nil})
	}
	return out
}

// patch replaces a pull request wherever the store holds it.
func (s *prStore) patch(pr *bitbucket.PullRequest) {
	key := pr.Key()
	for _, repos := range s.byKey {
		for _, rows := range repos {
			for index := range rows.prs {
				if rows.prs[index].Key() == key {
					rows.prs[index] = *pr
				}
			}
		}
	}
}

// add puts a pull request into every loaded list of open pull requests for its
// repository that does not hold it yet, and answers the fetch keys it changed.
// A fetch key leads with the state it asked for (see fetchKeyOf). A key that
// has not loaded the repository is left alone: one row would read as the whole.
func (s *prStore) add(slug string, pr *bitbucket.PullRequest, wantsState func(fetchKey string) bool) []string {
	var changed []string
	for fetchKey, repos := range s.byKey {
		rows, loaded := repos[slug]
		if !loaded || !wantsState(fetchKey) || holds(rows.prs, pr.Key()) {
			continue
		}
		rows.prs = append([]bitbucket.PullRequest{*pr}, rows.prs...)
		repos[slug] = rows
		changed = append(changed, fetchKey)
	}
	return changed
}

func holds(prs []bitbucket.PullRequest, key string) bool {
	for index := range prs {
		if prs[index].Key() == key {
			return true
		}
	}
	return false
}

// storeFetched records the answers of one fetch and rebuilds the flat list of
// its key. A repository that failed is forgotten: its rows leave the list and
// it counts as unloaded, so its count reads as unknown rather than as stale.
func (a *App) storeFetched(fetchKey string, results []bitbucket.RepoPullRequests) {
	now := a.nowFn()
	for _, result := range results {
		if result.Err != nil {
			a.fetched.drop(fetchKey, result.Repo)
			continue
		}
		a.fetched.put(fetchKey, result.Repo, result.PullRequests, now)
	}
	a.rebuildMerged(fetchKey)
}

// rebuildMerged derives a fetch key's flat list from the store. The entry is
// set even when it is empty, because its presence is what marks a tab as
// loaded (see refilterTabs).
func (a *App) rebuildMerged(fetchKey string) {
	if a.merged == nil {
		a.merged = make(map[string][]bitbucket.PullRequest)
	}
	a.merged[fetchKey] = bitbucket.MergePullRequests(a.fetched.results(fetchKey))
}

// repoDue reports whether a repository is worth a request for a fetch key: it
// was never loaded, or it is older than the refresh tick would have left it.
// The tick refreshes only the repositories in scope, so one that was out of
// scope for a while is due when it comes back. Without a refresh interval
// nothing goes stale by itself; the refresh key is what re-reads the list then.
func (a *App) repoDue(fetchKey, slug string) bool {
	rows, loaded := a.fetched.get(fetchKey, slug)
	if !loaded {
		return true
	}
	return a.refreshEvery > 0 && a.nowFn().Sub(rows.fetchedAt) >= a.refreshEvery
}

// ensureScopeLoaded is the one rule of the scoped fetch: the list holds, for
// the tab in view, every repository in scope, and none of them stale. Whatever
// changes the tab or the scope calls it rather than deciding for itself whether
// a request is due — a repository pick in either view, a tab switch, the way
// back into the pull request view, and the answer of a fetch.
//
// skip names the repositories not to ask for. The answer of a fetch passes the
// ones it has just covered: a repository that failed is still due, and without
// the skip it would be asked for again as soon as it answered.
func (a *App) ensureScopeLoaded(skip map[string]bool) tea.Cmd {
	fetchKey := a.fetchKeyOf(a.prList.GetTabIndex())
	var due []string
	for _, slug := range a.scopeRepos() {
		if !skip[slug] && a.repoDue(fetchKey, slug) {
			due = append(due, slug)
		}
	}
	return a.requestPullRequests(fetchKey, due)
}
