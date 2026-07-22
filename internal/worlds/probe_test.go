package worlds

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/evanstern/promptworld/internal/ipc"
)

// shortWorldDir returns a fresh world directory under /tmp (not t.TempDir(),
// which on darwin lands under a long $TMPDIR path that overflows the unix
// socket path length limit) — cleaned up on test end.
func shortWorldDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "sw-probe-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func writePidfile(t *testing.T, dir string, pid int) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "daemon.pid"), []byte(strconv.Itoa(pid)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeDaemon speaks just enough of the JSON-lines protocol to answer
// "status" for probe_test's classification cases; it is closed on cleanup.
func fakeDaemon(t *testing.T, sockPath string, sd *ipc.StatusData, delay time.Duration) net.Listener {
	t.Helper()
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				scanner := bufio.NewScanner(c)
				for scanner.Scan() {
					var req struct {
						ID int64 `json:"id"`
					}
					if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
						return
					}
					if delay > 0 {
						time.Sleep(delay)
					}
					data, _ := json.Marshal(sd)
					resp := map[string]any{"id": req.ID, "ok": true, "data": json.RawMessage(data)}
					b, _ := json.Marshal(resp)
					if _, err := c.Write(append(b, '\n')); err != nil {
						return
					}
				}
			}(conn)
		}
	}()
	return ln
}

func TestProbeOneRunning(t *testing.T) {
	setHome(t)
	dir := shortWorldDir(t)
	makeWorld(t, dir, "aria")
	writePidfile(t, dir, os.Getpid()) // our own pid is always "alive"
	sd := &ipc.StatusData{
		World: ipc.WorldStatus{Name: "aria", Seed: 1, FormatVersion: 1},
		Clock: ipc.ClockStatus{Tick: 42, GameTime: "d1 00:00:42", Speed: "4x"},
	}
	fakeDaemon(t, filepath.Join(dir, "daemon.sock"), sd, 0)

	inst := probeOne(Candidate{Name: "aria", Path: dir, Readable: true})
	if inst.State != Running {
		t.Fatalf("state = %s, want running", inst.State)
	}
	if inst.Status == nil || inst.Status.Clock.Tick != 42 {
		t.Fatalf("expected live status with tick 42, got %+v", inst.Status)
	}
	if inst.Pid != os.Getpid() {
		t.Errorf("pid = %d, want %d", inst.Pid, os.Getpid())
	}
	if inst.WorldName != "aria" {
		t.Errorf("world name = %q, want aria", inst.WorldName)
	}
}

func TestProbeOnePaused(t *testing.T) {
	setHome(t)
	dir := shortWorldDir(t)
	makeWorld(t, dir, "aria")
	writePidfile(t, dir, os.Getpid())
	sd := &ipc.StatusData{
		World: ipc.WorldStatus{Name: "aria"},
		Clock: ipc.ClockStatus{Tick: 7, Paused: true},
	}
	fakeDaemon(t, filepath.Join(dir, "daemon.sock"), sd, 0)

	inst := probeOne(Candidate{Name: "aria", Path: dir, Readable: true})
	if inst.State != Paused {
		t.Fatalf("state = %s, want paused", inst.State)
	}
}

func TestProbeOneUnresponsiveSlowDaemon(t *testing.T) {
	setHome(t)
	dir := shortWorldDir(t)
	makeWorld(t, dir, "aria")
	writePidfile(t, dir, os.Getpid())
	sd := &ipc.StatusData{World: ipc.WorldStatus{Name: "aria"}}
	fakeDaemon(t, filepath.Join(dir, "daemon.sock"), sd, probeBudget+500*time.Millisecond)

	start := time.Now()
	inst := probeOne(Candidate{Name: "aria", Path: dir, Readable: true})
	elapsed := time.Since(start)

	if inst.State != Unresponsive {
		t.Fatalf("state = %s, want unresponsive", inst.State)
	}
	if elapsed > probeBudget+time.Second {
		t.Errorf("probe took %v, expected to be bounded by probeBudget (%v)", elapsed, probeBudget)
	}
	// A wedged daemon must never be reported running (FR-002).
	if inst.Status != nil {
		t.Error("unresponsive instance must not carry a live status reply")
	}
}

func TestProbeOneUnresponsiveNoSocket(t *testing.T) {
	setHome(t)
	dir := shortWorldDir(t)
	makeWorld(t, dir, "aria")
	writePidfile(t, dir, os.Getpid()) // pid alive, but nothing listens on daemon.sock

	inst := probeOne(Candidate{Name: "aria", Path: dir, Readable: true})
	if inst.State != Unresponsive {
		t.Fatalf("state = %s, want unresponsive", inst.State)
	}
}

func TestProbeOneStoppedDeadPid(t *testing.T) {
	setHome(t)
	dir := shortWorldDir(t)
	makeWorld(t, dir, "aria")
	// A pid that cannot possibly be alive.
	writePidfile(t, dir, 999999)

	inst := probeOne(Candidate{Name: "aria", Path: dir, Readable: true})
	if inst.State != Stopped {
		t.Fatalf("state = %s, want stopped", inst.State)
	}
	if inst.WorldName != "aria" {
		t.Errorf("world name = %q, want aria", inst.WorldName)
	}
}

func TestProbeOneStoppedNoPidfile(t *testing.T) {
	setHome(t)
	dir := shortWorldDir(t)
	makeWorld(t, dir, "aria")

	inst := probeOne(Candidate{Name: "aria", Path: dir, Readable: true})
	if inst.State != Stopped {
		t.Fatalf("state = %s, want stopped", inst.State)
	}
}

func TestProbeOneUnreadable(t *testing.T) {
	setHome(t)
	dir := shortWorldDir(t)
	if err := os.WriteFile(filepath.Join(dir, "world.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	inst := probeOne(Candidate{Name: "corrupt", Path: dir})
	if inst.State != Unreadable {
		t.Fatalf("state = %s, want unreadable", inst.State)
	}
	if inst.Error == "" {
		t.Error("expected an error message for an unreadable world")
	}
}

func TestProbeOneMissing(t *testing.T) {
	inst := probeOne(Candidate{Name: "ghost", Path: "/nonexistent/ghost", Missing: true})
	if inst.State != Missing {
		t.Fatalf("state = %s, want missing", inst.State)
	}
}

func TestProbeMixedCandidatesNeverAborts(t *testing.T) {
	// T014: `ps --all` renders missing/unreadable rows alongside
	// running/stopped ones from a single Probe call — one broken candidate
	// must never abort the batch.
	setHome(t)

	runningDir := shortWorldDir(t)
	makeWorld(t, runningDir, "alive")
	writePidfile(t, runningDir, os.Getpid())
	fakeDaemon(t, filepath.Join(runningDir, "daemon.sock"), &ipc.StatusData{World: ipc.WorldStatus{Name: "alive"}}, 0)

	stoppedDir := shortWorldDir(t)
	makeWorld(t, stoppedDir, "sleeping")

	unreadableDir := shortWorldDir(t)
	if err := os.WriteFile(filepath.Join(unreadableDir, "world.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	candidates := []Candidate{
		{Name: "alive", Path: runningDir, Readable: true},
		{Name: "sleeping", Path: stoppedDir, Readable: true},
		{Name: "corrupt", Path: unreadableDir},
		{Name: "ghost", Path: "/nonexistent/ghost", Missing: true},
	}

	out := Probe(candidates)
	if len(out) != 4 {
		t.Fatalf("expected 4 instances (no candidate dropped), got %d: %+v", len(out), out)
	}
	want := map[string]State{"alive": Running, "sleeping": Stopped, "corrupt": Unreadable, "ghost": Missing}
	for _, inst := range out {
		if got, ok := want[inst.Name]; !ok || inst.State != got {
			t.Errorf("candidate %s: state = %s, want %s", inst.Name, inst.State, want[inst.Name])
		}
	}
}

func TestProbeRunsConcurrently(t *testing.T) {
	setHome(t)
	const n = 5
	var candidates []Candidate
	for i := 0; i < n; i++ {
		dir := shortWorldDir(t)
		name := "w" + strconv.Itoa(i)
		makeWorld(t, dir, name)
		writePidfile(t, dir, os.Getpid())
		fakeDaemon(t, filepath.Join(dir, "daemon.sock"), &ipc.StatusData{World: ipc.WorldStatus{Name: name}}, probeBudget-100*time.Millisecond)
		candidates = append(candidates, Candidate{Name: name, Path: dir, Readable: true})
	}

	start := time.Now()
	out := Probe(candidates)
	elapsed := time.Since(start)

	if len(out) != n {
		t.Fatalf("got %d instances, want %d", len(out), n)
	}
	// Sequential probing would take n * probeBudget; concurrent probing
	// must stay close to a single budget window (SC-001).
	if elapsed > 2*probeBudget {
		t.Errorf("Probe of %d candidates took %v — not running concurrently (budget %v)", n, elapsed, probeBudget)
	}
}
