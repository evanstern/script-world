// Package e2e drives the built scriptworld binary through the quickstart
// scenarios (specs/001-world-daemon/quickstart.md).
package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/evanstern/script-world/internal/store"
)

var bin string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "scriptworld-e2e")
	if err != nil {
		panic(err)
	}
	bin = filepath.Join(tmp, "scriptworld")
	build := exec.Command("go", "build", "-o", bin, "github.com/evanstern/script-world/cmd/scriptworld")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		panic(fmt.Sprintf("build: %v\n%s", err, out))
	}
	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

func run(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command(bin, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("scriptworld %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func runErr(args ...string) (string, error) {
	out, err := exec.Command(bin, args...).CombinedOutput()
	return string(out), err
}

type statusJSON struct {
	World struct {
		Name string `json:"name"`
		Seed uint64 `json:"seed"`
	} `json:"world"`
	Clock struct {
		Tick          int64   `json:"tick"`
		GameTime      string  `json:"game_time"`
		Paused        bool    `json:"paused"`
		Speed         string  `json:"speed"`
		EffectiveRate float64 `json:"effective_rate"`
	} `json:"clock"`
	Daemon struct {
		Pid     int   `json:"pid"`
		Running *bool `json:"running,omitempty"`
	} `json:"daemon"`
	Log struct {
		LastSeq int64 `json:"last_seq"`
	} `json:"log"`
}

func status(t *testing.T, dir string) statusJSON {
	t.Helper()
	out := run(t, "status", dir, "--json")
	var s statusJSON
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		t.Fatalf("status --json: %v\n%s", err, out)
	}
	return s
}

// newWorld creates and starts a world, and guarantees teardown.
func newWorld(t *testing.T, seed string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "w")
	run(t, "new", dir, "--seed", seed)
	run(t, "start", dir)
	t.Cleanup(func() { stopHard(dir) })
	return dir
}

func stopHard(dir string) {
	exec.Command(bin, "stop", dir).Run()
	if data, err := os.ReadFile(filepath.Join(dir, "daemon.pid")); err == nil {
		var pid int
		fmt.Sscanf(string(data), "%d", &pid)
		if pid > 0 {
			syscall.Kill(pid, syscall.SIGKILL)
		}
	}
}

func daemonPid(t *testing.T, dir string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "daemon.pid"))
	if err != nil {
		t.Fatalf("pidfile: %v", err)
	}
	var pid int
	fmt.Sscanf(string(data), "%d", &pid)
	if pid <= 0 {
		t.Fatalf("bad pidfile: %q", data)
	}
	return pid
}

// waitTick polls until the world's tick passes target (or fails after 15s).
func waitTick(t *testing.T, dir string, target int64) statusJSON {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		s := status(t, dir)
		if s.Clock.Tick >= target {
			return s
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("world never reached tick %d", target)
	return statusJSON{}
}

// --- Scenario A: the world runs without me (US1, SC-001/SC-002) ---

func TestScenarioA_AlwaysOnWorld(t *testing.T) {
	dir := newWorld(t, "42")

	s1 := status(t, dir)
	if s1.Clock.Speed != "4x" {
		t.Errorf("default speed = %s, want 4x", s1.Clock.Speed)
	}
	time.Sleep(2 * time.Second)
	s2 := status(t, dir)
	if s2.Clock.Tick <= s1.Clock.Tick {
		t.Fatalf("world not advancing: tick %d -> %d", s1.Clock.Tick, s2.Clock.Tick)
	}
	// At 4x, ~2s real ≈ 8 ticks (loose bounds against CI jitter).
	delta := s2.Clock.Tick - s1.Clock.Tick
	if delta < 4 || delta > 16 {
		t.Errorf("4x compression off: %d ticks in ~2s, expected ≈8", delta)
	}

	// A client observes events, then vanishes; the world must not care.
	run(t, "speed", dir, "max") // make events plentiful
	tail := exec.Command(bin, "tail", dir, "--follow")
	out, err := tail.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := tail.Start(); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4096)
	n, _ := out.Read(buf) // at least one event line arrives
	tail.Process.Kill()   // abrupt detach
	tail.Wait()
	if n == 0 {
		t.Fatal("tail --follow saw no events")
	}

	s3 := status(t, dir)
	time.Sleep(500 * time.Millisecond)
	s4 := status(t, dir)
	if s4.Clock.Tick <= s3.Clock.Tick {
		t.Fatal("detach paused the world (it must not)")
	}
	if s4.Clock.Paused {
		t.Fatal("world reports paused after client detach")
	}
}

// --- Scenario B: time is a dial (US2, SC-004/SC-005) ---

func TestScenarioB_PauseResumeFreezesClock(t *testing.T) {
	dir := newWorld(t, "7")

	out := run(t, "pause", dir)
	if !strings.Contains(out, "paused") {
		t.Errorf("pause output: %q", out)
	}
	s1 := status(t, dir)
	if !s1.Clock.Paused {
		t.Fatal("not paused")
	}
	time.Sleep(2 * time.Second)
	s2 := status(t, dir)
	if s2.Clock.Tick != s1.Clock.Tick {
		t.Fatalf("game time advanced while paused: %d -> %d (SC-004)", s1.Clock.Tick, s2.Clock.Tick)
	}

	run(t, "resume", dir)
	s3 := waitTick(t, dir, s2.Clock.Tick+1)
	if s3.Clock.Paused {
		t.Fatal("still paused after resume")
	}
	// Resume continues from the exact paused tick — nothing skipped.
	if s3.Clock.Tick < s2.Clock.Tick+1 {
		t.Fatalf("resume jumped: %d -> %d", s2.Clock.Tick, s3.Clock.Tick)
	}
}

func TestScenarioB_SpeedCompressionRatio(t *testing.T) {
	dir := newWorld(t, "7")

	measure := func(speed string, window time.Duration) float64 {
		run(t, "speed", dir, speed)
		time.Sleep(300 * time.Millisecond) // settle
		s1 := status(t, dir)
		start := time.Now()
		time.Sleep(window)
		s2 := status(t, dir)
		return float64(s2.Clock.Tick-s1.Clock.Tick) / time.Since(start).Seconds()
	}

	// SC-005 allows 5% over a 5-minute window; short e2e windows get 25%.
	r16 := measure("16x", 3*time.Second)
	if r16 < 12 || r16 > 20 {
		t.Errorf("16x achieved %.1f ticks/s, want ≈16", r16)
	}
	r1 := measure("1x", 3*time.Second)
	if r1 < 0.75 || r1 > 1.25 {
		t.Errorf("1x achieved %.2f ticks/s, want ≈1", r1)
	}

	if _, err := runErr("speed", dir, "9000x"); err == nil {
		t.Error("invalid speed accepted")
	}
}

// --- Scenario C: kill -9 and lossless resume (US3, SC-003) ---

func TestScenarioC_KillDashNineResume(t *testing.T) {
	dir := newWorld(t, "13")
	run(t, "speed", dir, "max")
	waitTick(t, dir, 30000) // accumulate meaningful history + snapshots
	beforeKill := status(t, dir)

	pid := daemonPid(t, dir)
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	waitPidGone(t, pid)

	// The recorded high-water mark at the moment of death: recovery must
	// resume at or past it, never behind it.
	recordedTick := lastLoggedTick(t, dir)

	restart := time.Now()
	run(t, "start", dir)
	recovery := time.Since(restart)
	if recovery > 10*time.Second {
		t.Errorf("recovery took %v, SC-003 requires < 10s", recovery)
	}

	after := status(t, dir)
	if after.Clock.Tick < recordedTick {
		t.Errorf("clock went backwards past recorded history: resumed at %d, recorded %d",
			after.Clock.Tick, recordedTick)
	}
	if beforeKill.Log.LastSeq == 0 {
		t.Fatal("no events recorded before kill")
	}
	if after.Clock.Speed != "max" {
		t.Errorf("speed not preserved across crash: %s", after.Clock.Speed)
	}
	assertLogContiguous(t, dir)
}

func TestScenarioC_RestartWhilePausedWakesPaused(t *testing.T) {
	dir := newWorld(t, "13")
	waitTick(t, dir, 4)
	run(t, "pause", dir)
	pausedAt := status(t, dir)

	pid := daemonPid(t, dir)
	syscall.Kill(pid, syscall.SIGKILL)
	waitPidGone(t, pid)

	run(t, "start", dir)
	after := status(t, dir)
	if !after.Clock.Paused {
		t.Fatal("world stopped while paused must wake paused")
	}
	if after.Clock.Tick != pausedAt.Clock.Tick {
		t.Errorf("paused tick moved across restart: %d -> %d", pausedAt.Clock.Tick, after.Clock.Tick)
	}
}

func TestScenarioC_GracefulStopAndRestart(t *testing.T) {
	dir := newWorld(t, "21")
	run(t, "speed", dir, "max")
	waitTick(t, dir, 5000)
	before := status(t, dir)

	out := run(t, "stop", dir)
	if !strings.Contains(out, "stopped") {
		t.Errorf("stop output: %q", out)
	}
	// Idempotent: stopping a stopped world succeeds.
	if out := run(t, "stop", dir); !strings.Contains(out, "not running") {
		t.Errorf("second stop output: %q", out)
	}

	// Offline status still answers from the store.
	off := status(t, dir)
	if off.Clock.Tick < before.Clock.Tick {
		t.Errorf("offline status lost time: %d < %d", off.Clock.Tick, before.Clock.Tick)
	}

	run(t, "start", dir)
	after := status(t, dir)
	if after.Clock.Tick < off.Clock.Tick {
		t.Errorf("restart lost time: %d < %d", after.Clock.Tick, off.Clock.Tick)
	}
	assertLogContiguous(t, dir)
}

// --- Scenario E: separable runs (FR-009) ---

func TestScenarioE_CopiedDirIsARunnableWorld(t *testing.T) {
	dir := newWorld(t, "5")
	run(t, "speed", dir, "max")
	waitTick(t, dir, 2000)
	run(t, "stop", dir)

	archive := filepath.Join(t.TempDir(), "archive")
	if out, err := exec.Command("cp", "-R", dir, archive).CombinedOutput(); err != nil {
		t.Fatalf("cp -R: %v\n%s", err, out)
	}
	t.Cleanup(func() { stopHard(archive) })

	frozen := status(t, archive)
	run(t, "start", archive)
	moved := waitTick(t, archive, frozen.Clock.Tick+1)
	if moved.World.Seed != 5 {
		t.Errorf("archive world seed = %d", moved.World.Seed)
	}
	assertLogContiguous(t, archive)
}

// --- helpers ---

func waitPidGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("pid %d still alive", pid)
}

func assertLogContiguous(t *testing.T, dir string) {
	t.Helper()
	st, err := store.Open(filepath.Join(dir, "world.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.CheckContiguity(); err != nil {
		t.Errorf("event log integrity after restart: %v", err)
	}
}

func lastLoggedTick(t *testing.T, dir string) int64 {
	t.Helper()
	st, err := store.Open(filepath.Join(dir, "world.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	tick, err := st.LastEventTick()
	if err != nil {
		t.Fatal(err)
	}
	return tick
}
