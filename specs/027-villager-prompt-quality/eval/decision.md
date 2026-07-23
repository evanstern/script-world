# Ship-gate decision — TASK-73 villager prompt quality

**Date**: 2026-07-23 | Spec 027 | Gate: FR-006/007, SC-001/003/004; exemplar
decision FR-004.

Three soaks, run **serially** on one machine, identical setup (research D3):
seed 4242, **8 game-hours** each (target tick 28800), local Ollama `cogito:3b`
with `tool_mode: json`, speed 16x. Numbers are tallied from the durable event
log: rate denominator = every villager-planner `cog.tool_call` event (planner
jobs isolated by joining `cog.thought` class `planner`; metatron/conversation
excluded — research D1). Full per-variant records: `old.md`, `new.md`,
`new-exemplar.md`.

Window note (research D3): the 6-game-hour target projected to ~276 planner
decisions from a 1-hour bring-up probe; the window was set to **8 game-hours**
for margin, applied identically to all three variants. Every variant cleared
the 200-decision floor with room to spare (denominators 789 / 896 / 982).

## Numbers

| variant | git ref | denom | rejected_malformed | rejected_cardinality | approx tokens |
|---------|---------|-------|--------------------|----------------------|---------------|
| `old` | origin/main (`b96c028`) | 789 | 121 = **15.34%** | 0 = **0.00%** | 189 |
| `new` | `dba7868` | 896 | 103 = **11.50%** | 0 = **0.00%** | 193 |
| `new+exemplar` | `0310442` | 982 | 147 = **14.97%** | 0 = **0.00%** | 251 |

(`old` was built from `origin/main` = `b96c028`, which had advanced past the
branch's merge-base `9754416` via the TASK-82 player-docs merge. That merge
changed only `docs/` and `.claude/skills/` — **no** `internal/` / `cmd/` /
`go.mod` code — so the `old` daemon binary differs from `new`/`new+exemplar`
only in `systemPrompt`. Comparison is apples-to-apples.)

## SC-001 — ship gate (both rejection rates ≤ old, for the shipped variant)

- **`new`**: malformed 11.50% ≤ 15.34% ✓ ; cardinality 0.00% ≤ 0.00% ✓ → **PASSES**.
  Malformed improved by **3.84 pp** (a 25% relative drop). Cardinality was, and
  stays, zero.
- `new+exemplar`: malformed 14.97% ≤ 15.34% ✓ (barely) ; cardinality 0.00% ✓ —
  technically passes vs `old`, but loses to `new` (below).

## FR-004 — exemplar decision: **REJECTED**, ship `new`

`new` beats `new+exemplar` on **both** deciding axes:

- **Rejection rate (primary)**: `new` malformed 11.50% vs `new+exemplar` 14.97%
  — adding the worked exemplar pushed malformed rejections **up 3.47 pp**, nearly
  all the way back to the `old` baseline. On a 3B-class local model the exemplar's
  added length and anchoring cost outweighed any teaching value — exactly the
  small-model degradation the spec's token-cost edge case warns about.
- **Token cost (tiebreak, moot here)**: `new` 193 vs `new+exemplar` 251 approx
  tokens (+58, +30%). Even had the rates tied, FR-004 ships the cheaper variant.

Anchoring signal (research D5 / SC-003 reject signal): the exemplar features
**cooking**, and `cook`'s landed share rose monotonically with the exemplar —
`old` 2.50% → `new` 3.45% → `new+exemplar` 5.52%. The variant drifted toward the
example's verb, the parroting-over-teaching failure D5 anticipated.

**Decision**: ship `new`. Revert the exemplar commit (`0310442`) so the branch
tip is the shipped variant (`new`); the exemplar and its numbers remain in
history and in this record.

## SC-003 — distribution screen (shipped variant `new` vs `old`)

Every acting tool at ≥5% share under `old` retains a **nonzero** share under
`new`:

| tool | old share | new share | note |
|------|-----------|-----------|------|
| muse | 34.37% | 18.73% | shrank — the musing flood is roughly halved |
| write_journal_entry | 13.43% | 17.86% | ×1.33 |
| forage | 10.93% | 11.81% | ×1.08 |
| wander | 10.93% | 15.56% | ×1.42 |
| quarry | 6.56% | 3.45% | shrank |
| sleep | 5.62% | 4.03% | shrank |
| talk_to | 5.62% | 9.79% | ×1.74 |

No tool that was major under `old` more than doubles, and none collapses to
zero. **No collapse; no muse explosion** — in fact muse (the largest old share)
*shrank* by ~16 pp, the single healthiest movement: the rewrite pulled villagers
out of the thinking-instead-of-doing rut the old prompt encouraged.

**Accepted deviations** (SC-003 permits explained deviations): a few *small*
tools grow past 2× off tiny bases — `eat` 1.56%→4.32% (5→15 calls), `chop`
0.62%→2.01% (2→7 calls), `goto_warmth` 2.81%→4.32%. These are low-count,
below the 5%-share screen threshold, and represent **broader action diversity**,
not collapse. Accepted.

## Verdict

- **SC-001**: PASS for `new` (both rates ≤ old; malformed materially better).
- **SC-002**: PASS (name-once, `TestSystemPromptNamesOnce`).
- **SC-003**: PASS (no collapse; muse flood reduced; small deviations accepted).
- **SC-004**: PASS (token counts recorded for all three variants).
- **SC-005**: PASS (byte-identical renders, `TestSystemPromptPurity`).
- **FR-004**: exemplar REJECTED with measured reason (worse malformed rate +
  higher token cost + cook-verb anchoring). Ship `new`.

**Shipped variant: `new`** (`dba7868`). The branch tip is reset to it by
reverting the exemplar commit.
