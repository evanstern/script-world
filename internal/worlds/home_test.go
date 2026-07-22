package worlds

import (
	"path/filepath"
	"testing"
)

func TestRootDefaultsToUserHome(t *testing.T) {
	t.Setenv("PROMPTWORLD_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	root, err := Root()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".promptworld")
	if root != want {
		t.Errorf("Root() = %q, want %q", root, want)
	}
}

func TestRootHonorsOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PROMPTWORLD_HOME", dir)
	root, err := Root()
	if err != nil {
		t.Fatal(err)
	}
	if root != dir {
		t.Errorf("Root() = %q, want %q", root, dir)
	}
}

func TestWorldsHomeAndRegistryPathDeriveFromRoot(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PROMPTWORLD_HOME", dir)

	wh, err := WorldsHome()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "worlds"); wh != want {
		t.Errorf("WorldsHome() = %q, want %q", wh, want)
	}

	rp, err := RegistryPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "known_worlds.json"); rp != want {
		t.Errorf("RegistryPath() = %q, want %q", rp, want)
	}
}

func TestValidateName(t *testing.T) {
	cases := []struct {
		name    string
		wantErr bool
	}{
		{"aria", false},
		{"harbor-2", false},
		{"", true},
		{"has/slash", true},
		{"-flag-like", true},
		{".hidden", true},
		{"..", true},
		{"~home", false}, // '~' is only special as a leading path marker on args, not a name char
	}
	for _, c := range cases {
		err := ValidateName(c.name)
		if (err != nil) != c.wantErr {
			t.Errorf("ValidateName(%q) error = %v, wantErr %v", c.name, err, c.wantErr)
		}
	}
}

func TestInsideWorldsHome(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PROMPTWORLD_HOME", root)
	home, err := WorldsHome()
	if err != nil {
		t.Fatal(err)
	}

	inside, err := InsideWorldsHome(filepath.Join(home, "aria"))
	if err != nil {
		t.Fatal(err)
	}
	if !inside {
		t.Error("expected a worlds-home subdir to be inside")
	}

	outside, err := InsideWorldsHome("/srv/games/harbor")
	if err != nil {
		t.Fatal(err)
	}
	if outside {
		t.Error("expected an unrelated path to be outside")
	}

	// A sibling directory that merely shares a prefix with the worlds home
	// name must not be misclassified as inside it.
	siblingLike := home + "-sibling"
	sibling, err := InsideWorldsHome(siblingLike)
	if err != nil {
		t.Fatal(err)
	}
	if sibling {
		t.Error("prefix-sharing sibling must not be classified as inside the worlds home")
	}
}
