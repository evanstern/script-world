package worlds

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/evanstern/script-world/internal/world"
)

// IsPathArg reports whether arg should be treated as a path — resolved
// exactly as today, bypassing name resolution entirely — rather than a bare
// world name (research.md D3): it contains a path separator, or begins with
// '.' or '~'. This subsumes "..", "./name", and "~/name".
func IsPathArg(arg string) bool {
	return strings.Contains(arg, "/") || strings.HasPrefix(arg, ".") || strings.HasPrefix(arg, "~")
}

// ErrAmbiguous is returned by Resolve when a bare name matches both a
// worlds-home world and a registry-remembered world at a different path
// (FR-011).
type ErrAmbiguous struct {
	Name  string
	Paths []string
}

func (e *ErrAmbiguous) Error() string {
	lines := make([]string, len(e.Paths))
	for i, p := range e.Paths {
		lines[i] = "  " + p
	}
	return fmt.Sprintf("name %q is ambiguous:\n%s\nuse a path to disambiguate", e.Name, strings.Join(lines, "\n"))
}

// ErrNotFound is returned by Resolve when a bare name matches nothing
// (FR-007): the error names the worlds home searched and suggests `ps --all`.
type ErrNotFound struct {
	Name       string
	WorldsHome string
}

func (e *ErrNotFound) Error() string {
	return fmt.Sprintf("no world named %q (searched %s and the known-worlds list) — try `scriptworld ps --all`", e.Name, e.WorldsHome)
}

// Resolve turns a bare world name into an absolute world directory,
// worlds-home-first then registry (data-model.md "Name resolution",
// FR-007/FR-011). Callers must route path-shaped arguments (IsPathArg)
// around Resolve entirely — it only ever sees names.
func Resolve(name string) (string, error) {
	home, err := WorldsHome()
	if err != nil {
		return "", err
	}
	homeCandidate := filepath.Join(home, name)
	homeOK := isReadableWorld(homeCandidate)

	reg, err := LoadRegistry()
	if err != nil {
		return "", err
	}
	regPath, hasReg := reg.Worlds[name]
	regOK := hasReg && isReadableWorld(regPath)

	switch {
	case homeOK && regOK:
		if samePath(homeCandidate, regPath) {
			return homeCandidate, nil
		}
		return "", &ErrAmbiguous{Name: name, Paths: []string{homeCandidate, regPath}}
	case homeOK:
		return homeCandidate, nil
	case regOK:
		return regPath, nil
	default:
		return "", &ErrNotFound{Name: name, WorldsHome: home}
	}
}

func isReadableWorld(dir string) bool {
	_, err := world.Open(dir)
	return err == nil
}

func samePath(a, b string) bool {
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	return errA == nil && errB == nil && aa == bb
}
