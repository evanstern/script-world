# Data Model: World Instance Manager

**Feature**: `008-instance-manager` | **Date**: 2026-07-21
**Sources**: spec.md Key Entities; research.md D1/D4/D6/D7

## Entities

### World (existing — unchanged on disk)

A self-contained save directory. Identity IS the directory (grounded decision).

| Field | Type | Notes |
|---|---|---|
| `Dir` | string | absolute path of the save directory |
| `Manifest.Name` | string | recorded display name; becomes the primary user-facing handle |
| `Manifest.Seed`, `CreatedAt`, `FormatVersion`, … | — | unchanged (`internal/world/world.go`) |

**Invariants (unchanged)**: pidfile `daemon.pid`, socket `daemon.sock`, log
`daemon.log`, LLM config `llm.json` all live inside `Dir`. Nothing this feature adds is
required for the world to run (FR-008, SC-004).

**New validation at creation (FR-009)** — applies to the bare-name form of `new` and to
`--name`: non-empty; no `/` (path separator); does not start with `-` (flag-like) or `.`
(hidden/path-like); usable as a directory name. Violations: exit 1 with the rule stated.

### Worlds home (new)

| Property | Value |
|---|---|
| Root | `$SCRIPTWORLD_HOME` if set, else `~/.scriptworld` |
| Worlds dir | `<root>/worlds` |
| Created | lazily, on first `new <name>` (FR-004) |
| Discovery | every immediate subdirectory containing a readable `world.json` |

A subdirectory with an unreadable/corrupt `world.json` is listed by `ps --all` as
`unreadable` (never aborts the listing — edge case).

### Known-worlds record (new — advisory)

File: `<root>/known_worlds.json`, atomic write (temp + rename).

```json
{
  "worlds": {
    "<name>": { "path": "/abs/path/to/world" }
  }
}
```

| Field | Type | Rules |
|---|---|---|
| key | string | manifest name at last registration |
| `path` | string | absolute world dir; never inside the current worlds home (home is scan-owned) |

**Lifecycle**: upserted on `new` at a custom path and on every daemon boot
(`daemon.Run`), keyed by manifest name (a moved world self-repairs on next start).
Read-tolerant: missing/corrupt file ⇒ empty registry, never an error. Entries whose
`path` lacks a readable `world.json` are skipped by resolution, shown `missing` in
`ps --all`, and pruned on next write. Registry writes are best-effort in the daemon —
a failure logs and continues (advisory, FR-008).

### Running instance (derived — never stored)

Computed per candidate world at `ps`/resolution time:

| Field | Source |
|---|---|
| `pid` | `daemon.pid` contents, pre-filtered by `kill(pid, 0)` |
| `state` | see state machine below |
| `tick`, `game_time`, `paused`, `speed` | live `status` reply; else last-known from store (offline path, as `cmdStatus` today) |
| `llm` | live: `StatusData.LLM != nil`; stopped: `llm.json` exists |

## State classification (per world, at query time)

```
                     ┌── status answers in budget ──▶ paused?  ─ yes ─▶ PAUSED
candidate ─ pid live ┤                                   └─ no ──▶ RUNNING
     │               └── no answer in budget ─────────────────────▶ UNRESPONSIVE
     └─ pid dead/absent ─▶ world.json readable? ─ yes ─▶ STOPPED
                                └─ no ─▶ dir exists? ─ yes ─▶ UNREADABLE
                                              └─ no ──▶ MISSING (registry entries only)
```

- Default `ps` shows only RUNNING / PAUSED / UNRESPONSIVE (live-pid states);
  `--all` adds STOPPED / MISSING / UNREADABLE.
- RUNNING is only ever produced by a live status round-trip (FR-002); UNRESPONSIVE is
  explicitly not "running" and never reported as such.

## Name resolution (deterministic, FR-007/FR-011)

```
arg contains "/" or starts with "." or "~"  ─▶ PATH: today's behavior, verbatim
otherwise NAME:
  home := <worlds home>/<name> is a world?          (H)
  reg  := registry[name] resolves to a world?       (R)
  H ∧ R ∧ different dirs  ─▶ error: ambiguous, list both paths, exit 1
  H                        ─▶ home dir
  R                        ─▶ registry path
  neither                  ─▶ error: not found; names worlds home searched;
                              suggests `scriptworld ps --all`; exit 1
```
