package bitbucket

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
)

// defaultFetchConcurrency is how many repositories FetchPullRequests and
// FetchPipelines query at once when the caller names no number. Bitbucket
// allows roughly 1,000 requests per hour against /2.0/repositories/* and asks
// for bounded concurrency, so the default is cautious. A caller that has
// measured a higher number passes it: the TUI does for the pull request list
// (see listFetchConcurrency in pkg/tui).
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

// RepoPipelines is one repository's slice of a cross-repository pipeline fetch.
type RepoPipelines struct {
	// Repo is the slug exactly as it was passed in.
	Repo string
	// Pipelines is nil when Err is set.
	Pipelines []Pipeline
	// Err is this repository's failure. It never aborts the other repositories:
	// one repository without Pipelines enabled must not blank the whole list.
	Err error
}

// PipelineEntry is one run together with the repository slug it was fetched
// from. The slug travels beside the run rather than being read back out of
// Pipeline.Repository, which is a payload field the caller did not choose and
// which a trimmed response may leave empty.
type PipelineEntry struct {
	Repo     string
	Pipeline Pipeline
}

// FetchPipelines lists pipeline runs for every repository concurrently, bounded
// by concurrency (0 means the default of 2), and returns one entry per
// repository in input order.
//
// It never fails as a whole — a per-repository failure lands in that entry's
// Err, which matters more here than for pull requests because a repository with
// Pipelines switched off answers an error every time — and it stops starting
// work once ctx is cancelled. It calls client from several goroutines, so any
// ClientInterface handed to it must be goroutine-safe.
func FetchPipelines(
	ctx context.Context,
	client ClientInterface,
	workspace string,
	repos []string,
	opts ListPipelinesOptions,
	concurrency int,
) []RepoPipelines {
	results := make([]RepoPipelines, len(repos))
	slots := make(chan struct{}, orDefault(concurrency, defaultFetchConcurrency))

	var wait sync.WaitGroup
	for index, repo := range repos {
		results[index] = RepoPipelines{Repo: repo, Pipelines: nil, Err: nil}

		wait.Go(func() {
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
				results[index].Err = fmt.Errorf("list pipelines %s/%s: %w", workspace, repo, ctx.Err())
				return
			}

			pipelines, err := client.ListPipelines(ctx, workspace, repo, opts)
			results[index].Pipelines = pipelines
			results[index].Err = err
		})
	}
	wait.Wait()

	return results
}

// MergePipelines flattens a fetch into one list ordered the way the pipelines
// pane shows it: newest run first, then repository slug, then descending build
// number. Repositories that failed are skipped — their error is reported
// separately rather than as a gap in the list.
func MergePipelines(results []RepoPipelines) []PipelineEntry {
	total := 0
	for _, result := range results {
		if result.Err == nil {
			total += len(result.Pipelines)
		}
	}

	entries := make([]PipelineEntry, 0, total)
	for _, result := range results {
		if result.Err != nil {
			continue
		}
		for _, pipeline := range result.Pipelines {
			entries = append(entries, PipelineEntry{Repo: result.Repo, Pipeline: pipeline})
		}
	}

	slices.SortStableFunc(entries, func(a, b PipelineEntry) int {
		if byCreated := b.Pipeline.CreatedOn.Compare(a.Pipeline.CreatedOn); byCreated != 0 {
			return byCreated
		}
		if byRepo := strings.Compare(a.Repo, b.Repo); byRepo != 0 {
			return byRepo
		}
		return cmp.Compare(b.Pipeline.BuildNumber, a.Pipeline.BuildNumber)
	})

	return entries
}
