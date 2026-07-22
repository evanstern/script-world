package main

import (
	"errors"
	"flag"
	"path/filepath"
	"testing"

	"github.com/evanstern/promptworld/internal/world"
	"github.com/evanstern/promptworld/internal/worlds"
)

// isolatedHome points PROMPTWORLD_HOME at a fresh temp dir for one test,
// mirroring e2e/manager_e2e_test.go's helper of the same purpose.
func isolatedHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("PROMPTWORLD_HOME", home)
	return home
}

// --- T011: cmdNew forms ---

func TestCmdNewNameFormCreatesUnderWorldsHome(t *testing.T) {
	isolatedHome(t)
	if err := cmdNew([]string{"aria", "--seed", "1"}); err != nil {
		t.Fatal(err)
	}
	home, err := worlds.WorldsHome()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "aria")
	w, err := world.Open(dir)
	if err != nil {
		t.Fatalf("expected a readable world at %s: %v", dir, err)
	}
	if w.Manifest.Name != "aria" {
		t.Errorf("manifest name = %q, want aria", w.Manifest.Name)
	}
}

func TestCmdNewNameFormRefusesDuplicateUntouched(t *testing.T) {
	isolatedHome(t)
	if err := cmdNew([]string{"aria", "--seed", "1"}); err != nil {
		t.Fatal(err)
	}
	home, err := worlds.WorldsHome()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "aria")
	before, err := world.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := cmdNew([]string{"aria", "--seed", "2"}); err == nil {
		t.Fatal("expected the duplicate `new aria` to be refused")
	}

	after, err := world.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if after.Manifest.Seed != before.Manifest.Seed {
		t.Errorf("existing world was touched: seed %d -> %d", before.Manifest.Seed, after.Manifest.Seed)
	}
}

func TestCmdNewNameFormRejectsNameFlag(t *testing.T) {
	isolatedHome(t)
	err := cmdNew([]string{"aria", "--name", "somethingelse", "--seed", "1"})
	if err == nil {
		t.Fatal("expected --name to be rejected in name-form")
	}
}

func TestCmdNewNameFormWithAtCreatesExactPathAndRegisters(t *testing.T) {
	isolatedHome(t)
	target := filepath.Join(t.TempDir(), "exact-spot")
	if err := cmdNew([]string{"custom", "--at", target, "--seed", "1"}); err != nil {
		t.Fatal(err)
	}
	w, err := world.Open(target)
	if err != nil {
		t.Fatalf("expected a world at the exact --at path: %v", err)
	}
	if w.Manifest.Name != "custom" {
		t.Errorf("manifest name = %q, want custom", w.Manifest.Name)
	}
	// --at is outside the worlds home, so it must be registry-addressable
	// by name afterward (D1/D6).
	resolved, err := worlds.Resolve("custom")
	if err != nil {
		t.Fatalf("expected `custom` to resolve via the registry: %v", err)
	}
	abs, _ := filepath.Abs(target)
	if resolved != abs {
		t.Errorf("resolved = %q, want %q", resolved, abs)
	}
}

func TestCmdNewPathFormUnchanged(t *testing.T) {
	isolatedHome(t)
	dir := filepath.Join(t.TempDir(), "w")
	if err := cmdNew([]string{dir, "--seed", "1"}); err != nil {
		t.Fatal(err)
	}
	w, err := world.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if w.Manifest.Name != "w" {
		t.Errorf("manifest name = %q, want basename \"w\"", w.Manifest.Name)
	}
	// A path-form world must NOT be registered — it isn't addressable by
	// name until a daemon boots for it (T007's job, not `new`'s).
	if _, err := worlds.Resolve("w"); err == nil {
		t.Error("expected a path-form world to not be registry-resolvable before any daemon boot")
	}
}

func TestCmdNewPathFormRejectsAt(t *testing.T) {
	isolatedHome(t)
	dir := filepath.Join(t.TempDir(), "w")
	err := cmdNew([]string{dir, "--at", "/tmp/should-not-be-used", "--seed", "1"})
	if err == nil {
		t.Fatal("expected --at to be rejected for a path-shaped argument")
	}
}

func TestCmdNewPathFormValidatesExplicitName(t *testing.T) {
	isolatedHome(t)
	dir := filepath.Join(t.TempDir(), "w")
	err := cmdNew([]string{dir, "--name", "-badname", "--seed", "1"})
	if err == nil {
		t.Fatal("expected an explicit flag-like --name to be rejected (contracts/cli.md D5)")
	}
}

func TestCmdNewPathFormDefaultBasenameStaysUnvalidated(t *testing.T) {
	// Backward compatibility (FR-012): the auto-derived basename was never
	// validated before this feature, and a dotted directory name (itself
	// only reachable via explicit path syntax, never as a bare name) must
	// keep working exactly as it did.
	isolatedHome(t)
	dir := filepath.Join(t.TempDir(), "my.world")
	if err := cmdNew([]string{dir, "--seed", "1"}); err != nil {
		t.Fatalf("expected the legacy dotted-basename default to still work: %v", err)
	}
}

// --- T012: name-or-path resolution plumbing ---

func TestResolveWorldPassesPathsThroughVerbatim(t *testing.T) {
	isolatedHome(t)
	for _, p := range []string{"./aria", "../aria", "~/aria", "/abs/aria", "rel/aria"} {
		got, err := resolveWorld(p)
		if err != nil {
			t.Fatalf("resolveWorld(%q) unexpected error: %v", p, err)
		}
		if got != p {
			t.Errorf("resolveWorld(%q) = %q, want verbatim passthrough", p, got)
		}
	}
}

func TestResolveWorldResolvesBareNameViaWorldsHome(t *testing.T) {
	isolatedHome(t)
	if err := cmdNew([]string{"aria", "--seed", "1"}); err != nil {
		t.Fatal(err)
	}
	dir, err := resolveWorld("aria")
	if err != nil {
		t.Fatal(err)
	}
	home, err := worlds.WorldsHome()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "aria"); dir != want {
		t.Errorf("resolveWorld(aria) = %q, want %q", dir, want)
	}
}

func TestResolveWorldUnknownNameErrors(t *testing.T) {
	isolatedHome(t)
	_, err := resolveWorld("never-created")
	if err == nil {
		t.Fatal("expected an error for an unresolvable bare name")
	}
	var nf *worlds.ErrNotFound
	if !errors.As(err, &nf) {
		t.Errorf("expected worlds.ErrNotFound, got %v (%T)", err, err)
	}
}

func TestWorldArgResolvesNameAtTheCallSite(t *testing.T) {
	isolatedHome(t)
	if err := cmdNew([]string{"harbor", "--seed", "1"}); err != nil {
		t.Fatal(err)
	}
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	dir, err := worldArg(fs, []string{"harbor"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := world.Open(dir); err != nil {
		t.Fatalf("worldArg did not resolve to a real world: %v", err)
	}
}

func TestParseWorldFlagsResolvesNameAtTheCallSite(t *testing.T) {
	isolatedHome(t)
	if err := cmdNew([]string{"harbor", "--seed", "1"}); err != nil {
		t.Fatal(err)
	}
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "")
	dir, err := parseWorldFlags(fs, []string{"harbor", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	if !*asJSON {
		t.Error("expected --json to still parse alongside a name argument")
	}
	if _, err := world.Open(dir); err != nil {
		t.Fatalf("parseWorldFlags did not resolve to a real world: %v", err)
	}
}

func TestWorldArgPathStillBypassesResolution(t *testing.T) {
	// A path to a directory that doesn't even exist yet must pass through
	// unresolved (today's exact behavior) rather than erroring inside
	// resolution — whatever downstream code (world.Open etc.) reports the
	// error, not worlds.Resolve.
	isolatedHome(t)
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	dir, err := worldArg(fs, []string{"./does-not-exist-anywhere"})
	if err != nil {
		t.Fatalf("path-shaped args must bypass resolution, got error: %v", err)
	}
	if dir != "./does-not-exist-anywhere" {
		t.Errorf("dir = %q, want verbatim path", dir)
	}
}

func TestCmdStatusAcceptsWorldByName(t *testing.T) {
	// End-to-end at the cmd-function layer (no subprocess): `status` on a
	// stopped, name-created world resolves and reports offline state.
	isolatedHome(t)
	if err := cmdNew([]string{"aria", "--seed", "1"}); err != nil {
		t.Fatal(err)
	}
	// cmdStatus prints to stdout; we only care that it resolves and
	// succeeds rather than erroring on "no world directory".
	if err := cmdStatus([]string{"aria"}); err != nil {
		t.Fatalf("cmdStatus by name failed: %v", err)
	}
}

func TestCmdStopIdempotentByName(t *testing.T) {
	isolatedHome(t)
	if err := cmdNew([]string{"aria", "--seed", "1"}); err != nil {
		t.Fatal(err)
	}
	if err := cmdStop([]string{"aria"}); err != nil {
		t.Fatalf("stop on a never-started name-created world must be idempotent, got: %v", err)
	}
}

func TestCmdStopUnknownNameFailsClearly(t *testing.T) {
	isolatedHome(t)
	err := cmdStop([]string{"nowhere"})
	if err == nil {
		t.Fatal("expected an error for an unknown world name")
	}
	var nf *worlds.ErrNotFound
	if !errors.As(err, &nf) {
		t.Errorf("expected worlds.ErrNotFound, got %v (%T)", err, err)
	}
}
