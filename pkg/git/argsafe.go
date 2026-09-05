package git

import (
	"errors"
	"fmt"
	"strings"
)

// ErrUnsafeName is wrapped for a branch or ref name that git would read as a
// command-line option instead of as a name.
//
// Names handed to this package come from the Bitbucket API, and git's
// ref-format rules do allow a leading "-": `git check-ref-format
// 'refs/heads/-evil'` exits 0 and `git update-ref refs/heads/-evil HEAD`
// creates the branch, even though the `git branch` porcelain refuses to. git's
// option parser does not stop at the first non-option argument, so such a name
// is consumed as a flag — and a name like "--upload-pack=..." makes git run an
// arbitrary program.
//
// Where git accepts "--" the name is passed after it and stays usable; the
// checkout family, for which git offers no separator that turns an
// option-looking argument back into a branch name, refuses with this error.
var ErrUnsafeName = errors.New("unsafe git name")

// checkName rejects a name that is empty or starts with "-". kind names the
// argument in the message, e.g. "branch".
func checkName(kind, name string) error {
	switch {
	case name == "":
		return fmt.Errorf("%w: empty %s name", ErrUnsafeName, kind)
	case strings.HasPrefix(name, "-"):
		return fmt.Errorf("%w: %s %q would be read as a git option", ErrUnsafeName, kind, name)
	}
	return nil
}
