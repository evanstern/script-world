package worlds

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/evanstern/script-world/internal/world"
)

func setHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("SCRIPTWORLD_HOME", dir)
	return dir
}

func makeWorld(t *testing.T, dir, name string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := world.Create(dir, name, 1); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoadRegistryMissingFileIsEmpty(t *testing.T) {
	setHome(t)
	reg, err := LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if len(reg.Worlds) != 0 {
		t.Errorf("expected empty registry, got %v", reg.Worlds)
	}
}

func TestLoadRegistryCorruptFileIsEmpty(t *testing.T) {
	root := setHome(t)
	regPath, err := RegistryPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(regPath, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, err := LoadRegistry()
	if err != nil {
		t.Fatalf("corrupt registry must not error, got %v", err)
	}
	if len(reg.Worlds) != 0 {
		t.Errorf("expected empty registry from corrupt file, got %v", reg.Worlds)
	}
}

func TestUpsertWritesAndReloads(t *testing.T) {
	setHome(t)
	wdir := t.TempDir()
	makeWorld(t, wdir, "harbor")

	if err := Upsert("harbor", wdir); err != nil {
		t.Fatal(err)
	}
	reg, err := LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(wdir)
	if got := reg.Worlds["harbor"]; got != abs {
		t.Errorf("registry path = %q, want %q", got, abs)
	}
}

func TestUpsertPrunesEntriesWithoutReadableWorld(t *testing.T) {
	setHome(t)
	wdir := t.TempDir()
	makeWorld(t, wdir, "harbor")
	if err := Upsert("harbor", wdir); err != nil {
		t.Fatal(err)
	}

	// A second world's directory vanishes before the next Upsert; that
	// Upsert must prune the stale entry rather than merely adding the new one.
	gone := t.TempDir()
	makeWorld(t, gone, "ghost")
	if err := Upsert("ghost", gone); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}

	other := t.TempDir()
	makeWorld(t, other, "other")
	if err := Upsert("other", other); err != nil {
		t.Fatal(err)
	}

	reg, err := LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Worlds["ghost"]; ok {
		t.Error("expected ghost entry to be pruned on write")
	}
	if _, ok := reg.Worlds["harbor"]; !ok {
		t.Error("expected harbor entry to survive")
	}
	if _, ok := reg.Worlds["other"]; !ok {
		t.Error("expected other entry to be written")
	}
}

func TestUpsertPrunesEntriesInsideWorldsHome(t *testing.T) {
	root := setHome(t)
	home, err := WorldsHome()
	if err != nil {
		t.Fatal(err)
	}
	insideDir := filepath.Join(home, "aria")
	makeWorld(t, insideDir, "aria")

	// Force a stale entry directly into the registry pointing inside the
	// worlds home, then trigger a write via an unrelated Upsert.
	regPath := filepath.Join(root, "known_worlds.json")
	reg := &Registry{Worlds: map[string]string{"aria": insideDir}}
	if err := writeRegistry(regPath, reg); err != nil {
		t.Fatal(err)
	}

	other := t.TempDir()
	makeWorld(t, other, "other")
	if err := Upsert("other", other); err != nil {
		t.Fatal(err)
	}

	got, err := LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Worlds["aria"]; ok {
		t.Error("expected worlds-home entry to be pruned from the registry")
	}
}

func TestUpsertRepairsMovedWorldByName(t *testing.T) {
	setHome(t)
	oldDir := t.TempDir()
	makeWorld(t, oldDir, "harbor")
	if err := Upsert("harbor", oldDir); err != nil {
		t.Fatal(err)
	}

	newDir := filepath.Join(t.TempDir(), "harbor-moved")
	if err := os.Rename(oldDir, newDir); err != nil {
		t.Fatal(err)
	}
	// Simulates the daemon boot re-registering by manifest name at the new
	// location (self-repair, D6).
	if err := Upsert("harbor", newDir); err != nil {
		t.Fatal(err)
	}

	reg, err := LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(newDir)
	if got := reg.Worlds["harbor"]; got != abs {
		t.Errorf("registry path after move = %q, want %q", got, abs)
	}
}
