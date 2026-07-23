# Contract: Memory Context (Layer 1)

The emission-time contract for situated memories and their rendering. Consumers: executor + convo (emitters), reducer (applier), scribe + prompts/consolidation (readers).

## Emission contract

1. **Every** episodic memory emitted after 019 carries `Where` = the acting agent's position at the emission tick, with `Desc` from `describePlace` (may be `""`).
2. `Why` is set **iff** the completing intent carries a non-empty `Reason` (planner-sourced). Reflex actions, witness memories, and system memories (dreams, consolidation) leave it empty. `Why` is copied verbatim — never paraphrased, truncated only by the existing intent-reason cap upstream.
3. `Conv` is set **iff** the memory summarizes a conversation; its value is the conversation id (`convoCtx.conv` = founding-talk tick) that keys every `social.conversation_turn` event of that scene.
4. Context is baked into `MemoryAddedPayload` **at emission**. Nothing may be derived at render or replay time from anything but the payload/reduced state.

## Situated text grammar (deterministic)

Base template + optional clauses, composed in this exact order:

```
<base text>[ <where-clause>][ — <why-clause>]
where-clause := "at <desc> (<x>,<y>)"  when Desc != ""
              | "at (<x>,<y>)"          when Desc == ""
why-clause   := the intent Reason, verbatim
```

Examples (templates pinned by unit test):
- `Built a fire at the rock outcrop (23,41) — keep the Gru away from camp tonight.`
- `Raised a shelter with my own hands at (7,12).`
- `Talked with Mira and Odo — argued about the storm watch.` (conversation memories keep their gist text; place from the speaking agent's tile; `Conv` carries the ref)

Rules: no fabricated clause when data is absent; punctuation of the base template is preserved (the where-clause splices before a trailing period); the grammar is implemented once (shared helper), not per call site.

## Reducer contract

- `agent.memory_added` copies `Where`/`Why`/`Conv` from payload to `Memory` unchanged. `Tick` still stamped from the event.
- Absent fields stay absent (`nil`/`""`/`0`) — a pre-019 event produces a pre-019-shaped `Memory`.

## Rendering contract (soul.md)

Memory line format (scribe):

```
- **<clock>** (<N>★) <text>[ · at <desc> (x,y) | · at (x,y)][ · why: <reason>][ · [conv <id>]]
```

- Suffixes render only when the field is present — pre-019 memories render byte-identically to today's format.
- The scribe reads ONLY reduced `Memory` fields; it never re-derives place or joins other events.
- soul.md remains a faithful regenerable view: identical state ⇒ identical bytes.

## Transcript retrieval contract

Given `Memory.Conv != 0`: the full transcript is `SELECT` of events where `type = "social.conversation_turn"` and `payload.conv == Conv`, ordered by `seq`; each yields `{Speaker, Listener, Text}`. This query MUST need nothing but the event log (SC-002). The closing `social.conversation` event (same `Conv`) provides gist/participants/tones if wanted.

## Compatibility contract

- All new JSON fields `omitempty`; pre-019 logs replay with zero behavior change and render as before (FR-014, SC-007).
- No existing payload field changes meaning; salience table untouched.
