# Quickstart Validation: Resources, Food, and Crafting v1

How to prove the feature works end to end. Prerequisites: Go toolchain, repo root,
`export PATH="/opt/homebrew/bin:$PATH"` (per project memory) for `go`.

## 1. Unit & determinism suite

```sh
go test ./...
```

Must pass, including (new or extended):

- `internal/worldmap`: same-seed ⇒ identical `Hash()` across a seed spread; outcrops
  present on every seed; water/trees/forage/dens still present; ≥25% buildable grass.
- `internal/sim`: replay byte-identity over a run exercising every new event type;
  contested quarry (two agents, one outcrop); fire burnout emits exactly once and
  refuel re-arms; eat consumes most-nutritious-first with absolute `food_after`;
  spear spend-lowest + break-at-zero (with memory); craft input re-validation;
  oven fuel-required (no wood ⇒ no effect); shelter rest bonus rate.
- Degraded-mode regression: planner-less village of 8 survives ≥3 game days with zero
  `agent.crafted`/`agent.cooked`/`agent.bathed` events (SC-002, FR-012/FR-008 of spec).
- `internal/world`: format v1 world refused with the unsupported-version error.

## 2. Fresh-world smoke (deterministic, no LLM)

```sh
go run ./cmd/promptworld new demo-012 --seed 42   # adjust to actual CLI shape
go run ./cmd/promptworld start demo-012
```

Expected within the first game days (observe via TUI or log tail):

- Map shows rock outcrops (new glyph); fires burn out at night if unfueled and agents
  refuel them (reflex rule); nobody crafts (no planner = subsistence).

## 3. Planner-driven progression (LLM on)

With the configured local tier running, watch for the SC-003 chain within ~2 game days:
`agent.quarried` → `agent.crafted{planks, refined_stone, spear}` →
`agent.built{oven}` → `agent.cooked{oven}` → eating meals; ideally `agent.bathed`.
Chronicle should narrate the firsts (oven, bath, spear breaking).

## 4. Replay check

Stop the daemon, replay the log (existing replay/verify tooling), and compare state
hashes — must be byte-identical including `Quarried` overlay, `FuelUntil` values, and
`Spears` slices.

## 5. Old-world refusal & migration (US6)

Point the v2 build at an un-migrated v1 world: daemon must refuse with the
unsupported-version error naming `promptworld migrate`, leaving the world untouched.

Then the real thing (SC-007), against `~/.promptworld/worlds/myworld-01`:

```sh
cp -R ~/.promptworld/worlds/myworld-01 ~/.promptworld/worlds/myworld-01.backup
# ensure cleanly stopped (v1 binary stop → finalSnapshot covers the log)
go run ./cmd/promptworld migrate myworld-01
go run ./cmd/promptworld start myworld-01
```

Expected: `world.v1.db` archived beside the new `world.db`; manifest at
`format_version: 2`; villagers keep their souls (memories/beliefs/relations/debts,
chronicle continues, same tick/day); map is the v2 regeneration (outcrops present);
no structures, everyone awake on passable tiles; second `migrate` run refuses.
Determinism proof: delete all rows from `snapshots`, restart — replay from genesis
(`world.created` → `world.migrated`) reproduces the identical state hash.

## 6. Post-merge (Definition of Done tail)

- `/grounding-wiki:wiki-update` — re-verify/re-pin: executor, event-types,
  worldmap-generation, reflex-policy, sim-state-reducer, tui-client, agent-mind,
  snapshots.
- `spec-bridge:sync` — board catches up to artifacts.
- Worktree cleanup per constitution II (root stays on main).
