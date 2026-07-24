# Quickstart Validation: Epistemic Hygiene

Prerequisites: Go toolchain, local model configured (docs/llm-providers.md) for live scenarios,
`export PATH="/opt/homebrew/bin:$PATH"`.

## 1. Unit + determinism proof (no model)

```sh
go test -count=1 ./internal/sim/ ./internal/mind/ ./internal/scribe/
go test ./...    # full-suite phase gate
```

Expected: origin-stamping per emission site; validator coercion both directions (witnessed kept with action/omen
evidence, coerced to told on gist-only evidence, to inferred on none — SC-001); EffectiveConfidence curve table
(formation, half-life at 8 game-days, floor 20 crossing, legacy grandfather, reinforcement reset — SC-002); replay
byte-identity for a log containing coerced beliefs + a reinforcement event (SC-003).

## 2. Provenance honesty live (US1)

Scratch world with a local model; let villagers converse and sleep. Inspect `agent.belief_revised` events: beliefs
citing only gist memories land told/inferred with `direct: false`; any witnessed belief's evidence includes an
action/witness/omen memory. Check a night's marker for the coercion counter when the model over-claims.

## 3. Decay and the myth floor (US2)

Fixture or live world with a told belief formed on day D: `promptworld` status/soul inspection at D+8 shows the
effective confidence halved; at the floor-crossing day the belief leaves prompts and the soul renders it as
half-remembered. Inject `agent.belief_reinforced` (test harness) and verify the curve resets and replay holds.

## 4. Gist attribution eval (US3 — BEFORE the prompt ships)

```sh
scripts/eval-prompt-79.sh    # fixtures × {old,new} × N samples, judge-scored
```

Ship bar per contracts/eval-protocol.md: ≥50% reduction in flattening/confabulation, controls within tolerance;
numbers into eval/decision.md and onto TASK-79. Then a live multi-scene sample with zero "after investigating"
shapes, recorded in quickstart-results.md.

## 5. Myth survives (SC-005)

Multi-game-day live sample: an invented-lore thread persists in souls/rumors while no above-floor belief about it
carries witnessed without direct evidence. Record in quickstart-results.md (012/T045 precedent for anything not
observable in budget).

Contracts: [consolidation-contract.md](contracts/consolidation-contract.md) ·
[events-and-decay.md](contracts/events-and-decay.md) · [eval-protocol.md](contracts/eval-protocol.md) ·
[data-model.md](data-model.md)
