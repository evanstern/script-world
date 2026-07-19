---
name: cli-scriptworld
description: The single scriptworld binary — subcommand dispatch, world management, daemon control, observation commands, exit discipline
kind: component
sources:
  - cmd/scriptworld/main.go
  - cmd/scriptworld/commands.go
verified_against: f4786fdb378059d04d20f2b8c8bced549d7a9922
---

# scriptworld CLI

One binary serves every role: daemon, client tools, world management. `main.go` is a
plain dispatch table; all behavior lives in `commands.go`. The prose contract is
`specs/001-world-daemon/contracts/cli.md`.

## How it works

Exit discipline: 0 on success; 1 with a one-line `scriptworld <cmd>: error` on stderr;
2 for usage errors.

- `new <dir> [--name] [--seed]` — `world.Create` + opens the store + appends the
  genesis `world.created` event at tick 0. Random default seed (crypto-random,
  right-shifted 12 bits to stay comfortably printable).
- `daemon <dir>` — the foreground primitive: `daemon.Run` directly.
- `start <dir>` — detached start: re-execs itself (`os.Executable()` + `daemon <dir>`)
  with stdio appended to `daemon.log` and `Setsid` to leave the session, then polls
  the socket up to 5 s for a status round trip before reporting success. Never waits
  on the child.
- `stop <dir>` — sends `shutdown` over the socket (falls back to SIGTERM if the
  socket is dead but the pid lives), waits ≤10 s for the pidfile to clear. Idempotent:
  "daemon not running" exits 0.
- `status <dir> [--json]` — online: full `StatusData` via the client. Offline:
  last-known state reconstructed read-only from the store (latest snapshot +
  `LastEventTick`), clearly labeled "daemon not running".
- `pause` / `resume` / `speed <v>` — one-shot time controls printing the resulting
  clock line.
- `ui <dir>` — the full-screen Bubble Tea client ([[tui-client]]): map, chronicle,
  metatron, souls panes over a live world replica; runs in the alternate screen.
- `attach <dir>` — line-mode: status header, live subscribe streamed to stdout,
  stdin commands (`pause`, `resume`, `speed <v>`, `status`, `quit`); handles
  `dropped` pushes by re-subscribing. Quit detaches; the world keeps running.
- `tail <dir> [--since SEQ] [--follow]` — history from the store (default last 20),
  works with no daemon; `--follow` additionally subscribes live and requires one.

`parseDirFlags` accepts both `cmd <dir> --flag` and `cmd --flag <dir>` orderings.

## Connections

[[daemon-lifecycle]] is what `daemon`/`start` run; [[ipc-client]] carries every online
command; [[world-save-directory]] and [[event-log]] back the offline paths;
[[game-clock]] formats times in `clockLine`/`eventLine`.

## Operational notes

`start` failure says "check daemon.log". Detached daemons survive terminal close
(Setsid); a machine reboot needs a manual `start` (launchd integration is future
work — the foreground `daemon` subcommand is what a plist would run).
