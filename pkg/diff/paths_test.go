package diff

import (
	"testing"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

func TestUnquoteC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "nothing to unquote", input: `"plain.txt"`, want: "plain.txt"},
		{name: "empty", input: `""`, want: ""},
		{
			name:  "every named escape",
			input: `"\a\b\f\n\r\t\v\\\"end"`,
			want:  "\a\b\f\n\r\t\v\\\"end",
		},
		{name: "unknown escape keeps the letter", input: `"a\zb"`, want: "azb"},
		{
			name:  "octal escapes are bytes",
			input: `"docs/caf\303\251.md"`,
			want:  "docs/café.md",
		},
		{name: "short octal escape", input: `"a\7b"`, want: "a\ab"},
		{name: "trailing backslash inside the quotes", input: `"ab\"`, want: `ab\`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			testkit.AssertEqual(t, "unquoteC", unquoteC(test.input), test.want)
		})
	}
}

func TestCleanPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		raw        string
		sidePrefix string
		want       string
	}{
		{name: "old side prefix", raw: "a/pkg/app.go", sidePrefix: oldPathPrefix, want: "pkg/app.go"},
		{name: "new side prefix", raw: "b/pkg/app.go", sidePrefix: newPathPrefix, want: "pkg/app.go"},
		{name: "no prefix to strip", raw: "pkg/app.go", sidePrefix: oldPathPrefix, want: "pkg/app.go"},
		{name: "dev null", raw: devNull, sidePrefix: oldPathPrefix, want: ""},
		{
			name: "timestamp trailer", raw: "a/x.txt\t2026-01-01 10:00:00 +0100",
			sidePrefix: oldPathPrefix, want: "x.txt",
		},
		{name: "quoted with a space", raw: `"a/docs/my file.md"`, sidePrefix: oldPathPrefix, want: quotedPath},
		{name: "unclosed quote is left alone", raw: `"a/x`, sidePrefix: oldPathPrefix, want: `"a/x`},
		{name: "no path at all", raw: "", sidePrefix: oldPathPrefix, want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			testkit.AssertEqual(t, "cleanPath", cleanPath(test.raw, test.sidePrefix), test.want)
		})
	}
}

func TestSplitGitPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantOld string
		wantNew string
	}{
		{name: "plain pair", input: "a/x.go b/x.go", wantOld: "a/x.go", wantNew: "b/x.go"},
		{
			name: "equal tails win over an earlier split", input: "a/docs/my file.md b/docs/my file.md",
			wantOld: "a/docs/my file.md", wantNew: "b/docs/my file.md",
		},
		{
			name: "rename has different tails", input: "a/old/name.go b/new/name.go",
			wantOld: "a/old/name.go", wantNew: "b/new/name.go",
		},
		{
			name: "quoted pair", input: `"a/docs/my file.md" "b/docs/my file.md"`,
			wantOld: `"a/docs/my file.md"`, wantNew: `"b/docs/my file.md"`,
		},
		{name: "no prefixes", input: "x.go y.go", wantOld: "x.go", wantNew: "y.go"},
		{name: "no split point", input: "a/x.go", wantOld: "a/x.go", wantNew: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			gotOld, gotNew := splitGitPaths(test.input)
			testkit.AssertEqual(t, "old", gotOld, test.wantOld)
			testkit.AssertEqual(t, "new", gotNew, test.wantNew)
		})
	}
}
