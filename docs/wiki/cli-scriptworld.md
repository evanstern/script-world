---
name: cli-scriptworld
description: The single scriptworld binary — subcommand dispatch, world management, daemon control, observation commands, exit discipline
kind: component
sources:
  - cmd/scriptworld/main.go
  - cmd/scriptworld/commands.go
  - cmd/scriptworld/calibrate.go
verified_against: 004a430ca16d3f31d9d303b5b59b176bde0bae5f
---

# scriptworld CLI

One binary serves every role: daemon, client tools, world management. `main.go` is a
plain dispatch table; behavior lives in `commands.go`, except `calibrate` in its own
`calibrate.go`. The prose contract is `specs/001-world-daemon/contracts/cli.md`
(and `specs/007-cognition-horizon/contracts/cli.md` for `calibrate`).

## How it works

Exit discipline: 0 on success; 1 with a one-line `scriptworld <cmd>: error` on stderr;
2 for usage errors.

- `new <dir> [--name] [--seed]` — `world.Create` + store + genesis `world.created`
  event, writes the default `llm.json`, seeds the eight personas and Metatron's
  charter (`persona.Genesis`, the one-and-only persona write — [[agent-mind]],
  [[metatron]]), and
  appends the tick-0 secret events ([[social-fabric]]). Random default seed (crypto-random,
  right-shifted 12 bits to stay comfortably printable).
- `daemon <dir>` — the foreground primitive: `daemon.Run` directly.
- `start <dir>` — detached start: re-execs itself (`os.Executable()` + `daemon <dir>`)
  with stdio appended to `daemon.log` and `Setsid` to leave the session, then polls
  the socket up to 5 s for a status round trip before reporting success. Never waits
  on the child.
- `stop <dir>` — sends `shutdown` over the socket (falls back to SIGTERM if the
  socket is dead but the pid lives), waits ≤30 s for the pidfile to clear. Idempotent:
  "daemon not running" exits 0.
- `status <dir> [--json]` — online: full `StatusData` via the client. Offline:
  last-known state reconstructed read-only from the store (latest snapshot +
  `LastEventTick`), clearly labeled "daemon not running".
- `pause` / `resume` / `speed <v>` — one-shot time controls printing the resulting
  clock line.
- `ui <dir>` — the full-screen Bubble Tea client ([[tui-client]]): map, chronicle,
  metatron, souls panes over a live world replica; runs in the alternate screen.
  If the TUI quits on an unrecoverable protocol error (`Model.FatalErr()`, e.g. a
  reply over the IPC cap — TASK-19), the command returns it as a real error and
  exits non-zero.
- `attach <dir>` — line-mode: status header, live subscribe streamed to stdout,
  stdin commands (`pause`, `resume`, `speed <v>`, `status`, `quit`); handles
  `dropped` pushes by re-subscribing. Quit detaches; the world keeps running.
- `tail <dir> [--since SEQ] [--follow]` — history from the store (default last 20),
  works with no daemon; `--follow` additionally subscribes live and requires one.
- `metatron <dir> [message...]` — the console one-shot ([[metatron]], TASK-12): with
  a message, one mediated turn (prints surfaced moments, the reply, any landed
  `⚡ dream/omen` line, and the charge bank); without, a model-free status peek
  (charges, charter provenance, recent soul notes).
- `llm <dir> <kind> <prompt...> [--system] [--max-tokens]` — one-shot model call via
  the daemon's `llm_call` command, printing tier, model, tokens, cost, and latency
  ([[llm-orchestrator]]). `new` also writes the default `llm.json` config.
- `calibrate <dir> [--tier local|cloud|all] [--samples N]` — the cognition horizon's
  setup stage ([[cognition]], TASK-32): benchmarks the configured host+model per tier
  against fixed reference prompt shapes (default 5 samples per shape, local tier only;
  cloud spend is opt-in and announced up front), takes the median seconds-per-point,
  writes/merges `calibration.json` in the save directory, and prints the horizon the
  hardware buys (e.g. "planner suppressed above 16x") by evaluating the registry
  across the watchable speed ladder. Uses an in-memory meter so it never contends
  with a running daemon's store; a tier whose every sample fails is not written.

`parseDirFlags` accepts both `cmd <dir> --flag` and `cmd --flag <dir>` orderings.

## Connections

[[daemon-lifecycle]] is what `daemon`/`start` run; [[ipc-client]] carries every online
command; [[world-save-directory]] and [[event-log]] back the offline paths;
[[game-clock]] formats times in `clockLine`/`eventLine`; `calibrate` writes the
profile [[cognition]] routes with.

## Operational notes

`start` failure says "check daemon.log". Detached daemons survive terminal close
(Setsid); a machine reboot needs a manual `start` (launchd integration is future
work — the foreground `daemon` subcommand is what a plist would run).
