---
name: social-fabric
description: The conflict engine — directed relation edges, debt ledger with computed reputation, rumors with provenance and mutation, authored secrets, chest-theft consequences, and model-driven conversations injected atomically
kind: component
sources:
  - internal/sim/social.go
  - internal/mind/convo.go
verified_against: 056c53a140df7431739d4d6cd5d727dc96aed001
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

**Ledger**: a give to a starving neighbor — one unit of `Inv.FoodRaw` moves
giver→receiver (spec 012 widened the single `Food` field to a raw/cooked/meals
triplet; giving stays denominated in the least-nutritious raw form) — opens
`Debt{due +2 game days}` (reducer-internal on `social.gave`); a matching
give-back settles it kept. Spec 013 (US1) added a carried-bulk guard: the
executor's `repayable`/`giveable` checks additionally require the receiver have
free bulk (`freeBulk(Inv) > 0`) before offering a give — a starving villager
already at the cap is carrying food and would eat rather than receive — and the
reducer clamps the receive defensively at `bulkCap`, so even a forged over-cap
`social.gave` can't push a recipient over it. The
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

**Theft** (spec 013 US4, FR-011/012, research R5): a non-owner withdrawing from a
builder-owned chest ([[executor]]) is never blocked — the goods already moved —
but always marked, through a companion batch the executor co-emits in the same
tick as the `agent.withdrew`: `social.chest_taken{owner, taker, x, y}` is the
distinct taking record itself (reducer-effect-free, chronicle/TUI material, same
idiom as `social.conversation_turn`); a `social.relation_changed` owner→taker
moves the edge through the same fixed-rule machinery as talk/give/broken-promise,
reason `"theft"`, at `theftTrustDelta` (−120) trust and `theftAffectionDelta`
(−40) affection; the owner (if living) gets a subject-tagged memory of the taker
at `theftMemoryTone` (−60) regardless of distance — a `TellableFor` gossip seed,
the same any-distance exemption a chest owner's "my things were taken" grievance
needs to travel; and every living, awake villager within `witnessRadius` (8) of
the chest, excluding the taker and the owner (who already has the stronger
any-distance memory), gets its own witness memory at the same tone. Since spec
019 (US1) both are built through `situatedMemoryAboutEvent` (memory.go) rather
than the bare `memoryAboutEvent`, so each carries a `Where` situated by the
rememberer's OWN tile — `PlaceAt(s, owner.X, owner.Y)` for the owner,
`PlaceAt(s, witness.X, witness.Y)` for each witness (a witness remembers where
it stood, not where the chest was). Witness memories carry no `Why` — the
witness did not drive the act ([[agent-mind]]'s situated-memory grammar). A
dead owner still gets the record, the relation delta, and the witness memories —
only the owner's own memory is skipped (the dead don't remember; the village
does). Owner withdrawing from their own chest emits `agent.withdrew` alone, no
companion batch (FR-011).

**Conversations** (`mind/convo.go`, scenes in TASK-22): on the executor's
`agent.talked` beat, the driver (slot = 1, immutable snapshot, 10-min deadline —
sized for a full scene at honest local pace)
forms a **scene**. Since TASK-32 the beat first passes the [[cognition]]
router gate: a scene is the costliest conversation-class thought (13 points), and if
it can't land inside its staleness budget at the current speed the encounter
stays a primitive talk with a `cog.outcome{suppressed}` record. An admitted scene also pins its PROVIDER at founding (spec 024 US3,
`Mind.sceneProvider` → the orchestrator's `ResolveProvider(KindConversation)`
dry chain-walk): every utterance and the outcome call stamp the same
`Request.Provider`, so a persona keeps one voice for the whole dialogue even if
a preferable candidate frees up mid-scene — mid-scene failure flows into the
TASK-42 tolerance path, never a re-resolve or provider switch. An admitted
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
topic; at most one rumor between the founding pair. Since spec 019 (US2) each
gist memory carries two situating fields set directly on the payload: `Where`
(`PlaceAt` on the remembering agent's own tile in the mind replica) and `Conv`
(`cc.conv`, the founding-talk tick that keys every `social.conversation_turn` of
the scene), so the full transcript is recoverable from the memory alone via the
log. The gist TEXT is left unchanged — no where/why clause is spliced into a
conversation memory (unlike executor-emitted memories, [[agent-mind]]); the
`Conv` ref IS its situating, and the scribe renders it as a `· [conv <id>]`
suffix. The scene's terminal
`cog.outcome{landed}` rides the same batch — the scene and its record land
atomically. Landing is also staleness-enforced (TASK-32): a completed scene
whose wall time overran the conversation class's budget in ticks (the router
admitted it, but the provider ran slower than predicted) injects nothing and
records `cog.outcome{rejected-stale}` with the arithmetic. Since TASK-42
(specs/011-conversation-robustness) a scene tolerates one bad reply per site
rather than dying on the first: a parse-failed utterance gets one same-speaker
retry (one utterance retry TOTAL per scene — retry-not-skip, preserving the
round-robin transcript), and a parse-failed outcome call gets one re-request;
each consumed retry emits a non-terminal `cog.outcome{retried}` carrying the
failed reply's verbatim text (`raw`, bounded at 2048 bytes, rune-boundary
truncated), and the scene's terminal event carries `retried: true`. Before
retrying, `parse.go`'s `lenientOutcome` repairs the observed unquoted-gist
shape with no model call at all. Transport/admission errors are NEVER retried
— backpressure stays authoritative — and a second parse failure at either
site abandons: the scene injects nothing and records a terminal
`cog.outcome{unusable}` (with `raw` when the killer was a parse failure); the
primitive talk stands alone. The stale-at-landing check runs after any retry,
so retry wall-time cannot smuggle a stale scene past its budget. The outcome
prompt states that `gist`/`retold` must be double-quoted JSON strings. Replay
is model-free.

**Conversation records** (TASK-22): `social.conversation` is no longer a reducer
no-op — the payload (`participants`, `topics`, `tones`; empty participants means
the legacy `[a, b]`) appends a `ConvoRecord` to `State.Conversations`, a bounded
ring (`convoRecordCap` 64). `LastConversationBetween` / `LastConversationInvolving`
serve it back to prompts — planner prompts carry a "Last conversation, with X:
<gist>" line, so encounters have continuity instead of amnesia.

## Connections

[[executor]] runs the deterministic acts (give/repay/talk/due-check/theft);
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
