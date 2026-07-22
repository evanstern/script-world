// Manager e2e scenarios (specs/008-instance-manager, User Story 1): `ps`
// answers "what's running, from anywhere" with live-proven state, never
// stale records. Follows the pattern in daemon_e2e_test.go (build once in
// TestMain, drive the real binary as a subprocess).
package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// isolatedHome points PROMPTWORLD_HOME at a fresh temp dir for the
// duration of one test — ps/registry state never leaks across tests or
// from a developer's real ~/.promptworld (research.md D4). t.Setenv is
// visible to subprocesses started with a nil Env (they inherit
// os.Environ() at Start time), so it flows through `start`'s detached
// `daemon` grandchild too.
func isolatedHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("PROMPTWORLD_HOME", home)
	return home
}

// newNamedWorld creates and starts a world at a distinct custom path with a
// distinct manifest name (unlike daemon_e2e_test.go's newWorld, which
// always names worlds "w" — fine for single-world scenarios but a name
// collision here, since ps discovers by name). LLM traffic is disabled
// (matches every other e2e world) unless withLLM is true.
func newNamedWorld(t *testing.T, name, seed string, withLLM bool) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	run(t, "new", dir, "--name", name, "--seed", seed)
	if !withLLM {
		os.Remove(filepath.Join(dir, "llm.json"))
	}
	run(t, "start", dir)
	t.Cleanup(func() { stopHard(dir) })
	return dir
}

type psRow struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	State string `json:"state"`
	World struct {
		Name string `json:"name"`
	} `json:"world"`
	Clock struct {
		Tick     int64  `json:"tick"`
		GameTime string `json:"game_time"`
		Speed    string `json:"speed"`
	} `json:"clock"`
	Daemon struct {
		Pid int `json:"pid"`
	} `json:"daemon"`
	LLM           json.RawMessage `json:"llm"`
	LLMConfigured *bool           `json:"llm_configured"`
}

func psJSON(t *testing.T, extra ...string) []psRow {
	t.Helper()
	out := run(t, append([]string{"ps", "--json"}, extra...)...)
	var rows []psRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("ps --json: %v\n%s", err, out)
	}
	return rows
}

// runFromElsewhere chdirs the test process into an unrelated directory
// (restored on cleanup) so the built binary is invoked from a CWD that has
// nothing to do with any world under test — proving `ps` is not
// CWD-sensitive (FR-001).
func runFromElsewhere(t *testing.T) {
	t.Helper()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	elsewhere := t.TempDir()
	if err := os.Chdir(elsewhere); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(oldWd) })
}

// --- empty listing ---

func TestManagerPsEmptyHomeReportsNoWorldsRunning(t *testing.T) {
	isolatedHome(t)
	out := run(t, "ps")
	if !strings.Contains(out, "no worlds running") {
		t.Errorf("ps output = %q, want \"no worlds running\"", out)
	}
	rows := psJSON(t)
	if len(rows) != 0 {
		t.Errorf("ps --json = %v, want an empty array", rows)
	}
}

// --- two worlds, visible machine-wide, from any CWD ---

func TestManagerPsListsRunningWorldsFromAnyCWD(t *testing.T) {
	isolatedHome(t)
	dirA := newNamedWorld(t, "aria", "1", false)
	dirB := newNamedWorld(t, "harbor", "2", false)
	runFromElsewhere(t)

	rows := psJSON(t)
	if len(rows) != 2 {
		t.Fatalf("ps --json returned %d rows, want 2: %+v", len(rows), rows)
	}
	byName := map[string]psRow{}
	for _, r := range rows {
		byName[r.Name] = r
	}
	for _, want := range []struct {
		name, dir string
	}{{"aria", dirA}, {"harbor", dirB}} {
		r, ok := byName[want.name]
		if !ok {
			t.Fatalf("missing row for %q in %+v", want.name, rows)
		}
		if r.State != "running" {
			t.Errorf("%s: state = %q, want running", want.name, r.State)
		}
		if r.Daemon.Pid <= 0 {
			t.Errorf("%s: expected a live pid, got %d", want.name, r.Daemon.Pid)
		}
		if r.Clock.Speed == "" {
			t.Errorf("%s: expected a speed field", want.name)
		}
		if r.Path != want.dir {
			t.Errorf("%s: path = %q, want %q", want.name, r.Path, want.dir)
		}
		if len(r.LLM) != 0 {
			t.Errorf("%s: expected no LLM status (llm.json removed), got %s", want.name, r.LLM)
		}
	}

	// The human table carries the same information (name/state/pid/tick/
	// game time/speed/LLM columns, contracts/cli.md).
	out := run(t, "ps")
	for _, want := range []string{"NAME", "STATE", "PID", "TICK", "GAME TIME", "SPEED", "LLM", "aria", "harbor", "running", "off"} {
		if !strings.Contains(out, want) {
			t.Errorf("human ps table missing %q:\n%s", want, out)
		}
	}
}

// --- LLM visibility (FR-013/SC-005): a world with an llm.json shows on ---

func TestManagerPsShowsLLMEnabled(t *testing.T) {
	isolatedHome(t)
	newNamedWorld(t, "inferred", "3", true)

	rows := psJSON(t)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %+v", rows)
	}
	if len(rows[0].LLM) == 0 {
		t.Errorf("expected an LLM status object for a world with llm.json, got %+v", rows[0])
	}
	out := run(t, "ps")
	if !strings.Contains(out, "on") {
		t.Errorf("human table missing an 'on' LLM column:\n%s", out)
	}
}

// --- SIGKILL leaves no false "running" (FR-002) ---

func TestManagerPsStaleAfterSIGKILL(t *testing.T) {
	isolatedHome(t)
	dir := newNamedWorld(t, "flicker", "4", false)
	pid := daemonPid(t, dir)

	before := psJSON(t)
	if len(before) != 1 || before[0].State != "running" {
		t.Fatalf("expected one running world before kill, got %+v", before)
	}

	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	waitPidGone(t, pid)

	after := psJSON(t)
	if len(after) != 0 {
		t.Fatalf("expected no running worlds after SIGKILL (stale pidfile/socket must not read as running), got %+v", after)
	}

	all := psJSON(t, "--all")
	found := false
	for _, r := range all {
		if r.Name == "flicker" {
			found = true
			if r.State != "stopped" {
				t.Errorf("flicker state under --all = %q, want stopped", r.State)
			}
		}
	}
	if !found {
		t.Errorf("expected flicker to still be listed (stopped) under --all, got %+v", all)
	}

	// Idempotent from the CLI side too: a bare `ps` after the kill still
	// exits 0 with "no worlds running".
	out := run(t, "ps")
	if !strings.Contains(out, "no worlds running") {
		t.Errorf("ps after SIGKILL = %q", out)
	}
}

// --- one wedged daemon cannot stall the whole listing (D2, SC-001) ---

func TestManagerPsWedgedDaemonRespectsBudget(t *testing.T) {
	isolatedHome(t)
	dir := newNamedWorld(t, "wedged", "5", false)
	pid := daemonPid(t, dir)

	// SIGSTOP freezes the process without killing it: the pidfile still
	// reads alive (kill(pid,0) succeeds) and the listening socket still
	// exists, but nothing ever answers a "status" call — exactly
	// "unresponsive" (data-model.md state machine), not "running".
	if err := syscall.Kill(pid, syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	// SIGKILL terminates a stopped process immediately, so cleanup does not
	// need to SIGCONT first.
	t.Cleanup(func() { syscall.Kill(pid, syscall.SIGKILL) })

	start := time.Now()
	rows := psJSON(t)
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Errorf("ps against one wedged daemon took %v, want < 2s (SC-001)", elapsed)
	}
	if len(rows) != 1 || rows[0].State != "unresponsive" {
		t.Fatalf("expected one unresponsive row, got %+v", rows)
	}
}

// --- User Story 2 (T013): full name lifecycle, zero paths typed ---

func TestManagerNewByNameFullLifecycle(t *testing.T) {
	home := isolatedHome(t)
	runFromElsewhere(t)

	out := run(t, "new", "aria", "--seed", "1")
	if !strings.Contains(out, `created world "aria"`) {
		t.Fatalf("new aria output = %q", out)
	}
	if !strings.Contains(out, "promptworld start aria") {
		t.Errorf("expected the name-form success hint to suggest `start aria`, got %q", out)
	}
	dir := filepath.Join(home, "worlds", "aria")
	manifest, err := os.ReadFile(filepath.Join(dir, "world.json"))
	if err != nil {
		t.Fatalf("expected a manifest under the worlds home: %v", err)
	}

	// Acceptance Scenario 2: duplicate name refused, existing world untouched.
	if out, err := runErr("new", "aria", "--seed", "2"); err == nil {
		t.Fatalf("expected duplicate `new aria` to be refused, got: %s", out)
	}
	after, err := os.ReadFile(filepath.Join(dir, "world.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(manifest) != string(after) {
		t.Error("existing world was touched by the refused duplicate `new`")
	}

	// The rest of the lifecycle by name only — never a path (SC-002).
	os.Remove(filepath.Join(dir, "llm.json"))
	run(t, "start", "aria")
	t.Cleanup(func() { stopHard(dir) })

	if out := run(t, "status", "aria"); !strings.Contains(out, "running") {
		t.Errorf("status aria = %q", out)
	}
	run(t, "speed", "aria", "8x")
	run(t, "pause", "aria")
	var s statusJSON
	if err := json.Unmarshal([]byte(run(t, "status", "aria", "--json")), &s); err != nil {
		t.Fatal(err)
	}
	if !s.Clock.Paused {
		t.Error("expected paused after `pause aria`")
	}
	run(t, "resume", "aria")

	if out := run(t, "stop", "aria"); !strings.Contains(out, "stopped") {
		t.Errorf("stop aria = %q", out)
	}
	// Idempotent by name exactly as by path (FR-010).
	if out := run(t, "stop", "aria"); !strings.Contains(out, "not running") {
		t.Errorf("second stop aria = %q", out)
	}
}

// TestManagerNewInvalidNames covers the spec's name-rule edge cases
// (FR-009). "bad/name" contains a path separator, so per the name-vs-path
// rule (research.md D3) it is a PATH, not a name — it is legacy path-form
// and legitimately succeeds at a relative path, not a name-validation
// failure; name validation for a "/"-bearing value is only reachable
// through --name (also exercised here), the one place a name-shaped string
// can arrive without first being classified as a path.
func TestManagerNewInvalidNames(t *testing.T) {
	isolatedHome(t)
	runFromElsewhere(t)

	if out, err := runErr("new", "-flag"); err == nil {
		t.Errorf("expected `new -flag` to be rejected, got: %s", out)
	}
	if out, err := runErr("new", ""); err == nil {
		t.Errorf("expected `new \"\"` to be rejected, got: %s", out)
	}
	if out, err := runErr("new", "bad/name", "--seed", "1"); err != nil {
		t.Errorf("expected `new bad/name` to succeed as legacy path-form (D3), got error: %v\n%s", err, out)
	} else if _, statErr := os.Stat(filepath.Join("bad", "name", "world.json")); statErr != nil {
		t.Errorf("expected a world created at relative path bad/name: %v", statErr)
	}

	dir := filepath.Join(t.TempDir(), "target")
	if out, err := runErr("new", dir, "--name", "bad/name"); err == nil {
		t.Errorf("expected --name with a path separator to be rejected, got: %s", out)
	}
	if out, err := runErr("new", dir, "--name", "-badname"); err == nil {
		t.Errorf("expected a flag-like --name to be rejected, got: %s", out)
	}
}

func TestManagerUnknownNameFailsWithHelpfulMessage(t *testing.T) {
	home := isolatedHome(t)
	runFromElsewhere(t)

	out, err := runErr("status", "nowhere")
	if err == nil {
		t.Fatalf("expected `status nowhere` to fail, got: %s", out)
	}
	worldsHome := filepath.Join(home, "worlds")
	if !strings.Contains(out, worldsHome) {
		t.Errorf("expected the error to name the worlds home searched (%s), got %q", worldsHome, out)
	}
	if !strings.Contains(out, "ps --all") {
		t.Errorf("expected the error to suggest `ps --all`, got %q", out)
	}
}

// TestManagerCopiedNameWorldRunsUnderFreshHome is SC-004: a stopped
// name-created world, copied to an arbitrary path under a completely
// unrelated PROMPTWORLD_HOME (no worlds-home entry, no registry entry —
// nothing), starts and runs by path with zero manager state present.
func TestManagerCopiedNameWorldRunsUnderFreshHome(t *testing.T) {
	home := isolatedHome(t)
	run(t, "new", "aria", "--seed", "42")
	dir := filepath.Join(home, "worlds", "aria")
	os.Remove(filepath.Join(dir, "llm.json"))
	run(t, "start", "aria")
	waitTick(t, dir, 1)
	run(t, "stop", "aria")

	archive := filepath.Join(t.TempDir(), "aria-archive")
	if out, err := exec.Command("cp", "-R", dir, archive).CombinedOutput(); err != nil {
		t.Fatalf("cp -R: %v\n%s", err, out)
	}

	isolatedHome(t) // a second, unrelated home — the copy must not need it
	run(t, "start", archive)
	t.Cleanup(func() { stopHard(archive) })
	s := waitTick(t, archive, 1)
	if s.World.Seed != 42 {
		t.Errorf("archive seed = %d, want 42", s.World.Seed)
	}
}

// --- User Story 3 (T015): custom-path worlds, addressable by name ---

func TestManagerCustomPathWorldStoppedByName(t *testing.T) {
	isolatedHome(t)
	dir := newNamedWorld(t, "harbor-drift", "6", false)

	rows := psJSON(t)
	if len(rows) != 1 || rows[0].Name != "harbor-drift" || rows[0].State != "running" {
		t.Fatalf("expected harbor-drift running, got %+v", rows)
	}

	out := run(t, "stop", "harbor-drift")
	if !strings.Contains(out, "stopped") {
		t.Errorf("stop by name output = %q", out)
	}

	all := psJSON(t, "--all")
	found := false
	for _, r := range all {
		if r.Name == "harbor-drift" {
			found = true
			if r.State != "stopped" {
				t.Errorf("state under --all = %q, want stopped", r.State)
			}
			if r.Path != dir {
				t.Errorf("path = %q, want %q", r.Path, dir)
			}
		}
	}
	if !found {
		t.Error("expected harbor-drift to survive to ps --all after stopping")
	}
}

// TestManagerDeletedCustomPathWorldGracefulByName: once a custom-path
// world's directory is gone, the manager forgets it gracefully — ps --all
// reports it missing (never aborts) and addressing it by name fails with a
// specific, helpful exit-1 message (never a raw open error, T014).
func TestManagerDeletedCustomPathWorldGracefulByName(t *testing.T) {
	isolatedHome(t)
	dir := newNamedWorld(t, "driftaway", "7", false)
	run(t, "stop", "driftaway")

	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}

	for _, r := range psJSON(t, "--all") {
		if r.Name == "driftaway" && r.State != "missing" {
			t.Errorf("expected driftaway state = missing after its dir vanished, got %q", r.State)
		}
	}

	out, err := runErr("status", "driftaway")
	if err == nil {
		t.Fatalf("expected `status driftaway` to fail after its dir vanished, got: %s", out)
	}
	if !strings.Contains(out, "driftaway") || !strings.Contains(out, "gone") {
		t.Errorf("expected a specific \"gone\" message, not a raw error: %q", out)
	}
}

// TestManagerNameCollisionAmbiguousRefusal: a worlds-home world and a
// registry-remembered custom-path world sharing a name must refuse rather
// than silently pick one (FR-011).
func TestManagerNameCollisionAmbiguousRefusal(t *testing.T) {
	home := isolatedHome(t)
	run(t, "new", "aria", "--seed", "1")
	homeAria := filepath.Join(home, "worlds", "aria")

	elsewhere := t.TempDir()
	customDir := filepath.Join(elsewhere, "aria-elsewhere")
	run(t, "new", customDir, "--name", "aria", "--seed", "2")
	os.Remove(filepath.Join(customDir, "llm.json"))
	run(t, "start", customDir)
	t.Cleanup(func() { stopHard(customDir) })

	out, err := runErr("status", "aria")
	if err == nil {
		t.Fatalf("expected addressing the colliding name to be refused as ambiguous, got: %s", out)
	}
	if !strings.Contains(out, "ambiguous") {
		t.Errorf("expected an ambiguity message, got %q", out)
	}
	if !strings.Contains(out, homeAria) || !strings.Contains(out, customDir) {
		t.Errorf("expected both candidate paths listed, got %q", out)
	}
}
