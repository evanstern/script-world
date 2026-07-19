---
name: daemon-lifecycle
description: Process lifecycle — startup recovery (snapshot+replay), pidfile with stale sweep, manifest↔meta validation, signal-driven graceful shutdown
kind: pipeline
sources:
  - internal/daemon/daemon.go
verified_against: cee600e086a1be15868205c16c395ee33aaa397e
---

# Daemon lifecycle

`daemon.Run(dir)` is the foreground primitive that turns a save directory into a
living world: validate, recover, bind, tick, and — on any exit path — leave the
directory in a state the next start can resume from losslessly.

## How it works

Startup sequence:

1. `world.Open` — manifest validation ([[world-save-directory]]).
2. `acquirePidfile` — one daemon per world: an existing pidfile with a live process
   (checked via `kill(pid, 0)`, EPERM counts as alive) is a hard error; a stale one
   (crash leftover) is swept along with the stale socket.
3. `store.Open` + `validateMeta` — first run stamps `seed`/`format_version` into
   store meta; later runs must match the manifest exactly, catching save directories
   corrupted or spliced from two runs.
4. `CheckContiguity` — a holed event log refuses to run ([[event-log]]).
5. `recoverState` — newest hash-valid snapshot unmarshaled into
   `sim.NewState(seed, w.Map())` (genesis derives terrain-valid agent positions
   from [[worldmap-generation]]), then `ReplayEvents(seq > snapshot.seq)` through the
   reducer, bumping `Tick` to the highest event tick ([[snapshots]]). Recovery
   duration is measured and recorded.
6. LLM orchestrator ([[llm-orchestrator]]): started only when `llm.json` exists in
   the save dir (`llm.LoadConfig` → `llm.New` → `srv.SetLLM`), closed on exit —
   config-gated, fully outside the loop, so inference failures can never touch the
   simulation.
7. Wire-up: `ipc.NewServer(w, st, cancel)` where cancel is the
   `signal.NotifyContext(SIGTERM, SIGINT)` cancel — so the protocol `shutdown`
   command and Unix signals share one graceful path. `SetLoop` closes the
   loop↔server mutual reference. The stale socket is removed before `Listen`.
8. `daemon.started` event appended (payload carries tick and `recovery_ms`) and
   broadcast; then `srv.Serve()` in a goroutine and `loop.Run(ctx)` in the
   foreground.

Shutdown: ctx cancellation (signal or `shutdown` cmd) returns from `Run` after the
loop's final snapshot; `daemon.stopped` is appended; deferred cleanup closes the
server (removing the socket), the store, and the pidfile. SIGKILL skips all of this —
that is the crash path recovery is tested against.

`IsRunning(dir)` (used by CLI `start`/`stop`) reads the pidfile and probes liveness
without touching the world.

## Connections

[[cli-scriptworld]] runs this via `daemon` and detaches it via `start`; [[sim-loop]]
is the foreground engine; [[ipc-server]] the concurrent face; [[event-types]] defines
the `daemon.*` bookkeeping events it emits.

## Operational notes

Measured recovery: 18 ms after kill -9 across 95k events. A world killed while paused
wakes paused (pause state lives in snapshots/replay). Startup prints one line with
tick, game time, recovery ms, and socket path to stdout — in detached mode that lands
in `daemon.log`.
