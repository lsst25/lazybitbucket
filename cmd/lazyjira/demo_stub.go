//go:build !demo

package main

import (
	"errors"

	"github.com/lsst25/lazybitbucket/pkg/config"
	"github.com/lsst25/lazybitbucket/pkg/jira"
	"github.com/lsst25/lazybitbucket/pkg/tui"
)

func startDemo(_ *config.Config) (jira.ClientInterface, tui.AuthMethod, func(), error) {
	return nil, "", nil, errors.New("demo mode not available (rebuild with: go build -tags demo)")
}
