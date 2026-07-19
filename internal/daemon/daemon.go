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
	"github.com/evanstern/script-world/internal/ipc"
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
	defer os.Remove(w.PidPath())

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
	recoveryMs := time.Since(startWall).Milliseconds()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	srv := ipc.NewServer(w, st, cancel)
	loop := sim.NewLoop(state, w.Map(), st, srv.Broadcast)
	srv.SetLoop(loop)

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
