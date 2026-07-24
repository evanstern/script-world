---
name: cli-promptworld
description: The single promptworld binary — subcommand dispatch, world management, daemon control, observation commands, v1→v2→v3 migration, exit discipline
kind: component
sources:
  - cmd/promptworld/main.go
  - cmd/promptworld/commands.go
  - cmd/promptworld/calibrate.go
  - cmd/promptworld/ps.go
  - cmd/promptworld/miracle.go
verified_against: be38288fa137064174eedbfb3b8a94cc5b1fb0b9
---

# promptworld CLI

One binary serves every role: daemon, client tools, world management. `main.go` is a
plain dispatch table; behavior lives in `commands.go`, except `calibrate` in its own
`calibrate.go` and `ps` in `ps.go` ([[instance-manager]]). The prose contract is
`specs/001-world-daemon/contracts/cli.md` (extended by
`specs/008-instance-manager/contracts/cli.md` for names/`ps`/`new`, and
`specs/007-cognition-horizon/contracts/cli.md` for `calibrate`).

## How it works

Exit discipline: 0 on success; 1 with a one-line `promptworld <cmd>: error` on stderr;
2 for usage errors.

Every per-world command takes `<world>` — a name or a path (TASK-43). Arguments
containing `/` or starting with `.`/`~` are paths and behave exactly as before;
bare names resolve through `resolveWorld` → `worlds.Resolve`
([[instance-manager]]: worlds home first, then the known-worlds registry;
ambiguous or unknown names exit 1). `worldArg`/`parseWorldFlags` wrap the older
`dirArg`/`parseDirFlags` with that resolution.

- `new <name> [--at DIR] [--seed]` / `new <path> [--name] [--seed]` — a bare-word
  argument is a name: the world is created at `<worlds-home>/<name>` (or exactly
  `--at DIR`, which also registers it in the known-worlds registry), manifest name =
  the argument, validated by `worlds.ValidateName`. A path-shaped argument keeps the
  legacy form byte-for-byte: create at that path, name from `--name` (validated) or
  the basename (unvalidated, backward-compatible). Both forms then run the same
  creation: `world.Create` + store + genesis `world.created`
  event, writes the default `llm.json`, seeds the eight personas and Metatron's
  charter (`persona.Genesis`, the one-and-only persona write — [[agent-mind]],
  [[metatron]]), and
  appends the tick-0 secret events ([[social-fabric]]). Random default seed (crypto-random,
  right-shifted 12 bits to stay comfortably printable).
- `migrate <world>` — the one-time upgrade of an older world (v1 or v2) to the
  current format (spec 012 US6 for v1→v2, spec 013 for v2→v3 —
  [[world-migration]]): resolves `<world>` via `resolveWorldForMigrate`, which
  unlike `resolveWorld`/`worlds.Resolve` must reach older-format worlds that this
  build cannot `world.Open` — a path argument passes through verbatim, a bare name
  resolves against the worlds home then the known-worlds registry by manifest
  *presence* alone, never the version gate. Hands the whole
  archive/transform/rewrite ceremony to `world.Migrate`
  ([[world-save-directory]]), which admits a v1 or v2 source (a v1 world chains
  1→2→3 in one run; an already-current world is refused outright) and archives the
  original database under a name keyed to the source format (`world.v1.db` or
  `world.v2.db`). Prints a human summary (seed, villagers carried, continuation
  tick, source event count, archive path, and the `start` command to run next).
- `ps [--all] [--json]` — machine-wide listing of worlds with live-proven state
  ([[instance-manager]]): discovery over the worlds home + registry, concurrent
  bounded probes, `NAME STATE PID TICK GAME TIME SPEED LLM PATH` table or a JSON
  array reusing the `status --json` vocabulary. Default shows live-pid states
  (`running`/`paused`/`unresponsive`); `--all` adds `stopped`/`missing`/`unreadable`.
  Empty listing prints "no worlds running", exit 0.
- `daemon <world>` — the foreground primitive: `daemon.Run` directly.
- `start <world>` — detached start: re-execs itself (`os.Executable()` + `daemon <dir>`)
  with stdio appended to `daemon.log` and `Setsid` to leave the session, then polls
  the socket up to 5 s for a status round trip before reporting success. Never waits
  on the child.
- `stop <world>` — sends `shutdown` over the socket (falls back to SIGTERM if the
  socket is dead but the pid lives), waits ≤30 s for the pidfile to clear. Idempotent:
  "daemon not running" exits 0.
- `status <world> [--json]` — online: full `StatusData` via the client. Offline:
  last-known state reconstructed read-only from the store (latest snapshot +
  `LastEventTick`), clearly labeled "daemon not running".
- `pause` / `resume` / `speed <v>` — one-shot time controls printing the resulting
  clock line.
- `ui <world>` — the full-screen Bubble Tea client ([[tui-client]]): map, chronicle,
  metatron, villagers panes over a live world replica (villagers renamed from
  souls, spec 015); runs in the alternate screen.
  If the TUI quits on an unrecoverable protocol error (`Model.FatalErr()`, e.g. a
  reply over the IPC cap — TASK-19), the command returns it as a real error and
  exits non-zero.
- `attach <world>` — line-mode: status header, live subscribe streamed to stdout,
  stdin commands (`pause`, `resume`, `speed <v>`, `status`, `quit`); handles
  `dropped` pushes by re-subscribing. Quit detaches; the world keeps running.
- `tail <world> [--since SEQ] [--follow]` — history from the store (default last 20),
  works with no daemon; `--follow` additionally subscribes live and requires one.
- `metatron <world> [message...]` — the console one-shot ([[metatron]], TASK-12): with
  a message, one mediated turn (prints surfaced moments, the reply, any landed
  `⚡ vision/omen` line, a `👁 watch set`/`👁 watch released` line for a placed or
  cancelled standing order, a `⏲` line for a landed pause/start/adjust_speed
  meta tool call (spec 029, [[metatron-orders]]), and the charge bank); without,
  a model-free status peek (charges, charter provenance, a `--- standing
  orders ---` block via `orderStatusLine` — id, fuzzy marker, origin,
  remaining game-day, status, condition — when any order stands, and recent
  soul notes).
- `miracle <world> <snap-time|give|move|remove> ... [--force]` — the operator door
  for Metatron's miracles ([[metatron-miracles]], spec 016 R6), a dedicated
  subcommand family independent of the `metatron` conversational path: `snap-time
  <day> <HH:MM>`, `give <villager> <item> <qty>`, `move <class> <x,y> <x1,y1>`,
  `remove <class> <x,y>` (`<class>` is `villager|structure|pile|terrain`; terrain
  is remove-only, villagers cannot be removed). Dials the daemon and calls the
  `miracle` IPC command directly — no LLM involved. `--force` sets the gratis flag
  that waives the charge cost, an override reachable only from this CLI door, never
  from the angel's own turn. Prints the miracle summary (`(forced)` suffix when
  gratis) and the remaining charge bank.
- `llm <world> <kind> <prompt...> [--system] [--max-tokens]` — one-shot model call via
  the daemon's `llm_call` command; `formatLLMOneShot` prints the serving PROVIDER
  (never a tier — spec 024 FR-011), model, tokens, cost, and latency, plus a
  `skipped: name (reason)` line whenever the chain-walk passed over candidates
  ([[llm-orchestrator]]). `new` also writes the default `llm.json` config (v2
  registry shape; its hint says "edit providers/routes/budget").
- `calibrate <world> [--provider <name>] [--samples N]` (`--tier local|cloud|all`
  kept as a deprecated alias — on a v2 registry `local`/`cloud` select every
  zero-priced/priced provider with a deprecation note; on a legacy config it
  behaves exactly as before) — the cognition horizon's
  setup stage ([[cognition]], TASK-32): benchmarks the DECLARED PROVIDERS (spec
  024 T020: a legacy config runs the untouched pre-024 path over its two derived
  providers, byte-identical output; any v2 registry — or `--provider` — iterates
  `orch.ProviderNames()`, each reference call pinned via `Request.Provider` so
  the sample measures the named provider regardless of what its kind's chain
  currently resolves to) against fixed reference prompt shapes (default 5
  samples per shape; priced-provider spend is opt-in and announced up front),
  takes the median seconds-per-point, writes/merges `calibration.json` (one
  profile entry per provider name — the shape `cognition.SeedFor` reads), and
  prints the horizon the hardware buys (e.g. "planner suppressed above 16x") by
  evaluating the registry
  across the watchable speed ladder (`planner`/`conversation`/`meeting` — `musing`
  dropped from the ladder with its retirement as a scheduled kind, spec 017). Since
  spec 017 (FR-011) the `planner-3pt` shape is a LOOP probe, not a bare
  completion: `villagerProbeJob` drives `toolloop.Run` with the real
  `tool.LoopRosterVillager()` roster and a no-op handler per tool (every read
  reports `read_ok`, every acting call reports `landed` — ending the loop on the
  model's first action, since calibration measures round-trip latency, not
  landings) so the seeded seconds-per-point is measured in the SAME whole-loop
  unit `Orchestrator.ObserveCognition` later feeds live ([[llm-orchestrator]],
  [[tool-loop]]) — a representative tool loop's wall time, not one call's. The
  probe's round cap is `cfg.Rounds()` (the daemon's own `loop_max_rounds`), so the
  calibration and the live cognition share one horizon. Reference shapes select
  by PRICING CLASS (`refShapesFor(priced)`, spec 024 T020 generalizing the old
  tier branch): zero-priced providers get the loop probe, priced providers'
  `consolidation-5pt` shape stays a plain single-shot `Submit` (consolidation did
  not adopt the loop, FR-014) — Metatron IS the cloud's loop cognition, but
  calibrating it would drive extra metered cloud calls the spec 017 contract
  doesn't invite; its live whole-loop observations converge the cloud estimator
  at run time instead. Uses an in-memory meter so it never contends
  with a running daemon's store; a provider whose every sample fails is not
  written.

`parseDirFlags` accepts both `cmd <arg> --flag` and `cmd --flag <arg>` orderings
(`parseWorldFlags` adds name resolution on top).

## Connections

[[daemon-lifecycle]] is what `daemon`/`start` run; [[instance-manager]] owns name
resolution, discovery, and the `ps` probe; [[ipc-client]] carries every online
command; [[world-save-directory]] and [[event-log]] back the offline paths;
[[game-clock]] formats times in `clockLine`/`eventLine`; `calibrate` writes the
profile [[cognition]] routes with; `migrate` hands off to [[world-migration]];
`miracle` hands off to [[metatron-miracles]]; `metatron`'s standing-orders
rendering reads [[metatron-orders]].

## Operational notes

`start` failure says "check daemon.log". Detached daemons survive terminal close
(Setsid); a machine reboot needs a manual `start` (launchd integration is future
work — the foreground `daemon` subcommand is what a plist would run).
