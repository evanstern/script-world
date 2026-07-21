package worlds

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/evanstern/script-world/internal/world"
)

// registryEntry / registryFile are the on-disk shape of known_worlds.json
// (data-model.md "Known-worlds record"): {"worlds": {"<name>": {"path": ...}}}.
type registryEntry struct {
	Path string `json:"path"`
}

type registryFile struct {
	Worlds map[string]registryEntry `json:"worlds"`
}

// Registry is the in-memory, load-tolerant view of the advisory
// known-worlds record: manifest name at last registration -> absolute path.
type Registry struct {
	Worlds map[string]string
}

// LoadRegistry reads the registry file. A missing or corrupt file yields an
// empty registry, never an error — the registry is advisory and must never
// block a read path (FR-008, D6).
func LoadRegistry() (*Registry, error) {
	path, err := RegistryPath()
	if err != nil {
		return nil, err
	}
	return loadRegistryFrom(path), nil
}

func loadRegistryFrom(path string) *Registry {
	reg := &Registry{Worlds: map[string]string{}}
	data, err := os.ReadFile(path)
	if err != nil {
		return reg // missing ⇒ empty, not an error
	}
	var rf registryFile
	if err := json.Unmarshal(data, &rf); err != nil {
		return reg // corrupt ⇒ empty, not an error
	}
	for name, e := range rf.Worlds {
		if e.Path != "" {
			reg.Worlds[name] = e.Path
		}
	}
	return reg
}

// Upsert registers (or repairs, by name) a name -> path pointer and writes
// the registry atomically (temp file + rename). Every write also prunes:
// entries whose path no longer contains a readable world.json, and entries
// that now fall inside the current worlds home (the scan owns those) —
// every read path tolerates lies, every write path heals them (D6).
func Upsert(name, path string) error {
	regPath, err := RegistryPath()
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	reg := loadRegistryFrom(regPath)
	reg.Worlds[name] = abs
	pruneRegistry(reg)
	return writeRegistry(regPath, reg)
}

// pruneRegistry drops entries that are no longer registry-owned or no
// longer resolve to a readable world. Best-effort: a failure to resolve the
// worlds home (e.g. $HOME unset) leaves entries as-is rather than pruning
// on incomplete information.
func pruneRegistry(reg *Registry) {
	for name, p := range reg.Worlds {
		if inside, err := InsideWorldsHome(p); err == nil && inside {
			delete(reg.Worlds, name)
			continue
		}
		if _, err := world.Open(p); err != nil {
			delete(reg.Worlds, name)
		}
	}
}

func writeRegistry(path string, reg *Registry) error {
	rf := registryFile{Worlds: map[string]registryEntry{}}
	for name, p := range reg.Worlds {
		rf.Worlds[name] = registryEntry{Path: p}
	}
	data, err := json.MarshalIndent(rf, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
