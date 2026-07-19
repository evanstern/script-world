# Data Model: Social Fabric

**Phase 1.** Event shapes in [contracts/social-events.md](contracts/social-events.md);
conversation prompts in [contracts/conversation-prompt.md](contracts/conversation-prompt.md).

## State additions

| Field | Type | Notes |
|---|---|---|
| `Relations` | []Relation | directed edges, lazy, flat slice (canonical-JSON safe) |
| `Debts` | []Debt | append-only lifecycle; `NextDebtID` counter |
| `Rumors` | []Rumor | registry identities; `NextRumorID` counter |
| `Agent.Known` | []KnownRumor | per-holder variants (text, confidence, provenance link) |
| `Agent.LastGive` | int64 | give-act cooldown |

Reputation is **computed**: `Reputation(state, agent) = clamp(500 + 100·kept − 200·broken)`.

## Types

```go
Relation  {From, To int; Trust, Affection int}          // −1000..1000, reducer-clamped
Debt      {ID int; Debtor, Creditor int; Kind string; Due int64; Status string} // open|kept|broken
Rumor     {ID int; Subject int; Tone int; Secret bool; OriginAgent int; OriginTick int64}
KnownRumor{RumorID int; Text string; Confidence int; From int; Tick int64}      // From −1 = originator
```

## Deterministic rules (executor / reducer)

| Act | Edge effect | Ledger effect |
|---|---|---|
| talk | +5 affection both ways | — |
| give (starving neighbor, food≥2, cooldown 1h) | receiver→giver +30 trust +20 affection; giver→receiver +10 affection | new open debt (due +2 game days) — unless it settles one |
| give-back (open debt exists) | same edges | oldest matching debt → kept |
| due-check (hourly) | creditor→debtor −150 trust −50 affection | overdue open → broken |
| hear rumor | listener→subject affection += tone/4 | — |
| conversation outcome | tone×25 affection, tone×12 trust each direction | — |

Tone at rumor birth (from source memory): death witnessed −80, near-death −40,
freezing night −20, broken promise −60, built +30, hunted +20, shared food +40,
talked +10.

## Conversation lifecycle (mind-side)

trigger `agent.talked` → slot acquire → immutable snapshot → ≤3 utterances/side →
outcome call → ONE `inject_social` batch:
`social.conversation_turn`×N, `social.conversation{a,b,gist,turns}`,
`agent.memory_added`×2 (gist, salience 4), `social.relation_changed`×2 (tones),
`social.rumor_told`×0..1 (paraphrased). Failure at any step → inject nothing.

## Provenance walk

Holder H's chain for rumor R: `H.Known[R].From → that agent's Known[R].From → … →
−1` (originator). Confidence strictly decreasing along the chain (×4/5 per hop,
floor 25 = no longer tellable).
