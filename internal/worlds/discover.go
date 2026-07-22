package worlds

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/evanstern/promptworld/internal/world"
)

// Source distinguishes where a discovered candidate came from.
type Source string

const (
	SourceHome     Source = "home"
	SourceRegistry Source = "registry"
)

// Candidate is one discovered world: enough to classify and probe it
// (probe.go) without re-scanning.
type Candidate struct {
	Name   string
	Path   string
	Source Source
	// Readable is false when Path exists but world.json is missing/corrupt
	// (home scan) — never fatal to the whole listing.
	Readable bool
	// Missing is true for a registry entry whose path no longer exists.
	Missing bool
}

// Discover enumerates every candidate world machine-wide: an
// immediate-subdirectory scan of the worlds home, unioned with the advisory
// registry, deduped by name — the worlds home wins a name collision (same
// order as Resolve, D3/D1). A subdirectory that isn't a world at all
// (no world.json) is silently skipped, not flagged; one that has a
// world.json the daemon can't parse is flagged Readable=false rather than
// aborting the scan. Results are sorted by name for stable output.
func Discover() ([]Candidate, error) {
	home, err := WorldsHome()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []Candidate

	entries, err := os.ReadDir(home)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		path := filepath.Join(home, name)
		if _, err := os.Stat(filepath.Join(path, world.ManifestName)); err != nil {
			continue // not a world dir — not ours to report
		}
		_, openErr := world.Open(path)
		out = append(out, Candidate{Name: name, Path: path, Source: SourceHome, Readable: openErr == nil})
		seen[name] = true
	}

	reg, err := LoadRegistry()
	if err != nil {
		return nil, err
	}
	for name, path := range reg.Worlds {
		if seen[name] {
			continue // worlds-home wins on name collision
		}
		c := Candidate{Name: name, Path: path, Source: SourceRegistry}
		if _, err := os.Stat(path); os.IsNotExist(err) {
			c.Missing = true
		} else {
			_, openErr := world.Open(path)
			c.Readable = openErr == nil
		}
		out = append(out, c)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
