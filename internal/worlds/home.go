// Package worlds implements the client-side instance manager
// (specs/008-instance-manager): the worlds home, an advisory registry of
// out-of-home worlds, name-vs-path resolution, discovery, and liveness
// probing. Nothing here is required for a world to run — it is a
// convenience layer over the self-contained save directory (internal/world,
// "one directory = one world, never global").
package worlds

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// homeEnv overrides the promptworld home; it moves the worlds home and the
// registry together (research.md D4) so the override is honored
// consistently by discovery, creation, and name resolution.
const homeEnv = "PROMPTWORLD_HOME"

// Root returns the promptworld home: $PROMPTWORLD_HOME if set, else
// ~/.promptworld.
func Root() (string, error) {
	if v := os.Getenv(homeEnv); v != "" {
		return filepath.Abs(v)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve promptworld home: %w", err)
	}
	return filepath.Join(home, ".promptworld"), nil
}

// WorldsHome returns <root>/worlds — where name-created worlds live and
// where discovery scans for them.
func WorldsHome() (string, error) {
	root, err := Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "worlds"), nil
}

// RegistryPath returns <root>/known_worlds.json — the advisory pointer
// cache for worlds outside the worlds home.
func RegistryPath() (string, error) {
	root, err := Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "known_worlds.json"), nil
}

// ValidateName enforces FR-009 for the bare-name form of `new` and for
// `--name`: non-empty, no path separator, not flag-like (leading '-'), not
// hidden/path-like (leading '.'), usable as a directory name.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("world name must not be empty")
	}
	if strings.Contains(name, "/") {
		return fmt.Errorf("world name %q must not contain a path separator", name)
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("world name %q must not start with '-' (looks like a flag)", name)
	}
	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("world name %q must not start with '.'", name)
	}
	return nil
}

// InsideWorldsHome reports whether path resolves inside the current worlds
// home — registry entries there are scan-owned, never registry-owned (D6),
// and the daemon only registers worlds that live outside it (D1/FR-008).
func InsideWorldsHome(path string) (bool, error) {
	home, err := WorldsHome()
	if err != nil {
		return false, err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(home, absPath)
	if err != nil {
		return false, nil
	}
	if rel == "." {
		return true, nil
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..", nil
}
