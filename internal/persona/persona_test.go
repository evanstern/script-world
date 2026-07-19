package persona

import (
	"os"
	"testing"

	"github.com/evanstern/script-world/internal/sim"
)

func TestGenesisWritesAllEightReadOnly(t *testing.T) {
	dir := t.TempDir()
	if err := Genesis(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range sim.AgentNames {
		info, err := os.Stat(PersonaPath(dir, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if perm := info.Mode().Perm(); perm != 0o444 {
			t.Errorf("%s persona mode = %o, want 0444 (AC#2)", name, perm)
		}
		data, err := os.ReadFile(PersonaPath(dir, name))
		if err != nil || len(data) == 0 {
			t.Errorf("%s persona unreadable/empty: %v", name, err)
		}
		if _, err := os.Stat(SoulPath(dir, name)); err != nil {
			t.Errorf("%s soul.md missing: %v", name, err)
		}
	}
}

func TestGenesisRunsOnce(t *testing.T) {
	dir := t.TempDir()
	if err := Genesis(dir); err != nil {
		t.Fatal(err)
	}
	if err := Genesis(dir); err == nil {
		t.Fatal("second genesis must refuse (personas are written exactly once)")
	}
}

func TestOSEnforcesReadOnly(t *testing.T) {
	dir := t.TempDir()
	if err := Genesis(dir); err != nil {
		t.Fatal(err)
	}
	// The filesystem itself refuses writes (belt to the structural braces).
	if err := os.WriteFile(PersonaPath(dir, "Ash"), []byte("tampered"), 0o644); err == nil {
		t.Fatal("writing a persona should fail at the OS level")
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	if err := Genesis(dir); err != nil {
		t.Fatal(err)
	}
	texts := Load(dir)
	for i, name := range sim.AgentNames {
		if texts[i] == "" {
			t.Errorf("persona %s loaded empty", name)
		}
	}
	// Worlds without personas degrade to empty strings, not errors.
	empty := Load(t.TempDir())
	if empty[0] != "" {
		t.Error("missing personas should load as empty strings")
	}
}

func TestEveryAgentHasAnAuthoredPersona(t *testing.T) {
	for _, name := range sim.AgentNames {
		if Texts[name] == "" {
			t.Errorf("no authored persona for %s", name)
		}
	}
}
