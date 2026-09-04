//go:build !demo

package main

import (
	"errors"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/config"
	"github.com/lsst25/lazybitbucket/pkg/tui"
)

// startDemo is the placeholder compiled into ordinary builds: the demo dataset
// and its server only ship with the `demo` build tag.
func startDemo(_ *config.Config) (bitbucket.ClientInterface, tui.AuthMethod, func(), error) {
	return nil, "", nil, errors.New("demo mode not available (rebuild with: go build -tags demo)")
}
