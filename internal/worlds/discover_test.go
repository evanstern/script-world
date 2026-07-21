package worlds

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverEmptyHome(t *testing.T) {
	setHome(t)
	out, err := Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Errorf("expected no candidates, got %v", out)
	}
}

func TestDiscoverHomeScan(t *testing.T) {
	setHome(t)
	home, err := WorldsHome()
	if err != nil {
		t.Fatal(err)
	}
	makeWorld(t, filepath.Join(home, "aria"), "aria")
	makeWorld(t, filepath.Join(home, "harbor"), "harbor")
	// A plain subdirectory with no world.json must be skipped silently.
	if err := os.MkdirAll(filepath.Join(home, "not-a-world"), 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 candidates, got %d: %v", len(out), out)
	}
	if out[0].Name != "aria" || out[1].Name != "harbor" {
		t.Errorf("expected sorted [aria harbor], got [%s %s]", out[0].Name, out[1].Name)
	}
	for _, c := range out {
		if c.Source != SourceHome || !c.Readable || c.Missing {
			t.Errorf("candidate %+v not as expected", c)
		}
	}
}

func TestDiscoverUnreadableManifestFlaggedNotFatal(t *testing.T) {
	setHome(t)
	home, err := WorldsHome()
	if err != nil {
		t.Fatal(err)
	}
	corrupt := filepath.Join(home, "corrupt")
	if err := os.MkdirAll(corrupt, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(corrupt, "world.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(out))
	}
	if out[0].Readable {
		t.Error("expected corrupt manifest to be flagged unreadable")
	}
}

func TestDiscoverRegistryEntryMissingDir(t *testing.T) {
	setHome(t)
	dir := t.TempDir()
	sub := filepath.Join(dir, "gone")
	makeWorld(t, sub, "gone")
	if err := Upsert("gone", sub); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(sub); err != nil {
		t.Fatal(err)
	}

	out, err := Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 candidate, got %d: %v", len(out), out)
	}
	if !out[0].Missing {
		t.Error("expected registry entry with a deleted dir to be flagged Missing")
	}
}

func TestDiscoverHomeWinsOnNameCollision(t *testing.T) {
	setHome(t)
	home, err := WorldsHome()
	if err != nil {
		t.Fatal(err)
	}
	homeDir := filepath.Join(home, "aria")
	makeWorld(t, homeDir, "aria")

	elsewhere := t.TempDir()
	custom := filepath.Join(elsewhere, "aria-elsewhere")
	makeWorld(t, custom, "aria")
	if err := Upsert("aria", custom); err != nil {
		t.Fatal(err)
	}

	out, err := Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("expected deduped single candidate, got %d: %v", len(out), out)
	}
	if out[0].Source != SourceHome {
		t.Errorf("expected home to win the collision, got source %q", out[0].Source)
	}
}

func TestDiscoverRegistryWorldOutsideHome(t *testing.T) {
	setHome(t)
	dir := t.TempDir()
	sub := filepath.Join(dir, "harbor")
	makeWorld(t, sub, "harbor")
	if err := Upsert("harbor", sub); err != nil {
		t.Fatal(err)
	}

	out, err := Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Name != "harbor" || out[0].Source != SourceRegistry {
		t.Fatalf("unexpected discovery result: %v", out)
	}
}
