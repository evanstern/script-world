# Data Model: Hail Protocol

## Entities

### AgentHail (new, on Agent)

The target-side pause state. Nil when the agent is not hailed.

| Field | Type | Meaning |
|-------|------|---------|
| `By` | `int` (`json:"by"`) | agent index of the hailer |
| `Until` | `int64` (`json:"until"`) | tick at which the pause expires |

- Carried as `Agent.Hail *AgentHail` with `json:"hail,omitempty"` — pre-feature
  snapshots unmarshal to nil and canonical bytes for un-hailed agents are unchanged
  (FR-010, determinism hash).
- Written ONLY by the reducer (`social.hailed` sets it; `social.hail_met`,
  `social.hail_expired`, `agent.died`, `agent.slept` clear it).

### Hail lifecycle (event-sourced state machine)

```text
              social.hailed {from,to,until}
   (none) ────────────────────────────────▶ paused (Hail = {By:from, Until:until})
                                              │
              hailer adjacent (≤1), tick<Until│  social.hail_met {from,to}  → (none)
              tick >= Until                   │  social.hail_expired {from,to} → (none)
              target dies / falls asleep      │  agent.died / agent.slept   → (none)
```

Met is checked before expiry in the same sweep — a hailer arriving on the expiry
tick wins.

## Derived predicates (pure functions of State, no stored state)

| Predicate | Definition |
|-----------|------------|
| `hailable(s, hailer, target)` | target alive ∧ awake ∧ `Hail == nil` ∧ not an active hailer (no agent k with `Agents[k].Hail.By == target`) ∧ not meeting-pinned ∧ Manhattan(hailer, target) ≤ `hailRadius` |
| `paused(a, tick)` | `a.Hail != nil ∧ tick < a.Hail.Until` |
| mutual-presence rung | actor's own hailer (`actor.Hail != nil ∧ actor.Hail.By == target`) → target treated as present, no new hail |

## Event contracts (additions)

See [contracts/events.md](contracts/events.md) for payload JSON and emitter/reducer
responsibilities.

| Event | Emitter | Reducer effect |
|-------|---------|----------------|
| `social.hailed` | loop (`inject_intent` talk_to landing), executor (`planStepEvents` talk_to firing) | sets target's `Hail` |
| `social.hail_met` | executor (per-tick hail sweep) | clears target's `Hail` (accompanying `agent.talked` applies its existing effects) |
| `social.hail_expired` | executor (per-tick hail sweep) | clears target's `Hail`; nothing else |

None of the three are in `injectSocialWhitelist` — they are world facts, not
model-injectable.

## Tunables (constants, `internal/sim`)

| Constant | Value | Constraint it encodes |
|----------|-------|----------------------|
| `hailRadius` | 64 | covers observed failure distances 35–50 with margin (FR-003) |
| `hailWindowTicks` | 480 | 8 game-minutes ≈ walk time at max hail range + ~50% margin (FR-005) |

## Validation rules

- `social.hailed` may only target a hailable agent (emitters check; reducer applies
  what the log says — replay fidelity over re-validation, same as every other event).
- Pause suppresses ONLY movement decisions (reflex, plan-step evaluation, en-route
  stepping); `Intent`, `Plan`, needs, and social participation are untouched (FR-004).
- First hail wins: emitters never hail an already-hailed target; `until` is never
  extended (spec US3 scenario 3).
