package main

import (
	"runtime/debug"
	"testing"
)

func TestResolveVersion(t *testing.T) {
	t.Parallel()
	vcs := func(revision, modified string) *debug.BuildInfo {
		return &debug.BuildInfo{
			Main: debug.Module{Version: "(devel)"},
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: revision},
				{Key: "vcs.modified", Value: modified},
			},
		}
	}
	tests := []struct {
		name    string
		stamped string
		info    *debug.BuildInfo
		want    string
	}{
		{"stamped wins", "0.2.0", vcs("abcdef1234567", "false"), "0.2.0"},
		{"stamped loses its v", "v0.2.0", nil, "0.2.0"},
		{"stamped describe output", "0.2.0-3-gabc1234-dirty", nil, "0.2.0-3-gabc1234-dirty"},
		{"untagged build stamps a bare hash", "07388ec", nil, "dev-07388ec"},
		{"untagged dirty build", "07388ec-dirty", nil, "dev-07388ec-dirty"},
		{"module version from go install", "dev", &debug.BuildInfo{Main: debug.Module{Version: "v0.3.1"}}, "0.3.1"},
		{"commit of a checkout", "dev", vcs("abcdef1234567", "false"), "dev-abcdef1"},
		{"dirty checkout", "dev", vcs("abcdef1234567", "true"), "dev-abcdef1-dirty"},
		{"no build information", "dev", nil, "dev"},
		{"no commit recorded", "", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, "dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveVersion(tt.stamped, tt.info); got != tt.want {
				t.Errorf("version = %q, want %q", got, tt.want)
			}
		})
	}
}
