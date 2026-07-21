# Contract: `scriptworld` CLI

> **Extended** by [`specs/008-instance-manager/contracts/cli.md`](../../008-instance-manager/contracts/cli.md):
> every `<dir>` below also accepts a world **name** (resolved against a default worlds
> home), plus the new `ps` command — path-based invocations documented here remain
> byte-compatible (FR-012/SC-003). This file is kept as historical record of the
> original path-only surface; the two documents together are the current contract.

Single binary. All subcommands take a save-directory path (`<dir>`). Exit code 0 on
success; non-zero with a one-line error on stderr otherwise. Human output on stdout;
`--json` gives machine output where noted.

## World management

### `scriptworld new <dir> [--name NAME] [--seed N]`
Creates the save directory (must not exist or be empty), writes `world.json` and an
initialized `world.db` (schema + `world.created` event at tick 0). Default name =
directory basename; default seed = crypto-random, printed.

### `scriptworld status <dir> [--json]`
If a daemon is running: attaches briefly and prints tick, game time (day/HH:MM), speed,
effective rate, paused/degraded flags, uptime, subscriber count. If not running: says so
and prints last-known clock state from the store. Never starts anything.

## Daemon lifecycle

### `scriptworld daemon <dir>`
Runs the daemon in the foreground (the primitive; used by e2e tests and service
managers). Recovers from snapshot+log, binds `<dir>/daemon.sock`, runs the loop until
SIGTERM/SIGINT or a `shutdown` command → final snapshot, clean exit 0.

### `scriptworld start <dir>`
Detached start: verifies no live daemon (pidfile liveness; sweeps stale pid/sock),
re-execs itself as `daemon <dir>` with stdio → `<dir>/daemon.log`, writes
`<dir>/daemon.pid`, waits until the socket answers a `status` round trip (≤ 5 s), prints
confirmation. Fails fast if a daemon is already running.

### `scriptworld stop <dir>`
Sends `shutdown` over the socket; waits for process exit (≤ 10 s), reports. If no
daemon is running, says so and exits 0 (idempotent).

## Time controls (one-shot conveniences over the same protocol)

### `scriptworld pause <dir>` / `scriptworld resume <dir>`
Sends `pause`/`resume`; prints resulting clock state. Idempotent (pausing a paused
world is a no-op success).

### `scriptworld speed <dir> <1x|4x|8x|16x|max>`
Sends `set_speed`; prints resulting clock state.

## Observation

### `scriptworld attach <dir>`
Interactive line client: prints a status header, then streams events as they arrive
(one line each: seq, tick, game time, type, payload summary). Reads commands from
stdin: `pause`, `resume`, `speed <v>`, `status`, `quit`. Ctrl-C / `quit` detaches;
the daemon keeps running (US1/US2 scenario driver until the TASK-3 TUI exists).

### `scriptworld tail <dir> [--since SEQ] [--follow]`
Prints events from the log starting at SEQ (default: last 20), optionally following
live. Works read-only even with no daemon running (except `--follow`, which requires one).
