//go:build demo

package main

import (
	"fmt"
	"time"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/bitbucket/demo"
	"github.com/lsst25/lazybitbucket/pkg/config"
	"github.com/lsst25/lazybitbucket/pkg/tui"
)

// startDemo serves the built-in dataset from an in-process REST server and
// points a real client at it, so --demo exercises the same code path as a live
// account. The returned cleanup shuts the server down.
func startDemo(cfg *config.Config) (bitbucket.ClientInterface, tui.AuthMethod, func(), error) {
	srv, err := demo.NewServerWithDataset(freshDataset(time.Now()))
	if err != nil {
		return nil, "", nil, fmt.Errorf("starting the demo server: %w", err)
	}

	cfg.Bitbucket.Workspace = srv.Data.Workspace.Slug
	cfg.Bitbucket.Repositories = demoRepositories(srv)
	cfg.Bitbucket.Email = ""
	cfg.Bitbucket.BaseURL = srv.URL

	client := bitbucket.NewClient(bitbucket.ClientOpts{
		BaseURL:   srv.URL,
		Token:     "demo-token",
		UserAgent: userAgent(),
		ReadOnly:  cfg.Bitbucket.ReadOnly,
	})
	return client, tui.AuthDemo, func() { _ = srv.Close() }, nil
}

// demoRelativeAge is how old the most recently updated pull request looks in
// demo mode. The dataset is anchored to a fixed date so it is reproducible, and
// left there every pull request would read as months stale.
const demoRelativeAge = time.Hour

// freshDataset is the demo dataset slid forward so its newest pull request was
// updated demoRelativeAge before now, which keeps every relative time in the UI
// ("2h ago", "yesterday") plausible while preserving the intervals between them.
func freshDataset(now time.Time) *demo.Dataset {
	data := demo.NewDataset()
	data.Shift(now.Add(-demoRelativeAge).Sub(data.Newest()))
	return data
}

// demoRepositories lists the dataset's repository slugs in dataset order.
func demoRepositories(srv *demo.Server) []string {
	repos := make([]string, 0, len(srv.Data.Repositories))
	for _, repo := range srv.Data.Repositories {
		repos = append(repos, repo.Slug)
	}
	return repos
}
