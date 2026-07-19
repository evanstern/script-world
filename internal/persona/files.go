package persona

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/evanstern/script-world/internal/sim"
)

// Dir returns the agent's directory under the world's agents/ root.
func Dir(worldDir, name string) string {
	return filepath.Join(worldDir, "agents", strings.ToLower(name))
}

func PersonaPath(worldDir, name string) string {
	return filepath.Join(Dir(worldDir, name), "persona.md")
}
func SoulPath(worldDir, name string) string { return filepath.Join(Dir(worldDir, name), "soul.md") }

// Genesis writes each agent's persona.md (read-only) and an empty soul.md.
// Called exactly once, by `scriptworld new` — the only write path to
// persona.md in the entire system.
func Genesis(worldDir string) error {
	for _, name := range sim.AgentNames {
		text, ok := Texts[name]
		if !ok {
			return fmt.Errorf("no authored persona for %q", name)
		}
		dir := Dir(worldDir, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		pPath := PersonaPath(worldDir, name)
		if _, err := os.Stat(pPath); err == nil {
			return fmt.Errorf("persona already exists at %s — genesis runs once", pPath)
		}
		if err := os.WriteFile(pPath, []byte(text), 0o444); err != nil {
			return err
		}
		soul := fmt.Sprintf("# %s — soul\n\n*Born day 1. No memories yet.*\n", name)
		if err := os.WriteFile(SoulPath(worldDir, name), []byte(soul), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Load reads all personas for prompt building (index-aligned with
// sim.AgentNames). Missing files (pre-TASK-7 worlds) yield empty strings —
// the mind degrades to a nameless prompt rather than failing.
func Load(worldDir string) [sim.AgentCount]string {
	var out [sim.AgentCount]string
	for i, name := range sim.AgentNames {
		if data, err := os.ReadFile(PersonaPath(worldDir, name)); err == nil {
			out[i] = string(data)
		}
	}
	return out
}
