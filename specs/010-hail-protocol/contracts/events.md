# Event Contracts: Hail Protocol

Additions to the world's event vocabulary. All three are deterministic
world-emitted events: never model-injectable (NOT in `injectSocialWhitelist`),
never emitted from wall-clock or unseeded-random paths.

## social.hailed

Emitted when a `talk_to` landing (planner injection or plan-step firing) targets a
hailable agent. One per landing; never against an already-hailed target.

```json
{"from": 5, "to": 3, "until": 123456}
```

| Field | Type | Meaning |
|-------|------|---------|
| `from` | int | hailer agent index |
| `to` | int | target agent index |
| `until` | int64 | expiry tick (`landing tick + hailWindowTicks`) |

**Reducer**: sets `Agents[to].Hail = {By: from, Until: until}`.

**Emitters**: `loop.go` `inject_intent` (after a successful/adapted talk_to
landing); `plan.go` `planStepEvents` (talk_to step firing).

## social.hail_met

Emitted by the per-tick hail sweep when the hailer is within Manhattan distance 1
of its paused target before expiry. Always accompanied (same tick, same batch) by
the deterministic talk founding: `agent.talked {a: from, b: to}` plus the standard
relation/memory events, bypassing the ambient talk cooldown check.

```json
{"from": 5, "to": 3}
```

**Reducer**: clears `Agents[to].Hail`. (The accompanying `agent.talked` applies its
existing morale/`LastTalk` effects.)

## social.hail_expired

Emitted by the per-tick hail sweep when `tick >= Until` and the hailer never
arrived. Met wins ties: adjacency is checked before expiry on the same tick.

```json
{"from": 5, "to": 3}
```

**Reducer**: clears `Agents[to].Hail`. No other state change — intent, plan, and
needs are exactly as the pause left them (FR-005, SC-003).

## Implicit clears (existing events, extended reducer semantics)

| Event | Added effect |
|-------|--------------|
| `agent.died` (target) | clears `Hail` |
| `agent.slept` (target) | clears `Hail` |

## Landing-ladder contract change (`inject_intent`, talk_to only)

Rung order for a metered `talk_to` landing, after generation and staleness checks:

1. `target_present` guard holds → land as today (adapted if target moved); hail the
   target if hailable.
2. Guard fails, target is the actor's own hailer → land as **adapted**; no new hail.
3. Guard fails, target hailable (within `hailRadius`) → land as **adapted**; emit
   `social.hailed`.
4. Otherwise → `rejected-guard` with the existing reason, unchanged.

Goals other than `talk_to`, and all other guard types, are untouched. The guard
vocabulary itself (`guard.go`) does not change.

## Observability

Payloads use `from`/`to` so the chronicle grammar's name resolution renders them
with agent names in `scriptworld tail` and the TUI feed with no view changes.
