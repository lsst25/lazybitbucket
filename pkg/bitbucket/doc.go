// Package bitbucket is the Bitbucket Cloud domain layer: wire-shaped models for
// pull requests, repositories, build statuses and pipelines, plus the read-only
// REST client that fetches them.
//
// The package is read-only in this phase: it exposes no write methods and never
// issues a non-GET request against Bitbucket. Anything that mutates state
// (merge, approve, decline, comment, trigger a pipeline) is deliberately absent.
//
// It imports nothing from pkg/tui or pkg/config, so the domain stays
// independently testable and the dependency runs one way only.
package bitbucket
