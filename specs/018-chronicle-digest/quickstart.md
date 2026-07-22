# Quickstart: Validating the Chronicle Digest

**Feature**: specs/018-chronicle-digest

## Prerequisites

- Go 1.26 toolchain; repo checked out in the task worktree (`.worktrees/task-60`).
- No new dependencies — `go build ./...` must stay clean.

## 1. Mechanical gates (no live world needed)

```sh
go test ./internal/tui/           # digest unit tests + catalog sweep
go vet ./...
```

Expected: the sweep test (`digest_test.go`) passes — every cataloged event type digests without raw-JSON fallback (SC-001, contract §7). Delete any one registry entry to confirm the sweep fails (gate proves itself).

## 2. Live feed (US1, US3)

```sh
go build -o promptworld ./cmd/promptworld   # (or the project's usual build entry)
./promptworld daemon <world> &              # a world with prior history is ideal
./promptworld attach <world>
```

- Open the chronicle (dock tab or solo). Expected: every line reads `TICK HH:MM type summary` — no `{"agent":2,...}` dumps; agent names resolved; columns aligned at solo width; dock shows time + short type, no tick.
- Crank speed. Expected: no rendering lag (SC-005); deaths/attacks/thefts pop in the alert style; conversations read as `Ash→Rowan "…"` (SC-004).

## 3. Inspect mode (US2)

- Pause (`space`). Expected: selection marker appears; the **detail pane** is visible immediately at the panel bottom showing the selected event — seq, tick, type, verbatim pretty payload with `// name` annotations — with zero extra keypresses (SC-003).
- `j`/`k`/`g`/`G`: selection moves item by item; pane follows; pane scroll resets per move.
- `J`/`K`: long payloads scroll inside the pane; footer counts remaining lines.
- Select a `world.migrated` event (in a migrated world, `g` reaches it): pane stays bounded and navigable (FR-011).
- `⏎`: does nothing (reserved — contract §5 extension point).
- Resume: feed snaps back to tail-follow.

## 4. Fallback behavior (FR-002)

In a test (not live): feed a synthetic `store.Event` with type `future.unknown_type` through `formatChronicleLine` — expect the compact resolved-name JSON fallback, styled dim, no panic.

## 5. Doc reconciliation (FR-012)

- `docs/design/tui/patterns/chronicle-grammar.md` — line format, hybrid voice, color roles match shipped behavior.
- `docs/design/tui/panels/chronicle.md` — Mode 2 mockup shows the detail pane, keymap table updated (`⏎` reserved, `J`/`K` scroll).
- `docs/design/tui/patterns/keymap.md` — inspect-mode rows updated.
- After merge: `/grounding-wiki:wiki-update` re-pins wiki notes sourcing `internal/tui` (Principle IV).
