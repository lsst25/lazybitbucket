//go:build demo

package main

import (
	"github.com/lsst25/lazybitbucket/pkg/config"
	"github.com/lsst25/lazybitbucket/pkg/jira"
	"github.com/lsst25/lazybitbucket/pkg/tui"
)

func startDemo(cfg *config.Config) (jira.ClientInterface, tui.AuthMethod, func(), error) {
	srv, err := jira.NewDemoServer()
	if err != nil {
		return nil, "", nil, err
	}
	cfg.Jira.Host = "https://demo.atlassian.net"
	cfg.Jira.Email = "demo@lazyjira.dev"
	client := jira.NewClient(srv.URL, cfg.Jira.Email, "demo-token")
	cleanup := func() { _ = srv.Close() }
	return client, tui.AuthDemo, cleanup, nil
}
