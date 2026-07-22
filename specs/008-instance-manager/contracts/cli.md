# CLI Contract: World Instance Manager

**Feature**: `008-instance-manager` | Extends `specs/001-world-daemon/contracts/cli.md`
(the base CLI contract). Only additions and changed semantics are listed; everything
else is byte-compatible (FR-012, SC-003).

## Global: `<world>` argument (all per-world commands)

Every command that took `<dir>` now takes `<world>` — a **name or a path**:

- **Path** (contains `/`, or starts with `.` or `~`): resolved exactly as today.
- **Name** (anything else): resolved worlds-home-first, then registry
  (see data-model.md "Name resolution").

Applies to: `daemon`, `start`, `stop`, `status`, `pause`, `resume`, `speed`, `ui`,
`attach`, `tail`, `metatron`, `llm`, `calibrate`.

**Errors (exit 1)**:

```
promptworld <cmd>: no world named "aria" (searched /Users/x/.promptworld/worlds and the known-worlds list) — try `promptworld ps --all`
promptworld <cmd>: name "aria" is ambiguous:
  /Users/x/.promptworld/worlds/aria
  /srv/games/aria
use a path to disambiguate
```

## `promptworld ps [--all] [--json]`

Lists worlds machine-wide from any CWD. Exit 0 always on success paths, including
"nothing running".

**Default**: worlds with a live daemon pid — states `running`, `paused`,
`unresponsive`. **`--all`**: adds `stopped`, `missing`, `unreadable` with last-known
tick/game time from the store.

**Human output** (columns; `no worlds running` when empty):

```
NAME      STATE     PID    TICK     GAME TIME     SPEED  LLM  PATH
aria      running   4242   180321   day 3 08:05   8x     on   ~/.promptworld/worlds/aria
harbor    paused    5150   99       day 1 06:01   1x     off  /srv/games/harbor
old-run   stopped   -      52100    day 1 20:28   -      off  ~/.promptworld/worlds/old-run
```

(`GAME TIME` is whatever `internal/clock.Format` renders — the same string `status`
has always printed; `ps` reuses it verbatim.)

**`--json`**: array, one element per world; reuses the `status --json` vocabulary:

```json
[
  {
    "name": "aria",
    "path": "/Users/x/.promptworld/worlds/aria",
    "state": "running",
    "world": { "name": "aria", "seed": 123, "format_version": 1 },
    "clock": { "tick": 180321, "game_time": "d3 02:05:21", "paused": false, "speed": "8x", "effective_rate": 8.0, "degraded": false, "metatron_charges": 3 },
    "daemon": { "pid": 4242, "uptime_seconds": 3600, "subscribers": 1 },
    "llm": { "...": "present iff inference enabled (as in status --json)" }
  },
  {
    "name": "old-run",
    "path": "/Users/x/.promptworld/worlds/old-run",
    "state": "stopped",
    "world": { "name": "old-run", "seed": 9, "format_version": 1 },
    "clock": { "tick": 52100, "game_time": "d1 14:28:20", "paused": false, "speed": "1x" },
    "daemon": { "running": false },
    "llm_configured": false
  }
]
```

- `state` ∈ `running | paused | unresponsive | stopped | missing | unreadable`.
- `running`/`paused` require a live status round-trip (FR-002); `unresponsive` = live
  pid, no answer within budget — never rendered as running.
- Stopped worlds: `clock` is last-known (store snapshot + last event tick, same as
  offline `status --json`); `llm_configured` reflects `llm.json` presence.
- `missing`/`unreadable`: `name`, `path`, `state` only (plus `error` string for
  `unreadable`).
- Whole-listing budget: < 2s regardless of wedged daemons (parallel probes, ~1s
  per-world budget).

## `promptworld new` (changed first-positional semantics)

```
promptworld new <name> [--at DIR] [--seed N]     name-form (NEW default)
promptworld new <path> [--name NAME] [--seed N]  path-form (unchanged legacy behavior)
```

- `<name>` (bare word): creates `<worlds-home>/<name>`, manifest name `<name>`,
  creating the worlds home if needed. Existing directory ⇒ refuse, exit 1, world
  untouched. `--name` is rejected in name-form (the argument IS the name).
- `--at DIR`: creates the world at exactly `DIR` (not `DIR/<name>`) with manifest name
  `<name>`, and registers it in the known-worlds record.
- `<path>` (contains `/` or starts with `.`/`~`): today's exact behavior — create at
  path, name from `--name` or basename. `--at` is rejected in path-form.
- Name validation (both `<name>` and `--name`): non-empty, no `/`, no leading `-` or
  `.`; violation ⇒ exit 1 with the rule stated.
- Success output additionally suggests name-based commands when a name-form world was
  created: `start it with: promptworld start <name>`.

## `promptworld stop <world>`

Unchanged semantics by path; by name identical after resolution: idempotent ("daemon
not running" ⇒ exit 0), graceful shutdown then SIGTERM fallback, 30s deadline.

## Daemon side effect (advisory)

`promptworld daemon <world>` (and thus `start`) upserts `{manifest-name → dir}` into
the known-worlds record at boot **iff** the dir is outside the current worlds home.
Failure to write is logged and non-fatal. No other daemon behavior changes; no IPC
protocol changes.

## Environment

| Variable | Effect |
|---|---|
| `PROMPTWORLD_HOME` | overrides `~/.promptworld` (worlds home `$PROMPTWORLD_HOME/worlds`, registry `$PROMPTWORLD_HOME/known_worlds.json`) |
