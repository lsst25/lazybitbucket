package bitbucket

import (
	"context"
	"fmt"
	"net/url"
)

// GetCurrentUser returns the account the credentials belong to. It is the
// client's connection test, and the source of the UUID the "Mine" and
// "Reviewing" tabs filter on.
//
// Repository, project and workspace access tokens are not attached to a user
// and answer 403 here; those setups have to name the account another way.
func (c *Client) GetCurrentUser(ctx context.Context) (*Account, error) {
	var account Account
	if err := c.getJSON(ctx, "/user", nil, &account); err != nil {
		return nil, fmt.Errorf("get current user: %w", err)
	}
	return &account, nil
}

// GetWorkspace returns the workspace with this slug (or braced UUID).
func (c *Client) GetWorkspace(ctx context.Context, workspace string) (*Workspace, error) {
	var result Workspace
	if err := c.getJSON(ctx, "/workspaces/"+url.PathEscape(workspace), nil, &result); err != nil {
		return nil, fmt.Errorf("get workspace %s: %w", workspace, err)
	}
	return &result, nil
}
