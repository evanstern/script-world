---
name: social-fabric
description: The conflict engine — directed relation edges, debt ledger with computed reputation, rumors with provenance and mutation, authored secrets, and model-driven conversations injected atomically
kind: component
sources:
  - internal/sim/social.go
  - internal/mind/convo.go
verified_against: 7565ba91c8c8503e4580ae0fc16d0bbf14f122a2
---

# Social fabric

TASK-8's conflict engine: everything villagers feel about each other, owe each
other, and say about each other — event-sourced in the deterministic core, with
model creativity (dialogue, paraphrase) entering only as recorded events.

## How it works

**Edges** (`sim/social.go`): directed `Relation{From, To, Trust, Affection}`
(−1000..1000, reducer-clamped, lazy). One event type moves them —
`social.relation_changed` with a reason — emitted by fixed rules: talk +5/+5
affection, give (+30 trust/+20 affection receiver→giver), promise broken (−150/−50
creditor→debtor), rumor tone/4 listener→subject, conversation tones ×12/×25.

**Ledger**: a give to a starving neighbor opens `Debt{due +2 game days}`
(reducer-internal on `social.gave`); a matching give-back settles it kept; the
executor's hourly due-check breaks overdue debts permanently — with the trust
penalty and a gossip-seed memory ("X never repaid…"). `Reputation` is computed
(500 +100·kept −200·broken), never stored.

**Rumors**: registry `Rumor` identity + per-holder `KnownRumor` variants (text,
confidence, heard-from, tick — the From chain IS the provenance). Deterministic
birth from salient memories about others (`Memory.Subject/Tone`); confidence
decays ×4/5 per hop, floor 25 kills tellability; hearing shifts affection toward
the subject. During primitive talks the executor passes rumors **verbatim** (the
model-free floor); conversations paraphrase (mutation on retell, recorded in the
event). `TellableFor` never surfaces secrets.

**Secrets**: one authored self-rumor per persona (`persona.Secrets`), seeded as
tick-0 events; only the conversation driver may pass one — owner→listener trust ≥
`SecretTrustGate` (700) plus a seeded 1-in-3 roll — after which it spreads like
any rumor.

**Conversations** (`mind/convo.go`): on the executor's `agent.talked` beat, the
driver (slot = 1, immutable snapshot, 6-min deadline) runs 2 utterances per side
(within the grounding's "~5 cap"; ~45 s/utterance is honest 12B pace) plus one
outcome call (gist, float-tolerant tones, rumor paraphrase). Effects land as ONE
atomic `inject_social` batch through the loop's whitelisted door — turns, summary,
dual gist memories, tone edges, at most one rumor. Any failure injects nothing;
the primitive talk stands alone. Replay is model-free.

## Connections

[[executor]] runs the deterministic acts (give/repay/talk/due-check);
[[sim-state-reducer]] carries all social state; [[sim-loop]]'s `inject_social` is
the second injection door beside `inject_intent`; the [[llm-orchestrator]]'s
priority lane keeps dialogue turns from starving behind planner traffic;
[[agent-mind]]'s planner prompts read bonds/debts/reputation/rumors; the scribe
renders the Bonds section into soul.md. TASK-13 (norms/votes) builds on these
edges; TASK-11's chronicle narrates the conversation events.

## Operational notes

First landed conversation (live, gemma4:12b-mlx): Birch — authored as finding
Cedar's silences unbearable — berated Cedar for four turns; both souls got the
gist; tones moved edges to the village's first grudge (trust −24, affection −45).
Engineering findings baked in: chat-while-working (mutual idleness starved the
fabric), planner debounce (trigger feedback loop), conversation priority lane +
worker call cap, float-tolerant tone parsing. Pace at 4x: one conversation ≈ 4
minutes wall, one at a time.
