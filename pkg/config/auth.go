package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Credentials stores the Bitbucket connection secret and the workspace it was
// verified against. AuthMethod is "bearer" (API token or access token sent as
// a Bearer header) or "basic" (Atlassian account email + API token).
type Credentials struct {
	AuthMethod string `json:"auth_method"`
	Email      string `json:"email,omitempty"`
	Token      string `json:"token"`
	Workspace  string `json:"workspace,omitempty"`
}

// AuthPath returns the path to auth.json in the config directory
func AuthPath() string {
	return filepath.Join(ConfigDir(), "auth.json")
}

// LoadCredentials reads saved credentials. Returns nil if not found
func LoadCredentials() (*Credentials, error) {
	data, err := os.ReadFile(AuthPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, err
	}

	return &creds, nil
}

// SaveCredentials writes credentials to auth.json with 0600 permissions
func SaveCredentials(creds *Credentials) error {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(AuthPath(), data, 0o600)
}

// ClearCredentials removes auth.json
func ClearCredentials() error {
	err := os.Remove(AuthPath())
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
}
