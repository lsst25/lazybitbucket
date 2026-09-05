//go:build demo

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/demo"
	"github.com/lsst25/lazybitbucket/pkg/config"
	"github.com/lsst25/lazybitbucket/pkg/tui"
)

func TestStartDemo_ConfiguresWorkspaceAndRepositories(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{}
	client, authMethod, cleanup, err := startDemo(cfg)
	if err != nil {
		t.Fatalf("startDemo: %v", err)
	}
	t.Cleanup(cleanup)

	assertEqual(t, "auth method", authMethod, tui.AuthDemo)
	assertEqual(t, "workspace", cfg.Bitbucket.Workspace, demo.Workspace)
	if len(cfg.Bitbucket.Repositories) == 0 {
		t.Fatal("repositories = empty, want the demo dataset's repositories")
	}

	repos, err := client.ListRepositories(t.Context(), cfg.Bitbucket.Workspace,
		bitbucket.ListRepositoriesOptions{})
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	slugs := make([]string, 0, len(repos))
	for _, repo := range repos {
		slugs = append(slugs, repo.Slug)
	}
	assertEqual(t, "served repositories",
		strings.Join(slugs, ","), strings.Join(cfg.Bitbucket.Repositories, ","))
}

// --demo --read-only builds a demo client that refuses writes too, so the
// read-only surface can be demonstrated without an account.
func TestStartDemo_ReadOnlyReachesTheClient(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{}
	cfg.Bitbucket.ReadOnly = true
	client, _, cleanup, err := startDemo(cfg)
	if err != nil {
		t.Fatalf("startDemo: %v", err)
	}
	t.Cleanup(cleanup)

	real, ok := client.(*bitbucket.Client)
	if !ok {
		t.Fatalf("client is %T, want *bitbucket.Client", client)
	}
	assertEqual(t, "ReadOnly", real.ReadOnly(), true)
}

func TestStartDemo_DefaultsToWritable(t *testing.T) {
	t.Parallel()
	client, _, cleanup, err := startDemo(&config.Config{})
	if err != nil {
		t.Fatalf("startDemo: %v", err)
	}
	t.Cleanup(cleanup)

	real, ok := client.(*bitbucket.Client)
	if !ok {
		t.Fatalf("client is %T, want *bitbucket.Client", client)
	}
	assertEqual(t, "ReadOnly", real.ReadOnly(), false)
}

func TestFreshDataset_IsAnHourOldAndKeepsIntervals(t *testing.T) {
	t.Parallel()

	now := time.Date(2027, time.January, 2, 15, 4, 5, 0, time.UTC)
	fixed := demo.NewDataset()
	shifted := freshDataset(now)

	assertEqual(t, "Newest()", shifted.Newest(), now.Add(-demoRelativeAge))

	// The whole world moved by one delta, so the gap between the newest pull
	// request and any other one is unchanged.
	delta := shifted.Newest().Sub(fixed.Newest())
	for slug, pullRequests := range shifted.PullRequests {
		for index, pullRequest := range pullRequests {
			original := fixed.PullRequests[slug][index]
			assertEqual(t, slug+" id", pullRequest.ID, original.ID)
			assertEqual(t, slug+" updated", pullRequest.UpdatedOn, original.UpdatedOn.Add(delta))
		}
	}
}

func TestStartDemo_ServesRecentPullRequests(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	client, _, cleanup, err := startDemo(cfg)
	if err != nil {
		t.Fatalf("startDemo: %v", err)
	}
	t.Cleanup(cleanup)

	prs, err := client.ListPullRequests(t.Context(), cfg.Bitbucket.Workspace,
		cfg.Bitbucket.Repositories[0], bitbucket.ListPullRequestsOptions{})
	if err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}
	if len(prs) == 0 {
		t.Fatal("no pull requests served in demo mode")
	}

	// The dataset is slid forward at startup, so its newest pull request must
	// look recent rather than months old.
	age := time.Since(prs[0].UpdatedOn)
	if age < 0 || age > 2*demoRelativeAge {
		t.Errorf("newest pull request age = %s, want between 0 and %s", age, 2*demoRelativeAge)
	}
}
