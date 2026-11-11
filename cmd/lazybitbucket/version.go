package main

import (
	"regexp"
	"runtime/debug"
	"strings"
)

// releaseVersion matches a version number such as "0.1.0"; a commit hash does
// not match, even when it starts with a digit.
var releaseVersion = regexp.MustCompile(`^\d+\.\d+`)

// devVersion is what a build without a version stamp reports.
const devVersion = "dev"

// shortRevision is how many characters of a commit hash a dev build shows.
const shortRevision = 7

// resolveVersion is the version to show. A stamped version (ldflags, from the
// git tag) wins. An unstamped build, such as `go install`, reads the module
// version or the commit from the build information the Go toolchain embeds.
// The result has no leading "v": the UI adds it.
func resolveVersion(stamped string, info *debug.BuildInfo) string {
	if stamped != "" && stamped != devVersion {
		stamped = strings.TrimPrefix(stamped, "v")
		if !releaseVersion.MatchString(stamped) {
			// `git describe --always` with no tag yields a bare commit hash.
			return devVersion + "-" + stamped
		}
		return stamped
	}
	if info == nil {
		return devVersion
	}
	if module := info.Main.Version; module != "" && module != "(devel)" {
		return strings.TrimPrefix(module, "v")
	}

	revision, dirty := "", false
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			dirty = setting.Value == "true"
		default:
		}
	}
	if revision == "" {
		return devVersion
	}
	revision = revision[:min(len(revision), shortRevision)]
	if dirty {
		revision += "-dirty"
	}
	return devVersion + "-" + revision
}
