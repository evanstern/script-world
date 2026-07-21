---
name: social-fabric
description: The conflict engine — directed relation edges, debt ledger with computed reputation, rumors with provenance and mutation, authored secrets, and model-driven conversations injected atomically
kind: component
sources:
  - internal/sim/social.go
  - internal/mind/convo.go
verified_against: a49d615ec26d41ff14784f5a8f03f89d0e6c96f9
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

**Conversations** (`mind/convo.go`, scenes in TASK-22): on the executor's
`agent.talked` beat, the driver (slot = 1, immutable snapshot, 10-min deadline —
sized for a full scene at honest local pace)
forms a **scene**. Since TASK-32 the beat first passes the [[cognition]]
router gate: a scene is the tier's most expensive thought (13 points), and if
it can't land inside its staleness budget at the current speed the encounter
stays a primitive talk with a `cog.outcome{suppressed}` record. An admitted
scene mints a telemetry identity at founding (`conversation-<founding tick>`,
agent = founding speaker) and emits `cog.thought` before the first turn.
The scene is the founding pair plus any awake villager within
`sceneJoinRadius` (2) of the founding speaker, up to `sceneCap` (4). Round-robin
turns, `ConvoTurnsPerSide` (2) each; the snapshot carries each participant's
feelings toward every other, open debts inside the scene, and the last
conversation between the founding pair (from the record ring below). One outcome
call returns gist, 1–3 topic tags, per-participant tones (the pre-TASK-22
`tone_a`/`tone_b` shape still parses), and the rumor paraphrase. Effects land as
ONE atomic `inject_social` batch — turns, summary, and per participant×counterpart
fodder: a gist memory **about** the counterpart (subject-tagged, toned ×30 — a
`TellableFor` gossip seed) and a tone edge per pair, reason-tagged with the first
topic; at most one rumor between the founding pair. The scene's terminal
`cog.outcome{landed}` rides the same batch — the scene and its record land
atomically. Landing is also staleness-enforced (TASK-32): a completed scene
whose wall time overran the conversation class's budget in ticks (the router
admitted it, but the tier ran slower than predicted) injects nothing and
records `cog.outcome{rejected-stale}` with the arithmetic. Any failure —
abandoned turn, unusable outcome call, rejected injection — likewise injects
nothing and records a terminal `cog.outcome{unusable}`;
the primitive talk stands alone. Replay is model-free.

**Conversation records** (TASK-22): `social.conversation` is no longer a reducer
no-op — the payload (`participants`, `topics`, `tones`; empty participants means
the legacy `[a, b]`) appends a `ConvoRecord` to `State.Conversations`, a bounded
ring (`convoRecordCap` 64). `LastConversationBetween` / `LastConversationInvolving`
serve it back to prompts — planner prompts carry a "Last conversation, with X:
<gist>" line, so encounters have continuity instead of amnesia.

## Connections

[[executor]] runs the deterministic acts (give/repay/talk/due-check);
[[sim-state-reducer]] carries all social state; [[sim-loop]]'s `inject_social` is
the second injection door beside `inject_intent`; the [[llm-orchestrator]]'s
priority lane keeps dialogue turns from starving behind planner traffic;
[[agent-mind]]'s planner prompts read bonds/debts/reputation/rumors; the scribe
renders the Bonds section into soul.md. [[governance]] (TASK-13) votes over these
edges and writes violation consequences back into them; TASK-11's chronicle
narrates the conversation events.

## Operational notes

First landed conversation (live, gemma4:12b-mlx): Birch — authored as finding
Cedar's silences unbearable — berated Cedar for four turns; both souls got the
gist; tones moved edges to the village's first grudge (trust −24, affection −45).
Engineering findings baked in: chat-while-working (mutual idleness starved the
fabric), planner debounce (trigger feedback loop), conversation priority lane +
worker call cap, float-tolerant tone parsing. Pace at 4x: one conversation ≈ 4
minutes wall, one at a time.
