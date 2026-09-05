// Package bitbucket is the Bitbucket Cloud domain layer: wire-shaped models for
// pull requests, repositories, build statuses and pipelines, plus the REST
// client that fetches and changes them.
//
// Most of the surface is reads. The writes are merging a pull request
// (MergePullRequest, with GetMergeTaskStatus for the merges Bitbucket queues),
// editing one (UpdatePullRequest: title, description, draft state, reviewers),
// approving one and withdrawing that approval (ApprovePullRequest,
// UnapprovePullRequest), and acting on a pipeline run (RunPipeline, which only
// repeats the target of a run that already exists, and StopPipeline). Declining,
// commenting and starting an arbitrary custom pipeline are deliberately absent.
//
// A client built with ClientOpts.ReadOnly refuses every non-GET request with
// ErrReadOnly before the request is built, so nothing can reach Bitbucket that
// would change it — the transport-level half of the application's browse-only
// mode.
//
// It imports nothing from pkg/tui or pkg/config, so the domain stays
// independently testable and the dependency runs one way only.
package bitbucket
