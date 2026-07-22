# Quickstart: validating the Villagers Tab

**Feature**: specs/015-villagers-tab

## Prerequisites

- Go 1.26.x toolchain (`export PATH="/opt/homebrew/bin:$PATH"` on this machine).
- A runnable world (any existing save, or bootstrap a fresh one).

## Automated validation

```sh
go build ./... && go vet ./...
go test ./internal/sim/ ./internal/tui/
go test ./...        # full suite before PR
```

Expected: all green, including
- sim: `intent_set` sets `LastGoal`/`LastGoalTick`; `intent_done` and
  `gru.attacked` preserve them; replay determinism; field-absent snapshot
  decodes to "never".
- tui: grammar tests for j/k/g/G/⏎/esc on the villagers tab (incl. esc chain
  detail→solo→home and nil-replica no-ops); render tests for cursor, detail
  sections, narrow/short budgets, and the absence of "soul" in user-visible
  strings.

## Manual validation (against a live world)

1. Start the daemon + TUI (`./scriptworld …` per README) at widescreen size.
2. Press `4` — the dock tab reads **villagers** (tab row, footer hint); the
   roster shows all 8 with a selection cursor. *(US1-AC1, SC-003)*
3. `j`/`k` to a villager who is actively working; press `⏎` — detail shows
   name/status/needs, itemized inventory, and the objective marked current.
   *(US1-AC2)*
4. Wait for that villager to finish (or pick an idle one) — detail still
   names the most recent objective, marked past. *(US2-AC2, SC-002)*
5. Confirm memories list most-recent-first; after a consolidated night,
   beliefs/narrative appear. *(US3)*
6. Leave the detail open while the world runs — inventory counts / objective
   / memories update without re-selecting. *(US1-AC4, SC-005)*
7. `esc` returns to the roster with the cursor where it was; `esc` again
   releases solo per the existing chain. *(US1-AC3)*
8. Shrink the terminal below the widescreen breakpoint and to minimum sizes —
   roster and detail condense/truncate, never overflow. *(FR-009, SC-004)*
9. Restart the TUI (fresh attach) and inspect an idle villager — the past
   objective is still shown (came in via snapshot, not live events).
   *(FR-006)*

## Reference

- Decisions: [research.md](research.md) (R1 last-goal mechanism, R2 grammar)
- Shapes: [data-model.md](data-model.md),
  [contracts/state-and-keys.md](contracts/state-and-keys.md)
