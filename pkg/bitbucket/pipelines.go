package bitbucket

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// ListPipelines returns pipeline runs for a repository, newest first.
//
// A build status only links back to a run through the build number in its URL,
// so the usual way to find the run behind a pull request's status is to filter
// by branch: target.branch matches ref_name on branch runs and source on
// pull-request runs.
func (c *Client) ListPipelines(
	ctx context.Context, workspace, repoSlug string, opts ListPipelinesOptions,
) ([]Pipeline, error) {
	maxItems := orDefault(opts.MaxItems, defaultMaxPipelines)

	query := url.Values{}
	query.Set("pagelen", strconv.Itoa(pageLen(maxPageLenPipelines, maxItems)))
	query.Set("sort", orString(opts.Sort, "-created_on"))
	if opts.TargetBranch != "" {
		query.Set("target.branch", opts.TargetBranch)
	}

	pipelines, err := getAll[Pipeline](ctx, c, repoPath(workspace, repoSlug)+"/pipelines", query, maxItems)
	if err != nil {
		return nil, fmt.Errorf("list pipelines %s/%s: %w", workspace, repoSlug, err)
	}
	return pipelines, nil
}

// GetPipeline returns one run. pipelineRef is either a braced UUID or a build
// number; the braces are percent-encoded for the path.
func (c *Client) GetPipeline(ctx context.Context, workspace, repoSlug, pipelineRef string) (*Pipeline, error) {
	var pipeline Pipeline
	if err := c.getJSON(ctx, pipelinePath(workspace, repoSlug, pipelineRef), nil, &pipeline); err != nil {
		return nil, fmt.Errorf("get pipeline %s/%s %s: %w", workspace, repoSlug, pipelineRef, err)
	}
	return &pipeline, nil
}

// ListPipelineSteps returns the steps of a run in execution order.
func (c *Client) ListPipelineSteps(
	ctx context.Context, workspace, repoSlug, pipelineRef string,
) ([]PipelineStep, error) {
	query := url.Values{}
	query.Set("pagelen", strconv.Itoa(maxPageLenDefault))

	path := pipelinePath(workspace, repoSlug, pipelineRef) + "/steps"
	steps, err := getAll[PipelineStep](ctx, c, path, query, 0)
	if err != nil {
		return nil, fmt.Errorf("list pipeline steps %s/%s %s: %w", workspace, repoSlug, pipelineRef, err)
	}
	return steps, nil
}

// GetPipelineStepLog returns a step's log as text. Logs run to megabytes, so
// opts.TailBytes asks for a suffix with a Range header rather than the whole
// file; the API documents and encourages Range on this endpoint.
//
// The log is served as application/octet-stream whatever its content, and the
// API answers 406 Not Acceptable for an Accept that admits only text — hence
// acceptLog rather than acceptText.
//
// Once a step finishes its log moves to long-term storage and the request is
// redirected there. Go's default redirect policy drops the Authorization header
// across hosts, which is what that presigned URL needs.
func (c *Client) GetPipelineStepLog(
	ctx context.Context, workspace, repoSlug, pipelineRef, stepUUID string, opts StepLogOptions,
) (string, error) {
	path := pipelinePath(workspace, repoSlug, pipelineRef) + "/steps/" + url.PathEscape(stepUUID) + "/log"

	var headers http.Header
	if opts.TailBytes > 0 {
		headers = http.Header{"Range": []string{"bytes=-" + strconv.Itoa(opts.TailBytes)}}
	}

	body, _, err := c.getRaw(ctx, path, "", acceptLog, headers)
	if err != nil {
		return "", fmt.Errorf("get step log %s/%s %s %s: %w", workspace, repoSlug, pipelineRef, stepUUID, err)
	}
	return string(body), nil
}

// pipelinePath is the API path of one run. pipelineRef arrives braced from a
// pipeline payload, and the braces are not legal in a path segment.
func pipelinePath(workspace, repoSlug, pipelineRef string) string {
	return repoPath(workspace, repoSlug) + "/pipelines/" + url.PathEscape(pipelineRef)
}
