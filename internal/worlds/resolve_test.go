package worlds

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestIsPathArg(t *testing.T) {
	cases := []struct {
		arg  string
		want bool
	}{
		{"aria", false},
		{"harbor-2", false},
		{"./aria", true},
		{"../aria", true},
		{"~/aria", true},
		{"~aria", true},
		{"/abs/path", true},
		{"rel/path", true},
		{".", true},
		{"..", true},
	}
	for _, c := range cases {
		if got := IsPathArg(c.arg); got != c.want {
			t.Errorf("IsPathArg(%q) = %v, want %v", c.arg, got, c.want)
		}
	}
}

func TestResolveHomeOnly(t *testing.T) {
	setHome(t)
	home, err := WorldsHome()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "aria")
	makeWorld(t, dir, "aria")

	got, err := Resolve("aria")
	if err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(dir)
	if got != abs {
		t.Errorf("Resolve = %q, want %q", got, abs)
	}
}

func TestResolveRegistryOnly(t *testing.T) {
	setHome(t)
	dir := t.TempDir()
	makeWorld(t, dir, "harbor")
	if err := Upsert("harbor", dir); err != nil {
		t.Fatal(err)
	}

	got, err := Resolve("harbor")
	if err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(dir)
	if got != abs {
		t.Errorf("Resolve = %q, want %q", got, abs)
	}
}

func TestResolveNotFound(t *testing.T) {
	setHome(t)
	_, err := Resolve("nowhere")
	var nf *ErrNotFound
	if !errors.As(err, &nf) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if nf.Name != "nowhere" {
		t.Errorf("ErrNotFound.Name = %q", nf.Name)
	}
}

func TestResolveAmbiguous(t *testing.T) {
	setHome(t)
	home, err := WorldsHome()
	if err != nil {
		t.Fatal(err)
	}
	homeDir := filepath.Join(home, "aria")
	makeWorld(t, homeDir, "aria")

	customDir := t.TempDir()
	makeWorld(t, filepath.Join(customDir, "elsewhere"), "aria")
	if err := Upsert("aria", filepath.Join(customDir, "elsewhere")); err != nil {
		t.Fatal(err)
	}

	_, err = Resolve("aria")
	var amb *ErrAmbiguous
	if !errors.As(err, &amb) {
		t.Fatalf("expected ErrAmbiguous, got %v", err)
	}
	if len(amb.Paths) != 2 {
		t.Errorf("expected 2 candidate paths, got %v", amb.Paths)
	}
}

func TestResolveSamePathIsNotAmbiguous(t *testing.T) {
	// A registry entry that happens to point at the same worlds-home dir
	// (e.g. a stale self-entry) must not be flagged ambiguous.
	setHome(t)
	home, err := WorldsHome()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "aria")
	makeWorld(t, dir, "aria")
	// Force a registry entry pointing at the same home dir without going
	// through Upsert's home-pruning (Upsert would prune it away, which is
	// exactly the case Resolve must also tolerate if it ever appears).
	regPath, err := RegistryPath()
	if err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(dir)
	if err := writeRegistry(regPath, &Registry{Worlds: map[string]string{"aria": abs}}); err != nil {
		t.Fatal(err)
	}

	got, err := Resolve("aria")
	if err != nil {
		t.Fatalf("expected no error for same-path collision, got %v", err)
	}
	if got != abs {
		t.Errorf("Resolve = %q, want %q", got, abs)
	}
}
