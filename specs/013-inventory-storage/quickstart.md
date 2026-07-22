# Quickstart: Inventory & Storage v1 — validation guide

How to prove the feature works end-to-end. Shapes in [data-model.md](data-model.md);
behavior in [contracts/events.md](contracts/events.md); decisions in
[research.md](research.md).

## Prerequisites

- Go toolchain; repo root on the task worktree (`.worktrees/task-51`).
- A scratch worlds home so e2e runs never touch `~/.promptworld` (TASK-49 is
  open on exactly this — export the worlds-home override the instance manager
  honors, or pass explicit world paths).

## Automated gates (all must pass)

```sh
go build ./... && go vet ./...
go test ./...
```

Suites the implementation must add (alongside code, per testing-strategy):

1. **Determinism/replay**: a scripted run exercising drop, pick_up, deposit,
   withdraw (owner + thief), build_chest, a death spill, and a rot expiry
   replays to **byte-identical state** — piles, batches, spoil deadlines, chest
   contents, owner (SC-005). New event types no-op under a pre-013 reducer stub.
2. **Bulk audit**: table test over every acquisition edge (research R2 table) —
   truncation at partial space, no-event/no-depletion at zero space, craft
   no-fit ⇒ intent_done, give skipped at full receiver, cook/bathe/build never
   need a check (net ≤ 0 asserted).
3. **Degraded mode** (SC-001): 8 agents, no planner, 3+ game days — everyone
   lives, **zero** storage events in the log, cap never deadlocks the raw loop.
4. **Theft** (SC-003): non-owner withdrawal ⇒ `social.chest_taken` +
   reason-`theft` relation delta + owner memory + in-range witness memories, in
   one batch; owner withdrawal ⇒ none of those; 0% blocked either way.
5. **Rot** (SC-004): ground food gone within `rotWindowTicks` + 1 game minute;
   chest food immortal; non-food immortal everywhere.
6. **Migration**: a v2 fixture world migrates 2→3 — people/structures/overlays
   verbatim, no land reset, over-cap carry spilled to a pile at the agent's
   tile, `world.v2.db` archived; a v1 fixture chains 1→2→3.

## Live validation (SC-002, SC-006)

```sh
go run ./cmd/promptworld new smoke-013 --seed 42   # v3 world
go run ./cmd/promptworld start smoke-013
go run ./cmd/promptworld attach smoke-013          # TUI
```

- Within 2 game days of planks existing (planner on): at least one chest built,
  deposits + withdrawals in the log, a ground pile/stockpile in active use.
- From the TUI alone answer: where goods are stored (pile/chest glyphs; adjacent
  piles read as one zone), what a given pile/chest holds, who owns a chest, and
  each villager's carried bulk (`n/24`).
- Chronicle picks up storage stories (first chest, a taking, a death-site
  recovery).
- Migration smoke: `go run ./cmd/promptworld migrate <v2-world>` then start +
  attach; villagers, structures, and any spilled piles present.

## Post-merge (constitution IV)

Re-pin wiki notes whose sources this touches: executor, event-types,
sim-state-reducer, reflex-policy (vocabulary tables), agent-mind (goal
vocabulary), social-fabric (theft wiring), world-migration + world-save-directory
(format v3), snapshots (state shape), tui-client, cli-promptworld (migrate 2→3),
testing-strategy — via `/grounding-wiki:wiki-update`.
