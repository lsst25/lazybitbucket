package bitbucket

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
)

// defaultFetchConcurrency is how many repositories FetchPullRequests queries at
// once. Bitbucket allows roughly 1,000 requests per hour against
// /2.0/repositories/* and asks for bounded concurrency, and the TUI runs its
// own prefetch and preview streams alongside this one, so two is deliberate
// rather than a placeholder.
const defaultFetchConcurrency = 2

// RepoPullRequests is one repository's slice of a cross-repository fetch.
type RepoPullRequests struct {
	// Repo is the slug exactly as it was passed in.
	Repo string
	// PullRequests is nil when Err is set.
	PullRequests []PullRequest
	// Err is this repository's failure. It never aborts the other repositories:
	// one unreadable repository must not blank the whole list pane.
	Err error
}

// FetchPullRequests lists pull requests for every repository concurrently,
// bounded by concurrency (0 means the default of 2), and returns one entry per
// repository in input order.
//
// It never fails as a whole — a per-repository failure lands in that entry's
// Err — and it stops starting work once ctx is cancelled. It calls client from
// several goroutines, so any ClientInterface handed to it must be
// goroutine-safe.
func FetchPullRequests(
	ctx context.Context,
	client ClientInterface,
	workspace string,
	repos []string,
	opts ListPullRequestsOptions,
	concurrency int,
) []RepoPullRequests {
	results := make([]RepoPullRequests, len(repos))
	slots := make(chan struct{}, orDefault(concurrency, defaultFetchConcurrency))

	var wait sync.WaitGroup
	for index, repo := range repos {
		results[index] = RepoPullRequests{Repo: repo, PullRequests: nil, Err: nil}

		wait.Go(func() {
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
				results[index].Err = fmt.Errorf("list pull requests %s/%s: %w", workspace, repo, ctx.Err())
				return
			}

			prs, err := client.ListPullRequests(ctx, workspace, repo, opts)
			results[index].PullRequests = prs
			results[index].Err = err
		})
	}
	wait.Wait()

	return results
}

// MergePullRequests flattens a fetch into one list ordered the way the list
// pane shows it: most recently updated first, then repository slug, then
// descending id. Repositories that failed are skipped — their error is reported
// separately rather than as a gap in the list.
func MergePullRequests(results []RepoPullRequests) []PullRequest {
	total := 0
	for _, result := range results {
		if result.Err == nil {
			total += len(result.PullRequests)
		}
	}

	type entry struct {
		repo string
		pr   PullRequest
	}

	entries := make([]entry, 0, total)
	for _, result := range results {
		if result.Err != nil {
			continue
		}
		for _, pr := range result.PullRequests {
			entries = append(entries, entry{repo: result.Repo, pr: pr})
		}
	}

	slices.SortStableFunc(entries, func(a, b entry) int {
		if byUpdated := b.pr.UpdatedOn.Compare(a.pr.UpdatedOn); byUpdated != 0 {
			return byUpdated
		}
		if byRepo := strings.Compare(a.repo, b.repo); byRepo != 0 {
			return byRepo
		}
		return cmp.Compare(b.pr.ID, a.pr.ID)
	})

	merged := make([]PullRequest, 0, total)
	for _, item := range entries {
		merged = append(merged, item.pr)
	}
	return merged
}
