package worlds

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/evanstern/promptworld/internal/clock"
	"github.com/evanstern/promptworld/internal/ipc"
	"github.com/evanstern/promptworld/internal/sim"
	"github.com/evanstern/promptworld/internal/store"
	"github.com/evanstern/promptworld/internal/world"
)

// State classifies a candidate world at query time — always re-proven from
// live evidence, never from records (FR-002; data-model.md state machine).
type State string

const (
	Running      State = "running"
	Paused       State = "paused"
	Unresponsive State = "unresponsive"
	Stopped      State = "stopped"
	Missing      State = "missing"
	Unreadable   State = "unreadable"
)

// probeBudget bounds one candidate's dial+status round trip so a wedged
// daemon cannot stall the whole listing (D2, SC-001: < 2s machine-wide).
const probeBudget = 1 * time.Second

// Instance is one probed world: state plus everything ps needs to render a
// row, live or last-known.
type Instance struct {
	Name  string
	Path  string
	State State
	Error string // set for Unreadable

	WorldName     string
	Seed          uint64
	FormatVersion int

	Pid int // live pid; set whenever the pidfile pre-filter finds one alive

	// Status is the live "status" reply — non-nil only for Running/Paused.
	Status *ipc.StatusData

	// Offline fields — populated for Stopped (last-known, read from the
	// store, never from a live daemon).
	Tick          int64
	GameTime      string
	OfflinePaused bool
	OfflineSpeed  string
	LastSeq       int64
	LLMConfigured bool
}

// Probe classifies every candidate concurrently, each bounded by
// probeBudget, and returns one Instance per candidate in the same order
// (D2: parallel so one wedged daemon cannot stall the listing).
func Probe(candidates []Candidate) []Instance {
	out := make([]Instance, len(candidates))
	var wg sync.WaitGroup
	for i, c := range candidates {
		wg.Add(1)
		go func(i int, c Candidate) {
			defer wg.Done()
			out[i] = probeOne(c)
		}(i, c)
	}
	wg.Wait()
	return out
}

// probeOne follows the data-model.md state machine: the pidfile pre-filter
// runs first regardless of manifest readability (the pidfile's path is a
// fixed filename inside the dir, independent of the manifest), then a
// bounded status round trip; only a dead/absent pid falls back to the
// manifest to distinguish stopped/unreadable/missing.
func probeOne(c Candidate) Instance {
	inst := Instance{Name: c.Name, Path: c.Path}
	if c.Missing {
		inst.State = Missing
		return inst
	}

	if running, pid := isRunning(filepath.Join(c.Path, "daemon.pid")); running {
		inst.Pid = pid
		sockPath := filepath.Join(c.Path, "daemon.sock")
		if sd, ok := statusWithBudget(sockPath, probeBudget); ok {
			inst.Status = sd
			inst.WorldName, inst.Seed, inst.FormatVersion = sd.World.Name, sd.World.Seed, sd.World.FormatVersion
			if sd.Clock.Paused {
				inst.State = Paused
			} else {
				inst.State = Running
			}
			return inst
		}
		inst.State = Unresponsive
		if w, err := world.Open(c.Path); err == nil {
			inst.WorldName, inst.Seed, inst.FormatVersion = w.Manifest.Name, w.Manifest.Seed, w.Manifest.FormatVersion
		}
		return inst
	}

	w, err := world.Open(c.Path)
	if err != nil {
		if _, statErr := os.Stat(c.Path); statErr != nil {
			inst.State = Missing
			return inst
		}
		inst.State = Unreadable
		inst.Error = err.Error()
		return inst
	}
	inst.WorldName, inst.Seed, inst.FormatVersion = w.Manifest.Name, w.Manifest.Seed, w.Manifest.FormatVersion
	inst.State = Stopped
	fillOfflineSnapshot(&inst, w)
	return inst
}

// isRunning is a local pidfile-liveness pre-filter, mirroring
// internal/daemon's acquirePidfile/IsRunning check exactly. It is
// duplicated rather than imported: internal/daemon registers worlds into
// this package on boot (T007), and internal/worlds classifies liveness for
// ps — importing internal/daemon here would close that cycle.
func isRunning(pidPath string) (bool, int) {
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return false, 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || !pidAlive(pid) {
		return false, 0
	}
	return true, pid
}

func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// statusWithBudget dials and calls "status" with its own deadline, wrapping
// ipc.Dial in a goroutine rather than adding protocol surface (D2).
func statusWithBudget(sockPath string, budget time.Duration) (*ipc.StatusData, bool) {
	type result struct {
		sd  *ipc.StatusData
		err error
	}
	ch := make(chan result, 1)
	go func() {
		c, err := ipc.Dial(sockPath)
		if err != nil {
			ch <- result{nil, err}
			return
		}
		defer c.Close()
		sd, err := c.Status("status", nil)
		ch <- result{sd, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			return nil, false
		}
		return r.sd, true
	case <-time.After(budget):
		return nil, false
	}
}

// OfflineSnapshot reads a world's last-known clock straight from its store,
// without a live daemon — extracted from cmdStatus's offline branch
// (cmd/promptworld/commands.go) so both `status` and `ps --all` share the
// one read instead of duplicating it (research.md D7).
func OfflineSnapshot(w *world.World) (tick int64, paused bool, speed string, lastSeq int64, err error) {
	st, err := store.Open(w.DBPath())
	if err != nil {
		return 0, false, "", 0, err
	}
	defer st.Close()
	state := sim.NewState(w.Manifest.Seed, w.Map())
	if snap, serr := st.LatestValidSnapshot(); serr == nil && snap != nil {
		json.Unmarshal(snap.State, state)
	}
	if lastTick, terr := st.LastEventTick(); terr == nil && lastTick > state.Tick {
		state.Tick = lastTick
	}
	return state.Tick, state.Paused, string(state.Speed), st.LastSeq(), nil
}

// fillOfflineSnapshot populates a Stopped Instance's last-known fields plus
// whether an LLM config is present (llm.json existence — the stopped-world
// analogue of StatusData.LLM != nil).
func fillOfflineSnapshot(inst *Instance, w *world.World) {
	tick, paused, speed, lastSeq, err := OfflineSnapshot(w)
	if err != nil {
		return // best effort — leave zero values, still reported Stopped
	}
	inst.Tick = tick
	inst.GameTime = clock.Format(tick)
	inst.OfflinePaused = paused
	inst.OfflineSpeed = speed
	inst.LastSeq = lastSeq
	if _, statErr := os.Stat(w.LLMConfigPath()); statErr == nil {
		inst.LLMConfigured = true
	}
}
