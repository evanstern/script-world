package world

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateOpenRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "w1")
	w, err := Create(dir, "testworld", 42)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if w.Manifest.Seed != 42 || w.Manifest.FormatVersion != FormatVersion {
		t.Errorf("manifest = %+v", w.Manifest)
	}
	if _, err := os.Stat(filepath.Join(dir, "agents")); err != nil {
		t.Errorf("agents/ dir missing: %v", err)
	}
	got, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got.Manifest != w.Manifest {
		t.Errorf("Open manifest %+v != created %+v", got.Manifest, w.Manifest)
	}
}

func TestCreateRefusesNonEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "junk"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(dir, "w", 1); err == nil {
		t.Fatal("Create on non-empty dir should fail")
	}
}

func TestOpenRejectsBadFormat(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ManifestName),
		[]byte(`{"name":"x","seed":1,"format_version":99,"tick_game_seconds":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("Open should reject unknown format_version")
	}
}
