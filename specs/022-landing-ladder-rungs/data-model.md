# Data Model: Extract the Intent-Landing Ladder into Named Rungs

**Feature**: 022-landing-ladder-rungs | **Date**: 2026-07-23

Pure refactor — no persisted data, event schema, or wire format changes. The only new
data shape is an in-memory decision value private to `internal/sim`.

## landingDecision (new, package-private, in-memory only)

The explicit outcome of the guard walk, replacing the `adapted` / `failed` / `hailTarget`
cross-loop flags.

| Field | Type | Meaning |
|---|---|---|
| `outcome` | string | One of the existing doctrine constants: `OutcomeLanded`, `OutcomeAdapted`, `OutcomeUnavailable`, `OutcomeSuperseded`, `OutcomeRejectedStale`, `OutcomeRejectedGuard` (cognition.go:15-21). No new values. |
| `reason` | string | Rejection reason, verbatim the strings produced today (Eval's `why`, `fmt.Sprintf` texts). Empty on accept. |
| `hailTarget` | int | Agent index to hail on the goal path's success, `-1` for none. Written by hail-relaxed and in-radius rungs; last write wins (walk order preserved). |

**Validation rules**: `outcome` ∈ the six constants above; `hailTarget` only meaningful
when `outcome` is `OutcomeLanded`/`OutcomeAdapted`; rejection ⇒ `reason` non-empty.

**State transitions** (the ladder, order frozen):

```text
bounds-check ─err only─▶ (no events)
unavailable (dead → asleep) ─▶ reject
[class only] superseded ─▶ reject
[class only] stale ─▶ reject
[class only] guard walk:
    guard fails ─▶ hail-relaxed (mutual-hailer → adapted; hailable → adapted+hail)
                └▶ guard-failed ─▶ reject (short-circuit)
    guard holds ─▶ adapted (moved target) / in-radius hail marking
plan path (validate → plan_set)  |  goal path (roster → resolve → intent_set/ate → hailed)
[class only] cog.outcome landed|adapted
```

## Unchanged entities (consumed as-is)

- **Events**: `agent.intent_rejected`, `cog.outcome`, `agent.thought`, `agent.plan_set`,
  `agent.intent_set`, `agent.ate`, `social.hailed` — kinds, payload structs, and emission
  order untouched.
- **Guard** (guard.go): vocabulary and `Eval` untouched.
- **Outcome constants** (cognition.go): untouched.
- **Hail predicates** (hail.go): `hailable`, `hailWindowTicks` consumed unchanged.
