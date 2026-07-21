// Package daemon wires world + store + sim loop + ipc server into the
// always-on process, and owns lifecycle: recovery, pidfile, signals,
// graceful shutdown.
package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/evanstern/script-world/internal/clock"
	"github.com/evanstern/script-world/internal/cognition"
	"github.com/evanstern/script-world/internal/ipc"
	"github.com/evanstern/script-world/internal/llm"
	"github.com/evanstern/script-world/internal/metatron"
	"github.com/evanstern/script-world/internal/mind"
	"github.com/evanstern/script-world/internal/persona"
	"github.com/evanstern/script-world/internal/scribe"
	"github.com/evanstern/script-world/internal/sim"
	"github.com/evanstern/script-world/internal/store"
	"github.com/evanstern/script-world/internal/world"
)

// Run is the foreground daemon primitive: recover, bind, tick until
// SIGTERM/SIGINT or a shutdown command, then snapshot and exit cleanly.
func Run(dir string) error {
	startWall := time.Now()

	w, err := world.Open(dir)
	if err != nil {
		return err
	}
	if err := acquirePidfile(w); err != nil {
		return err
	}
	// Remove the pidfile ONLY if it is still ours: a slow shutdown can
	// overlap a successor daemon that has already claimed the file, and
	// deleting its pid would orphan it (live-found: stop then reported "not
	// running" while the successor still held the database).
	defer func() {
		if data, err := os.ReadFile(w.PidPath()); err == nil &&
			strings.TrimSpace(string(data)) == strconv.Itoa(os.Getpid()) {
			os.Remove(w.PidPath())
		}
	}()

	st, err := store.Open(w.DBPath())
	if err != nil {
		return err
	}
	defer st.Close()

	if err := validateMeta(w, st); err != nil {
		return err
	}
	if err := st.CheckContiguity(); err != nil {
		return err
	}

	state, err := recoverState(w, st)
	if err != nil {
		return err
	}
	if err := seedMeetingConvention(w, st, state); err != nil {
		return err
	}
	recoveryMs := time.Since(startWall).Milliseconds()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	srv := ipc.NewServer(w, st, cancel)

	// Notify fan-out: the IPC broadcast, the always-on soul scribe, and
	// (when an orchestrator exists) the mind driver. All consumers are
	// non-blocking by contract.
	var consumers []func([]store.Event)
	consumers = append(consumers, srv.Broadcast)
	scr, err := scribe.New(dir, w.Manifest.Seed, w.Map(), state.Marshal())
	if err != nil {
		return err
	}
	defer scr.Close()
	consumers = append(consumers, scr.Observe)
	notify := func(evs []store.Event) {
		for _, c := range consumers {
			c(evs)
		}
	}
	loop := sim.NewLoop(state, w.Map(), st, notify)
	srv.SetLoop(loop)

	// LLM orchestrator: optional (config-gated), fully outside the sim loop —
	// an unreachable model degrades the AI layer, never the world.
	if llmCfg, err := llm.LoadConfig(w.LLMConfigPath()); err != nil {
		return err
	} else if llmCfg != nil {
		// Cognition-horizon gate (FR-002): every call kind must resolve to
		// a registered decision class before a model is ever reachable.
		kinds := make([]string, 0, 8)
		for _, k := range llm.Kinds() {
			kinds = append(kinds, string(k))
		}
		if err := cognition.ValidateKinds(kinds); err != nil {
			return err
		}
		orch, err := llm.New(*llmCfg, st)
		if err != nil {
			return err
		}
		defer orch.Close()
		srv.SetLLM(orch)
		// Seed the seconds-per-point estimators from the calibration
		// profile before any traffic; a missing or unreadable file means
		// pessimistic bootstrap defaults (fail toward reflex, never toward
		// stale action).
		if prof, perr := cognition.LoadProfile(w.CalibrationPath()); perr != nil {
			fmt.Printf("daemon: %v — using bootstrap calibration defaults\n", perr)
		} else if prof != nil {
			orch.SeedCalibration(prof)
			fmt.Printf("daemon: calibration seeded (local %.1fs/pt, cloud %.1fs/pt, calibrated %s)\n",
				cognition.SeedFor(prof, "local"), cognition.SeedFor(prof, "cloud"), prof.CalibratedAt)
		} else {
			fmt.Printf("daemon: no calibration profile — bootstrap defaults (local %.0fs/pt, cloud %.0fs/pt); run `scriptworld calibrate`\n",
				cognition.BootstrapLocalSecPerPt, cognition.BootstrapCloudSecPerPt)
		}
		cloudDesc := llmCfg.Cloud.Model
		if llmCfg.Cloud.Provider == llm.ProviderOpenAICompat {
			cloudDesc = fmt.Sprintf("%s @ %s", llmCfg.Cloud.Model, llmCfg.Cloud.Endpoint)
		}
		// Local-tier concurrency (TASK-45): surface the effective worker count
		// when it exceeds the default, and warn (never fatal) when the operator
		// configured an out-of-range value that was clamped.
		localWorkers, workersWarn := llmCfg.Local.Workers()
		if workersWarn != "" {
			fmt.Printf("daemon: %s\n", workersWarn)
		}
		localDesc := fmt.Sprintf("local %s @ %s", llmCfg.Local.Model, llmCfg.Local.Endpoint)
		if localWorkers > 1 {
			localDesc += fmt.Sprintf(", parallel %d", localWorkers)
		}
		fmt.Printf("daemon: llm orchestrator on (%s, cloud %s, budget $%.0f/mo)\n",
			localDesc, cloudDesc, llmCfg.MonthlyBudgetUSD)
		md, err := mind.New(orch, loop, loop, w.Map(), w.Manifest.Seed, state.Marshal(), persona.Load(dir))
		if err != nil {
			return err
		}
		defer md.Close()
		consumers = append(consumers, md.Observe)
		// Drift signal: a tier's estimator breaching its spike-rate
		// threshold lands as cog.recalibration_recommended telemetry.
		orch.SetRecalibrateHook(md.RecalibrateSignal)
		fmt.Printf("daemon: mind driver on (%d villagers, cadence %d game-min)\n",
			sim.AgentCount, sim.PlannerCadenceTicks/60)
		mt, err := metatron.New(orch, loop, w.Map(), w.Manifest.Seed, state.Marshal(), dir)
		if err != nil {
			return err
		}
		defer mt.Close()
		consumers = append(consumers, mt.Observe)
		srv.SetMetatron(mt)
		fmt.Printf("daemon: metatron on (charges %d/%d)\n", state.MetatronCharges, sim.MetatronChargeCap)
	}

	// Stale socket from a crashed daemon: the pidfile said no one is alive.
	os.Remove(w.SockPath())
	if err := srv.Listen(); err != nil {
		return err
	}
	defer srv.Close()

	if err := appendDaemonEvent(st, srv, "daemon.started",
		sim.DaemonStartedPayload{Tick: state.Tick, RecoveryMs: recoveryMs}, state.Tick); err != nil {
		return err
	}
	fmt.Printf("daemon: world %q at tick %d (%s), recovery %dms, socket %s\n",
		w.Manifest.Name, state.Tick, clock.Format(state.Tick), recoveryMs, w.SockPath())

	go srv.Serve()

	runErr := loop.Run(ctx) // returns after final snapshot

	if err := appendDaemonEvent(st, srv, "daemon.stopped",
		sim.DaemonStoppedPayload{Tick: state.Tick}, state.Tick); err != nil && runErr == nil {
		runErr = err
	}
	fmt.Printf("daemon: stopped at tick %d\n", state.Tick)
	return runErr
}

// seedMeetingConvention injects the config-declared meeting convention on boot
// if the manifest declares one and none has taken hold yet (TASK-36) — once,
// at the recovered tick. It lands in the log like genesis, so replay
// re-applies it and this boot-time seed never fires twice (the reducer is
// one-shot, and the guard skips re-injection once state carries a convention).
func seedMeetingConvention(w *world.World, st *store.Store, state *sim.State) error {
	mc := w.Manifest.Meeting
	if mc == nil || state.MeetingConvention != nil {
		return nil
	}
	convene, open, err := mc.Seconds()
	if err != nil {
		return err // already validated in world.Open; defensive
	}
	ev := sim.NewConventionEvent(state, w.Map(), state.Tick, convene, open, mc.X, mc.Y)
	if err := state.Apply(ev); err != nil {
		return err
	}
	return st.AppendEvents([]store.Event{ev})
}

func appendDaemonEvent(st *store.Store, srv *ipc.Server, typ string, payload any, tick int64) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	events := []store.Event{{Tick: tick, Type: typ, Payload: b}}
	if err := st.AppendEvents(events); err != nil {
		return err
	}
	srv.Broadcast(events)
	return nil
}

// recoverState rebuilds world state from the newest valid snapshot plus
// event replay through the same reducer the live loop uses. The clock
// resumes at max(snapshot tick, last event tick); quiet trailing ticks
// re-run deterministically.
func recoverState(w *world.World, st *store.Store) (*sim.State, error) {
	state := sim.NewState(w.Manifest.Seed, w.Map())
	var since int64
	if snap, err := st.LatestValidSnapshot(); err != nil {
		return nil, err
	} else if snap != nil {
		if err := json.Unmarshal(snap.State, state); err != nil {
			return nil, fmt.Errorf("snapshot %d unreadable despite valid hash: %w", snap.ID, err)
		}
		since = snap.Seq
	}
	err := st.ReplayEvents(since, func(e store.Event) error {
		if err := state.Apply(e); err != nil {
			return err
		}
		if e.Tick > state.Tick {
			state.Tick = e.Tick
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("replay: %w", err)
	}
	return state, nil
}

func validateMeta(w *world.World, st *store.Store) error {
	// First daemon run stamps meta; later runs must match the manifest.
	for key, want := range map[string]string{
		"seed":           strconv.FormatUint(w.Manifest.Seed, 10),
		"format_version": strconv.Itoa(w.Manifest.FormatVersion),
	} {
		got, err := st.GetMeta(key)
		if err != nil {
			return err
		}
		if got == "" {
			if err := st.SetMeta(key, want); err != nil {
				return err
			}
			continue
		}
		if got != want {
			return fmt.Errorf("world.json and world.db disagree on %s (%s vs %s) — this save directory is corrupt or mixed from two runs", key, want, got)
		}
	}
	return nil
}

// acquirePidfile enforces one daemon per world dir, sweeping leftovers from
// crashed daemons (stale pid whose process is gone).
func acquirePidfile(w *world.World) error {
	if data, err := os.ReadFile(w.PidPath()); err == nil {
		if pid, perr := strconv.Atoi(strings.TrimSpace(string(data))); perr == nil && pidAlive(pid) {
			return fmt.Errorf("daemon already running (pid %d)", pid)
		}
		// Stale: crashed daemon left it behind. Sweep pid + socket.
		os.Remove(w.PidPath())
		os.Remove(w.SockPath())
	}
	return os.WriteFile(w.PidPath(), []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644)
}

func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// IsRunning reports whether a live daemon holds this world's pidfile.
func IsRunning(dir string) (bool, int) {
	w, err := world.Open(dir)
	if err != nil {
		return false, 0
	}
	data, err := os.ReadFile(w.PidPath())
	if err != nil {
		return false, 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || !pidAlive(pid) {
		return false, 0
	}
	return true, pid
}
