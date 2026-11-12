package main

import (
	"regexp"
	"runtime/debug"
	"strings"
)

// releaseVersion matches a version number such as "v0.1.0" or "0.1.0"; a commit
// hash does not match, even when it starts with a digit.
var releaseVersion = regexp.MustCompile(`^v?\d+\.\d+`)

// devVersion is what a build without a version stamp reports.
const devVersion = "dev"

// shortRevision is how many characters of a commit hash a dev build shows.
const shortRevision = 7

// resolveVersion is the version to show. A stamped version (ldflags, from the
// git tag) wins. An unstamped build, such as `go install`, reads the module
// version or the commit from the build information the Go toolchain embeds.
//
// The result is the text every surface shows as it is — `lazybitbucket
// version`, the splash and the status panel — so the form is decided here and
// nowhere else: "v0.1.0" for a version number, the form the tag and the Go
// module version already have, and "dev-…" for anything else.
func resolveVersion(stamped string, info *debug.BuildInfo) string {
	if stamped != "" && stamped != devVersion {
		return versionLabel(stamped)
	}
	if info == nil {
		return devVersion
	}
	if module := info.Main.Version; module != "" && module != "(devel)" {
		return versionLabel(module)
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

// versionLabel gives a raw version its shown form. A version number gets its
// leading "v" when it lacks one (VERSION=0.2.0). Anything else is a dev build:
// `git describe --always` with no tag yields a bare commit hash, and so does a
// Nix build, which stamps the short revision.
func versionLabel(raw string) string {
	switch {
	case strings.HasPrefix(raw, devVersion):
		return raw
	case !releaseVersion.MatchString(raw):
		return devVersion + "-" + raw
	case strings.HasPrefix(raw, "v"):
		return raw
	default:
		return "v" + raw
	}
}
